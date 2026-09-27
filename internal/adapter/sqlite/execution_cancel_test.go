package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/yukihito-jokyu/TEJUN/internal/application"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/shared"
)

func TestExecutionCancellation(t *testing.T) {
	cases := []struct {
		name, state, session, run, turn, job, wantStatus, wantError string
		wantRunning                                                 bool
	}{
		{"queued run", "queued", "s", "r", "", "", "cancelled", "", false},
		{"running turn", "running", "s", "", "t", "", "cancellation_requested", "", true},
		{"wrong session", "running", "other", "", "", "j", "", "validation_error", false},
		{"mismatched ids", "running", "s", "r", "wrong", "", "", "not_found", false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			repo, at := executionFixture(t)

			ctx := context.Background()
			if _, err := repo.db.ExecContext(ctx, `INSERT INTO execution_agent_jobs
(job_id,execution_id,run_id,turn_id,session_id,kind,state,prompt_text,accepted_at)
VALUES ('j','e',NULL,NULL,'s','message',?,'hello',?)`, tt.state, at.Format("2006-01-02T15:04:05Z07:00")); err != nil {
				t.Fatal(err)
			}

			if _, err := repo.db.ExecContext(
				ctx,
				`INSERT INTO execution_turns(turn_id,execution_id,role,text,status,created_at)
VALUES ('t','e','user','hello','queued',?)`,
				at.Format("2006-01-02T15:04:05Z07:00"),
			); err != nil {
				t.Fatal(err)
			}

			if _, err := repo.db.ExecContext(
				ctx,
				`UPDATE execution_agent_jobs SET turn_id='t' WHERE job_id='j'`,
			); err != nil {
				t.Fatal(err)
			}

			if _, err := repo.db.ExecContext(
				ctx,
				`INSERT INTO execution_runs(run_id,execution_id,session_id,job_id,state,accepted_at)
VALUES ('r','e','s','j',?,?)`,
				tt.state,
				at.Format("2006-01-02T15:04:05Z07:00"),
			); err != nil {
				t.Fatal(err)
			}

			if _, err := repo.db.ExecContext(
				ctx,
				`UPDATE execution_agent_jobs SET run_id='r' WHERE job_id='j'`,
			); err != nil {
				t.Fatal(err)
			}

			in := application.CancelAgentOperationRecord{
				CancelAgentOperationInput: application.CancelAgentOperationInput{
					SessionID: tt.session, RunID: tt.run, TurnID: tt.turn, JobID: tt.job, OperationID: "cancel:1",
				},
				CancellationJobID: "cancel-job",
				EventID:           "cancel-event",
				RequestedAt:       at,
			}
			got, running, jobID, err := repo.AcceptExecutionCancellation(ctx, in)

			if tt.wantError != "" {
				var domainErr *shared.Error
				if !errors.As(err, &domainErr) || domainErr.Code != tt.wantError {
					t.Fatalf("err=%v", err)
				}

				return
			}

			if err != nil || got.Data.TargetStatus != tt.wantStatus || running != tt.wantRunning || jobID != "j" {
				t.Fatalf("result=%+v running=%v job=%s err=%v", got, running, jobID, err)
			}

			replayed, rerun, _, err := repo.AcceptExecutionCancellation(ctx, in)
			if err != nil || rerun || replayed.Receipt != got.Receipt {
				t.Fatalf("replay=%+v running=%v err=%v", replayed, rerun, err)
			}
		})
	}
}
