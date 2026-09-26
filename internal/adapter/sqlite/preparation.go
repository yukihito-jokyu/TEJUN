package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/yukihito-jokyu/TEJUN/internal/application"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/shared"
)

type PreparationRepository struct{ db *sql.DB }

func NewPreparationRepository(db *sql.DB) *PreparationRepository { return &PreparationRepository{db} }

func (r *PreparationRepository) GetPreparation(
	ctx context.Context,
	in application.PreparationViewQuery,
) (application.PreparationView, error) {
	view := application.PreparationView{
		CheckPlan:    application.CheckPlanView{Items: []application.CheckItem{}},
		Conversation: application.ConversationPage{Items: []application.ConversationItem{}},
		Chats:        []application.PreparationChat{},
		Elicitations: []application.ElicitationRequestView{},
		Readiness:    application.Readiness{BlockingReasons: []application.ReadinessIssue{}},
	}

	if in.ConversationLimit == 0 {
		in.ConversationLimit = 50
	}

	if in.ConversationLimit < 1 || in.ConversationLimit > 100 {
		return view, &shared.Error{Code: "validation_error", Message: "conversationLimitが範囲外です"}
	}

	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return view, err
	}

	defer func() { _ = tx.Rollback() }()

	var (
		created, updated int64
		currentSession   string
	)

	err = tx.QueryRowContext(ctx, `SELECT project_id,name,description,workspace_path,status,current_stage,revision,created_at,updated_at,COALESCE(current_session_id,'') FROM projects WHERE project_id=?`, in.ProjectID).
		Scan(&view.Project.ProjectID, &view.Project.Name, &view.Project.Description, &view.Project.WorkspacePath, &view.Project.Status, &view.Project.CurrentStage, &view.Project.Revision, &created, &updated, &currentSession)
	if err != nil {
		return view, notFound(err)
	}

	chatRows, err := tx.QueryContext(
		ctx,
		`SELECT s.session_id,s.started_at,COALESCE((SELECT m.content_json FROM preparation_messages m WHERE m.session_id=s.session_id AND m.role='user' ORDER BY m.sequence LIMIT 1),'') FROM preparation_sessions s WHERE s.project_id=? ORDER BY s.started_at DESC,s.session_id DESC`,
		in.ProjectID,
	)
	if err != nil {
		return view, err
	}

	for chatRows.Next() {
		var (
			chat             application.PreparationChat
			started, content string
		)
		if err = chatRows.Scan(&chat.SessionID, &started, &content); err != nil {
			_ = chatRows.Close()
			return view, err
		}

		if chat.StartedAt, err = time.Parse(time.RFC3339Nano, started); err != nil {
			_ = chatRows.Close()
			return view, err
		}

		chat.Title = "新しいチャット"

		if content != "" {
			var parts []application.ContentPart
			if err = json.Unmarshal([]byte(content), &parts); err != nil {
				_ = chatRows.Close()
				return view, err
			}

			if len(parts) > 0 && parts[0].Text != "" {
				chat.Title = parts[0].Text
				if len([]rune(chat.Title)) > 40 {
					chat.Title = string([]rune(chat.Title)[:40]) + "…"
				}
			}
		}

		view.Chats = append(view.Chats, chat)
	}

	if err = chatRows.Err(); err != nil {
		_ = chatRows.Close()
		return view, err
	}

	_ = chatRows.Close()

	chatSession := currentSession
	if in.ChatID != "" {
		chatSession = ""

		for _, chat := range view.Chats {
			if chat.SessionID == in.ChatID {
				chatSession = in.ChatID
				break
			}
		}

		if chatSession == "" {
			return view, &shared.Error{Code: "not_found", Message: "チャットがありません"}
		}
	}

	view.Project.CreatedAt = time.UnixMicro(created).UTC()
	view.Project.UpdatedAt = time.UnixMicro(updated).UTC()

	var argsJSON string

	connection := application.AgentConnectionSummary{Args: []string{}}

	err = tx.QueryRowContext(ctx, `SELECT c.connection_id,c.display_name,c.command,c.args_json,c.transport,c.resolved_executable_path,c.last_verified_at,c.protocol_version,c.auth_state,c.schema_artifact_version FROM agent_connections c JOIN projects p ON p.connection_id=c.connection_id WHERE p.project_id=?`, in.ProjectID).
		Scan(&connection.ConnectionID, &connection.DisplayName, &connection.Command, &argsJSON, &connection.Transport, &connection.ResolvedExecutablePath, &connection.LastVerifiedAt, &connection.ProtocolVersion, &connection.AuthState, &connection.SchemaArtifactVersion)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return view, err
	}

	if err == nil {
		if err = json.Unmarshal([]byte(argsJSON), &connection.Args); err != nil {
			return view, err
		}

		view.Project.Connection = &connection
	}

	var criteriaJSON string

	err = tx.QueryRowContext(ctx, `SELECT purpose,completion_criteria_json,intended_users,revision FROM preparations WHERE project_id=?`, in.ProjectID).
		Scan(&view.Preparation.Purpose, &criteriaJSON, &view.Preparation.IntendedUsers, &view.Preparation.Revision)
	if err != nil {
		return view, err
	}

	if err = json.Unmarshal([]byte(criteriaJSON), &view.Preparation.CompletionCriteria); err != nil {
		return view, err
	}

	err = tx.QueryRowContext(ctx, `SELECT project_id,revision FROM check_plans WHERE project_id=?`, in.ProjectID).
		Scan(&view.CheckPlan.PlanID, &view.CheckPlan.Revision)
	if err != nil {
		return view, err
	}

	rows, err := tx.QueryContext(
		ctx,
		`SELECT check_id,sequence,title,instruction,expected_result,suggested_command,ai_required,human_required,human_evidence_requirement FROM check_items WHERE project_id=? ORDER BY sequence`,
		in.ProjectID,
	)
	if err != nil {
		return view, err
	}

	for rows.Next() {
		var item application.CheckItem
		if err = rows.Scan(
			&item.CheckID,
			&item.Sequence,
			&item.Title,
			&item.Instruction,
			&item.ExpectedResult,
			&item.SuggestedCommand,
			&item.AIRequired,
			&item.HumanRequired,
			&item.HumanEvidenceRequirement,
		); err != nil {
			_ = rows.Close()
			return view, err
		}

		view.CheckPlan.Items = append(view.CheckPlan.Items, item)
	}

	err = rows.Err()
	_ = rows.Close()

	if err != nil {
		return view, err
	}

	if currentSession != "" {
		s := application.SessionSummary{}

		var (
			started, modesJSON, configJSON, capabilitiesJSON string
			disconnected                                     sql.NullString
		)

		err = tx.QueryRowContext(ctx, `SELECT session_id,state,protocol_version,agent_name,agent_version,started_at,disconnected_at,permission_mode,permission_revision,revision,change_sequence,modes_json,config_options_json,capabilities_json FROM preparation_sessions WHERE session_id=?`, currentSession).
			Scan(&s.SessionID, &s.State, &s.ProtocolVersion, &s.AgentName, &s.AgentVersion, &started, &disconnected, &s.PermissionPolicy.Mode, &s.PermissionPolicy.Revision, &s.Revision, &s.ChangeSequence, &modesJSON, &configJSON, &capabilitiesJSON)
		if err != nil {
			return view, err
		}

		s.StartedAt, err = time.Parse(time.RFC3339Nano, started)
		if err != nil {
			return view, err
		}

		if disconnected.Valid {
			v, e := time.Parse(time.RFC3339Nano, disconnected.String)
			if e != nil {
				return view, e
			}

			s.DisconnectedAt = &v
		}

		s.PermissionPolicy.SessionID = s.SessionID
		if err = json.Unmarshal([]byte(modesJSON), &s.Modes); err != nil {
			return view, err
		}

		if err = json.Unmarshal([]byte(configJSON), &s.ConfigOptions); err != nil {
			return view, err
		}

		if err = json.Unmarshal([]byte(capabilitiesJSON), &s.Capabilities); err != nil {
			return view, err
		}

		view.Session = &s
		query := `SELECT message_id,turn_id,role,content_json,status,created_at,sequence FROM preparation_messages WHERE session_id=?`
		args := []any{chatSession}

		if in.ConversationCursor != nil {
			query += ` AND sequence < ?`

			args = append(args, *in.ConversationCursor)
		}

		query += ` ORDER BY sequence DESC LIMIT ?`

		args = append(args, in.ConversationLimit+1)

		mrows, e := tx.QueryContext(ctx, query, args...)
		if e != nil {
			return view, e
		}

		for mrows.Next() {
			var (
				m           application.ConversationItem
				content, at string
			)
			if e = mrows.Scan(&m.MessageID, &m.TurnID, &m.Role, &content, &m.Status, &at, &m.Sequence); e != nil {
				_ = mrows.Close()
				return view, e
			}

			if e = json.Unmarshal([]byte(content), &m.Content); e != nil {
				_ = mrows.Close()
				return view, e
			}

			m.CreatedAt, e = time.Parse(time.RFC3339Nano, at)
			if e != nil {
				_ = mrows.Close()
				return view, e
			}

			view.Conversation.Items = append(view.Conversation.Items, m)
		}

		e = mrows.Err()
		_ = mrows.Close()

		if e != nil {
			return view, e
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
	}

	if currentSession != "" {
		erows, e := tx.QueryContext(
			ctx,
			`SELECT elicitation_request_id,mode,message,state,requested_at,expires_at,scope_json,requested_schema_json,COALESCE(url,'') FROM elicitation_requests WHERE session_id=? AND state IN ('pending','responding') ORDER BY requested_at`,
			currentSession,
		)
		if e != nil {
			return view, e
		}

		for erows.Next() {
			var (
				v               application.ElicitationRequestView
				at, scope       string
				expires, schema sql.NullString
			)
			if e = erows.Scan(
				&v.ElicitationRequestID,
				&v.Mode,
				&v.Message,
				&v.Status,
				&at,
				&expires,
				&scope,
				&schema,
				&v.URL,
			); e != nil {
				_ = erows.Close()
				return view, e
			}

			v.SessionID = currentSession

			v.RequestedAt, e = time.Parse(time.RFC3339Nano, at)
			if e != nil {
				_ = erows.Close()
				return view, e
			}

			if expires.Valid {
				t, ee := time.Parse(time.RFC3339Nano, expires.String)
				if ee != nil {
					_ = erows.Close()
					return view, ee
				}

				v.ExpiresAt = &t
			}

			if e = json.Unmarshal([]byte(scope), &v.Scope); e != nil {
				_ = erows.Close()
				return view, e
			}

			if schema.Valid {
				if e = json.Unmarshal([]byte(schema.String), &v.RequestedSchema); e != nil {
					_ = erows.Close()
					return view, e
				}
			}

			view.Elicitations = append(view.Elicitations, v)
		}

		e = erows.Err()
		_ = erows.Close()

		if e != nil {
			return view, e
		}
	}

	var activeJobs int
	if currentSession != "" {
		err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM preparation_jobs WHERE session_id=? AND state IN ('pending','running')`, currentSession).
			Scan(&activeJobs)
		if err != nil {
			return view, err
		}
	}

	view.Readiness = readiness(view.Preparation, view.CheckPlan, view.Session, activeJobs)

	err = tx.QueryRowContext(ctx, `SELECT change_sequence FROM app_settings WHERE singleton=1`).
		Scan(&view.ChangeSequence)
	if err != nil {
		return view, err
	}

	return view, tx.Commit()
}

func readiness(
	brief application.PreparationBrief,
	plan application.CheckPlanView,
	session *application.SessionSummary,
	activeJobs int,
) application.Readiness {
	r := application.Readiness{CanStartExecution: true, BlockingReasons: []application.ReadinessIssue{}}

	add := func(code, msg string) {
		r.CanStartExecution = false
		r.BlockingReasons = append(r.BlockingReasons, application.ReadinessIssue{Code: code, Message: msg})
	}
	if brief.Purpose == "" {
		add("purpose_required", "目的を入力してください")
	}

	if len(brief.CompletionCriteria) == 0 {
		add("criteria_required", "完了条件を入力してください")
	}

	if brief.IntendedUsers == "" {
		add("users_required", "利用者を入力してください")
	}

	if len(plan.Items) == 0 {
		add("plan_required", "チェック項目を追加してください")
	}

	if session == nil || session.State != "ready" {
		add("session_not_ready", "Agent接続が必要です")
	}

	if activeJobs > 0 {
		add("operation_in_progress", "Agentの処理完了を待ってください")
	}

	return r
}

func transactionReadiness(
	ctx context.Context,
	tx *sql.Tx,
	projectID string,
	brief application.PreparationBrief,
	items []application.CheckItem,
) (application.Readiness, error) {
	if brief.Purpose == "" {
		var criteriaJSON string

		err := tx.QueryRowContext(ctx, `SELECT purpose,completion_criteria_json,intended_users FROM preparations WHERE project_id=?`, projectID).
			Scan(&brief.Purpose, &criteriaJSON, &brief.IntendedUsers)
		if err != nil {
			return application.Readiness{}, err
		}

		if err = json.Unmarshal([]byte(criteriaJSON), &brief.CompletionCriteria); err != nil {
			return application.Readiness{}, err
		}
	}

	if items == nil {
		items = []application.CheckItem{}

		rows, err := tx.QueryContext(ctx, `SELECT check_id FROM check_items WHERE project_id=?`, projectID)
		if err != nil {
			return application.Readiness{}, err
		}

		for rows.Next() {
			var item application.CheckItem
			if err = rows.Scan(&item.CheckID); err != nil {
				_ = rows.Close()
				return application.Readiness{}, err
			}

			items = append(items, item)
		}

		err = rows.Err()
		_ = rows.Close()

		if err != nil {
			return application.Readiness{}, err
		}
	}

	var (
		state      sql.NullString
		activeJobs int
	)

	err := tx.QueryRowContext(ctx, `SELECT s.state,(SELECT COUNT(*) FROM preparation_jobs j WHERE j.session_id=s.session_id AND j.state IN ('pending','running')) FROM projects p LEFT JOIN preparation_sessions s ON s.session_id=p.current_session_id WHERE p.project_id=?`, projectID).
		Scan(&state, &activeJobs)
	if err != nil {
		return application.Readiness{}, err
	}

	var session *application.SessionSummary
	if state.Valid {
		session = &application.SessionSummary{State: state.String}
	}

	return readiness(brief, application.CheckPlanView{Items: items}, session, activeJobs), nil
}

func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return &shared.Error{Code: "not_found", Message: "対象がありません"}
	}

	return err
}

func revisionError(current int64) error {
	return &shared.Error{Code: "revision_conflict", Message: "更新内容が競合しました", CurrentRevision: &current}
}

func receipt(id string, at time.Time) application.MutationReceipt {
	return application.MutationReceipt{OperationID: id, CommittedAt: at}
}

func event(id, name, kind, aggregate string, at time.Time) application.OutboxEvent {
	return application.OutboxEvent{ID: id, Name: name, AggregateType: kind, AggregateID: aggregate, EmittedAt: at}
}

func correlatedEvent(id, name, kind, aggregate string, at time.Time, correlation string) application.OutboxEvent {
	e := event(id, name, kind, aggregate, at)
	e.Correlation = correlation

	return e
}

func sequence(ctx context.Context, tx *sql.Tx) (int64, error) {
	var n int64

	err := tx.QueryRowContext(ctx, `UPDATE app_settings SET change_sequence=change_sequence+1 WHERE singleton=1 RETURNING change_sequence`).
		Scan(&n)

	return n, err
}

func mutation[T any](
	ctx context.Context,
	tx *sql.Tx,
	scope, op, hash string,
	data T,
	at time.Time,
	ev application.OutboxEvent,
) (application.MutationResult[T], error) {
	out := application.MutationResult[T]{Data: data, Receipt: receipt(op, at)}

	seq, err := sequence(ctx, tx)
	if err != nil {
		return out, err
	}

	if err = saveMutation(ctx, tx, scope, op, hash, out, ev, seq); err != nil {
		return out, err
	}

	return out, tx.Commit()
}
func begin(ctx context.Context, db *sql.DB) (*sql.Tx, error) { return db.BeginTx(ctx, nil) }

func digest(
	v any,
) string {
	b, _ := json.Marshal(v)
	return requestDigestParts(string(b))
}
func requestDigestParts(v string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(v))) }

func (r *PreparationRepository) SavePreparationBrief(
	ctx context.Context,
	in application.SavePreparationBriefRecord,
) (application.MutationResult[application.PreparationUpdate], error) {
	var zero application.MutationResult[application.PreparationUpdate]
	if in.Brief.Purpose == "" || in.Brief.IntendedUsers == "" || len(in.Brief.CompletionCriteria) == 0 {
		return zero, &shared.Error{Code: "validation_error", Message: "briefの必須項目がありません"}
	}

	tx, err := begin(ctx, r.db)
	if err != nil {
		return zero, err
	}

	defer func() { _ = tx.Rollback() }()

	hash := digest(in.SavePreparationBriefInput)
	if out, found, e := existingResult[application.PreparationUpdate](
		ctx,
		tx,
		"save_brief",
		in.OperationID,
		hash,
	); e != nil ||
		found {
		return out, e
	}

	var current int64

	err = tx.QueryRowContext(ctx, `SELECT revision FROM preparations WHERE project_id=?`, in.ProjectID).Scan(&current)
	if err != nil {
		return zero, notFound(err)
	}

	if current != in.ExpectedPreparationRevision {
		return zero, revisionError(current)
	}

	criteria, e := json.Marshal(in.Brief.CompletionCriteria)
	if e != nil {
		return zero, e
	}

	_, err = tx.ExecContext(
		ctx,
		`UPDATE preparations SET purpose=?,completion_criteria_json=?,intended_users=?,revision=revision+1 WHERE project_id=?`,
		in.Brief.Purpose,
		string(criteria),
		in.Brief.IntendedUsers,
		in.ProjectID,
	)
	if err != nil {
		return zero, err
	}

	in.Brief.Revision = current + 1
	out := application.PreparationUpdate{
		ProjectID:           in.ProjectID,
		PreparationRevision: current + 1,
		Brief:               in.Brief,
		AffectedCheckIDs:    []string{},
		Readiness:           application.Readiness{BlockingReasons: []application.ReadinessIssue{}},
		UpdatedAt:           in.UpdatedAt,
	}

	out.Readiness, err = transactionReadiness(ctx, tx, in.ProjectID, in.Brief, nil)
	if err != nil {
		return zero, err
	}

	return mutation(
		ctx,
		tx,
		"save_brief",
		in.OperationID,
		hash,
		out,
		in.UpdatedAt,
		event(in.EventID, "preparation.changed", "preparation", in.ProjectID, in.UpdatedAt),
	)
}

func (r *PreparationRepository) SaveCheckPlan(
	ctx context.Context,
	in application.SaveCheckPlanRecord,
) (application.MutationResult[application.SavedCheckPlan], error) {
	var zero application.MutationResult[application.SavedCheckPlan]

	seenKey := map[string]bool{}
	seenID := map[string]bool{}

	for _, item := range in.Items {
		if item.ClientKey == "" || seenKey[item.ClientKey] || item.Title == "" || item.ExpectedResult == "" {
			return zero, &shared.Error{Code: "validation_error", Message: "check itemが不正です"}
		}

		seenKey[item.ClientKey] = true
		if item.CheckID != "" {
			if seenID[item.CheckID] {
				return zero, &shared.Error{Code: "validation_error", Message: "checkIdが重複しています"}
			}

			seenID[item.CheckID] = true
		}

		switch item.HumanEvidenceRequirement {
		case "none", "text", "image", "text_or_image":
		default:
			return zero, &shared.Error{Code: "validation_error", Message: "evidence要件が不正です"}
		}
	}

	tx, err := begin(ctx, r.db)
	if err != nil {
		return zero, err
	}

	defer func() { _ = tx.Rollback() }()

	hash := digest(in.SaveCheckPlanInput)
	if out, found, e := existingResult[application.SavedCheckPlan](
		ctx,
		tx,
		"save_plan",
		in.OperationID,
		hash,
	); e != nil ||
		found {
		return out, e
	}

	var (
		prepRev, planRev int64
		planID           string
	)

	err = tx.QueryRowContext(ctx, `SELECT p.revision,c.revision,c.project_id FROM preparations p JOIN check_plans c USING(project_id) WHERE project_id=?`, in.ProjectID).
		Scan(&prepRev, &planRev, &planID)
	if err != nil {
		return zero, notFound(err)
	}

	if prepRev != in.ExpectedPreparationRevision {
		return zero, revisionError(prepRev)
	}

	if planRev != in.ExpectedPlanRevision {
		return zero, revisionError(planRev)
	}

	oldRows, e := tx.QueryContext(ctx, `SELECT check_id FROM check_items WHERE project_id=?`, in.ProjectID)
	if e != nil {
		return zero, e
	}

	old := map[string]bool{}

	for oldRows.Next() {
		var id string
		if e = oldRows.Scan(&id); e != nil {
			_ = oldRows.Close()
			return zero, e
		}

		old[id] = true
	}

	e = oldRows.Err()
	_ = oldRows.Close()

	if e != nil {
		return zero, e
	}

	for _, item := range in.Items {
		if item.CheckID != "" && !old[item.CheckID] {
			return zero, &shared.Error{Code: "validation_error", Message: "checkIdがplanに属しません"}
		}
	}

	_, err = tx.ExecContext(ctx, `DELETE FROM check_items WHERE project_id=?`, in.ProjectID)
	if err != nil {
		return zero, err
	}

	out := application.SavedCheckPlan{
		ProjectID:           in.ProjectID,
		PreparationRevision: prepRev,
		PlanRevision:        planRev + 1,
		Items:               []application.CheckItem{},
		AssignedIDs:         []application.AssignedCheckID{},
		Readiness:           application.Readiness{BlockingReasons: []application.ReadinessIssue{}},
		UpdatedAt:           in.UpdatedAt,
	}
	for i, item := range in.Items {
		id := item.CheckID
		if id == "" {
			id = in.NewIDs[i]
			out.AssignedIDs = append(
				out.AssignedIDs,
				application.AssignedCheckID{ClientKey: item.ClientKey, CheckID: id},
			)
		}

		ci := application.CheckItem{
			CheckID:                  id,
			Sequence:                 i + 1,
			Title:                    item.Title,
			Instruction:              item.Instruction,
			ExpectedResult:           item.ExpectedResult,
			SuggestedCommand:         item.SuggestedCommand,
			AIRequired:               item.AIRequired,
			HumanRequired:            item.HumanRequired,
			HumanEvidenceRequirement: item.HumanEvidenceRequirement,
		}

		_, e = tx.ExecContext(
			ctx,
			`INSERT INTO check_items(check_id,project_id,sequence,title,instruction,expected_result,suggested_command,ai_required,human_required,human_evidence_requirement) VALUES(?,?,?,?,?,?,?,?,?,?)`,
			id,
			in.ProjectID,
			ci.Sequence,
			ci.Title,
			ci.Instruction,
			ci.ExpectedResult,
			ci.SuggestedCommand,
			ci.AIRequired,
			ci.HumanRequired,
			ci.HumanEvidenceRequirement,
		)
		if e != nil {
			return zero, e
		}

		out.Items = append(out.Items, ci)
	}

	_, err = tx.ExecContext(ctx, `UPDATE check_plans SET revision=revision+1 WHERE project_id=?`, in.ProjectID)
	if err != nil {
		return zero, err
	}

	out.Readiness, err = transactionReadiness(ctx, tx, in.ProjectID, application.PreparationBrief{}, out.Items)
	if err != nil {
		return zero, err
	}

	return mutation(
		ctx,
		tx,
		"save_plan",
		in.OperationID,
		hash,
		out,
		in.UpdatedAt,
		event(in.EventID, "preparation.changed", "preparation", in.ProjectID, in.UpdatedAt),
	)
}

func (r *PreparationRepository) SaveSessionPermissionPolicy(
	ctx context.Context,
	in application.SaveSessionPermissionPolicyRecord,
) (application.MutationResult[application.SessionPermissionPolicy], error) {
	var zero application.MutationResult[application.SessionPermissionPolicy]
	if in.Mode != "ask_every_time" && in.Mode != "reuse_explicit_always_choice" {
		return zero, &shared.Error{Code: "validation_error", Message: "permission policyが不正です"}
	}

	tx, err := begin(ctx, r.db)
	if err != nil {
		return zero, err
	}

	defer func() { _ = tx.Rollback() }()

	hash := digest(in.SaveSessionPermissionPolicyInput)
	if out, found, e := existingResult[application.SessionPermissionPolicy](
		ctx,
		tx,
		"save_policy",
		in.OperationID,
		hash,
	); e != nil ||
		found {
		return out, e
	}

	var current, policyRevision int64

	err = tx.QueryRowContext(ctx, `SELECT s.revision,s.permission_revision FROM preparation_sessions s JOIN projects p ON p.current_session_id=s.session_id WHERE s.session_id=?`, in.SessionID).
		Scan(&current, &policyRevision)
	if err != nil {
		return zero, notFound(err)
	}

	if current != in.ExpectedSessionRevision {
		return zero, revisionError(current)
	}

	_, err = tx.ExecContext(
		ctx,
		`UPDATE preparation_sessions SET permission_mode=?,permission_revision=permission_revision+1,revision=revision+1 WHERE session_id=?`,
		in.Mode,
		in.SessionID,
	)
	if err != nil {
		return zero, err
	}

	out := application.SessionPermissionPolicy{SessionID: in.SessionID, Mode: in.Mode, Revision: policyRevision + 1}

	return mutation(
		ctx,
		tx,
		"save_policy",
		in.OperationID,
		hash,
		out,
		in.UpdatedAt,
		event(in.EventID, "session.policy.changed", "session", in.SessionID, in.UpdatedAt),
	)
}

func (r *PreparationRepository) SendPreparationMessage(
	ctx context.Context,
	in application.SendPreparationMessageRecord,
) (application.MutationResult[application.AcceptedTurn], error) {
	var zero application.MutationResult[application.AcceptedTurn]
	if len(in.Content) == 0 {
		return zero, &shared.Error{Code: "validation_error", Message: "messageが空です"}
	}

	for _, part := range in.Content {
		if part.Type != "text" || part.Text == "" {
			return zero, &shared.Error{Code: "validation_error", Message: "contentが不正です"}
		}
	}

	tx, err := begin(ctx, r.db)
	if err != nil {
		return zero, err
	}

	defer func() { _ = tx.Rollback() }()

	hash := digest(in.SendPreparationMessageInput)
	if out, found, e := existingResult[application.AcceptedTurn](
		ctx,
		tx,
		"send_preparation_message",
		in.OperationID,
		hash,
	); e != nil ||
		found {
		return out, e
	}

	var current sql.NullString

	err = tx.QueryRowContext(ctx, `SELECT current_session_id FROM projects WHERE project_id=?`, in.ProjectID).
		Scan(&current)
	if err != nil {
		return zero, notFound(err)
	}

	sessionID := in.SessionID
	if current.Valid {
		if sessionID != "" && sessionID != current.String {
			return zero, &shared.Error{Code: "invalid_state", Message: "現在のsessionではありません"}
		}

		sessionID = current.String
	} else {
		if sessionID != "" {
			return zero, &shared.Error{Code: "invalid_state", Message: "現在のsessionではありません"}
		}

		sessionID = in.SessionIDNew

		_, err = tx.ExecContext(
			ctx,
			`INSERT INTO preparation_sessions(session_id,project_id,state,started_at) VALUES(?,?,'connecting',?)`,
			sessionID,
			in.ProjectID,
			in.AcceptedAt.Format(time.RFC3339Nano),
		)
		if err != nil {
			return zero, err
		}

		_, err = tx.ExecContext(
			ctx,
			`UPDATE projects SET current_session_id=? WHERE project_id=?`,
			sessionID,
			in.ProjectID,
		)
		if err != nil {
			return zero, err
		}

		_, err = tx.ExecContext(
			ctx,
			`INSERT INTO preparation_jobs(job_id,session_id,kind,state,accepted_at) VALUES(?,? ,'connect','pending',?)`,
			in.SessionIDNew+":connect",
			sessionID,
			in.AcceptedAt.Format(time.RFC3339Nano),
		)
		if err != nil {
			return zero, err
		}
	}

	content, e := json.Marshal(in.Content)
	if e != nil {
		return zero, e
	}

	var msgSeq int64

	err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(sequence),0)+1 FROM preparation_messages WHERE session_id=?`, sessionID).
		Scan(&msgSeq)
	if err != nil {
		return zero, err
	}

	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO preparation_messages(message_id,session_id,turn_id,role,content_json,status,created_at,sequence) VALUES(?,?,?,'user',?,'pending',?,?)`,
		in.MessageID,
		sessionID,
		in.TurnID,
		string(content),
		in.AcceptedAt.Format(time.RFC3339Nano),
		msgSeq,
	)
	if err != nil {
		return zero, err
	}

	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO preparation_turns(turn_id,session_id,state,accepted_at,brief_revision,plan_revision) VALUES(?,?,'pending',?,(SELECT revision FROM preparations WHERE project_id=?),(SELECT revision FROM check_plans WHERE project_id=?))`,
		in.TurnID,
		sessionID,
		in.AcceptedAt.Format(time.RFC3339Nano),
		in.ProjectID,
		in.ProjectID,
	)
	if err != nil {
		return zero, err
	}

	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO preparation_jobs(job_id,session_id,turn_id,kind,state,accepted_at) VALUES(?,?,?,'turn','pending',?)`,
		in.JobID,
		sessionID,
		in.TurnID,
		in.AcceptedAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return zero, err
	}

	out := application.AcceptedTurn{SessionID: sessionID, TurnID: in.TurnID, JobID: in.JobID, AcceptedAt: in.AcceptedAt}

	return mutation(
		ctx,
		tx,
		"send_preparation_message",
		in.OperationID,
		hash,
		out,
		in.AcceptedAt,
		correlatedEvent(in.EventID, "session.turn.updated", "session", sessionID, in.AcceptedAt, in.JobID),
	)
}

func (r *PreparationRepository) ChangeProjectWorkspace(
	ctx context.Context,
	in application.ChangeProjectWorkspaceRecord,
) (application.MutationResult[application.WorkspaceChangeAccepted], error) {
	var zero application.MutationResult[application.WorkspaceChangeAccepted]
	if !in.ConfirmSessionReset || in.NewWorkspacePath == "" {
		return zero, &shared.Error{Code: "validation_error", Message: "workspace変更の確認が必要です"}
	}

	tx, err := begin(ctx, r.db)
	if err != nil {
		return zero, err
	}

	defer func() { _ = tx.Rollback() }()

	hash := digest(in.ChangeProjectWorkspaceInput)
	if out, found, e := existingResult[application.WorkspaceChangeAccepted](
		ctx,
		tx,
		"change_workspace",
		in.OperationID,
		hash,
	); e != nil ||
		found {
		return out, e
	}

	var (
		current int64
		old     sql.NullString
	)

	err = tx.QueryRowContext(ctx, `SELECT revision,current_session_id FROM projects WHERE project_id=?`, in.ProjectID).
		Scan(&current, &old)
	if err != nil {
		return zero, notFound(err)
	}

	if current != in.ExpectedProjectRevision {
		return zero, revisionError(current)
	}

	if old.Valid {
		var active int

		err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM preparation_jobs WHERE session_id=? AND state IN ('pending','running')`, old.String).
			Scan(&active)
		if err != nil {
			return zero, err
		}

		if active > 0 {
			return zero, &shared.Error{Code: "invalid_state", Message: "実行中のAgent操作があります"}
		}

		_, err = tx.ExecContext(
			ctx,
			`UPDATE preparation_sessions SET state='interrupted',disconnected_at=?,revision=revision+1 WHERE session_id=?`,
			in.AcceptedAt.Format(time.RFC3339Nano),
			old.String,
		)
		if err != nil {
			return zero, err
		}
	}

	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO preparation_sessions(session_id,previous_session_id,project_id,state,started_at) VALUES(?,?,?,'connecting',?)`,
		in.SessionID,
		old.String,
		in.ProjectID,
		in.AcceptedAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return zero, err
	}

	_, err = tx.ExecContext(
		ctx,
		`UPDATE projects SET workspace_path=?,revision=revision+1,current_session_id=?,updated_at=? WHERE project_id=?`,
		in.NewWorkspacePath,
		in.SessionID,
		in.AcceptedAt.UnixMicro(),
		in.ProjectID,
	)
	if err != nil {
		return zero, err
	}

	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO preparation_jobs(job_id,session_id,kind,state,accepted_at) VALUES(?,?,'connect','pending',?)`,
		in.JobID,
		in.SessionID,
		in.AcceptedAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return zero, err
	}

	out := application.WorkspaceChangeAccepted{
		ProjectID:              in.ProjectID,
		ProjectRevision:        current + 1,
		SessionID:              in.SessionID,
		JobID:                  in.JobID,
		State:                  "reconnecting",
		ConversationContinuity: "reset",
		AcceptedAt:             in.AcceptedAt,
	}
	if old.Valid {
		out.PreviousSessionID = old.String
	}

	return mutation(
		ctx,
		tx,
		"change_workspace",
		in.OperationID,
		hash,
		out,
		in.AcceptedAt,
		correlatedEvent(in.EventID, "project.changed", "project", in.ProjectID, in.AcceptedAt, in.JobID),
	)
}

func (r *PreparationRepository) StartExecution(
	ctx context.Context,
	in application.StartExecutionRecord,
) (application.MutationResult[application.ExecutionStarted], error) {
	var zero application.MutationResult[application.ExecutionStarted]

	tx, err := begin(ctx, r.db)
	if err != nil {
		return zero, err
	}

	defer func() { _ = tx.Rollback() }()

	hash := digest(in.StartExecutionInput)
	if out, found, e := existingResult[application.ExecutionStarted](
		ctx,
		tx,
		"start_execution",
		in.OperationID,
		hash,
	); e != nil ||
		found {
		return out, e
	}

	var (
		session                  sql.NullString
		purpose, criteria, users string
		prepRev, planRev         int64
	)

	err = tx.QueryRowContext(ctx, `SELECT p.current_session_id,b.purpose,b.completion_criteria_json,b.intended_users,b.revision,c.revision FROM projects p JOIN preparations b USING(project_id) JOIN check_plans c USING(project_id) WHERE p.project_id=? AND p.current_stage='preparation'`, in.ProjectID).
		Scan(&session, &purpose, &criteria, &users, &prepRev, &planRev)
	if err != nil {
		return zero, notFound(err)
	}

	if prepRev != in.ExpectedPreparationRevision {
		return zero, revisionError(prepRev)
	}

	if planRev != in.ExpectedPlanRevision {
		return zero, revisionError(planRev)
	}

	var count int

	err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM check_items WHERE project_id=?`, in.ProjectID).Scan(&count)
	if err != nil {
		return zero, err
	}

	var state string
	if session.Valid {
		err = tx.QueryRowContext(ctx, `SELECT state FROM preparation_sessions WHERE session_id=?`, session.String).
			Scan(&state)
		if err != nil {
			return zero, err
		}
	}

	var activeJobs int
	if session.Valid {
		err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM preparation_jobs WHERE session_id=? AND state IN ('pending','running')`, session.String).
			Scan(&activeJobs)
		if err != nil {
			return zero, err
		}
	}

	var crit []string
	if err = json.Unmarshal([]byte(criteria), &crit); err != nil {
		return zero, err
	}

	if purpose == "" || users == "" || len(crit) == 0 || count == 0 || state != "ready" || activeJobs > 0 {
		return zero, &shared.Error{Code: "not_ready", Message: "準備条件を満たしていません"}
	}

	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO executions(execution_id,project_id,revision,session_id,purpose,completion_criteria_json,intended_users,preparation_revision,plan_revision,started_at) VALUES(?,?,1,?,?,?,?,?,?,?)`,
		in.ExecutionID,
		in.ProjectID,
		session.String,
		purpose,
		criteria,
		users,
		prepRev,
		planRev,
		in.StartedAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return zero, err
	}

	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO execution_checks(execution_id,check_id,sequence,title,instruction,expected_result,suggested_command,ai_required,human_required,human_evidence_requirement) SELECT ?,check_id,sequence,title,instruction,expected_result,suggested_command,ai_required,human_required,human_evidence_requirement FROM check_items WHERE project_id=?`,
		in.ExecutionID,
		in.ProjectID,
	)
	if err != nil {
		return zero, err
	}

	_, err = tx.ExecContext(
		ctx,
		`UPDATE projects SET current_stage='execution',status='ai_running',revision=revision+1,updated_at=? WHERE project_id=?`,
		in.StartedAt.UnixMicro(),
		in.ProjectID,
	)
	if err != nil {
		return zero, err
	}

	out := application.ExecutionStarted{
		ProjectID:         in.ProjectID,
		ExecutionID:       in.ExecutionID,
		SessionID:         session.String,
		ExecutionRevision: 1,
		NextRoute:         "#/projects/" + in.ProjectID + "/check",
		StartedAt:         in.StartedAt,
	}

	return mutation(
		ctx,
		tx,
		"start_execution",
		in.OperationID,
		hash,
		out,
		in.StartedAt,
		event(in.EventID, "project.changed", "project", in.ProjectID, in.StartedAt),
	)
}

func (r *PreparationRepository) ClaimPreparationJob(ctx context.Context) (*application.ClaimedPreparationJob, error) {
	return r.claimPreparationJob(ctx, false)
}

func (r *PreparationRepository) ClaimPreparationCancellation(
	ctx context.Context,
) (*application.ClaimedPreparationJob, error) {
	return r.claimPreparationJob(ctx, true)
}

func (r *PreparationRepository) claimPreparationJob(
	ctx context.Context,
	cancellation bool,
) (*application.ClaimedPreparationJob, error) {
	tx, err := begin(ctx, r.db)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	job := application.ClaimedPreparationJob{}

	var argsJSON string

	kindFilter := "kind != 'cancel'"
	if cancellation {
		kindFilter = "kind = 'cancel'"
	}

	err = tx.QueryRowContext(ctx, `UPDATE preparation_jobs SET state='running' WHERE job_id=(SELECT job_id FROM preparation_jobs WHERE state='pending' AND `+kindFilter+` ORDER BY CASE kind WHEN 'connect' THEN 0 ELSE 1 END,accepted_at,job_id LIMIT 1) RETURNING job_id,kind,session_id,COALESCE(turn_id,''),target_job_id,run_id`).
		Scan(&job.JobID, &job.Kind, &job.SessionID, &job.TurnID, &job.TargetJobID, &job.RunID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	err = tx.QueryRowContext(ctx, `SELECT s.agent_session_id,s.previous_session_id,p.workspace_path,c.display_name,c.command,c.args_json,c.transport FROM preparation_sessions s JOIN projects p ON p.project_id=s.project_id JOIN agent_connections c ON c.connection_id=p.connection_id WHERE s.session_id=?`, job.SessionID).
		Scan(&job.AgentSessionID, &job.PreviousSessionID, &job.WorkspacePath, &job.Connection.DisplayName, &job.Connection.Command, &argsJSON, &job.Connection.Transport)
	if err != nil {
		return nil, err
	}

	if err = json.Unmarshal([]byte(argsJSON), &job.Connection.Args); err != nil {
		return nil, err
	}

	if job.Kind == "turn" {
		var content string

		err = tx.QueryRowContext(ctx, `SELECT content_json FROM preparation_messages WHERE turn_id=? AND role='user' ORDER BY sequence DESC LIMIT 1`, job.TurnID).
			Scan(&content)
		if err != nil {
			return nil, err
		}

		if err = json.Unmarshal([]byte(content), &job.Content); err != nil {
			return nil, err
		}

		_, err = tx.ExecContext(
			ctx,
			`UPDATE preparation_turns SET state='running' WHERE turn_id=? AND state='pending'`,
			job.TurnID,
		)
		if err != nil {
			return nil, err
		}

		_, err = tx.ExecContext(
			ctx,
			`UPDATE preparation_messages SET status='streaming' WHERE turn_id=? AND role='user'`,
			job.TurnID,
		)
		if err != nil {
			return nil, err
		}

		_, err = tx.ExecContext(
			ctx,
			`UPDATE preparation_sessions SET state='busy' WHERE session_id=? AND state='ready'`,
			job.SessionID,
		)
		if err != nil {
			return nil, err
		}
	}

	return &job, tx.Commit()
}

func (r *PreparationRepository) CompletePreparationJob(
	ctx context.Context,
	c application.PreparationJobCompletion,
) error {
	tx, err := begin(ctx, r.db)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	state := "failed"
	if c.Success {
		state = "succeeded"
	}

	var kind, turnID string

	err = tx.QueryRowContext(ctx, `SELECT kind,COALESCE(turn_id,'') FROM preparation_jobs WHERE job_id=? AND session_id=? AND state='running'`, c.JobID, c.SessionID).
		Scan(&kind, &turnID)
	if err != nil {
		return notFound(err)
	}

	_, err = tx.ExecContext(
		ctx,
		`UPDATE preparation_jobs SET state=?,completed_at=? WHERE job_id=?`,
		state,
		c.CompletedAt.Format(time.RFC3339Nano),
		c.JobID,
	)
	if err != nil {
		return err
	}

	if kind == "connect" {
		sessionState := "failed"
		if c.Success {
			sessionState = "ready"
		}

		modesJSON, e := json.Marshal(c.Modes)
		if e != nil {
			return e
		}

		configJSON, e := json.Marshal(c.ConfigOptions)
		if e != nil {
			return e
		}

		capabilitiesJSON, e := json.Marshal(c.Capabilities)
		if e != nil {
			return e
		}

		_, err = tx.ExecContext(
			ctx,
			`UPDATE preparation_sessions SET state=?,agent_session_id=?,protocol_version=?,agent_name=?,agent_version=?,modes_json=CASE WHEN acp_receive_sequence=0 THEN ? ELSE modes_json END,config_options_json=CASE WHEN acp_receive_sequence=0 THEN ? ELSE config_options_json END,capabilities_json=?,revision=revision+1 WHERE session_id=?`,
			sessionState,
			c.AgentSessionID,
			c.ProtocolVersion,
			c.AgentName,
			c.AgentVersion,
			string(modesJSON),
			string(configJSON),
			string(capabilitiesJSON),
			c.SessionID,
		)
		if err != nil {
			return err
		}
	}

	if kind == "turn" {
		turnState := "failed"
		if c.Success {
			turnState = "completed"
		}

		if c.StopReason == "cancelled" {
			turnState = "cancelled"
		}

		if c.StopReason == "interrupted" {
			turnState = "interrupted"
		}

		_, err = tx.ExecContext(
			ctx,
			`UPDATE preparation_turns SET state=?,completed_at=? WHERE turn_id=? AND state='running'`,
			turnState,
			c.CompletedAt.Format(time.RFC3339Nano),
			turnID,
		)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(
			ctx,
			`UPDATE preparation_messages SET status=? WHERE turn_id=? AND role='user'`,
			turnState,
			turnID,
		)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(
			ctx,
			`UPDATE preparation_sessions SET state='ready' WHERE session_id=? AND state='busy'`,
			c.SessionID,
		)
		if err != nil {
			return err
		}

		for _, m := range c.Messages {
			var seq int64

			err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(sequence),0)+1 FROM preparation_messages WHERE session_id=?`, c.SessionID).
				Scan(&seq)
			if err != nil {
				return err
			}

			content, e := json.Marshal(m.Content)
			if e != nil {
				return e
			}

			_, err = tx.ExecContext(
				ctx,
				`INSERT INTO preparation_messages(message_id,session_id,turn_id,role,content_json,status,created_at,sequence) VALUES(?,?,?,?,?,?,?,?)`,
				m.MessageID,
				c.SessionID,
				turnID,
				m.Role,
				string(content),
				m.Status,
				m.CreatedAt.Format(time.RFC3339Nano),
				seq,
			)
			if err != nil {
				return err
			}
		}

		if c.Success && c.BriefSuggestion != nil {
			if err := applyBriefSuggestion(ctx, tx, turnID, *c.BriefSuggestion); err != nil {
				return err
			}

			if err := applyCheckSuggestion(ctx, tx, turnID, c.BriefSuggestion.CheckItems); err != nil {
				return err
			}
		}
	}

	_, err = sequence(ctx, tx)
	if err != nil {
		return err
	}

	if err = insertOutbox(
		ctx,
		tx,
		correlatedEvent(
			"preparation-job:"+c.JobID,
			"session.turn.updated",
			"session",
			c.SessionID,
			c.CompletedAt,
			c.JobID,
		),
		1,
	); err != nil {
		return err
	}

	return tx.Commit()
}

func applyBriefSuggestion(
	ctx context.Context,
	tx *sql.Tx,
	turnID string,
	suggestion application.PreparationBriefSuggestion,
) error {
	if suggestion.Purpose == "" && suggestion.IntendedUsers == "" && len(suggestion.CompletionCriteria) == 0 {
		return nil
	}

	var (
		projectID                    string
		revision                     int64
		purpose, users, criteriaJSON string
	)
	if err := tx.QueryRowContext(ctx, `SELECT p.project_id,t.brief_revision,p.purpose,p.intended_users,p.completion_criteria_json FROM preparation_turns t JOIN preparation_sessions s USING(session_id) JOIN preparations p ON p.project_id=s.project_id WHERE t.turn_id=?`, turnID).
		Scan(&projectID, &revision, &purpose, &users, &criteriaJSON); err != nil {
		return err
	}

	if suggestion.Purpose != "" {
		purpose = suggestion.Purpose
	}

	if suggestion.IntendedUsers != "" {
		users = suggestion.IntendedUsers
	}

	if len(suggestion.CompletionCriteria) > 0 {
		encoded, err := json.Marshal(suggestion.CompletionCriteria)
		if err != nil {
			return err
		}

		criteriaJSON = string(encoded)
	}

	_, err := tx.ExecContext(
		ctx,
		`UPDATE preparations SET purpose=?,intended_users=?,completion_criteria_json=?,revision=revision+1 WHERE project_id=? AND revision=?`,
		purpose,
		users,
		criteriaJSON,
		projectID,
		revision,
	)

	return err
}

func applyCheckSuggestion(
	ctx context.Context,
	tx *sql.Tx,
	turnID string,
	items []application.PreparationCheckSuggestion,
) error {
	if len(items) == 0 || len(items) > 30 {
		return nil
	}

	for _, item := range items {
		if item.Title == "" || item.ExpectedResult == "" {
			return nil
		}
	}

	var (
		projectID string
		revision  int64
	)
	if err := tx.QueryRowContext(ctx, `SELECT s.project_id,t.plan_revision FROM preparation_turns t JOIN preparation_sessions s USING(session_id) WHERE t.turn_id=?`, turnID).
		Scan(&projectID, &revision); err != nil {
		return err
	}

	result, err := tx.ExecContext(
		ctx,
		`UPDATE check_plans SET revision=revision+1 WHERE project_id=? AND revision=?`,
		projectID,
		revision,
	)
	if err != nil {
		return err
	}

	changed, err := result.RowsAffected()
	if err != nil || changed == 0 {
		return err
	}

	if _, err = tx.ExecContext(ctx, `DELETE FROM check_items WHERE project_id=?`, projectID); err != nil {
		return err
	}

	for i, item := range items {
		if _, err = tx.ExecContext(
			ctx,
			`INSERT INTO check_items(check_id,project_id,sequence,title,instruction,expected_result,suggested_command,ai_required,human_required,human_evidence_requirement) VALUES(?,?,?,?,?,?,?,0,1,'none')`,
			fmt.Sprintf("%s:%d", turnID, i+1),
			projectID,
			i+1,
			item.Title,
			item.Instruction,
			item.ExpectedResult,
			item.SuggestedCommand,
		); err != nil {
			return err
		}
	}

	return nil
}

func (r *PreparationRepository) FailInterruptedPreparationJobs(ctx context.Context, at time.Time) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	timestamp := at.Format(time.RFC3339Nano)
	if _, err = tx.ExecContext(
		ctx,
		`UPDATE preparation_turns SET state='interrupted',completed_at=? WHERE turn_id IN (SELECT turn_id FROM preparation_jobs WHERE kind='turn' AND state='running')`,
		timestamp,
	); err != nil {
		return err
	}

	if _, err = tx.ExecContext(
		ctx,
		`UPDATE preparation_sessions SET state='failed',disconnected_at=?,revision=revision+1 WHERE session_id IN (SELECT session_id FROM preparation_jobs WHERE kind='connect' AND state='running')`,
		timestamp,
	); err != nil {
		return err
	}

	if _, err = tx.ExecContext(
		ctx,
		`UPDATE preparation_jobs SET state='failed',completed_at=? WHERE state='running'`,
		timestamp,
	); err != nil {
		return err
	}

	if _, err = tx.ExecContext(ctx, `DELETE FROM session_configuration_claims`); err != nil {
		return err
	}

	return tx.Commit()
}
