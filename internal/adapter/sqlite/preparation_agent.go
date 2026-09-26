package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/yukihito-jokyu/TEJUN/internal/application"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/shared"
)

func (r *PreparationRepository) AcceptCancellation(
	ctx context.Context,
	in application.CancelAgentOperationRecord,
) (application.MutationResult[application.CancellationAccepted], error) {
	var zero application.MutationResult[application.CancellationAccepted]
	if in.SessionID == "" || in.OperationID == "" || (in.TurnID == "" && in.RunID == "" && in.JobID == "") {
		return zero, &shared.Error{Code: "validation_error", Message: "取消対象が必要です"}
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return zero, err
	}

	defer func() { _ = tx.Rollback() }()

	hash := digest(in.CancelAgentOperationInput)
	if out, found, e := existingResult[application.CancellationAccepted](
		ctx,
		tx,
		"cancel_agent_operation",
		in.OperationID,
		hash,
	); e != nil ||
		found {
		return out, e
	}

	var current string

	err = tx.QueryRowContext(ctx, `SELECT p.current_session_id FROM projects p JOIN preparation_sessions s ON s.project_id=p.project_id WHERE s.session_id=?`, in.SessionID).
		Scan(&current)
	if err != nil {
		return zero, notFound(err)
	}

	if current != in.SessionID {
		return zero, &shared.Error{Code: "invalid_state", Message: "現在のsessionではありません"}
	}

	if in.RunID != "" {
		return zero, &shared.Error{Code: "invalid_state", Message: "実行中runがありません"}
	}

	targetJob := in.JobID

	turnID := in.TurnID
	if turnID != "" {
		var id string

		err = tx.QueryRowContext(ctx, `SELECT job_id FROM preparation_jobs WHERE session_id=? AND turn_id=? AND kind='turn' AND state IN ('pending','running')`, in.SessionID, turnID).
			Scan(&id)
		if err != nil {
			return zero, notFound(err)
		}

		if targetJob != "" && targetJob != id {
			return zero, &shared.Error{Code: "validation_error", Message: "取消対象が一致しません"}
		}

		targetJob = id
	}

	var state string

	err = tx.QueryRowContext(ctx, `SELECT state FROM preparation_jobs WHERE job_id=? AND session_id=?`, targetJob, in.SessionID).
		Scan(&state)
	if err != nil {
		return zero, notFound(err)
	}

	if state != "pending" && state != "running" {
		return zero, &shared.Error{Code: "invalid_state", Message: "取消対象は既に完了しています"}
	}

	if state == "pending" && turnID != "" {
		at := in.RequestedAt.Format(time.RFC3339Nano)
		if _, err = tx.ExecContext(
			ctx,
			`UPDATE preparation_jobs SET state='cancelled',completed_at=? WHERE job_id=? AND state='pending'`,
			at,
			targetJob,
		); err != nil {
			return zero, err
		}

		if _, err = tx.ExecContext(
			ctx,
			`UPDATE preparation_turns SET state='cancelled',completed_at=? WHERE turn_id=? AND state='pending'`,
			at,
			turnID,
		); err != nil {
			return zero, err
		}

		if _, err = tx.ExecContext(
			ctx,
			`UPDATE preparation_messages SET status='cancelled' WHERE turn_id=? AND role='user'`,
			turnID,
		); err != nil {
			return zero, err
		}

		return mutation(ctx, tx, "cancel_agent_operation", in.OperationID, hash, application.CancellationAccepted{
			JobID:        in.CancellationJobID,
			TargetStatus: "cancelled",
			RequestedAt:  in.RequestedAt,
			Mechanism:    "local_cancel",
		}, in.RequestedAt, correlatedEvent(in.EventID, "session.turn.updated", "session", in.SessionID, in.RequestedAt, in.CancellationJobID))
	}

	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO preparation_jobs(job_id,session_id,turn_id,target_job_id,kind,state,accepted_at) VALUES(?,?,?,?, 'cancel','pending',?)`,
		in.CancellationJobID,
		in.SessionID,
		sql.NullString{String: turnID, Valid: turnID != ""},
		targetJob,
		in.RequestedAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return zero, err
	}

	mechanism := "rpc_cancel"
	if turnID != "" {
		mechanism = "session_cancel"
	}

	out := application.CancellationAccepted{
		JobID:        in.CancellationJobID,
		TargetStatus: "cancellation_requested",
		RequestedAt:  in.RequestedAt,
		Mechanism:    mechanism,
	}

	return mutation(
		ctx,
		tx,
		"cancel_agent_operation",
		in.OperationID,
		hash,
		out,
		in.RequestedAt,
		correlatedEvent(
			in.EventID,
			"session.turn.updated",
			"session",
			in.SessionID,
			in.RequestedAt,
			in.CancellationJobID,
		),
	)
}

func (r *PreparationRepository) ClaimSessionConfiguration(
	ctx context.Context,
	in application.SessionConfigurationClaim,
) (application.SessionConfigurationTarget, application.MutationResult[application.SessionSummary], bool, error) {
	var (
		target application.SessionConfigurationTarget
		zero   application.MutationResult[application.SessionSummary]
	)
	if in.SessionID == "" || in.OperationID == "" {
		return target, zero, false, &shared.Error{Code: "validation_error", Message: "sessionとoperationIdが必要です"}
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return target, zero, false, err
	}

	defer func() { _ = tx.Rollback() }()

	hash := digest(in.SetAgentSessionConfigurationInput)
	if out, found, e := existingResult[application.SessionSummary](
		ctx,
		tx,
		"set_session_configuration",
		in.OperationID,
		hash,
	); e != nil ||
		found {
		return target, out, false, e
	}

	var (
		current               int64
		modesJSON, configJSON string
	)

	err = tx.QueryRowContext(ctx, `SELECT s.revision,s.agent_session_id,p.workspace_path,s.modes_json,s.config_options_json,s.acp_receive_sequence FROM preparation_sessions s JOIN projects p ON p.current_session_id=s.session_id WHERE s.session_id=? AND s.state='ready'`, in.SessionID).
		Scan(&current, &target.AgentSessionID, &target.WorkspacePath, &modesJSON, &configJSON, &target.ReceiveSequence)
	if err != nil {
		return target, zero, false, notFound(err)
	}

	if current != in.ExpectedRevision {
		return target, zero, false, revisionError(current)
	}

	var (
		modes   *application.SessionModes
		options []application.SessionConfigOption
	)

	if err = json.Unmarshal([]byte(modesJSON), &modes); err != nil {
		return target, zero, false, err
	}

	if err = json.Unmarshal([]byte(configJSON), &options); err != nil {
		return target, zero, false, err
	}

	switch in.Change.Kind {
	case "mode":
		found := false

		if modes != nil {
			for _, v := range modes.Available {
				if v.ModeID == in.Change.ModeID {
					found = true
				}
			}
		}

		if !found {
			return target, zero, false, &shared.Error{Code: "validation_error", Message: "modeは提示されていません"}
		}
	case "config_option":
		found := false

		for _, v := range options {
			if v.ConfigID != in.Change.ConfigID {
				continue
			}

			if v.Type == "boolean" {
				_, found = in.Change.Value.(bool)
			} else if v.Type == "select" && v.Options != nil {
				value, ok := in.Change.Value.(string)
				if ok {
					for _, choice := range v.Options.Items {
						if choice.Value == value {
							found = true
						}
					}

					for _, group := range v.Options.Groups {
						for _, choice := range group.Options {
							if choice.Value == value {
								found = true
							}
						}
					}
				}
			}
		}

		if !found {
			return target, zero, false, &shared.Error{Code: "validation_error", Message: "optionは提示されていません"}
		}
	default:
		return target, zero, false, &shared.Error{Code: "validation_error", Message: "設定種別が不正です"}
	}

	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO session_configuration_claims(session_id,operation_id,request_hash,claimed_at) VALUES(?,?,?,?)`,
		in.SessionID,
		in.OperationID,
		hash,
		in.ClaimedAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return target, zero, false, &shared.Error{Code: "invalid_state", Message: "別の設定変更が進行中です"}
	}

	target.SessionID = in.SessionID
	target.Change = in.Change

	return target, zero, true, tx.Commit()
}

func (r *PreparationRepository) CompleteSessionConfiguration(
	ctx context.Context,
	in application.SessionConfigurationClaim,
	result application.SessionConfigurationResult,
) (application.MutationResult[application.SessionSummary], error) {
	var zero application.MutationResult[application.SessionSummary]

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return zero, err
	}

	defer func() { _ = tx.Rollback() }()

	var operationID string

	err = tx.QueryRowContext(ctx, `SELECT operation_id FROM session_configuration_claims WHERE session_id=?`, in.SessionID).
		Scan(&operationID)
	if err != nil {
		return zero, notFound(err)
	}

	if operationID != in.OperationID {
		return zero, &shared.Error{Code: "invalid_state", Message: "設定変更の所有が異なります"}
	}

	var modesJSON, optionsJSON string

	var receiveSequence int64

	err = tx.QueryRowContext(ctx, `SELECT modes_json,config_options_json,acp_receive_sequence FROM preparation_sessions WHERE session_id=?`, in.SessionID).
		Scan(&modesJSON, &optionsJSON, &receiveSequence)
	if err != nil {
		return zero, err
	}

	var e error

	if receiveSequence == in.ReceiveSequence {
		if in.Change.Kind == "mode" {
			modes, marshalErr := json.Marshal(result.Modes)
			if marshalErr != nil {
				return zero, marshalErr
			}

			modesJSON = string(modes)
		} else {
			options, marshalErr := json.Marshal(result.ConfigOptions)
			if marshalErr != nil {
				return zero, marshalErr
			}

			optionsJSON = string(options)
		}
	}

	_, err = tx.ExecContext(
		ctx,
		`UPDATE preparation_sessions SET modes_json=?,config_options_json=?,revision=revision+1 WHERE session_id=?`,
		modesJSON,
		optionsJSON,
		in.SessionID,
	)
	if err != nil {
		return zero, err
	}

	var (
		s                    application.SessionSummary
		at, capabilitiesJSON string
	)

	err = tx.QueryRowContext(ctx, `SELECT session_id,state,protocol_version,agent_name,agent_version,started_at,permission_mode,permission_revision,revision,change_sequence,capabilities_json FROM preparation_sessions WHERE session_id=?`, in.SessionID).
		Scan(&s.SessionID, &s.State, &s.ProtocolVersion, &s.AgentName, &s.AgentVersion, &at, &s.PermissionPolicy.Mode, &s.PermissionPolicy.Revision, &s.Revision, &s.ChangeSequence, &capabilitiesJSON)
	if err != nil {
		return zero, err
	}

	s.StartedAt, e = time.Parse(time.RFC3339Nano, at)
	if e != nil {
		return zero, e
	}

	s.PermissionPolicy.SessionID = s.SessionID
	if e = json.Unmarshal([]byte(modesJSON), &s.Modes); e != nil {
		return zero, e
	}

	if e = json.Unmarshal([]byte(optionsJSON), &s.ConfigOptions); e != nil {
		return zero, e
	}

	if e = json.Unmarshal([]byte(capabilitiesJSON), &s.Capabilities); e != nil {
		return zero, e
	}

	_, err = tx.ExecContext(ctx, `DELETE FROM session_configuration_claims WHERE session_id=?`, in.SessionID)
	if err != nil {
		return zero, err
	}

	return mutation(
		ctx,
		tx,
		"set_session_configuration",
		in.OperationID,
		digest(in.SetAgentSessionConfigurationInput),
		s,
		in.ClaimedAt,
		correlatedEvent(
			in.EventID,
			"session.configuration.changed",
			"session",
			in.SessionID,
			in.ClaimedAt,
			in.OperationID,
		),
	)
}

func (r *PreparationRepository) FailSessionConfiguration(
	ctx context.Context,
	in application.SessionConfigurationClaim,
) error {
	_, err := r.db.ExecContext(
		ctx,
		`DELETE FROM session_configuration_claims WHERE session_id=? AND operation_id=?`,
		in.SessionID,
		in.OperationID,
	)

	return err
}
