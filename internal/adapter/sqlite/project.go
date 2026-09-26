package sqlite

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/yukihito-jokyu/TEJUN/internal/application"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/project"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/shared"
	"github.com/yukihito-jokyu/TEJUN/internal/trace"
)

type ProjectRepository struct {
	db       *sql.DB
	evidence *EvidenceStore
}

func NewProjectRepository(db *sql.DB) *ProjectRepository { return &ProjectRepository{db: db} }

func (r *ProjectRepository) SetEvidenceStore(store *EvidenceStore) { r.evidence = store }

type projectCursor struct {
	Version  int    `json:"v"`
	Sequence int64  `json:"q"`
	Sort     string `json:"s"`
	Filter   string `json:"f"`
	Value    string `json:"x"`
	ID       string `json:"i"`
}

func (r *ProjectRepository) ListProjects(
	ctx context.Context,
	query application.ProjectListQuery,
) (application.ProjectListResult, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return application.ProjectListResult{}, err
	}
	defer func() { _ = tx.Rollback() }()

	result := application.ProjectListResult{Items: []application.ProjectSummary{}, GeneratedAt: time.Now().UTC()}
	if err := tx.QueryRowContext(ctx, `SELECT change_sequence FROM app_settings WHERE singleton = 1`).
		Scan(&result.ChangeSequence); err != nil {
		return result, err
	}

	where := []string{"p.archived_at IS NULL"}
	args := []any{}

	if len(query.Statuses) > 0 {
		placeholders := make([]string, len(query.Statuses))
		for i, status := range query.Statuses {
			placeholders[i] = "?"

			args = append(args, status)
		}

		where = append(where, "p.status IN ("+strings.Join(placeholders, ",")+")")
	}

	if query.Search != "" {
		pattern := "%" + escapeLike(query.Search) + "%"

		where = append(where, `(p.name LIKE ? ESCAPE '\' OR p.description LIKE ? ESCAPE '\')`)
		args = append(args, pattern, pattern)
	}

	base := " FROM projects p WHERE " + strings.Join(where, " AND ")
	sortValue := "p.updated_at"
	direction := "DESC"

	switch query.Sort {
	case "attention_desc":
		sortValue = attentionSQL
	case "name_asc":
		sortValue = "p.name COLLATE NOCASE"
		direction = "ASC"
	}

	filter, _ := json.Marshal([]any{query.Search, query.Statuses})
	filterKey := base64.RawURLEncoding.EncodeToString(filter)

	pageArgs := append([]any{}, args...)

	if query.Cursor != "" {
		if len(query.Cursor) > 2048 {
			return result, invalidCursor()
		}

		encoded, err := base64.RawURLEncoding.DecodeString(query.Cursor)
		if err != nil {
			return result, invalidCursor()
		}

		var cursor projectCursor
		if json.Unmarshal(encoded, &cursor) != nil || cursor.Version != 2 || cursor.Sort != query.Sort ||
			cursor.Filter != filterKey ||
			cursor.ID == "" {
			return result, invalidCursor()
		}

		if cursor.Sequence != result.ChangeSequence {
			return result, &shared.Error{Code: "revision_conflict", Message: "一覧が更新されました。最初から読み直してください"}
		}

		operator := "<"
		if direction == "ASC" {
			operator = ">"
		}

		base += fmt.Sprintf(
			" AND ((%s) %s ? OR ((%s) = ? AND p.project_id %s ?))",
			sortValue,
			operator,
			sortValue,
			operator,
		)

		var cursorValue any = cursor.Value

		switch query.Sort {
		case "attention_desc":
			rank, err := strconv.Atoi(cursor.Value)
			if err != nil || rank < 0 || rank > 5 {
				return result, invalidCursor()
			}

			cursorValue = rank
		case "updated_desc":
			updated, err := strconv.ParseInt(cursor.Value, 10, 64)
			if err != nil {
				return result, invalidCursor()
			}

			cursorValue = updated
		}

		pageArgs = append(pageArgs, cursorValue, cursorValue, cursor.ID)
	}

	countBase := " FROM projects p WHERE " + strings.Join(where, " AND ")
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*)"+countBase, args...).Scan(&result.Total); err != nil {
		return result, err
	}

	statement := `SELECT p.project_id,p.name,p.description,p.workspace_path,p.status,p.current_stage,p.revision,p.updated_at,p.completed_at,
 p.error_code, (SELECT procedure_id FROM procedures WHERE project_id=p.project_id AND status='completed' ORDER BY completed_at DESC, procedure_id DESC LIMIT 1),
 (SELECT revision FROM procedures WHERE project_id=p.project_id AND status='completed' ORDER BY completed_at DESC, procedure_id DESC LIMIT 1),
 (SELECT state FROM acp_sessions WHERE project_id=p.project_id ORDER BY created_at DESC LIMIT 1), (` + sortValue + `) AS sort_value` + base +
		" ORDER BY sort_value " + direction + ", p.project_id " + direction + " LIMIT ?"

	rows, err := tx.QueryContext(ctx, statement, append(pageArgs, query.Limit+1)...)
	if err != nil {
		return result, err
	}

	defer func() { _ = rows.Close() }()

	type listed struct {
		summary application.ProjectSummary
		sort    string
	}

	items := []listed{}

	for rows.Next() {
		var (
			item                                    application.ProjectSummary
			updated                                 int64
			completed                               sql.NullInt64
			errorCode, procedureID, connectionState sql.NullString
			procedureRevision                       sql.NullInt64
			sort                                    any
		)
		if err := rows.Scan(
			&item.ProjectID,
			&item.Name,
			&item.Description,
			&item.WorkspacePath,
			&item.Status,
			&item.CurrentStage,
			&item.Revision,
			&updated,
			&completed,
			&errorCode,
			&procedureID,
			&procedureRevision,
			&connectionState,
			&sort,
		); err != nil {
			return result, err
		}

		item.UpdatedAt = time.UnixMicro(updated).UTC()
		if completed.Valid {
			value := time.UnixMicro(completed.Int64).UTC()
			item.CompletedAt = &value
		}

		if errorCode.Valid {
			item.ErrorSummary = &errorCode.String
		}

		if procedureID.Valid {
			item.CurrentProcedureID = &procedureID.String
			if procedureRevision.Valid {
				item.CurrentProcedureRevision = &procedureRevision.Int64
			}
		}

		item.ConnectionState = "disconnected"
		if connectionState.Valid {
			item.ConnectionState = connectionState.String
		}

		item.ResumeRoute = project.ResumeRoute(item.ProjectID, item.CurrentStage)
		item.AttentionRank, item.AttentionReason = attention(item.Status)
		item.Progress = progress(item.CurrentStage)

		switch value := sort.(type) {
		case int64:
			items = append(items, listed{item, strconv.FormatInt(value, 10)})
		case string:
			items = append(items, listed{item, value})
		default:
			return result, fmt.Errorf("unsupported project sort value %T", sort)
		}
	}

	if err := rows.Err(); err != nil {
		return result, err
	}

	if len(items) > query.Limit {
		last := items[query.Limit-1]
		encoded, _ := json.Marshal(
			projectCursor{
				Version:  2,
				Sequence: result.ChangeSequence,
				Sort:     query.Sort,
				Filter:   filterKey,
				Value:    last.sort,
				ID:       last.summary.ProjectID,
			},
		)
		cursor := base64.RawURLEncoding.EncodeToString(encoded)
		result.NextCursor = &cursor
		items = items[:query.Limit]
	}

	for _, item := range items {
		result.Items = append(result.Items, item.summary)
	}

	if err := tx.Commit(); err != nil {
		return application.ProjectListResult{}, err
	}

	return result, nil
}

const attentionSQL = `CASE p.status WHEN 'human_waiting' THEN 5 WHEN 'error' THEN 4 WHEN 'ai_running' THEN 3 WHEN 'procedure_editing' THEN 2 WHEN 'preparing' THEN 1 ELSE 0 END`

func attention(status project.Status) (int, *string) {
	var (
		rank   int
		reason string
	)

	switch status {
	case project.HumanWaiting:
		rank, reason = 5, "人間の確認待ち"
	case project.Error:
		rank, reason = 4, "確認が必要"
	case project.AIRunning:
		rank, reason = 3, "AI実行中"
	case project.ProcedureEditing:
		rank = 2
	case project.Preparing:
		rank = 1
	}

	if reason == "" {
		return rank, nil
	}

	return rank, &reason
}

func progress(stage string) application.ProjectProgress {
	switch stage {
	case "execution":
		return application.ProjectProgress{Completed: 1, Total: 3}
	case "procedure":
		return application.ProjectProgress{Completed: 2, Total: 3}
	case "completed":
		return application.ProjectProgress{Completed: 3, Total: 3}
	default:
		return application.ProjectProgress{Completed: 0, Total: 3}
	}
}

func escapeLike(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `%`, `\%`)

	return strings.ReplaceAll(value, `_`, `\_`)
}

func invalidCursor() error {
	return &shared.Error{
		Code:        "validation_error",
		Message:     "cursorが不正です",
		FieldErrors: map[string]string{"cursor": "不正なcursorです"},
	}
}

func (r *ProjectRepository) CreateProject(
	ctx context.Context,
	record application.ProjectCreateRecord,
) (application.MutationResult[application.CreatedProject], error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return application.MutationResult[application.CreatedProject]{}, err
	}
	defer func() { _ = tx.Rollback() }()

	if result, found, err := existingResult[application.CreatedProject](
		ctx,
		tx,
		"create_project",
		record.OperationID,
		record.RequestHash,
	); err != nil ||
		found {
		return result, err
	}

	connectionID := record.ConnectionID
	if connectionID == "" {
		var defaultConnection sql.NullString
		if err := tx.QueryRowContext(ctx, `SELECT default_connection_id FROM app_settings WHERE singleton=1`).
			Scan(&defaultConnection); err != nil {
			return application.MutationResult[application.CreatedProject]{}, err
		}

		connectionID = defaultConnection.String
	}

	if connectionID == "" {
		return application.MutationResult[application.CreatedProject]{}, &shared.Error{
			Code:    "connection_required",
			Message: "Agent接続が必要です",
		}
	}

	now := projectTimestamp(record.Receipt.CommittedAt)
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO projects(project_id,lineage_id,connection_id,name,description,workspace_path,status,current_stage,revision,created_at,updated_at)
VALUES(?,?,?,?,?,?,'preparing','preparation',1,?,?)`,
		record.ProjectID,
		record.ProjectID,
		connectionID,
		record.Name,
		record.Description,
		record.WorkspacePath,
		now,
		now,
	); err != nil {
		return application.MutationResult[application.CreatedProject]{}, err
	}

	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO preparations(project_id,revision) VALUES(?,1)`,
		record.ProjectID,
	); err != nil {
		return application.MutationResult[application.CreatedProject]{}, err
	}

	result := createdProject(record.ProjectID, record.Receipt.CommittedAt, record.Receipt)
	if err := finishProjectMutation(
		ctx,
		tx,
		"create_project",
		record.OperationID,
		record.RequestHash,
		&result,
		record.Event,
	); err != nil {
		return result, err
	}

	return commitProjectMutation(ctx, tx, result, "CreateProject", record.ProjectID, "")
}

func (r *ProjectRepository) DuplicateProject(
	ctx context.Context,
	record application.ProjectCopyRecord,
) (application.MutationResult[application.CreatedProject], error) {
	return r.copyProject(ctx, record, false)
}

func (r *ProjectRepository) CreateRevision(
	ctx context.Context,
	record application.ProjectCopyRecord,
) (application.MutationResult[application.CreatedProject], error) {
	return r.copyProject(ctx, record, true)
}

func (r *ProjectRepository) copyProject(
	ctx context.Context,
	record application.ProjectCopyRecord,
	revision bool,
) (application.MutationResult[application.CreatedProject], error) {
	scope := "duplicate_project"
	if revision {
		scope = "create_revision"
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return application.MutationResult[application.CreatedProject]{}, err
	}

	defer func() { _ = tx.Rollback() }()

	if result, found, err := existingResult[application.CreatedProject](
		ctx,
		tx,
		scope,
		record.OperationID,
		record.RequestHash,
	); err != nil ||
		found {
		return result, err
	}

	var (
		lineage, connectionID, description, status string
		sourceRevision                             int64
	)

	err = tx.QueryRowContext(ctx, `SELECT lineage_id,connection_id,description,status,revision FROM projects WHERE project_id=? AND archived_at IS NULL`, record.SourceProjectID).
		Scan(&lineage, &connectionID, &description, &status, &sourceRevision)
	if errors.Is(err, sql.ErrNoRows) {
		return application.MutationResult[application.CreatedProject]{}, &shared.Error{
			Code:    "project_not_found",
			Message: "元のプロジェクトが見つかりません",
		}
	}

	if err != nil {
		return application.MutationResult[application.CreatedProject]{}, err
	}

	if revision {
		if err := project.CanRevise(project.Status(status)); err != nil {
			return application.MutationResult[application.CreatedProject]{}, err
		}

		var procedureRevision int64

		err := tx.QueryRowContext(ctx, `SELECT revision FROM procedures WHERE procedure_id=? AND project_id=? AND status='completed'`, record.SourceProcedureID, record.SourceProjectID).
			Scan(&procedureRevision)
		if errors.Is(err, sql.ErrNoRows) {
			return application.MutationResult[application.CreatedProject]{}, &shared.Error{
				Code:    "procedure_not_found",
				Message: "完成版が見つかりません",
			}
		}

		if err != nil {
			return application.MutationResult[application.CreatedProject]{}, err
		}

		if procedureRevision != record.SourceProcedureRevision {
			return application.MutationResult[application.CreatedProject]{}, revisionConflict(procedureRevision)
		}
	} else if sourceRevision != record.SourceRevision {
		return application.MutationResult[application.CreatedProject]{}, revisionConflict(sourceRevision)
	}

	if !revision {
		lineage = record.ProjectID
	}

	now := projectTimestamp(record.Receipt.CommittedAt)
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO projects(project_id,lineage_id,source_project_id,source_procedure_id,connection_id,name,description,workspace_path,status,current_stage,revision,created_at,updated_at)
VALUES(?,?,?,?,?,?,?,?,'preparing','preparation',1,?,?)`,
		record.ProjectID,
		lineage,
		record.SourceProjectID,
		nullableString(record.SourceProcedureID),
		connectionID,
		record.Name,
		description,
		record.WorkspacePath,
		now,
		now,
	); err != nil {
		return application.MutationResult[application.CreatedProject]{}, err
	}

	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO preparations(project_id,purpose,completion_criteria_json,intended_users,revision)
SELECT ?,purpose,completion_criteria_json,intended_users,1 FROM preparations WHERE project_id=?`,
		record.ProjectID,
		record.SourceProjectID,
	); err != nil {
		return application.MutationResult[application.CreatedProject]{}, err
	}

	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO check_plans(project_id,revision) SELECT ?,1 FROM check_plans WHERE project_id=?`,
		record.ProjectID,
		record.SourceProjectID,
	); err != nil {
		return application.MutationResult[application.CreatedProject]{}, err
	}

	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO check_items(check_id,project_id,sequence,title,instruction,expected_result,suggested_command,ai_required,human_required,human_evidence_requirement)
SELECT ? || ':' || sequence,?,sequence,title,instruction,expected_result,suggested_command,ai_required,human_required,human_evidence_requirement
FROM check_items WHERE project_id=?`,
		record.ProjectID,
		record.ProjectID,
		record.SourceProjectID,
	); err != nil {
		return application.MutationResult[application.CreatedProject]{}, err
	}

	result := createdProject(record.ProjectID, record.Receipt.CommittedAt, record.Receipt)
	if err := finishProjectMutation(
		ctx,
		tx,
		scope,
		record.OperationID,
		record.RequestHash,
		&result,
		record.Event,
	); err != nil {
		return result, err
	}

	method := "DuplicateProject"
	if revision {
		method = "CreateRevision"
	}

	return commitProjectMutation(ctx, tx, result, method, record.ProjectID, "")
}

func (r *ProjectRepository) ArchiveProject(
	ctx context.Context,
	record application.ProjectArchiveRecord,
) (application.MutationResult[application.ArchivedProject], error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return application.MutationResult[application.ArchivedProject]{}, err
	}
	defer func() { _ = tx.Rollback() }()

	if result, found, err := existingResult[application.ArchivedProject](
		ctx,
		tx,
		"archive_project",
		record.OperationID,
		record.RequestHash,
	); err != nil ||
		found {
		return result, err
	}

	var (
		status          string
		currentRevision int64
	)

	err = tx.QueryRowContext(ctx, `SELECT status,revision FROM projects WHERE project_id=?`, record.ProjectID).
		Scan(&status, &currentRevision)
	if errors.Is(err, sql.ErrNoRows) {
		return application.MutationResult[application.ArchivedProject]{}, &shared.Error{
			Code:    "project_not_found",
			Message: "プロジェクトが見つかりません",
		}
	}

	if err != nil {
		return application.MutationResult[application.ArchivedProject]{}, err
	}

	if currentRevision != record.ExpectedRevision {
		return application.MutationResult[application.ArchivedProject]{}, revisionConflict(currentRevision)
	}

	var active int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM background_jobs WHERE project_id=? AND state IN ('pending','running')`, record.ProjectID).
		Scan(&active); err != nil {
		return application.MutationResult[application.ArchivedProject]{}, err
	}

	if err := project.CanArchive(project.Status(status), active > 0); err != nil {
		return application.MutationResult[application.ArchivedProject]{}, err
	}

	now := projectTimestamp(record.Receipt.CommittedAt)

	updated, err := tx.ExecContext(
		ctx,
		`UPDATE projects SET status='archived',revision=revision+1,archived_at=?,updated_at=? WHERE project_id=? AND revision=?`,
		now,
		now,
		record.ProjectID,
		record.ExpectedRevision,
	)
	if err != nil {
		return application.MutationResult[application.ArchivedProject]{}, err
	}

	count, err := updated.RowsAffected()
	if err != nil {
		return application.MutationResult[application.ArchivedProject]{}, err
	}

	if count == 0 {
		return application.MutationResult[application.ArchivedProject]{}, revisionConflict(currentRevision)
	}

	result := application.MutationResult[application.ArchivedProject]{
		Data: application.ArchivedProject{
			ProjectID:  record.ProjectID,
			Revision:   currentRevision + 1,
			ArchivedAt: record.Receipt.CommittedAt,
		},
		Receipt: record.Receipt,
	}
	if err := finishProjectMutation(
		ctx,
		tx,
		"archive_project",
		record.OperationID,
		record.RequestHash,
		&result,
		record.Event,
	); err != nil {
		return result, err
	}

	return commitProjectMutation(ctx, tx, result, "ArchiveProject", record.ProjectID, "")
}

func (r *ProjectRepository) DeleteProject(
	ctx context.Context,
	record application.ProjectDeleteRecord,
) (application.MutationResult[application.DeletedProject], error) {
	if r.evidence != nil {
		r.evidence.mu.Lock()
		defer r.evidence.mu.Unlock()
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return application.MutationResult[application.DeletedProject]{}, err
	}

	defer func() { _ = tx.Rollback() }()

	if result, found, err := existingResult[application.DeletedProject](
		ctx,
		tx,
		"delete_project",
		record.OperationID,
		record.RequestHash,
	); err != nil ||
		found {
		return result, err
	}

	var revision int64

	err = tx.QueryRowContext(ctx, `SELECT revision FROM projects WHERE project_id=?`, record.ProjectID).Scan(&revision)
	if errors.Is(err, sql.ErrNoRows) {
		return application.MutationResult[application.DeletedProject]{}, &shared.Error{
			Code:    "project_not_found",
			Message: "プロジェクトが見つかりません",
		}
	}

	if err != nil {
		return application.MutationResult[application.DeletedProject]{}, err
	}

	if revision != record.ExpectedRevision {
		return application.MutationResult[application.DeletedProject]{}, revisionConflict(revision)
	}

	var active int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM background_jobs WHERE project_id=? AND state IN ('pending','running')`, record.ProjectID).
		Scan(&active); err != nil {
		return application.MutationResult[application.DeletedProject]{}, err
	}

	if active > 0 {
		return application.MutationResult[application.DeletedProject]{}, &shared.Error{
			Code:    "active_job",
			Message: "処理中のプロジェクトは削除できません",
		}
	}

	rows, err := tx.QueryContext(
		ctx,
		`SELECT DISTINCT er.blob_hash FROM evidence_records er JOIN executions e ON e.execution_id=er.execution_id WHERE e.project_id=?`,
		record.ProjectID,
	)
	if err != nil {
		return application.MutationResult[application.DeletedProject]{}, err
	}

	var hashes []string

	for rows.Next() {
		var hash string
		if err := rows.Scan(&hash); err != nil {
			_ = rows.Close()
			return application.MutationResult[application.DeletedProject]{}, err
		}

		hashes = append(hashes, hash)
	}

	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return application.MutationResult[application.DeletedProject]{}, err
	}

	statements := []string{
		`DELETE FROM procedure_evidence WHERE procedure_id IN (SELECT procedure_id FROM procedures WHERE project_id=?)`,
		`DELETE FROM procedure_sources WHERE procedure_id IN (SELECT procedure_id FROM procedures WHERE project_id=?)`,
		`DELETE FROM check_evidence WHERE check_id IN (SELECT check_id FROM check_items WHERE project_id=?)`,
		`DELETE FROM evidence_records WHERE execution_id IN (SELECT execution_id FROM executions WHERE project_id=?)`,
		`DELETE FROM executions WHERE project_id=?`,
		`DELETE FROM exports WHERE project_id=?`,
		`DELETE FROM background_jobs WHERE project_id=?`,
		`DELETE FROM acp_sessions WHERE project_id=?`,
		`DELETE FROM procedures WHERE project_id=?`,
		`DELETE FROM check_items WHERE project_id=?`,
		`DELETE FROM check_plans WHERE project_id=?`,
		`DELETE FROM preparations WHERE project_id=?`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement, record.ProjectID); err != nil {
			return application.MutationResult[application.DeletedProject]{}, err
		}
	}

	for _, hash := range hashes {
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE evidence_blobs SET ref_count=(SELECT COUNT(*) FROM evidence_records WHERE blob_hash=?) WHERE hash=?`,
			hash,
			hash,
		); err != nil {
			return application.MutationResult[application.DeletedProject]{}, err
		}

		if _, err := tx.ExecContext(
			ctx,
			`INSERT OR IGNORE INTO evidence_blob_deletions(hash,relative_path) SELECT hash,relative_path FROM evidence_blobs WHERE hash=? AND ref_count=0`,
			hash,
		); err != nil {
			return application.MutationResult[application.DeletedProject]{}, err
		}

		if _, err := tx.ExecContext(ctx, `DELETE FROM evidence_blobs WHERE hash=? AND ref_count=0`, hash); err != nil {
			return application.MutationResult[application.DeletedProject]{}, err
		}
	}

	if _, err := tx.ExecContext(
		ctx,
		`UPDATE projects SET source_project_id=NULL,source_procedure_id=NULL WHERE source_project_id=?`,
		record.ProjectID,
	); err != nil {
		return application.MutationResult[application.DeletedProject]{}, err
	}

	if _, err := tx.ExecContext(
		ctx,
		`DELETE FROM projects WHERE project_id=? AND revision=?`,
		record.ProjectID,
		revision,
	); err != nil {
		return application.MutationResult[application.DeletedProject]{}, err
	}

	result := application.MutationResult[application.DeletedProject]{
		Data:    application.DeletedProject{ProjectID: record.ProjectID, DeletedAt: record.Receipt.CommittedAt},
		Receipt: record.Receipt,
	}
	if err := finishProjectMutation(
		ctx,
		tx,
		"delete_project",
		record.OperationID,
		record.RequestHash,
		&result,
		record.Event,
	); err != nil {
		return result, err
	}

	result, err = commitProjectMutation(ctx, tx, result, "DeleteProject", record.ProjectID, "")
	if err != nil {
		return result, err
	}

	if r.evidence != nil {
		if err := r.evidence.cleanupDeleted(ctx); err != nil {
			return result, fmt.Errorf("プロジェクトは削除済みですが証跡fileの後処理に失敗しました: %w", err)
		}
	}

	return result, nil
}

func commitProjectMutation[T any](
	ctx context.Context, tx *sql.Tx, result application.MutationResult[T], method, aggregateID, jobID string,
) (application.MutationResult[T], error) {
	if err := tx.Commit(); err != nil {
		return result, err
	}

	trace.Record(ctx, trace.Entry{
		Phase: "accept_commit", Method: method,
		OperationID: result.Receipt.OperationID, JobID: jobID, AggregateID: aggregateID,
		ChangeSequence: result.Receipt.ChangeSequence, Status: "succeeded",
	})

	return result, nil
}

func createdProject(
	id string,
	now time.Time,
	receipt application.MutationReceipt,
) application.MutationResult[application.CreatedProject] {
	return application.MutationResult[application.CreatedProject]{
		Data: application.CreatedProject{
			ProjectID:    id,
			Revision:     1,
			Status:       project.Preparing,
			CurrentStage: "preparation",
			NextRoute:    project.ResumeRoute(id, "preparation"),
			CreatedAt:    now,
		},
		Receipt: receipt,
	}
}

func finishProjectMutation[T any](
	ctx context.Context,
	tx *sql.Tx,
	scope, operationID, requestHash string,
	result *application.MutationResult[T],
	event application.OutboxEvent,
) error {
	var sequence int64
	if err := tx.QueryRowContext(ctx, `UPDATE app_settings SET change_sequence=change_sequence+1 WHERE singleton=1 RETURNING change_sequence`).
		Scan(&sequence); err != nil {
		return err
	}

	result.Receipt.ChangeSequence = sequence

	return saveMutation(ctx, tx, scope, operationID, requestHash, *result, event, sequence)
}

func revisionConflict(current int64) error {
	return &shared.Error{Code: "revision_conflict", Message: "更新された内容があります", CurrentRevision: &current}
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}

	return value
}

func projectTimestamp(value time.Time) int64 {
	return value.UTC().UnixMicro()
}
