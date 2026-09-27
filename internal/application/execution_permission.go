package application

import (
	"context"
	"errors"
	"time"

	"github.com/yukihito-jokyu/TEJUN/internal/domain/execution"
)

type IncomingPermission struct {
	ID, ExecutionID, SessionID, RunID, ToolCallID, Title string
	AgentSessionID, RPCRequestIDJSON, CommandDigest      string
	ProcessGeneration                                    int64
	Options                                              []execution.PermissionOption
	RequestedAt                                          time.Time
	ExpiresAt                                            *time.Time
}

type PermissionClaim struct {
	SessionID, PermissionRequestID, OptionID, OperationID string
	ProcessGeneration                                     int64
	At                                                    time.Time
}

type PermissionResponseResult struct {
	PermissionRequestID, Status, SelectedOptionID string
	RespondedAt                                   time.Time
}

type PermissionRepository interface {
	RegisterPermission(context.Context, IncomingPermission) error
	ClaimPermission(context.Context, PermissionClaim) (MutationResult[PermissionResponseResult], bool, error)
	CompletePermission(context.Context, string, string, bool, time.Time) error
}

type PermissionWire interface {
	Generation(context.Context, string) (int64, error)
	RespondPermission(context.Context, string, string, int64) error
}

var ErrPermissionDeliveryIndeterminate = errors.New("permission delivery is indeterminate")

type ExecutionPermission struct {
	repository PermissionRepository
	wire       PermissionWire
	now        func() time.Time
}

func NewExecutionPermission(
	repository PermissionRepository,
	wire PermissionWire,
	now func() time.Time,
) *ExecutionPermission {
	return &ExecutionPermission{repository: repository, wire: wire, now: now}
}

func (p *ExecutionPermission) Respond(
	ctx context.Context,
	claim PermissionClaim,
) (MutationResult[PermissionResponseResult], error) {
	generation, err := p.wire.Generation(ctx, claim.SessionID)
	if err != nil {
		return MutationResult[PermissionResponseResult]{}, err
	}

	claim.ProcessGeneration = generation
	claim.At = p.now()

	result, claimed, err := p.repository.ClaimPermission(ctx, claim)
	if err != nil || !claimed {
		return result, err
	}

	wireErr := p.wire.RespondPermission(ctx, claim.PermissionRequestID, claim.OptionID, generation)
	if errors.Is(wireErr, ErrPermissionDeliveryIndeterminate) {
		return MutationResult[PermissionResponseResult]{}, wireErr
	}

	completionCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), jobCompletionTimeout)
	defer cancel()

	if err := p.repository.CompletePermission(completionCtx, claim.PermissionRequestID, claim.OperationID,
		wireErr == nil, p.now()); err != nil {
		return MutationResult[PermissionResponseResult]{}, err
	}

	if wireErr != nil {
		return MutationResult[PermissionResponseResult]{}, wireErr
	}

	result.Data.Status = "sent"
	result.Data.RespondedAt = p.now()

	return result, nil
}
