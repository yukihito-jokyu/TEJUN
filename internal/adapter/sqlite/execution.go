package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/yukihito-jokyu/TEJUN/internal/application"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/execution"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/shared"
)

type ExecutionRepository struct{ db *sql.DB }

func NewExecutionRepository(db *sql.DB) *ExecutionRepository { return &ExecutionRepository{db: db} }

func (r *ExecutionRepository) GetExecution(
	ctx context.Context,
	projectID string,
	conversationLimit int,
) (execution.Snapshot, error) {
	if conversationLimit < 1 || conversationLimit > 100 {
		return execution.Snapshot{}, &shared.Error{Code: "validation_failed", Message: "conversationLimitが範囲外です"}
	}

	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return execution.Snapshot{}, err
	}
	defer func() { _ = tx.Rollback() }()

	snapshot, err := readExecution(ctx, tx, projectID, "", conversationLimit)
	if err != nil {
		return execution.Snapshot{}, err
	}

	if err := tx.Commit(); err != nil {
		return execution.Snapshot{}, err
	}

	return snapshot, nil
}

func readExecution(
	ctx context.Context,
	tx *sql.Tx,
	projectID, executionID string,
	limit int,
) (execution.Snapshot, error) {
	snapshot := execution.Snapshot{
		Checks: []execution.Check{}, Permissions: []execution.Permission{},
		Conversation: []execution.Turn{}, BlockingReasons: []string{},
	}

	var (
		startedAt                string
		completedAt, activeRunID sql.NullString
	)

	err := tx.QueryRowContext(ctx, `SELECT e.project_id, e.execution_id, e.status, COALESCE(e.session_id, ''), e.revision,
e.started_at, e.completed_at, e.active_run_id, s.change_sequence
FROM executions e CROSS JOIN app_settings s WHERE e.project_id = ? AND s.singleton = 1
AND (? = '' OR e.execution_id = ?) ORDER BY e.started_at DESC, e.execution_id DESC LIMIT 1`,
		projectID, executionID, executionID).
		Scan(&snapshot.ProjectID, &snapshot.ExecutionID, &snapshot.Status, &snapshot.SessionID,
			&snapshot.Revision, &startedAt, &completedAt, &activeRunID, &snapshot.ChangeSequence)
	if errors.Is(err, sql.ErrNoRows) {
		return execution.Snapshot{}, &shared.Error{Code: "not_found", Message: "executionがありません"}
	}

	if err != nil {
		return execution.Snapshot{}, err
	}

	if snapshot.StartedAt, err = time.Parse(time.RFC3339Nano, startedAt); err != nil {
		return execution.Snapshot{}, err
	}

	if completedAt.Valid {
		at, parseErr := time.Parse(time.RFC3339Nano, completedAt.String)
		if parseErr != nil {
			return execution.Snapshot{}, parseErr
		}

		snapshot.CompletedAt = &at
	}

	if activeRunID.Valid {
		snapshot.ActiveRunID = activeRunID.String
	}

	rows, err := tx.QueryContext(ctx, `SELECT check_id, sequence, title, instruction, expected_result,
ai_required, human_required, ai_status, human_status, human_evidence_requirement,
ai_checked_at, human_checked_at, ai_failure_summary FROM execution_checks
WHERE execution_id = ? ORDER BY sequence`, snapshot.ExecutionID)
	if err != nil {
		return execution.Snapshot{}, err
	}

	for rows.Next() {
		check := execution.Check{Evidence: []execution.Evidence{}}

		var aiCheckedAt, humanCheckedAt sql.NullString
		if err := rows.Scan(&check.ID, &check.Sequence, &check.Title, &check.Instruction,
			&check.ExpectedResult, &check.AIRequired, &check.HumanRequired, &check.AIStatus, &check.HumanStatus,
			&check.HumanEvidenceRequirement, &aiCheckedAt, &humanCheckedAt, &check.AIFailureSummary); err != nil {
			_ = rows.Close()
			return execution.Snapshot{}, err
		}

		if aiCheckedAt.Valid {
			at, err := time.Parse(time.RFC3339Nano, aiCheckedAt.String)
			if err != nil {
				_ = rows.Close()
				return execution.Snapshot{}, err
			}

			check.AICheckedAt = &at
		}

		if humanCheckedAt.Valid {
			at, err := time.Parse(time.RFC3339Nano, humanCheckedAt.String)
			if err != nil {
				_ = rows.Close()
				return execution.Snapshot{}, err
			}

			check.HumanCheckedAt = &at
		}

		snapshot.Checks = append(snapshot.Checks, check)
	}

	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return execution.Snapshot{}, err
	}

	_ = rows.Close()

	rows, err = tx.QueryContext(ctx, `SELECT permission_request_id, session_id, COALESCE(run_id, ''),
tool_call_id, title, options_json, state, requested_at, expires_at
FROM execution_permissions WHERE execution_id = ? AND state IN ('pending', 'responding', 'failed')
ORDER BY requested_at, permission_request_id`, snapshot.ExecutionID)
	if err != nil {
		return execution.Snapshot{}, err
	}

	for rows.Next() {
		var (
			permission               execution.Permission
			optionsJSON, requestedAt string
			expiresAt                sql.NullString
		)
		if err := rows.Scan(&permission.ID, &permission.SessionID, &permission.RunID, &permission.ToolCallID,
			&permission.Title, &optionsJSON, &permission.Status, &requestedAt, &expiresAt); err != nil {
			_ = rows.Close()
			return execution.Snapshot{}, err
		}

		if err := json.Unmarshal([]byte(optionsJSON), &permission.Options); err != nil {
			_ = rows.Close()
			return execution.Snapshot{}, err
		}

		if permission.RequestedAt, err = time.Parse(time.RFC3339Nano, requestedAt); err != nil {
			_ = rows.Close()
			return execution.Snapshot{}, err
		}

		if expiresAt.Valid {
			at, parseErr := time.Parse(time.RFC3339Nano, expiresAt.String)
			if parseErr != nil {
				_ = rows.Close()
				return execution.Snapshot{}, parseErr
			}

			permission.ExpiresAt = &at
		}

		snapshot.Permissions = append(snapshot.Permissions, permission)
	}

	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return execution.Snapshot{}, err
	}

	_ = rows.Close()

	rows, err = tx.QueryContext(ctx, `SELECT v.evidence_id, v.check_id, v.actor, v.kind, v.text,
v.display_name, COALESCE(v.blob_hash, ''), COALESCE(b.mime, ''), COALESCE(b.size, 0), CASE WHEN o.operation_id IS NOT NULL THEN 'pending' ELSE COALESCE(b.status, 'available') END, v.created_at
FROM execution_evidence v LEFT JOIN evidence_blobs b ON b.hash = v.blob_hash
LEFT JOIN operation_receipts o ON o.scope = 'evidence_pending' AND o.result_json = v.evidence_id
WHERE v.execution_id = ? ORDER BY v.created_at, v.evidence_id`, snapshot.ExecutionID)
	if err != nil {
		return execution.Snapshot{}, err
	}

	for rows.Next() {
		var (
			evidence  execution.Evidence
			createdAt string
		)
		if err := rows.Scan(
			&evidence.ID,
			&evidence.CheckID,
			&evidence.Actor,
			&evidence.Kind,
			&evidence.Text,
			&evidence.DisplayName,
			&evidence.BlobHash,
			&evidence.MimeType,
			&evidence.Size,
			&evidence.Status,
			&createdAt,
		); err != nil {
			_ = rows.Close()
			return execution.Snapshot{}, err
		}

		if evidence.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
			_ = rows.Close()
			return execution.Snapshot{}, err
		}

		for index := range snapshot.Checks {
			if snapshot.Checks[index].ID == evidence.CheckID {
				snapshot.Checks[index].Evidence = append(snapshot.Checks[index].Evidence, evidence)
				break
			}
		}
	}

	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return execution.Snapshot{}, err
	}

	_ = rows.Close()

	rows, err = tx.QueryContext(ctx, `SELECT turn_id, role, text, status, created_at FROM execution_turns
WHERE execution_id = ? ORDER BY created_at DESC, turn_id DESC LIMIT ?`, snapshot.ExecutionID, limit)
	if err != nil {
		return execution.Snapshot{}, err
	}

	for rows.Next() {
		var (
			turn      execution.Turn
			createdAt string
		)
		if err := rows.Scan(&turn.ID, &turn.Role, &turn.Text, &turn.Status, &createdAt); err != nil {
			_ = rows.Close()
			return execution.Snapshot{}, err
		}

		if turn.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
			_ = rows.Close()
			return execution.Snapshot{}, err
		}

		snapshot.Conversation = append(snapshot.Conversation, turn)
	}

	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return execution.Snapshot{}, err
	}

	_ = rows.Close()

	snapshot.CanGenerate, snapshot.BlockingReasons = snapshot.Readiness()

	return snapshot, nil
}

func (r *ExecutionRepository) SetHumanCheck(
	ctx context.Context,
	record application.HumanCheckRecord,
) (application.MutationResult[application.CheckUpdate], error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return application.MutationResult[application.CheckUpdate]{}, err
	}
	defer func() { _ = tx.Rollback() }()

	scope := "human_check"

	hash := requestHash(
		record.ExecutionID,
		record.CheckID,
		fmt.Sprint(record.Checked),
		fmt.Sprint(record.ExpectedRevision),
	)
	if result, found, err := existingResult[application.CheckUpdate](
		ctx,
		tx,
		scope,
		record.OperationID,
		hash,
	); err != nil ||
		found {
		return result, err
	}

	var projectID string
	if err := tx.QueryRowContext(ctx, `SELECT project_id FROM executions WHERE execution_id = ?`, record.ExecutionID).
		Scan(&projectID); err != nil {
		return application.MutationResult[application.CheckUpdate]{}, err
	}

	snapshot, err := readExecution(ctx, tx, projectID, record.ExecutionID, 1)
	if err != nil {
		return application.MutationResult[application.CheckUpdate]{}, err
	}

	if snapshot.Revision != record.ExpectedRevision {
		return application.MutationResult[application.CheckUpdate]{}, &shared.Error{
			Code: "revision_conflict", Message: "executionが更新されています", CurrentRevision: &snapshot.Revision,
		}
	}

	if snapshot.Status != "active" {
		return application.MutationResult[application.CheckUpdate]{}, &shared.Error{
			Code:    "invalid_state",
			Message: "executionは操作できません",
		}
	}

	var check *execution.Check

	for index := range snapshot.Checks {
		if snapshot.Checks[index].ID == record.CheckID {
			check = &snapshot.Checks[index]
			break
		}
	}

	if check == nil {
		return application.MutationResult[application.CheckUpdate]{}, &shared.Error{
			Code:    "not_found",
			Message: "checkがありません",
		}
	}

	if record.Checked {
		if check.AIRequired && check.AIStatus != "completed" {
			return application.MutationResult[application.CheckUpdate]{}, &shared.Error{
				Code:    "invalid_state",
				Message: "AIチェックが未完了です",
			}
		}

		if check.HumanRequired && check.HumanEvidenceRequirement != "none" {
			found := false

			for _, evidence := range check.Evidence {
				if evidence.Actor == "human" && evidence.Kind == check.HumanEvidenceRequirement &&
					evidence.Status == "available" {
					found = true
				}
			}

			if !found {
				return application.MutationResult[application.CheckUpdate]{}, &shared.Error{
					Code:    "evidence_required",
					Message: "必須証跡がありません",
				}
			}
		}
	} else {
		var procedureID string

		err := tx.QueryRowContext(ctx, `SELECT procedure_id FROM procedure_drafts WHERE execution_id = ?`, record.ExecutionID).
			Scan(&procedureID)
		if err == nil {
			return application.MutationResult[application.CheckUpdate]{}, &shared.Error{
				Code:    "invalid_state",
				Message: "手順書生成後は解除できません",
			}
		}

		if !errors.Is(err, sql.ErrNoRows) {
			return application.MutationResult[application.CheckUpdate]{}, err
		}
	}

	status := "pending"

	var checkedAt any

	if record.Checked {
		status = "completed"
		checkedAt = record.At.Format(time.RFC3339Nano)
	}

	if _, err := tx.ExecContext(
		ctx,
		`UPDATE execution_checks SET human_status = ?, human_checked_at = ? WHERE execution_id = ? AND check_id = ?`,
		status,
		checkedAt,
		record.ExecutionID,
		check.ID,
	); err != nil {
		return application.MutationResult[application.CheckUpdate]{}, err
	}

	check.HumanStatus = status

	check.HumanCheckedAt = nil
	if record.Checked {
		check.HumanCheckedAt = &record.At
	}

	if err := tx.QueryRowContext(ctx, `UPDATE executions SET revision = revision + 1 WHERE execution_id = ? RETURNING revision`,
		record.ExecutionID).
		Scan(&snapshot.Revision); err != nil {
		return application.MutationResult[application.CheckUpdate]{}, err
	}

	var sequence int64
	if err := tx.QueryRowContext(ctx, `UPDATE app_settings SET change_sequence = change_sequence + 1
WHERE singleton = 1 RETURNING change_sequence`).Scan(&sequence); err != nil {
		return application.MutationResult[application.CheckUpdate]{}, err
	}

	canGenerate, reasons := snapshot.Readiness()

	result := application.MutationResult[application.CheckUpdate]{
		Data: application.CheckUpdate{
			ExecutionID: record.ExecutionID, Check: application.ExecutionCheckFromSnapshot(*check),
			ExecutionRevision: snapshot.Revision, CanGenerate: canGenerate, BlockingReasons: reasons,
		},
		Receipt: application.MutationReceipt{OperationID: record.OperationID, CommittedAt: record.At},
	}
	if err := saveMutation(ctx, tx, scope, record.OperationID, hash, result, record.Event, sequence); err != nil {
		return application.MutationResult[application.CheckUpdate]{}, err
	}

	if err := tx.Commit(); err != nil {
		return application.MutationResult[application.CheckUpdate]{}, err
	}

	return result, nil
}

func (r *ExecutionRepository) GenerateProcedureDraft(
	ctx context.Context,
	record application.ProcedureDraftRecord,
) (application.MutationResult[application.ProcedureGenerationAccepted], error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return application.MutationResult[application.ProcedureGenerationAccepted]{}, err
	}
	defer func() { _ = tx.Rollback() }()

	scope := "generate_procedure_draft"

	hash := requestHash(record.ExecutionID, fmt.Sprint(record.ExpectedRevision))
	if result, found, err := existingResult[application.ProcedureGenerationAccepted](
		ctx,
		tx,
		scope,
		record.OperationID,
		hash,
	); err != nil ||
		found {
		return result, err
	}

	var projectID string
	if err := tx.QueryRowContext(ctx, `SELECT project_id FROM executions WHERE execution_id = ?`, record.ExecutionID).
		Scan(&projectID); err != nil {
		return application.MutationResult[application.ProcedureGenerationAccepted]{}, err
	}

	snapshot, err := readExecution(ctx, tx, projectID, record.ExecutionID, 1)
	if err != nil {
		return application.MutationResult[application.ProcedureGenerationAccepted]{}, err
	}

	if snapshot.Revision != record.ExpectedRevision {
		return application.MutationResult[application.ProcedureGenerationAccepted]{}, &shared.Error{
			Code: "revision_conflict", Message: "executionが更新されています", CurrentRevision: &snapshot.Revision,
		}
	}

	if !snapshot.CanGenerate {
		return application.MutationResult[application.ProcedureGenerationAccepted]{}, &shared.Error{
			Code: "invalid_state", Message: "必須チェックと証跡が揃っていません",
		}
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO procedure_drafts
(procedure_id, execution_id, source_revision, created_at) VALUES (?, ?, ?, ?)`, record.ProcedureID,
		record.ExecutionID, snapshot.Revision, record.At.Format(time.RFC3339Nano)); err != nil {
		return application.MutationResult[application.ProcedureGenerationAccepted]{}, err
	}

	if _, err := tx.ExecContext(
		ctx,
		`UPDATE executions SET status = 'completed', completed_at = ?, revision = revision + 1
WHERE execution_id = ?`,
		record.At.Format(time.RFC3339Nano),
		record.ExecutionID,
	); err != nil {
		return application.MutationResult[application.ProcedureGenerationAccepted]{}, err
	}

	var sequence int64
	if err := tx.QueryRowContext(ctx, `UPDATE app_settings SET change_sequence = change_sequence + 1
WHERE singleton = 1 RETURNING change_sequence`).Scan(&sequence); err != nil {
		return application.MutationResult[application.ProcedureGenerationAccepted]{}, err
	}

	result := application.MutationResult[application.ProcedureGenerationAccepted]{
		Data: application.ProcedureGenerationAccepted{
			ProcedureID: record.ProcedureID, NextRoute: "#/procedure/" + projectID, AcceptedAt: record.At,
		},
		Receipt: application.MutationReceipt{OperationID: record.OperationID, CommittedAt: record.At},
	}
	if err := saveMutation(ctx, tx, scope, record.OperationID, hash, result, record.Event, sequence); err != nil {
		return application.MutationResult[application.ProcedureGenerationAccepted]{}, err
	}

	if err := tx.Commit(); err != nil {
		return application.MutationResult[application.ProcedureGenerationAccepted]{}, err
	}

	return result, nil
}

func requestHash(parts ...string) string {
	hash := sha256.New()
	for _, part := range parts {
		_, _ = hash.Write([]byte(part))
		_, _ = hash.Write([]byte{0})
	}

	return hex.EncodeToString(hash.Sum(nil))
}

func (r *ExecutionRepository) ProjectIDForExecution(ctx context.Context, executionID string) (string, error) {
	var projectID string

	err := r.db.QueryRowContext(ctx, `SELECT project_id FROM executions WHERE execution_id=?`, executionID).
		Scan(&projectID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", &shared.Error{Code: "not_found", Message: "executionがありません"}
	}

	return projectID, err
}
