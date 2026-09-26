package application

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/yukihito-jokyu/TEJUN/internal/domain/agentconnection"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/shared"
)

type PreparationRepository interface {
	GetPreparation(context.Context, PreparationViewQuery) (PreparationView, error)
	SendPreparationMessage(context.Context, SendPreparationMessageRecord) (MutationResult[AcceptedTurn], error)
	SavePreparationBrief(context.Context, SavePreparationBriefRecord) (MutationResult[PreparationUpdate], error)
	SaveCheckPlan(context.Context, SaveCheckPlanRecord) (MutationResult[SavedCheckPlan], error)
	ChangeProjectWorkspace(
		context.Context,
		ChangeProjectWorkspaceRecord,
	) (MutationResult[WorkspaceChangeAccepted], error)
	SaveSessionPermissionPolicy(
		context.Context,
		SaveSessionPermissionPolicyRecord,
	) (MutationResult[SessionPermissionPolicy], error)
	StartExecution(context.Context, StartExecutionRecord) (MutationResult[ExecutionStarted], error)
}

type Preparation struct {
	repository PreparationRepository
	now        func() time.Time
	newID      func() string
}

func NewPreparation(repository PreparationRepository, now func() time.Time, newID func() string) *Preparation {
	return &Preparation{repository, now, newID}
}

type PreparationViewQuery struct {
	ProjectID          string `json:"projectId"`
	ChatID             string `json:"chatId,omitempty"`
	ConversationCursor *int64 `json:"conversationCursor"`
	ConversationLimit  int    `json:"conversationLimit"`
}
type ProjectHeader struct {
	ProjectID     string                  `json:"projectId"`
	Name          string                  `json:"name"`
	Description   string                  `json:"description"`
	WorkspacePath string                  `json:"workspacePath"`
	Status        string                  `json:"status"`
	CurrentStage  string                  `json:"currentStage"`
	Revision      int64                   `json:"revision"`
	CreatedAt     time.Time               `json:"createdAt"`
	UpdatedAt     time.Time               `json:"updatedAt"`
	Connection    *AgentConnectionSummary `json:"connection"`
}
type AgentConnectionSummary struct {
	ConnectionID           string   `json:"connectionId"`
	DisplayName            string   `json:"displayName"`
	Command                string   `json:"command"`
	Args                   []string `json:"args"`
	Transport              string   `json:"transport"`
	ResolvedExecutablePath string   `json:"resolvedExecutablePath"`
	LastVerifiedAt         string   `json:"lastVerifiedAt"`
	ProtocolVersion        string   `json:"protocolVersion"`
	AuthState              string   `json:"authState"`
	SchemaArtifactVersion  string   `json:"schemaArtifactVersion"`
}
type PreparationBrief struct {
	Purpose            string   `json:"purpose"`
	CompletionCriteria []string `json:"completionCriteria"`
	IntendedUsers      string   `json:"intendedUsers"`
	Revision           int64    `json:"revision"`
}
type CheckItem struct {
	CheckID                  string `json:"checkId"`
	Sequence                 int    `json:"sequence"`
	Title                    string `json:"title"`
	Instruction              string `json:"instruction"`
	ExpectedResult           string `json:"expectedResult"`
	SuggestedCommand         string `json:"suggestedCommand"`
	AIRequired               bool   `json:"aiRequired"`
	HumanRequired            bool   `json:"humanRequired"`
	HumanEvidenceRequirement string `json:"humanEvidenceRequirement"`
}
type CheckPlanView struct {
	PlanID   string      `json:"planId"`
	Revision int64       `json:"revision"`
	Items    []CheckItem `json:"items"`
}
type SessionPermissionPolicy struct {
	SessionID string `json:"sessionId"`
	Mode      string `json:"mode"`
	Revision  int64  `json:"revision"`
}
type SessionSummary struct {
	SessionID        string                  `json:"sessionId"`
	State            string                  `json:"state"`
	ProtocolVersion  string                  `json:"protocolVersion"`
	AgentName        string                  `json:"agentName"`
	AgentVersion     string                  `json:"agentVersion"`
	StartedAt        time.Time               `json:"startedAt"`
	DisconnectedAt   *time.Time              `json:"disconnectedAt"`
	PermissionPolicy SessionPermissionPolicy `json:"permissionPolicy"`
	Revision         int64                   `json:"revision"`
	ChangeSequence   int64                   `json:"changeSequence"`
	Capabilities     map[string]any          `json:"capabilities"`
	Modes            *SessionModes           `json:"modes"`
	ConfigOptions    []SessionConfigOption   `json:"configOptions"`
}
type SessionModes struct {
	CurrentModeID string        `json:"currentModeId"`
	Available     []SessionMode `json:"available"`
}
type SessionMode struct {
	ModeID      string `json:"modeId"`
	Name        string `json:"name"`
	Description string `json:"description"`
}
type SessionConfigOption struct {
	Type         string                      `json:"type"`
	ConfigID     string                      `json:"configId"`
	Name         string                      `json:"name"`
	Description  string                      `json:"description"`
	Category     string                      `json:"category"`
	CurrentValue any                         `json:"currentValue"`
	Options      *SessionConfigOptionChoices `json:"options,omitempty"`
}
type SessionConfigOptionChoices struct {
	Layout string                `json:"layout"`
	Items  []SessionConfigChoice `json:"items,omitempty"`
	Groups []SessionConfigGroup  `json:"groups,omitempty"`
}
type SessionConfigChoice struct {
	Value       string `json:"value"`
	Name        string `json:"name"`
	Description string `json:"description"`
}
type SessionConfigGroup struct {
	GroupID string                `json:"groupId"`
	Name    string                `json:"name"`
	Options []SessionConfigChoice `json:"options"`
}
type ContentPart struct {
	Type       string `json:"type"`
	Text       string `json:"text"`
	EvidenceID string `json:"evidenceId"`
	URL        string `json:"url"`
	Name       string `json:"name"`
	MimeType   string `json:"mimeType"`
}
type ConversationItem struct {
	MessageID string        `json:"messageId"`
	TurnID    string        `json:"turnId"`
	Role      string        `json:"role"`
	Status    string        `json:"status"`
	Content   []ContentPart `json:"content"`
	CreatedAt time.Time     `json:"createdAt"`
	Sequence  int64         `json:"sequence"`
}
type ConversationPage struct {
	Items          []ConversationItem `json:"items"`
	PreviousCursor *int64             `json:"previousCursor"`
	HasPrevious    bool               `json:"hasPrevious"`
}
type ReadinessIssue struct {
	Code                string `json:"code"`
	Message             string `json:"message"`
	CheckID             string `json:"checkId"`
	EvidenceRequirement string `json:"evidenceRequirement"`
}
type Readiness struct {
	CanStartExecution bool             `json:"canStartExecution"`
	BlockingReasons   []ReadinessIssue `json:"blockingReasons"`
}
type PreparationView struct {
	Project        ProjectHeader            `json:"project"`
	Preparation    PreparationBrief         `json:"preparation"`
	CheckPlan      CheckPlanView            `json:"checkPlan"`
	Session        *SessionSummary          `json:"session"`
	Conversation   ConversationPage         `json:"conversation"`
	Chats          []PreparationChat        `json:"chats"`
	Elicitations   []ElicitationRequestView `json:"elicitations"`
	Readiness      Readiness                `json:"readiness"`
	ChangeSequence int64                    `json:"changeSequence"`
}
type PreparationChat struct {
	SessionID string    `json:"sessionId"`
	Title     string    `json:"title"`
	StartedAt time.Time `json:"startedAt"`
}

type ElicitationRequestView struct {
	ElicitationRequestID string         `json:"elicitationRequestId"`
	SessionID            string         `json:"sessionId"`
	Scope                map[string]any `json:"scope"`
	RequestedSchema      map[string]any `json:"requestedSchema"`
	URL                  string         `json:"url"`
	Mode                 string         `json:"mode"`
	Message              string         `json:"message"`
	Status               string         `json:"status"`
	RequestedAt          time.Time      `json:"requestedAt"`
	ExpiresAt            *time.Time     `json:"expiresAt"`
}
type SendPreparationMessageInput struct {
	ProjectID   string        `json:"projectId"`
	SessionID   string        `json:"sessionId"`
	OperationID string        `json:"operationId"`
	Content     []ContentPart `json:"content"`
}
type AcceptedTurn struct {
	SessionID  string    `json:"sessionId"`
	TurnID     string    `json:"turnId"`
	JobID      string    `json:"jobId"`
	AcceptedAt time.Time `json:"acceptedAt"`
}
type SendPreparationMessageRecord struct {
	SendPreparationMessageInput
	SessionIDNew string    `json:"sessionIdNew"`
	TurnID       string    `json:"turnId"`
	JobID        string    `json:"jobId"`
	MessageID    string    `json:"messageId"`
	EventID      string    `json:"eventId"`
	AcceptedAt   time.Time `json:"acceptedAt"`
}

func (p *Preparation) SendPreparationMessage(
	ctx context.Context,
	in SendPreparationMessageInput,
) (MutationResult[AcceptedTurn], error) {
	return p.repository.SendPreparationMessage(
		ctx,
		SendPreparationMessageRecord{
			SendPreparationMessageInput: in,
			SessionIDNew:                p.newID(),
			TurnID:                      p.newID(),
			JobID:                       p.newID(),
			MessageID:                   p.newID(),
			EventID:                     p.newID(),
			AcceptedAt:                  p.now(),
		},
	)
}

type SavePreparationBriefInput struct {
	ProjectID                   string           `json:"projectId"`
	ExpectedPreparationRevision int64            `json:"expectedPreparationRevision"`
	OperationID                 string           `json:"operationId"`
	Brief                       PreparationBrief `json:"brief"`
}
type PreparationUpdate struct {
	ProjectID           string           `json:"projectId"`
	PreparationRevision int64            `json:"preparationRevision"`
	Brief               PreparationBrief `json:"brief"`
	AffectedCheckIDs    []string         `json:"affectedCheckIds"`
	Readiness           Readiness        `json:"readiness"`
	UpdatedAt           time.Time        `json:"updatedAt"`
}
type SavePreparationBriefRecord struct {
	SavePreparationBriefInput
	EventID   string    `json:"eventId"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (p *Preparation) SavePreparationBrief(
	ctx context.Context,
	in SavePreparationBriefInput,
) (MutationResult[PreparationUpdate], error) {
	return p.repository.SavePreparationBrief(ctx, SavePreparationBriefRecord{in, p.newID(), p.now()})
}

type CheckItemInput struct {
	CheckID                  string `json:"checkId"`
	ClientKey                string `json:"clientKey"`
	Title                    string `json:"title"`
	Instruction              string `json:"instruction"`
	ExpectedResult           string `json:"expectedResult"`
	SuggestedCommand         string `json:"suggestedCommand"`
	AIRequired               bool   `json:"aiRequired"`
	HumanRequired            bool   `json:"humanRequired"`
	HumanEvidenceRequirement string `json:"humanEvidenceRequirement"`
}
type SaveCheckPlanInput struct {
	ProjectID                   string           `json:"projectId"`
	ExpectedPreparationRevision int64            `json:"expectedPreparationRevision"`
	ExpectedPlanRevision        int64            `json:"expectedPlanRevision"`
	OperationID                 string           `json:"operationId"`
	Items                       []CheckItemInput `json:"items"`
}
type AssignedCheckID struct {
	ClientKey string `json:"clientKey"`
	CheckID   string `json:"checkId"`
}
type SavedCheckPlan struct {
	ProjectID           string            `json:"projectId"`
	PreparationRevision int64             `json:"preparationRevision"`
	PlanRevision        int64             `json:"planRevision"`
	Items               []CheckItem       `json:"items"`
	AssignedIDs         []AssignedCheckID `json:"assignedIds"`
	Readiness           Readiness         `json:"readiness"`
	UpdatedAt           time.Time         `json:"updatedAt"`
}
type SaveCheckPlanRecord struct {
	SaveCheckPlanInput
	NewIDs    []string  `json:"newIds"`
	EventID   string    `json:"eventId"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (p *Preparation) SaveCheckPlan(
	ctx context.Context,
	in SaveCheckPlanInput,
) (MutationResult[SavedCheckPlan], error) {
	ids := make([]string, len(in.Items))
	for i := range ids {
		ids[i] = p.newID()
	}

	return p.repository.SaveCheckPlan(ctx, SaveCheckPlanRecord{in, ids, p.newID(), p.now()})
}

type ChangeProjectWorkspaceInput struct {
	ProjectID               string `json:"projectId"`
	NewWorkspacePath        string `json:"newWorkspacePath"`
	OperationID             string `json:"operationId"`
	ExpectedProjectRevision int64  `json:"expectedProjectRevision"`
	ConfirmSessionReset     bool   `json:"confirmSessionReset"`
}
type WorkspaceChangeAccepted struct {
	ProjectID              string    `json:"projectId"`
	ProjectRevision        int64     `json:"projectRevision"`
	PreviousSessionID      string    `json:"previousSessionId"`
	SessionID              string    `json:"sessionId"`
	JobID                  string    `json:"jobId"`
	State                  string    `json:"state"`
	ConversationContinuity string    `json:"conversationContinuity"`
	AcceptedAt             time.Time `json:"acceptedAt"`
}
type ChangeProjectWorkspaceRecord struct {
	ChangeProjectWorkspaceInput
	SessionID  string    `json:"sessionId"`
	JobID      string    `json:"jobId"`
	EventID    string    `json:"eventId"`
	AcceptedAt time.Time `json:"acceptedAt"`
}

func (p *Preparation) ChangeProjectWorkspace(
	ctx context.Context,
	in ChangeProjectWorkspaceInput,
) (MutationResult[WorkspaceChangeAccepted], error) {
	abs, err := filepath.Abs(in.NewWorkspacePath)
	if err != nil {
		return MutationResult[WorkspaceChangeAccepted]{}, err
	}

	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return MutationResult[WorkspaceChangeAccepted]{}, err
	}

	if real != abs {
		return MutationResult[WorkspaceChangeAccepted]{}, &shared.Error{
			Code:    "validation_error",
			Message: "symlinkを含むworkspaceは使えません",
		}
	}

	root, err := os.OpenRoot(real)
	if err != nil {
		return MutationResult[WorkspaceChangeAccepted]{}, err
	}

	_ = root.Close()
	in.NewWorkspacePath = real

	return p.repository.ChangeProjectWorkspace(
		ctx,
		ChangeProjectWorkspaceRecord{in, p.newID(), p.newID(), p.newID(), p.now()},
	)
}

type SaveSessionPermissionPolicyInput struct {
	SessionID               string `json:"sessionId"`
	OperationID             string `json:"operationId"`
	Mode                    string `json:"mode"`
	ExpectedSessionRevision int64  `json:"expectedSessionRevision"`
}
type SaveSessionPermissionPolicyRecord struct {
	SaveSessionPermissionPolicyInput
	EventID   string    `json:"eventId"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (p *Preparation) SaveSessionPermissionPolicy(
	ctx context.Context,
	in SaveSessionPermissionPolicyInput,
) (MutationResult[SessionPermissionPolicy], error) {
	return p.repository.SaveSessionPermissionPolicy(ctx, SaveSessionPermissionPolicyRecord{in, p.newID(), p.now()})
}

type StartExecutionInput struct {
	ProjectID                   string `json:"projectId"`
	OperationID                 string `json:"operationId"`
	ExpectedPreparationRevision int64  `json:"expectedPreparationRevision"`
	ExpectedPlanRevision        int64  `json:"expectedPlanRevision"`
}
type ExecutionStarted struct {
	ProjectID         string    `json:"projectId"`
	ExecutionID       string    `json:"executionId"`
	SessionID         string    `json:"sessionId"`
	NextRoute         string    `json:"nextRoute"`
	ExecutionRevision int64     `json:"executionRevision"`
	StartedAt         time.Time `json:"startedAt"`
}
type StartExecutionRecord struct {
	StartExecutionInput
	ExecutionID string    `json:"executionId"`
	EventID     string    `json:"eventId"`
	StartedAt   time.Time `json:"startedAt"`
}

func (p *Preparation) StartExecution(
	ctx context.Context,
	in StartExecutionInput,
) (MutationResult[ExecutionStarted], error) {
	return p.repository.StartExecution(ctx, StartExecutionRecord{in, p.newID(), p.newID(), p.now()})
}

func (p *Preparation) GetPreparation(ctx context.Context, in PreparationViewQuery) (PreparationView, error) {
	return p.repository.GetPreparation(ctx, in)
}

type PreparationJobRepository interface {
	ClaimPreparationJob(context.Context) (*ClaimedPreparationJob, error)
	ClaimPreparationCancellation(context.Context) (*ClaimedPreparationJob, error)
	CompletePreparationJob(context.Context, PreparationJobCompletion) error
}
type ClaimedPreparationJob struct {
	JobID             string
	Kind              string
	SessionID         string
	PreviousSessionID string
	TurnID            string
	TargetJobID       string
	RunID             string
	AgentSessionID    string
	WorkspacePath     string
	Connection        agentconnection.ConnectionInput
	Content           []ContentPart
}
type PreparationJobCompletion struct {
	JobID           string
	SessionID       string
	AgentSessionID  string
	Success         bool
	StopReason      string
	Messages        []ConversationItem
	BriefSuggestion *PreparationBriefSuggestion
	Modes           *SessionModes
	ConfigOptions   []SessionConfigOption
	Capabilities    map[string]any
	ProtocolVersion string
	AgentName       string
	AgentVersion    string
	CompletedAt     time.Time
}
type PreparationBriefSuggestion struct {
	Purpose            string                       `json:"purpose"`
	CompletionCriteria []string                     `json:"completionCriteria"`
	IntendedUsers      string                       `json:"intendedUsers"`
	CheckItems         []PreparationCheckSuggestion `json:"checkItems"`
}
type PreparationCheckSuggestion struct {
	Title            string `json:"title"`
	Instruction      string `json:"instruction"`
	ExpectedResult   string `json:"expectedResult"`
	SuggestedCommand string `json:"suggestedCommand"`
}
type PreparationJobExecutor interface {
	ExecutePreparation(context.Context, ClaimedPreparationJob) (PreparationJobCompletion, error)
}

func RunPreparationJob(
	ctx context.Context,
	repo PreparationJobRepository,
	executor PreparationJobExecutor,
	now func() time.Time,
) (bool, error) {
	job, err := repo.ClaimPreparationJob(ctx)
	if err != nil || job == nil {
		return false, err
	}

	completion, executeErr := executor.ExecutePreparation(ctx, *job)
	completion.JobID = job.JobID
	completion.SessionID = job.SessionID
	completion.Success = executeErr == nil
	completion.CompletedAt = now()

	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()

	if err := repo.CompletePreparationJob(finishCtx, completion); err != nil {
		return true, err
	}

	return true, executeErr
}

func RunPreparationCancellation(
	ctx context.Context,
	repo PreparationJobRepository,
	executor PreparationJobExecutor,
	now func() time.Time,
) (bool, error) {
	job, err := repo.ClaimPreparationCancellation(ctx)
	if err != nil || job == nil {
		return false, err
	}

	completion, executeErr := executor.ExecutePreparation(ctx, *job)
	completion.JobID = job.JobID
	completion.SessionID = job.SessionID
	completion.Success = executeErr == nil
	completion.CompletedAt = now()

	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()

	if err := repo.CompletePreparationJob(finishCtx, completion); err != nil {
		return true, err
	}

	return true, executeErr
}
