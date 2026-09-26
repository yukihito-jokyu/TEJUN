package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	sdk "github.com/coder/acp-go-sdk"
	"github.com/yukihito-jokyu/TEJUN/internal/application"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/agentconnection"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/shared"
)

func TestManagerACPContractAndCleanup(t *testing.T) {
	ids := []string{"probe-1"}
	manager := NewManager(nil, time.Now, func() string {
		id := ids[0]
		ids = ids[1:]

		return id
	})

	input := application.CheckAuthenticationInput{Connection: agentconnection.ConnectionInput{
		Command: os.Args[0], Args: []string{"-test.run=TestFakeACPProcess"}, Transport: agentconnection.TransportStdio,
		EnvironmentOverrides: []agentconnection.EnvironmentVariable{{Name: "GO_WANT_FAKE_ACP", Value: "1"}},
	}, OperationID: "operation-1"}

	result, err := manager.Check(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}

	if result.Probe.ID != "probe-1" || result.Probe.ProtocolVersion != fmt.Sprint(sdk.ProtocolVersionNumber) {
		t.Fatalf("unexpected probe: %+v", result.Probe)
	}

	if result.Probe.ResolvedExecutablePath == "" {
		t.Fatal("resolved executable path is empty")
	}

	if len(result.AuthMethods) != 1 || result.AuthMethods[0].Type != "agent" {
		t.Fatalf("unexpected auth methods: %+v", result.AuthMethods)
	}

	if result.Probe.AuthState != agentconnection.AuthUnknown {
		t.Fatalf("initialize auth state = %q, want unknown", result.Probe.AuthState)
	}

	repeated, err := manager.Check(context.Background(), input)
	if err != nil || repeated.Probe.ID != result.Probe.ID {
		t.Fatalf("deduplicated Check() = (%+v, %v)", repeated, err)
	}

	conflicting := input

	conflicting.Connection.Command = "different"
	if _, err := manager.Check(context.Background(), conflicting); err == nil {
		t.Fatal("expected operation id conflict")
	}

	methodType, err := manager.ResolveAuthMethod(context.Background(), result.Probe.ID, "agent-login")
	if err != nil || methodType != "agent" {
		t.Fatalf("ResolveAuthMethod() = (%q, %v)", methodType, err)
	}

	if err := manager.Execute(context.Background(), application.ClaimedAgentJob{
		Kind: agentconnection.JobAuthenticate, TargetID: result.Probe.ID, AuthMethodID: "agent-login",
	}); err != nil {
		t.Fatal(err)
	}

	authenticated, err := manager.Get(context.Background(), result.Probe.ID)
	if err != nil || authenticated.AuthState != agentconnection.AuthAuthenticated {
		t.Fatalf("authenticated probe = (%+v, %v)", authenticated, err)
	}

	if err := manager.Execute(context.Background(), application.ClaimedAgentJob{
		Kind: agentconnection.JobLogout, TargetID: result.Probe.ID,
	}); err != nil {
		t.Fatal(err)
	}

	if err := manager.Close(); err != nil {
		t.Fatal(err)
	}

	if len(manager.sessions) != 0 {
		t.Fatalf("sessions were not cleaned up: %d", len(manager.sessions))
	}

	if len(manager.checks) != 0 {
		t.Fatalf("checks were not cleaned up: %d", len(manager.checks))
	}
}

func TestFingerprintDoesNotExposeSecret(t *testing.T) {
	manager := NewManager(nil, time.Now, func() string { return "unused" })
	input := agentconnection.ConnectionInput{
		Command: "agent", Args: []string{"--stdio"},
		EnvironmentOverrides: []agentconnection.EnvironmentVariable{{Name: "TOKEN", Value: "top-secret"}},
	}

	got := manager.Fingerprint(input)
	if got == "" || got == "top-secret" {
		t.Fatalf("unexpected fingerprint %q", got)
	}
}

func TestManagerInitializeWithoutAuthIsNotRequired(t *testing.T) {
	manager := NewManager(nil, time.Now, func() string { return "no-auth-probe" })

	result, err := manager.Check(context.Background(), application.CheckAuthenticationInput{
		Connection: agentconnection.ConnectionInput{
			Command: os.Args[0], Args: []string{"-test.run=TestFakeACPProcess"},
			EnvironmentOverrides: []agentconnection.EnvironmentVariable{
				{Name: "GO_WANT_FAKE_ACP", Value: "1"}, {Name: "FAKE_ACP_MODE", Value: "noauth"},
			},
		},
		OperationID: "no-auth-operation",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = manager.Close() }()

	if result.Probe.AuthState != agentconnection.AuthNotRequired {
		t.Fatalf("auth state = %q, want not_required", result.Probe.AuthState)
	}
}

func TestSessionCleanupOnlyClosesItsOwner(t *testing.T) {
	ids := []string{"old-probe", "new-probe"}
	manager := NewManager(nil, time.Now, func() string {
		id := ids[0]
		ids = ids[1:]

		return id
	})
	input := application.CheckAuthenticationInput{Connection: agentconnection.ConnectionInput{
		Command: os.Args[0], Args: []string{"-test.run=TestFakeACPProcess"},
		EnvironmentOverrides: []agentconnection.EnvironmentVariable{
			{Name: "GO_WANT_FAKE_ACP", Value: "1"}, {Name: "FAKE_ACP_MODE", Value: "noauth"},
		},
	}}

	if _, err := manager.Check(context.Background(), input); err != nil {
		t.Fatal(err)
	}

	if _, err := manager.Check(context.Background(), input); err != nil {
		t.Fatal(err)
	}

	manager.mu.Lock()
	oldSession := manager.sessions["old-probe"]
	newSession := manager.sessions["new-probe"]
	delete(manager.sessions, "new-probe")

	newSession.probe.ID = "old-probe"
	manager.sessions["old-probe"] = newSession
	manager.mu.Unlock()

	if err := manager.closeOwnedSession("old-probe", oldSession); err != nil {
		t.Fatal(err)
	}

	if _, err := manager.Get(context.Background(), "old-probe"); err != nil {
		t.Fatalf("stale cleanup closed replacement: %v", err)
	}

	if err := oldSession.process.close(); err != nil {
		t.Fatal(err)
	}

	if err := manager.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestManagerProbeFailuresCleanup(t *testing.T) {
	tests := []struct {
		name     string
		mode     string
		timeout  time.Duration
		wantCode string
	}{
		{name: "incompatible protocol", mode: "incompatible", timeout: time.Second, wantCode: "acp_not_compatible"},
		{name: "timeout", mode: "timeout", timeout: 20 * time.Millisecond, wantCode: "acp_timeout"},
		{name: "cancelled", mode: "timeout", timeout: 0, wantCode: "operation_cancelled"},
		{name: "abnormal exit", mode: "exit", timeout: time.Second, wantCode: "agent_process_exited"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := NewManager(nil, time.Now, func() string { return "failed-probe" })

			ctx, cancel := context.WithCancel(context.Background())
			if tt.timeout == 0 {
				cancel()
			} else {
				ctx, cancel = context.WithTimeout(ctx, tt.timeout)
				defer cancel()
			}

			_, err := manager.Check(
				ctx,
				application.CheckAuthenticationInput{Connection: agentconnection.ConnectionInput{
					Command: os.Args[0], Args: []string{"-test.run=TestFakeACPProcess"},
					EnvironmentOverrides: []agentconnection.EnvironmentVariable{
						{Name: "GO_WANT_FAKE_ACP", Value: "1"}, {Name: "FAKE_ACP_MODE", Value: tt.mode},
					},
				}},
			)
			if err == nil {
				t.Fatal("expected probe error")
			}

			var appErr *shared.Error
			if !errors.As(err, &appErr) || appErr.Code != tt.wantCode {
				t.Fatalf("error=%v, want code %q", err, tt.wantCode)
			}

			if len(manager.sessions) != 0 {
				t.Fatalf("sessions were not cleaned up: %d", len(manager.sessions))
			}
		})
	}
}

func TestManagerCloseCancelsInitializingProcess(t *testing.T) {
	manager := NewManager(nil, time.Now, func() string { return "unused" })
	done := make(chan error, 1)

	go func() {
		_, err := manager.Check(context.Background(), application.CheckAuthenticationInput{
			Connection: agentconnection.ConnectionInput{
				Command: os.Args[0], Args: []string{"-test.run=TestFakeACPProcess"},
				EnvironmentOverrides: []agentconnection.EnvironmentVariable{
					{Name: "GO_WANT_FAKE_ACP", Value: "1"}, {Name: "FAKE_ACP_MODE", Value: "timeout"},
				},
			},
		})
		done <- err
	}()

	time.Sleep(20 * time.Millisecond)

	if err := manager.Close(); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-done:
		var appErr *shared.Error
		if !errors.As(err, &appErr) || appErr.Code != "operation_cancelled" {
			t.Fatalf("error=%v, want operation_cancelled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("initializing process was not cancelled")
	}
}

func TestFakeACPProcess(t *testing.T) {
	if os.Getenv("GO_WANT_FAKE_ACP") != "1" {
		return
	}

	reader := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)

	for reader.Scan() {
		var request struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      json.RawMessage `json:"id"`
			Method  string          `json:"method"`
		}
		if err := json.Unmarshal(reader.Bytes(), &request); err != nil {
			os.Exit(2)
		}

		var result any = struct{}{}

		if request.Method == "initialize" {
			if os.Getenv("FAKE_ACP_MODE") == "exit" {
				os.Exit(4)
			}

			if os.Getenv("FAKE_ACP_MODE") == "timeout" {
				continue
			}

			protocolVersion := sdk.ProtocolVersionNumber
			if os.Getenv("FAKE_ACP_MODE") == "incompatible" {
				protocolVersion++
			}

			authMethods := []map[string]any{{"id": "agent-login", "name": "Agent login"}}
			if os.Getenv("FAKE_ACP_MODE") == "noauth" {
				authMethods = []map[string]any{}
			}

			result = map[string]any{
				"protocolVersion": protocolVersion,
				"authMethods":     authMethods,
			}
		}

		if err := encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result}); err != nil {
			os.Exit(3)
		}
	}

	os.Exit(0)
}
