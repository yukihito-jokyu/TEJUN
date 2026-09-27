package sqlite

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	appexport "github.com/yukihito-jokyu/TEJUN/internal/adapter/export"
	appwails "github.com/yukihito-jokyu/TEJUN/internal/adapter/wails"
	"github.com/yukihito-jokyu/TEJUN/internal/application"
)

func TestProcedureLiveServiceSQLiteExport(t *testing.T) {
	ctx := context.Background()
	execution, at := executionFixture(t)

	db := execution.db
	for _, statement := range []string{
		`INSERT INTO execution_evidence(evidence_id,execution_id,check_id,actor,kind,text,display_name,created_at) VALUES ('v','e','c','human','text','observed','Evidence','2026-09-26T00:00:00Z')`,
		`UPDATE execution_checks SET human_status='completed' WHERE execution_id='e' AND check_id='c'`,
	} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}

	root := t.TempDir()

	store, err := NewEvidenceStore(db, root)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = store.Close() })

	files, err := OpenEvidenceFiles(root)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = files.Close() })

	var picture bytes.Buffer
	if err := png.Encode(&picture, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}

	hashBytes := sha256.Sum256(picture.Bytes())
	hash := hex.EncodeToString(hashBytes[:])

	path := blobPath(hash)
	if err := os.MkdirAll(filepath.Join(root, filepath.Dir(path)), 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(root, path), picture.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO evidence_blobs(hash,size,mime,relative_path,status,ref_count) VALUES (?,?,?,?,'available',1)`,
		hash,
		picture.Len(),
		"image/png",
		path,
	); err != nil {
		t.Fatal(err)
	}

	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO execution_evidence(evidence_id,execution_id,check_id,actor,kind,blob_hash,display_name,created_at) VALUES ('image','e','c','human','image',?,'Screenshot','2026-09-26T00:00:00Z')`,
		hash,
	); err != nil {
		t.Fatal(err)
	}

	ids := 0
	newID := func() string { ids++; return "live-" + string(rune('a'+ids)) }
	now := func() time.Time { return at }

	generated, err := application.NewExecution(execution, now, newID).
		GenerateProcedureDraft(ctx, application.ProcedureDraftRecord{
			ExecutionID: "e", ExpectedRevision: 1, OperationID: "generate",
		})
	if err != nil {
		t.Fatal(err)
	}

	procedureID := generated.Data.ProcedureID
	repo := NewProcedureRepository(db, files)
	exporter := appexport.NewExporter()

	t.Cleanup(func() { _ = exporter.Close() })

	externalRepo := NewProjectExternalRepository(db)
	external := application.NewProjectExternal(externalRepo, exporter, now, newID)
	projectService := appwails.NewProjectService(nil, external, nil)
	service := appwails.NewProcedureService(
		application.NewProcedure(repo, now, newID),
		repo,
		projectService,
		nil,
		nil,
		nil,
	)

	view, err := service.GetProcedure(ctx, application.ProcedureViewQuery{ProjectID: "p"})
	if err != nil || view.Procedure.ProcedureID != procedureID || len(view.Evidence.Human) != 2 {
		t.Fatalf("generated view=%+v err=%v", view, err)
	}

	evidence, err := service.GetEvidence(ctx, appwails.GetEvidenceInput{ProcedureID: procedureID, EvidenceID: "v"})
	if err != nil || evidence.TextPage == nil || evidence.TextPage.Content != "observed" {
		t.Fatalf("evidence=%+v err=%v", evidence, err)
	}

	doc := view.Procedure.Document
	doc.Overview = "live integration"

	exporter.SetEvidenceReader(func(procedureID, evidenceID string) (appexport.EvidenceContent, error) {
		record, err := repo.GetEvidence(ctx, procedureID, evidenceID)
		if err != nil || record.Kind == "text" {
			return appexport.EvidenceContent{Kind: record.Kind, Data: []byte(record.Text)}, err
		}

		data, err := store.ReadForGeneratedProcedure(ctx, procedureID, evidenceID)

		return appexport.EvidenceContent{Kind: "image", Data: data}, err
	})

	saved, err := service.SaveProcedureDraft(
		ctx,
		application.ProcedureSaveInput{
			ProcedureID:      procedureID,
			ExpectedRevision: 1,
			OperationID:      "save",
			Document:         doc,
		},
	)
	if err != nil || saved.Data.Revision != 2 {
		t.Fatalf("saved=%+v err=%v", saved, err)
	}

	completed, err := service.CompleteProcedure(
		ctx,
		application.ProcedureCompleteInput{ProcedureID: procedureID, ExpectedRevision: 2, OperationID: "complete"},
	)
	if err != nil || completed.Data.Revision != 3 {
		t.Fatalf("completed=%+v err=%v", completed, err)
	}

	for _, format := range []string{"markdown", "pdf"} {
		ext := ".md"
		if format == "pdf" {
			ext = ".pdf"
		}

		target := filepath.Join(t.TempDir(), "procedure"+ext)

		prepared, err := service.PrepareExportProcedure(
			ctx,
			appwails.PrepareExportProcedureInput{ProcedureID: procedureID, ProcedureRevision: 3, AbsolutePath: target},
		)
		if err != nil {
			t.Fatalf("prepare %s: %v", format, err)
		}

		accepted, err := service.ExportProcedure(
			ctx,
			appwails.ExportProcedureInput{
				ProcedureID:       procedureID,
				ProcedureRevision: 3,
				Format:            format,
				Destination:       prepared.Destination,
				OperationID:       "export-" + format,
			},
		)
		if err != nil {
			t.Fatalf("accept %s: %v", format, err)
		}

		ran, err := application.RunProjectJob(ctx, externalRepo, nil, exporter, now)
		if err != nil || !ran {
			t.Fatalf("run %s: %v %v", format, ran, err)
		}

		output, err := os.ReadFile(target)
		if err != nil || len(output) == 0 {
			t.Fatalf("output %s: %v", format, err)
		}

		if format == "markdown" && !strings.Contains(string(output), "live integration") {
			t.Fatalf("markdown missing saved text: %s", output)
		}

		if format == "pdf" && !bytes.HasPrefix(output, []byte("%PDF-")) {
			t.Fatal("PDF header missing")
		}

		if format == "pdf" && !bytes.Contains(output, []byte("/Subtype /Image")) {
			t.Fatal("PDF image missing")
		}

		if accepted.Data.JobID == "" {
			t.Fatal("empty export job")
		}
	}

	for _, test := range []struct {
		name, query string
	}{
		{name: "unlinked", query: `DELETE FROM procedure_source_evidence WHERE procedure_id=? AND evidence_id='image'`},
		{name: "pending", query: `UPDATE evidence_blobs SET status='pending' WHERE hash=?`},
		{name: "wrong MIME", query: `UPDATE evidence_blobs SET mime='image/jpeg' WHERE hash=?`},
	} {
		t.Run(test.name, func(t *testing.T) {
			argument := any(hash)
			if test.name == "unlinked" {
				argument = procedureID
			}

			if _, err := db.ExecContext(ctx, test.query, argument); err != nil {
				t.Fatal(err)
			}

			if _, err := store.ReadForGeneratedProcedure(ctx, procedureID, "image"); err == nil {
				t.Fatal("invalid image was readable")
			}

			switch test.name {
			case "unlinked":
				_, err = db.ExecContext(
					ctx,
					`INSERT INTO procedure_source_evidence(procedure_id,evidence_id) VALUES (?,'image')`,
					procedureID,
				)
			case "pending":
				_, err = db.ExecContext(ctx, `UPDATE evidence_blobs SET status='available' WHERE hash=?`, hash)
			default:
				_, err = db.ExecContext(ctx, `UPDATE evidence_blobs SET mime='image/png' WHERE hash=?`, hash)
			}

			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
