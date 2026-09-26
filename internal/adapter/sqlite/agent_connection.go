package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/yukihito-jokyu/TEJUN/internal/application"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/agentconnection"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/shared"
)

type AgentConnectionRepository struct{ db *sql.DB }

type PendingEvent struct {
	ID, Name, EmittedAt, AggregateType, AggregateID, StreamKey, Correlation, Payload, OperationID string
	ChangeSequence, StreamRevision                                                                int64
}

func NewAgentConnectionRepository(db *sql.DB) *AgentConnectionRepository {
	return &AgentConnectionRepository{db: db}
}

func (r *AgentConnectionRepository) FailInterruptedElicitations(ctx context.Context, completedAt time.Time) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM operation_receipts
WHERE scope = 'respond_to_elicitation' AND json_extract(result_json, '$.data.elicitationRequestId') IN
(SELECT elicitation_request_id FROM elicitation_requests WHERE state = 'responding')`); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `UPDATE elicitation_requests SET state = 'failed', responded_at = ?
WHERE state = 'responding'`, completedAt.Format(time.RFC3339Nano)); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *AgentConnectionRepository) PendingEvents(ctx context.Context) ([]PendingEvent, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT event_id, name, emitted_at, aggregate_type, aggregate_id,
change_sequence, stream_key, stream_revision, correlation, payload_json,
COALESCE((SELECT operation_id FROM operation_receipts WHERE
json_extract(result_json,'$.Receipt.ChangeSequence')=event_outbox.change_sequence LIMIT 1),'')
FROM event_outbox
WHERE dispatched_at IS NULL ORDER BY change_sequence, event_id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	events := []PendingEvent{}

	for rows.Next() {
		var event PendingEvent
		if err := rows.Scan(
			&event.ID,
			&event.Name,
			&event.EmittedAt,
			&event.AggregateType,
			&event.AggregateID,
			&event.ChangeSequence,
			&event.StreamKey,
			&event.StreamRevision,
			&event.Correlation,
			&event.Payload,
			&event.OperationID,
		); err != nil {
			return nil, err
		}

		events = append(events, event)
	}

	return events, rows.Err()
}

func (r *AgentConnectionRepository) MarkEventDispatched(ctx context.Context, eventID string, at time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE event_outbox SET dispatched_at = ?
WHERE event_id = ? AND dispatched_at IS NULL`, at.Format(time.RFC3339Nano), eventID)

	return err
}

func (r *AgentConnectionRepository) GetStartupState(ctx context.Context) (application.StartupState, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return application.StartupState{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var (
		connectionID sql.NullString
		sequence     int64
	)

	if err := tx.QueryRowContext(
		ctx,
		"SELECT default_connection_id, change_sequence FROM app_settings WHERE singleton = 1",
	).Scan(&connectionID, &sequence); err != nil {
		return application.StartupState{}, err
	}

	state := application.StartupState{
		InitialSetupRequired: !connectionID.Valid,
		NextRoute:            "#/setup",
		ChangeSequence:       sequence,
	}
	if connectionID.Valid {
		connection, err := getConnection(ctx, tx, connectionID.String)
		if err != nil {
			return application.StartupState{}, err
		}

		state.InitialSetupRequired = false
		state.DefaultConnection = &connection
		state.NextRoute = "#/projects"
	}

	var interrupted int
	if err := tx.QueryRowContext(
		ctx,
		"SELECT COUNT(*) FROM agent_jobs WHERE state IN ('pending', 'running')",
	).Scan(&interrupted); err != nil {
		return application.StartupState{}, err
	}

	if interrupted > 0 {
		state.RecoveryNotice = "中断されたAgent操作があります"
	}

	if err := tx.Commit(); err != nil {
		return application.StartupState{}, err
	}

	return state, nil
}

func (r *AgentConnectionRepository) CompleteInitialSetup(
	ctx context.Context,
	record application.CompleteSetupRecord,
) (application.MutationResult[application.InitialSetupResult], error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return application.MutationResult[application.InitialSetupResult]{}, err
	}
	defer func() { _ = tx.Rollback() }()

	if result, found, err := existingResult[application.InitialSetupResult](
		ctx, tx, "complete_initial_setup", record.OperationID, record.RequestHash,
	); err != nil || found {
		return result, err
	}

	args, err := json.Marshal(record.Connection.Args)
	if err != nil {
		return application.MutationResult[application.InitialSetupResult]{}, err
	}

	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO agent_connections (
connection_id, display_name, command, args_json, transport, resolved_executable_path, last_verified_at,
protocol_version, auth_state, schema_artifact_version, revision) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.Connection.ID, record.Connection.DisplayName, record.Connection.Command,
		string(args), record.Connection.Transport, record.Connection.ResolvedExecutablePath,
		record.Connection.LastVerifiedAt.Format(time.RFC3339Nano), record.Connection.ProtocolVersion,
		record.Connection.AuthState, record.Connection.SchemaArtifactVersion, record.Connection.Revision,
	)
	if err != nil {
		return application.MutationResult[application.InitialSetupResult]{}, err
	}

	var sequence int64
	if err := tx.QueryRowContext(
		ctx,
		`UPDATE app_settings SET default_connection_id = ?, change_sequence = change_sequence + 1
WHERE singleton = 1 RETURNING change_sequence`,
		record.Connection.ID,
	).Scan(&sequence); err != nil {
		return application.MutationResult[application.InitialSetupResult]{}, err
	}

	record.Event.AggregateID = record.Connection.ID

	result := application.MutationResult[application.InitialSetupResult]{
		Data: application.InitialSetupResult{
			Connection: record.Connection, NextRoute: "#/projects", ChangeSequence: sequence,
		},
		Receipt: record.Receipt,
	}
	if err := saveMutation(
		ctx, tx, "complete_initial_setup", record.OperationID,
		record.RequestHash, result, record.Event, sequence,
	); err != nil {
		return application.MutationResult[application.InitialSetupResult]{}, err
	}

	if err := tx.Commit(); err != nil {
		return application.MutationResult[application.InitialSetupResult]{}, err
	}

	return result, nil
}

func (r *AgentConnectionRepository) AcceptAgentJob(
	ctx context.Context,
	record application.AgentJobRecord,
) (application.MutationResult[application.AgentJobAccepted], error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return application.MutationResult[application.AgentJobAccepted]{}, err
	}
	defer func() { _ = tx.Rollback() }()

	scope := "agent_job:" + string(record.Kind)
	if result, found, err := existingResult[application.AgentJobAccepted](
		ctx, tx, scope, record.OperationID, record.RequestHash,
	); err != nil || found {
		return result, err
	}

	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO agent_jobs
(job_id, kind, target_id, auth_method_id, method_type, state, accepted_at) VALUES (?, ?, ?, ?, ?, 'pending', ?)`,
		record.JobID, record.Kind, record.TargetID, record.AuthMethodID,
		record.MethodType, record.AcceptedAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return application.MutationResult[application.AgentJobAccepted]{}, err
	}

	acceptedState := "authenticating"
	if record.Kind == agentconnection.JobLogout {
		acceptedState = "logging_out"
	}

	result := application.MutationResult[application.AgentJobAccepted]{
		Data: application.AgentJobAccepted{
			JobID: record.JobID, TargetID: record.TargetID, MethodType: record.MethodType,
			State: acceptedState, AcceptedAt: record.AcceptedAt,
		},
		Receipt: record.Receipt,
	}
	if err := saveMutation(
		ctx, tx, scope, record.OperationID, record.RequestHash, result, record.Event, 1,
	); err != nil {
		return application.MutationResult[application.AgentJobAccepted]{}, err
	}

	if err := tx.Commit(); err != nil {
		return application.MutationResult[application.AgentJobAccepted]{}, err
	}

	return result, nil
}

func (r *AgentConnectionRepository) ClaimAgentJob(ctx context.Context) (*application.ClaimedAgentJob, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var (
		job                                       application.ClaimedAgentJob
		kind                                      string
		displayName, command, argsJSON, transport sql.NullString
	)

	err = tx.QueryRowContext(ctx, `UPDATE agent_jobs SET state = 'running'
WHERE job_id = (SELECT job_id FROM agent_jobs WHERE state = 'pending' ORDER BY accepted_at, job_id LIMIT 1)
RETURNING job_id, kind, target_id, auth_method_id, method_type,
(SELECT display_name FROM agent_connections WHERE connection_id = target_id),
(SELECT command FROM agent_connections WHERE connection_id = target_id),
(SELECT args_json FROM agent_connections WHERE connection_id = target_id),
(SELECT transport FROM agent_connections WHERE connection_id = target_id)`).
		Scan(&job.JobID, &kind, &job.TargetID, &job.AuthMethodID, &job.MethodType, &displayName, &command, &argsJSON, &transport)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	job.Kind = agentconnection.JobKind(kind)

	if command.Valid {
		connection := agentconnection.ConnectionInput{
			DisplayName: displayName.String,
			Command:     command.String,
			Transport:   transport.String,
		}
		if err := json.Unmarshal([]byte(argsJSON.String), &connection.Args); err != nil {
			return nil, err
		}

		job.Connection = &connection
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &job, nil
}

func (r *AgentConnectionRepository) FailInterruptedAgentJobs(ctx context.Context, completedAt time.Time) error {
	_, err := r.db.ExecContext(
		ctx,
		`UPDATE agent_jobs SET state = 'failed', completed_at = ? WHERE state = 'running'`,
		completedAt.Format(time.RFC3339Nano),
	)

	return err
}

func (r *AgentConnectionRepository) RegisterElicitation(
	ctx context.Context,
	incoming application.IncomingElicitation,
) error {
	var expiresAt any
	if incoming.ExpiresAt != nil {
		expiresAt = incoming.ExpiresAt.Format(time.RFC3339Nano)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx, `INSERT INTO elicitation_requests
(elicitation_request_id, connection_attempt_id, process_generation, mode, message, state, requested_at, expires_at)
VALUES (?, ?, ?, ?, ?, 'pending', ?, ?)`, incoming.ID, incoming.ConnectionAttemptID, incoming.ProcessGeneration,
		incoming.Mode, incoming.Message, incoming.RequestedAt.Format(time.RFC3339Nano), expiresAt)
	if err != nil {
		return err
	}

	event := application.OutboxEvent{
		ID: "elicitation:pending:" + incoming.ID, Name: "agent.elicitation.updated",
		EmittedAt: incoming.RequestedAt, AggregateType: "elicitation", AggregateID: incoming.ID,
	}
	if err := insertOutbox(ctx, tx, event, 1); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *AgentConnectionRepository) CompleteAgentJob(
	ctx context.Context,
	jobID string,
	succeeded bool,
	completedAt time.Time,
) error {
	state := agentconnection.JobFailed
	if succeeded {
		state = agentconnection.JobSucceeded
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(
		ctx,
		`UPDATE agent_jobs SET state = ?, completed_at = ? WHERE job_id = ? AND state = 'running'`,
		state, completedAt.Format(time.RFC3339Nano), jobID,
	)
	if err != nil {
		return err
	}

	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		if err != nil {
			return err
		}

		return &shared.Error{Code: "not_found", Message: "実行中のAgent jobがありません"}
	}

	event := application.OutboxEvent{
		ID: "agent-job:" + jobID, Name: "agent.authentication.updated",
		EmittedAt: completedAt, AggregateType: "agent_job", AggregateID: jobID,
	}
	if err := insertOutbox(ctx, tx, event, 1); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *AgentConnectionRepository) ClaimElicitation(
	ctx context.Context,
	claim application.ElicitationClaim,
) (application.MutationResult[application.ElicitationResponseResult], bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return application.MutationResult[application.ElicitationResponseResult]{}, false, err
	}
	defer func() { _ = tx.Rollback() }()

	if result, found, err := existingResult[application.ElicitationResponseResult](
		ctx, tx, "respond_to_elicitation", claim.OperationID, claim.RequestHash,
	); err != nil || found {
		return result, false, err
	}

	result := application.MutationResult[application.ElicitationResponseResult]{
		Data: application.ElicitationResponseResult{
			ElicitationRequestID: claim.ElicitationRequestID,
			Status:               string(agentconnection.ElicitationResponding),
		},
		Receipt: claim.Receipt,
	}

	update, err := tx.ExecContext(
		ctx,
		`UPDATE elicitation_requests SET state = 'responding', action = ?, responded_at = NULL
WHERE elicitation_request_id = ? AND state IN ('pending', 'failed')`,
		claim.Action,
		claim.ElicitationRequestID,
	)
	if err != nil {
		return application.MutationResult[application.ElicitationResponseResult]{}, false, err
	}

	if affected, err := update.RowsAffected(); err != nil || affected != 1 {
		if err != nil {
			return application.MutationResult[application.ElicitationResponseResult]{}, false, err
		}

		return application.MutationResult[application.ElicitationResponseResult]{}, false, &shared.Error{
			Code: "not_found", Message: "回答可能なelicitationがありません",
		}
	}

	encoded, err := json.Marshal(result)
	if err != nil {
		return application.MutationResult[application.ElicitationResponseResult]{}, false, err
	}

	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO operation_receipts
(scope, operation_id, request_hash, result_json, committed_at) VALUES (?, ?, ?, ?, ?)`,
		"respond_to_elicitation", claim.OperationID, claim.RequestHash,
		string(encoded), claim.Receipt.CommittedAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return application.MutationResult[application.ElicitationResponseResult]{}, false, err
	}

	if err := tx.Commit(); err != nil {
		return application.MutationResult[application.ElicitationResponseResult]{}, false, err
	}

	return result, true, nil
}

func (r *AgentConnectionRepository) CompleteElicitation(
	ctx context.Context,
	requestID string,
	operationID string,
	succeeded bool,
	completedAt time.Time,
) error {
	state := agentconnection.ElicitationFailed
	if succeeded {
		state = agentconnection.ElicitationResponded
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx, `UPDATE elicitation_requests SET state = ?, responded_at = ?
WHERE elicitation_request_id = ? AND state = 'responding'`, state, completedAt.Format(time.RFC3339Nano), requestID)
	if err != nil {
		return err
	}

	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		if err != nil {
			return err
		}

		return &shared.Error{Code: "not_found", Message: "応答中のelicitationがありません"}
	}

	event := application.OutboxEvent{
		ID:        "elicitation:" + requestID + ":" + operationID + ":" + completedAt.Format(time.RFC3339Nano),
		Name:      "agent.elicitation.updated",
		EmittedAt: completedAt, AggregateType: "elicitation", AggregateID: requestID,
	}
	if err := insertOutbox(ctx, tx, event, 1); err != nil {
		return err
	}

	if succeeded {
		var committedAt string
		if err := tx.QueryRowContext(ctx, `SELECT committed_at FROM operation_receipts
WHERE scope = 'respond_to_elicitation' AND operation_id = ?`, operationID).Scan(&committedAt); err != nil {
			return err
		}

		receiptCommittedAt, err := time.Parse(time.RFC3339Nano, committedAt)
		if err != nil {
			return err
		}

		result := application.MutationResult[application.ElicitationResponseResult]{
			Data: application.ElicitationResponseResult{
				ElicitationRequestID: requestID, Status: string(state), RespondedAt: completedAt,
			},
			Receipt: application.MutationReceipt{OperationID: operationID, CommittedAt: receiptCommittedAt},
		}

		encoded, err := json.Marshal(result)
		if err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx, `UPDATE operation_receipts SET result_json = ?
WHERE scope = 'respond_to_elicitation' AND operation_id = ?`, string(encoded), operationID); err != nil {
			return err
		}
	} else if _, err := tx.ExecContext(ctx, `DELETE FROM operation_receipts
WHERE scope = 'respond_to_elicitation' AND operation_id = ?`, operationID); err != nil {
		return err
	}

	return tx.Commit()
}

func existingResult[T any](
	ctx context.Context,
	tx *sql.Tx,
	scope string,
	operationID string,
	requestHash string,
) (application.MutationResult[T], bool, error) {
	var (
		storedHash, resultJSON string
		storedVersion          int
	)

	err := tx.QueryRowContext(ctx, `SELECT request_hash, request_hash_version, result_json FROM operation_receipts
WHERE scope = ? AND operation_id = ?`, scope, operationID).Scan(&storedHash, &storedVersion, &resultJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return application.MutationResult[T]{}, false, nil
	}

	if err != nil {
		return application.MutationResult[T]{}, false, err
	}

	if storedHash != requestHash || storedVersion != requestHashVersion(scope) {
		return application.MutationResult[T]{}, false, &shared.Error{
			Code: "operation_id_conflict", Message: "operationIdが別の入力で使用されています",
		}
	}

	var result application.MutationResult[T]
	if err := json.Unmarshal([]byte(resultJSON), &result); err != nil {
		return application.MutationResult[T]{}, false, err
	}

	return result, true, nil
}

func saveMutation[T any](
	ctx context.Context,
	tx *sql.Tx,
	scope string,
	operationID string,
	requestHash string,
	result application.MutationResult[T],
	event application.OutboxEvent,
	sequence int64,
) error {
	encoded, err := json.Marshal(result)
	if err != nil {
		return err
	}

	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO operation_receipts
(scope, operation_id, request_hash, request_hash_version, result_json, committed_at) VALUES (?, ?, ?, ?, ?, ?)`,
		scope, operationID, requestHash, requestHashVersion(scope), string(encoded),
		result.Receipt.CommittedAt.Format(time.RFC3339Nano),
	); err != nil {
		return err
	}

	return insertOutbox(ctx, tx, event, sequence)
}

func requestHashVersion(scope string) int {
	switch scope {
	case "create_project",
		"duplicate_project",
		"create_revision",
		"archive_project",
		"delete_project",
		"reconnect_project_session",
		"export_procedure":
		return 1
	default:
		return 0
	}
}

func insertOutbox(ctx context.Context, tx *sql.Tx, event application.OutboxEvent, sequence int64) error {
	correlation, payload, err := eventData(ctx, tx, event)
	if err != nil {
		return err
	}

	correlationJSON, err := json.Marshal(correlation)
	if err != nil {
		return err
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO event_outbox
(event_id, name, emitted_at, aggregate_type, aggregate_id, change_sequence, stream_key, stream_revision, correlation, payload_json)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		event.ID,
		event.Name,
		event.EmittedAt.Format(time.RFC3339Nano),
		event.AggregateType,
		event.AggregateID,
		sequence,
		event.AggregateType+":"+event.AggregateID,
		sequence,
		string(correlationJSON),
		string(payloadJSON),
	)

	return err
}

func eventData(ctx context.Context, tx *sql.Tx, event application.OutboxEvent) (map[string]string, any, error) {
	correlation := map[string]string{}

	switch event.Name {
	case "project.deleted":
		correlation["projectId"] = event.AggregateID
		return correlation, map[string]any{"projectId": event.AggregateID, "deleted": true}, nil
	case "project.changed":
		var status, stage string
		if err := tx.QueryRowContext(ctx, `SELECT status,current_stage FROM projects WHERE project_id=?`, event.AggregateID).
			Scan(&status, &stage); err != nil {
			return nil, nil, err
		}

		correlation["projectId"] = event.AggregateID

		return correlation, struct {
			ProjectID    string `json:"projectId"`
			Status       string `json:"status"`
			CurrentStage string `json:"currentStage"`
		}{event.AggregateID, status, stage}, nil
	case "session.connection_changed":
		var (
			projectID, sessionID, state string
			jobID                       string
		)
		if event.Correlation != "" && event.AggregateID != "" {
			jobID = event.Correlation
		}

		if err := tx.QueryRowContext(ctx, `SELECT project_id,session_id,state FROM acp_sessions WHERE session_id=? OR project_id=? ORDER BY created_at DESC LIMIT 1`, event.AggregateID, event.AggregateID).
			Scan(&projectID, &sessionID, &state); err != nil {
			return nil, nil, err
		}

		correlation["projectId"], correlation["sessionId"] = projectID, sessionID
		if event.AggregateType == "session" && event.AggregateID == sessionID && jobID != "" {
			correlation["jobId"] = jobID
		}

		return correlation, struct {
			ProjectID string `json:"projectId"`
			SessionID string `json:"sessionId"`
			State     string `json:"state"`
		}{projectID, sessionID, state}, nil
	case "export.updated":
		var (
			exportID, procedureID, status, displayName string
			errorCode                                  sql.NullString
		)
		if err := tx.QueryRowContext(ctx, `SELECT export_id,procedure_id,state,destination_display_name,error_code FROM exports WHERE export_id=? OR procedure_id=? ORDER BY accepted_at DESC LIMIT 1`, event.AggregateID, event.AggregateID).
			Scan(&exportID, &procedureID, &status, &displayName, &errorCode); err != nil {
			return nil, nil, err
		}

		correlation["procedureId"] = procedureID
		if event.AggregateID == exportID && event.Correlation != "" {
			correlation["jobId"] = event.Correlation
		}

		return correlation, struct {
			ExportID               string `json:"exportId"`
			Status                 string `json:"status"`
			DestinationDisplayName string `json:"destinationDisplayName"`
			ErrorSummary           string `json:"errorSummary,omitempty"`
		}{exportID, status, displayName, errorCode.String}, nil
	case "agent.connection.changed":
		var state string
		if err := tx.QueryRowContext(ctx, `SELECT auth_state FROM agent_connections WHERE connection_id=?`, event.AggregateID).
			Scan(&state); err != nil {
			return nil, nil, err
		}

		return correlation, struct {
			ConnectionID string `json:"connectionId"`
			AuthState    string `json:"authState"`
		}{event.AggregateID, state}, nil
	case "agent.authentication.updated":
		var connectionID, state, jobID string
		if err := tx.QueryRowContext(ctx, `SELECT target_id,state,job_id FROM agent_jobs WHERE job_id=? OR target_id=? ORDER BY accepted_at DESC LIMIT 1`, event.AggregateID, event.AggregateID).
			Scan(&connectionID, &state, &jobID); err != nil {
			return nil, nil, err
		}

		correlation["jobId"] = jobID

		return correlation, struct {
			ConnectionID string `json:"connectionId"`
			AuthState    string `json:"authState"`
			JobID        string `json:"jobId"`
		}{connectionID, state, jobID}, nil
	case "agent.elicitation.updated":
		var state string
		if err := tx.QueryRowContext(ctx, `SELECT state FROM elicitation_requests WHERE elicitation_request_id=?`, event.AggregateID).
			Scan(&state); err != nil {
			return nil, nil, err
		}

		return correlation, struct {
			ElicitationRequestID string `json:"elicitationRequestId"`
			Status               string `json:"status"`
		}{event.AggregateID, state}, nil
	default:
		return correlation, map[string]string{}, nil
	}
}

func getConnection(
	ctx context.Context,
	tx *sql.Tx,
	id string,
) (applicationConnection agentconnection.Connection, err error) {
	var argsJSON, verifiedAt, authState string

	err = tx.QueryRowContext(ctx, `SELECT connection_id, display_name, command, args_json, transport, resolved_executable_path,
last_verified_at, protocol_version, auth_state, schema_artifact_version, revision FROM agent_connections WHERE connection_id = ?`, id).
		Scan(&applicationConnection.ID, &applicationConnection.DisplayName, &applicationConnection.Command, &argsJSON,
			&applicationConnection.Transport, &applicationConnection.ResolvedExecutablePath, &verifiedAt,
			&applicationConnection.ProtocolVersion, &authState, &applicationConnection.SchemaArtifactVersion, &applicationConnection.Revision)
	if err != nil {
		return agentconnection.Connection{}, err
	}

	if err := json.Unmarshal([]byte(argsJSON), &applicationConnection.Args); err != nil {
		return agentconnection.Connection{}, fmt.Errorf("argsの読取に失敗しました: %w", err)
	}

	if applicationConnection.LastVerifiedAt, err = time.Parse(time.RFC3339Nano, verifiedAt); err != nil {
		return agentconnection.Connection{}, fmt.Errorf("last_verified_atの読取に失敗しました: %w", err)
	}

	applicationConnection.AuthState = agentconnection.AuthState(authState)

	return applicationConnection, nil
}
