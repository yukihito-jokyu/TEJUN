package sqlite

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type EvidenceStore struct {
	db   *sql.DB
	root *os.Root
	// ponytail: 保存と削除を直列化する。証跡処理が詰まる場合はhash単位のlockへ分割する。
	mu sync.Mutex
}

func NewEvidenceStore(db *sql.DB, dataDir string) (*EvidenceStore, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}

	root, err := os.OpenRoot(dataDir)
	if err != nil {
		return nil, err
	}

	store := &EvidenceStore{db: db, root: root}
	for _, name := range []string{"blobs/sha256", "staging"} {
		if err := store.mkdirAll(name); err != nil {
			_ = root.Close()
			return nil, err
		}
	}

	return store, nil
}

func (s *EvidenceStore) Close() error { return s.root.Close() }

func (s *EvidenceStore) SaveImage(
	ctx context.Context,
	evidenceID, executionID, checkID, actor, description string,
	data []byte,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db != nil {
		if err := s.cleanupDeleted(ctx); err != nil {
			return err
		}
	}

	if evidenceID == "" || strings.ContainsAny(evidenceID, `/\`) || executionID == "" || checkID == "" ||
		(actor != "human" && actor != "ai") ||
		len(data) == 0 ||
		len(data) > 25<<20 {
		return errors.New("証跡の入力が不正です")
	}

	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "png" && format != "jpeg") || int64(config.Width)*int64(config.Height) > 100_000_000 {
		return errors.New("証跡画像が不正です")
	}

	if _, _, err := image.Decode(bytes.NewReader(data)); err != nil {
		return err
	}

	magic := httpImageMime(data)
	if (format == "png" && magic != "image/png") || (format == "jpeg" && magic != "image/jpeg") {
		return errors.New("証跡MIMEが一致しません")
	}

	hash := sha256.Sum256(data)
	id := hex.EncodeToString(hash[:])
	relative := filepath.Join("blobs", "sha256", id[:2], id)

	if err := s.mkdirAll(filepath.Join("blobs", "sha256", id[:2])); err != nil {
		return err
	}

	staging := filepath.Join("staging", evidenceID+".tmp")

	file, err := s.openFile(staging, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}

	committed := false
	defer func() {
		if !committed {
			_ = s.remove(staging)
		}
	}()

	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}

	err = errors.Join(err, file.Close())
	if err != nil {
		_ = s.remove(staging)
		return err
	}

	if err := s.syncDirectory("staging"); err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	defer func() { _ = tx.Rollback() }()

	var related int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM executions e JOIN projects p ON p.project_id=e.project_id JOIN check_items c ON c.project_id=p.project_id WHERE e.execution_id=? AND c.check_id=?`, executionID, checkID).
		Scan(&related); err != nil {
		return fmt.Errorf("証跡の所属が不正です: %w", err)
	}

	var existing string

	err = tx.QueryRowContext(ctx, `SELECT status FROM evidence_blobs WHERE hash=?`, id).Scan(&existing)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	if existing != "" {
		if existing != "available" {
			return errors.New("同じ証跡blobを保存中です")
		}

		if _, err := s.readBlob(relative, id, magic); err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, `UPDATE evidence_blobs SET ref_count=ref_count+1 WHERE hash=?`, id)
	} else {
		_, err = tx.ExecContext(
			ctx,
			`INSERT INTO evidence_blobs(hash,size,mime,relative_path,status,staging_name,ref_count) VALUES(?,?,?,?,'pending',?,1)`,
			id,
			len(data),
			magic,
			relative,
			staging,
		)
	}

	if err != nil {
		return err
	}

	if _, err = tx.ExecContext(
		ctx,
		`INSERT INTO evidence_records(evidence_id,execution_id,blob_hash,actor,description,created_at_us) VALUES(?,?,?,?,?,?)`,
		evidenceID,
		executionID,
		id,
		actor,
		description,
		time.Now().UTC().UnixMicro(),
	); err != nil {
		return err
	}

	if _, err = tx.ExecContext(
		ctx,
		`INSERT INTO check_evidence(check_id,evidence_id) VALUES(?,?)`,
		checkID,
		evidenceID,
	); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	committed = true

	if existing == "available" {
		return s.remove(staging)
	}

	if err := s.link(staging, relative); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return err
		}

		if _, err := s.readBlob(relative, id, magic); err != nil {
			return err
		}
	}

	_ = s.remove(staging)
	if err := s.syncBlob(relative); err != nil {
		return err
	}

	return s.markAvailable(ctx, id)
}

func httpImageMime(data []byte) string {
	if len(data) >= 8 && bytes.Equal(data[:8], []byte("\x89PNG\r\n\x1a\n")) {
		return "image/png"
	}

	if len(data) >= 3 && bytes.Equal(data[:3], []byte("\xff\xd8\xff")) {
		return "image/jpeg"
	}

	return ""
}

func (s *EvidenceStore) syncBlob(relative string) error {
	f, err := s.openFile(relative, os.O_RDONLY, 0)
	if err != nil {
		return err
	}

	if err := errors.Join(f.Sync(), f.Close()); err != nil {
		return err
	}

	return s.syncDirectory(filepath.Dir(relative))
}

func (s *EvidenceStore) syncDirectory(relative string) error {
	d, err := s.openDir(relative)
	if err != nil {
		return err
	}

	return errors.Join(d.Sync(), d.Close())
}

func (s *EvidenceStore) markAvailable(ctx context.Context, hash string) error {
	_, err := s.db.ExecContext(
		ctx,
		`UPDATE evidence_blobs SET status='available',staging_name=NULL WHERE hash=? AND status='pending'`,
		hash,
	)

	return err
}

func (s *EvidenceStore) Reconcile(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.cleanupDeleted(ctx); err != nil {
		return err
	}

	rows, err := s.db.QueryContext(
		ctx,
		`SELECT hash,mime,relative_path,staging_name FROM evidence_blobs WHERE status='pending'`,
	)
	if err != nil {
		return err
	}

	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var hash, mime, relative string

		var staging sql.NullString
		if err := rows.Scan(&hash, &mime, &relative, &staging); err != nil {
			return err
		}

		if staging.Valid {
			if f, err := s.openFile(relative, os.O_RDONLY, 0); errors.Is(err, os.ErrNotExist) {
				_ = s.link(staging.String, relative)
			} else if err == nil {
				_ = f.Close()
			}
		}

		data, err := s.readBlob(relative, hash, mime)
		if err == nil && len(data) > 0 {
			if err := s.syncBlob(relative); err != nil {
				return err
			}

			if err := s.markAvailable(ctx, hash); err != nil {
				return err
			}

			if staging.Valid {
				_ = s.remove(staging.String)
			}
		} else if _, err := s.db.ExecContext(ctx, `UPDATE evidence_blobs SET status='corrupt' WHERE hash=?`, hash); err != nil {
			return err
		}
	}

	return rows.Err()
}

func (s *EvidenceStore) cleanupDeleted(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `SELECT hash,relative_path FROM evidence_blob_deletions`)
	if err != nil {
		return err
	}

	type pending struct{ hash, path string }

	var files []pending

	for rows.Next() {
		var file pending
		if err := rows.Scan(&file.hash, &file.path); err != nil {
			_ = rows.Close()
			return err
		}

		files = append(files, file)
	}

	err = errors.Join(rows.Err(), rows.Close())
	if err != nil {
		return err
	}

	for _, file := range files {
		if len(file.hash) != 64 || file.path != filepath.Join("blobs", "sha256", file.hash[:2], file.hash) {
			return errors.New("削除対象の証跡pathが不正です")
		}

		if err := s.remove(file.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}

		if err := s.syncDirectory(filepath.Dir(file.path)); err != nil {
			return err
		}

		if _, err := s.db.ExecContext(ctx, `DELETE FROM evidence_blob_deletions WHERE hash=?`, file.hash); err != nil {
			return err
		}
	}

	return nil
}

func (s *EvidenceStore) readBlob(relative, hash, mime string) ([]byte, error) {
	if len(hash) != 64 || relative != filepath.Join("blobs", "sha256", hash[:2], hash) {
		return nil, errors.New("証跡の保存先が不正です")
	}

	f, err := s.openFile(relative, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}

	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 25<<20 {
		return nil, errors.New("証跡fileが不正です")
	}

	data, err := io.ReadAll(io.LimitReader(f, 25<<20+1))
	if err != nil {
		return nil, err
	}

	actual := sha256.Sum256(data)
	if hex.EncodeToString(actual[:]) != hash || (mime != "" && httpImageMime(data) != mime) {
		return nil, errors.New("証跡blobの検証に失敗しました")
	}

	return data, nil
}

func (s *EvidenceStore) ReadForProcedure(ctx context.Context, procedureID, evidenceID string) ([]byte, error) {
	var hash, mime, relative, status string

	err := s.db.QueryRowContext(ctx, `SELECT b.hash,b.mime,b.relative_path,b.status FROM procedure_sources ps JOIN procedure_evidence pe ON pe.procedure_id=ps.procedure_id JOIN evidence_records er ON er.evidence_id=pe.evidence_id AND er.execution_id=ps.execution_id JOIN evidence_blobs b ON b.hash=er.blob_hash WHERE ps.procedure_id=? AND pe.evidence_id=?`, procedureID, evidenceID).
		Scan(&hash, &mime, &relative, &status)
	if err != nil || status != "available" {
		return nil, errors.New("利用可能な証跡ではありません")
	}

	return s.readBlob(relative, hash, mime)
}

func (s *EvidenceStore) LinkProcedureEvidence(
	ctx context.Context,
	procedureID, executionID string,
	evidenceIDs []string,
) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var matches int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM procedures p JOIN executions e ON e.project_id=p.project_id WHERE p.procedure_id=? AND e.execution_id=?`, procedureID, executionID).
		Scan(&matches); err != nil {
		return err
	}

	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO procedure_sources(procedure_id,execution_id) VALUES(?,?)`,
		procedureID,
		executionID,
	); err != nil {
		return err
	}

	for _, evidenceID := range evidenceIDs {
		if err := tx.QueryRowContext(ctx, `SELECT 1 FROM evidence_records er JOIN evidence_blobs b ON b.hash=er.blob_hash WHERE er.evidence_id=? AND er.execution_id=? AND b.status='available'`, evidenceID, executionID).
			Scan(&matches); err != nil {
			return fmt.Errorf("証跡の所属または状態が不正です: %w", err)
		}

		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO procedure_evidence(procedure_id,evidence_id) VALUES(?,?)`,
			procedureID,
			evidenceID,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}
