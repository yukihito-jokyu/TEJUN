package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/yukihito-jokyu/TEJUN/internal/application"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/execution"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/shared"
)

func (r *ExecutionRepository) GetExecutionView(
	ctx context.Context,
	in application.ExecutionViewQuery,
) (application.ExecutionView, error) {
	view := application.ExecutionView{
		Checks: []application.ExecutionCheckView{}, PendingPermissions: []application.PermissionRequestView{},
		Conversation: application.ConversationPage{Items: []application.ConversationItem{}},
		Readiness:    application.ExecutionReadiness{BlockingReasons: []application.ReadinessIssue{}},
	}

	if in.ConversationLimit == 0 {
		in.ConversationLimit = 50
	}

	if in.ConversationLimit < 1 || in.ConversationLimit > 100 {
		return view, &shared.Error{Code: "validation_failed", Message: "conversationLimitが範囲外です"}
	}

	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return view, err
	}

	defer func() { _ = tx.Rollback() }()

	snapshot, err := readExecution(ctx, tx, in.ProjectID, "", 1)
	if err != nil {
		return view, err
	}

	view.Execution = application.ExecutionSummary{
		ExecutionID: snapshot.ExecutionID,
		Status:      executionViewStatus(snapshot),
		Revision:    snapshot.Revision,
		StartedAt:   snapshot.StartedAt,
		CompletedAt: snapshot.CompletedAt,
	}
	view.ChangeSequence = snapshot.ChangeSequence

	var (
		created, updated                                                                                                                                                                                                    int64
		argsJSON                                                                                                                                                                                                            string
		viewConnectionID, viewConnectionDisplayName, viewConnectionCommand, viewConnectionTransport, viewConnectionResolvedPath, viewConnectionVerifiedAt, viewConnectionProtocol, viewConnectionAuth, viewConnectionSchema string
	)

	err = tx.QueryRowContext(ctx, `SELECT p.project_id,p.name,p.description,p.workspace_path,p.status,p.current_stage,p.revision,p.created_at,p.updated_at,
 c.connection_id,c.display_name,c.command,c.args_json,c.transport,c.resolved_executable_path,c.last_verified_at,c.protocol_version,c.auth_state,c.schema_artifact_version
 FROM projects p JOIN agent_connections c ON c.connection_id=p.connection_id WHERE p.project_id=?`, in.ProjectID).
		Scan(
			&view.Project.ProjectID, &view.Project.Name, &view.Project.Description, &view.Project.WorkspacePath, &view.Project.Status, &view.Project.CurrentStage, &view.Project.Revision, &created, &updated,
			&viewConnectionID, &viewConnectionDisplayName, &viewConnectionCommand, &argsJSON, &viewConnectionTransport, &viewConnectionResolvedPath, &viewConnectionVerifiedAt, &viewConnectionProtocol, &viewConnectionAuth, &viewConnectionSchema)
	if err != nil {
		return view, err
	}

	view.Project.CreatedAt = time.UnixMicro(created).UTC()
	view.Project.UpdatedAt = time.UnixMicro(updated).UTC()

	connection := application.AgentConnectionSummary{
		ConnectionID:           viewConnectionID,
		DisplayName:            viewConnectionDisplayName,
		Command:                viewConnectionCommand,
		Transport:              viewConnectionTransport,
		ResolvedExecutablePath: viewConnectionResolvedPath,
		LastVerifiedAt:         viewConnectionVerifiedAt,
		ProtocolVersion:        viewConnectionProtocol,
		AuthState:              viewConnectionAuth,
		SchemaArtifactVersion:  viewConnectionSchema,
		Args:                   []string{},
	}
	if err = json.Unmarshal([]byte(argsJSON), &connection.Args); err != nil {
		return view, err
	}

	view.Project.Connection = &connection

	var (
		started, modesJSON, configJSON, capsJSON string
		disconnected                             sql.NullString
	)

	err = tx.QueryRowContext(ctx, `SELECT session_id,state,protocol_version,agent_name,agent_version,started_at,disconnected_at,permission_mode,permission_revision,revision,change_sequence,modes_json,config_options_json,capabilities_json FROM preparation_sessions WHERE session_id=? AND project_id=?`, snapshot.SessionID, in.ProjectID).
		Scan(
			&view.Session.SessionID, &view.Session.State, &view.Session.ProtocolVersion, &view.Session.AgentName, &view.Session.AgentVersion, &started, &disconnected, &view.Session.PermissionPolicy.Mode, &view.Session.PermissionPolicy.Revision, &view.Session.Revision, &view.Session.ChangeSequence, &modesJSON, &configJSON, &capsJSON)
	if err != nil {
		return view, err
	}

	view.Session.PermissionPolicy.SessionID = view.Session.SessionID
	if view.Session.StartedAt, err = time.Parse(time.RFC3339Nano, started); err != nil {
		return view, err
	}

	if disconnected.Valid {
		t, e := time.Parse(time.RFC3339Nano, disconnected.String)
		if e != nil {
			return view, e
		}

		view.Session.DisconnectedAt = &t
	}

	if err = json.Unmarshal([]byte(modesJSON), &view.Session.Modes); err != nil {
		return view, err
	}

	if err = json.Unmarshal([]byte(configJSON), &view.Session.ConfigOptions); err != nil {
		return view, err
	}

	if err = json.Unmarshal([]byte(capsJSON), &view.Session.Capabilities); err != nil {
		return view, err
	}

	if view.Session.ConfigOptions == nil {
		view.Session.ConfigOptions = []application.SessionConfigOption{}
	}

	if view.Session.Capabilities == nil {
		view.Session.Capabilities = map[string]any{}
	}

	if view.Session.Modes != nil && view.Session.Modes.Available == nil {
		view.Session.Modes.Available = []application.SessionMode{}
	}

	if snapshot.ActiveRunID != "" {
		run := application.RunSummary{TargetedCheckIDs: []string{}}

		var (
			accepted string
			finished sql.NullString
		)

		err = tx.QueryRowContext(ctx, `SELECT run_id,state,accepted_at,completed_at FROM execution_runs WHERE run_id=? AND execution_id=?`, snapshot.ActiveRunID, snapshot.ExecutionID).
			Scan(&run.RunID, &run.Status, &accepted, &finished)
		if err != nil {
			return view, err
		}

		t, e := time.Parse(time.RFC3339Nano, accepted)
		if e != nil {
			return view, e
		}

		run.StartedAt = &t
		if finished.Valid {
			t, e = time.Parse(time.RFC3339Nano, finished.String)
			if e != nil {
				return view, e
			}

			run.FinishedAt = &t
		}

		rows, e := tx.QueryContext(
			ctx,
			`SELECT check_id FROM execution_run_checks WHERE run_id=? ORDER BY check_id`,
			run.RunID,
		)
		if e != nil {
			return view, e
		}

		for rows.Next() {
			var id string
			if e = rows.Scan(&id); e != nil {
				_ = rows.Close()
				return view, e
			}

			run.TargetedCheckIDs = append(run.TargetedCheckIDs, id)
		}

		e = rows.Err()
		_ = rows.Close()

		if e != nil {
			return view, e
		}

		view.ActiveRun = &run
	}

	for _, check := range snapshot.Checks {
		view.Checks = append(view.Checks, application.ExecutionCheckFromSnapshot(check))
	}

	for _, p := range snapshot.Permissions {
		item := application.PermissionRequestView{
			PermissionRequestID: p.ID,
			SessionID:           p.SessionID,
			RunID:               p.RunID,
			ToolCall: application.ToolCallPresentation{
				ToolCallID: p.ToolCallID,
				Title:      p.Title,
				Locations:  []application.ToolCallLocation{},
				Details:    []application.ToolCallDetail{},
			},
			Options:     p.Options,
			Status:      p.Status,
			RequestedAt: p.RequestedAt,
			ExpiresAt:   p.ExpiresAt,
		}
		if item.Options == nil {
			item.Options = []execution.PermissionOption{}
		}

		view.PendingPermissions = append(view.PendingPermissions, item)
	}

	for _, reason := range snapshot.BlockingReasons {
		code, checkID, _ := strings.Cut(reason, ":")

		issue := application.ReadinessIssue{Code: code, Message: code, CheckID: checkID}
		if code == "evidence_missing" {
			for _, check := range snapshot.Checks {
				if check.ID == checkID {
					issue.EvidenceRequirement = check.HumanEvidenceRequirement
					break
				}
			}
		}

		view.Readiness.BlockingReasons = append(view.Readiness.BlockingReasons, issue)
	}

	view.Readiness.CanGenerateProcedure = snapshot.CanGenerate
	query := `SELECT turn_id,role,text,status,created_at,rowid FROM execution_turns WHERE execution_id=?`
	args := []any{snapshot.ExecutionID}

	if in.ConversationCursor != nil {
		query += ` AND rowid < ?`

		args = append(args, *in.ConversationCursor)
	}

	query += ` ORDER BY rowid DESC LIMIT ?`

	args = append(args, in.ConversationLimit+1)

	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return view, err
	}

	for rows.Next() {
		var (
			m         application.ConversationItem
			value, at string
		)
		if err = rows.Scan(&m.TurnID, &m.Role, &value, &m.Status, &at, &m.Sequence); err != nil {
			_ = rows.Close()
			return view, err
		}

		m.MessageID = m.TurnID

		m.Content = []application.ContentPart{{Type: "text", Text: value}}
		if m.CreatedAt, err = time.Parse(time.RFC3339Nano, at); err != nil {
			_ = rows.Close()
			return view, err
		}

		view.Conversation.Items = append(view.Conversation.Items, m)
	}

	err = rows.Err()
	_ = rows.Close()

	if err != nil {
		return view, err
	}

	if len(view.Conversation.Items) > in.ConversationLimit {
		view.Conversation.HasPrevious = true
		view.Conversation.Items = view.Conversation.Items[:in.ConversationLimit]
	}

	for i, j := 0, len(view.Conversation.Items)-1; i < j; i, j = i+1, j-1 {
		view.Conversation.Items[i], view.Conversation.Items[j] = view.Conversation.Items[j], view.Conversation.Items[i]
	}

	if view.Conversation.HasPrevious {
		v := view.Conversation.Items[0].Sequence
		view.Conversation.PreviousCursor = &v
	}

	if err = tx.Commit(); err != nil {
		return view, err
	}

	return view, nil
}

func executionViewStatus(snapshot execution.Snapshot) string {
	switch snapshot.Status {
	case "completed":
		return "completed"
	case "cancelled":
		return "interrupted"
	case "active":
		if snapshot.ActiveRunID != "" {
			return "running"
		}

		awaitingHuman := false

		for _, check := range snapshot.Checks {
			if check.AIStatus == "failed" {
				return "failed"
			}

			if check.AIStatus == "completed" && check.HumanStatus != "completed" {
				awaitingHuman = true
			}
		}

		if awaitingHuman {
			return "awaiting_human"
		}

		return "ready"
	default:
		return snapshot.Status
	}
}
