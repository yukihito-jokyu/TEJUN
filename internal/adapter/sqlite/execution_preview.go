package sqlite

import (
	"context"
	"errors"
)

// ReadForExecution returns an image only when it belongs to the requested project, execution, and check.
func (s *EvidenceStore) ReadForExecution(
	ctx context.Context,
	projectID, executionID, checkID, evidenceID string,
) ([]byte, string, error) {
	var hash, mime, relative string

	err := s.db.QueryRowContext(ctx, `SELECT b.hash, b.mime, b.relative_path
FROM execution_evidence v
JOIN executions e ON e.execution_id = v.execution_id
JOIN evidence_blobs b ON b.hash = v.blob_hash
WHERE e.project_id = ? AND v.execution_id = ? AND v.check_id = ? AND v.evidence_id = ?
AND v.kind = 'image' AND b.status = 'available'`, projectID, executionID, checkID, evidenceID).
		Scan(&hash, &mime, &relative)
	if err != nil {
		return nil, "", errors.New("利用可能な画像証跡ではありません")
	}

	data, err := s.readBlob(relative, hash, mime)
	if err != nil {
		return nil, "", err
	}

	return data, mime, nil
}
