package wails

import (
	"context"
	"strings"
	"time"

	"github.com/yukihito-jokyu/TEJUN/internal/application"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/agentconnection"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/shared"
)

type (
	StartupService      struct{ startup *application.Startup }
	PreparationService  struct{ preparation *application.Preparation }
	ExecutionService    struct{}
	ProcedureService    struct{}
	AgentControlService struct {
		control     *application.AgentControl
		preparation *application.PreparationAgentControl
	}
)

func NewStartupService(startup *application.Startup) *StartupService {
	return &StartupService{startup: startup}
}

func NewAgentControlService(
	control *application.AgentControl,
	preparation *application.PreparationAgentControl,
) *AgentControlService {
	return &AgentControlService{control: control, preparation: preparation}
}

type EnvironmentVariableInput struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type AgentConnectionInput struct {
	DisplayName          string                     `json:"displayName"`
	Command              string                     `json:"command"`
	Args                 []string                   `json:"args"`
	Transport            string                     `json:"transport"`
	EnvironmentOverrides []EnvironmentVariableInput `json:"environmentOverrides"`
}

type AgentCandidateQuery struct {
	Refresh bool `json:"refresh"`
}

type CheckAuthenticationInput struct {
	Connection  AgentConnectionInput `json:"connection"`
	OperationID string               `json:"operationId"`
}

type CompleteSetupInput struct {
	Connection  AgentConnectionInput `json:"connection"`
	ProbeID     string               `json:"probeId"`
	OperationID string               `json:"operationId"`
}

type AuthenticateAgentInput struct {
	ProbeID      string `json:"probeId"`
	ConnectionID string `json:"connectionId"`
	AuthMethodID string `json:"authMethodId"`
	OperationID  string `json:"operationId"`
}

type LogoutAgentInput struct {
	ConnectionID string `json:"connectionId"`
	OperationID  string `json:"operationId"`
}

type RespondToElicitationInput struct {
	ElicitationRequestID string `json:"elicitationRequestId"`
	Action               string `json:"action"`
	Content              string `json:"content"`
	OperationID          string `json:"operationId"`
}

type CancelAgentOperationInput struct {
	SessionID   string `json:"sessionId"`
	TurnID      string `json:"turnId,omitempty"`
	RunID       string `json:"runId,omitempty"`
	JobID       string `json:"jobId,omitempty"`
	OperationID string `json:"operationId"`
}

type CancellationAccepted struct {
	JobID        string `json:"jobId"`
	TargetStatus string `json:"targetStatus"`
	RequestedAt  string `json:"requestedAt"`
	Mechanism    string `json:"mechanism"`
}

type AgentSessionConfigurationChange struct {
	Kind     string `json:"kind"`
	ModeID   string `json:"modeId,omitempty"`
	ConfigID string `json:"configId,omitempty"`
	Value    any    `json:"value,omitempty"`
}

type SetAgentSessionConfigurationInput struct {
	SessionID        string                          `json:"sessionId"`
	ExpectedRevision int64                           `json:"expectedRevision"`
	OperationID      string                          `json:"operationId"`
	Change           AgentSessionConfigurationChange `json:"change"`
}

type AgentCandidate struct {
	CandidateKey           string   `json:"candidateKey"`
	DisplayName            string   `json:"displayName"`
	Command                string   `json:"command"`
	Transport              string   `json:"transport"`
	Source                 string   `json:"source"`
	ResolvedExecutablePath string   `json:"resolvedExecutablePath,omitempty"`
	Args                   []string `json:"args"`
	Warnings               []string `json:"warnings"`
}

type AgentCandidateResult struct {
	Items     []AgentCandidate `json:"items"`
	ScannedAt string           `json:"scannedAt"`
	Warnings  []string         `json:"warnings"`
}

type AgentConnectionSummary struct {
	ConnectionID           string   `json:"connectionId"`
	DisplayName            string   `json:"displayName"`
	Command                string   `json:"command"`
	Transport              string   `json:"transport"`
	ResolvedExecutablePath string   `json:"resolvedExecutablePath"`
	Args                   []string `json:"args"`
	LastVerifiedAt         string   `json:"lastVerifiedAt"`
	ProtocolVersion        string   `json:"protocolVersion"`
	AuthState              string   `json:"authState"`
	SchemaArtifactVersion  string   `json:"schemaArtifactVersion"`
}

type StartupState struct {
	InitialSetupRequired bool                    `json:"initialSetupRequired"`
	DefaultConnection    *AgentConnectionSummary `json:"defaultConnection,omitempty"`
	RecoveryNotice       string                  `json:"recoveryNotice,omitempty"`
	NextRoute            string                  `json:"nextRoute"`
	ChangeSequence       int64                   `json:"changeSequence"`
}

type AuthMethodView struct {
	Type             string   `json:"type"`
	AuthMethodID     string   `json:"authMethodId"`
	Name             string   `json:"name"`
	Description      string   `json:"description,omitempty"`
	Args             []string `json:"args"`
	EnvironmentNames []string `json:"environmentNames"`
}

type AuthObservation struct {
	Status            string `json:"status"`
	Source            string `json:"source"`
	ObservedAt        string `json:"observedAt"`
	ProcessGeneration int64  `json:"processGeneration"`
}

type AgentProbeResult struct {
	ProbeID           string           `json:"probeId"`
	ConfigFingerprint string           `json:"configFingerprint"`
	Compatibility     string           `json:"compatibility"`
	ProtocolVersion   string           `json:"protocolVersion,omitempty"`
	AuthState         string           `json:"authState"`
	ExpiresAt         string           `json:"expiresAt"`
	AuthMethods       []AuthMethodView `json:"authMethods"`
	AuthObservation   AuthObservation  `json:"authObservation"`
	Capabilities      map[string]bool  `json:"capabilities,omitempty"`
}

type MutationReceipt struct {
	OperationID    string `json:"operationId"`
	CommittedAt    string `json:"committedAt"`
	ChangeSequence int64  `json:"changeSequence"`
}

type MutationResult[T any] struct {
	Data    T               `json:"data"`
	Receipt MutationReceipt `json:"receipt"`
}

type InitialSetupResult struct {
	Connection     AgentConnectionSummary `json:"connection"`
	NextRoute      string                 `json:"nextRoute"`
	ChangeSequence int64                  `json:"changeSequence"`
}

type AgentJobAccepted struct {
	JobID      string `json:"jobId"`
	TargetID   string `json:"targetId"`
	MethodType string `json:"methodType,omitempty"`
	State      string `json:"state"`
	AcceptedAt string `json:"acceptedAt"`
}

type LogoutAccepted struct {
	JobID        string `json:"jobId"`
	ConnectionID string `json:"connectionId"`
	State        string `json:"state"`
	AcceptedAt   string `json:"acceptedAt"`
}

type ElicitationResponseResult struct {
	ElicitationRequestID string `json:"elicitationRequestId"`
	Status               string `json:"status"`
	RespondedAt          string `json:"respondedAt"`
}

func (s *StartupService) GetStartupState(ctx context.Context) (StartupState, error) {
	state, err := s.startup.GetState(ctx)
	if err != nil {
		return StartupState{}, storageError(err)
	}

	result := StartupState{
		InitialSetupRequired: state.InitialSetupRequired,
		RecoveryNotice:       state.RecoveryNotice,
		NextRoute:            state.NextRoute,
		ChangeSequence:       state.ChangeSequence,
	}
	if state.DefaultConnection != nil {
		connection := connectionSummary(*state.DefaultConnection)
		result.DefaultConnection = &connection
	}

	return result, nil
}

func (s *StartupService) ListAgentCandidates(
	ctx context.Context,
	query AgentCandidateQuery,
) (AgentCandidateResult, error) {
	result, err := s.startup.ListAgentCandidates(ctx, query.Refresh)
	if err != nil {
		return AgentCandidateResult{}, storageError(err)
	}

	items := make([]AgentCandidate, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(
			items,
			AgentCandidate{
				CandidateKey:           item.CandidateKey,
				DisplayName:            item.DisplayName,
				Command:                item.Command,
				Args:                   NonNilSlice(item.Args),
				Transport:              item.Transport,
				Source:                 item.Source,
				ResolvedExecutablePath: item.ResolvedExecutablePath,
				Warnings:               NonNilSlice(item.Warnings),
			},
		)
	}

	return AgentCandidateResult{
		Items:     items,
		ScannedAt: result.ScannedAt.Format(time.RFC3339Nano),
		Warnings:  NonNilSlice(result.Warnings),
	}, nil
}

func (s *StartupService) CheckAuthentication(
	ctx context.Context,
	input CheckAuthenticationInput,
) (AgentProbeResult, error) {
	connection, err := validateConnection(input.Connection)
	if err != nil {
		return AgentProbeResult{}, err
	}

	if err := required("operationId", input.OperationID); err != nil {
		return AgentProbeResult{}, err
	}

	result, err := s.startup.CheckAuthentication(
		ctx,
		application.CheckAuthenticationInput{Connection: connection, OperationID: input.OperationID},
	)
	if err != nil {
		return AgentProbeResult{}, err
	}

	methods := make([]AuthMethodView, 0, len(result.AuthMethods))
	for _, method := range result.AuthMethods {
		methods = append(
			methods,
			AuthMethodView{
				Type:             method.Type,
				AuthMethodID:     method.ID,
				Name:             method.Name,
				Description:      method.Description,
				Args:             NonNilSlice(method.Args),
				EnvironmentNames: NonNilSlice(method.EnvironmentNames),
			},
		)
	}

	probe := result.Probe

	return AgentProbeResult{
		ProbeID:           probe.ID,
		ConfigFingerprint: probe.ConfigFingerprint,
		Compatibility:     probe.Compatibility,
		ProtocolVersion:   probe.ProtocolVersion,
		AuthMethods:       methods,
		AuthState:         string(probe.AuthState),
		AuthObservation: AuthObservation{
			Status:            string(probe.AuthState),
			Source:            "initialize_only",
			ObservedAt:        time.Now().UTC().Format(time.RFC3339Nano),
			ProcessGeneration: probe.ProcessGeneration,
		},
		Capabilities: result.Capabilities,
		ExpiresAt:    probe.ExpiresAt.Format(time.RFC3339Nano),
	}, nil
}

func (s *StartupService) CompleteInitialSetup(
	ctx context.Context,
	input CompleteSetupInput,
) (MutationResult[InitialSetupResult], error) {
	connection, err := validateConnection(input.Connection)
	if err != nil {
		return MutationResult[InitialSetupResult]{}, err
	}

	if err := requireFields(map[string]string{"probeId": input.ProbeID, "operationId": input.OperationID}); err != nil {
		return MutationResult[InitialSetupResult]{}, err
	}

	result, err := s.startup.CompleteInitialSetup(
		ctx,
		application.CompleteSetupInput{Connection: connection, ProbeID: input.ProbeID, OperationID: input.OperationID},
	)
	if err != nil {
		return MutationResult[InitialSetupResult]{}, err
	}

	return MutationResult[InitialSetupResult]{
		Data: InitialSetupResult{
			Connection:     connectionSummary(result.Data.Connection),
			NextRoute:      result.Data.NextRoute,
			ChangeSequence: result.Data.ChangeSequence,
		},
		Receipt: receipt(result.Receipt),
	}, nil
}

func (s *AgentControlService) AuthenticateAgent(
	ctx context.Context,
	input AuthenticateAgentInput,
) (MutationResult[AgentJobAccepted], error) {
	if (input.ProbeID == "") == (input.ConnectionID == "") {
		return MutationResult[AgentJobAccepted]{}, validation(
			map[string]string{"probeId": "probeIdとconnectionIdの片方だけを指定してください"},
		)
	}

	if err := requireFields(
		map[string]string{"authMethodId": input.AuthMethodID, "operationId": input.OperationID},
	); err != nil {
		return MutationResult[AgentJobAccepted]{}, err
	}

	result, err := s.control.Authenticate(
		ctx,
		application.AuthenticateAgentInput{
			ProbeID:      input.ProbeID,
			ConnectionID: input.ConnectionID,
			AuthMethodID: input.AuthMethodID,
			OperationID:  input.OperationID,
		},
	)
	if err != nil {
		return MutationResult[AgentJobAccepted]{}, err
	}

	return jobResult(result), nil
}

func (s *AgentControlService) LogoutAgent(
	ctx context.Context,
	input LogoutAgentInput,
) (MutationResult[LogoutAccepted], error) {
	if err := requireFields(
		map[string]string{"connectionId": input.ConnectionID, "operationId": input.OperationID},
	); err != nil {
		return MutationResult[LogoutAccepted]{}, err
	}

	result, err := s.control.Logout(
		ctx,
		application.LogoutAgentInput{ConnectionID: input.ConnectionID, OperationID: input.OperationID},
	)
	if err != nil {
		return MutationResult[LogoutAccepted]{}, err
	}

	return MutationResult[LogoutAccepted]{
		Data: LogoutAccepted{
			JobID:        result.Data.JobID,
			ConnectionID: result.Data.TargetID,
			State:        result.Data.State,
			AcceptedAt:   result.Data.AcceptedAt.Format(time.RFC3339Nano),
		},
		Receipt: receipt(result.Receipt),
	}, nil
}

func (s *AgentControlService) RespondToElicitation(
	ctx context.Context,
	input RespondToElicitationInput,
) (MutationResult[ElicitationResponseResult], error) {
	if err := requireFields(
		map[string]string{
			"elicitationRequestId": input.ElicitationRequestID,
			"action":               input.Action,
			"operationId":          input.OperationID,
		},
	); err != nil {
		return MutationResult[ElicitationResponseResult]{}, err
	}

	if input.Action != "accept" && input.Action != "decline" && input.Action != "cancel" {
		return MutationResult[ElicitationResponseResult]{}, validation(
			map[string]string{"action": "accept、decline、cancelのいずれかを指定してください"},
		)
	}

	result, err := s.control.RespondToElicitation(
		ctx,
		application.RespondToElicitationInput{
			ElicitationRequestID: input.ElicitationRequestID,
			Action:               input.Action,
			Content:              input.Content,
			OperationID:          input.OperationID,
		},
	)
	if err != nil {
		return MutationResult[ElicitationResponseResult]{}, err
	}

	return MutationResult[ElicitationResponseResult]{
		Data: ElicitationResponseResult{
			ElicitationRequestID: result.Data.ElicitationRequestID,
			Status:               result.Data.Status,
			RespondedAt:          result.Data.RespondedAt.Format(time.RFC3339Nano),
		},
		Receipt: receipt(result.Receipt),
	}, nil
}

func validateConnection(input AgentConnectionInput) (agentconnection.ConnectionInput, error) {
	if err := requireFields(
		map[string]string{"displayName": input.DisplayName, "command": input.Command, "transport": input.Transport},
	); err != nil {
		return agentconnection.ConnectionInput{}, err
	}

	if input.Transport != agentconnection.TransportStdio {
		return agentconnection.ConnectionInput{}, validation(map[string]string{"transport": "stdioだけを指定できます"})
	}

	environment := make([]agentconnection.EnvironmentVariable, 0, len(input.EnvironmentOverrides))
	for _, variable := range input.EnvironmentOverrides {
		if strings.TrimSpace(variable.Name) == "" || strings.Contains(variable.Name, "=") {
			return agentconnection.ConnectionInput{}, validation(
				map[string]string{"environmentOverrides": "環境変数名が不正です"},
			)
		}

		environment = append(
			environment,
			agentconnection.EnvironmentVariable{Name: variable.Name, Value: variable.Value},
		)
	}

	return agentconnection.ConnectionInput{
		DisplayName:          input.DisplayName,
		Command:              input.Command,
		Args:                 NonNilSlice(input.Args),
		Transport:            input.Transport,
		EnvironmentOverrides: environment,
	}, nil
}

func required(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return validation(map[string]string{field: "必須です"})
	}

	return nil
}

func requireFields(fields map[string]string) error {
	errors := make(map[string]string)

	for field, value := range fields {
		if strings.TrimSpace(value) == "" {
			errors[field] = "必須です"
		}
	}

	if len(errors) != 0 {
		return validation(errors)
	}

	return nil
}

func validation(fields map[string]string) error {
	return &shared.Error{Code: "validation_failed", Message: "入力内容を確認してください", FieldErrors: fields}
}

func storageError(err error) error {
	if _, ok := err.(*shared.Error); ok {
		return err
	}

	return &shared.Error{Code: "storage_failed", Message: "保存データを読み取れませんでした", Retryable: true}
}

func connectionSummary(connection agentconnection.Connection) AgentConnectionSummary {
	return AgentConnectionSummary{
		ConnectionID:           connection.ID,
		DisplayName:            connection.DisplayName,
		Command:                connection.Command,
		Args:                   NonNilSlice(connection.Args),
		Transport:              connection.Transport,
		ResolvedExecutablePath: connection.ResolvedExecutablePath,
		LastVerifiedAt:         connection.LastVerifiedAt.Format(time.RFC3339Nano),
		ProtocolVersion:        connection.ProtocolVersion,
		AuthState:              string(connection.AuthState),
		SchemaArtifactVersion:  connection.SchemaArtifactVersion,
	}
}

func receipt(value application.MutationReceipt) MutationReceipt {
	return MutationReceipt{
		OperationID:    value.OperationID,
		CommittedAt:    value.CommittedAt.Format(time.RFC3339Nano),
		ChangeSequence: value.ChangeSequence,
	}
}

func jobResult(result application.MutationResult[application.AgentJobAccepted]) MutationResult[AgentJobAccepted] {
	return MutationResult[AgentJobAccepted]{
		Data: AgentJobAccepted{
			JobID:      result.Data.JobID,
			TargetID:   result.Data.TargetID,
			MethodType: result.Data.MethodType,
			State:      result.Data.State,
			AcceptedAt: result.Data.AcceptedAt.Format(time.RFC3339Nano),
		},
		Receipt: receipt(result.Receipt),
	}
}

func NonNilSlice[T any](items []T) []T {
	if items == nil {
		return []T{}
	}

	return items
}
