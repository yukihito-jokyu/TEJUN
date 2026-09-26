package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yukihito-jokyu/TEJUN/internal/application"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/project"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/shared"
)

func projectTestDB(t *testing.T) *ProjectRepository {
	t.Helper()

	db, err := Open(context.Background(), t.TempDir()+"/project.db")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec(
		`INSERT INTO agent_connections(connection_id,display_name,command,args_json,transport,resolved_executable_path,last_verified_at,protocol_version,auth_state,schema_artifact_version,revision)
VALUES('connection','Agent','agent','[]','stdio','/bin/agent','2026-09-26T00:00:00Z','1','not_required','1',1)`,
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(`UPDATE app_settings SET default_connection_id='connection' WHERE singleton=1`)
	if err != nil {
		t.Fatal(err)
	}

	return NewProjectRepository(db)
}

func projectRecord(id, operation string) application.ProjectCreateRecord {
	now := time.Date(2026, 9, 26, 1, 2, 3, 0, time.UTC)

	return application.ProjectCreateRecord{
		CreateProjectInput: application.CreateProjectInput{
			Name:          "手順",
			Description:   "説明",
			WorkspacePath: "/tmp",
			OperationID:   operation,
		},
		ProjectID:   id,
		RequestHash: "hash:" + operation,
		Receipt:     application.MutationReceipt{OperationID: operation, CommittedAt: now},
		Event: application.OutboxEvent{
			ID:            "event:" + operation,
			Name:          "project.changed",
			EmittedAt:     now,
			AggregateType: "project",
			AggregateID:   id,
			Correlation:   operation,
		},
	}
}

func TestDeleteProjectKeepsDescendantAndSharedEvidence(t *testing.T) {
	ctx := context.Background()
	repo := projectTestDB(t)
	root := t.TempDir()

	store, err := NewEvidenceStore(repo.db, root)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = store.Close() })
	repo.SetEvidenceStore(store)

	for _, id := range []string{"parent", "child"} {
		if _, err := repo.CreateProject(ctx, projectRecord(id, "create:"+id)); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := repo.db.Exec(
		`UPDATE projects SET source_project_id='parent',source_procedure_id='procedure' WHERE project_id='child'`,
	); err != nil {
		t.Fatal(err)
	}

	hash := strings.Repeat("a", 64)

	path := filepath.Join("blobs", "sha256", hash[:2], hash)
	if err := store.mkdirAll(filepath.Dir(path)); err != nil {
		t.Fatal(err)
	}

	f, err := store.openFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := f.Write([]byte("evidence")); err != nil {
		t.Fatal(err)
	}

	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	fixtures := []struct {
		query string
		args  []any
	}{
		{
			`INSERT INTO evidence_blobs(hash,size,mime,relative_path,status,ref_count) VALUES(?,8,'image/png',?,'available',2)`,
			[]any{hash, path},
		},
		{
			`INSERT INTO executions(execution_id,project_id,revision) VALUES('parent-exec','parent',1),('child-exec','child',1)`,
			nil,
		},
		{
			`INSERT INTO evidence_records(evidence_id,execution_id,blob_hash,actor,created_at_us) VALUES('parent-evidence','parent-exec',?,'human',1),('child-evidence','child-exec',?,'human',1)`,
			[]any{hash, hash},
		},
	}
	for _, fixture := range fixtures {
		if _, err := repo.db.Exec(fixture.query, fixture.args...); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := repo.db.Exec(
		`INSERT INTO background_jobs(job_id,project_id,kind,state,target_id,accepted_at) VALUES('active','parent','reconnect','running','parent',1)`,
	); err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		name, id string
		revision int64
		wantCode string
	}{
		{"revision conflict", "parent", 2, "revision_conflict"},
		{"active job", "parent", 1, "active_job"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := repo.DeleteProject(ctx, application.ProjectDeleteRecord{
				DeleteProjectInput: application.DeleteProjectInput{
					ProjectID:        tt.id,
					ExpectedRevision: tt.revision,
					OperationID:      tt.name,
				},
				RequestHash: tt.name,
			})

			var appErr *shared.Error
			if !errors.As(err, &appErr) || appErr.Code != tt.wantCode {
				t.Fatalf("error=%v, want %s", err, tt.wantCode)
			}
		})
	}

	if _, err := repo.db.Exec(`DELETE FROM background_jobs WHERE job_id='active'`); err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		name, id, operation string
		wantFile            bool
	}{
		{"parent removal preserves shared blob", "parent", "delete:parent", true},
		{"last reference removes blob", "child", "delete:child", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result, err := repo.DeleteProject(ctx, application.ProjectDeleteRecord{
				DeleteProjectInput: application.DeleteProjectInput{
					ProjectID:        tt.id,
					ExpectedRevision: 1,
					OperationID:      tt.operation,
				},
				RequestHash: tt.operation,
				Receipt:     application.MutationReceipt{OperationID: tt.operation, CommittedAt: time.Now().UTC()},
				Event: application.OutboxEvent{
					ID:            "event:" + tt.operation,
					Name:          "project.deleted",
					EmittedAt:     time.Now().UTC(),
					AggregateType: "project",
					AggregateID:   tt.id,
					Correlation:   tt.operation,
				},
			})
			if err != nil {
				t.Fatal(err)
			}

			if result.Data.ProjectID != tt.id {
				t.Fatalf("result=%+v", result)
			}

			file, err := store.openFile(path, os.O_RDONLY, 0)
			if err == nil {
				_ = file.Close()
			}

			if (err == nil) != tt.wantFile {
				t.Fatalf("blob exists=%t, want %t; err=%v", err == nil, tt.wantFile, err)
			}

			var remaining int
			if err := repo.db.QueryRow(`SELECT COUNT(*) FROM projects WHERE project_id=?`, tt.id).
				Scan(&remaining); err != nil {
				t.Fatal(err)
			}

			if remaining != 0 {
				t.Fatalf("project remains: %d", remaining)
			}

			if tt.id == "parent" {
				var source, procedure sql.NullString
				if err := repo.db.QueryRow(`SELECT source_project_id,source_procedure_id FROM projects WHERE project_id='child'`).
					Scan(&source, &procedure); err != nil {
					t.Fatal(err)
				}

				if source.Valid || procedure.Valid {
					t.Fatalf("child still references deleted parent: %v %v", source, procedure)
				}
			}
		})
	}

	f, err = store.openFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}

	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := repo.db.Exec(
		`INSERT INTO evidence_blob_deletions(hash,relative_path) VALUES(?,?)`,
		hash,
		path,
	); err != nil {
		t.Fatal(err)
	}

	if err := store.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}

	if f, err := store.openFile(path, os.O_RDONLY, 0); err == nil {
		_ = f.Close()

		t.Fatal("reconcile left deleted blob")
	}
}

func TestProjectCreateDedupAndAtomicity(t *testing.T) {
	repo := projectTestDB(t)
	ctx := context.Background()
	first := projectRecord("project-1", "operation-1")
	changed := first
	changed.RequestHash = "different"

	tests := []struct {
		name     string
		record   application.ProjectCreateRecord
		wantCode string
	}{
		{"create", first, ""}, {"same operation", first, ""}, {"different payload", changed, "operation_id_conflict"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := repo.CreateProject(ctx, tt.record)
			if tt.wantCode != "" {
				var appErr *shared.Error
				if !errors.As(err, &appErr) || appErr.Code != tt.wantCode {
					t.Fatalf("error=%v", err)
				}

				return
			}

			if err != nil {
				t.Fatal(err)
			}

			if result.Data.ProjectID != "project-1" || result.Data.NextRoute != "#/projects/project-1/prepare" {
				t.Fatalf("result=%+v", result)
			}
		})
	}

	var projects, preparations, receipts, events int
	if err := repo.db.QueryRow(`SELECT (SELECT COUNT(*) FROM projects),(SELECT COUNT(*) FROM preparations),(SELECT COUNT(*) FROM operation_receipts),(SELECT COUNT(*) FROM event_outbox)`).
		Scan(&projects, &preparations, &receipts, &events); err != nil {
		t.Fatal(err)
	}

	if projects != 1 || preparations != 1 || receipts != 1 || events != 1 {
		t.Fatalf("counts=%d/%d/%d/%d", projects, preparations, receipts, events)
	}
}

func TestProjectOutboxCarriesTypedPayload(t *testing.T) {
	repo := projectTestDB(t)

	ctx := context.Background()
	if _, err := repo.CreateProject(ctx, projectRecord("project", "create")); err != nil {
		t.Fatal(err)
	}

	events, err := NewAgentConnectionRepository(repo.db).PendingEvents(ctx)
	if err != nil || len(events) != 1 {
		t.Fatalf("events = %+v, error = %v", events, err)
	}

	if events[0].OperationID != "create" {
		t.Fatalf("event operation = %q", events[0].OperationID)
	}

	var (
		payload struct {
			ProjectID    string `json:"projectId"`
			Status       string `json:"status"`
			CurrentStage string `json:"currentStage"`
		}
		correlation map[string]string
	)

	if err := json.Unmarshal([]byte(events[0].Payload), &payload); err != nil {
		t.Fatal(err)
	}

	if err := json.Unmarshal([]byte(events[0].Correlation), &correlation); err != nil {
		t.Fatal(err)
	}

	if payload.ProjectID != "project" || payload.Status != "preparing" || payload.CurrentStage != "preparation" ||
		correlation["projectId"] != "project" ||
		correlation["jobId"] != "" {
		t.Fatalf("payload = %+v, correlation = %+v", payload, correlation)
	}
}

func TestProjectCreateWithoutDefaultConnection(t *testing.T) {
	repo := projectTestDB(t)
	if _, err := repo.db.Exec(`UPDATE app_settings SET default_connection_id=NULL WHERE singleton=1`); err != nil {
		t.Fatal(err)
	}

	_, err := repo.CreateProject(context.Background(), projectRecord("project", "missing-connection"))

	var appErr *shared.Error
	if !errors.As(err, &appErr) || appErr.Code != "connection_required" {
		t.Fatalf("error=%v", err)
	}
}

func TestProjectCopyRevisionAndArchive(t *testing.T) {
	repo := projectTestDB(t)

	ctx := context.Background()
	if _, err := repo.CreateProject(ctx, projectRecord("source", "create")); err != nil {
		t.Fatal(err)
	}

	for _, statement := range []string{
		`UPDATE preparations SET purpose='目的',completion_criteria_json='["完了"]' WHERE project_id='source'`,
		`INSERT INTO check_plans(project_id,revision) VALUES('source',1)`,
		`INSERT INTO check_items(check_id,project_id,sequence,title,instruction,expected_result,ai_required,human_required,human_evidence_requirement) VALUES('check','source',1,'確認','実行','成功',1,1,'text')`,
		`UPDATE projects SET status='completed',current_stage='completed',completed_at=1790384523000000 WHERE project_id='source'`,
		`INSERT INTO procedures(procedure_id,project_id,revision,status,document_json,created_at,completed_at) VALUES('procedure','source',1,'completed','{}',1790384523000000,1790384523000000)`,
	} {
		if _, err := repo.db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}

	now := time.Date(2026, 9, 26, 2, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		revision bool
		record   application.ProjectCopyRecord
		wantCode string
	}{
		{
			"duplicate",
			false,
			application.ProjectCopyRecord{
				SourceProjectID: "source",
				SourceRevision:  1,
				ProjectID:       "duplicate",
				Name:            "複製",
				WorkspacePath:   "/tmp",
				OperationID:     "dup",
				RequestHash:     "dup",
				Receipt:         application.MutationReceipt{OperationID: "dup", CommittedAt: now},
				Event: application.OutboxEvent{
					ID:            "ev-dup",
					Name:          "project.changed",
					EmittedAt:     now,
					AggregateType: "project",
					AggregateID:   "duplicate",
				},
			},
			"",
		},
		{
			"stale duplicate",
			false,
			application.ProjectCopyRecord{
				SourceProjectID: "source",
				SourceRevision:  2,
				ProjectID:       "stale",
				Name:            "複製",
				WorkspacePath:   "/tmp",
				OperationID:     "stale",
				RequestHash:     "stale",
			},
			"revision_conflict",
		},
		{
			"revision",
			true,
			application.ProjectCopyRecord{
				SourceProjectID:         "source",
				SourceProcedureID:       "procedure",
				SourceProcedureRevision: 1,
				ProjectID:               "revised",
				Name:                    "改訂",
				WorkspacePath:           "/tmp",
				OperationID:             "rev",
				RequestHash:             "rev",
				Receipt:                 application.MutationReceipt{OperationID: "rev", CommittedAt: now},
				Event: application.OutboxEvent{
					ID:            "ev-rev",
					Name:          "project.changed",
					EmittedAt:     now,
					AggregateType: "project",
					AggregateID:   "revised",
				},
			},
			"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				result application.MutationResult[application.CreatedProject]
				err    error
			)
			if tt.revision {
				result, err = repo.CreateRevision(ctx, tt.record)
			} else {
				result, err = repo.DuplicateProject(ctx, tt.record)
			}

			if tt.wantCode != "" {
				var appErr *shared.Error
				if !errors.As(err, &appErr) || appErr.Code != tt.wantCode {
					t.Fatalf("error=%v", err)
				}

				return
			}

			if err != nil {
				t.Fatal(err)
			}

			if result.Data.Status != project.Preparing {
				t.Fatalf("result=%+v", result)
			}

			var (
				purpose    string
				checkCount int
			)

			if err := repo.db.QueryRow(`SELECT purpose FROM preparations WHERE project_id=?`, tt.record.ProjectID).
				Scan(&purpose); err != nil {
				t.Fatal(err)
			}

			if err := repo.db.QueryRow(`SELECT COUNT(*) FROM check_items WHERE project_id=?`, tt.record.ProjectID).
				Scan(&checkCount); err != nil {
				t.Fatal(err)
			}

			if purpose != "目的" || checkCount != 1 {
				t.Fatalf("purpose=%s checks=%d", purpose, checkCount)
			}
		})
	}

	var sourceStatus string
	if err := repo.db.QueryRow(`SELECT status FROM projects WHERE project_id='source'`).
		Scan(&sourceStatus); err != nil {
		t.Fatal(err)
	}

	if sourceStatus != "completed" {
		t.Fatalf("source=%s", sourceStatus)
	}

	archive := application.ProjectArchiveRecord{
		ArchiveProjectInput: application.ArchiveProjectInput{
			ProjectID:        "duplicate",
			ExpectedRevision: 1,
			OperationID:      "archive",
		},
		RequestHash: "archive",
		Receipt:     application.MutationReceipt{OperationID: "archive", CommittedAt: now},
		Event: application.OutboxEvent{
			ID:            "ev-archive",
			Name:          "project.changed",
			EmittedAt:     now,
			AggregateType: "project",
			AggregateID:   "duplicate",
		},
	}

	if _, err := repo.db.Exec(
		`INSERT INTO background_jobs(job_id,project_id,kind,state,target_id,accepted_at) VALUES('job','duplicate','reconnect','pending','session',1790388000000000)`,
	); err != nil {
		t.Fatal(err)
	}

	if _, err := repo.ArchiveProject(ctx, archive); err == nil {
		t.Fatal("active job was archived")
	}

	if _, err := repo.db.Exec(`UPDATE background_jobs SET state='succeeded' WHERE job_id='job'`); err != nil {
		t.Fatal(err)
	}

	result, err := repo.ArchiveProject(ctx, archive)
	if err != nil {
		t.Fatal(err)
	}

	if result.Data.Revision != 2 {
		t.Fatalf("revision=%d", result.Data.Revision)
	}
}

func TestProjectListEscapedSearchAndCursor(t *testing.T) {
	repo := projectTestDB(t)
	ctx := context.Background()

	for i, name := range []string{"100%", "100_", "plain"} {
		record := projectRecord(name, "op-"+name)
		record.Name = name
		record.ProjectID = string(rune('a' + i))

		record.Event.AggregateID = record.ProjectID
		if _, err := repo.CreateProject(ctx, record); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		name  string
		query application.ProjectListQuery
		want  int
		next  bool
	}{
		{"percent literal", application.ProjectListQuery{Search: "%", Sort: "name_asc", Limit: 10}, 1, false},
		{"underscore literal", application.ProjectListQuery{Search: "_", Sort: "name_asc", Limit: 10}, 1, false},
		{"page", application.ProjectListQuery{Sort: "name_asc", Limit: 2}, 3, true},
		{"updated page", application.ProjectListQuery{Sort: "updated_desc", Limit: 2}, 3, true},
		{"attention page", application.ProjectListQuery{Sort: "attention_desc", Limit: 2}, 3, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := repo.ListProjects(ctx, tt.query)
			if err != nil {
				t.Fatal(err)
			}

			if result.Total != tt.want || (result.NextCursor != nil) != tt.next {
				t.Fatalf("result=%+v", result)
			}

			if tt.next {
				tt.query.Cursor = *result.NextCursor

				second, err := repo.ListProjects(ctx, tt.query)
				if err != nil {
					t.Fatal(err)
				}

				if len(second.Items) != 1 || second.Items[0].ProjectID == result.Items[0].ProjectID {
					t.Fatalf("second=%+v", second)
				}
			}
		})
	}
}

func TestProjectCursorRejectsChangedSequence(t *testing.T) {
	tests := []struct {
		name string
		sort string
	}{
		{"name", "name_asc"},
		{"updated", "updated_desc"},
		{"attention", "attention_desc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := projectTestDB(t)

			ctx := context.Background()
			for _, id := range []string{"a", "b"} {
				if _, err := repo.CreateProject(ctx, projectRecord(id, "create-"+id)); err != nil {
					t.Fatal(err)
				}
			}

			query := application.ProjectListQuery{Sort: tt.sort, Limit: 1}

			first, err := repo.ListProjects(ctx, query)
			if err != nil || first.NextCursor == nil {
				t.Fatalf("first=%+v error=%v", first, err)
			}

			query.Cursor = *first.NextCursor

			if _, err := repo.CreateProject(ctx, projectRecord("c", "create-c")); err != nil {
				t.Fatal(err)
			}

			_, err = repo.ListProjects(ctx, query)

			var appErr *shared.Error
			if !errors.As(err, &appErr) || appErr.Code != "revision_conflict" {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestProjectTimestampsAndHashVersion(t *testing.T) {
	repo := projectTestDB(t)

	record := projectRecord("project", "create")
	if _, err := repo.CreateProject(context.Background(), record); err != nil {
		t.Fatal(err)
	}

	var (
		createdType, updatedType string
		created                  int64
		version                  int
	)

	if err := repo.db.QueryRow(`SELECT typeof(created_at),typeof(updated_at),created_at FROM projects WHERE project_id='project'`).
		Scan(&createdType, &updatedType, &created); err != nil {
		t.Fatal(err)
	}

	if err := repo.db.QueryRow(`SELECT request_hash_version FROM operation_receipts WHERE scope='create_project'`).
		Scan(&version); err != nil {
		t.Fatal(err)
	}

	if createdType != "integer" || updatedType != "integer" || created != record.Receipt.CommittedAt.UnixMicro() ||
		version != 1 {
		t.Fatalf("created=%s updated=%s timestamp=%d version=%d", createdType, updatedType, created, version)
	}
}
