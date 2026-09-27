package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/yukihito-jokyu/TEJUN/internal/application"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/procedure"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/shared"
	"github.com/yukihito-jokyu/TEJUN/internal/trace"
)

type ProcedureRepository struct {
	db       *sql.DB
	evidence *EvidenceFiles
}

func NewProcedureRepository(db *sql.DB, evidence *EvidenceFiles) *ProcedureRepository {
	return &ProcedureRepository{db: db, evidence: evidence}
}

func (r *ProcedureRepository) GetProcedure(
	ctx context.Context,
	in application.ProcedureViewQuery,
) (application.ProcedureView, error) {
	view := application.ProcedureView{
		Conversation: application.ConversationPage{Items: []application.ConversationItem{}},
	}
	view.Evidence.AI = []application.ProcedureEvidenceSummary{}
	view.Evidence.Human = []application.ProcedureEvidenceSummary{}
	view.Elicitations = []application.ElicitationRequestView{}

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

	var (
		created, updated, completed                        sql.NullInt64
		procedureCreated, procedureUpdated, sourceCaptured int64
		doc                                                string
	)

	err = tx.QueryRowContext(ctx, `SELECT p.project_id,p.name,p.status,p.current_stage,p.revision,p.created_at,p.updated_at,
 q.procedure_id,q.revision,q.status,q.document_json,q.created_at,q.updated_at,q.completed_at,
 s.execution_id,s.execution_revision,s.check_count,s.evidence_count,s.captured_at,a.change_sequence
 FROM projects p JOIN procedures q ON q.project_id=p.project_id
 JOIN procedure_sources s ON s.procedure_id=q.procedure_id CROSS JOIN app_settings a
 WHERE p.project_id=? AND a.singleton=1 ORDER BY q.created_at DESC,q.procedure_id DESC LIMIT 1`, in.ProjectID).
		Scan(&view.Project.ProjectID, &view.Project.Name, &view.Project.Status, &view.Project.CurrentStage, &view.Project.Revision, &created, &updated,
			&view.Procedure.ProcedureID, &view.Procedure.Revision, &view.Procedure.Status, &doc, &procedureCreated, &procedureUpdated, &completed,
			&view.Source.ExecutionID, &view.Source.ExecutionRevision, &view.Source.CheckCount, &view.Source.EvidenceCount, &sourceCaptured, &view.ChangeSequence)
	if err != nil {
		return view, notFound(err)
	}

	view.Project.CreatedAt = time.UnixMicro(created.Int64).UTC()
	view.Project.UpdatedAt = time.UnixMicro(updated.Int64).UTC()
	view.Procedure.CreatedAt = time.UnixMicro(procedureCreated).UTC()
	view.Procedure.UpdatedAt = time.UnixMicro(procedureUpdated).UTC()
	view.Source.CapturedAt = time.UnixMicro(sourceCaptured).UTC()
	view.Procedure.RevisionNumber = 1

	if completed.Valid {
		t := time.UnixMicro(completed.Int64).UTC()
		view.Procedure.CompletedAt = &t
	}

	if err = json.Unmarshal([]byte(doc), &view.Procedure.Document); err != nil {
		return view, err
	}

	if view.Procedure.Document.Prerequisites == nil {
		view.Procedure.Document.Prerequisites = []string{}
	}

	if view.Procedure.Document.Steps == nil {
		view.Procedure.Document.Steps = []procedure.Step{}
	}

	evidenceIDs := map[string]bool{}

	rows, err := tx.QueryContext(ctx, `SELECT e.evidence_id,e.actor,e.kind,e.display_name,e.created_at
 FROM procedure_source_evidence pe JOIN execution_evidence e ON e.evidence_id=pe.evidence_id
 WHERE pe.procedure_id=? ORDER BY e.created_at,e.evidence_id`, view.Procedure.ProcedureID)
	if err != nil {
		return view, err
	}

	for rows.Next() {
		var (
			item application.ProcedureEvidenceSummary
			at   string
		)
		if err = rows.Scan(&item.EvidenceID, &item.Actor, &item.Kind, &item.DisplayName, &at); err != nil {
			_ = rows.Close()
			return view, err
		}

		if item.CreatedAt, err = time.Parse(time.RFC3339Nano, at); err != nil {
			_ = rows.Close()
			return view, err
		}

		evidenceIDs[item.EvidenceID] = true
		if item.Actor == "ai" {
			view.Evidence.AI = append(view.Evidence.AI, item)
		} else {
			view.Evidence.Human = append(view.Evidence.Human, item)
		}
	}

	err = rows.Err()
	_ = rows.Close()

	if err != nil {
		return view, err
	}

	view.Integrity = procedure.Evaluate(view.Procedure.Document, evidenceIDs)

	cursor := int64(1<<63 - 1)
	if in.ConversationCursor != nil {
		cursor = *in.ConversationCursor
	}

	rows, err = tx.QueryContext(ctx, `SELECT rowid,turn_id,role,content_json,status,created_at FROM procedure_turns
 WHERE procedure_id=? AND rowid<? ORDER BY rowid DESC LIMIT ?`, view.Procedure.ProcedureID, cursor, in.ConversationLimit+1)
	if err != nil {
		return view, err
	}

	for rows.Next() {
		var (
			item    application.ConversationItem
			content string
			at      int64
		)
		if err = rows.Scan(&item.Sequence, &item.MessageID, &item.Role, &content, &item.Status, &at); err != nil {
			_ = rows.Close()
			return view, err
		}

		item.CreatedAt = time.UnixMicro(at).UTC()
		if err = json.Unmarshal([]byte(content), &item.Content); err != nil {
			_ = rows.Close()
			return view, err
		}

		if len(view.Conversation.Items) == in.ConversationLimit {
			view.Conversation.HasPrevious = true
			break
		}

		view.Conversation.Items = append(view.Conversation.Items, item)
	}

	err = rows.Err()
	_ = rows.Close()

	if err != nil {
		return view, err
	}

	if view.Conversation.HasPrevious {
		last := view.Conversation.Items[len(view.Conversation.Items)-1].Sequence
		view.Conversation.PreviousCursor = &last
	}

	var active application.ProcedureActiveRevision

	err = tx.QueryRowContext(ctx, `SELECT session_id,turn_id,job_id,state FROM procedure_revision_jobs
 WHERE procedure_id=? AND state IN ('queued','running','cancellation_requested')
 ORDER BY accepted_at DESC,job_id DESC LIMIT 1`, view.Procedure.ProcedureID).
		Scan(&active.SessionID, &active.TurnID, &active.JobID, &active.Status)
	if err != nil && err != sql.ErrNoRows {
		return view, err
	}

	if err == nil {
		view.ActiveRevision = &active

		rows, err = tx.QueryContext(
			ctx,
			`SELECT elicitation_request_id,mode,message,state,requested_at,expires_at,scope_json,requested_schema_json,COALESCE(url,'')
 FROM elicitation_requests WHERE session_id=? AND state IN ('pending','responding') ORDER BY requested_at,elicitation_request_id`,
			active.SessionID,
		)
		if err != nil {
			return view, err
		}

		for rows.Next() {
			var (
				item            application.ElicitationRequestView
				at, scope       string
				expires, schema sql.NullString
			)
			if err = rows.Scan(
				&item.ElicitationRequestID,
				&item.Mode,
				&item.Message,
				&item.Status,
				&at,
				&expires,
				&scope,
				&schema,
				&item.URL,
			); err != nil {
				_ = rows.Close()
				return view, err
			}

			item.SessionID = active.SessionID
			if item.RequestedAt, err = time.Parse(time.RFC3339Nano, at); err != nil {
				_ = rows.Close()
				return view, err
			}

			if expires.Valid {
				t, parseErr := time.Parse(time.RFC3339Nano, expires.String)
				if parseErr != nil {
					_ = rows.Close()
					return view, parseErr
				}

				item.ExpiresAt = &t
			}

			if err = json.Unmarshal([]byte(scope), &item.Scope); err != nil {
				_ = rows.Close()
				return view, err
			}

			if schema.Valid {
				if err = json.Unmarshal([]byte(schema.String), &item.RequestedSchema); err != nil {
					_ = rows.Close()
					return view, err
				}
			}

			view.Elicitations = append(view.Elicitations, item)
		}

		err = rows.Err()
		_ = rows.Close()

		if err != nil {
			return view, err
		}
	}

	if err = tx.Commit(); err != nil {
		return view, err
	}

	return view, nil
}

func (r *ProcedureRepository) SaveProcedureDraft(
	ctx context.Context,
	record application.ProcedureSaveRecord,
) (application.MutationResult[application.ProcedureSaved], error) {
	var zero application.MutationResult[application.ProcedureSaved]

	doc, err := json.Marshal(record.Document)
	if err != nil {
		return zero, err
	}

	if len(doc) > 5<<20 || !validProcedureDocumentText(record.Document) {
		return zero, &shared.Error{Code: "validation_failed", Message: "documentが不正です"}
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return zero, err
	}

	defer func() { _ = tx.Rollback() }()

	hash := requestHash(record.ProcedureID, fmt.Sprint(record.ExpectedRevision), string(doc))

	scope := "save_procedure_draft"
	if result, found, e := existingResult[application.ProcedureSaved](
		ctx,
		tx,
		scope,
		record.OperationID,
		hash,
	); e != nil ||
		found {
		return result, e
	}

	var (
		revision int64
		status   string
	)

	err = tx.QueryRowContext(ctx, `SELECT revision,status FROM procedures WHERE procedure_id=?`, record.ProcedureID).
		Scan(&revision, &status)
	if err != nil {
		return zero, notFound(err)
	}

	if status != "draft" {
		return zero, &shared.Error{Code: "invalid_state", Message: "draftのみ編集できます"}
	}

	if revision != record.ExpectedRevision {
		return zero, &shared.Error{Code: "revision_conflict", Message: "手順書が更新されています", CurrentRevision: &revision}
	}

	ids, err := procedureEvidenceIDs(ctx, tx, record.ProcedureID)
	if err != nil {
		return zero, err
	}

	integrity := procedure.Evaluate(record.Document, ids)

	_, err = tx.ExecContext(
		ctx,
		`UPDATE procedures SET document_json=?,revision=revision+1,updated_at=? WHERE procedure_id=? AND revision=? AND status='draft'`,
		string(doc),
		record.At.UnixMicro(),
		record.ProcedureID,
		revision,
	)
	if err != nil {
		return zero, err
	}

	var sequence int64
	if err = tx.QueryRowContext(ctx, `UPDATE app_settings SET change_sequence=change_sequence+1 WHERE singleton=1 RETURNING change_sequence`).
		Scan(&sequence); err != nil {
		return zero, err
	}

	result := application.MutationResult[application.ProcedureSaved]{
		Data: application.ProcedureSaved{
			ProcedureID: record.ProcedureID,
			Revision:    revision + 1,
			Document:    record.Document,
			Integrity:   integrity,
			UpdatedAt:   record.At,
		},
		Receipt: application.MutationReceipt{
			OperationID:    record.OperationID,
			CommittedAt:    record.At,
			ChangeSequence: sequence,
		},
	}
	if err = saveMutation(ctx, tx, scope, record.OperationID, hash, result, record.Event, sequence); err != nil {
		return zero, err
	}

	if err = tx.Commit(); err != nil {
		return zero, err
	}

	trace.Record(
		ctx,
		trace.Entry{
			Phase:          "transaction_commit",
			Method:         "SaveProcedureDraft",
			OperationID:    record.OperationID,
			EventID:        record.Event.ID,
			AggregateID:    record.ProcedureID,
			ChangeSequence: sequence,
			Status:         "succeeded",
		},
	)

	return result, nil
}

func (r *ProcedureRepository) CompleteProcedure(
	ctx context.Context,
	record application.ProcedureCompleteRecord,
) (application.MutationResult[application.ProcedureCompleted], error) {
	var zero application.MutationResult[application.ProcedureCompleted]

	hash := requestHash(record.ProcedureID, fmt.Sprint(record.ExpectedRevision))
	scope := "complete_procedure"

	readTx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return zero, err
	}

	replayed, found, replayErr := existingResult[application.ProcedureCompleted](
		ctx,
		readTx,
		scope,
		record.OperationID,
		hash,
	)
	_ = readTx.Rollback()

	if replayErr != nil || found {
		return replayed, replayErr
	}

	images, err := procedureImages(ctx, r.db, record.ProcedureID)
	if err != nil {
		return zero, err
	}

	if len(images) > 0 && r.evidence == nil {
		return zero, &shared.Error{Code: "evidence_invalid", Message: "証跡を検証できません"}
	}

	for _, item := range images {
		valid, verifyErr := r.evidence.Verify(ctx, item.StagedEvidence)
		if verifyErr != nil || !valid {
			return zero, &shared.Error{Code: "evidence_invalid", Message: "証跡を検証できません"}
		}
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return zero, err
	}

	defer func() { _ = tx.Rollback() }()

	if result, found, e := existingResult[application.ProcedureCompleted](
		ctx,
		tx,
		scope,
		record.OperationID,
		hash,
	); e != nil ||
		found {
		return result, e
	}

	var (
		revision, projectRevision int64
		status, projectID, doc    string
	)

	err = tx.QueryRowContext(ctx, `SELECT q.revision,q.status,q.document_json,p.project_id,p.revision FROM procedures q JOIN projects p ON p.project_id=q.project_id WHERE q.procedure_id=?`, record.ProcedureID).
		Scan(&revision, &status, &doc, &projectID, &projectRevision)
	if err != nil {
		return zero, notFound(err)
	}

	if status != "draft" {
		return zero, &shared.Error{Code: "invalid_state", Message: "draftのみ完成できます"}
	}

	if revision != record.ExpectedRevision {
		return zero, &shared.Error{Code: "revision_conflict", Message: "手順書が更新されています", CurrentRevision: &revision}
	}

	var document procedure.Document
	if err = json.Unmarshal([]byte(doc), &document); err != nil {
		return zero, err
	}

	ids, err := procedureEvidenceIDs(ctx, tx, record.ProcedureID)
	if err != nil {
		return zero, err
	}

	if procedure.Evaluate(document, ids).Status == "blocked" {
		return zero, &shared.Error{Code: "invalid_state", Message: "手順書の整合性を確認してください"}
	}

	currentImages, err := procedureImages(ctx, tx, record.ProcedureID)
	if err != nil {
		return zero, err
	}

	if !slices.Equal(images, currentImages) {
		return zero, &shared.Error{Code: "evidence_invalid", Message: "証跡を検証できません"}
	}

	var pending int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM procedure_turns WHERE procedure_id=? AND status IN ('queued','running','cancellation_requested')`, record.ProcedureID).
		Scan(&pending); err != nil {
		return zero, err
	}

	if pending > 0 {
		return zero, &shared.Error{Code: "invalid_state", Message: "生成処理が完了していません"}
	}

	if _, err = tx.ExecContext(
		ctx,
		`UPDATE procedures SET status='completed',revision=revision+1,updated_at=?,completed_at=? WHERE procedure_id=? AND revision=? AND status='draft'`,
		record.At.UnixMicro(),
		record.At.UnixMicro(),
		record.ProcedureID,
		revision,
	); err != nil {
		return zero, err
	}

	if _, err = tx.ExecContext(
		ctx,
		`UPDATE projects SET status='completed',current_stage='completed',revision=revision+1,updated_at=?,completed_at=? WHERE project_id=?`,
		record.At.UnixMicro(),
		record.At.UnixMicro(),
		projectID,
	); err != nil {
		return zero, err
	}

	var sequence int64
	if err = tx.QueryRowContext(ctx, `UPDATE app_settings SET change_sequence=change_sequence+1 WHERE singleton=1 RETURNING change_sequence`).
		Scan(&sequence); err != nil {
		return zero, err
	}

	result := application.MutationResult[application.ProcedureCompleted]{
		Data: application.ProcedureCompleted{
			ProcedureID:     record.ProcedureID,
			Revision:        revision + 1,
			Status:          procedure.Completed,
			CompletedAt:     record.At,
			ProjectID:       projectID,
			ProjectRevision: projectRevision + 1,
		},
		Receipt: application.MutationReceipt{
			OperationID:    record.OperationID,
			CommittedAt:    record.At,
			ChangeSequence: sequence,
		},
	}
	if err = saveMutation(ctx, tx, scope, record.OperationID, hash, result, record.Event, sequence); err != nil {
		return zero, err
	}

	record.ProjectEvent.AggregateID = projectID
	if err = insertOutbox(ctx, tx, record.ProjectEvent, sequence); err != nil {
		return zero, err
	}

	if err = tx.Commit(); err != nil {
		return zero, err
	}

	trace.Record(
		ctx,
		trace.Entry{
			Phase:          "transaction_commit",
			Method:         "CompleteProcedure",
			OperationID:    record.OperationID,
			EventID:        record.Event.ID,
			AggregateID:    record.ProcedureID,
			ChangeSequence: sequence,
			Status:         "succeeded",
		},
	)

	return result, nil
}

type procedureImage struct {
	EvidenceID string
	application.StagedEvidence
}

type procedureImageQuerier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func procedureImages(ctx context.Context, db procedureImageQuerier, procedureID string) ([]procedureImage, error) {
	rows, err := db.QueryContext(
		ctx,
		`SELECT ee.evidence_id,ee.blob_hash,b.hash,b.mime,b.size,b.status
FROM procedure_source_evidence pse JOIN execution_evidence ee ON ee.evidence_id=pse.evidence_id
JOIN procedure_sources ps ON ps.procedure_id=pse.procedure_id AND ps.execution_id=ee.execution_id
LEFT JOIN evidence_blobs b ON b.hash=ee.blob_hash
WHERE pse.procedure_id=? AND ee.kind='image' ORDER BY ee.evidence_id`,
		procedureID,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var images []procedureImage

	for rows.Next() {
		var (
			item                               procedureImage
			sourceHash, blobHash, mime, status sql.NullString
			size                               sql.NullInt64
		)
		if err := rows.Scan(
			&item.EvidenceID,
			&sourceHash,
			&blobHash,
			&mime,
			&size,
			&status,
		); err != nil {
			return nil, err
		}

		if !sourceHash.Valid || !blobHash.Valid ||
			sourceHash.String != blobHash.String || status.String != "available" || !mime.Valid || !size.Valid {
			return nil, &shared.Error{Code: "evidence_invalid", Message: "証跡を検証できません"}
		}

		item.StagedEvidence = application.StagedEvidence{
			Hash:        blobHash.String,
			MIME:        mime.String,
			Size:        size.Int64,
			StagingName: "staging/verify",
		}
		images = append(images, item)
	}

	return images, rows.Err()
}

func procedureEvidenceIDs(ctx context.Context, tx *sql.Tx, procedureID string) (map[string]bool, error) {
	rows, err := tx.QueryContext(
		ctx,
		`SELECT evidence_id FROM procedure_source_evidence WHERE procedure_id=?`,
		procedureID,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	ids := map[string]bool{}

	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}

		ids[id] = true
	}

	return ids, rows.Err()
}

func validProcedureDocumentText(document procedure.Document) bool {
	validText := func(value string) bool {
		return !strings.ContainsAny(value, "<>") && !strings.Contains(value, "```")
	}
	if !validText(document.Title) || !validText(document.Overview) {
		return false
	}

	for _, value := range document.Prerequisites {
		if !validText(value) {
			return false
		}
	}

	for _, step := range document.Steps {
		if !validText(step.Title) || !validText(step.Description) || !validText(step.Command) {
			return false
		}

		for _, note := range step.Notes {
			if !validText(note) {
				return false
			}
		}

		for _, ref := range step.EvidenceRefs {
			if !validText(ref.DisplayName) {
				return false
			}
		}
	}

	return true
}
