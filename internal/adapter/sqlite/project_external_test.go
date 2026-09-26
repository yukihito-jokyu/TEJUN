package sqlite

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	appexport "github.com/yukihito-jokyu/TEJUN/internal/adapter/export"
	"github.com/yukihito-jokyu/TEJUN/internal/application"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/shared"
	"github.com/yukihito-jokyu/TEJUN/internal/trace"
)

type failingProjectConnector struct{}

func (failingProjectConnector) ConnectProject(
	context.Context, application.ClaimedProjectJob,
) (application.ProjectSessionConnected, error) {
	return application.ProjectSessionConnected{}, &shared.Error{Code: "acp_timeout", Message: "secret payload"}
}

func TestProjectWorkerTraceFailure(t *testing.T) {
	project := projectTestDB(t)

	ctx := context.Background()
	if _, err := project.CreateProject(ctx, projectRecord("project", "create")); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 9, 26, 1, 2, 3, 0, time.UTC)
	repository := NewProjectExternalRepository(project.db)

	_, err := repository.AcceptReconnect(ctx, application.ProjectReconnectRecord{
		ReconnectSessionInput: application.ReconnectSessionInput{
			ProjectID: "project", ExpectedRevision: 1, Strategy: "auto", OperationID: "reconnect",
		},
		SessionID: "session", JobID: "job", RequestHash: "same",
		Receipt: application.MutationReceipt{OperationID: "reconnect", CommittedAt: now},
		Event: application.OutboxEvent{
			ID: "event", Name: "session.connection_changed",
			AggregateType: "session", AggregateID: "session", EmittedAt: now,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()

	writer, err := trace.Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = writer.Close() })

	ctx = trace.WithWriter(ctx, writer)

	ran, err := application.RunProjectJob(
		ctx, repository, failingProjectConnector{}, nil, func() time.Time { return now },
	)
	if !ran || err == nil {
		t.Fatalf("worker = (%v, %v)", ran, err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "trace.jsonl"))
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(string(data), "secret payload") {
		t.Fatal("trace contains raw error")
	}

	want := []string{"worker_claim", "external_io_start", "external_io_end", "complete_commit"}

	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var entry struct {
			Phase       string `json:"phase"`
			OperationID string `json:"operationId"`
			JobID       string `json:"jobId"`
			Status      string `json:"status"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatal(err)
		}

		if len(want) == 0 || entry.Phase != want[0] || entry.OperationID != "reconnect" || entry.JobID != "job" {
			t.Fatalf("unexpected trace entry %+v, remaining phases %v", entry, want)
		}

		if entry.Phase == "complete_commit" && entry.Status != "failed" {
			t.Fatalf("completion trace = %+v", entry)
		}

		want = want[1:]
	}

	if len(want) != 0 {
		t.Fatalf("missing phases %v", want)
	}
}

func TestProjectExternalReconnectLifecycle(t *testing.T) {
	project := projectTestDB(t)

	ctx := context.Background()
	if _, err := project.CreateProject(ctx, projectRecord("project", "create")); err != nil {
		t.Fatal(err)
	}

	repository := NewProjectExternalRepository(project.db)
	now := time.Date(2026, 9, 26, 1, 2, 3, 0, time.UTC)
	record := application.ProjectReconnectRecord{
		ReconnectSessionInput: application.ReconnectSessionInput{
			ProjectID:        "project",
			ExpectedRevision: 1,
			Strategy:         "auto",
			OperationID:      "reconnect",
		},
		SessionID:   "session",
		JobID:       "job",
		RequestHash: "same",
		Receipt:     application.MutationReceipt{OperationID: "reconnect", CommittedAt: now},
		Event: application.OutboxEvent{
			ID:            "event-reconnect",
			Name:          "session.connection_changed",
			AggregateType: "session",
			AggregateID:   "session",
			EmittedAt:     now,
		},
	}

	tests := []struct{ name, hash, wantCode string }{
		{name: "accepted", hash: "same"},
		{name: "replayed", hash: "same"},
		{name: "different payload", hash: "changed", wantCode: "operation_id_conflict"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := record
			request.RequestHash = test.hash
			accepted, err := repository.AcceptReconnect(ctx, request)

			if test.wantCode != "" {
				var appErr *shared.Error
				if !errors.As(err, &appErr) || appErr.Code != test.wantCode {
					t.Fatalf("error = %v", err)
				}

				return
			}

			if err != nil || accepted.Data.RecoveryMode != "new" {
				t.Fatalf("accepted = %+v, error = %v", accepted, err)
			}
		})
	}

	job, err := repository.ClaimProjectJob(ctx)
	if err != nil || job == nil || job.SessionID != "session" || job.Connection.Command != "agent" {
		t.Fatalf("job = %+v, error = %v", job, err)
	}

	if err := repository.CompleteProjectJob(
		ctx,
		application.ProjectJobCompletion{
			JobID:       "job",
			Kind:        "reconnect",
			TargetID:    "session",
			Success:     false,
			ErrorCode:   "acp_timeout",
			Session:     application.ProjectSessionConnected{RecoveryMode: "new"},
			CompletedAt: now.Add(time.Minute),
		},
	); err != nil {
		t.Fatal(err)
	}

	var sessionState, jobState string
	if err := project.db.QueryRow(`SELECT s.state,j.state FROM acp_sessions s JOIN background_jobs j ON j.target_id=s.session_id WHERE s.session_id='session'`).
		Scan(&sessionState, &jobState); err != nil {
		t.Fatal(err)
	}

	if sessionState != "failed" || jobState != "failed" {
		t.Fatalf("states = %s / %s", sessionState, jobState)
	}

	var acceptedType, completedType string
	if err := project.db.QueryRow(`SELECT typeof(accepted_at),typeof(completed_at) FROM background_jobs WHERE job_id='job'`).
		Scan(&acceptedType, &completedType); err != nil {
		t.Fatal(err)
	}

	if acceptedType != "integer" || completedType != "integer" {
		t.Fatalf("timestamp types = %s / %s", acceptedType, completedType)
	}
}

func TestExportCompletionCommitFailureReconcilesPublishedFile(t *testing.T) {
	for _, test := range []struct {
		name, old string
		published bool
	}{
		{name: "new file", published: true},
		{name: "overwrite", old: "old output", published: true},
		{name: "wrong file", old: "old output"},
	} {
		t.Run(test.name, func(t *testing.T) {
			project := projectTestDB(t)

			ctx := context.Background()
			if _, err := project.CreateProject(ctx, projectRecord("project", "create")); err != nil {
				t.Fatal(err)
			}

			if _, err := project.db.Exec(
				`INSERT INTO procedures(procedure_id,project_id,revision,status,document_json,created_at,completed_at) VALUES('procedure','project',1,'completed','{}',1,1)`,
			); err != nil {
				t.Fatal(err)
			}

			path := filepath.Join(t.TempDir(), "procedure.md")
			if test.old != "" {
				if err := os.WriteFile(path, []byte(test.old), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			now := time.Date(2026, 9, 26, 1, 2, 3, 0, time.UTC)
			repository := NewProjectExternalRepository(project.db)

			record := application.ProjectExportRecord{
				ExportProcedureInput: application.ExportProcedureInput{
					ProcedureID:       "procedure",
					ProcedureRevision: 1,
					Format:            "markdown",
					Destination:       application.VerifiedPathSelection{AbsolutePath: path, ResolvedPath: path},
					OperationID:       "export",
				},
				ExportID:    "export",
				JobID:       "job",
				RequestHash: "hash",
				Receipt:     application.MutationReceipt{OperationID: "export", CommittedAt: now},
				Event: application.OutboxEvent{
					ID:            "accepted",
					Name:          "export.updated",
					AggregateType: "export",
					AggregateID:   "export",
					EmittedAt:     now,
				},
			}
			if _, err := repository.AcceptExport(ctx, record); err != nil {
				t.Fatal(err)
			}

			if _, err := repository.ClaimProjectJob(ctx); err != nil {
				t.Fatal(err)
			}

			content := []byte("published output")

			digest := sha256.Sum256(content)
			if err := repository.MarkExportReady(ctx, "export", hex.EncodeToString(digest[:])); err != nil {
				t.Fatal(err)
			}

			actual := content
			if !test.published {
				actual = []byte("different output")
			}

			if err := os.WriteFile(path, actual, 0o600); err != nil {
				t.Fatal(err)
			}

			if _, err := project.db.Exec(
				`CREATE TRIGGER fail_export_complete BEFORE UPDATE ON background_jobs WHEN NEW.state='succeeded' BEGIN SELECT RAISE(ABORT,'commit failpoint'); END`,
			); err != nil {
				t.Fatal(err)
			}

			if err := repository.CompleteProjectJob(
				ctx,
				application.ProjectJobCompletion{
					JobID:       "job",
					Kind:        "export",
					TargetID:    "export",
					Success:     true,
					CompletedAt: now,
				},
			); err == nil {
				t.Fatal("completion should fail")
			}

			if _, err := project.db.Exec(`DROP TRIGGER fail_export_complete`); err != nil {
				t.Fatal(err)
			}

			if err := repository.FailInterruptedProjectJobs(ctx, now.Add(time.Minute)); err != nil {
				t.Fatal(err)
			}

			var exportState, jobState string
			if err := project.db.QueryRow(`SELECT e.state,j.state FROM exports e JOIN background_jobs j ON j.target_id=e.export_id WHERE e.export_id='export'`).
				Scan(&exportState, &jobState); err != nil {
				t.Fatal(err)
			}

			want := "succeeded"
			if !test.published {
				want = "interrupted"
			}

			if !test.published {
				if exportState != "failed" || jobState != want {
					t.Fatalf("states = %s / %s", exportState, jobState)
				}

				return
			}

			if exportState != want || jobState != want {
				t.Fatalf("states = %s / %s", exportState, jobState)
			}
		})
	}
}

func TestExportReplayAfterFreshPrepare(t *testing.T) {
	project := projectTestDB(t)

	ctx := context.Background()
	if _, err := project.CreateProject(ctx, projectRecord("project", "create")); err != nil {
		t.Fatal(err)
	}

	if _, err := project.db.Exec(
		`INSERT INTO procedures(procedure_id,project_id,revision,status,document_json,created_at,completed_at) VALUES('procedure','project',1,'completed','{}',1,1)`,
	); err != nil {
		t.Fatal(err)
	}

	exporter := appexport.NewExporter()
	defer func() { _ = exporter.Close() }()

	repository := NewProjectExternalRepository(project.db)
	id := 0
	usecase := application.NewProjectExternal(repository, exporter, time.Now, func() string {
		id++
		return "generated-" + string(rune('0'+id))
	})
	path := filepath.Join(t.TempDir(), "procedure.md")

	var first application.MutationResult[application.ExportAccepted]

	for attempt := range 2 {
		selection, _, err := exporter.Verify(path, "procedure", 1)
		if err != nil {
			t.Fatal(err)
		}

		result, err := usecase.ExportProcedure(ctx, application.ExportProcedureInput{
			ProcedureID: "procedure", ProcedureRevision: 1, Format: "markdown",
			Destination: selection, OperationID: "same-operation",
		})
		if err != nil {
			t.Fatal(err)
		}

		if attempt == 0 {
			first = result
		} else if result.Data.ExportID != first.Data.ExportID || result.Receipt.OperationID != first.Receipt.OperationID {
			t.Fatalf("replay = %+v, want %+v", result, first)
		}
	}
}

func TestReconnectRevisionChangesRequestHash(t *testing.T) {
	project := projectTestDB(t)

	ctx := context.Background()
	if _, err := project.CreateProject(ctx, projectRecord("project", "create")); err != nil {
		t.Fatal(err)
	}

	repository := NewProjectExternalRepository(project.db)
	sequence := 0
	usecase := application.NewProjectExternal(
		repository,
		nil,
		func() time.Time { return time.Date(2026, 9, 26, 1, 2, 3, 0, time.UTC) },
		func() string {
			sequence++
			return "generated-" + string(rune('0'+sequence))
		},
	)

	tests := []struct {
		name     string
		revision int64
		wantCode string
	}{
		{name: "accepted", revision: 1},
		{name: "replayed", revision: 1},
		{name: "different revision", revision: 2, wantCode: "operation_id_conflict"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := usecase.ReconnectProjectSession(
				ctx,
				application.ReconnectSessionInput{
					ProjectID:        "project",
					ExpectedRevision: test.revision,
					Strategy:         "auto",
					OperationID:      "same-operation",
				},
			)

			var appErr *shared.Error
			if test.wantCode == "" && err != nil ||
				test.wantCode != "" && (!errors.As(err, &appErr) || appErr.Code != test.wantCode) {
				t.Fatalf("error = %v, want %q", err, test.wantCode)
			}
		})
	}
}
