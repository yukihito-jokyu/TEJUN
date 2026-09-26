package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/yukihito-jokyu/TEJUN/internal/domain/agentconnection"
)

type jobTestRepository struct {
	job                   *ClaimedAgentJob
	complete              bool
	completeContextError  error
	elicitationContextErr error
	elicitationSucceeded  bool
	elicitationClaimed    bool
	elicitationResult     MutationResult[ElicitationResponseResult]
	elicitationCompleted  int
}

func (r *jobTestRepository) AcceptAgentJob(context.Context, AgentJobRecord) (MutationResult[AgentJobAccepted], error) {
	return MutationResult[AgentJobAccepted]{}, nil
}

func (r *jobTestRepository) ClaimAgentJob(context.Context) (*ClaimedAgentJob, error) {
	job := r.job
	r.job = nil

	return job, nil
}

func (r *jobTestRepository) CompleteAgentJob(ctx context.Context, _ string, succeeded bool, _ time.Time) error {
	r.complete = succeeded
	r.completeContextError = ctx.Err()

	return nil
}

func (r *jobTestRepository) ClaimElicitation(
	context.Context,
	ElicitationClaim,
) (MutationResult[ElicitationResponseResult], bool, error) {
	return r.elicitationResult, r.elicitationClaimed, nil
}

func (r *jobTestRepository) CompleteElicitation(
	ctx context.Context,
	_ string,
	_ string,
	succeeded bool,
	_ time.Time,
) error {
	r.elicitationContextErr = ctx.Err()
	r.elicitationSucceeded = succeeded
	r.elicitationCompleted++

	return nil
}

type jobTestExecutor struct{ err error }

func (e jobTestExecutor) Execute(context.Context, ClaimedAgentJob) error { return e.err }

type elicitationTestResponder struct {
	err   error
	calls *int
}

func (r elicitationTestResponder) Respond(context.Context, ElicitationResponse) error {
	if r.calls != nil {
		*r.calls++
	}

	return r.err
}

type authMethodTestResolver struct{}

func (authMethodTestResolver) ResolveAuthMethod(context.Context, string, string) (string, error) {
	return "", nil
}

func TestRunAgentJob(t *testing.T) {
	tests := []struct {
		name        string
		job         *ClaimedAgentJob
		executeErr  error
		cancel      bool
		wantRan     bool
		wantSuccess bool
	}{
		{name: "no pending job"},
		{
			name: "success", job: &ClaimedAgentJob{JobID: "job-1", Kind: agentconnection.JobAuthenticate},
			wantRan: true, wantSuccess: true,
		},
		{
			name: "external failure", job: &ClaimedAgentJob{JobID: "job-1", Kind: agentconnection.JobLogout},
			executeErr: errors.New("failed"), wantRan: true,
		},
		{
			name: "cancelled execution is persisted", job: &ClaimedAgentJob{JobID: "job-1"},
			executeErr: context.Canceled, cancel: true, wantRan: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repository := &jobTestRepository{job: tt.job}

			ctx, cancel := context.WithCancel(context.Background())
			if tt.cancel {
				cancel()
			} else {
				defer cancel()
			}

			ran, err := RunAgentJob(ctx, repository, jobTestExecutor{err: tt.executeErr}, time.Now)
			if ran != tt.wantRan || !errors.Is(err, tt.executeErr) {
				t.Fatalf("RunAgentJob() = (%v, %v), want (%v, %v)", ran, err, tt.wantRan, tt.executeErr)
			}

			if repository.complete != tt.wantSuccess {
				t.Fatalf("complete=%v, want %v", repository.complete, tt.wantSuccess)
			}

			if repository.completeContextError != nil {
				t.Fatalf("completion context error=%v", repository.completeContextError)
			}
		})
	}
}

func TestRespondToElicitationPersistsCancelledResponse(t *testing.T) {
	repository := &jobTestRepository{elicitationClaimed: true}
	control := NewAgentControl(
		repository,
		elicitationTestResponder{err: context.Canceled},
		authMethodTestResolver{},
		time.Now,
		func() string { return "unused" },
	)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := control.RespondToElicitation(ctx, RespondToElicitationInput{
		ElicitationRequestID: "elicitation-1", OperationID: "operation-1", Action: "decline",
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v, want context.Canceled", err)
	}

	if repository.elicitationContextErr != nil || repository.elicitationSucceeded {
		t.Fatalf(
			"completion context error=%v succeeded=%v",
			repository.elicitationContextErr,
			repository.elicitationSucceeded,
		)
	}
}

func TestRespondToElicitationReturnsStoredResult(t *testing.T) {
	respondedAt := time.Date(2026, 9, 26, 1, 2, 3, 0, time.UTC)
	stored := MutationResult[ElicitationResponseResult]{
		Data: ElicitationResponseResult{
			ElicitationRequestID: "elicitation-1", Status: "responded", RespondedAt: respondedAt,
		},
		Receipt: MutationReceipt{OperationID: "operation-1", CommittedAt: respondedAt},
	}
	repository := &jobTestRepository{elicitationResult: stored}
	responderCalls := 0
	control := NewAgentControl(
		repository,
		elicitationTestResponder{calls: &responderCalls},
		authMethodTestResolver{},
		time.Now,
		func() string { return "unused" },
	)

	result, err := control.RespondToElicitation(context.Background(), RespondToElicitationInput{
		ElicitationRequestID: "elicitation-1", OperationID: "operation-1", Action: "decline",
	})
	if err != nil {
		t.Fatal(err)
	}

	if result != stored || responderCalls != 0 || repository.elicitationCompleted != 0 {
		t.Fatalf(
			"result=%+v responder calls=%d completions=%d",
			result,
			responderCalls,
			repository.elicitationCompleted,
		)
	}
}
