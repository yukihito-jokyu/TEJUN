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
	"testing"
)

func TestReadForExecution(t *testing.T) {
	repo, _ := executionFixture(t)
	root := t.TempDir()

	store, err := NewEvidenceStore(repo.db, root)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = store.Close() })

	var imageData bytes.Buffer
	if err := png.Encode(&imageData, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}

	hashBytes := sha256.Sum256(imageData.Bytes())
	hash := hex.EncodeToString(hashBytes[:])

	path := blobPath(hash)
	if err := os.MkdirAll(filepath.Join(root, filepath.Dir(path)), 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(root, path), imageData.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	if _, err := repo.db.ExecContext(ctx, `INSERT INTO evidence_blobs
(hash,size,mime,relative_path,status,ref_count) VALUES (?,?,?,?,'available',1)`,
		hash, imageData.Len(), "image/png", path); err != nil {
		t.Fatal(err)
	}

	if _, err := repo.db.ExecContext(ctx, `INSERT INTO execution_evidence
(evidence_id,execution_id,check_id,actor,kind,blob_hash,created_at)
VALUES ('v','e','c','human','image',?,'2026-09-26T00:00:00Z')`, hash); err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		name, project, execution, check, evidence, status string
		remove, symlink, corrupt                          bool
		wantOK                                            bool
	}{
		{name: "valid", project: "p", execution: "e", check: "c", evidence: "v", wantOK: true},
		{name: "other project", project: "other", execution: "e", check: "c", evidence: "v"},
		{name: "other execution", project: "p", execution: "other", check: "c", evidence: "v"},
		{name: "other check", project: "p", execution: "e", check: "other", evidence: "v"},
		{name: "other evidence", project: "p", execution: "e", check: "c", evidence: "other"},
		{name: "pending", project: "p", execution: "e", check: "c", evidence: "v", status: "pending"},
		{name: "missing", project: "p", execution: "e", check: "c", evidence: "v", remove: true},
		{name: "corrupt", project: "p", execution: "e", check: "c", evidence: "v", corrupt: true},
		{name: "symlink outside root", project: "p", execution: "e", check: "c", evidence: "v", symlink: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.status != "" {
				_, _ = repo.db.ExecContext(ctx, `UPDATE evidence_blobs SET status=? WHERE hash=?`, tt.status, hash)
			}

			defer func() {
				_, _ = repo.db.ExecContext(ctx, `UPDATE evidence_blobs SET status='available' WHERE hash=?`, hash)
			}()

			if tt.remove || tt.corrupt || tt.symlink {
				if err := os.Remove(filepath.Join(root, path)); err != nil {
					t.Fatal(err)
				}
				defer func() {
					_ = os.Remove(filepath.Join(root, path))
					_ = os.WriteFile(filepath.Join(root, path), imageData.Bytes(), 0o600)
				}()

				if tt.corrupt {
					_ = os.WriteFile(filepath.Join(root, path), []byte("bad"), 0o600)
				}

				if tt.symlink {
					outside := filepath.Join(t.TempDir(), "outside")

					_ = os.WriteFile(outside, imageData.Bytes(), 0o600)
					if err := os.Symlink(outside, filepath.Join(root, path)); err != nil {
						t.Fatal(err)
					}
				}
			}

			data, mime, err := store.ReadForExecution(ctx, tt.project, tt.execution, tt.check, tt.evidence)
			if tt.wantOK {
				if err != nil || mime != "image/png" || !bytes.Equal(data, imageData.Bytes()) {
					t.Fatalf("mime=%q err=%v", mime, err)
				}
			} else if err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
}
