package acp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	sdk "github.com/coder/acp-go-sdk"
	"github.com/yukihito-jokyu/TEJUN/internal/application"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/agentconnection"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/shared"
)

const (
	probeLifetime = 5 * time.Minute
	probeTimeout  = 10 * time.Second
)

var errElicitationNotFound = errors.New("elicitation request not found")

var (
	_ application.ProbeManager         = (*Manager)(nil)
	_ application.AgentJobExecutor     = (*Manager)(nil)
	_ application.AuthMethodResolver   = (*Manager)(nil)
	_ application.ElicitationResponder = (*Manager)(nil)
)

type Manager struct {
	checkMu         sync.Mutex
	mu              sync.Mutex
	sessions        map[string]*session
	checks          map[string]probeCheck
	projectSessions map[string]*projectSession
	elicitations    map[string]chan application.ElicitationResponse
	sink            application.ElicitationSink
	now             func() time.Time
	newID           func() string
	generation      atomic.Int64
	lifecycle       context.Context
	cancel          context.CancelFunc
}

type probeCheck struct {
	fingerprint string
	result      application.AgentProbeResult
}

type session struct {
	process         *process
	connection      *sdk.ClientSideConnection
	agentSessionID  sdk.SessionId
	pendingTurnID   string
	pendingTurnDone chan struct{}
	cancelTimedOut  bool
	messages        []application.ConversationItem
	modes           *application.SessionModes
	configOptions   []application.SessionConfigOption
	receiveSequence int64
	probe           agentconnection.Probe
	methods         map[string]authMethod
	input           agentconnection.ConnectionInput
}

type authMethod struct {
	kind string
	args []string
	env  map[string]string
}

func NewManager(sink application.ElicitationSink, now func() time.Time, newID func() string) *Manager {
	lifecycle, cancel := context.WithCancel(context.Background())

	return &Manager{
		sessions: make(map[string]*session), checks: make(map[string]probeCheck),
		projectSessions: make(map[string]*projectSession),
		elicitations:    make(map[string]chan application.ElicitationResponse),
		sink:            sink, now: now, newID: newID, lifecycle: lifecycle, cancel: cancel,
	}
}

func (m *Manager) Check(
	ctx context.Context,
	input application.CheckAuthenticationInput,
) (application.AgentProbeResult, error) {
	// ponytail: probe数が多くなったらoperationId単位のlockへ分割する。
	m.checkMu.Lock()
	defer m.checkMu.Unlock()

	fingerprint := m.Fingerprint(input.Connection)
	m.mu.Lock()

	prior, found := m.checks[input.OperationID]
	if input.OperationID != "" && found {
		m.mu.Unlock()

		if prior.fingerprint != fingerprint {
			return application.AgentProbeResult{}, &shared.Error{
				Code: "operation_id_conflict", Message: "operationIdが別の入力で使用されています",
			}
		}

		return prior.result, nil
	}
	m.mu.Unlock()

	environment := buildEnvironment(input.Connection.EnvironmentOverrides, nil)

	if err := ctx.Err(); err != nil {
		return application.AgentProbeResult{}, acpError(err, "initialize")
	}

	initializeContext, cancel := context.WithTimeout(ctx, probeTimeout)

	stopLifecycleCancel := context.AfterFunc(m.lifecycle, cancel)
	defer stopLifecycleCancel()
	defer cancel()

	proc, stdout, err := startProcess(
		m.lifecycle,
		initializeContext,
		input.Connection.Command,
		input.Connection.Args,
		environment,
	)
	if err != nil {
		return application.AgentProbeResult{}, acpError(err, "start")
	}

	probeID := m.newID()
	generation := m.generation.Add(1)
	client := &client{manager: m, attemptID: probeID, generation: generation}
	connection := sdk.NewClientSideConnection(client, proc.stdin, stdout)

	response, err := connection.Initialize(initializeContext, sdk.InitializeRequest{
		ProtocolVersion:    sdk.ProtocolVersionNumber,
		ClientCapabilities: sdk.ClientCapabilities{Auth: sdk.AuthCapabilities{Terminal: true}},
		ClientInfo:         &sdk.Implementation{Name: "tejun", Version: "1"},
	})
	if err != nil {
		_ = proc.close()

		if contextErr := initializeContext.Err(); contextErr != nil {
			err = contextErr
		}

		return application.AgentProbeResult{}, acpError(err, "initialize")
	}

	if response.ProtocolVersion != sdk.ProtocolVersionNumber {
		_ = proc.close()

		return application.AgentProbeResult{}, &shared.Error{
			Code: "acp_not_compatible", Message: "AgentのACP protocol versionに互換性がありません", Retryable: true,
		}
	}

	methods, exposed := convertAuthMethods(response.AuthMethods)

	authState := agentconnection.AuthNotRequired
	if len(methods) != 0 {
		authState = agentconnection.AuthUnknown
	}

	probe := agentconnection.Probe{
		ID: probeID, ConfigFingerprint: fingerprint, Compatibility: agentconnection.CompatibilityOK,
		ProtocolVersion: strconv.Itoa(int(response.ProtocolVersion)), AuthState: authState,
		ResolvedExecutablePath: proc.path, ProcessGeneration: generation, ExpiresAt: m.now().Add(probeLifetime),
	}

	result := application.AgentProbeResult{
		Probe: probe, AuthMethods: exposed,
		Capabilities: map[string]bool{"terminalAuthentication": true},
	}

	m.mu.Lock()

	s := &session{
		process: proc, connection: connection, probe: probe, methods: methods, input: input.Connection,
	}

	m.sessions[probeID] = s
	if input.OperationID != "" {
		m.checks[input.OperationID] = probeCheck{fingerprint: fingerprint, result: result}
	}
	m.mu.Unlock()

	m.watchSession(probeID, s)

	return result, nil
}

func (m *Manager) ResolveAuthMethod(_ context.Context, targetID, methodID string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.sessions[targetID]
	if !ok {
		return "", &shared.Error{Code: "not_found", Message: "接続attemptがありません"}
	}

	method, ok := s.methods[methodID]
	if !ok {
		return "", &shared.Error{Code: "validation_failed", Message: "提示されていない認証方法です"}
	}

	return method.kind, nil
}

func (m *Manager) Get(_ context.Context, probeID string) (agentconnection.Probe, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.sessions[probeID]
	if !ok {
		return agentconnection.Probe{}, errors.New("probe not found")
	}

	return s.probe, nil
}

func (m *Manager) Fingerprint(input agentconnection.ConnectionInput) string {
	hash := sha256.New()

	_, _ = hash.Write([]byte(input.Command))
	for _, arg := range input.Args {
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(arg))
	}

	for _, variable := range input.EnvironmentOverrides {
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(variable.Name))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(variable.Value))
	}

	return hex.EncodeToString(hash.Sum(nil))
}

func (m *Manager) Authenticate(ctx context.Context, probeID, methodID string) error {
	m.mu.Lock()

	s, ok := m.sessions[probeID]
	if !ok {
		m.mu.Unlock()

		return errors.New("probe not found")
	}

	method, ok := s.methods[methodID]
	m.mu.Unlock()

	if !ok {
		return errors.New("authentication method not found")
	}

	if method.kind == "terminal" {
		return m.authenticateTerminal(ctx, probeID, s, method)
	}

	_, err := s.connection.Authenticate(ctx, sdk.AuthenticateRequest{MethodId: methodID})
	if err != nil {
		return acpError(err, "authenticate")
	}

	m.mu.Lock()
	s.probe.AuthState = agentconnection.AuthAuthenticated
	m.mu.Unlock()

	return nil
}

func (m *Manager) authenticateTerminal(ctx context.Context, probeID string, s *session, method authMethod) error {
	cmd := exec.CommandContext(ctx, s.process.cmd.Path, method.args...)
	cmd.Env = buildEnvironment(nil, method.env)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout

	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return acpError(err, "authenticate")
	}

	if err := m.closeSession(probeID); err != nil {
		return err
	}

	result, err := m.Check(ctx, application.CheckAuthenticationInput{Connection: s.input})
	if err != nil {
		return err
	}

	if result.Probe.AuthState != agentconnection.AuthNotRequired {
		_ = m.closeSession(result.Probe.ID)

		return errors.New("terminal authentication did not complete")
	}

	m.mu.Lock()

	restarted, ok := m.sessions[result.Probe.ID]
	if !ok {
		m.mu.Unlock()

		return errors.New("restarted ACP process exited")
	}

	delete(m.sessions, result.Probe.ID)
	restarted.probe.ID = probeID
	restarted.probe.AuthState = agentconnection.AuthAuthenticated
	m.sessions[probeID] = restarted
	m.mu.Unlock()
	m.watchSession(probeID, restarted)

	return nil
}

func (m *Manager) Logout(ctx context.Context, probeID string) error {
	m.mu.Lock()
	s, ok := m.sessions[probeID]
	m.mu.Unlock()

	if !ok {
		return errors.New("probe not found")
	}

	_, err := s.connection.Logout(ctx, sdk.LogoutRequest{})
	if err != nil {
		return acpError(err, "logout")
	}

	return nil
}

func (m *Manager) Execute(ctx context.Context, job application.ClaimedAgentJob) error {
	switch job.Kind {
	case agentconnection.JobAuthenticate:
		if job.Connection != nil {
			return errors.New("authentication requires an active probe")
		}

		return m.Authenticate(ctx, job.TargetID, job.AuthMethodID)
	case agentconnection.JobLogout:
		if job.Connection != nil {
			result, err := m.Check(
				ctx,
				application.CheckAuthenticationInput{Connection: *job.Connection, OperationID: job.JobID},
			)
			if err != nil {
				return err
			}
			defer func() { _ = m.closeSession(result.Probe.ID) }()

			return m.Logout(ctx, result.Probe.ID)
		}

		return m.Logout(ctx, job.TargetID)
	default:
		return errors.New("unsupported agent job")
	}
}

func (m *Manager) Respond(_ context.Context, response application.ElicitationResponse) error {
	if response.Action == "accept" {
		var content map[string]any
		if err := json.Unmarshal([]byte(response.Content), &content); err != nil {
			return errors.New("elicitation content must be a JSON object")
		}
	}

	m.mu.Lock()

	ch, ok := m.elicitations[response.ElicitationRequestID]
	if ok {
		delete(m.elicitations, response.ElicitationRequestID)
	}
	m.mu.Unlock()

	if !ok {
		return errElicitationNotFound
	}

	ch <- response

	return nil
}

func (m *Manager) Close() error {
	m.cancel()

	m.mu.Lock()

	ids := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}

	projectSessions := m.projectSessions
	m.projectSessions = make(map[string]*projectSession)
	m.mu.Unlock()

	var err error
	for _, id := range ids {
		err = errors.Join(err, m.closeSession(id))
	}

	for _, session := range projectSessions {
		err = errors.Join(err, session.process.close())
	}

	return err
}

func acpError(err error, phase string) error {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return &shared.Error{Code: "acp_timeout", Message: "Agentから時間内に応答がありませんでした", Retryable: true}
	case errors.Is(err, context.Canceled):
		return &shared.Error{Code: "operation_cancelled", Message: "Agentへの接続を取り消しました", Retryable: true}
	case phase == "start":
		return &shared.Error{Code: "process_start_failed", Message: "Agentを起動できませんでした", Retryable: true}
	default:
		return &shared.Error{Code: "agent_process_exited", Message: "Agent processが異常終了しました", Retryable: true}
	}
}

func (m *Manager) closeSession(id string) error {
	return m.closeOwnedSession(id, nil)
}

func (m *Manager) watchSession(id string, s *session) {
	go func() {
		select {
		case <-s.process.done:
			_ = m.closeOwnedSession(id, s)
		case <-time.After(time.Until(s.probe.ExpiresAt)):
			_ = m.closeOwnedSession(id, s)
		}
	}()
}

func (m *Manager) closeOwnedSession(id string, owner *session) error {
	m.mu.Lock()

	s, ok := m.sessions[id]
	if ok && owner != nil && s != owner {
		ok = false
	}

	if ok {
		delete(m.sessions, id)

		for operationID, check := range m.checks {
			if check.result.Probe.ID == id {
				delete(m.checks, operationID)
			}
		}
	}
	m.mu.Unlock()

	if !ok {
		return nil
	}

	return s.process.close()
}

func buildEnvironment(overrides []agentconnection.EnvironmentVariable, terminal map[string]string) []string {
	values := make(map[string]string)

	for _, entry := range os.Environ() {
		for index := range entry {
			if entry[index] == '=' {
				values[entry[:index]] = entry[index+1:]

				break
			}
		}
	}

	for _, variable := range overrides {
		values[variable.Name] = variable.Value
	}

	for name, value := range terminal {
		values[name] = value
	}

	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}

	sort.Strings(names)

	environment := make([]string, 0, len(names))
	for _, name := range names {
		environment = append(environment, name+"="+values[name])
	}

	return environment
}

func convertAuthMethods(methods []sdk.AuthMethod) (map[string]authMethod, []application.AuthMethod) {
	internal := make(map[string]authMethod, len(methods))

	exposed := make([]application.AuthMethod, 0, len(methods))
	for _, method := range methods {
		switch {
		case method.Agent != nil:
			internal[method.Agent.Id] = authMethod{kind: "agent"}
			exposed = append(
				exposed,
				application.AuthMethod{
					Type:        "agent",
					ID:          method.Agent.Id,
					Name:        method.Agent.Name,
					Description: value(method.Agent.Description),
				},
			)
		case method.Terminal != nil:
			environment := make(map[string]string, len(method.Terminal.Env))

			names := make([]string, 0, len(method.Terminal.Env))
			for name, raw := range method.Terminal.Env {
				text, ok := raw.(string)
				if !ok {
					continue
				}

				environment[name] = text
				names = append(names, name)
			}

			sort.Strings(names)

			internal[method.Terminal.Id] = authMethod{
				kind: "terminal",
				args: append([]string(nil), method.Terminal.Args...),
				env:  environment,
			}
			exposed = append(exposed, application.AuthMethod{
				Type: "terminal",
				ID:   method.Terminal.Id,
				Name: method.Terminal.Name,
				Description: value(
					method.Terminal.Description,
				),
				Args:             append([]string(nil), method.Terminal.Args...),
				EnvironmentNames: names,
			})
		}
	}

	return internal, exposed
}

func value(text *string) string {
	if text == nil {
		return ""
	}

	return *text
}
