package sqlite

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/yukihito-jokyu/TEJUN/internal/application"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/procedure"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/shared"
)

func TestProcedureDraftLifecycle(t *testing.T) {
	ctx := context.Background()
	executionRepo, at := executionFixture(t)

	db := executionRepo.db
	for _, statement := range []string{
		`INSERT INTO execution_evidence(evidence_id,execution_id,check_id,actor,kind,text,display_name,created_at) VALUES ('v','e','c','human','text','observed','Evidence','2026-09-26T00:00:00Z')`,
		`UPDATE execution_checks SET human_status='completed' WHERE execution_id='e' AND check_id='c'`,
	} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}

	generated, err := executionRepo.GenerateProcedureDraft(
		ctx,
		application.ProcedureDraftRecord{
			ExecutionID:      "e",
			OperationID:      "gen",
			ProcedureID:      "pdoc",
			ExpectedRevision: 1,
			At:               at,
			Event: application.OutboxEvent{
				ID:            "gen-event",
				Name:          "procedure.updated",
				AggregateType: "procedure",
				AggregateID:   "pdoc",
				EmittedAt:     at,
			},
		},
	)
	if err != nil || generated.Data.NextRoute != "#/projects/p/procedure" {
		t.Fatalf("generate=%+v err=%v", generated, err)
	}

	repo := NewProcedureRepository(db, nil)

	view, err := repo.GetProcedure(ctx, application.ProcedureViewQuery{ProjectID: "p"})
	if err != nil {
		t.Fatal(err)
	}

	if view.Source.ExecutionRevision != 1 || view.Source.EvidenceCount != 1 || len(view.Evidence.Human) != 1 ||
		view.Integrity.Status != "valid" {
		t.Fatalf("view=%+v", view)
	}

	doc := view.Procedure.Document
	if len(doc.Steps) != 1 || doc.Steps[0].Command != "go version" {
		t.Fatalf("generated command = %+v", doc.Steps)
	}

	doc.Overview = "updated"

	tests := []struct {
		name      string
		revision  int64
		operation string
		document  procedure.Document
		want      string
	}{
		{name: "save", revision: 1, operation: "save", document: doc},
		{name: "replay", revision: 1, operation: "save", document: doc},
		{name: "conflict", revision: 1, operation: "new", document: doc, want: "revision_conflict"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := repo.SaveProcedureDraft(
				ctx,
				application.ProcedureSaveRecord{
					ProcedureSaveInput: application.ProcedureSaveInput{
						ProcedureID:      "pdoc",
						ExpectedRevision: tt.revision,
						OperationID:      tt.operation,
						Document:         tt.document,
					},
					At: at,
					Event: application.OutboxEvent{
						ID:            "event:" + tt.operation,
						Name:          "procedure.updated",
						AggregateType: "procedure",
						AggregateID:   "pdoc",
						EmittedAt:     at,
					},
				},
			)
			if tt.want != "" {
				var business *shared.Error
				if !errors.As(err, &business) || business.Code != tt.want {
					t.Fatalf("err=%v", err)
				}

				return
			}

			if err != nil || result.Data.Revision != 2 {
				t.Fatalf("result=%+v err=%v", result, err)
			}
		})
	}

	complete, err := repo.CompleteProcedure(
		ctx,
		application.ProcedureCompleteRecord{
			ProcedureCompleteInput: application.ProcedureCompleteInput{
				ProcedureID:      "pdoc",
				ExpectedRevision: 2,
				OperationID:      "complete",
			},
			At: at,
			Event: application.OutboxEvent{
				ID:            "completed-event",
				Name:          "procedure.updated",
				AggregateType: "procedure",
				AggregateID:   "pdoc",
				EmittedAt:     at,
			},
			ProjectEvent: application.OutboxEvent{
				ID:            "project-event",
				Name:          "project.changed",
				AggregateType: "project",
				EmittedAt:     at,
			},
		},
	)
	if err != nil || complete.Data.Status != procedure.Completed || complete.Data.Revision != 3 {
		t.Fatalf("complete=%+v err=%v", complete, err)
	}

	_, err = repo.SaveProcedureDraft(
		ctx,
		application.ProcedureSaveRecord{
			ProcedureSaveInput: application.ProcedureSaveInput{
				ProcedureID:      "pdoc",
				ExpectedRevision: 3,
				OperationID:      "later",
				Document:         doc,
			},
			At: at,
		},
	)

	var business *shared.Error
	if !errors.As(err, &business) || business.Code != "invalid_state" {
		t.Fatalf("completed edit err=%v", err)
	}

	_, err = NewProjectRepository(db).DeleteProject(ctx, application.ProjectDeleteRecord{
		DeleteProjectInput: application.DeleteProjectInput{ProjectID: "p", ExpectedRevision: 3, OperationID: "delete"},
		RequestHash:        "delete",
		Receipt:            application.MutationReceipt{OperationID: "delete", CommittedAt: at},
		Event: application.OutboxEvent{
			ID:            "delete-event",
			Name:          "project.deleted",
			AggregateType: "project",
			AggregateID:   "p",
			EmittedAt:     at,
			Correlation:   "delete",
		},
	})
	if err != nil {
		t.Fatalf("delete generated project: %v", err)
	}
}

func TestCompleteProcedureRejectsUnstoredImage(t *testing.T) {
	for _, tc := range []struct {
		name, mutation string
	}{
		{name: "missing procedure relation", mutation: `DELETE FROM procedure_source_evidence WHERE procedure_id='pdoc' AND evidence_id='image'`},
		{name: "pending blob", mutation: `UPDATE evidence_blobs SET status='pending' WHERE hash=?`},
		{name: "wrong MIME", mutation: `UPDATE evidence_blobs SET mime='image/jpeg' WHERE hash=?`},
		{name: "missing file", mutation: "remove"},
		{name: "corrupt file", mutation: "corrupt"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			executionRepo, at := executionFixture(t)

			db := executionRepo.db
			root := t.TempDir()

			files, err := OpenEvidenceFiles(root)
			if err != nil {
				t.Fatal(err)
			}

			t.Cleanup(func() { _ = files.Close() })

			var picture bytes.Buffer
			if err := png.Encode(&picture, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
				t.Fatal(err)
			}

			sourcePath := filepath.Join(root, "source.png")
			if err := os.WriteFile(sourcePath, picture.Bytes(), 0o600); err != nil {
				t.Fatal(err)
			}

			staged, err := files.Stage(ctx, sourcePath)
			if err != nil {
				t.Fatal(err)
			}

			if err := files.Commit(ctx, staged); err != nil {
				t.Fatal(err)
			}

			for _, query := range []string{
				`INSERT INTO execution_evidence(evidence_id,execution_id,check_id,actor,kind,text,display_name,created_at) VALUES ('text','e','c','human','text','observed','Text','2026-09-26T00:00:00Z')`,
				`UPDATE execution_checks SET human_status='completed' WHERE execution_id='e' AND check_id='c'`,
			} {
				if _, err := db.ExecContext(ctx, query); err != nil {
					t.Fatal(err)
				}
			}

			if _, err := db.ExecContext(ctx,
				`INSERT INTO evidence_blobs(hash,size,mime,relative_path,status) VALUES (?,?,?,?,'available')`,
				staged.Hash, staged.Size, staged.MIME, blobPath(staged.Hash)); err != nil {
				t.Fatal(err)
			}

			if _, err := db.ExecContext(
				ctx,
				`INSERT INTO execution_evidence(evidence_id,execution_id,check_id,actor,kind,blob_hash,display_name,created_at) VALUES ('image','e','c','human','image',?,'Screenshot','2026-09-26T00:00:00Z')`,
				staged.Hash,
			); err != nil {
				t.Fatal(err)
			}

			_, err = executionRepo.GenerateProcedureDraft(
				ctx,
				application.ProcedureDraftRecord{
					ExecutionID:      "e",
					OperationID:      "gen",
					ProcedureID:      "pdoc",
					ExpectedRevision: 1,
					At:               at,
					Event: application.OutboxEvent{
						ID:            "gen-event",
						Name:          "procedure.updated",
						AggregateType: "procedure",
						AggregateID:   "pdoc",
						EmittedAt:     at,
					},
				},
			)
			if err != nil {
				t.Fatal(err)
			}

			switch tc.mutation {
			case "remove":
				err = os.Remove(filepath.Join(root, blobPath(staged.Hash)))
			case "corrupt":
				err = os.WriteFile(
					filepath.Join(root, blobPath(staged.Hash)),
					bytes.Repeat([]byte("x"), int(staged.Size)),
					0o600,
				)
			default:
				_, err = db.ExecContext(ctx, tc.mutation, staged.Hash)
			}

			if err != nil {
				t.Fatal(err)
			}

			_, err = NewProcedureRepository(
				db,
				files,
			).CompleteProcedure(ctx, application.ProcedureCompleteRecord{ProcedureCompleteInput: application.ProcedureCompleteInput{ProcedureID: "pdoc", ExpectedRevision: 1, OperationID: "complete"}, At: at})

			want := "evidence_invalid"
			if tc.name == "missing procedure relation" {
				want = "invalid_state"
			}

			var business *shared.Error
			if !errors.As(err, &business) || business.Code != want {
				t.Fatalf("err=%v", err)
			}
		})
	}
}
