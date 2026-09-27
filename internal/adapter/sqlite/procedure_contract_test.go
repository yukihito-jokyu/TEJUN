package sqlite

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/yukihito-jokyu/TEJUN/internal/application"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/shared"
)

func TestProcedureSaveContractRollbackAndReplay(t *testing.T) {
	ctx := context.Background()
	repo, at := procedureContractFixture(t)
	app := application.NewProcedure(repo, func() time.Time { return at }, func() string { return "save-event" })

	view, err := app.Get(ctx, application.ProcedureViewQuery{ProjectID: "p"})
	if err != nil || view.Procedure.Revision != 1 {
		t.Fatalf("get=%+v err=%v", view, err)
	}

	doc := view.Procedure.Document
	doc.Overview = "編集後"
	input := application.ProcedureSaveInput{
		ProcedureID:      "procedure",
		ExpectedRevision: 1,
		OperationID:      "save",
		Document:         doc,
	}

	if _, err := repo.db.ExecContext(
		ctx,
		`CREATE TRIGGER reject_procedure_event BEFORE INSERT ON event_outbox WHEN NEW.event_id='save-event' BEGIN SELECT RAISE(ABORT,'failpoint'); END`,
	); err != nil {
		t.Fatal(err)
	}

	if _, err := app.SaveDraft(ctx, input); err == nil {
		t.Fatal("outbox failure accepted")
	}

	view, err = app.Get(ctx, application.ProcedureViewQuery{ProjectID: "p"})
	if err != nil || view.Procedure.Revision != 1 || view.Procedure.Document.Overview == "編集後" {
		t.Fatalf("partial commit: %+v err=%v", view, err)
	}

	if _, err := repo.db.ExecContext(ctx, `DROP TRIGGER reject_procedure_event`); err != nil {
		t.Fatal(err)
	}

	first, err := app.SaveDraft(ctx, input)
	if err != nil || first.Data.Revision != 2 || first.Receipt.ChangeSequence == 0 {
		t.Fatalf("save=%+v err=%v", first, err)
	}

	replay, err := app.SaveDraft(ctx, input)
	if err != nil || !reflect.DeepEqual(replay, first) {
		t.Fatalf("replay=%+v err=%v", replay, err)
	}

	for _, tc := range []struct{ name, operation, overview, code string }{
		{"same id different payload", "save", "別内容", "operation_id_conflict"},
		{"new id stale revision", "second", "編集後", "revision_conflict"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := doc
			changed.Overview = tc.overview
			_, err := app.SaveDraft(
				ctx,
				application.ProcedureSaveInput{
					ProcedureID:      "procedure",
					ExpectedRevision: 1,
					OperationID:      tc.operation,
					Document:         changed,
				},
			)

			var business *shared.Error
			if !errors.As(err, &business) || business.Code != tc.code {
				t.Fatalf("err=%v, want %s", err, tc.code)
			}
		})
	}

	var eventCount int
	if err := repo.db.QueryRowContext(ctx, `SELECT count(*) FROM event_outbox WHERE event_id='save-event' AND name='procedure.updated'`).
		Scan(&eventCount); err != nil ||
		eventCount != 1 {
		t.Fatalf("outbox count=%d err=%v", eventCount, err)
	}
}

func TestProcedureRevisionRequestContract(t *testing.T) {
	ctx := context.Background()
	repo, at := procedureContractFixture(t)
	input := application.ProcedureRevisionRecord{
		RequestProcedureRevisionInput: application.RequestProcedureRevisionInput{
			ProcedureID: "procedure", ExpectedRevision: 1, OperationID: "revise",
			Content: []application.ContentPart{{Type: "text", Text: "概要を直す"}},
		},
		TurnID: "turn", JobID: "job", SessionID: "session", EventID: "revision-event", At: at,
	}

	first, err := repo.AcceptProcedureRevision(ctx, input)
	if err != nil || first.Data.JobID != "job" || first.Receipt.ChangeSequence == 0 {
		t.Fatalf("accept=%+v err=%v", first, err)
	}

	input.TurnID, input.JobID, input.SessionID, input.EventID = "other-turn", "other-job", "other-session", "other-event"

	replay, err := repo.AcceptProcedureRevision(ctx, input)
	if err != nil || !reflect.DeepEqual(replay, first) {
		t.Fatalf("replay=%+v err=%v", replay, err)
	}

	for _, tc := range []struct{ name, operation, text, code string }{
		{"same id different request", "revise", "別の依頼", "operation_id_conflict"},
		{"new id while pending", "second", "概要を直す", "invalid_state"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := input
			changed.OperationID = tc.operation
			changed.Content = []application.ContentPart{{Type: "text", Text: tc.text}}
			_, err := repo.AcceptProcedureRevision(ctx, changed)

			var business *shared.Error
			if !errors.As(err, &business) || business.Code != tc.code {
				t.Fatalf("err=%v, want %s", err, tc.code)
			}
		})
	}

	var jobs, events int
	if err := repo.db.QueryRowContext(ctx, `SELECT count(*) FROM procedure_revision_jobs WHERE procedure_id='procedure'`).
		Scan(&jobs); err != nil {
		t.Fatal(err)
	}

	if err := repo.db.QueryRowContext(ctx, `SELECT count(*) FROM event_outbox WHERE event_id='revision-event'`).
		Scan(&events); err != nil || jobs != 1 ||
		events != 1 {
		t.Fatalf("jobs=%d events=%d err=%v", jobs, events, err)
	}
}

func procedureContractFixture(t *testing.T) (*ProcedureRepository, time.Time) {
	t.Helper()

	ctx := context.Background()

	execution, at := executionFixture(t)
	for _, query := range []string{
		`INSERT INTO execution_evidence(evidence_id,execution_id,check_id,actor,kind,text,display_name,created_at) VALUES ('evidence','e','c','human','text','observed','Text','2026-09-26T00:00:00Z')`,
		`UPDATE execution_checks SET human_status='completed' WHERE execution_id='e' AND check_id='c'`,
	} {
		if _, err := execution.db.ExecContext(ctx, query); err != nil {
			t.Fatal(err)
		}
	}

	_, err := execution.GenerateProcedureDraft(ctx, application.ProcedureDraftRecord{
		ExecutionID:      "e",
		ProcedureID:      "procedure",
		OperationID:      "generate",
		ExpectedRevision: 1,
		At:               at,
		Event: application.OutboxEvent{
			ID:            "generate-event",
			Name:          "procedure.updated",
			AggregateType: "procedure",
			AggregateID:   "procedure",
			EmittedAt:     at,
		},
	})
	if err != nil {
		t.Fatal(fmt.Errorf("generate fixture: %w", err))
	}

	return NewProcedureRepository(execution.db, nil), at
}
