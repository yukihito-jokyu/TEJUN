package wails

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	appexport "github.com/yukihito-jokyu/TEJUN/internal/adapter/export"
	appsqlite "github.com/yukihito-jokyu/TEJUN/internal/adapter/sqlite"
	"github.com/yukihito-jokyu/TEJUN/internal/adapter/workspace"
	"github.com/yukihito-jokyu/TEJUN/internal/application"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/shared"
	"github.com/yukihito-jokyu/TEJUN/internal/trace"
)

func TestProjectBindingRejectsInvalidInput(t *testing.T) {
	service := NewProjectService(
		application.NewProjectUseCases(nil, nil, time.Now, func() string { return "id" }),
		application.NewProjectExternal(nil, nil, time.Now, func() string { return "id" }),
	)
	ctx := context.Background()

	tests := []struct {
		name string
		call func() error
	}{
		{"list", func() error { _, err := service.ListProjects(ctx, ProjectListQuery{Limit: 101}); return err }},
		{"create", func() error { _, err := service.CreateProject(ctx, CreateProjectInput{}); return err }},
		{"duplicate", func() error { _, err := service.DuplicateProject(ctx, DuplicateProjectInput{}); return err }},
		{"revision", func() error { _, err := service.CreateRevision(ctx, CreateRevisionInput{}); return err }},
		{"archive", func() error { _, err := service.ArchiveProject(ctx, ArchiveProjectInput{}); return err }},
		{"delete", func() error { _, err := service.DeleteProject(ctx, DeleteProjectInput{}); return err }},
		{
			"reconnect",
			func() error { _, err := service.ReconnectProjectSession(ctx, ReconnectSessionInput{}); return err },
		},
		{"export", func() error { _, err := service.ExportProcedure(ctx, ExportProcedureInput{}); return err }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); err == nil {
				t.Fatal("invalid input accepted")
			}
		})
	}
}

func TestProjectBindingsUseCommittedData(t *testing.T) {
	ctx := context.Background()
	directory := t.TempDir()

	db, err := appsqlite.Open(ctx, filepath.Join(directory, "app.db"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(
		`INSERT INTO agent_connections(connection_id,display_name,command,args_json,transport,resolved_executable_path,last_verified_at,protocol_version,auth_state,schema_artifact_version,revision) VALUES('connection','Agent','agent','[]','stdio','/bin/agent','2026-09-26T00:00:00Z','1','not_required','1',1)`,
	); err != nil {
		t.Fatal(err)
	}

	if _, err := db.Exec(`UPDATE app_settings SET default_connection_id='connection' WHERE singleton=1`); err != nil {
		t.Fatal(err)
	}

	var next atomic.Int64

	newID := func() string { return fmt.Sprintf("id-%d", next.Add(1)) }
	now := func() time.Time { return time.Date(2026, 9, 26, 1, 2, 3, 0, time.UTC) }
	exporter := appexport.NewExporter()

	t.Cleanup(func() { _ = exporter.Close() })

	service := NewProjectService(
		application.NewProjectUseCases(appsqlite.NewProjectRepository(db), workspace.Validator{}, now, newID),
		application.NewProjectExternal(appsqlite.NewProjectExternalRepository(db), exporter, now, newID),
	)

	traceWriter, err := trace.Open(directory)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = traceWriter.Close() })
	service.SetTrace(traceWriter)

	created, err := service.CreateProject(
		ctx,
		CreateProjectInput{Name: "手順", WorkspacePath: directory, OperationID: "create"},
	)
	if err != nil || created.Data.ProjectID == "" || created.Receipt.ChangeSequence < 1 {
		t.Fatalf("create = %+v, error = %v", created, err)
	}

	replayed, err := service.CreateProject(
		ctx,
		CreateProjectInput{Name: "手順", WorkspacePath: directory, OperationID: "create"},
	)
	if err != nil || replayed != created {
		t.Fatalf("replay = %+v, error = %v", replayed, err)
	}

	list, err := service.ListProjects(ctx, ProjectListQuery{})
	if err != nil || len(list.Items) != 1 || list.Items[0].ProjectID != created.Data.ProjectID {
		t.Fatalf("list = %+v, error = %v", list, err)
	}

	duplicate, err := service.DuplicateProject(
		ctx,
		DuplicateProjectInput{
			SourceProjectID: created.Data.ProjectID,
			SourceRevision:  1,
			Name:            "複製",
			WorkspacePath:   directory,
			OperationID:     "duplicate",
		},
	)
	if err != nil || duplicate.Data.ProjectID == created.Data.ProjectID ||
		duplicate.Receipt.ChangeSequence <= created.Receipt.ChangeSequence {
		t.Fatalf("duplicate = %+v, error = %v", duplicate, err)
	}

	archived, err := service.ArchiveProject(
		ctx,
		ArchiveProjectInput{ProjectID: duplicate.Data.ProjectID, ExpectedRevision: 1, OperationID: "archive"},
	)
	if err != nil || archived.Data.Revision != 2 ||
		archived.Receipt.ChangeSequence <= duplicate.Receipt.ChangeSequence {
		t.Fatalf("archive = %+v, error = %v", archived, err)
	}

	reconnected, err := service.ReconnectProjectSession(
		ctx,
		ReconnectSessionInput{
			ProjectID:        created.Data.ProjectID,
			ExpectedRevision: 1,
			Strategy:         "auto",
			OperationID:      "reconnect",
		},
	)
	if err != nil || reconnected.Data.State != "connecting" ||
		reconnected.Receipt.ChangeSequence <= archived.Receipt.ChangeSequence {
		t.Fatalf("reconnect = %+v, error = %v", reconnected, err)
	}

	if _, err := db.Exec(
		`UPDATE background_jobs SET state='succeeded' WHERE job_id=?`,
		reconnected.Data.JobID,
	); err != nil {
		t.Fatal(err)
	}

	if _, err := db.Exec(
		`UPDATE projects SET status='completed',current_stage='completed' WHERE project_id=?`,
		created.Data.ProjectID,
	); err != nil {
		t.Fatal(err)
	}

	if _, err := db.Exec(
		`INSERT INTO procedures(procedure_id,project_id,revision,status,document_json,created_at) VALUES('procedure',?,1,'completed','{}',?)`,
		created.Data.ProjectID,
		now().UnixMicro(),
	); err != nil {
		t.Fatal(err)
	}

	withProcedure, err := service.ListProjects(ctx, ProjectListQuery{Sort: "updated_desc", Limit: 50})
	if err != nil || len(withProcedure.Items) == 0 {
		t.Fatalf("list with procedure = %+v, error = %v", withProcedure, err)
	}

	if withProcedure.Items[0].CurrentProcedureID == nil || *withProcedure.Items[0].CurrentProcedureID != "procedure" ||
		withProcedure.Items[0].CurrentProcedureRevision == nil || *withProcedure.Items[0].CurrentProcedureRevision != 1 {
		t.Fatalf("procedure snapshot = %+v", withProcedure.Items[0])
	}

	revision, err := service.CreateRevision(
		ctx,
		CreateRevisionInput{
			SourceProjectID:         created.Data.ProjectID,
			SourceProcedureID:       "procedure",
			SourceProcedureRevision: 1,
			Name:                    "改訂",
			WorkspacePath:           directory,
			OperationID:             "revision",
		},
	)
	if err != nil || revision.Data.ProjectID == created.Data.ProjectID {
		t.Fatalf("revision = %+v, error = %v", revision, err)
	}

	prepared, err := service.PrepareExportProcedure(ctx, PrepareExportProcedureInput{
		ProcedureID: "procedure", ProcedureRevision: 1, AbsolutePath: filepath.Join(directory, "procedure.md"),
	})
	if err != nil || prepared.Destination.VerifiedRootID == "" || prepared.OverwriteRequired {
		t.Fatalf("prepare = %+v, error = %v", prepared, err)
	}

	listed, err := service.ListProjects(ctx, ProjectListQuery{Sort: "updated_desc", Limit: 50})
	if err != nil || listed.ChangeSequence != revision.Receipt.ChangeSequence {
		t.Fatalf("prepare changed snapshot: %+v, error = %v", listed, err)
	}

	exported, err := service.ExportProcedure(
		ctx,
		ExportProcedureInput{
			ProcedureID:       "procedure",
			ProcedureRevision: 1,
			Format:            "markdown",
			Destination:       prepared.Destination,
			OperationID:       "export",
		},
	)
	if err != nil || exported.Data.ExportID == "" ||
		exported.Receipt.ChangeSequence <= revision.Receipt.ChangeSequence {
		t.Fatalf("export = %+v, error = %v", exported, err)
	}

	replayTests := []struct {
		name string
		call func() (int64, error)
		want int64
	}{
		{"duplicate", func() (int64, error) {
			value, err := service.DuplicateProject(
				ctx,
				DuplicateProjectInput{
					SourceProjectID: created.Data.ProjectID,
					SourceRevision:  1,
					Name:            "複製",
					WorkspacePath:   directory,
					OperationID:     "duplicate",
				},
			)

			return value.Receipt.ChangeSequence, err
		}, duplicate.Receipt.ChangeSequence},
		{"revision", func() (int64, error) {
			value, err := service.CreateRevision(
				ctx,
				CreateRevisionInput{
					SourceProjectID:         created.Data.ProjectID,
					SourceProcedureID:       "procedure",
					SourceProcedureRevision: 1,
					Name:                    "改訂",
					WorkspacePath:           directory,
					OperationID:             "revision",
				},
			)

			return value.Receipt.ChangeSequence, err
		}, revision.Receipt.ChangeSequence},
		{"archive", func() (int64, error) {
			value, err := service.ArchiveProject(
				ctx,
				ArchiveProjectInput{ProjectID: duplicate.Data.ProjectID, ExpectedRevision: 1, OperationID: "archive"},
			)

			return value.Receipt.ChangeSequence, err
		}, archived.Receipt.ChangeSequence},
		{"reconnect", func() (int64, error) {
			value, err := service.ReconnectProjectSession(
				ctx,
				ReconnectSessionInput{
					ProjectID:        created.Data.ProjectID,
					ExpectedRevision: 1,
					Strategy:         "auto",
					OperationID:      "reconnect",
				},
			)

			return value.Receipt.ChangeSequence, err
		}, reconnected.Receipt.ChangeSequence},
		{"export", func() (int64, error) {
			value, err := service.ExportProcedure(
				ctx,
				ExportProcedureInput{
					ProcedureID:       "procedure",
					ProcedureRevision: 1,
					Format:            "markdown",
					Destination:       prepared.Destination,
					OperationID:       "export",
				},
			)

			return value.Receipt.ChangeSequence, err
		}, exported.Receipt.ChangeSequence},
	}
	for _, test := range replayTests {
		t.Run("replay "+test.name, func(t *testing.T) {
			got, err := test.call()
			if err != nil || got != test.want {
				t.Fatalf("change sequence = %d, error = %v, want = %d", got, err, test.want)
			}
		})
	}

	tests := []struct {
		name string
		call func() error
		code string
	}{
		{"create conflict", func() error {
			_, err := service.CreateProject(
				ctx,
				CreateProjectInput{Name: "別名", WorkspacePath: directory, OperationID: "create"},
			)

			return err
		}, "operation_id_conflict"},
		{"duplicate stale", func() error {
			_, err := service.DuplicateProject(
				ctx,
				DuplicateProjectInput{
					SourceProjectID: created.Data.ProjectID,
					SourceRevision:  2,
					Name:            "複製",
					WorkspacePath:   directory,
					OperationID:     "duplicate-stale",
				},
			)

			return err
		}, "revision_conflict"},
		{"archive stale", func() error {
			_, err := service.ArchiveProject(
				ctx,
				ArchiveProjectInput{
					ProjectID:        duplicate.Data.ProjectID,
					ExpectedRevision: 1,
					OperationID:      "archive-stale",
				},
			)

			return err
		}, "revision_conflict"},
		{"reconnect conflict", func() error {
			_, err := service.ReconnectProjectSession(
				ctx,
				ReconnectSessionInput{
					ProjectID:        created.Data.ProjectID,
					ExpectedRevision: 2,
					Strategy:         "auto",
					OperationID:      "reconnect",
				},
			)

			return err
		}, "operation_id_conflict"},
		{"export conflict", func() error {
			_, err := service.ExportProcedure(
				ctx,
				ExportProcedureInput{
					ProcedureID:       "procedure",
					ProcedureRevision: 1,
					Format:            "pdf",
					Destination: VerifiedPathSelection{
						AbsolutePath:   filepath.Join(directory, "procedure.pdf"),
						VerifiedRootID: prepared.Destination.VerifiedRootID,
					},
					OperationID: "export",
				},
			)

			return err
		}, "operation_id_conflict"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var appErr *shared.Error
			if err := test.call(); !errors.As(err, &appErr) || appErr.Code != test.code {
				t.Fatalf("error = %v, want %s", err, test.code)
			}
		})
	}

	deleted, err := service.DeleteProject(ctx, DeleteProjectInput{
		ProjectID: duplicate.Data.ProjectID, ExpectedRevision: 2, OperationID: "delete",
	})
	if err != nil || deleted.Data.ProjectID != duplicate.Data.ProjectID ||
		deleted.Receipt.ChangeSequence <= archived.Receipt.ChangeSequence {
		t.Fatalf("delete = %+v, error = %v", deleted, err)
	}

	entries, err := os.ReadFile(filepath.Join(directory, "trace.jsonl"))
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(string(entries), directory) {
		t.Fatal("trace contains workspace or export path")
	}

	var createCommits, createResponses, conflictCommits int

	for _, line := range strings.Split(strings.TrimSpace(string(entries)), "\n") {
		var entry struct {
			Phase       string `json:"phase"`
			OperationID string `json:"operationId"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatal(err)
		}

		if entry.OperationID == "create" && entry.Phase == "accept_commit" {
			createCommits++
		}

		if entry.OperationID == "create" && entry.Phase == "accepted_response" {
			createResponses++
		}

		if entry.OperationID == "duplicate-stale" && entry.Phase == "accept_commit" {
			conflictCommits++
		}
	}

	if createCommits != 1 || createResponses != 2 || conflictCommits != 0 {
		t.Fatalf("trace commits/responses/conflicts = %d/%d/%d", createCommits, createResponses, conflictCommits)
	}
}

func TestProjectReceiptIncludesChangeSequence(t *testing.T) {
	result := createdProjectResult(application.MutationResult[application.CreatedProject]{
		Data: application.CreatedProject{ProjectID: "project", CreatedAt: time.Unix(0, 0)},
		Receipt: application.MutationReceipt{
			OperationID:    "operation",
			ChangeSequence: 42,
			CommittedAt:    time.Unix(0, 0),
		},
	})
	if result.Receipt.ChangeSequence != 42 || result.Receipt.OperationID != "operation" {
		t.Fatalf("receipt = %+v", result.Receipt)
	}
}
