package application

import (
	"context"
	"time"

	"github.com/yukihito-jokyu/TEJUN/internal/domain/procedure"
)

type ProcedureViewQuery struct {
	ProjectID          string `json:"projectId"`
	ConversationCursor *int64 `json:"conversationCursor"`
	ConversationLimit  int    `json:"conversationLimit"`
}

type ProcedureSummary struct {
	ProcedureID    string             `json:"procedureId"`
	RevisionNumber int64              `json:"revisionNumber"`
	Revision       int64              `json:"revision"`
	Status         procedure.Status   `json:"status"`
	Document       procedure.Document `json:"document"`
	CreatedAt      time.Time          `json:"createdAt"`
	UpdatedAt      time.Time          `json:"updatedAt"`
	CompletedAt    *time.Time         `json:"completedAt"`
}

type ProcedureSourceSummary struct {
	ExecutionID       string    `json:"executionId"`
	ExecutionRevision int64     `json:"executionRevision"`
	CheckCount        int       `json:"checkCount"`
	EvidenceCount     int       `json:"evidenceCount"`
	CapturedAt        time.Time `json:"capturedAt"`
}

type ProcedureEvidenceSummary struct {
	EvidenceID  string    `json:"evidenceId"`
	Actor       string    `json:"actor"`
	Kind        string    `json:"kind"`
	DisplayName string    `json:"displayName"`
	CreatedAt   time.Time `json:"createdAt"`
}

type ProcedureActiveRevision struct {
	SessionID string `json:"sessionId"`
	TurnID    string `json:"turnId"`
	JobID     string `json:"jobId"`
	Status    string `json:"status"`
}

type ProcedureView struct {
	Project   ProjectHeader          `json:"project"`
	Procedure ProcedureSummary       `json:"procedure"`
	Source    ProcedureSourceSummary `json:"source"`
	Evidence  struct {
		AI    []ProcedureEvidenceSummary `json:"ai"`
		Human []ProcedureEvidenceSummary `json:"human"`
	} `json:"evidence"`
	Integrity      procedure.Integrity      `json:"integrity"`
	Conversation   ConversationPage         `json:"conversation"`
	ActiveRevision *ProcedureActiveRevision `json:"activeRevision"`
	Elicitations   []ElicitationRequestView `json:"elicitations"`
	ChangeSequence int64                    `json:"changeSequence"`
}

type ProcedureSaveInput struct {
	ProcedureID      string             `json:"procedureId"`
	ExpectedRevision int64              `json:"expectedRevision"`
	OperationID      string             `json:"operationId"`
	Document         procedure.Document `json:"document"`
}

type ProcedureSaved struct {
	ProcedureID string              `json:"procedureId"`
	Revision    int64               `json:"revision"`
	Document    procedure.Document  `json:"document"`
	Integrity   procedure.Integrity `json:"integrity"`
	UpdatedAt   time.Time           `json:"updatedAt"`
}

type ProcedureCompleteInput struct {
	ProcedureID      string `json:"procedureId"`
	ExpectedRevision int64  `json:"expectedRevision"`
	OperationID      string `json:"operationId"`
}

type ProcedureCompleted struct {
	ProcedureID     string           `json:"procedureId"`
	Revision        int64            `json:"revision"`
	Status          procedure.Status `json:"status"`
	CompletedAt     time.Time        `json:"completedAt"`
	ProjectID       string           `json:"projectId"`
	ProjectRevision int64            `json:"projectRevision"`
}

type ProcedureSaveRecord struct {
	ProcedureSaveInput
	At    time.Time
	Event OutboxEvent
}
type ProcedureCompleteRecord struct {
	ProcedureCompleteInput
	At           time.Time
	Event        OutboxEvent
	ProjectEvent OutboxEvent
}

type ProcedureRepository interface {
	GetProcedure(context.Context, ProcedureViewQuery) (ProcedureView, error)
	SaveProcedureDraft(context.Context, ProcedureSaveRecord) (MutationResult[ProcedureSaved], error)
	CompleteProcedure(context.Context, ProcedureCompleteRecord) (MutationResult[ProcedureCompleted], error)
}

type Procedure struct {
	repository ProcedureRepository
	now        func() time.Time
	newID      func() string
}

func NewProcedure(repository ProcedureRepository, now func() time.Time, newID func() string) *Procedure {
	return &Procedure{repository: repository, now: now, newID: newID}
}

func (p *Procedure) Get(ctx context.Context, query ProcedureViewQuery) (ProcedureView, error) {
	return p.repository.GetProcedure(ctx, query)
}

func (p *Procedure) SaveDraft(ctx context.Context, input ProcedureSaveInput) (MutationResult[ProcedureSaved], error) {
	at := p.now()

	return p.repository.SaveProcedureDraft(
		ctx,
		ProcedureSaveRecord{
			ProcedureSaveInput: input,
			At:                 at,
			Event: OutboxEvent{
				ID:            p.newID(),
				Name:          "procedure.updated",
				AggregateType: "procedure",
				AggregateID:   input.ProcedureID,
				EmittedAt:     at,
				Correlation:   input.OperationID,
			},
		},
	)
}

func (p *Procedure) Complete(
	ctx context.Context,
	input ProcedureCompleteInput,
) (MutationResult[ProcedureCompleted], error) {
	at := p.now()

	return p.repository.CompleteProcedure(ctx, ProcedureCompleteRecord{
		ProcedureCompleteInput: input,
		At:                     at,
		Event: OutboxEvent{
			ID:            p.newID(),
			Name:          "procedure.updated",
			AggregateType: "procedure",
			AggregateID:   input.ProcedureID,
			EmittedAt:     at,
			Correlation:   input.OperationID,
		},
		ProjectEvent: OutboxEvent{
			ID:            p.newID(),
			Name:          "project.changed",
			AggregateType: "project",
			EmittedAt:     at,
			Correlation:   input.OperationID,
		},
	})
}
