package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/yukihito-jokyu/TEJUN/internal/application"
)

func TestProcedureRevisionTerminalRecovery(t *testing.T) {
	for _, tc := range []struct {
		name, action, wantState, wantCode string
	}{
		{"worker failure", "fail", "failed", "generation_failed"},
		{"running cancellation", "cancel", "cancelled", "cancelled"},
		{"restart interruption", "restart", "interrupted", "interrupted"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			repo, at := procedureRevisionFixture(t)

			job, err := repo.ClaimProcedureRevision(ctx)
			if err != nil || job == nil {
				t.Fatalf("claim=%+v err=%v", job, err)
			}

			switch tc.action {
			case "fail":
				err = repo.FinishProcedureRevision(ctx, *job, "", errors.New("ACP failed"), at, "terminal")
			case "cancel":
				result, running, target, cancelErr := repo.CancelProcedureRevision(
					ctx,
					"session",
					"turn",
					"job",
					"cancel",
					at,
				)
				if cancelErr != nil || !running || target != "job" ||
					result.Data.TargetStatus != "cancellation_requested" {
					t.Fatalf("cancel=%+v running=%t target=%s err=%v", result, running, target, cancelErr)
				}

				err = repo.FinishProcedureRevision(ctx, *job, "", errors.New("cancelled"), at, "terminal")
			case "restart":
				err = repo.FailInterruptedProcedureRevisions(ctx, at)
			}

			if err != nil {
				t.Fatal(err)
			}

			var jobState, turnState, code string
			if err := repo.db.QueryRowContext(ctx, `SELECT j.state,t.status,j.error_code FROM procedure_revision_jobs j JOIN procedure_turns t ON t.turn_id=j.turn_id WHERE j.job_id='job'`).
				Scan(&jobState, &turnState, &code); err != nil {
				t.Fatal(err)
			}

			if jobState != tc.wantState || turnState != tc.wantState || code != tc.wantCode {
				t.Fatalf("job=%s turn=%s code=%s", jobState, turnState, code)
			}

			view, err := repo.GetProcedure(ctx, application.ProcedureViewQuery{ProjectID: "p"})
			if err != nil || view.Procedure.Revision != 1 || view.ActiveRevision != nil {
				t.Fatalf("view=%+v err=%v", view, err)
			}

			var events int
			if err := repo.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM event_outbox WHERE aggregate_id='procedure' AND event_id IN ('accepted','terminal','job:cancel','job:interrupted')`).
				Scan(&events); err != nil ||
				events < 2 {
				t.Fatalf("durable events=%d err=%v", events, err)
			}
		})
	}
}

func TestProcedureRevisionCommitSurvivesEventDeliveryGap(t *testing.T) {
	ctx := context.Background()
	repo, at := procedureRevisionFixture(t)

	job, err := repo.ClaimProcedureRevision(ctx)
	if err != nil || job == nil {
		t.Fatalf("claim=%+v err=%v", job, err)
	}

	if err := repo.FinishProcedureRevision(ctx, *job, "invalid response", nil, at, "terminal"); err != nil {
		t.Fatal(err)
	}

	// No dispatcher runs: the committed state must remain readable with its event in the outbox.
	view, err := repo.GetProcedure(ctx, application.ProcedureViewQuery{ProjectID: "p"})
	if err != nil || view.Procedure.Revision != 1 || view.ActiveRevision != nil {
		t.Fatalf("view=%+v err=%v", view, err)
	}

	var (
		state        string
		dispatchedAt any
	)

	if err := repo.db.QueryRowContext(ctx, `SELECT state FROM procedure_revision_jobs WHERE job_id='job'`).
		Scan(&state); err != nil {
		t.Fatal(err)
	}

	if err := repo.db.QueryRowContext(ctx, `SELECT dispatched_at FROM event_outbox WHERE event_id='terminal'`).
		Scan(&dispatchedAt); err != nil {
		t.Fatal(err)
	}

	if state != "failed" || dispatchedAt != nil {
		t.Fatalf("job=%s dispatched=%v", state, dispatchedAt)
	}
}
