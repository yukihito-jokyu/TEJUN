package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/yukihito-jokyu/TEJUN/internal/application"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/shared"
	"github.com/yukihito-jokyu/TEJUN/internal/trace"
)

type ProjectExternalRepository struct{ db *sql.DB }

func NewProjectExternalRepository(db *sql.DB) *ProjectExternalRepository {
	return &ProjectExternalRepository{db: db}
}

func (r *ProjectExternalRepository) ProcedureExportable(ctx context.Context, id string, revision int64) (bool, error) {
	var found int

	err := r.db.QueryRowContext(ctx, `SELECT 1 FROM procedures WHERE procedure_id=? AND revision=? AND status='completed'`, id, revision).
		Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}

	return found == 1, err
}

func (r *ProjectExternalRepository) ExistingExport(
	ctx context.Context,
	operationID, hash string,
) (application.MutationResult[application.ExportAccepted], bool, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return application.MutationResult[application.ExportAccepted]{}, false, err
	}
	defer func() { _ = tx.Rollback() }()

	result, found, err := existingResult[application.ExportAccepted](ctx, tx, "export_procedure", operationID, hash)
	if err != nil {
		return result, found, err
	}

	if err := tx.Commit(); err != nil {
		return application.MutationResult[application.ExportAccepted]{}, false, err
	}

	return result, found, nil
}

func (r *ProjectExternalRepository) AcceptReconnect(
	ctx context.Context,
	record application.ProjectReconnectRecord,
) (application.MutationResult[application.SessionConnectionAccepted], error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return application.MutationResult[application.SessionConnectionAccepted]{}, err
	}
	defer func() { _ = tx.Rollback() }()

	if result, found, err := existingResult[application.SessionConnectionAccepted](
		ctx,
		tx,
		"reconnect_project_session",
		record.OperationID,
		record.RequestHash,
	); err != nil ||
		found {
		return result, err
	}

	var (
		revision int64
		status   string
	)
	if err := tx.QueryRowContext(ctx, `SELECT revision,status FROM projects WHERE project_id=?`, record.ProjectID).
		Scan(&revision, &status); err != nil {
		return application.MutationResult[application.SessionConnectionAccepted]{}, projectExternalNotFound(err)
	}

	if revision != record.ExpectedRevision {
		return application.MutationResult[application.SessionConnectionAccepted]{}, &shared.Error{
			Code:            "revision_conflict",
			Message:         "projectが更新されています",
			CurrentRevision: &revision,
		}
	}

	if status == "archived" {
		return application.MutationResult[application.SessionConnectionAccepted]{}, &shared.Error{
			Code:    "invalid_state",
			Message: "アーカイブ済みprojectは再接続できません",
		}
	}

	var active int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM background_jobs WHERE project_id=? AND state IN ('pending','running')`, record.ProjectID).
		Scan(&active); err != nil {
		return application.MutationResult[application.SessionConnectionAccepted]{}, err
	}

	if active != 0 {
		return application.MutationResult[application.SessionConnectionAccepted]{}, &shared.Error{
			Code:      "invalid_state",
			Message:   "処理中のjobがあります",
			Retryable: true,
		}
	}

	var (
		previousAgentID                sql.NullString
		resumeSupported, loadSupported int
	)

	err = tx.QueryRowContext(ctx, `SELECT agent_session_id,resume_supported,load_supported FROM acp_sessions WHERE project_id=? AND state='connected' ORDER BY created_at DESC LIMIT 1`, record.ProjectID).
		Scan(&previousAgentID, &resumeSupported, &loadSupported)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return application.MutationResult[application.SessionConnectionAccepted]{}, err
	}

	mode := "new"

	if record.Strategy == "resume_existing" {
		if !previousAgentID.Valid || resumeSupported == 0 {
			return application.MutationResult[application.SessionConnectionAccepted]{}, &shared.Error{
				Code:    "acp_not_compatible",
				Message: "再開できるsessionがありません",
			}
		}

		mode = "resume"
	} else if record.Strategy == "auto" && previousAgentID.Valid {
		if resumeSupported != 0 {
			mode = "resume"
		} else if loadSupported != 0 {
			mode = "load"
		}
	}

	acceptedAt := record.Receipt.CommittedAt.UTC().UnixMicro()
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO acp_sessions(session_id,project_id,requested_strategy,recovery_mode,state,created_at,updated_at) VALUES(?,?,?,?,'connecting',?,?)`,
		record.SessionID,
		record.ProjectID,
		record.Strategy,
		mode,
		acceptedAt,
		acceptedAt,
	); err != nil {
		return application.MutationResult[application.SessionConnectionAccepted]{}, err
	}

	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO background_jobs(job_id,project_id,kind,state,target_id,accepted_at) VALUES(?,?,'reconnect','pending',?,?)`,
		record.JobID,
		record.ProjectID,
		record.SessionID,
		acceptedAt,
	); err != nil {
		return application.MutationResult[application.SessionConnectionAccepted]{}, err
	}

	var sequence int64
	if err := tx.QueryRowContext(ctx, `UPDATE app_settings SET change_sequence=change_sequence+1 WHERE singleton=1 RETURNING change_sequence`).
		Scan(&sequence); err != nil {
		return application.MutationResult[application.SessionConnectionAccepted]{}, err
	}

	result := application.MutationResult[application.SessionConnectionAccepted]{
		Data: application.SessionConnectionAccepted{
			ProjectID:    record.ProjectID,
			SessionID:    record.SessionID,
			JobID:        record.JobID,
			State:        "connecting",
			RecoveryMode: mode,
			AcceptedAt:   record.Receipt.CommittedAt,
		},
		Receipt: record.Receipt,
	}
	result.Receipt.ChangeSequence = sequence

	record.Event.AggregateID = record.SessionID
	if err := saveMutation(
		ctx,
		tx,
		"reconnect_project_session",
		record.OperationID,
		record.RequestHash,
		result,
		record.Event,
		sequence,
	); err != nil {
		return application.MutationResult[application.SessionConnectionAccepted]{}, err
	}

	return commitProjectMutation(ctx, tx, result, "ReconnectProjectSession", record.ProjectID, record.JobID)
}

func (r *ProjectExternalRepository) AcceptExport(
	ctx context.Context,
	record application.ProjectExportRecord,
) (application.MutationResult[application.ExportAccepted], error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return application.MutationResult[application.ExportAccepted]{}, err
	}
	defer func() { _ = tx.Rollback() }()

	if result, found, err := existingResult[application.ExportAccepted](
		ctx,
		tx,
		"export_procedure",
		record.OperationID,
		record.RequestHash,
	); err != nil ||
		found {
		return result, err
	}

	var (
		projectID, status string
		revision          int64
	)
	if err := tx.QueryRowContext(ctx, `SELECT project_id,revision,status FROM procedures WHERE procedure_id=?`, record.ProcedureID).
		Scan(&projectID, &revision, &status); err != nil {
		return application.MutationResult[application.ExportAccepted]{}, projectExternalNotFound(err)
	}

	if revision != record.ProcedureRevision {
		return application.MutationResult[application.ExportAccepted]{}, &shared.Error{
			Code:            "revision_conflict",
			Message:         "手順書が更新されています",
			CurrentRevision: &revision,
		}
	}

	if status != "completed" {
		return application.MutationResult[application.ExportAccepted]{}, &shared.Error{
			Code:    "invalid_state",
			Message: "完成した手順書だけ出力できます",
		}
	}

	selectionJSON, err := json.Marshal(record.Destination)
	if err != nil {
		return application.MutationResult[application.ExportAccepted]{}, err
	}

	identityJSON := ""

	if record.OverwriteIdentity != nil {
		encoded, err := json.Marshal(record.OverwriteIdentity)
		if err != nil {
			return application.MutationResult[application.ExportAccepted]{}, err
		}

		identityJSON = string(encoded)
	}

	acceptedAt := record.Receipt.CommittedAt.UTC().UnixMicro()
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO exports(export_id,project_id,procedure_id,procedure_revision,format,destination_display_name,destination_path,overwrite_confirmed,overwrite_identity_json,destination_metadata_json,state,accepted_at) VALUES(?,?,?,?,?,?,?,?,?,?,'pending',?)`,
		record.ExportID,
		projectID,
		record.ProcedureID,
		record.ProcedureRevision,
		record.Format,
		filepath.Base(record.Destination.AbsolutePath),
		record.Destination.ResolvedPath,
		record.OverwriteConfirmed,
		identityJSON,
		string(selectionJSON),
		acceptedAt,
	); err != nil {
		return application.MutationResult[application.ExportAccepted]{}, err
	}

	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO background_jobs(job_id,project_id,kind,state,target_id,accepted_at) VALUES(?,?,'export','pending',?,?)`,
		record.JobID,
		projectID,
		record.ExportID,
		acceptedAt,
	); err != nil {
		return application.MutationResult[application.ExportAccepted]{}, err
	}

	var sequence int64
	if err := tx.QueryRowContext(ctx, `UPDATE app_settings SET change_sequence=change_sequence+1 WHERE singleton=1 RETURNING change_sequence`).
		Scan(&sequence); err != nil {
		return application.MutationResult[application.ExportAccepted]{}, err
	}

	result := application.MutationResult[application.ExportAccepted]{
		Data: application.ExportAccepted{
			ExportID:               record.ExportID,
			JobID:                  record.JobID,
			ProcedureID:            record.ProcedureID,
			Format:                 record.Format,
			DestinationDisplayName: filepath.Base(record.Destination.AbsolutePath),
			AcceptedAt:             record.Receipt.CommittedAt,
		},
		Receipt: record.Receipt,
	}
	result.Receipt.ChangeSequence = sequence

	record.Event.AggregateID = record.ExportID
	if err := saveMutation(
		ctx,
		tx,
		"export_procedure",
		record.OperationID,
		record.RequestHash,
		result,
		record.Event,
		sequence,
	); err != nil {
		return application.MutationResult[application.ExportAccepted]{}, err
	}

	return commitProjectMutation(ctx, tx, result, "ExportProcedure", record.ExportID, record.JobID)
}

func (r *ProjectExternalRepository) ClaimProjectJob(ctx context.Context) (*application.ClaimedProjectJob, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	job := &application.ClaimedProjectJob{}

	err = tx.QueryRowContext(ctx, `SELECT job_id,kind,project_id,target_id FROM background_jobs WHERE state='pending' ORDER BY accepted_at,job_id LIMIT 1`).
		Scan(&job.JobID, &job.Kind, &job.ProjectID, &job.TargetID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	if err := tx.QueryRowContext(ctx, `SELECT operation_id FROM operation_receipts
WHERE scope IN ('reconnect_project_session','export_procedure')
AND json_extract(result_json,'$.Data.JobID')=? LIMIT 1`, job.JobID).Scan(&job.OperationID); err != nil {
		return nil, err
	}

	if _, err := tx.ExecContext(
		ctx,
		`UPDATE background_jobs SET state='running' WHERE job_id=? AND state='pending'`,
		job.JobID,
	); err != nil {
		return nil, err
	}

	if job.Kind == "reconnect" {
		job.SessionID = job.TargetID

		var argsJSON string
		if err := tx.QueryRowContext(ctx, `SELECT s.requested_strategy,s.recovery_mode,p.workspace_path,c.command,c.args_json,c.transport FROM acp_sessions s JOIN projects p ON p.project_id=s.project_id JOIN agent_connections c ON c.connection_id=p.connection_id WHERE s.session_id=?`, job.SessionID).
			Scan(&job.Strategy, &job.RecoveryMode, &job.WorkspacePath, &job.Connection.Command, &argsJSON, &job.Connection.Transport); err != nil {
			return nil, err
		}

		if err := json.Unmarshal([]byte(argsJSON), &job.Connection.Args); err != nil {
			return nil, err
		}

		var previous sql.NullString

		err := tx.QueryRowContext(ctx, `SELECT agent_session_id FROM acp_sessions WHERE project_id=? AND state='connected' AND session_id<>? ORDER BY created_at DESC LIMIT 1`, job.ProjectID, job.SessionID).
			Scan(&previous)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}

		job.PreviousAgentSessionID = previous.String
	} else {
		var selectionJSON, identityJSON string
		if err := tx.QueryRowContext(ctx, `SELECT e.procedure_id,e.procedure_revision,e.format,e.destination_metadata_json,COALESCE(e.overwrite_identity_json,''),e.overwrite_confirmed,p.document_json FROM exports e JOIN procedures p ON p.procedure_id=e.procedure_id WHERE e.export_id=? AND p.status='completed' AND p.revision=e.procedure_revision`, job.TargetID).
			Scan(&job.ProcedureID, &job.ProcedureRevision, &job.Format, &selectionJSON, &identityJSON, &job.OverwriteConfirmed, &job.DocumentJSON); err != nil {
			return nil, err
		}

		if err := json.Unmarshal([]byte(selectionJSON), &job.Destination); err != nil {
			return nil, err
		}

		if identityJSON != "" {
			job.OverwriteIdentity = &application.OverwriteIdentity{}
			if err := json.Unmarshal([]byte(identityJSON), job.OverwriteIdentity); err != nil {
				return nil, err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return job, nil
}

func (r *ProjectExternalRepository) MarkExportReady(ctx context.Context, exportID, hash string) error {
	if len(hash) != 64 {
		return &shared.Error{Code: "validation_error", Message: "出力内容のhashが不正です"}
	}

	result, err := r.db.ExecContext(
		ctx,
		`UPDATE exports SET state='running',destination_metadata_json=json_set(destination_metadata_json,'$.publishedSha256',?) WHERE export_id=? AND state='pending'`,
		hash,
		exportID,
	)
	if err != nil {
		return err
	}

	count, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if count != 1 {
		return &shared.Error{Code: "invalid_state", Message: "出力jobは準備中ではありません"}
	}

	return nil
}

func (r *ProjectExternalRepository) CompleteProjectJob(
	ctx context.Context,
	result application.ProjectJobCompletion,
) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	state := "failed"
	if result.Success {
		state = "succeeded"
	}

	completedAt := result.CompletedAt.UTC().UnixMicro()

	updated, err := tx.ExecContext(
		ctx,
		`UPDATE background_jobs SET state=?,completed_at=?,error_code=? WHERE job_id=? AND state='running'`,
		state,
		completedAt,
		result.ErrorCode,
		result.JobID,
	)
	if err != nil {
		return err
	}

	count, err := updated.RowsAffected()
	if err != nil {
		return err
	}

	if count != 1 {
		return &shared.Error{Code: "invalid_state", Message: "jobは実行中ではありません"}
	}

	if result.Kind == "reconnect" {
		sessionState := "failed"
		if result.Success {
			sessionState = "connected"
		}

		if _, err := tx.ExecContext(
			ctx,
			`UPDATE acp_sessions SET state=?,agent_session_id=?,recovery_mode=?,resume_supported=?,load_supported=?,updated_at=?,completed_at=?,error_code=? WHERE session_id=?`,
			sessionState,
			result.Session.AgentSessionID,
			result.Session.RecoveryMode,
			result.Session.ResumeSupported,
			result.Session.LoadSupported,
			completedAt,
			completedAt,
			result.ErrorCode,
			result.TargetID,
		); err != nil {
			return err
		}
	} else {
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE exports SET state=?,completed_at=?,error_code=? WHERE export_id=?`,
			state,
			completedAt,
			result.ErrorCode,
			result.TargetID,
		); err != nil {
			return err
		}
	}

	var sequence int64
	if err := tx.QueryRowContext(ctx, `UPDATE app_settings SET change_sequence=change_sequence+1 WHERE singleton=1 RETURNING change_sequence`).
		Scan(&sequence); err != nil {
		return err
	}

	name, aggregateType := "export.updated", "export"
	if result.Kind == "reconnect" {
		name, aggregateType = "session.connection_changed", "session"
	}

	if err := insertOutbox(
		ctx,
		tx,
		application.OutboxEvent{
			ID:            result.JobID + ":completed",
			Name:          name,
			AggregateType: aggregateType,
			AggregateID:   result.TargetID,
			EmittedAt:     result.CompletedAt,
			Correlation:   result.JobID,
		},
		sequence,
	); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	status := "failed"
	if result.Success {
		status = "succeeded"
	}

	trace.Record(ctx, trace.Entry{
		Phase: "complete_commit", OperationID: result.OperationID,
		JobID: result.JobID, AggregateID: result.TargetID, ChangeSequence: sequence,
		Status: status, ErrorCode: result.ErrorCode,
	})

	return nil
}

func (r *ProjectExternalRepository) FailInterruptedProjectJobs(ctx context.Context, at time.Time) error {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT export_id,destination_path,json_extract(destination_metadata_json,'$.publishedSha256') FROM exports WHERE state='running' AND json_extract(destination_metadata_json,'$.publishedSha256') IS NOT NULL`,
	)
	if err != nil {
		return err
	}

	var published []string

	for rows.Next() {
		var exportID, path, hash string
		if err := rows.Scan(&exportID, &path, &hash); err != nil {
			_ = rows.Close()
			return err
		}

		if matchesPublishedExport(path, hash) {
			published = append(published, exportID)
		}
	}

	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}

	if err := rows.Close(); err != nil {
		return err
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	defer func() { _ = tx.Rollback() }()

	stamp := at.UTC().UnixMicro()
	if _, err := tx.ExecContext(
		ctx,
		`UPDATE acp_sessions SET state='interrupted',updated_at=?,completed_at=?,error_code='operation_interrupted' WHERE session_id IN (SELECT target_id FROM background_jobs WHERE kind='reconnect' AND state IN ('pending','running'))`,
		stamp,
		stamp,
	); err != nil {
		return err
	}

	if _, err := tx.ExecContext(
		ctx,
		`UPDATE exports SET state='failed',completed_at=?,error_code='operation_interrupted' WHERE export_id IN (SELECT target_id FROM background_jobs WHERE kind='export' AND state IN ('pending','running'))`,
		stamp,
	); err != nil {
		return err
	}

	if _, err := tx.ExecContext(
		ctx,
		`UPDATE background_jobs SET state='interrupted',completed_at=?,error_code='operation_interrupted' WHERE state IN ('pending','running')`,
		stamp,
	); err != nil {
		return err
	}

	for _, exportID := range published {
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE exports SET state='succeeded',completed_at=?,error_code=NULL WHERE export_id=?`,
			stamp,
			exportID,
		); err != nil {
			return err
		}

		var jobID string
		if err := tx.QueryRowContext(ctx, `UPDATE background_jobs SET state='succeeded',completed_at=?,error_code=NULL WHERE kind='export' AND target_id=? RETURNING job_id`, stamp, exportID).
			Scan(&jobID); err != nil {
			return err
		}

		var sequence int64
		if err := tx.QueryRowContext(ctx, `UPDATE app_settings SET change_sequence=change_sequence+1 WHERE singleton=1 RETURNING change_sequence`).
			Scan(&sequence); err != nil {
			return err
		}

		if err := insertOutbox(
			ctx,
			tx,
			application.OutboxEvent{
				ID:            jobID + ":completed",
				Name:          "export.updated",
				AggregateType: "export",
				AggregateID:   exportID,
				EmittedAt:     at.UTC(),
				Correlation:   jobID,
			},
			sequence,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func matchesPublishedExport(path, wantHash string) bool {
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return false
	}
	defer func() { _ = root.Close() }()

	info, err := root.Lstat(filepath.Base(path))
	if err != nil || !info.Mode().IsRegular() {
		return false
	}

	file, err := root.Open(filepath.Base(path))
	if err != nil {
		return false
	}

	defer func() { _ = file.Close() }()

	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return false
	}

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return false
	}

	return hex.EncodeToString(hash.Sum(nil)) == wantHash
}

func projectExternalNotFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return &shared.Error{Code: "not_found", Message: "対象が見つかりません"}
	}

	return err
}

var _ application.ProjectExternalRepository = (*ProjectExternalRepository)(nil)
