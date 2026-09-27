package sqlite

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/yukihito-jokyu/TEJUN/internal/application"
)

func TestProcedureRevisionJob(t *testing.T) {
	ctx := context.Background()
	executionRepo, at := executionFixture(t)

	db := executionRepo.db
	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO execution_evidence(evidence_id,execution_id,check_id,actor,kind,text,display_name,created_at) VALUES ('evidence','e','c','human','text','observed','Text','2026-09-26T00:00:00Z')`,
	); err != nil {
		t.Fatal(err)
	}

	if _, err := db.ExecContext(
		ctx,
		`UPDATE execution_checks SET human_status='completed' WHERE execution_id='e' AND check_id='c'`,
	); err != nil {
		t.Fatal(err)
	}

	_, err := executionRepo.GenerateProcedureDraft(
		ctx,
		application.ProcedureDraftRecord{
			ExecutionID:      "e",
			ProcedureID:      "procedure",
			OperationID:      "generate",
			ExpectedRevision: 1,
			At:               at,
			Event: application.OutboxEvent{
				ID:            "event",
				Name:          "procedure.updated",
				AggregateType: "procedure",
				AggregateID:   "procedure",
				EmittedAt:     at,
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	repo := NewProcedureRepository(db, nil)
	input := application.ProcedureRevisionRecord{
		RequestProcedureRevisionInput: application.RequestProcedureRevisionInput{
			ProcedureID:      "procedure",
			ExpectedRevision: 1,
			OperationID:      "revision",
			Content:          []application.ContentPart{{Type: "text", Text: "概要を更新"}},
		},
		TurnID:    "turn",
		JobID:     "job",
		SessionID: "session",
		EventID:   "accepted",
		At:        at,
	}

	accepted, err := repo.AcceptProcedureRevision(ctx, input)
	if err != nil || accepted.Data.JobID != "job" {
		t.Fatalf("accepted=%+v err=%v", accepted, err)
	}

	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO elicitation_requests(elicitation_request_id,connection_attempt_id,process_generation,mode,message,state,requested_at,session_id,scope_json,requested_schema_json)
 VALUES('question','attempt',1,'form','続けますか','pending','2026-09-26T00:00:00Z','session','{}','{"type":"object"}')`,
	); err != nil {
		t.Fatal(err)
	}

	control, err := repo.GetProcedure(ctx, application.ProcedureViewQuery{ProjectID: "p"})
	if err != nil || control.ActiveRevision == nil || control.ActiveRevision.SessionID != "session" ||
		control.ActiveRevision.TurnID != "turn" || control.ActiveRevision.JobID != "job" ||
		len(control.Elicitations) != 1 || control.Elicitations[0].ElicitationRequestID != "question" {
		t.Fatalf("control=%+v err=%v", control, err)
	}

	duplicate, err := repo.AcceptProcedureRevision(ctx, input)
	if err != nil || duplicate.Data.JobID != "job" {
		t.Fatalf("duplicate=%+v err=%v", duplicate, err)
	}

	job, err := repo.ClaimProcedureRevision(ctx)
	if err != nil || job == nil || job.SessionID != "session" {
		t.Fatalf("job=%+v err=%v", job, err)
	}

	view, err := repo.GetProcedure(ctx, application.ProcedureViewQuery{ProjectID: "p"})
	if err != nil {
		t.Fatal(err)
	}

	doc := view.Procedure.Document
	doc.Overview = "更新済み"

	answer, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}

	if err := repo.FinishProcedureRevision(ctx, *job, string(answer), nil, at, "finished"); err != nil {
		t.Fatal(err)
	}

	view, err = repo.GetProcedure(ctx, application.ProcedureViewQuery{ProjectID: "p"})
	if err != nil || view.Procedure.Revision != 2 || view.Procedure.Document.Overview != "更新済み" {
		t.Fatalf("view=%+v err=%v", view, err)
	}

	if view.ActiveRevision != nil || view.Elicitations == nil || len(view.Elicitations) != 0 {
		t.Fatalf("finished control=%+v", view)
	}
}

func TestProcedureRevisionRejectsTrailingJSON(t *testing.T) {
	ctx := context.Background()
	repo, at := procedureRevisionFixture(t)

	job, err := repo.ClaimProcedureRevision(ctx)
	if err != nil || job == nil {
		t.Fatalf("claim=%+v err=%v", job, err)
	}

	view, err := repo.GetProcedure(ctx, application.ProcedureViewQuery{ProjectID: "p"})
	if err != nil {
		t.Fatal(err)
	}

	answer, err := json.Marshal(view.Procedure.Document)
	if err != nil {
		t.Fatal(err)
	}

	for _, suffix := range []string{" {}", " trailing"} {
		if err := repo.FinishProcedureRevision(
			ctx,
			*job,
			string(answer)+suffix,
			nil,
			at,
			"finished"+suffix,
		); err != nil {
			t.Fatal(err)
		}

		var state, code string
		if err := repo.db.QueryRowContext(ctx, `SELECT state,error_code FROM procedure_revision_jobs WHERE job_id='job'`).
			Scan(&state, &code); err != nil {
			t.Fatal(err)
		}

		if state != "failed" || code != "invalid_response" {
			t.Fatalf("state=%s code=%s", state, code)
		}
	}
}

func TestProcedureRevisionRecovery(t *testing.T) {
	ctx := context.Background()

	repo, at := procedureRevisionFixture(t)
	if _, err := repo.ClaimProcedureRevision(ctx); err != nil {
		t.Fatal(err)
	}

	if err := repo.FailInterruptedProcedureRevisions(ctx, at); err != nil {
		t.Fatal(err)
	}

	var jobState, turnState string
	if err := repo.db.QueryRowContext(ctx, `SELECT j.state,t.status FROM procedure_revision_jobs j JOIN procedure_turns t ON t.turn_id=j.turn_id WHERE j.job_id='job'`).
		Scan(&jobState, &turnState); err != nil {
		t.Fatal(err)
	}

	if jobState != "interrupted" || turnState != "interrupted" {
		t.Fatalf("job=%s turn=%s", jobState, turnState)
	}

	var events int
	if err := repo.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM event_outbox WHERE event_id='job:interrupted' AND name='procedure.updated'`).
		Scan(&events); err != nil ||
		events != 1 {
		t.Fatalf("events=%d err=%v", events, err)
	}

	if err := repo.FailInterruptedProcedureRevisions(ctx, at); err != nil {
		t.Fatal(err)
	}
}

func procedureRevisionFixture(t *testing.T) (*ProcedureRepository, time.Time) {
	t.Helper()

	ctx := context.Background()
	executionRepo, at := executionFixture(t)

	db := executionRepo.db
	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO execution_evidence(evidence_id,execution_id,check_id,actor,kind,text,display_name,created_at) VALUES ('evidence','e','c','human','text','observed','Text','2026-09-26T00:00:00Z')`,
	); err != nil {
		t.Fatal(err)
	}

	if _, err := db.ExecContext(
		ctx,
		`UPDATE execution_checks SET human_status='completed' WHERE execution_id='e' AND check_id='c'`,
	); err != nil {
		t.Fatal(err)
	}

	if _, err := executionRepo.GenerateProcedureDraft(
		ctx,
		application.ProcedureDraftRecord{
			ExecutionID:      "e",
			ProcedureID:      "procedure",
			OperationID:      "generate",
			ExpectedRevision: 1,
			At:               at,
			Event: application.OutboxEvent{
				ID:            "event",
				Name:          "procedure.updated",
				AggregateType: "procedure",
				AggregateID:   "procedure",
				EmittedAt:     at,
			},
		},
	); err != nil {
		t.Fatal(err)
	}

	repo := NewProcedureRepository(db, nil)

	input := application.ProcedureRevisionRecord{
		RequestProcedureRevisionInput: application.RequestProcedureRevisionInput{
			ProcedureID:      "procedure",
			ExpectedRevision: 1,
			OperationID:      "revision",
			Content:          []application.ContentPart{{Type: "text", Text: "概要を更新"}},
		},
		TurnID:    "turn",
		JobID:     "job",
		SessionID: "session",
		EventID:   "accepted",
		At:        at,
	}
	if _, err := repo.AcceptProcedureRevision(ctx, input); err != nil {
		t.Fatal(err)
	}

	return repo, at
}
