package application

import (
	"context"
	"time"

	"github.com/yukihito-jokyu/TEJUN/internal/domain/execution"
)

type ExecutionRepository interface {
	GetExecution(context.Context, string, int) (execution.Snapshot, error)
	GetExecutionView(context.Context, ExecutionViewQuery) (ExecutionView, error)
	ProjectIDForExecution(context.Context, string) (string, error)
	SetHumanCheck(context.Context, HumanCheckRecord) (MutationResult[CheckUpdate], error)
	GenerateProcedureDraft(context.Context, ProcedureDraftRecord) (MutationResult[ProcedureGenerationAccepted], error)
}

type Execution struct {
	repository ExecutionRepository
	now        func() time.Time
	newID      func() string
}

func NewExecution(repository ExecutionRepository, now func() time.Time, newID func() string) *Execution {
	return &Execution{repository: repository, now: now, newID: newID}
}

type HumanCheckRecord struct {
	ExecutionID, CheckID, OperationID string
	Checked                           bool
	ExpectedRevision                  int64
	At                                time.Time
	Event                             OutboxEvent
}

type CheckUpdate struct {
	ExecutionID       string
	Check             ExecutionCheckView
	ExecutionRevision int64
	CanGenerate       bool
	BlockingReasons   []string
}

type ProcedureDraftRecord struct {
	ExecutionID, OperationID, ProcedureID string
	ExpectedRevision                      int64
	At                                    time.Time
	Event                                 OutboxEvent
}

type ProcedureGenerationAccepted struct {
	ProcedureID string
	NextRoute   string
	AcceptedAt  time.Time
}

func (e *Execution) Get(ctx context.Context, projectID string, conversationLimit int) (execution.Snapshot, error) {
	return e.repository.GetExecution(ctx, projectID, conversationLimit)
}

func (e *Execution) SetHumanCheck(
	ctx context.Context,
	input HumanCheckRecord,
) (MutationResult[CheckUpdate], error) {
	input.At = e.now()
	input.Event = OutboxEvent{
		ID: e.newID(), Name: "execution.updated", AggregateType: "execution",
		AggregateID: input.ExecutionID, EmittedAt: input.At, Correlation: input.OperationID,
	}

	return e.repository.SetHumanCheck(ctx, input)
}

func (e *Execution) GenerateProcedureDraft(
	ctx context.Context,
	input ProcedureDraftRecord,
) (MutationResult[ProcedureGenerationAccepted], error) {
	input.At = e.now()
	input.ProcedureID = e.newID()
	input.Event = OutboxEvent{
		ID: e.newID(), Name: "procedure.updated", AggregateType: "procedure",
		AggregateID: input.ProcedureID, EmittedAt: input.At, Correlation: input.OperationID,
	}

	return e.repository.GenerateProcedureDraft(ctx, input)
}

type ExecutionViewQuery struct {
	ProjectID          string `json:"projectId"`
	ConversationCursor *int64 `json:"conversationCursor"`
	ConversationLimit  int    `json:"conversationLimit"`
}

type ExecutionSummary struct {
	ExecutionID string     `json:"executionId"`
	Status      string     `json:"status"`
	Revision    int64      `json:"revision"`
	StartedAt   time.Time  `json:"startedAt"`
	CompletedAt *time.Time `json:"completedAt"`
}

type RunSummary struct {
	RunID            string     `json:"runId"`
	Status           string     `json:"status"`
	TargetedCheckIDs []string   `json:"targetedCheckIds"`
	StartedAt        *time.Time `json:"startedAt"`
	FinishedAt       *time.Time `json:"finishedAt"`
}

type CheckSideView struct {
	Required       bool       `json:"required"`
	Status         string     `json:"status"`
	Checked        bool       `json:"checked"`
	CheckedAt      *time.Time `json:"checkedAt"`
	FailureSummary string     `json:"failureSummary"`
}

type EvidenceSummary struct {
	EvidenceID  string    `json:"evidenceId"`
	Actor       string    `json:"actor"`
	Kind        string    `json:"kind"`
	DisplayName string    `json:"displayName"`
	MimeType    string    `json:"mimeType"`
	Size        int64     `json:"size"`
	PreviewURL  string    `json:"previewUrl"`
	CreatedAt   time.Time `json:"createdAt"`
}

type CheckEvidence struct {
	AI    []EvidenceSummary `json:"ai"`
	Human []EvidenceSummary `json:"human"`
}

type ExecutionCheckView struct {
	CheckID                  string        `json:"checkId"`
	Sequence                 int           `json:"sequence"`
	Title                    string        `json:"title"`
	Instruction              string        `json:"instruction"`
	ExpectedResult           string        `json:"expectedResult"`
	AI                       CheckSideView `json:"ai"`
	Human                    CheckSideView `json:"human"`
	HumanEvidenceRequirement string        `json:"humanEvidenceRequirement"`
	OverallStatus            string        `json:"overallStatus"`
	Evidence                 CheckEvidence `json:"evidence"`
}

type ToolCallPresentation struct {
	ToolCallID string             `json:"toolCallId"`
	Title      string             `json:"title"`
	Kind       string             `json:"kind"`
	Status     string             `json:"status"`
	Command    string             `json:"command"`
	Locations  []ToolCallLocation `json:"locations"`
	Details    []ToolCallDetail   `json:"details"`
}

type ToolCallLocation struct {
	Path string `json:"path"`
	Line *int   `json:"line"`
}

type ToolCallDetail struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type PermissionRequestView struct {
	PermissionRequestID string                       `json:"permissionRequestId"`
	SessionID           string                       `json:"sessionId"`
	RunID               string                       `json:"runId"`
	ToolCall            ToolCallPresentation         `json:"toolCall"`
	Options             []execution.PermissionOption `json:"options"`
	Status              string                       `json:"status"`
	RequestedAt         time.Time                    `json:"requestedAt"`
	ExpiresAt           *time.Time                   `json:"expiresAt"`
}

type ExecutionReadiness struct {
	CanGenerateProcedure bool             `json:"canGenerateProcedure"`
	BlockingReasons      []ReadinessIssue `json:"blockingReasons"`
}

type ExecutionView struct {
	Project            ProjectHeader           `json:"project"`
	Execution          ExecutionSummary        `json:"execution"`
	Session            SessionSummary          `json:"session"`
	ActiveRun          *RunSummary             `json:"activeRun"`
	Checks             []ExecutionCheckView    `json:"checks"`
	PendingPermissions []PermissionRequestView `json:"pendingPermissions"`
	Conversation       ConversationPage        `json:"conversation"`
	Readiness          ExecutionReadiness      `json:"readiness"`
	ChangeSequence     int64                   `json:"changeSequence"`
}

func (e *Execution) GetView(ctx context.Context, query ExecutionViewQuery) (ExecutionView, error) {
	return e.repository.GetExecutionView(ctx, query)
}

func ExecutionCheckFromSnapshot(check execution.Check) ExecutionCheckView {
	item := ExecutionCheckView{
		CheckID:                  check.ID,
		Sequence:                 check.Sequence,
		Title:                    check.Title,
		Instruction:              check.Instruction,
		ExpectedResult:           check.ExpectedResult,
		HumanEvidenceRequirement: check.HumanEvidenceRequirement,
		Evidence:                 CheckEvidence{AI: []EvidenceSummary{}, Human: []EvidenceSummary{}},
	}

	aiStatus := check.AIStatus
	if !check.AIRequired && aiStatus == "pending" {
		aiStatus = "not_required"
	} else if aiStatus == "queued" {
		aiStatus = "running"
	}

	item.AI = CheckSideView{
		Required:       check.AIRequired,
		Status:         aiStatus,
		Checked:        check.AIStatus == "completed",
		CheckedAt:      check.AICheckedAt,
		FailureSummary: check.AIFailureSummary,
	}

	humanStatus := check.HumanStatus
	if !check.HumanRequired {
		humanStatus = "not_required"
	}

	item.Human = CheckSideView{
		Required:  check.HumanRequired,
		Status:    humanStatus,
		Checked:   check.HumanStatus == "completed",
		CheckedAt: check.HumanCheckedAt,
	}
	switch {
	case check.AIStatus == "failed":
		item.OverallStatus = "failed"
	case check.AIStatus == "completed" && check.HumanStatus == "completed":
		item.OverallStatus = "completed"
	case check.AIStatus == "running" || check.AIStatus == "queued":
		item.OverallStatus = "running"
	case check.AIStatus == "completed":
		item.OverallStatus = "awaiting_human"
	default:
		item.OverallStatus = "pending"
	}

	for _, ev := range check.Evidence {
		if ev.Status != "available" {
			continue
		}

		summary := EvidenceSummaryFromRecord(ev)
		if ev.Actor == "ai" {
			item.Evidence.AI = append(item.Evidence.AI, summary)
		} else {
			item.Evidence.Human = append(item.Evidence.Human, summary)
		}
	}

	return item
}

func EvidenceSummaryFromRecord(ev execution.Evidence) EvidenceSummary {
	return EvidenceSummary{
		EvidenceID:  ev.ID,
		Actor:       ev.Actor,
		Kind:        ev.Kind,
		DisplayName: ev.DisplayName,
		MimeType:    ev.MimeType,
		Size:        ev.Size,
		CreatedAt:   ev.CreatedAt,
	}
}

func (e *Execution) ProjectIDForExecution(ctx context.Context, executionID string) (string, error) {
	return e.repository.ProjectIDForExecution(ctx, executionID)
}
