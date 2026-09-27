package application

import (
	"context"
	"time"

	"github.com/yukihito-jokyu/TEJUN/internal/domain/agentconnection"
)

type RunPendingChecksInput struct {
	ExecutionID      string
	ExpectedRevision int64
	CheckIDs         []string
	OperationID      string
}

type SendExecutionMessageInput struct {
	ExecutionID, Text, OperationID string
}

type ExecutionJobRecord struct {
	ExecutionID, JobID, RunID, TurnID, Kind, PromptText, OperationID string
	ExpectedRevision                                                 int64
	CheckIDs                                                         []string
	AcceptedAt                                                       time.Time
	Event                                                            OutboxEvent
}

type ExecutionJobAccepted struct {
	ExecutionID, RunID, TurnID, SessionID, JobID string
	AcceptedAt                                   time.Time
	TargetedCheckIDs                             []string
}

type ClaimedExecutionJob struct {
	ExecutionID, RunID, TurnID, SessionID, JobID, Kind, PromptText string
	WorkspacePath                                                  string
	Connection                                                     agentconnection.ConnectionInput
	CheckIDs                                                       []string
	Checks                                                         []ExecutionCheckTarget
}

type ExecutionCheckTarget struct {
	CheckID, Instruction, ExpectedResult string
}

type ExecutionCheckResult struct {
	CheckID, Status, Evidence string
}

type ExecutionJobResult struct {
	Message   string
	Checks    []ExecutionCheckResult
	Cancelled bool
}

type ExecutionJobRepository interface {
	AcceptExecutionJob(context.Context, ExecutionJobRecord) (MutationResult[ExecutionJobAccepted], error)
	ClaimExecutionJob(context.Context) (*ClaimedExecutionJob, error)
	ReconnectExecutionSession(context.Context, ClaimedExecutionJob, string, string, time.Time) error
	CompleteExecutionJob(context.Context, ClaimedExecutionJob, ExecutionJobResult, error, time.Time) error
	CancelExecutionJob(context.Context, string, string, time.Time) (bool, error)
	AcceptExecutionCancellation(
		context.Context,
		CancelAgentOperationRecord,
	) (MutationResult[CancellationAccepted], bool, string, error)
	FailInterruptedExecutionJobs(context.Context, time.Time) error
}

type ExecutionJobExecutor interface {
	ExecuteExecutionJob(context.Context, ClaimedExecutionJob) (ExecutionJobResult, error)
	ExecutionSessionConnected(string) bool
	ConnectExecutionSession(context.Context, ClaimedExecutionJob, string) (string, error)
	DisconnectExecutionSession(string) error
	CancelExecutionOperation(context.Context, string) error
}

type ExecutionRunner struct {
	repository ExecutionJobRepository
	executor   ExecutionJobExecutor
	now        func() time.Time
	newID      func() string
}

func NewExecutionRunner(repository ExecutionJobRepository, executor ExecutionJobExecutor,
	now func() time.Time, newID func() string,
) *ExecutionRunner {
	return &ExecutionRunner{repository: repository, executor: executor, now: now, newID: newID}
}

func (r *ExecutionRunner) RunPendingChecks(ctx context.Context,
	input RunPendingChecksInput,
) (MutationResult[ExecutionJobAccepted], error) {
	at := r.now()

	return r.repository.AcceptExecutionJob(ctx, ExecutionJobRecord{
		ExecutionID: input.ExecutionID, JobID: r.newID(), RunID: r.newID(), Kind: "checks",
		ExpectedRevision: input.ExpectedRevision, CheckIDs: input.CheckIDs, OperationID: input.OperationID,
		AcceptedAt: at, Event: OutboxEvent{
			ID: r.newID(), Name: "execution.updated",
			AggregateType: "execution", AggregateID: input.ExecutionID, EmittedAt: at,
			Correlation: input.OperationID,
		},
	})
}

func (r *ExecutionRunner) SendMessage(ctx context.Context,
	input SendExecutionMessageInput,
) (MutationResult[ExecutionJobAccepted], error) {
	at := r.now()

	return r.repository.AcceptExecutionJob(ctx, ExecutionJobRecord{
		ExecutionID: input.ExecutionID, JobID: r.newID(), TurnID: r.newID(), Kind: "message",
		PromptText: input.Text, OperationID: input.OperationID, AcceptedAt: at,
		Event: OutboxEvent{
			ID: r.newID(), Name: "execution.updated", AggregateType: "execution",
			AggregateID: input.ExecutionID, EmittedAt: at, Correlation: input.OperationID,
		},
	})
}

func (r *ExecutionRunner) RunOne(ctx context.Context) (bool, error) {
	job, err := r.repository.ClaimExecutionJob(ctx)
	if err != nil || job == nil {
		return false, err
	}

	var (
		response   ExecutionJobResult
		executeErr error
	)

	if !r.executor.ExecutionSessionConnected(job.SessionID) {
		newSessionID := r.newID()

		var agentSessionID string

		agentSessionID, executeErr = r.executor.ConnectExecutionSession(ctx, *job, newSessionID)
		if executeErr == nil {
			executeErr = r.repository.ReconnectExecutionSession(ctx, *job, newSessionID, agentSessionID, r.now())
			if executeErr == nil {
				job.SessionID = newSessionID
			} else {
				_ = r.executor.DisconnectExecutionSession(newSessionID)
			}
		}
	}

	if executeErr == nil {
		response, executeErr = r.executor.ExecuteExecutionJob(ctx, *job)
	}

	completionCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), jobCompletionTimeout)
	defer cancel()

	if err := r.repository.CompleteExecutionJob(completionCtx, *job, response, executeErr, r.now()); err != nil {
		return true, err
	}

	return true, executeErr
}

func (r *ExecutionRunner) Cancel(ctx context.Context, jobID, operationID string) error {
	running, err := r.repository.CancelExecutionJob(ctx, jobID, operationID, r.now())
	if err != nil {
		return err
	}

	if !running {
		return nil
	}

	return r.executor.CancelExecutionOperation(ctx, jobID)
}

func (r *ExecutionRunner) CancelAgentOperation(
	ctx context.Context,
	input CancelAgentOperationInput,
) (MutationResult[CancellationAccepted], error) {
	result, running, jobID, err := r.repository.AcceptExecutionCancellation(ctx, CancelAgentOperationRecord{
		CancelAgentOperationInput: input,
		CancellationJobID:         r.newID(), EventID: r.newID(), RequestedAt: r.now(),
	})
	if err != nil || !running {
		return result, err
	}

	if err := r.executor.CancelExecutionOperation(ctx, jobID); err != nil {
		return result, err
	}

	return result, nil
}
