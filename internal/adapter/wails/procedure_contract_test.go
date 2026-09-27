package wails_test

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/yukihito-jokyu/TEJUN/internal/adapter/sqlite"
	appwails "github.com/yukihito-jokyu/TEJUN/internal/adapter/wails"
	"github.com/yukihito-jokyu/TEJUN/internal/application"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/shared"
	"github.com/yukihito-jokyu/TEJUN/internal/trace"
)

func TestProcedureTraceFollowsCommit(t *testing.T) {
	_, db := procedureServiceFixture(t)
	directory := t.TempDir()

	writer, err := trace.Open(directory)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = writer.Close() })

	repo := sqlite.NewProcedureRepository(db, nil)
	service := appwails.NewProcedureService(
		application.NewProcedure(repo, time.Now, func() string { return "trace-event" }), repo,
		appwails.NewProjectService(nil, nil, writer), nil, nil, nil,
	)

	view, err := service.GetProcedure(context.Background(), application.ProcedureViewQuery{ProjectID: "p"})
	if err != nil {
		t.Fatal(err)
	}

	in := application.ProcedureSaveInput{
		ProcedureID: "procedure", ExpectedRevision: 1, OperationID: "save-trace", Document: view.Procedure.Document,
	}

	if _, err := db.Exec(
		`CREATE TRIGGER reject_trace_event BEFORE INSERT ON event_outbox WHEN NEW.name='procedure.updated' BEGIN SELECT RAISE(ABORT,'failpoint'); END`,
	); err != nil {
		t.Fatal(err)
	}

	if _, err := service.SaveProcedureDraft(context.Background(), in); err == nil {
		t.Fatal("failed transaction accepted")
	}

	if _, err := db.Exec(`DROP TRIGGER reject_trace_event`); err != nil {
		t.Fatal(err)
	}

	result, err := service.SaveProcedureDraft(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := service.SaveProcedureDraft(context.Background(), in); err != nil {
		t.Fatal(err)
	}

	var entries []procedureTraceLine

	for _, row := range readProcedureTrace(t, directory) {
		if row.OperationID == in.OperationID {
			entries = append(entries, row)
		}
	}

	var phases []string
	for _, row := range entries {
		phases = append(phases, row.Phase)
		if row.AggregateID != in.ProcedureID {
			t.Fatalf("aggregate = %q", row.AggregateID)
		}
	}

	if !reflect.DeepEqual(
		phases,
		[]string{
			"binding_entry",
			"binding_entry",
			"transaction_commit",
			"accepted_response",
			"binding_entry",
			"accepted_response",
		},
	) {
		t.Fatalf("phase order = %v", phases)
	}

	if entries[2].EventID == "" || entries[2].ChangeSequence != result.Receipt.ChangeSequence ||
		entries[3].ChangeSequence != result.Receipt.ChangeSequence || entries[5].ChangeSequence != result.Receipt.ChangeSequence {
		t.Fatalf("commit correlation = %+v", entries)
	}
}

type procedureTraceLine struct {
	Phase          string `json:"phase"`
	Method         string `json:"method"`
	OperationID    string `json:"operationId"`
	JobID          string `json:"jobId"`
	EventID        string `json:"eventId"`
	AggregateID    string `json:"aggregateId"`
	ChangeSequence int64  `json:"changeSequence"`
}

func readProcedureTrace(t *testing.T, directory string) []procedureTraceLine {
	t.Helper()

	file, err := os.Open(filepath.Join(directory, "trace.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()

	var entries []procedureTraceLine

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var row procedureTraceLine
		if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
			t.Fatal(err)
		}

		entries = append(entries, row)
	}

	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}

	return entries
}

func TestProcedureCancelTraceFollowsCommit(t *testing.T) {
	_, db := procedureServiceFixture(t)
	directory := t.TempDir()

	writer, err := trace.Open(directory)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = writer.Close() })

	repo := sqlite.NewProcedureRepository(db, nil)
	id := 0
	newID := func() string { id++; return "cancel-id-" + string(rune('a'+id)) }
	revision := application.NewProcedureRevision(repo, nil, time.Now, newID)
	service := appwails.NewProcedureService(
		application.NewProcedure(repo, time.Now, newID), repo,
		appwails.NewProjectService(nil, nil, writer), revision, nil, nil,
	)

	accepted, err := service.RequestProcedureRevision(context.Background(), application.RequestProcedureRevisionInput{
		ProcedureID: "procedure", ExpectedRevision: 1, OperationID: "request-cancel-trace",
		Content: []application.ContentPart{{Type: "text", Text: "Update overview"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	view, err := service.GetProcedure(context.Background(), application.ProcedureViewQuery{ProjectID: "p"})
	if err != nil || view.ActiveRevision == nil {
		t.Fatalf("view = %+v, error = %v", view.ActiveRevision, err)
	}

	control := appwails.NewAgentControlService(nil, nil, nil, revision, writer)

	result, err := control.CancelAgentOperation(context.Background(), appwails.CancelAgentOperationInput{
		SessionID: view.ActiveRevision.SessionID, TurnID: view.ActiveRevision.TurnID,
		JobID: accepted.Data.JobID, OperationID: "cancel-trace",
	})
	if err != nil {
		t.Fatal(err)
	}

	var entries []procedureTraceLine

	for _, row := range readProcedureTrace(t, directory) {
		if row.OperationID == "cancel-trace" {
			entries = append(entries, row)
		}
	}

	var phases []string
	for _, row := range entries {
		phases = append(phases, row.Phase)
		if row.JobID != accepted.Data.JobID {
			t.Fatalf("job correlation = %+v", entries)
		}
	}

	if !reflect.DeepEqual(phases, []string{"binding_entry", "transaction_commit", "accepted_response"}) ||
		entries[1].EventID == "" || entries[1].ChangeSequence != result.Receipt.ChangeSequence ||
		entries[2].ChangeSequence != result.Receipt.ChangeSequence {
		t.Fatalf("cancel trace = %+v", entries)
	}
}

func procedureServiceFixture(t *testing.T) (*appwails.ProcedureService, *sql.DB) {
	t.Helper()

	ctx := context.Background()

	db, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "procedure.db"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	for _, statement := range []string{
		`INSERT INTO agent_connections(connection_id,display_name,command,args_json,transport,resolved_executable_path,last_verified_at,protocol_version,auth_state,schema_artifact_version,revision) VALUES('connection','Agent','agent','[]','stdio','/bin/agent','2026-09-26T00:00:00Z','1','not_required','1',1)`,
		`INSERT INTO projects(project_id,lineage_id,connection_id,name,workspace_path,status,current_stage,revision,created_at,updated_at) VALUES('p','p','connection','Project','/tmp','procedure_editing','procedure',1,1,1)`,
		`INSERT INTO preparation_sessions(session_id,project_id,state,started_at) VALUES('s','p','ready','2026-09-26T00:00:00Z')`,
		`INSERT INTO executions(execution_id,project_id,session_id,status,revision,started_at) VALUES('e','p','s','active',1,'2026-09-26T00:00:00Z')`,
		`INSERT INTO execution_checks(execution_id,check_id,sequence,title,instruction,expected_result,suggested_command,ai_required,human_required,ai_status,human_status,human_evidence_requirement) VALUES('e','c',1,'Check','Do it','Done','',1,1,'completed','completed','text')`,
		`INSERT INTO execution_evidence(evidence_id,execution_id,check_id,actor,kind,text,display_name,created_at) VALUES('v','e','c','human','text','observed','Text','2026-09-26T00:00:00Z')`,
	} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}

	at := time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)

	_, err = sqlite.NewExecutionRepository(db).GenerateProcedureDraft(ctx, application.ProcedureDraftRecord{
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
		t.Fatal(err)
	}

	repo := sqlite.NewProcedureRepository(db, nil)
	ids := 0
	newID := func() string { ids++; return "id-" + string(rune('a'+ids)) }
	now := func() time.Time { return at }

	return appwails.NewProcedureService(
		application.NewProcedure(repo, now, newID),
		repo,
		nil,
		application.NewProcedureRevision(repo, nil, now, newID),
		nil,
		nil,
	), db
}

func procedureCode(t *testing.T, err error, code string) {
	t.Helper()

	var business *shared.Error
	if !errors.As(err, &business) || business.Code != code {
		t.Fatalf("error=%v, want %s", err, code)
	}
}

func TestProcedureServiceSaveAndCompleteContract(t *testing.T) {
	ctx := context.Background()
	s, db := procedureServiceFixture(t)

	view, err := s.GetProcedure(ctx, application.ProcedureViewQuery{ProjectID: "p"})
	if err != nil || view.Procedure.Revision != 1 || len(view.Evidence.Human) != 1 {
		t.Fatalf("view=%+v err=%v", view, err)
	}

	evidence, err := s.GetEvidence(ctx, appwails.GetEvidenceInput{ProcedureID: "procedure", EvidenceID: "v"})
	if err != nil || evidence.TextPage == nil || evidence.TextPage.Content != "observed" {
		t.Fatalf("evidence=%+v err=%v", evidence, err)
	}

	_, err = s.GetEvidence(ctx, appwails.GetEvidenceInput{ProcedureID: "other", EvidenceID: "v"})
	procedureCode(t, err, "not_found")

	doc := view.Procedure.Document
	doc.Overview = "edited"
	in := application.ProcedureSaveInput{
		ProcedureID:      "procedure",
		ExpectedRevision: 1,
		OperationID:      "save",
		Document:         doc,
	}

	if _, err = db.ExecContext(
		ctx,
		`CREATE TRIGGER fail_procedure_event BEFORE INSERT ON event_outbox WHEN NEW.name='procedure.updated' BEGIN SELECT RAISE(ABORT,'failpoint'); END`,
	); err != nil {
		t.Fatal(err)
	}

	if _, err = s.SaveProcedureDraft(ctx, in); err == nil {
		t.Fatal("outbox failure accepted")
	}

	view, err = s.GetProcedure(ctx, application.ProcedureViewQuery{ProjectID: "p"})
	if err != nil || view.Procedure.Revision != 1 {
		t.Fatalf("partial commit: %+v %v", view, err)
	}

	if _, err = db.ExecContext(ctx, `DROP TRIGGER fail_procedure_event`); err != nil {
		t.Fatal(err)
	}

	first, err := s.SaveProcedureDraft(ctx, in)
	if err != nil || first.Data.Revision != 2 {
		t.Fatalf("save=%+v %v", first, err)
	}

	replay, err := s.SaveProcedureDraft(ctx, in)
	if err != nil || !reflect.DeepEqual(first, replay) {
		t.Fatalf("replay=%+v %v", replay, err)
	}

	changed := in
	changed.Document.Overview = "different"
	_, err = s.SaveProcedureDraft(ctx, changed)
	procedureCode(t, err, "operation_id_conflict")

	changed.OperationID = "stale"
	_, err = s.SaveProcedureDraft(ctx, changed)
	procedureCode(t, err, "revision_conflict")

	completed, err := s.CompleteProcedure(
		ctx,
		application.ProcedureCompleteInput{ProcedureID: "procedure", ExpectedRevision: 2, OperationID: "complete"},
	)
	if err != nil || completed.Data.Revision != 3 {
		t.Fatalf("complete=%+v %v", completed, err)
	}

	view, err = s.GetProcedure(ctx, application.ProcedureViewQuery{ProjectID: "p"})
	if err != nil || string(view.Procedure.Status) != "completed" {
		t.Fatalf("completed view=%+v %v", view, err)
	}

	in.OperationID = "after-complete"
	in.ExpectedRevision = 3
	_, err = s.SaveProcedureDraft(ctx, in)
	procedureCode(t, err, "invalid_state")
}

func TestProcedureServiceRevisionContract(t *testing.T) {
	ctx := context.Background()
	s, db := procedureServiceFixture(t)
	in := application.RequestProcedureRevisionInput{
		ProcedureID:      "procedure",
		ExpectedRevision: 1,
		OperationID:      "revise",
		Content:          []application.ContentPart{{Type: "text", Text: "Update overview"}},
	}

	accepted, err := s.RequestProcedureRevision(ctx, in)
	if err != nil || accepted.Data.JobID == "" || accepted.Receipt.ChangeSequence == 0 {
		t.Fatalf("accepted=%+v %v", accepted, err)
	}

	view, err := s.GetProcedure(ctx, application.ProcedureViewQuery{ProjectID: "p"})
	if err != nil || view.ActiveRevision == nil || view.ActiveRevision.JobID != accepted.Data.JobID {
		t.Fatalf("active=%+v %v", view, err)
	}

	if _, err = db.ExecContext(
		ctx,
		`INSERT INTO elicitation_requests(elicitation_request_id,connection_attempt_id,process_generation,mode,message,state,requested_at,session_id,scope_json,requested_schema_json) VALUES('question','attempt',1,'form','Continue?','pending','2026-09-26T00:00:00Z',?,'{}','{"type":"object"}')`,
		view.ActiveRevision.SessionID,
	); err != nil {
		t.Fatal(err)
	}

	view, err = s.GetProcedure(ctx, application.ProcedureViewQuery{ProjectID: "p"})
	if err != nil || len(view.Elicitations) != 1 {
		t.Fatalf("elicitation=%+v %v", view, err)
	}

	replay, err := s.RequestProcedureRevision(ctx, in)
	if err != nil || !reflect.DeepEqual(replay, accepted) {
		t.Fatalf("replay=%+v %v", replay, err)
	}

	in.Content[0].Text = "Different"
	_, err = s.RequestProcedureRevision(ctx, in)
	procedureCode(t, err, "operation_id_conflict")

	in.OperationID = "second"
	_, err = s.RequestProcedureRevision(ctx, in)
	procedureCode(t, err, "invalid_state")

	in.Content = nil
	_, err = s.RequestProcedureRevision(ctx, in)
	procedureCode(t, err, "validation_failed")
}
