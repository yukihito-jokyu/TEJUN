package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/yukihito-jokyu/TEJUN/internal/application"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/execution"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/shared"
)

func (r *ExecutionRepository) RegisterPermission(ctx context.Context, incoming application.IncomingPermission) error {
	if incoming.ID == "" || incoming.ExecutionID == "" || incoming.SessionID == "" ||
		incoming.ProcessGeneration <= 0 || len(incoming.Options) == 0 {
		return &shared.Error{Code: "validation_failed", Message: "permissionの入力が不正です"}
	}

	seen := make(map[string]bool, len(incoming.Options))
	for _, option := range incoming.Options {
		if option.ID == "" || seen[option.ID] {
			return &shared.Error{Code: "validation_failed", Message: "permission optionが不正です"}
		}

		seen[option.ID] = true
	}

	options, err := json.Marshal(incoming.Options)
	if err != nil {
		return err
	}

	var expiresAt any
	if incoming.ExpiresAt != nil {
		expiresAt = incoming.ExpiresAt.Format(time.RFC3339Nano)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx, `INSERT INTO execution_permissions
(permission_request_id, execution_id, session_id, run_id, tool_call_id, title, options_json,
rpc_request_id_json, agent_session_id, process_generation, command_digest, state, requested_at, expires_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', ?, ?)`, incoming.ID, incoming.ExecutionID,
		incoming.SessionID, incoming.RunID, incoming.ToolCallID, incoming.Title, string(options),
		incoming.RPCRequestIDJSON, incoming.AgentSessionID, incoming.ProcessGeneration, incoming.CommandDigest,
		incoming.RequestedAt.Format(time.RFC3339Nano), expiresAt)
	if err != nil {
		return err
	}

	var sequence int64
	if err := tx.QueryRowContext(ctx, `UPDATE app_settings SET change_sequence = change_sequence + 1
WHERE singleton = 1 RETURNING change_sequence`).Scan(&sequence); err != nil {
		return err
	}

	if err := insertOutbox(ctx, tx, application.OutboxEvent{
		ID: incoming.ID + ":pending", Name: "execution.permission_updated", EmittedAt: incoming.RequestedAt,
		AggregateType: "execution", AggregateID: incoming.ExecutionID,
	}, sequence); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *ExecutionRepository) ClaimPermission(
	ctx context.Context,
	claim application.PermissionClaim,
) (application.MutationResult[application.PermissionResponseResult], bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return application.MutationResult[application.PermissionResponseResult]{}, false, err
	}
	defer func() { _ = tx.Rollback() }()

	hash := requestHash(claim.SessionID, claim.PermissionRequestID, claim.OptionID)
	if result, found, err := existingResult[application.PermissionResponseResult](ctx, tx,
		"permission_response", claim.OperationID, hash); err != nil || found {
		return result, false, err
	}

	var (
		optionsJSON, state string
		generation         int64
		expiresAt          sql.NullString
	)

	err = tx.QueryRowContext(ctx, `SELECT options_json, state, process_generation, expires_at
FROM execution_permissions WHERE permission_request_id = ? AND session_id = ?`, claim.PermissionRequestID,
		claim.SessionID).Scan(&optionsJSON, &state, &generation, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return application.MutationResult[application.PermissionResponseResult]{}, false,
			&shared.Error{Code: "not_found", Message: "permissionがありません"}
	}

	if err != nil {
		return application.MutationResult[application.PermissionResponseResult]{}, false, err
	}

	if state != "pending" || generation != claim.ProcessGeneration {
		return application.MutationResult[application.PermissionResponseResult]{}, false,
			&shared.Error{Code: "invalid_state", Message: "回答可能なpermissionではありません"}
	}

	if expiresAt.Valid {
		at, err := time.Parse(time.RFC3339Nano, expiresAt.String)
		if err != nil {
			return application.MutationResult[application.PermissionResponseResult]{}, false, err
		}

		if !claim.At.Before(at) {
			return application.MutationResult[application.PermissionResponseResult]{}, false,
				&shared.Error{Code: "invalid_state", Message: "permissionの期限が切れています"}
		}
	}

	var options []execution.PermissionOption
	if err := json.Unmarshal([]byte(optionsJSON), &options); err != nil {
		return application.MutationResult[application.PermissionResponseResult]{}, false, err
	}

	found := false

	for _, option := range options {
		if option.ID == claim.OptionID {
			found = true
			break
		}
	}

	if !found {
		return application.MutationResult[application.PermissionResponseResult]{}, false,
			&shared.Error{Code: "validation_failed", Message: "提示されていないoptionです"}
	}

	if _, err := tx.ExecContext(ctx, `UPDATE execution_permissions SET state = 'responding', selected_option_id = ?
WHERE permission_request_id = ? AND state = 'pending'`, claim.OptionID, claim.PermissionRequestID); err != nil {
		return application.MutationResult[application.PermissionResponseResult]{}, false, err
	}

	result := application.MutationResult[application.PermissionResponseResult]{
		Data: application.PermissionResponseResult{
			PermissionRequestID: claim.PermissionRequestID,
			Status:              "responding", SelectedOptionID: claim.OptionID,
		},
		Receipt: application.MutationReceipt{OperationID: claim.OperationID, CommittedAt: claim.At},
	}

	encoded, err := json.Marshal(result)
	if err != nil {
		return application.MutationResult[application.PermissionResponseResult]{}, false, err
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO operation_receipts
(scope, operation_id, request_hash, result_json, committed_at) VALUES ('permission_response', ?, ?, ?, ?)`,
		claim.OperationID, hash, string(encoded), claim.At.Format(time.RFC3339Nano)); err != nil {
		return application.MutationResult[application.PermissionResponseResult]{}, false, err
	}

	if err := tx.Commit(); err != nil {
		return application.MutationResult[application.PermissionResponseResult]{}, false, err
	}

	return result, true, nil
}

func (r *ExecutionRepository) CompletePermission(
	ctx context.Context,
	requestID, operationID string,
	succeeded bool,
	at time.Time,
) error {
	state := "failed"
	if succeeded {
		state = "sent"
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	defer func() { _ = tx.Rollback() }()

	var executionID, selectedOptionID string
	if err := tx.QueryRowContext(ctx, `UPDATE execution_permissions SET state = ?, responded_at = ?
WHERE permission_request_id = ? AND state = 'responding'
RETURNING execution_id, selected_option_id`, state, at.Format(time.RFC3339Nano), requestID).
		Scan(&executionID, &selectedOptionID); err != nil {
		return err
	}

	if succeeded {
		result := application.MutationResult[application.PermissionResponseResult]{
			Data: application.PermissionResponseResult{
				PermissionRequestID: requestID, Status: state,
				SelectedOptionID: selectedOptionID, RespondedAt: at,
			},
			Receipt: application.MutationReceipt{OperationID: operationID},
		}

		var committedAt string
		if err := tx.QueryRowContext(ctx, `SELECT committed_at FROM operation_receipts
WHERE scope = 'permission_response' AND operation_id = ?`, operationID).Scan(&committedAt); err != nil {
			return err
		}

		if result.Receipt.CommittedAt, err = time.Parse(time.RFC3339Nano, committedAt); err != nil {
			return err
		}

		encoded, err := json.Marshal(result)
		if err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx, `UPDATE operation_receipts SET result_json = ?
WHERE scope = 'permission_response' AND operation_id = ?`, string(encoded), operationID); err != nil {
			return err
		}
	} else if _, err := tx.ExecContext(ctx, `DELETE FROM operation_receipts
WHERE scope = 'permission_response' AND operation_id = ?`, operationID); err != nil {
		return err
	}

	var sequence int64
	if err := tx.QueryRowContext(ctx, `UPDATE app_settings SET change_sequence = change_sequence + 1
WHERE singleton = 1 RETURNING change_sequence`).Scan(&sequence); err != nil {
		return err
	}

	if err := insertOutbox(ctx, tx, application.OutboxEvent{
		ID: requestID + ":" + state, Name: "execution.permission_updated", EmittedAt: at,
		AggregateType: "execution", AggregateID: executionID, Correlation: operationID,
	}, sequence); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *ExecutionRepository) FailInterruptedPermissions(ctx context.Context, at time.Time) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, `SELECT permission_request_id, execution_id FROM execution_permissions
WHERE state = 'responding'`)
	if err != nil {
		return err
	}

	type interrupted struct{ requestID, executionID string }

	var items []interrupted

	for rows.Next() {
		var item interrupted
		if err := rows.Scan(&item.requestID, &item.executionID); err != nil {
			_ = rows.Close()
			return err
		}

		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}

	if err := rows.Close(); err != nil {
		return err
	}

	for _, item := range items {
		if _, err := tx.ExecContext(ctx, `UPDATE execution_permissions SET state = 'failed', responded_at = ?
WHERE permission_request_id = ?`, at.Format(time.RFC3339Nano), item.requestID); err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx, `DELETE FROM operation_receipts WHERE scope = 'permission_response'
AND json_extract(result_json, '$.Data.PermissionRequestID') = ?`, item.requestID); err != nil {
			return err
		}

		var sequence int64
		if err := tx.QueryRowContext(ctx, `UPDATE app_settings SET change_sequence = change_sequence + 1
WHERE singleton = 1 RETURNING change_sequence`).Scan(&sequence); err != nil {
			return err
		}

		if err := insertOutbox(ctx, tx, application.OutboxEvent{
			ID: item.requestID + ":failed", Name: "execution.permission_updated", EmittedAt: at,
			AggregateType: "execution", AggregateID: item.executionID,
		}, sequence); err != nil {
			return err
		}
	}

	return tx.Commit()
}
