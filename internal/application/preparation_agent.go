package application

import (
	"context"
	"time"
)

type CancelAgentOperationInput struct {
	SessionID   string `json:"sessionId"`
	TurnID      string `json:"turnId,omitempty"`
	RunID       string `json:"runId,omitempty"`
	JobID       string `json:"jobId,omitempty"`
	OperationID string `json:"operationId"`
}
type CancellationAccepted struct {
	JobID        string    `json:"jobId"`
	TargetStatus string    `json:"targetStatus"`
	RequestedAt  time.Time `json:"requestedAt"`
	Mechanism    string    `json:"mechanism"`
}
type CancelAgentOperationRecord struct {
	CancelAgentOperationInput
	CancellationJobID string
	EventID           string
	RequestedAt       time.Time
}
type SessionConfigurationChange struct {
	Kind     string `json:"kind"`
	ModeID   string `json:"modeId,omitempty"`
	ConfigID string `json:"configId,omitempty"`
	Value    any    `json:"value,omitempty"`
}
type SetAgentSessionConfigurationInput struct {
	SessionID        string                     `json:"sessionId"`
	Change           SessionConfigurationChange `json:"change"`
	ExpectedRevision int64                      `json:"expectedRevision"`
	OperationID      string                     `json:"operationId"`
}
type SessionConfigurationClaim struct {
	SetAgentSessionConfigurationInput
	EventID         string
	ClaimedAt       time.Time
	ReceiveSequence int64
}
type SessionConfigurationTarget struct {
	SessionID       string
	AgentSessionID  string
	WorkspacePath   string
	Change          SessionConfigurationChange
	ReceiveSequence int64
}
type SessionConfigurationResult struct {
	Modes         *SessionModes
	ConfigOptions []SessionConfigOption
}
type SessionConfigurationSetter interface {
	SetConfiguration(context.Context, SessionConfigurationTarget) (SessionConfigurationResult, error)
}
type PreparationAgentRepository interface {
	AcceptCancellation(context.Context, CancelAgentOperationRecord) (MutationResult[CancellationAccepted], error)
	ClaimSessionConfiguration(
		context.Context,
		SessionConfigurationClaim,
	) (SessionConfigurationTarget, MutationResult[SessionSummary], bool, error)
	CompleteSessionConfiguration(
		context.Context,
		SessionConfigurationClaim,
		SessionConfigurationResult,
	) (MutationResult[SessionSummary], error)
	FailSessionConfiguration(context.Context, SessionConfigurationClaim) error
}
type PreparationAgentControl struct {
	repository PreparationAgentRepository
	setter     SessionConfigurationSetter
	now        func() time.Time
	newID      func() string
}

func NewPreparationAgentControl(
	repo PreparationAgentRepository,
	setter SessionConfigurationSetter,
	now func() time.Time,
	newID func() string,
) *PreparationAgentControl {
	return &PreparationAgentControl{repo, setter, now, newID}
}

func (a *PreparationAgentControl) CancelAgentOperation(
	ctx context.Context,
	in CancelAgentOperationInput,
) (MutationResult[CancellationAccepted], error) {
	return a.repository.AcceptCancellation(
		ctx,
		CancelAgentOperationRecord{
			CancelAgentOperationInput: in,
			CancellationJobID:         a.newID(),
			EventID:                   a.newID(),
			RequestedAt:               a.now(),
		},
	)
}

func (a *PreparationAgentControl) SetAgentSessionConfiguration(
	ctx context.Context,
	in SetAgentSessionConfigurationInput,
) (MutationResult[SessionSummary], error) {
	claim := SessionConfigurationClaim{SetAgentSessionConfigurationInput: in, EventID: a.newID(), ClaimedAt: a.now()}

	target, replayed, claimed, err := a.repository.ClaimSessionConfiguration(ctx, claim)
	if err != nil || !claimed {
		return replayed, err
	}

	claim.ReceiveSequence = target.ReceiveSequence

	result, err := a.setter.SetConfiguration(ctx, target)
	if err != nil {
		_ = a.repository.FailSessionConfiguration(context.WithoutCancel(ctx), claim)
		return MutationResult[SessionSummary]{}, err
	}

	return a.repository.CompleteSessionConfiguration(context.WithoutCancel(ctx), claim, result)
}
