//go:build darwin || linux

package sqlite

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestEvidenceStoreOwnershipAndRecovery(t *testing.T) {
	project := projectTestDB(t)

	ctx := context.Background()
	if _, err := project.CreateProject(ctx, projectRecord("project", "create")); err != nil {
		t.Fatal(err)
	}

	for _, query := range []string{
		`INSERT INTO check_items(check_id,project_id,sequence,title,instruction,expected_result,ai_required,human_required,human_evidence_requirement) VALUES('check','project',1,'t','i','e',0,1,'required')`,
		`INSERT INTO executions(execution_id,project_id,revision) VALUES('execution','project',1)`,
		`INSERT INTO procedures(procedure_id,project_id,revision,status,document_json,created_at) VALUES('procedure','project',1,'completed','{}',1)`,
	} {
		if _, err := project.db.ExecContext(ctx, query); err != nil {
			t.Fatal(err)
		}
	}

	imageData := image.NewRGBA(image.Rect(0, 0, 4, 3))
	imageData.Set(0, 0, color.RGBA{R: 255, A: 255})

	var encoded bytes.Buffer
	if err := png.Encode(&encoded, imageData); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()

	store, err := NewEvidenceStore(project.db, root)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = store.Close() }()

	if err := store.SaveImage(ctx, "evidence", "execution", "check", "human", "capture", encoded.Bytes()); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name, procedure, evidence string
		link                      bool
		wantError                 bool
	}{
		{"unlinked", "procedure", "evidence", false, true},
		{"linked", "procedure", "evidence", true, false},
		{"other evidence", "procedure", "other", false, true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if test.link {
				if err := store.LinkProcedureEvidence(
					ctx,
					test.procedure,
					"execution",
					[]string{test.evidence},
				); err != nil {
					t.Fatal(err)
				}
			}

			data, err := store.ReadForProcedure(ctx, test.procedure, test.evidence)
			if (err != nil) != test.wantError || (!test.wantError && !bytes.Equal(data, encoded.Bytes())) {
				t.Fatalf("read = %d bytes, error = %v", len(data), err)
			}
		})
	}

	hash := sha256.Sum256(encoded.Bytes())
	id := hex.EncodeToString(hash[:])
	path := filepath.Join(root, "blobs", "sha256", id[:2], id)

	stage := filepath.Join(root, "staging", "recovery.tmp")
	if err := os.Rename(path, stage); err != nil {
		t.Fatal(err)
	}

	if _, err := project.db.ExecContext(
		ctx,
		`UPDATE evidence_blobs SET status='pending',staging_name='staging/recovery.tmp' WHERE hash=?`,
		id,
	); err != nil {
		t.Fatal(err)
	}

	if _, err := store.ReadForProcedure(ctx, "procedure", "evidence"); err == nil {
		t.Fatal("pending evidence was readable")
	}

	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = NewEvidenceStore(project.db, root)
	if err != nil {
		t.Fatal(err)
	}

	if err := store.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}

	if _, err := store.ReadForProcedure(ctx, "procedure", "evidence"); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := store.ReadForProcedure(ctx, "procedure", "evidence"); err == nil {
		t.Fatal("tampered evidence was readable")
	}

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	outside := filepath.Join(t.TempDir(), id)
	if err := os.WriteFile(outside, encoded.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.Symlink(outside, path); err != nil {
		t.Fatal(err)
	}

	if _, err := project.db.ExecContext(
		ctx,
		`UPDATE evidence_blobs SET status='pending',staging_name='staging/missing.tmp' WHERE hash=?`,
		id,
	); err != nil {
		t.Fatal(err)
	}

	if err := store.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}

	var status string
	if err := project.db.QueryRowContext(
		ctx,
		`SELECT status FROM evidence_blobs WHERE hash=?`,
		id,
	).Scan(&status); err != nil {
		t.Fatal(err)
	}

	if status != "corrupt" {
		t.Fatalf("symlink status = %q, want corrupt", status)
	}
}

func TestEvidencePathNoFollow(t *testing.T) {
	imageData := image.NewRGBA(image.Rect(0, 0, 1, 1))

	var encoded bytes.Buffer
	if err := png.Encode(&encoded, imageData); err != nil {
		t.Fatal(err)
	}

	data := encoded.Bytes()
	hash := sha256.Sum256(data)
	id := hex.EncodeToString(hash[:])
	relative := filepath.Join("blobs", "sha256", id[:2], id)

	cases := []struct {
		name   string
		save   bool
		attack func(t *testing.T, root, outside string)
	}{
		{
			name: "intermediate symlink outside root",
			save: true,
			attack: func(t *testing.T, root, outside string) {
				t.Helper()

				if err := os.Symlink(outside, filepath.Join(root, "blobs", "sha256", id[:2])); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "final symlink to same inode",
			attack: func(t *testing.T, root, outside string) {
				t.Helper()

				dir := filepath.Join(root, "blobs", "sha256", id[:2])
				if err := os.Mkdir(dir, 0o700); err != nil {
					t.Fatal(err)
				}

				if err := os.Link(filepath.Join(outside, id), filepath.Join(dir, "same-inode")); err != nil {
					t.Fatal(err)
				}

				if err := os.Symlink("same-inode", filepath.Join(dir, id)); err != nil {
					t.Fatal(err)
				}
			},
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()

			outside := t.TempDir()
			if err := os.WriteFile(filepath.Join(outside, id), data, 0o600); err != nil {
				t.Fatal(err)
			}

			store, err := NewEvidenceStore(nil, root)
			if err != nil {
				t.Fatal(err)
			}

			defer func() { _ = store.Close() }()

			test.attack(t, root, outside)

			if test.save {
				if err := store.SaveImage(
					context.Background(), "other", "execution", "check", "human", "capture", data,
				); err == nil {
					t.Fatal("save followed intermediate symlink")
				}
			}

			if _, err := store.readBlob(relative, id, ""); err == nil {
				t.Fatal("symlink was followed")
			}

			if err := store.syncBlob(relative); err == nil {
				t.Fatal("symlink was synced")
			}
		})
	}
}
