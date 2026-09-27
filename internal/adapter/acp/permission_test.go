package acp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	sdk "github.com/coder/acp-go-sdk"
	"github.com/yukihito-jokyu/TEJUN/internal/application"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/agentconnection"
)

type permissionCaptureStore struct {
	application.PermissionRepository
	registered chan string
}

type permissionCancelStore struct {
	permissionCaptureStore
	completed chan bool
}

func (s permissionCancelStore) ClaimPermission(context.Context, application.PermissionClaim) (
	application.MutationResult[application.PermissionResponseResult], bool, error,
) {
	return application.MutationResult[application.PermissionResponseResult]{}, true, nil
}

func (s permissionCancelStore) CompletePermission(
	_ context.Context, _, _ string, sent bool, _ time.Time,
) error {
	s.completed <- sent

	return nil
}

func (s permissionCaptureStore) RegisterPermission(_ context.Context, p application.IncomingPermission) error {
	s.registered <- p.ID
	return nil
}

type controlledAfterContext struct {
	context.Context
	callback chan func()
}

func (c controlledAfterContext) Value(any) any { return nil }

func (c controlledAfterContext) AfterFunc(f func()) func() bool {
	c.callback <- f
	return func() bool { return true }
}

type permissionRejectStore struct {
	application.PermissionRepository
}

func (permissionRejectStore) RegisterPermission(context.Context, application.IncomingPermission) error {
	panic("unsafe permission was registered")
}

func TestCommandPermissionRejectsWithoutRegistration(t *testing.T) {
	for _, tc := range []struct {
		name, rpcID string
		reject      bool
	}{
		{"string ID", `"request"`, true},
		{"number ID", `42`, true},
		{"no reject option", `43`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := NewManager(nil, time.Now, func() string { return "unused" })
			defer func() {
				delete(m.sessions, "app")
				_ = m.Close()
			}()

			m.permissionStore = permissionRejectStore{}
			m.sessions["app"] = &session{
				agentSessionID: "agent", pendingExecutionID: "execution",
				probe: agentconnection.Probe{ProcessGeneration: 1},
			}
			execute := sdk.ToolKindExecute

			options := []sdk.PermissionOption{{OptionId: "allow", Kind: sdk.PermissionOptionKindAllowOnce}}
			if tc.reject {
				options = append(options, sdk.PermissionOption{
					OptionId: "deny", Kind: sdk.PermissionOptionKindRejectOnce,
				})
			}

			response, err := (client{manager: m, generation: 1}).requestPermission(context.Background(),
				sdk.RequestPermissionRequest{
					Meta: map[string]any{permissionRPCIDMeta: tc.rpcID}, SessionId: "agent",
					ToolCall: sdk.ToolCallUpdate{Kind: &execute},
					Options:  options,
				})

			selected := response.Outcome.Selected
			if tc.reject && (err != nil || selected == nil || selected.OptionId != "deny") ||
				!tc.reject && (err == nil || selected != nil) {
				t.Fatalf("response = %+v, error = %v", response, err)
			}

			if len(m.permissions) != 0 {
				t.Fatal("unsafe permission was offered to the user")
			}
		})
	}
}

func TestSafePermissionTool(t *testing.T) {
	read, execute, unknown := sdk.ToolKindRead, sdk.ToolKindExecute, sdk.ToolKindOther
	for _, tc := range []struct {
		name string
		tool sdk.ToolCallUpdate
		want bool
	}{
		{"read", sdk.ToolCallUpdate{Kind: &read}, true},
		{"execute", sdk.ToolCallUpdate{Kind: &execute}, false},
		{"unknown", sdk.ToolCallUpdate{Kind: &unknown}, false},
		{"missing kind", sdk.ToolCallUpdate{}, false},
		{"raw command", sdk.ToolCallUpdate{Kind: &read, RawInput: map[string]any{"command": "rm -rf"}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := safePermissionTool(tc.tool); got != tc.want {
				t.Fatalf("safePermissionTool() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPermissionReaderPreservesRPCID(t *testing.T) {
	tests := []struct{ name, id string }{{"string", `"req-1"`}, {"number", `12345678901234567890`}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := `{"jsonrpc":"2.0","id":` + tt.id + `,"method":"session/request_permission","params":{"sessionId":"s","options":[],"toolCall":{"toolCallId":"t"}}}` + "\n"

			output, err := io.ReadAll(permissionReader(strings.NewReader(input)))
			if err != nil {
				t.Fatal(err)
			}

			var envelope struct {
				ID     json.RawMessage `json:"id"`
				Params struct {
					Meta map[string]string `json:"_meta"`
				} `json:"params"`
			}
			if err := json.Unmarshal(output, &envelope); err != nil {
				t.Fatal(err)
			}

			if string(envelope.ID) != tt.id || envelope.Params.Meta[permissionRPCIDMeta] != tt.id {
				t.Fatalf("ID changed: %s", output)
			}
		})
	}
}

func TestPermissionReaderIgnoresSpoofedMetaID(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":"real","method":"session/request_permission","params":{"_meta":{"tejun.rpcRequestIdJSON":"\"fake\""},"sessionId":"s","options":[],"toolCall":{"toolCallId":"t"}}}` + "\n"

	output, err := io.ReadAll(permissionReader(strings.NewReader(input)))
	if err != nil {
		t.Fatal(err)
	}

	var envelope struct {
		Params struct {
			Meta map[string]string `json:"_meta"`
		} `json:"params"`
	}
	if err := json.Unmarshal(output, &envelope); err != nil {
		t.Fatal(err)
	}

	if got := envelope.Params.Meta[permissionRPCIDMeta]; got != `"real"` {
		t.Fatalf("RPC ID = %q", got)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("closed") }

type gatedPermissionWriter struct {
	started chan struct{}
	release chan struct{}
	n       int
	err     error
}

func (w gatedPermissionWriter) Write(data []byte) (int, error) {
	close(w.started)
	<-w.release

	if w.n >= 0 {
		return w.n, w.err
	}

	return len(data), w.err
}

func TestPermissionRequestCancelBeforeWrite(t *testing.T) {
	for _, tc := range []struct {
		name, cleanup string
	}{
		{"late frame", "frame"},
		{"process closed", "process"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := NewManager(nil, time.Now, func() string { return "p" })
			defer func() { _ = m.Close() }()

			store := permissionCancelStore{
				permissionCaptureStore: permissionCaptureStore{registered: make(chan string, 1)},
				completed:              make(chan bool, 1),
			}
			m.permissionStore = store

			reader, writer, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = reader.Close() }()

			done := make(chan struct{})
			close(done)
			m.sessions["app"] = &session{
				process:        &process{cmd: &exec.Cmd{}, stdin: writer, done: done, cancel: func() {}},
				agentSessionID: "agent", pendingExecutionID: "execution",
				probe: agentconnection.Probe{ProcessGeneration: 1},
			}

			base, cancel := context.WithCancel(context.Background())
			defer cancel()

			requestCtx := controlledAfterContext{Context: base, callback: make(chan func(), 1)}
			requestResult := make(chan error, 1)

			read := sdk.ToolKindRead

			go func() {
				_, err := (client{manager: m, generation: 1}).requestPermission(requestCtx,
					sdk.RequestPermissionRequest{
						Meta: map[string]any{permissionRPCIDMeta: `"r"`}, SessionId: "agent",
						ToolCall: sdk.ToolCallUpdate{Kind: &read},
						Options:  []sdk.PermissionOption{{OptionId: "allow", Kind: sdk.PermissionOptionKindAllowOnce}},
					})
				requestResult <- err
			}()

			id := <-store.registered
			callback := <-requestCtx.callback
			responseResult := make(chan error, 1)

			go func() {
				_, err := application.NewExecutionPermission(store, m, time.Now).Respond(context.Background(),
					application.PermissionClaim{SessionID: "app", PermissionRequestID: id, OptionID: "allow"})
				responseResult <- err
			}()

			if err := <-requestResult; err != nil {
				t.Fatal(err)
			}

			cancel()
			callback()

			select {
			case err := <-responseResult:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("response error = %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("response remained blocked")
			}

			if sent := <-store.completed; sent {
				t.Fatal("canceled delivery was marked sent")
			}

			var target bytes.Buffer

			permissionWriter := &permissionWriter{target: &target, manager: m, generation: 1}
			if tc.cleanup == "frame" {
				_, err = permissionWriter.Write([]byte(`{"jsonrpc":"2.0","id":"r","result":{}}` + "\n"))
				if err != nil || target.Len() != 0 {
					t.Fatalf("late response delivered: bytes=%d error=%v", target.Len(), err)
				}
			} else if err := m.closeSession("app"); err != nil {
				t.Fatal(err)
			}

			m.mu.Lock()
			defer m.mu.Unlock()

			if len(m.permissions) != 0 {
				t.Fatalf("pending permissions remain: %d", len(m.permissions))
			}
		})
	}
}

func TestPermissionCancelDuringWrite(t *testing.T) {
	frame := []byte(`{"jsonrpc":"2.0","id":"r","result":{}}` + "\n")

	for _, tc := range []struct {
		name string
		n    int
		err  error
		want error
	}{
		{"full write", -1, nil, nil},
		{"short write", 1, nil, io.ErrShortWrite},
		{"write error", 0, io.ErrClosedPipe, io.ErrClosedPipe},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := NewManager(nil, time.Now, func() string { return "unused" })
			defer func() { _ = m.Close() }()

			p := &pendingPermission{
				rpcID: `"r"`, generation: 1, options: map[string]struct{}{"allow": {}},
				answer: make(chan string, 1), written: make(chan error, 1),
			}
			m.permissions["p"] = p

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			result := make(chan error, 1)
			go func() { result <- m.RespondPermission(ctx, "p", "allow", 1) }()

			<-p.answer

			target := gatedPermissionWriter{
				started: make(chan struct{}),
				release: make(chan struct{}),
				n:       tc.n,
				err:     tc.err,
			}
			writer := &permissionWriter{target: target, manager: m, generation: 1}
			writeDone := make(chan struct{})

			go func() { _, _ = writer.Write(frame); close(writeDone) }()

			<-target.started
			cancel()
			close(target.release)
			<-writeDone

			if err := <-result; !errors.Is(err, tc.want) || (tc.want == nil && err != nil) {
				t.Fatalf("response error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestPermissionRequestCancelDuringWrite(t *testing.T) {
	frame := []byte(`{"jsonrpc":"2.0","id":"r","result":{}}` + "\n")

	for _, tc := range []struct {
		name string
		n    int
		err  error
		want error
	}{
		{"full write", -1, nil, nil},
		{"short write", 1, nil, io.ErrShortWrite},
		{"write error", 0, io.ErrClosedPipe, io.ErrClosedPipe},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := NewManager(nil, time.Now, func() string { return "p" })
			defer func() {
				delete(m.sessions, "app")
				_ = m.Close()
			}()

			store := permissionCaptureStore{registered: make(chan string, 1)}
			m.permissionStore = store
			m.sessions["app"] = &session{
				agentSessionID: "agent", pendingExecutionID: "execution",
				probe: agentconnection.Probe{ProcessGeneration: 1},
			}

			base, cancel := context.WithCancel(context.Background())
			defer cancel()

			requestCtx := controlledAfterContext{Context: base, callback: make(chan func(), 1)}
			requestResult := make(chan error, 1)
			read := sdk.ToolKindRead

			go func() {
				_, err := (client{manager: m, generation: 1}).requestPermission(requestCtx,
					sdk.RequestPermissionRequest{
						Meta: map[string]any{permissionRPCIDMeta: `"r"`}, SessionId: "agent",
						ToolCall: sdk.ToolCallUpdate{Kind: &read},
						Options:  []sdk.PermissionOption{{OptionId: "allow", Kind: sdk.PermissionOptionKindAllowOnce}},
					})
				requestResult <- err
			}()

			id := <-store.registered
			callback := <-requestCtx.callback

			wireResult := make(chan error, 1)
			go func() { wireResult <- m.RespondPermission(context.Background(), id, "allow", 1) }()

			if err := <-requestResult; err != nil {
				t.Fatal(err)
			}

			target := gatedPermissionWriter{
				started: make(chan struct{}), release: make(chan struct{}), n: tc.n, err: tc.err,
			}
			writer := &permissionWriter{target: target, manager: m, generation: 1}
			writeDone := make(chan struct{})

			go func() { _, _ = writer.Write(frame); close(writeDone) }()

			<-target.started
			cancel()
			callback()
			m.mu.Lock()
			_, retained := m.permissions[id]
			m.mu.Unlock()

			if !retained {
				t.Fatal("pending permission lost during write")
			}

			close(target.release)
			<-writeDone

			if err := <-wireResult; !errors.Is(err, tc.want) || (tc.want == nil && err != nil) {
				t.Fatalf("response error = %v, want %v", err, tc.want)
			}

			m.mu.Lock()
			defer m.mu.Unlock()

			if len(m.permissions) != 0 {
				t.Fatalf("pending permissions remain: %d", len(m.permissions))
			}
		})
	}
}

func TestPermissionBlockedWriteIsIndeterminate(t *testing.T) {
	m := NewManager(nil, time.Now, func() string { return "unused" })
	defer func() { _ = m.Close() }()

	p := &pendingPermission{
		rpcID: `"r"`, generation: 1, options: map[string]struct{}{"allow": {}},
		answer: make(chan string, 1), written: make(chan error, 1),
	}
	m.permissions["p"] = p

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	result := make(chan error, 1)
	go func() { result <- m.RespondPermission(ctx, "p", "allow", 1) }()

	<-p.answer

	target := gatedPermissionWriter{started: make(chan struct{}), release: make(chan struct{}), n: -1}
	writer := &permissionWriter{target: target, manager: m, generation: 1}
	writeDone := make(chan struct{})

	go func() {
		_, _ = writer.Write([]byte(`{"jsonrpc":"2.0","id":"r","result":{}}` + "\n"))

		close(writeDone)
	}()

	<-target.started
	cancel()

	if err := <-result; !errors.Is(err, application.ErrPermissionDeliveryIndeterminate) {
		t.Fatalf("response error = %v", err)
	}

	close(target.release)
	<-writeDone
}

func TestPermissionWriterAndOneShot(t *testing.T) {
	tests := []struct {
		name      string
		target    io.Writer
		wantError bool
	}{{"success", &bytes.Buffer{}, false}, {"write error", failingWriter{}, true}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewManager(nil, time.Now, func() string { return "unused" })
			defer func() { _ = m.Close() }()

			p := &pendingPermission{
				rpcID:      `"r"`,
				generation: 1,
				options:    map[string]struct{}{"allow": {}},
				answer:     make(chan string, 1),
				written:    make(chan error, 1),
			}
			m.permissions["p"] = p

			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			result := make(chan error, 1)
			go func() { result <- m.RespondPermission(ctx, "p", "allow", 1) }()

			if got := <-p.answer; got != "allow" {
				t.Fatalf("answer = %s", got)
			}

			if err := m.RespondPermission(ctx, "p", "allow", 1); err == nil {
				t.Fatal("duplicate response accepted")
			}

			writer := &permissionWriter{target: tt.target, manager: m, generation: 1}
			_, _ = writer.Write([]byte(`{"jsonrpc":"2.0","id":"r","result":{"outcome":"selected"}}` + "\n"))

			if err := <-result; (err != nil) != tt.wantError {
				t.Fatalf("response error = %v", err)
			}
		})
	}
}

func TestPermissionWriterMatchesConcurrentRequests(t *testing.T) {
	m := NewManager(nil, time.Now, func() string { return "unused" })
	defer func() { _ = m.Close() }()

	first := &pendingPermission{rpcID: `"first"`, generation: 1, claimed: true, written: make(chan error, 1)}
	second := &pendingPermission{rpcID: `42`, generation: 1, claimed: true, written: make(chan error, 1)}
	m.permissions["p1"] = first
	m.permissions["p2"] = second

	writer := &permissionWriter{target: &bytes.Buffer{}, manager: m, generation: 1}
	if _, err := writer.Write([]byte(`{"jsonrpc":"2.0","id":42,"result":{}}` + "\n")); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-second.written:
		if err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatal("numeric ID was not acknowledged")
	}

	select {
	case <-first.written:
		t.Fatal("unrelated request was acknowledged")
	default:
	}
}

func TestRespondPermissionWaitEndsWithoutWrite(t *testing.T) {
	tests := []struct {
		name string
		stop func(*Manager, context.CancelFunc)
	}{
		{"caller canceled", func(_ *Manager, cancel context.CancelFunc) { cancel() }},
		{"connection closed", func(m *Manager, _ context.CancelFunc) { _ = m.Close() }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewManager(nil, time.Now, func() string { return "unused" })
			defer func() { _ = m.Close() }()

			p := &pendingPermission{
				rpcID:      `"r"`,
				generation: 1,
				options:    map[string]struct{}{"allow": {}},
				answer:     make(chan string, 1),
				written:    make(chan error, 1),
			}
			m.permissions["p"] = p

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			result := make(chan error, 1)
			go func() { result <- m.RespondPermission(ctx, "p", "allow", 1) }()

			<-p.answer
			tt.stop(m, cancel)

			select {
			case err := <-result:
				if err == nil {
					t.Fatal("missing cancellation error")
				}
			case <-time.After(time.Second):
				t.Fatal("response remained blocked")
			}

			var target bytes.Buffer

			writer := &permissionWriter{target: &target, manager: m, generation: 1}

			frame := []byte(`{"jsonrpc":"2.0","id":"r","result":{}}` + "\n")
			if _, err := writer.Write(frame); err != nil {
				t.Fatal(err)
			}

			if target.Len() != 0 {
				t.Fatal("late response was written")
			}
		})
	}
}

func TestPermissionReaderDisconnect(t *testing.T) {
	_, err := io.ReadAll(permissionReader(strings.NewReader("")))
	if err != nil {
		t.Fatal(err)
	}
}
