package application

import (
	"context"
	"testing"
	"time"
)

type sequenceExecutionRepository struct {
	ExecutionJobRepository
	job     *ClaimedExecutionJob
	started []string
	saved   []string
	ended   bool
}

func (r *sequenceExecutionRepository) ClaimExecutionJob(context.Context) (*ClaimedExecutionJob, error) {
	return r.job, nil
}

func (r *sequenceExecutionRepository) StartExecutionCheck(_ context.Context, _ ClaimedExecutionJob,
	check ExecutionCheckTarget, _ time.Time,
) error {
	r.started = append(r.started, check.CheckID)
	return nil
}

func (r *sequenceExecutionRepository) CompleteExecutionCheck(_ context.Context, _ ClaimedExecutionJob,
	check ExecutionCheckTarget, _ ExecutionCheckResult, _ []string, _ time.Time,
) error {
	r.saved = append(r.saved, check.CheckID)
	return nil
}

func (r *sequenceExecutionRepository) CompleteExecutionJob(_ context.Context, _ ClaimedExecutionJob,
	result ExecutionJobResult, err error, _ time.Time,
) error {
	r.ended = err == nil && result.Incremental && len(result.Checks) == 2
	return nil
}

type sequenceExecutionExecutor struct {
	ExecutionJobExecutor
	repository *sequenceExecutionRepository
	called     []string
}

func (*sequenceExecutionExecutor) ExecutionSessionConnected(string) bool { return true }

func (e *sequenceExecutionExecutor) ExecuteExecutionJob(_ context.Context,
	job ClaimedExecutionJob,
) (ExecutionJobResult, error) {
	check := job.Checks[0]
	if len(job.Checks) != 1 || len(e.repository.saved) != len(e.called) {
		return ExecutionJobResult{}, context.Canceled
	}

	e.called = append(e.called, check.CheckID)

	return ExecutionJobResult{Checks: []ExecutionCheckResult{{
		CheckID: check.CheckID, Status: "completed", Evidence: "observed",
	}}}, nil
}

func TestExecutionRunnerSavesEachCheckBeforeNext(t *testing.T) {
	for _, tc := range []struct {
		name   string
		checks []ExecutionCheckTarget
	}{
		{"two checks", []ExecutionCheckTarget{{CheckID: "c1", Sequence: 1}, {CheckID: "c2", Sequence: 2}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repository := &sequenceExecutionRepository{job: &ClaimedExecutionJob{
				JobID: "job", SessionID: "session", Kind: "checks", Checks: tc.checks,
			}}
			executor := &sequenceExecutionExecutor{repository: repository}
			runner := NewExecutionRunner(repository, executor, time.Now, func() string { return "id" })

			run, err := runner.RunOne(context.Background())
			if err != nil || !run || !repository.ended || len(repository.saved) != 2 ||
				repository.started[0] != "c1" || repository.started[1] != "c2" ||
				repository.saved[0] != "c1" || repository.saved[1] != "c2" {
				t.Fatalf("run=%t err=%v started=%v called=%v saved=%v ended=%t",
					run, err, repository.started, executor.called, repository.saved, repository.ended)
			}
		})
	}
}
