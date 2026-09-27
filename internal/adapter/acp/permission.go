package acp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"time"

	sdk "github.com/coder/acp-go-sdk"
	"github.com/yukihito-jokyu/TEJUN/internal/application"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/execution"
)

const permissionRPCIDMeta = "tejun.rpcRequestIdJSON"

const permissionWriteWait = 5 * time.Second

type pendingPermission struct {
	rpcID      string
	generation int64
	options    map[string]struct{}
	answer     chan string
	written    chan error
	claimed    bool
	abandoned  bool
	started    bool
}

// SetPermissionRepository connects the durable permission store before starting an Agent session.
func (m *Manager) SetPermissionRepository(store application.PermissionRepository) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.permissionStore = store
}

func (m *Manager) Generation(_ context.Context, sessionID string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	s := m.sessions[sessionID]
	if s == nil || s.agentSessionID == "" {
		return 0, errors.New("agent session is disconnected")
	}

	return s.probe.ProcessGeneration, nil
}

func (m *Manager) RespondPermission(ctx context.Context, id, option string, generation int64) error {
	m.mu.Lock()

	p := m.permissions[id]
	if p == nil || p.generation != generation || p.claimed {
		m.mu.Unlock()
		return errors.New("permission request is unavailable")
	}

	if _, ok := p.options[option]; !ok {
		m.mu.Unlock()
		return errors.New("permission option was not offered")
	}

	p.claimed = true
	m.mu.Unlock()

	select {
	case p.answer <- option:
	case <-ctx.Done():
		return ctx.Err()
	}

	select {
	case err := <-p.written:
		return err
	case <-ctx.Done():
		return m.finishCanceledPermission(p, ctx.Err())
	case <-m.lifecycle.Done():
		return m.finishCanceledPermission(p, m.lifecycle.Err())
	}
}

func (m *Manager) finishCanceledPermission(p *pendingPermission, canceled error) error {
	m.mu.Lock()
	if !p.started {
		p.abandoned = true
		m.mu.Unlock()

		return canceled
	}
	m.mu.Unlock()

	timer := time.NewTimer(permissionWriteWait)
	defer timer.Stop()

	select {
	case err := <-p.written:
		return err
	case <-timer.C:
		select {
		case err := <-p.written:
			return err
		default:
		}

		return application.ErrPermissionDeliveryIndeterminate
	}
}

func (c client) requestPermission(
	ctx context.Context,
	request sdk.RequestPermissionRequest,
) (sdk.RequestPermissionResponse, error) {
	raw, ok := request.Meta[permissionRPCIDMeta].(string)
	if !ok || !validRPCID([]byte(raw)) {
		return sdk.RequestPermissionResponse{}, errors.New("permission request ID is missing")
	}

	c.manager.mu.Lock()

	var appSessionID, executionID, runID string

	for id, s := range c.manager.sessions {
		if s.probe.ProcessGeneration == c.generation && s.agentSessionID == request.SessionId {
			appSessionID, executionID, runID = id, s.pendingExecutionID, s.pendingRunID
			break
		}
	}

	store := c.manager.permissionStore
	c.manager.mu.Unlock()

	if appSessionID == "" || executionID == "" || store == nil {
		return sdk.RequestPermissionResponse{}, errors.New("permission session is unavailable")
	}

	// ACP does not provide an execution-time command hook, so command permissions cannot be bound to a digest.
	if !safePermissionTool(request.ToolCall) {
		for _, option := range request.Options {
			if option.OptionId != "" && (option.Kind == sdk.PermissionOptionKindRejectOnce ||
				option.Kind == sdk.PermissionOptionKindRejectAlways) {
				return sdk.RequestPermissionResponse{Outcome: sdk.RequestPermissionOutcome{
					Selected: &sdk.RequestPermissionOutcomeSelected{Outcome: "selected", OptionId: option.OptionId},
				}}, nil
			}
		}

		return sdk.RequestPermissionResponse{}, errors.New("command permission cannot be granted")
	}

	options := make([]execution.PermissionOption, 0, len(request.Options))

	offered := make(map[string]struct{}, len(request.Options))
	for _, option := range request.Options {
		id := string(option.OptionId)
		if id == "" {
			return sdk.RequestPermissionResponse{}, errors.New("permission option ID is empty")
		}

		if _, exists := offered[id]; exists {
			return sdk.RequestPermissionResponse{}, errors.New("duplicate permission option")
		}

		offered[id] = struct{}{}
		options = append(options, execution.PermissionOption{ID: id, Name: option.Name, Kind: string(option.Kind)})
	}

	id := c.manager.newID()
	p := &pendingPermission{
		rpcID:      raw,
		generation: c.generation,
		options:    offered,
		answer:     make(chan string, 1),
		written:    make(chan error, 1),
	}
	c.manager.mu.Lock()
	for _, existing := range c.manager.permissions {
		if existing.generation == c.generation && existing.rpcID == raw {
			c.manager.mu.Unlock()
			return sdk.RequestPermissionResponse{}, errors.New("duplicate permission request ID")
		}
	}

	c.manager.permissions[id] = p
	c.manager.mu.Unlock()
	context.AfterFunc(ctx, func() {
		c.manager.mu.Lock()
		if !p.started {
			if p.claimed {
				p.abandoned = true
				// 遅着frameを捨てるかprocessが閉じるまでRPC IDを保持する。
				select {
				case p.written <- ctx.Err():
				default:
				}
			} else {
				delete(c.manager.permissions, id)
			}
		}
		c.manager.mu.Unlock()
	})

	incoming := application.IncomingPermission{
		ID: id, ExecutionID: executionID, SessionID: appSessionID, RunID: runID,
		ToolCallID: string(request.ToolCall.ToolCallId), Title: value(request.ToolCall.Title),
		AgentSessionID: string(request.SessionId), RPCRequestIDJSON: raw,
		ProcessGeneration: c.generation, Options: options, RequestedAt: c.manager.now(),
	}
	if err := store.RegisterPermission(ctx, incoming); err != nil {
		c.manager.mu.Lock()
		delete(c.manager.permissions, id)
		c.manager.mu.Unlock()

		return sdk.RequestPermissionResponse{}, err
	}

	select {
	case choice := <-p.answer:
		return sdk.RequestPermissionResponse{
			Outcome: sdk.RequestPermissionOutcome{
				Selected: &sdk.RequestPermissionOutcomeSelected{
					Outcome:  "selected",
					OptionId: sdk.PermissionOptionId(choice),
				},
			},
		}, nil
	case <-ctx.Done():
		c.manager.mu.Lock()
		if p.claimed {
			p.abandoned = true
			select {
			case p.written <- ctx.Err():
			default:
			}
		} else {
			delete(c.manager.permissions, id)
		}
		c.manager.mu.Unlock()

		return sdk.RequestPermissionResponse{}, ctx.Err()
	}
}

func safePermissionTool(tool sdk.ToolCallUpdate) bool {
	if tool.Kind == nil || tool.RawInput != nil {
		return false
	}

	switch *tool.Kind {
	case sdk.ToolKindRead, sdk.ToolKindEdit, sdk.ToolKindDelete, sdk.ToolKindMove,
		sdk.ToolKindSearch, sdk.ToolKindThink, sdk.ToolKindFetch, sdk.ToolKindSwitchMode:
		return true
	default:
		return false
	}
}

// permissionReader preserves the original JSON-RPC ID for the SDK callback, which only receives params.
func permissionReader(source io.Reader) io.Reader {
	reader, writer := io.Pipe()

	go func() {
		scanner := bufio.NewScanner(source)
		scanner.Buffer(make([]byte, 64*1024), 10*1024*1024)

		for scanner.Scan() {
			line := append([]byte(nil), scanner.Bytes()...)

			var message struct {
				Method string          `json:"method"`
				ID     json.RawMessage `json:"id"`
				Params json.RawMessage `json:"params"`
			}
			if json.Unmarshal(line, &message) == nil && message.Method == "session/request_permission" &&
				validRPCID(message.ID) {
				var params map[string]json.RawMessage
				if json.Unmarshal(message.Params, &params) == nil {
					var meta map[string]json.RawMessage
					if rawMeta, exists := params["_meta"]; !exists ||
						json.Unmarshal(rawMeta, &meta) == nil && meta != nil {
						if meta == nil {
							meta = make(map[string]json.RawMessage)
						}

						id, _ := json.Marshal(string(message.ID))
						meta[permissionRPCIDMeta] = id
						params["_meta"], _ = json.Marshal(meta)
						message.Params, _ = json.Marshal(params)

						var envelope map[string]json.RawMessage
						if json.Unmarshal(line, &envelope) == nil {
							envelope["params"] = message.Params
							line, _ = json.Marshal(envelope)
						}
					}
				}
			}

			if _, err := writer.Write(append(line, '\n')); err != nil {
				_ = writer.CloseWithError(err)
				return
			}
		}

		_ = writer.CloseWithError(scanner.Err())
	}()

	return reader
}

func validRPCID(raw []byte) bool {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return false
	}

	var value any
	if json.Unmarshal(raw, &value) != nil {
		return false
	}

	switch value.(type) {
	case string, float64:
		return true
	default:
		return false
	}
}

type permissionWriter struct {
	target     io.Writer
	manager    *Manager
	generation int64
	mu         sync.Mutex
}

func (w *permissionWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	var response struct {
		ID     json.RawMessage `json:"id"`
		Result json.RawMessage `json:"result"`
	}

	responseFrame := json.Unmarshal(bytes.TrimSpace(data), &response) == nil && validRPCID(response.ID) &&
		len(response.Result) != 0
	if responseFrame {
		w.manager.mu.Lock()
		for id, p := range w.manager.permissions {
			if p.generation == w.generation && p.rpcID == string(response.ID) {
				if p.abandoned {
					delete(w.manager.permissions, id)
					w.manager.mu.Unlock()

					return len(data), nil
				}

				p.started = true

				break
			}
		}
		w.manager.mu.Unlock()
	}

	n, err := w.target.Write(data)

	if responseFrame {
		w.manager.mu.Lock()
		for id, p := range w.manager.permissions {
			if p.generation == w.generation && p.rpcID == string(response.ID) && p.claimed {
				writeErr := err
				if writeErr == nil && n != len(data) {
					writeErr = io.ErrShortWrite
				}

				select {
				case p.written <- writeErr:
				default:
				}

				delete(w.manager.permissions, id)

				break
			}
		}
		w.manager.mu.Unlock()
	}

	return n, err
}
