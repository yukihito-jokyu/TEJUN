package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/yukihito-jokyu/TEJUN/internal/application"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/execution"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/shared"
)

func (r *ExecutionRepository) FindEvidenceOperation(ctx context.Context,
	input application.AttachHumanEvidenceInput,
) (application.MutationResult[application.EvidenceAttached], bool, bool, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return application.MutationResult[application.EvidenceAttached]{}, false, false, err
	}
	defer func() { _ = tx.Rollback() }()

	hash := evidenceHash(input)

	result, found, err := existingResult[application.EvidenceAttached](ctx, tx,
		"attach_evidence", input.OperationID, hash)
	if err != nil || found {
		return result, found, false, err
	}

	var pendingHash, pendingScope string

	err = tx.QueryRowContext(ctx, `SELECT scope, request_hash FROM operation_receipts
WHERE scope IN ('evidence_pending', 'evidence_corrupt') AND operation_id = ?`, input.OperationID).
		Scan(&pendingScope, &pendingHash)
	if errors.Is(err, sql.ErrNoRows) {
		return application.MutationResult[application.EvidenceAttached]{}, false, false, nil
	}

	if err != nil {
		return application.MutationResult[application.EvidenceAttached]{}, false, false, err
	}

	if pendingHash != hash {
		return application.MutationResult[application.EvidenceAttached]{}, false, false,
			&shared.Error{Code: "operation_id_conflict", Message: "operationIdが別の入力で使用されています"}
	}

	if pendingScope == "evidence_corrupt" {
		return application.MutationResult[application.EvidenceAttached]{}, false, false,
			&shared.Error{Code: "evidence_corrupt", Message: "画像証跡を確定できませんでした"}
	}

	return application.MutationResult[application.EvidenceAttached]{}, false, true, nil
}

func (r *ExecutionRepository) AttachTextEvidence(ctx context.Context,
	record application.EvidenceRecord,
) (application.MutationResult[application.EvidenceAttached], error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return application.MutationResult[application.EvidenceAttached]{}, err
	}
	defer func() { _ = tx.Rollback() }()

	hash := evidenceHash(record.Input)
	if result, found, err := existingResult[application.EvidenceAttached](ctx, tx,
		"attach_evidence", record.Input.OperationID, hash); err != nil || found {
		return result, err
	}

	if record.Input.Kind != "text" || strings.TrimSpace(record.Input.Text) == "" || record.Input.SourcePath != "" {
		return application.MutationResult[application.EvidenceAttached]{}, &shared.Error{
			Code: "validation_failed", Message: "text証跡の入力が不正です",
		}
	}

	if err := validateEvidenceTarget(ctx, tx, record.Input); err != nil {
		return application.MutationResult[application.EvidenceAttached]{}, err
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO execution_evidence
(evidence_id, execution_id, check_id, actor, kind, text, created_at)
VALUES (?, ?, ?, 'human', 'text', ?, ?)`, record.EvidenceID, record.Input.ExecutionID,
		record.Input.CheckID, record.Input.Text, record.At.Format(time.RFC3339Nano)); err != nil {
		return application.MutationResult[application.EvidenceAttached]{}, err
	}

	result, sequence, err := evidenceResult(ctx, tx, record.Input.ExecutionID, record.EvidenceID,
		record.Input.OperationID, record.At)
	if err != nil {
		return application.MutationResult[application.EvidenceAttached]{}, err
	}

	if err := saveMutation(ctx, tx, "attach_evidence", record.Input.OperationID, hash,
		result, record.Event, sequence); err != nil {
		return application.MutationResult[application.EvidenceAttached]{}, err
	}

	if err := tx.Commit(); err != nil {
		return application.MutationResult[application.EvidenceAttached]{}, err
	}

	return result, nil
}

func (r *ExecutionRepository) BeginImageEvidence(ctx context.Context, record application.EvidenceRecord) error {
	if record.Staged == nil || !validStaged(*record.Staged) ||
		(record.Staged.MIME != "image/png" && record.Staged.MIME != "image/jpeg") ||
		record.Input.Kind != "image" || record.Input.Text != "" || record.Input.SourcePath == "" {
		return &shared.Error{Code: "validation_failed", Message: "image証跡の入力が不正です"}
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	defer func() { _ = tx.Rollback() }()

	if err := validateEvidenceTarget(ctx, tx, record.Input); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO evidence_blobs
(hash, status, size, mime, relative_path, staging_name, ref_count)
VALUES (?, 'pending', ?, ?, ?, ?, 1)
ON CONFLICT(hash) DO UPDATE SET
ref_count = evidence_blobs.ref_count + 1`, record.Staged.Hash, record.Staged.Size, record.Staged.MIME,
		blobPath(record.Staged.Hash), record.Staged.StagingName); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO execution_evidence
(evidence_id, execution_id, check_id, actor, kind, display_name, blob_hash, staging_name, created_at)
VALUES (?, ?, ?, 'human', 'image', ?, ?, ?, ?)`, record.EvidenceID, record.Input.ExecutionID,
		record.Input.CheckID, record.Input.DisplayName, record.Staged.Hash, record.Staged.StagingName,
		record.At.Format(time.RFC3339Nano)); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO evidence_records
(evidence_id, execution_id, blob_hash, actor, description, created_at_us)
VALUES (?, ?, ?, 'human', ?, ?)`, record.EvidenceID, record.Input.ExecutionID,
		record.Staged.Hash, record.Input.DisplayName, record.At.UnixMicro()); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO check_evidence(check_id, evidence_id) VALUES (?, ?)`,
		record.Input.CheckID, record.EvidenceID); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO operation_receipts
(scope, operation_id, request_hash, result_json, committed_at)
VALUES ('evidence_pending', ?, ?, ?, ?)`, record.Input.OperationID, evidenceHash(record.Input),
		record.EvidenceID, record.At.Format(time.RFC3339Nano)); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *ExecutionRepository) CompleteImageEvidence(ctx context.Context, evidenceID string,
	available bool, at time.Time,
) (application.MutationResult[application.EvidenceAttached], error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return application.MutationResult[application.EvidenceAttached]{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var executionID, checkID, blobHash, operationID, hash string

	err = tx.QueryRowContext(ctx, `SELECT v.execution_id, v.check_id, v.blob_hash, o.operation_id, o.request_hash
FROM execution_evidence v JOIN operation_receipts o ON o.result_json = v.evidence_id
AND o.scope = 'evidence_pending' WHERE v.evidence_id = ?`, evidenceID).
		Scan(&executionID, &checkID, &blobHash, &operationID, &hash)
	if err != nil {
		return application.MutationResult[application.EvidenceAttached]{}, err
	}

	if !available {
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE evidence_blobs SET status = 'corrupt' WHERE hash = ?`,
			blobHash,
		); err != nil {
			return application.MutationResult[application.EvidenceAttached]{}, err
		}

		if _, err := tx.ExecContext(ctx, `UPDATE operation_receipts SET scope = 'evidence_corrupt'
WHERE scope = 'evidence_pending' AND operation_id = ?`, operationID); err != nil {
			return application.MutationResult[application.EvidenceAttached]{}, err
		}

		return application.MutationResult[application.EvidenceAttached]{}, tx.Commit()
	}

	if _, err := tx.ExecContext(
		ctx,
		`UPDATE evidence_blobs SET status = 'available' WHERE hash = ?`,
		blobHash,
	); err != nil {
		return application.MutationResult[application.EvidenceAttached]{}, err
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM operation_receipts WHERE scope = 'evidence_pending'
AND operation_id = ?`, operationID); err != nil {
		return application.MutationResult[application.EvidenceAttached]{}, err
	}

	result, sequence, err := evidenceResult(ctx, tx, executionID, evidenceID, operationID, at)
	if err != nil {
		return application.MutationResult[application.EvidenceAttached]{}, err
	}

	if err := saveMutation(ctx, tx, "attach_evidence", operationID, hash, result,
		application.OutboxEvent{
			ID: evidenceID + ":available", Name: "execution.updated", EmittedAt: at,
			AggregateType: "execution", AggregateID: executionID, Correlation: operationID,
		}, sequence); err != nil {
		return application.MutationResult[application.EvidenceAttached]{}, err
	}

	if err := tx.Commit(); err != nil {
		return application.MutationResult[application.EvidenceAttached]{}, err
	}

	return result, nil
}

func (r *ExecutionRepository) PendingImageEvidence(ctx context.Context) ([]application.PendingEvidence, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT v.evidence_id, b.hash, v.staging_name,
b.mime, b.size FROM execution_evidence v JOIN evidence_blobs b ON b.hash = v.blob_hash
JOIN operation_receipts o ON o.scope = 'evidence_pending' AND o.result_json = v.evidence_id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	items := []application.PendingEvidence{}

	for rows.Next() {
		var item application.PendingEvidence
		if err := rows.Scan(&item.EvidenceID, &item.Staged.Hash, &item.Staged.StagingName,
			&item.Staged.MIME, &item.Staged.Size); err != nil {
			return nil, err
		}

		items = append(items, item)
	}

	return items, rows.Err()
}

func validateEvidenceTarget(ctx context.Context, tx *sql.Tx, input application.AttachHumanEvidenceInput) error {
	var (
		revision            int64
		status, requirement string
	)

	err := tx.QueryRowContext(ctx, `SELECT e.revision, e.status, c.human_evidence_requirement
FROM executions e JOIN execution_checks c ON c.execution_id = e.execution_id
WHERE e.execution_id = ? AND c.check_id = ?`, input.ExecutionID, input.CheckID).
		Scan(&revision, &status, &requirement)
	if errors.Is(err, sql.ErrNoRows) {
		return &shared.Error{Code: "not_found", Message: "checkがありません"}
	}

	if err != nil {
		return err
	}

	if revision != input.ExpectedRevision {
		return &shared.Error{
			Code: "revision_conflict", Message: "executionが更新されています",
			CurrentRevision: &revision,
		}
	}

	if status != "active" || (requirement != "none" && requirement != input.Kind) {
		return &shared.Error{Code: "invalid_state", Message: "このcheckに証跡を追加できません"}
	}

	return nil
}

func evidenceResult(ctx context.Context, tx *sql.Tx, executionID, evidenceID, operationID string,
	at time.Time,
) (application.MutationResult[application.EvidenceAttached], int64, error) {
	var revision, sequence int64
	if err := tx.QueryRowContext(ctx, `UPDATE executions SET revision = revision + 1
WHERE execution_id = ? RETURNING revision`, executionID).Scan(&revision); err != nil {
		return application.MutationResult[application.EvidenceAttached]{}, 0, err
	}

	if err := tx.QueryRowContext(ctx, `UPDATE app_settings SET change_sequence = change_sequence + 1
WHERE singleton = 1 RETURNING change_sequence`).Scan(&sequence); err != nil {
		return application.MutationResult[application.EvidenceAttached]{}, 0, err
	}

	var projectID string
	if err := tx.QueryRowContext(ctx, `SELECT project_id FROM executions WHERE execution_id = ?`, executionID).
		Scan(&projectID); err != nil {
		return application.MutationResult[application.EvidenceAttached]{}, 0, err
	}

	snapshot, err := readExecution(ctx, tx, projectID, executionID, 1)
	if err != nil {
		return application.MutationResult[application.EvidenceAttached]{}, 0, err
	}

	var evidence execution.Evidence

	for _, check := range snapshot.Checks {
		for _, item := range check.Evidence {
			if item.ID == evidenceID {
				evidence = item
				break
			}
		}
	}

	if evidence.ID == "" {
		return application.MutationResult[application.EvidenceAttached]{}, 0, fmt.Errorf(
			"evidence %s missing",
			evidenceID,
		)
	}

	return application.MutationResult[application.EvidenceAttached]{
		Data: application.EvidenceAttached{
			ExecutionID: executionID, CheckID: evidence.CheckID,
			Evidence: application.EvidenceSummaryFromRecord(evidence), ExecutionRevision: revision,
			CanGenerate: snapshot.CanGenerate, BlockingReasons: snapshot.BlockingReasons,
		},
		Receipt: application.MutationReceipt{OperationID: operationID, CommittedAt: at},
	}, sequence, nil
}

func evidenceHash(input application.AttachHumanEvidenceInput) string {
	return requestHash(input.ExecutionID, input.CheckID, input.Kind, input.Text,
		input.SourcePath, input.DisplayName, fmt.Sprint(input.ExpectedRevision))
}
