package application

import (
	"context"
	"strings"
	"time"

	"github.com/yukihito-jokyu/TEJUN/internal/domain/agentconnection"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/shared"
	"github.com/yukihito-jokyu/TEJUN/internal/trace"
)

type RequestProcedureRevisionInput struct {
	ProcedureID      string        `json:"procedureId"`
	ExpectedRevision int64         `json:"expectedRevision"`
	Content          []ContentPart `json:"content"`
	OperationID      string        `json:"operationId"`
}
type ProcedureRevisionRecord struct {
	RequestProcedureRevisionInput
	TurnID, JobID, SessionID, EventID string
	At                                time.Time
}
type ClaimedProcedureRevision struct {
	ProcedureID, TurnID, JobID, SessionID, PromptText, WorkspacePath string
	ExpectedRevision                                                 int64
	Connection                                                       agentconnection.ConnectionInput
}
type ProcedureRevisionRepository interface {
	AcceptProcedureRevision(context.Context, ProcedureRevisionRecord) (MutationResult[AcceptedTurn], error)
	ClaimProcedureRevision(context.Context) (*ClaimedProcedureRevision, error)
	FinishProcedureRevision(context.Context, ClaimedProcedureRevision, string, error, time.Time, string) error
	FailInterruptedProcedureRevisions(context.Context, time.Time) error
	CancelProcedureRevision(
		context.Context,
		string,
		string,
		string,
		string,
		time.Time,
	) (MutationResult[CancellationAccepted], bool, string, error)
}

type ProcedureRevision struct {
	repository ProcedureRevisionRepository
	executor   ExecutionJobExecutor
	now        func() time.Time
	newID      func() string
}

func NewProcedureRevision(
	repo ProcedureRevisionRepository,
	executor ExecutionJobExecutor,
	now func() time.Time,
	newID func() string,
) *ProcedureRevision {
	return &ProcedureRevision{repo, executor, now, newID}
}

func (p *ProcedureRevision) Request(
	ctx context.Context,
	in RequestProcedureRevisionInput,
) (MutationResult[AcceptedTurn], error) {
	if in.ProcedureID == "" || in.OperationID == "" || in.ExpectedRevision < 1 || len(in.Content) == 0 {
		return MutationResult[AcceptedTurn]{}, &shared.Error{Code: "validation_failed", Message: "修正依頼が不正です"}
	}

	for _, part := range in.Content {
		if part.Type != "text" || strings.TrimSpace(part.Text) == "" || len(part.Text) > 100000 {
			return MutationResult[AcceptedTurn]{}, &shared.Error{Code: "validation_failed", Message: "contentが不正です"}
		}
	}

	return p.repository.AcceptProcedureRevision(
		ctx,
		ProcedureRevisionRecord{
			RequestProcedureRevisionInput: in,
			TurnID:                        p.newID(),
			JobID:                         p.newID(),
			SessionID:                     p.newID(),
			EventID:                       p.newID(),
			At:                            p.now(),
		},
	)
}

func (p *ProcedureRevision) RunOne(ctx context.Context) (bool, error) {
	job, err := p.repository.ClaimProcedureRevision(ctx)
	if err != nil || job == nil {
		return false, err
	}

	trace.Record(
		ctx,
		trace.Entry{
			Phase:       "worker_claim",
			Method:      "RequestProcedureRevision",
			JobID:       job.JobID,
			AggregateID: job.ProcedureID,
			Status:      "succeeded",
		},
	)

	execute := ClaimedExecutionJob{
		ExecutionID:   job.ProcedureID,
		TurnID:        job.TurnID,
		JobID:         job.JobID,
		SessionID:     job.SessionID,
		Kind:          "message",
		PromptText:    job.PromptText,
		WorkspacePath: job.WorkspacePath,
		Connection:    job.Connection,
	}
	trace.Record(
		ctx,
		trace.Entry{
			Phase:       "external_io_start",
			Method:      "ConnectProcedureRevision",
			JobID:       job.JobID,
			AggregateID: job.ProcedureID,
		},
	)
	_, err = p.executor.ConnectExecutionSession(ctx, execute, job.SessionID)

	var processGeneration int64

	if err == nil {
		if provider, ok := p.executor.(interface {
			Generation(context.Context, string) (int64, error)
		}); ok {
			processGeneration, _ = provider.Generation(ctx, job.SessionID)
		}
	}

	trace.Record(
		ctx,
		trace.Entry{
			Phase:             "external_io_end",
			Method:            "ConnectProcedureRevision",
			JobID:             job.JobID,
			AggregateID:       job.ProcedureID,
			Status:            traceStatus(err),
			ProcessGeneration: processGeneration,
		},
	)

	var response ExecutionJobResult

	if err == nil {
		trace.Record(
			ctx,
			trace.Entry{
				Phase:       "external_io_start",
				Method:      "ExecuteProcedureRevision",
				JobID:       job.JobID,
				AggregateID: job.ProcedureID,
			},
		)
		response, err = p.executor.ExecuteExecutionJob(ctx, execute)
		trace.Record(
			ctx,
			trace.Entry{
				Phase:       "external_io_end",
				Method:      "ExecuteProcedureRevision",
				JobID:       job.JobID,
				AggregateID: job.ProcedureID,
				Status:      traceStatus(err),
			},
		)
	}

	_ = p.executor.DisconnectExecutionSession(job.SessionID)

	answer := response.Message
	if response.Cancelled {
		err = &shared.Error{Code: "cancelled", Message: "修正依頼を取り消しました"}
	}

	completionCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()

	if finishErr := p.repository.FinishProcedureRevision(
		completionCtx,
		*job,
		answer,
		err,
		p.now(),
		p.newID(),
	); finishErr != nil {
		return true, finishErr
	}

	return true, err
}

func (p *ProcedureRevision) Cancel(
	ctx context.Context,
	sessionID, turnID, jobID, operationID string,
) (MutationResult[CancellationAccepted], error) {
	result, running, target, err := p.repository.CancelProcedureRevision(
		ctx,
		sessionID,
		turnID,
		jobID,
		operationID,
		p.now(),
	)
	if err != nil || !running {
		return result, err
	}
	// 取消受付は既にcommit済み。ACPへの通知失敗はworkerの取消状態で吸収する。
	trace.Record(
		ctx,
		trace.Entry{
			Phase:       "external_io_start",
			Method:      "CancelProcedureRevision",
			OperationID: operationID,
			JobID:       target,
		},
	)
	cancelErr := p.executor.CancelExecutionOperation(ctx, target)
	trace.Record(
		ctx,
		trace.Entry{
			Phase:       "external_io_end",
			Method:      "CancelProcedureRevision",
			OperationID: operationID,
			JobID:       target,
			Status:      traceStatus(cancelErr),
		},
	)

	return result, nil
}
