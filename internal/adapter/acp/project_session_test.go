package acp

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	sdk "github.com/coder/acp-go-sdk"
	"github.com/yukihito-jokyu/TEJUN/internal/application"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/agentconnection"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/shared"
)

func TestProjectRecoveryMode(t *testing.T) {
	resume := sdk.AgentCapabilities{
		LoadSession:         true,
		SessionCapabilities: sdk.SessionCapabilities{Resume: &sdk.SessionResumeCapabilities{}},
	}
	load := sdk.AgentCapabilities{LoadSession: true}

	tests := []struct {
		name, strategy, previous, want string
		capabilities                   sdk.AgentCapabilities
	}{
		{name: "new requested", strategy: "new_session", previous: "prior", capabilities: resume, want: "new"},
		{name: "resume", strategy: "auto", previous: "prior", capabilities: resume, want: "resume"},
		{name: "load fallback", strategy: "auto", previous: "prior", capabilities: load, want: "load"},
		{name: "new fallback", strategy: "auto", previous: "prior", want: "new"},
		{name: "no prior", strategy: "auto", capabilities: resume, want: "new"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := projectRecoveryMode(test.strategy, test.previous, test.capabilities); got != test.want {
				t.Fatalf("mode = %s, want %s", got, test.want)
			}
		})
	}
}

func TestConnectProjectSessionTimeout(t *testing.T) {
	manager := NewManager(nil, time.Now, func() string { return "unused" })
	defer func() { _ = manager.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, err := manager.ConnectProject(ctx, application.ClaimedProjectJob{
		SessionID: "session", WorkspacePath: t.TempDir(),
		Connection: agentconnection.ConnectionInput{
			Command: os.Args[0], Args: []string{"-test.run=TestFakeACPProcess"},
			EnvironmentOverrides: []agentconnection.EnvironmentVariable{
				{Name: "GO_WANT_FAKE_ACP", Value: "1"}, {Name: "FAKE_ACP_MODE", Value: "session_timeout"},
			},
		},
	})

	var appErr *shared.Error
	if !errors.As(err, &appErr) || appErr.Code != "acp_timeout" {
		t.Fatalf("error = %v, want acp_timeout", err)
	}
}
