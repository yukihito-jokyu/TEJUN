package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/yukihito-jokyu/TEJUN/internal/application"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/shared"
)

func (r *ExecutionRepository) AcceptExecutionJob(ctx context.Context,
	record application.ExecutionJobRecord,
) (application.MutationResult[application.ExecutionJobAccepted], error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return application.MutationResult[application.ExecutionJobAccepted]{}, err
	}
	defer func() { _ = tx.Rollback() }()

	scope := "execution_job:" + record.Kind

	hash := requestHash(record.ExecutionID, record.PromptText, strings.Join(record.CheckIDs, "\x00"),
		fmt.Sprint(record.ExpectedRevision))
	if result, found, err := existingResult[application.ExecutionJobAccepted](ctx, tx,
		scope, record.OperationID, hash); err != nil || found {
		return result, err
	}

	var (
		sessionID, status, activeRunID string
		revision                       int64
	)

	err = tx.QueryRowContext(ctx, `SELECT session_id, status, COALESCE(active_run_id, ''), revision
FROM executions WHERE execution_id = ?`, record.ExecutionID).
		Scan(&sessionID, &status, &activeRunID, &revision)
	if errors.Is(err, sql.ErrNoRows) {
		return application.MutationResult[application.ExecutionJobAccepted]{}, &shared.Error{
			Code: "not_found", Message: "executionがありません",
		}
	}

	if err != nil {
		return application.MutationResult[application.ExecutionJobAccepted]{}, err
	}

	if status != "active" || activeRunID != "" {
		return application.MutationResult[application.ExecutionJobAccepted]{}, &shared.Error{
			Code: "invalid_state", Message: "executionは実行できません",
		}
	}

	if record.Kind == "checks" && revision != record.ExpectedRevision {
		return application.MutationResult[application.ExecutionJobAccepted]{}, &shared.Error{
			Code: "revision_conflict", Message: "executionが更新されています", CurrentRevision: &revision,
		}
	}

	if record.Kind == "message" && strings.TrimSpace(record.PromptText) == "" {
		return application.MutationResult[application.ExecutionJobAccepted]{}, &shared.Error{
			Code: "validation_failed", Message: "空のmessageです",
		}
	}

	targets := []string{}

	if record.Kind == "checks" {
		selected := make(map[string]bool, len(record.CheckIDs))
		for _, id := range record.CheckIDs {
			if id == "" || selected[id] {
				return application.MutationResult[application.ExecutionJobAccepted]{}, &shared.Error{
					Code: "validation_failed", Message: "checkIdsが不正です",
				}
			}

			selected[id] = true
		}

		rows, err := tx.QueryContext(ctx, `SELECT check_id, ai_status FROM execution_checks
WHERE execution_id = ? ORDER BY sequence`, record.ExecutionID)
		if err != nil {
			return application.MutationResult[application.ExecutionJobAccepted]{}, err
		}

		for rows.Next() {
			var id, aiStatus string
			if err := rows.Scan(&id, &aiStatus); err != nil {
				_ = rows.Close()
				return application.MutationResult[application.ExecutionJobAccepted]{}, err
			}

			if (len(selected) == 0 && (aiStatus == "pending" || aiStatus == "failed")) || selected[id] {
				if aiStatus != "pending" && aiStatus != "failed" {
					_ = rows.Close()

					return application.MutationResult[application.ExecutionJobAccepted]{}, &shared.Error{
						Code: "invalid_state", Message: "pendingではないcheckがあります",
					}
				}

				targets = append(targets, id)
				delete(selected, id)
			}
		}

		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return application.MutationResult[application.ExecutionJobAccepted]{}, err
		}

		_ = rows.Close()

		if len(selected) > 0 || len(targets) == 0 {
			return application.MutationResult[application.ExecutionJobAccepted]{}, &shared.Error{
				Code: "validation_failed", Message: "対象checkがありません",
			}
		}
	}

	if record.Kind == "checks" {
		if _, err := tx.ExecContext(ctx, `INSERT INTO execution_runs
(run_id, execution_id, session_id, job_id, state, accepted_at) VALUES (?, ?, ?, ?, 'queued', ?)`,
			record.RunID, record.ExecutionID, sessionID, record.JobID,
			record.AcceptedAt.Format(time.RFC3339Nano)); err != nil {
			return application.MutationResult[application.ExecutionJobAccepted]{}, err
		}

		for _, id := range targets {
			if _, err := tx.ExecContext(ctx, `INSERT INTO execution_run_checks (run_id, check_id) VALUES (?, ?)`,
				record.RunID, id); err != nil {
				return application.MutationResult[application.ExecutionJobAccepted]{}, err
			}

			if _, err := tx.ExecContext(
				ctx,
				`UPDATE execution_checks SET ai_status = 'queued', ai_checked_at = NULL, ai_failure_summary = ''
WHERE execution_id = ? AND check_id = ?`,
				record.ExecutionID,
				id,
			); err != nil {
				return application.MutationResult[application.ExecutionJobAccepted]{}, err
			}
		}

		if _, err := tx.ExecContext(ctx, `UPDATE executions SET active_run_id = ? WHERE execution_id = ?`,
			record.RunID, record.ExecutionID); err != nil {
			return application.MutationResult[application.ExecutionJobAccepted]{}, err
		}
	} else if _, err := tx.ExecContext(ctx, `INSERT INTO execution_turns
(turn_id, execution_id, role, text, status, created_at) VALUES (?, ?, 'user', ?, 'queued', ?)`,
		record.TurnID, record.ExecutionID, record.PromptText, record.AcceptedAt.Format(time.RFC3339Nano)); err != nil {
		return application.MutationResult[application.ExecutionJobAccepted]{}, err
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO execution_agent_jobs
(job_id, execution_id, run_id, turn_id, session_id, kind, state, prompt_text, accepted_at)
VALUES (?, ?, ?, ?, ?, ?, 'queued', ?, ?)`, record.JobID, record.ExecutionID,
		nullIfEmpty(record.RunID), nullIfEmpty(record.TurnID), sessionID, record.Kind,
		record.PromptText, record.AcceptedAt.Format(time.RFC3339Nano)); err != nil {
		return application.MutationResult[application.ExecutionJobAccepted]{}, err
	}

	var sequence int64
	if err := tx.QueryRowContext(ctx, `UPDATE app_settings SET change_sequence = change_sequence + 1
WHERE singleton = 1 RETURNING change_sequence`).Scan(&sequence); err != nil {
		return application.MutationResult[application.ExecutionJobAccepted]{}, err
	}

	result := application.MutationResult[application.ExecutionJobAccepted]{
		Data: application.ExecutionJobAccepted{
			ExecutionID: record.ExecutionID, RunID: record.RunID,
			TurnID: record.TurnID, SessionID: sessionID, JobID: record.JobID,
			AcceptedAt: record.AcceptedAt, TargetedCheckIDs: targets,
		},
		Receipt: application.MutationReceipt{OperationID: record.OperationID, CommittedAt: record.AcceptedAt},
	}
	if err := saveMutation(ctx, tx, scope, record.OperationID, hash, result, record.Event, sequence); err != nil {
		return application.MutationResult[application.ExecutionJobAccepted]{}, err
	}

	if err := tx.Commit(); err != nil {
		return application.MutationResult[application.ExecutionJobAccepted]{}, err
	}

	return result, nil
}

func (r *ExecutionRepository) ClaimExecutionJob(ctx context.Context) (*application.ClaimedExecutionJob, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var (
		job           application.ClaimedExecutionJob
		runID, turnID sql.NullString
	)

	err = tx.QueryRowContext(ctx, `UPDATE execution_agent_jobs SET state = 'running'
WHERE job_id = (SELECT job_id FROM execution_agent_jobs WHERE state = 'queued'
ORDER BY accepted_at, job_id LIMIT 1)
RETURNING job_id, execution_id, run_id, turn_id, session_id, kind, prompt_text`).
		Scan(&job.JobID, &job.ExecutionID, &runID, &turnID, &job.SessionID, &job.Kind, &job.PromptText)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	job.RunID, job.TurnID = runID.String, turnID.String

	var argsJSON string
	if err := tx.QueryRowContext(ctx, `SELECT p.workspace_path,c.command,c.args_json,c.transport
FROM executions e JOIN projects p ON p.project_id=e.project_id
JOIN agent_connections c ON c.connection_id=p.connection_id WHERE e.execution_id=?`, job.ExecutionID).
		Scan(&job.WorkspacePath, &job.Connection.Command, &argsJSON, &job.Connection.Transport); err != nil {
		return nil, err
	}

	if err := json.Unmarshal([]byte(argsJSON), &job.Connection.Args); err != nil {
		return nil, err
	}

	job.CheckIDs = []string{}
	if job.RunID != "" {
		rows, err := tx.QueryContext(
			ctx,
			`SELECT c.check_id, c.sequence, c.title, c.instruction, c.expected_result, c.suggested_command
FROM execution_run_checks rc JOIN execution_checks c ON c.check_id = rc.check_id AND c.execution_id = ?
WHERE rc.run_id = ? ORDER BY c.sequence`,
			job.ExecutionID,
			job.RunID,
		)
		if err != nil {
			return nil, err
		}

		for rows.Next() {
			var target application.ExecutionCheckTarget
			if err := rows.Scan(&target.CheckID, &target.Sequence, &target.Title,
				&target.Instruction, &target.ExpectedResult, &target.SuggestedCommand); err != nil {
				_ = rows.Close()
				return nil, err
			}

			job.CheckIDs = append(job.CheckIDs, target.CheckID)
			job.Checks = append(job.Checks, target)
		}

		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, err
		}

		_ = rows.Close()

		if _, err := tx.ExecContext(
			ctx,
			`UPDATE execution_runs SET state = 'running' WHERE run_id = ?`,
			job.RunID,
		); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &job, nil
}

func (r *ExecutionRepository) StartExecutionCheck(ctx context.Context,
	job application.ClaimedExecutionJob, check application.ExecutionCheckTarget, at time.Time,
) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx, `UPDATE execution_checks SET ai_status='running'
WHERE execution_id=? AND check_id=? AND ai_status='queued'
AND EXISTS (SELECT 1 FROM execution_agent_jobs WHERE job_id=? AND state='running')`,
		job.ExecutionID, check.CheckID, job.JobID)
	if err != nil {
		return err
	}

	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		return &shared.Error{Code: "invalid_state", Message: "実行対象のチェックがありません"}
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO execution_turns
(turn_id,execution_id,role,text,status,created_at) VALUES(?,?,'system',?,'completed',?)`,
		job.JobID+":"+check.CheckID+":start", job.ExecutionID,
		fmt.Sprintf("%d. %s のAIチェックを開始しました", check.Sequence, check.Title),
		at.Format(time.RFC3339Nano)); err != nil {
		return err
	}

	if _, err := tx.ExecContext(
		ctx,
		`UPDATE app_settings SET change_sequence=change_sequence+1 WHERE singleton=1`,
	); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *ExecutionRepository) CompleteExecutionCheck(ctx context.Context,
	job application.ClaimedExecutionJob, check application.ExecutionCheckTarget,
	item application.ExecutionCheckResult, logs []string, at time.Time,
) error {
	if item.CheckID != check.CheckID || strings.TrimSpace(item.Evidence) == "" ||
		(item.Status != "completed" && item.Status != "failed") {
		return &shared.Error{Code: "validation_failed", Message: "AIチェックの結果が不正です"}
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	checkedAt := any(nil)
	failureSummary := ""
	statusLabel := "失敗"

	if item.Status == "completed" {
		checkedAt = at.Format(time.RFC3339Nano)
		statusLabel = "完了"
	} else {
		failureSummary = item.Evidence
	}

	result, err := tx.ExecContext(ctx, `UPDATE execution_checks
SET ai_status=?,ai_checked_at=?,ai_failure_summary=?
WHERE execution_id=? AND check_id=? AND ai_status='running'`,
		item.Status, checkedAt, failureSummary, job.ExecutionID, check.CheckID)
	if err != nil {
		return err
	}

	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		return &shared.Error{Code: "invalid_state", Message: "実行中のチェックがありません"}
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO execution_evidence
(evidence_id,execution_id,check_id,actor,kind,text,created_at)
VALUES(?,?,?,'ai','text',?,?)`, job.JobID+":"+check.CheckID, job.ExecutionID,
		check.CheckID, item.Evidence, at.Format(time.RFC3339Nano)); err != nil {
		return err
	}

	for i, log := range logs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO execution_turns
(turn_id,execution_id,role,text,status,created_at) VALUES(?,?,'system',?,'completed',?)`,
			fmt.Sprintf("%s:%s:tool:%d", job.JobID, check.CheckID, i), job.ExecutionID,
			log, at.Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO execution_turns
(turn_id,execution_id,role,text,status,created_at) VALUES(?,?,'agent',?,'completed',?)`,
		job.JobID+":"+check.CheckID+":result", job.ExecutionID,
		fmt.Sprintf("%d. %s: %s。証跡: %s", check.Sequence, check.Title, statusLabel, item.Evidence),
		at.Format(time.RFC3339Nano)); err != nil {
		return err
	}

	if _, err := tx.ExecContext(
		ctx,
		`UPDATE app_settings SET change_sequence=change_sequence+1 WHERE singleton=1`,
	); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *ExecutionRepository) ReconnectExecutionSession(ctx context.Context,
	job application.ClaimedExecutionJob, sessionID, agentSessionID string, at time.Time,
) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var projectID, currentSessionID string
	if err := tx.QueryRowContext(ctx, `SELECT e.project_id,COALESCE(p.current_session_id,e.session_id) FROM executions e
JOIN projects p ON p.project_id=e.project_id WHERE e.execution_id=? AND e.session_id=? AND COALESCE(e.active_run_id,'')=?`,
		job.ExecutionID, job.SessionID, job.RunID).Scan(&projectID, &currentSessionID); err != nil {
		return err
	}

	if currentSessionID != job.SessionID {
		return &shared.Error{Code: "invalid_state", Message: "現在のAgent sessionではありません"}
	}

	timestamp := at.Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(
		ctx,
		`UPDATE preparation_sessions SET state='interrupted',disconnected_at=?,revision=revision+1
WHERE session_id=?`,
		timestamp,
		job.SessionID,
	); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO preparation_sessions
(session_id,agent_session_id,previous_session_id,project_id,state,started_at)
VALUES (?,?,?,?,'ready',?)`, sessionID, agentSessionID, job.SessionID, projectID, timestamp); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `UPDATE projects SET current_session_id=?,revision=revision+1,updated_at=?
WHERE project_id=?`, sessionID, at.UnixMicro(), projectID); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `UPDATE executions SET session_id=?,revision=revision+1 WHERE execution_id=?`,
		sessionID, job.ExecutionID); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `UPDATE execution_agent_jobs SET session_id=? WHERE job_id=? AND state='running'`,
		sessionID, job.JobID); err != nil {
		return err
	}

	if job.RunID != "" {
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE execution_runs SET session_id=? WHERE run_id=?`,
			sessionID,
			job.RunID,
		); err != nil {
			return err
		}
	}

	if _, err := tx.ExecContext(ctx, `UPDATE execution_permissions SET state='expired'
WHERE session_id=? AND state IN ('pending','responding')`, job.SessionID); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *ExecutionRepository) CompleteExecutionJob(ctx context.Context,
	job application.ClaimedExecutionJob, output application.ExecutionJobResult, executeErr error, at time.Time,
) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var currentState string
	if err := tx.QueryRowContext(ctx, `SELECT state FROM execution_agent_jobs WHERE job_id = ?`,
		job.JobID).Scan(&currentState); err != nil {
		return err
	}

	if currentState != "running" && currentState != "cancellation_requested" {
		return &shared.Error{Code: "invalid_state", Message: "jobは既に終了しています"}
	}

	state := "failed"
	validResults := false

	if output.Cancelled {
		state = "cancelled"
	} else if executeErr == nil {
		state = "completed"
	} else if currentState == "cancellation_requested" {
		state = "interrupted"
	}

	if job.RunID != "" && state == "completed" {
		expected := make(map[string]bool, len(job.CheckIDs))

		rows, err := tx.QueryContext(ctx, `SELECT check_id FROM execution_run_checks WHERE run_id = ?`,
			job.RunID)
		if err != nil {
			return err
		}

		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				_ = rows.Close()
				return err
			}

			expected[id] = true
		}

		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return err
		}

		_ = rows.Close()

		if len(expected) == 0 || len(output.Checks) != len(expected) {
			state = "failed"
		} else {
			validResults = true

			for _, item := range output.Checks {
				if !expected[item.CheckID] || strings.TrimSpace(item.Evidence) == "" ||
					(item.Status != "completed" && item.Status != "failed") {
					state = "failed"
					validResults = false

					break
				}

				delete(expected, item.CheckID)

				if item.Status == "failed" {
					state = "failed"
				}
			}

			if len(expected) != 0 {
				state = "failed"
				validResults = false
			}
		}
	}

	result, err := tx.ExecContext(ctx, `UPDATE execution_agent_jobs SET state = ?, completed_at = ?
WHERE job_id = ? AND state IN ('running', 'cancellation_requested')`, state, at.Format(time.RFC3339Nano), job.JobID)
	if err != nil {
		return err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if affected == 0 {
		return &shared.Error{Code: "invalid_state", Message: "jobは既に終了しています"}
	}

	if job.RunID != "" {
		if _, err := tx.ExecContext(ctx, `UPDATE execution_runs SET state = ?, completed_at = ? WHERE run_id = ?`,
			state, at.Format(time.RFC3339Nano), job.RunID); err != nil {
			return err
		}

		if state == "cancelled" {
			if _, err := tx.ExecContext(ctx, `UPDATE execution_checks SET ai_status = 'pending'
WHERE execution_id = ? AND ai_status IN ('queued','running')
AND check_id IN (SELECT check_id FROM execution_run_checks WHERE run_id = ?)`,
				job.ExecutionID, job.RunID); err != nil {
				return err
			}
		} else if validResults && !output.Incremental {
			for _, item := range output.Checks {
				if item.Status != "completed" && item.Status != "failed" {
					continue
				}

				var matched int
				if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM execution_run_checks
WHERE run_id = ? AND check_id = ?`, job.RunID, item.CheckID).Scan(&matched); err != nil {
					return err
				}

				if matched != 1 {
					continue
				}

				checkedAt := any(nil)
				failureSummary := ""

				if item.Status == "completed" {
					checkedAt = at.Format(time.RFC3339Nano)
				} else {
					failureSummary = item.Evidence
				}

				if _, err := tx.ExecContext(
					ctx,
					`UPDATE execution_checks SET ai_status = ?, ai_checked_at = ?, ai_failure_summary = ?
WHERE execution_id = ? AND check_id = ?`,
					item.Status,
					checkedAt,
					failureSummary,
					job.ExecutionID,
					item.CheckID,
				); err != nil {
					return err
				}

				if _, err := tx.ExecContext(ctx, `INSERT INTO execution_evidence
(evidence_id, execution_id, check_id, actor, kind, text, created_at)
VALUES (?, ?, ?, 'ai', 'text', ?, ?)`, job.JobID+":"+item.CheckID, job.ExecutionID,
					item.CheckID, item.Evidence, at.Format(time.RFC3339Nano)); err != nil {
					return err
				}
			}
		} else if !validResults {
			failureSummary := "AIチェックに失敗しました"

			var appErr *shared.Error
			if errors.As(executeErr, &appErr) {
				failureSummary = appErr.Message
			} else if executeErr != nil {
				failureSummary = executeErr.Error()
			}

			if _, err := tx.ExecContext(
				ctx,
				`UPDATE execution_checks SET ai_status = 'failed', ai_checked_at = NULL, ai_failure_summary = ?
WHERE execution_id = ? AND ai_status = 'running'
AND check_id IN (SELECT check_id FROM execution_run_checks WHERE run_id = ?)`,
				failureSummary,
				job.ExecutionID,
				job.RunID,
			); err != nil {
				return err
			}

			if _, err := tx.ExecContext(ctx, `UPDATE execution_checks SET ai_status='pending'
WHERE execution_id=? AND ai_status='queued'
AND check_id IN (SELECT check_id FROM execution_run_checks WHERE run_id=?)`,
				job.ExecutionID, job.RunID); err != nil {
				return err
			}

			if executeErr != nil {
				for i, log := range output.Logs {
					if _, err := tx.ExecContext(ctx, `INSERT INTO execution_turns
(turn_id,execution_id,role,text,status,created_at) VALUES(?,?,'system',?,'completed',?)`,
						fmt.Sprintf("%s:failed-log:%d", job.JobID, i), job.ExecutionID, log,
						at.Format(time.RFC3339Nano)); err != nil {
						return err
					}
				}

				if _, err := tx.ExecContext(ctx, `INSERT INTO execution_turns
(turn_id,execution_id,role,text,status,created_at) VALUES(?,?,'system',?,'failed',?)`,
					job.JobID+":error", job.ExecutionID, failureSummary,
					at.Format(time.RFC3339Nano)); err != nil {
					return err
				}
			}
		}

		if _, err := tx.ExecContext(ctx, `UPDATE executions SET active_run_id = NULL, revision = revision + 1
WHERE execution_id = ? AND active_run_id = ?`, job.ExecutionID, job.RunID); err != nil {
			return err
		}
	}

	if job.TurnID != "" {
		if _, err := tx.ExecContext(ctx, `UPDATE execution_turns SET status = ? WHERE turn_id = ?`, state,
			job.TurnID); err != nil {
			return err
		}
	}

	if output.Message != "" {
		if _, err := tx.ExecContext(ctx, `INSERT INTO execution_turns
(turn_id, execution_id, role, text, status, created_at) VALUES (?, ?, 'agent', ?, ?, ?)`,
			job.JobID+":response", job.ExecutionID, output.Message, state, at.Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}

	var sequence int64
	if err := tx.QueryRowContext(ctx, `UPDATE app_settings SET change_sequence = change_sequence + 1
WHERE singleton = 1 RETURNING change_sequence`).Scan(&sequence); err != nil {
		return err
	}

	if err := insertOutbox(ctx, tx, application.OutboxEvent{
		ID: job.JobID + ":" + state, Name: "execution.updated", EmittedAt: at,
		AggregateType: "execution", AggregateID: job.ExecutionID,
	}, sequence); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *ExecutionRepository) CancelExecutionJob(ctx context.Context,
	jobID, operationID string, at time.Time,
) (bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	scope, hash := "execution_job:cancel", requestHash(jobID)
	if _, found, err := existingResult[bool](ctx, tx, scope, operationID, hash); err != nil || found {
		return false, err
	}

	var executionID, runID, state string

	err = tx.QueryRowContext(ctx, `SELECT execution_id, COALESCE(run_id, ''), state
FROM execution_agent_jobs WHERE job_id = ?`, jobID).Scan(&executionID, &runID, &state)
	if err != nil {
		return false, err
	}

	if state != "queued" && state != "running" && state != "cancellation_requested" {
		return false, &shared.Error{Code: "invalid_state", Message: "jobは既に終了しています"}
	}

	running := state == "running"

	switch state {
	case "queued":
		if _, err := tx.ExecContext(ctx, `UPDATE execution_agent_jobs SET state = 'cancelled', completed_at = ?
WHERE job_id = ? AND state = 'queued'`, at.Format(time.RFC3339Nano), jobID); err != nil {
			return false, err
		}

		if runID != "" {
			if _, err := tx.ExecContext(ctx, `UPDATE execution_runs SET state = 'cancelled', completed_at = ?
WHERE run_id = ?`, at.Format(time.RFC3339Nano), runID); err != nil {
				return false, err
			}

			if _, err := tx.ExecContext(ctx, `UPDATE execution_checks SET ai_status = 'pending'
WHERE execution_id = ? AND check_id IN (SELECT check_id FROM execution_run_checks WHERE run_id = ?)`, executionID, runID); err != nil {
				return false, err
			}

			if _, err := tx.ExecContext(ctx, `UPDATE executions SET active_run_id = NULL, revision = revision + 1
WHERE execution_id = ? AND active_run_id = ?`, executionID, runID); err != nil {
				return false, err
			}
		}

		if _, err := tx.ExecContext(ctx, `UPDATE execution_turns SET status = 'cancelled'
WHERE turn_id = (SELECT turn_id FROM execution_agent_jobs WHERE job_id = ?)`, jobID); err != nil {
			return false, err
		}
	case "running":
		if _, err := tx.ExecContext(ctx, `UPDATE execution_agent_jobs SET state = 'cancellation_requested'
WHERE job_id = ? AND state = 'running'`, jobID); err != nil {
			return false, err
		}

		if runID != "" {
			if _, err := tx.ExecContext(ctx, `UPDATE execution_runs SET state = 'cancellation_requested'
WHERE run_id = ? AND state = 'running'`, runID); err != nil {
				return false, err
			}
		}
	}

	var sequence int64
	if err := tx.QueryRowContext(ctx, `UPDATE app_settings SET change_sequence = change_sequence + 1
WHERE singleton = 1 RETURNING change_sequence`).Scan(&sequence); err != nil {
		return false, err
	}

	result := application.MutationResult[bool]{
		Data: running, Receipt: application.MutationReceipt{OperationID: operationID, CommittedAt: at},
	}
	if err := saveMutation(ctx, tx, scope, operationID, hash, result, application.OutboxEvent{
		ID: jobID + ":cancel:" + operationID, Name: "execution.updated", EmittedAt: at,
		AggregateType: "execution", AggregateID: executionID, Correlation: operationID,
	}, sequence); err != nil {
		return false, err
	}

	return running, tx.Commit()
}

func (r *ExecutionRepository) FailInterruptedExecutionJobs(ctx context.Context, at time.Time) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT execution_id FROM execution_agent_jobs
WHERE state IN ('running', 'cancellation_requested')`)
	if err != nil {
		return err
	}

	var executionIDs []string

	for rows.Next() {
		var executionID string
		if err := rows.Scan(&executionID); err != nil {
			_ = rows.Close()
			return err
		}

		executionIDs = append(executionIDs, executionID)
	}

	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}

	if err := rows.Close(); err != nil {
		return err
	}

	if _, err := tx.ExecContext(
		ctx,
		`UPDATE execution_checks SET ai_status = 'failed', ai_checked_at = NULL, ai_failure_summary = 'AIチェックが中断されました'
WHERE (execution_id, check_id) IN (SELECT j.execution_id, rc.check_id FROM execution_run_checks rc
JOIN execution_agent_jobs j ON j.run_id = rc.run_id
WHERE j.state IN ('running', 'cancellation_requested'))`,
	); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `UPDATE execution_runs SET state =
CASE WHEN state = 'cancellation_requested' THEN 'interrupted' ELSE 'failed' END, completed_at = ?
WHERE run_id IN (SELECT run_id FROM execution_agent_jobs
WHERE state IN ('running', 'cancellation_requested'))`,
		at.Format(time.RFC3339Nano)); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `UPDATE executions SET active_run_id = NULL, revision = revision + 1
WHERE active_run_id IN (SELECT run_id FROM execution_agent_jobs
WHERE state IN ('running', 'cancellation_requested'))`); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `UPDATE execution_agent_jobs SET state =
CASE WHEN state = 'cancellation_requested' THEN 'interrupted' ELSE 'failed' END, completed_at = ?
WHERE state IN ('running', 'cancellation_requested')`, at.Format(time.RFC3339Nano)); err != nil {
		return err
	}

	for _, executionID := range executionIDs {
		var sequence int64
		if err := tx.QueryRowContext(ctx, `UPDATE app_settings SET change_sequence = change_sequence + 1
WHERE singleton = 1 RETURNING change_sequence`).Scan(&sequence); err != nil {
			return err
		}

		if err := insertOutbox(ctx, tx, application.OutboxEvent{
			ID:   "execution:" + executionID + ":recovered:" + at.Format(time.RFC3339Nano),
			Name: "execution.updated", EmittedAt: at, AggregateType: "execution", AggregateID: executionID,
		}, sequence); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func nullIfEmpty(value string) any {
	if value == "" {
		return nil
	}

	return value
}

func (r *ExecutionRepository) AcceptExecutionCancellation(ctx context.Context,
	in application.CancelAgentOperationRecord,
) (application.MutationResult[application.CancellationAccepted], bool, string, error) {
	var zero application.MutationResult[application.CancellationAccepted]
	if in.SessionID == "" || in.OperationID == "" || (in.RunID == "" && in.TurnID == "" && in.JobID == "") {
		return zero, false, "", &shared.Error{Code: "validation_error", Message: "取消対象が必要です"}
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return zero, false, "", err
	}
	defer func() { _ = tx.Rollback() }()

	const scope = "cancel_agent_operation"

	hash := digest(in.CancelAgentOperationInput)
	if result, found, err := existingResult[application.CancellationAccepted](
		ctx,
		tx,
		scope,
		in.OperationID,
		hash,
	); err != nil ||
		found {
		return result, false, "", err
	}

	var jobID, executionID, sessionID, runID, turnID, state string

	err = tx.QueryRowContext(ctx, `SELECT job_id, execution_id, session_id, COALESCE(run_id, ''), COALESCE(turn_id, ''), state
FROM execution_agent_jobs WHERE (? = '' OR job_id = ?) AND (? = '' OR run_id = ?) AND (? = '' OR turn_id = ?)
ORDER BY accepted_at DESC LIMIT 1`, in.JobID, in.JobID, in.RunID, in.RunID, in.TurnID, in.TurnID).
		Scan(&jobID, &executionID, &sessionID, &runID, &turnID, &state)
	if err != nil {
		return zero, false, "", notFound(err)
	}

	if sessionID != in.SessionID {
		return zero, false, "", &shared.Error{Code: "validation_error", Message: "取消対象のsessionが一致しません"}
	}

	if state != "queued" && state != "running" && state != "cancellation_requested" {
		return zero, false, "", &shared.Error{Code: "invalid_state", Message: "取消対象は既に完了しています"}
	}

	queued, running := state == "queued", state == "running"
	targetStatus := "cancellation_requested"

	mechanism := "rpc_cancel"
	if runID != "" || turnID != "" {
		mechanism = "session_cancel"
	}

	if queued {
		targetStatus, mechanism = "cancelled", "local_cancel"
	}

	if state != "cancellation_requested" {
		if queued {
			_, err = tx.ExecContext(
				ctx,
				`UPDATE execution_agent_jobs SET state = 'cancelled', completed_at = ? WHERE job_id = ? AND state = 'queued'`,
				in.RequestedAt.Format(time.RFC3339Nano),
				jobID,
			)
		} else {
			_, err = tx.ExecContext(
				ctx,
				`UPDATE execution_agent_jobs SET state = 'cancellation_requested' WHERE job_id = ? AND state = 'running'`,
				jobID,
			)
		}

		if err != nil {
			return zero, false, "", err
		}

		if runID != "" {
			if queued {
				_, err = tx.ExecContext(
					ctx,
					`UPDATE execution_runs SET state = 'cancelled', completed_at = ? WHERE run_id = ?`,
					in.RequestedAt.Format(time.RFC3339Nano),
					runID,
				)
			} else {
				_, err = tx.ExecContext(
					ctx,
					`UPDATE execution_runs SET state = 'cancellation_requested' WHERE run_id = ? AND state = 'running'`,
					runID,
				)
			}

			if err != nil {
				return zero, false, "", err
			}

			if queued {
				if _, err = tx.ExecContext(
					ctx,
					`UPDATE execution_checks SET ai_status = 'pending' WHERE execution_id = ? AND check_id IN (SELECT check_id FROM execution_run_checks WHERE run_id = ?)`,
					executionID,
					runID,
				); err != nil {
					return zero, false, "", err
				}

				if _, err = tx.ExecContext(
					ctx,
					`UPDATE executions SET active_run_id = NULL, revision = revision + 1 WHERE execution_id = ? AND active_run_id = ?`,
					executionID,
					runID,
				); err != nil {
					return zero, false, "", err
				}
			}
		}

		if queued && turnID != "" {
			if _, err = tx.ExecContext(
				ctx,
				`UPDATE execution_turns SET status = 'cancelled' WHERE turn_id = ?`,
				turnID,
			); err != nil {
				return zero, false, "", err
			}
		}
	}

	result, err := mutation(ctx, tx, scope, in.OperationID, hash, application.CancellationAccepted{
		JobID: in.CancellationJobID, TargetStatus: targetStatus,
		RequestedAt: in.RequestedAt, Mechanism: mechanism,
	}, in.RequestedAt, application.OutboxEvent{
		ID: in.EventID, Name: "execution.updated", EmittedAt: in.RequestedAt,
		AggregateType: "execution", AggregateID: executionID, Correlation: in.OperationID,
	})

	return result, running, jobID, err
}
