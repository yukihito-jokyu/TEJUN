package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/yukihito-jokyu/TEJUN/internal/application"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/procedure"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/shared"
	"github.com/yukihito-jokyu/TEJUN/internal/trace"
)

func (r *ProcedureRepository) AcceptProcedureRevision(
	ctx context.Context,
	in application.ProcedureRevisionRecord,
) (application.MutationResult[application.AcceptedTurn], error) {
	var zero application.MutationResult[application.AcceptedTurn]

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return zero, err
	}

	defer func() { _ = tx.Rollback() }()

	content, err := json.Marshal(in.Content)
	if err != nil {
		return zero, err
	}

	hash := requestHash(in.ProcedureID, fmt.Sprint(in.ExpectedRevision), string(content))
	if result, found, e := existingResult[application.AcceptedTurn](
		ctx,
		tx,
		"request_procedure_revision",
		in.OperationID,
		hash,
	); e != nil ||
		found {
		return result, e
	}

	var (
		doc, status string
		revision    int64
	)

	err = tx.QueryRowContext(ctx, `SELECT document_json,status,revision FROM procedures WHERE procedure_id=?`, in.ProcedureID).
		Scan(&doc, &status, &revision)
	if err != nil {
		return zero, notFound(err)
	}

	if status != "draft" {
		return zero, &shared.Error{Code: "invalid_state", Message: "draftのみ修正できます"}
	}

	if revision != in.ExpectedRevision {
		return zero, &shared.Error{Code: "revision_conflict", Message: "手順書が更新されています", CurrentRevision: &revision}
	}

	var pending int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM procedure_revision_jobs WHERE procedure_id=? AND state IN ('queued','running','cancellation_requested')`, in.ProcedureID).
		Scan(&pending); err != nil {
		return zero, err
	}

	if pending != 0 {
		return zero, &shared.Error{Code: "invalid_state", Message: "修正依頼が進行中です"}
	}

	prompt := "次の手順書を利用者の依頼に沿って修正し、ProcedureDocumentのJSONオブジェクトのみを返してください。既存のevidenceIdは維持してください。手順書: " + doc + "\n依頼: " + string(
		content,
	)
	if len(prompt) > 6<<20 {
		return zero, &shared.Error{Code: "validation_failed", Message: "修正依頼が長すぎます"}
	}

	if _, err = tx.ExecContext(
		ctx,
		`INSERT INTO procedure_turns(turn_id,procedure_id,role,content_json,status,created_at) VALUES(?,?, 'user',?,'queued',?)`,
		in.TurnID,
		in.ProcedureID,
		string(content),
		in.At.UnixMicro(),
	); err != nil {
		return zero, err
	}

	if _, err = tx.ExecContext(
		ctx,
		`INSERT INTO procedure_revision_jobs(job_id,procedure_id,turn_id,session_id,state,prompt_text,expected_revision,accepted_at) VALUES(?,?,?,?,'queued',?,?,?)`,
		in.JobID,
		in.ProcedureID,
		in.TurnID,
		in.SessionID,
		prompt,
		in.ExpectedRevision,
		in.At.UnixMicro(),
	); err != nil {
		return zero, err
	}

	var sequence int64
	if err = tx.QueryRowContext(ctx, `UPDATE app_settings SET change_sequence=change_sequence+1 WHERE singleton=1 RETURNING change_sequence`).
		Scan(&sequence); err != nil {
		return zero, err
	}

	result := application.MutationResult[application.AcceptedTurn]{
		Data: application.AcceptedTurn{
			SessionID:  in.SessionID,
			TurnID:     in.TurnID,
			JobID:      in.JobID,
			AcceptedAt: in.At,
		},
		Receipt: application.MutationReceipt{OperationID: in.OperationID, CommittedAt: in.At, ChangeSequence: sequence},
	}

	event := application.OutboxEvent{
		ID:            in.EventID,
		Name:          "procedure.updated",
		AggregateType: "procedure",
		AggregateID:   in.ProcedureID,
		EmittedAt:     in.At,
		Correlation:   in.JobID,
	}
	if err = saveMutation(
		ctx,
		tx,
		"request_procedure_revision",
		in.OperationID,
		hash,
		result,
		event,
		sequence,
	); err != nil {
		return zero, err
	}

	if err = tx.Commit(); err != nil {
		return zero, err
	}

	trace.Record(
		ctx,
		trace.Entry{
			Phase:          "transaction_commit",
			Method:         "RequestProcedureRevision",
			OperationID:    in.OperationID,
			JobID:          in.JobID,
			EventID:        in.EventID,
			AggregateID:    in.ProcedureID,
			ChangeSequence: sequence,
			Status:         "succeeded",
		},
	)

	return result, nil
}

func (r *ProcedureRepository) ClaimProcedureRevision(
	ctx context.Context,
) (*application.ClaimedProcedureRevision, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var job application.ClaimedProcedureRevision

	err = tx.QueryRowContext(ctx, `UPDATE procedure_revision_jobs SET state='running' WHERE job_id=(SELECT job_id FROM procedure_revision_jobs WHERE state='queued' ORDER BY accepted_at,job_id LIMIT 1) RETURNING job_id,procedure_id,turn_id,session_id,prompt_text,expected_revision`).
		Scan(&job.JobID, &job.ProcedureID, &job.TurnID, &job.SessionID, &job.PromptText, &job.ExpectedRevision)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	var args string

	err = tx.QueryRowContext(ctx, `SELECT p.workspace_path,c.command,c.args_json,c.transport FROM procedures q JOIN projects p ON p.project_id=q.project_id JOIN agent_connections c ON c.connection_id=p.connection_id WHERE q.procedure_id=?`, job.ProcedureID).
		Scan(&job.WorkspacePath, &job.Connection.Command, &args, &job.Connection.Transport)
	if err != nil {
		return nil, err
	}

	if err = json.Unmarshal([]byte(args), &job.Connection.Args); err != nil {
		return nil, err
	}

	if _, err = tx.ExecContext(
		ctx,
		`UPDATE procedure_turns SET status='running' WHERE turn_id=?`,
		job.TurnID,
	); err != nil {
		return nil, err
	}

	if err = tx.Commit(); err != nil {
		return nil, err
	}

	trace.Record(
		ctx,
		trace.Entry{
			Phase:       "transaction_commit",
			Method:      "ClaimProcedureRevision",
			JobID:       job.JobID,
			AggregateID: job.ProcedureID,
			Status:      "succeeded",
		},
	)

	return &job, nil
}

func (r *ProcedureRepository) FinishProcedureRevision(
	ctx context.Context,
	job application.ClaimedProcedureRevision,
	answer string,
	runErr error,
	at time.Time,
	eventID string,
) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var (
		state    string
		revision int64
	)

	err = tx.QueryRowContext(ctx, `SELECT j.state,q.revision FROM procedure_revision_jobs j JOIN procedures q ON q.procedure_id=j.procedure_id WHERE j.job_id=?`, job.JobID).
		Scan(&state, &revision)
	if err != nil {
		return err
	}

	status := "completed"
	code := ""

	if state == "cancellation_requested" {
		status = "cancelled"
		code = "cancelled"
	} else if runErr != nil {
		status = "failed"
		code = "generation_failed"
	} else if revision != job.ExpectedRevision {
		status = "failed"
		code = "revision_conflict"
	} else {
		var doc procedure.Document

		decoder := json.NewDecoder(strings.NewReader(answer))
		decoder.DisallowUnknownFields()

		if err = decoder.Decode(&doc); err != nil {
			status = "failed"
			code = "invalid_response"
		} else if err = decoder.Decode(new(any)); err != io.EOF {
			status = "failed"
			code = "invalid_response"
		} else if len(answer) > 5<<20 || !validProcedureDocumentText(doc) {
			status = "failed"
			code = "invalid_response"
		} else {
			ids, e := procedureEvidenceIDs(ctx, tx, job.ProcedureID)
			if e != nil {
				return e
			}

			if procedure.Evaluate(doc, ids).Status == "blocked" {
				status = "failed"
				code = "invalid_response"
			} else {
				canonical, e := json.Marshal(doc)
				if e != nil {
					return e
				}

				result, e := tx.ExecContext(
					ctx,
					`UPDATE procedures SET document_json=?,revision=revision+1,updated_at=? WHERE procedure_id=? AND revision=? AND status='draft'`,
					string(canonical),
					at.UnixMicro(),
					job.ProcedureID,
					job.ExpectedRevision,
				)
				if e != nil {
					return e
				}

				count, e := result.RowsAffected()
				if e != nil {
					return e
				}

				if count != 1 {
					status = "failed"
					code = "revision_conflict"
				}
			}
		}
	}

	if _, err = tx.ExecContext(
		ctx,
		`UPDATE procedure_revision_jobs SET state=?,completed_at=?,error_code=? WHERE job_id=?`,
		status,
		at.UnixMicro(),
		code,
		job.JobID,
	); err != nil {
		return err
	}

	if _, err = tx.ExecContext(
		ctx,
		`UPDATE procedure_turns SET status=? WHERE turn_id=?`,
		status,
		job.TurnID,
	); err != nil {
		return err
	}

	if status == "completed" {
		content, _ := json.Marshal([]application.ContentPart{{Type: "text", Text: answer}})
		if _, err = tx.ExecContext(
			ctx,
			`INSERT INTO procedure_turns(turn_id,procedure_id,role,content_json,status,created_at) VALUES(?,?,'agent',?,'completed',?)`,
			job.JobID+":reply",
			job.ProcedureID,
			string(content),
			at.UnixMicro(),
		); err != nil {
			return err
		}
	}

	var sequence int64
	if err = tx.QueryRowContext(ctx, `UPDATE app_settings SET change_sequence=change_sequence+1 WHERE singleton=1 RETURNING change_sequence`).
		Scan(&sequence); err != nil {
		return err
	}

	event := application.OutboxEvent{
		ID:            eventID,
		Name:          "procedure.updated",
		AggregateType: "procedure",
		AggregateID:   job.ProcedureID,
		EmittedAt:     at,
		Correlation:   job.JobID,
	}
	if err = insertOutbox(ctx, tx, event, sequence); err != nil {
		return err
	}

	if err = tx.Commit(); err != nil {
		return err
	}

	trace.Record(
		ctx,
		trace.Entry{
			Phase:          "complete_commit",
			Method:         "FinishProcedureRevision",
			JobID:          job.JobID,
			EventID:        eventID,
			AggregateID:    job.ProcedureID,
			ChangeSequence: sequence,
			Status:         status,
			ErrorCode:      code,
		},
	)

	return nil
}

func (r *ProcedureRepository) FailInterruptedProcedureRevisions(ctx context.Context, at time.Time) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(
		ctx,
		`SELECT job_id,procedure_id FROM procedure_revision_jobs WHERE state IN ('running','cancellation_requested')`,
	)
	if err != nil {
		return err
	}

	type interrupted struct{ jobID, procedureID string }

	var jobs []interrupted

	for rows.Next() {
		var job interrupted
		if err := rows.Scan(&job.jobID, &job.procedureID); err != nil {
			_ = rows.Close()
			return err
		}

		jobs = append(jobs, job)
	}

	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}

	if err := rows.Close(); err != nil {
		return err
	}

	_, err = tx.ExecContext(
		ctx,
		`UPDATE procedure_revision_jobs SET state='interrupted',completed_at=?,error_code='interrupted' WHERE state IN ('running','cancellation_requested')`,
		at.UnixMicro(),
	)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(
		ctx,
		`UPDATE procedure_turns SET status='interrupted' WHERE turn_id IN (SELECT turn_id FROM procedure_revision_jobs WHERE state='interrupted')`,
	)
	if err != nil {
		return err
	}

	for _, job := range jobs {
		var sequence int64
		if err := tx.QueryRowContext(ctx, `UPDATE app_settings SET change_sequence=change_sequence+1 WHERE singleton=1 RETURNING change_sequence`).
			Scan(&sequence); err != nil {
			return err
		}

		if err := insertOutbox(ctx, tx, application.OutboxEvent{
			ID: job.jobID + ":interrupted", Name: "procedure.updated", EmittedAt: at,
			AggregateType: "procedure", AggregateID: job.procedureID, Correlation: job.jobID,
		}, sequence); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *ProcedureRepository) CancelProcedureRevision(
	ctx context.Context,
	sessionID, turnID, jobID, operationID string,
	at time.Time,
) (application.MutationResult[application.CancellationAccepted], bool, string, error) {
	var zero application.MutationResult[application.CancellationAccepted]

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return zero, false, "", err
	}

	defer func() { _ = tx.Rollback() }()

	hash := requestHash(sessionID, turnID, jobID)
	if result, found, e := existingResult[application.CancellationAccepted](
		ctx,
		tx,
		"cancel_procedure_revision",
		operationID,
		hash,
	); e != nil ||
		found {
		return result, false, "", e
	}

	var target, state, procedureID string

	err = tx.QueryRowContext(ctx, `SELECT job_id,state,procedure_id FROM procedure_revision_jobs WHERE session_id=? AND (turn_id=? OR job_id=?) ORDER BY accepted_at DESC LIMIT 1`, sessionID, turnID, jobID).
		Scan(&target, &state, &procedureID)
	if err != nil {
		return zero, false, "", notFound(err)
	}

	running := state == "running"
	if state != "queued" && !running {
		return zero, false, "", &shared.Error{Code: "invalid_state", Message: "取消対象が進行中ではありません"}
	}

	next := "cancellation_requested"
	if state == "queued" {
		next = "cancelled"
	}

	if _, err = tx.ExecContext(
		ctx,
		`UPDATE procedure_revision_jobs SET state=?,completed_at=CASE WHEN ?='cancelled' THEN ? ELSE completed_at END WHERE job_id=?`,
		next,
		next,
		at.UnixMicro(),
		target,
	); err != nil {
		return zero, false, "", err
	}

	if _, err = tx.ExecContext(
		ctx,
		`UPDATE procedure_turns SET status=? WHERE turn_id=(SELECT turn_id FROM procedure_revision_jobs WHERE job_id=?)`,
		next,
		target,
	); err != nil {
		return zero, false, "", err
	}

	var sequence int64
	if err = tx.QueryRowContext(ctx, `UPDATE app_settings SET change_sequence=change_sequence+1 WHERE singleton=1 RETURNING change_sequence`).
		Scan(&sequence); err != nil {
		return zero, false, "", err
	}

	result := application.MutationResult[application.CancellationAccepted]{
		Data: application.CancellationAccepted{
			JobID:        target,
			TargetStatus: next,
			RequestedAt:  at,
			Mechanism:    "acp_cancel",
		},
		Receipt: application.MutationReceipt{OperationID: operationID, CommittedAt: at, ChangeSequence: sequence},
	}

	event := application.OutboxEvent{
		ID:            target + ":" + operationID,
		Name:          "procedure.updated",
		AggregateType: "procedure",
		AggregateID:   procedureID,
		EmittedAt:     at,
		Correlation:   target,
	}
	if err = saveMutation(
		ctx,
		tx,
		"cancel_procedure_revision",
		operationID,
		hash,
		result,
		event,
		sequence,
	); err != nil {
		return zero, false, "", err
	}

	if err = tx.Commit(); err != nil {
		return zero, false, "", err
	}

	trace.Record(
		ctx,
		trace.Entry{
			Phase:          "transaction_commit",
			Method:         "CancelProcedureRevision",
			OperationID:    operationID,
			JobID:          target,
			EventID:        event.ID,
			AggregateID:    procedureID,
			ChangeSequence: sequence,
			Status:         "succeeded",
		},
	)

	return result, running, target, nil
}
