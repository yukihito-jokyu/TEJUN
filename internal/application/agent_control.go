package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/yukihito-jokyu/TEJUN/internal/domain/agentconnection"
)

type AgentControlRepository interface {
	AcceptAgentJob(context.Context, AgentJobRecord) (MutationResult[AgentJobAccepted], error)
	ClaimAgentJob(context.Context) (*ClaimedAgentJob, error)
	CompleteAgentJob(context.Context, string, bool, time.Time) error
	ClaimElicitation(context.Context, ElicitationClaim) (MutationResult[ElicitationResponseResult], bool, error)
	CompleteElicitation(context.Context, string, string, bool, time.Time) error
}

const jobCompletionTimeout = 5 * time.Second

type ElicitationResponder interface {
	Respond(context.Context, ElicitationResponse) error
}

type ElicitationSink interface {
	RegisterElicitation(context.Context, IncomingElicitation) error
}

type IncomingElicitation struct {
	ID                  string
	ConnectionAttemptID string
	ProcessGeneration   int64
	Mode                string
	Message             string
	RequestedAt         time.Time
	ExpiresAt           *time.Time
}

type AgentJobExecutor interface {
	Execute(context.Context, ClaimedAgentJob) error
}

type AuthMethodResolver interface {
	ResolveAuthMethod(context.Context, string, string) (string, error)
}

type ClaimedAgentJob struct {
	JobID        string
	Kind         agentconnection.JobKind
	TargetID     string
	AuthMethodID string
	MethodType   string
	Connection   *agentconnection.ConnectionInput
}

// RunAgentJobは短いtransactionの間で外部I/Oを待たない。
func RunAgentJob(
	ctx context.Context,
	repository AgentControlRepository,
	executor AgentJobExecutor,
	now func() time.Time,
) (bool, error) {
	job, err := repository.ClaimAgentJob(ctx)
	if err != nil || job == nil {
		return false, err
	}

	executeErr := executor.Execute(ctx, *job)

	completionContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), jobCompletionTimeout)
	defer cancel()

	if err := repository.CompleteAgentJob(completionContext, job.JobID, executeErr == nil, now()); err != nil {
		return true, err
	}

	return true, executeErr
}

type AgentControl struct {
	repository AgentControlRepository
	responder  ElicitationResponder
	methods    AuthMethodResolver
	now        func() time.Time
	newID      func() string
}

func NewAgentControl(
	repository AgentControlRepository,
	responder ElicitationResponder,
	methods AuthMethodResolver,
	now func() time.Time,
	newID func() string,
) *AgentControl {
	return &AgentControl{repository: repository, responder: responder, methods: methods, now: now, newID: newID}
}

type AuthenticateAgentInput struct {
	ProbeID      string
	ConnectionID string
	AuthMethodID string
	OperationID  string
}

type LogoutAgentInput struct {
	ConnectionID string
	OperationID  string
}

type AgentJobRecord struct {
	JobID        string
	Kind         agentconnection.JobKind
	TargetID     string
	AuthMethodID string
	MethodType   string
	OperationID  string
	RequestHash  string
	AcceptedAt   time.Time
	Event        OutboxEvent
	Receipt      MutationReceipt
}

type AgentJobAccepted struct {
	JobID      string
	TargetID   string
	MethodType string
	State      string
	AcceptedAt time.Time
}

func (a *AgentControl) Authenticate(
	ctx context.Context,
	input AuthenticateAgentInput,
) (MutationResult[AgentJobAccepted], error) {
	targetID := input.ProbeID
	if targetID == "" {
		targetID = input.ConnectionID
	}

	methodType, err := a.methods.ResolveAuthMethod(ctx, targetID, input.AuthMethodID)
	if err != nil {
		return MutationResult[AgentJobAccepted]{}, err
	}

	return a.acceptJob(
		ctx, agentconnection.JobAuthenticate, targetID,
		input.AuthMethodID, methodType, input.OperationID,
	)
}

func (a *AgentControl) Logout(ctx context.Context, input LogoutAgentInput) (MutationResult[AgentJobAccepted], error) {
	return a.acceptJob(ctx, agentconnection.JobLogout, input.ConnectionID, "", "", input.OperationID)
}

func (a *AgentControl) acceptJob(
	ctx context.Context,
	kind agentconnection.JobKind,
	targetID string,
	authMethodID string,
	methodType string,
	operationID string,
) (MutationResult[AgentJobAccepted], error) {
	now := a.now()
	jobID := a.newID()
	record := AgentJobRecord{
		JobID:        jobID,
		Kind:         kind,
		TargetID:     targetID,
		AuthMethodID: authMethodID,
		MethodType:   methodType,
		OperationID:  operationID,
		RequestHash:  string(kind) + "\x00" + targetID + "\x00" + authMethodID,
		AcceptedAt:   now,
		Receipt:      MutationReceipt{OperationID: operationID, CommittedAt: now},
		Event: OutboxEvent{
			ID: a.newID(), Name: "agent.authentication.updated", AggregateType: "agent_connection",
			AggregateID: targetID, EmittedAt: now,
		},
	}

	return a.repository.AcceptAgentJob(ctx, record)
}

type RespondToElicitationInput struct {
	ElicitationRequestID string
	Action               string
	Content              string
	OperationID          string
}

type ElicitationClaim struct {
	ElicitationRequestID string
	Action               string
	Content              string
	OperationID          string
	RequestHash          string
	ClaimedAt            time.Time
	Receipt              MutationReceipt
}

type ElicitationResponse struct {
	ElicitationRequestID string
	Action               string
	Content              string
}

type ElicitationResponseResult struct {
	ElicitationRequestID string
	Status               string
	RespondedAt          time.Time
}

func (a *AgentControl) RespondToElicitation(
	ctx context.Context,
	input RespondToElicitationInput,
) (MutationResult[ElicitationResponseResult], error) {
	now := a.now()
	claim := ElicitationClaim{
		ElicitationRequestID: input.ElicitationRequestID,
		Action:               input.Action,
		Content:              input.Content,
		OperationID:          input.OperationID,
		RequestHash:          requestDigest(input.ElicitationRequestID, input.Action, input.Content),
		ClaimedAt:            now, Receipt: MutationReceipt{OperationID: input.OperationID, CommittedAt: now},
	}

	result, claimed, err := a.repository.ClaimElicitation(ctx, claim)
	if err != nil {
		return MutationResult[ElicitationResponseResult]{}, err
	}

	if !claimed {
		return result, nil
	}

	err = a.responder.Respond(ctx, ElicitationResponse{
		ElicitationRequestID: input.ElicitationRequestID,
		Action:               input.Action,
		Content:              input.Content,
	})

	completedAt := a.now()
	completionContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), jobCompletionTimeout)

	defer cancel()

	if completeErr := a.repository.CompleteElicitation(
		completionContext,
		input.ElicitationRequestID,
		input.OperationID,
		err == nil,
		completedAt,
	); completeErr != nil {
		return MutationResult[ElicitationResponseResult]{}, completeErr
	}

	if err != nil {
		return MutationResult[ElicitationResponseResult]{}, err
	}

	result.Data.Status = string(agentconnection.ElicitationResponded)
	result.Data.RespondedAt = completedAt

	return result, nil
}

func requestDigest(parts ...string) string {
	hash := sha256.New()
	for _, part := range parts {
		_, _ = hash.Write([]byte(part))
		_, _ = hash.Write([]byte{0})
	}

	return hex.EncodeToString(hash.Sum(nil))
}
