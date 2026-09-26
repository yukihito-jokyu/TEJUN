package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/yukihito-jokyu/TEJUN/internal/domain/project"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/shared"
)

type ProjectRepository interface {
	ListProjects(context.Context, ProjectListQuery) (ProjectListResult, error)
	CreateProject(context.Context, ProjectCreateRecord) (MutationResult[CreatedProject], error)
	DuplicateProject(context.Context, ProjectCopyRecord) (MutationResult[CreatedProject], error)
	CreateRevision(context.Context, ProjectCopyRecord) (MutationResult[CreatedProject], error)
	ArchiveProject(context.Context, ProjectArchiveRecord) (MutationResult[ArchivedProject], error)
	DeleteProject(context.Context, ProjectDeleteRecord) (MutationResult[DeletedProject], error)
}

type WorkspaceValidator interface{ Validate(string) error }

type ProjectUseCases struct {
	repository ProjectRepository
	workspace  WorkspaceValidator
	now        func() time.Time
	newID      func() string
}

func NewProjectUseCases(
	repository ProjectRepository,
	workspace WorkspaceValidator,
	now func() time.Time,
	newID func() string,
) *ProjectUseCases {
	return &ProjectUseCases{repository: repository, workspace: workspace, now: now, newID: newID}
}

type ProjectListQuery struct {
	Search   string
	Statuses []project.Status
	Sort     string
	Cursor   string
	Limit    int
}

type ProjectSummary struct {
	ProjectID                string
	Name                     string
	Description              string
	WorkspacePath            string
	Status                   project.Status
	CurrentStage             string
	Progress                 ProjectProgress
	AttentionRank            int
	AttentionReason          *string
	UpdatedAt                time.Time
	CompletedAt              *time.Time
	ResumeRoute              string
	CurrentProcedureID       *string
	CurrentProcedureRevision *int64
	ConnectionState          string
	ErrorSummary             *string
	Revision                 int64
}

type ProjectProgress struct{ Completed, Total int }

type ProjectListResult struct {
	Items          []ProjectSummary
	Total          int
	NextCursor     *string
	GeneratedAt    time.Time
	ChangeSequence int64
}

type CreateProjectInput struct {
	Name, Description, WorkspacePath, ConnectionID, OperationID string
}

type DuplicateProjectInput struct {
	SourceProjectID string
	SourceRevision  int64
	Name            string
	WorkspacePath   string
	OperationID     string
}

type CreateRevisionInput struct {
	SourceProjectID         string
	SourceProcedureID       string
	SourceProcedureRevision int64
	Name                    string
	WorkspacePath           string
	OperationID             string
}

type ArchiveProjectInput struct {
	ProjectID        string
	ExpectedRevision int64
	OperationID      string
}

type DeleteProjectInput struct {
	ProjectID        string
	ExpectedRevision int64
	OperationID      string
}

type DeletedProject struct {
	ProjectID string
	DeletedAt time.Time
}

type CreatedProject struct {
	ProjectID    string
	Revision     int64
	Status       project.Status
	CurrentStage string
	NextRoute    string
	CreatedAt    time.Time
}

type ArchivedProject struct {
	ProjectID  string
	Revision   int64
	ArchivedAt time.Time
}

type ProjectCreateRecord struct {
	CreateProjectInput
	ProjectID, RequestHash string
	Receipt                MutationReceipt
	Event                  OutboxEvent
}

type ProjectCopyRecord struct {
	SourceProjectID, SourceProcedureID, Name, WorkspacePath, ProjectID, RequestHash string
	SourceRevision, SourceProcedureRevision                                         int64
	OperationID                                                                     string
	Receipt                                                                         MutationReceipt
	Event                                                                           OutboxEvent
}

type ProjectArchiveRecord struct {
	ArchiveProjectInput
	RequestHash string
	Receipt     MutationReceipt
	Event       OutboxEvent
}

type ProjectDeleteRecord struct {
	DeleteProjectInput
	RequestHash string
	Receipt     MutationReceipt
	Event       OutboxEvent
}

func (u *ProjectUseCases) ListProjects(ctx context.Context, query ProjectListQuery) (ProjectListResult, error) {
	if query.Limit == 0 {
		query.Limit = 50
	}

	if query.Limit < 1 || query.Limit > 100 {
		return ProjectListResult{}, fieldError("limit", "1から100を指定してください")
	}

	if query.Sort == "" {
		query.Sort = "attention_desc"
	}

	if query.Sort != "attention_desc" && query.Sort != "updated_desc" && query.Sort != "name_asc" {
		return ProjectListResult{}, fieldError("sort", "並び順が不正です")
	}

	for _, status := range query.Statuses {
		if !project.ValidStatus(status) {
			return ProjectListResult{}, fieldError("statuses", "状態が不正です")
		}
	}

	return u.repository.ListProjects(ctx, query)
}

func (u *ProjectUseCases) CreateProject(
	ctx context.Context,
	input CreateProjectInput,
) (MutationResult[CreatedProject], error) {
	if err := validateProjectInput(input.Name, input.Description, input.WorkspacePath, input.OperationID); err != nil {
		return MutationResult[CreatedProject]{}, err
	}

	if err := u.workspace.Validate(input.WorkspacePath); err != nil {
		return MutationResult[CreatedProject]{}, err
	}

	now := u.now().UTC()
	id := u.newID()
	record := ProjectCreateRecord{
		CreateProjectInput: input, ProjectID: id, RequestHash: projectHash(input),
		Receipt: MutationReceipt{OperationID: input.OperationID, CommittedAt: now},
		Event: OutboxEvent{
			ID: u.newID(), Name: "project.changed", EmittedAt: now,
			AggregateType: "project", AggregateID: id, Correlation: input.OperationID,
		},
	}

	return u.repository.CreateProject(ctx, record)
}

func (u *ProjectUseCases) DuplicateProject(
	ctx context.Context,
	input DuplicateProjectInput,
) (MutationResult[CreatedProject], error) {
	if err := validateProjectInput(input.Name, "", input.WorkspacePath, input.OperationID); err != nil {
		return MutationResult[CreatedProject]{}, err
	}

	if err := u.workspace.Validate(input.WorkspacePath); err != nil {
		return MutationResult[CreatedProject]{}, err
	}

	if input.SourceProjectID == "" || input.SourceRevision < 1 {
		return MutationResult[CreatedProject]{}, fieldError("sourceProjectId", "複製元が不正です")
	}

	now := u.now().UTC()
	id := u.newID()

	return u.repository.DuplicateProject(ctx, ProjectCopyRecord{
		SourceProjectID: input.SourceProjectID, SourceRevision: input.SourceRevision,
		Name: input.Name, WorkspacePath: input.WorkspacePath, OperationID: input.OperationID,
		ProjectID: id, RequestHash: projectHash(input),
		Receipt: MutationReceipt{OperationID: input.OperationID, CommittedAt: now},
		Event: OutboxEvent{
			ID: u.newID(), Name: "project.changed", EmittedAt: now,
			AggregateType: "project", AggregateID: id, Correlation: input.OperationID,
		},
	})
}

func (u *ProjectUseCases) CreateRevision(
	ctx context.Context,
	input CreateRevisionInput,
) (MutationResult[CreatedProject], error) {
	if err := validateProjectInput(input.Name, "", input.WorkspacePath, input.OperationID); err != nil {
		return MutationResult[CreatedProject]{}, err
	}

	if err := u.workspace.Validate(input.WorkspacePath); err != nil {
		return MutationResult[CreatedProject]{}, err
	}

	if input.SourceProjectID == "" || input.SourceProcedureID == "" || input.SourceProcedureRevision < 1 {
		return MutationResult[CreatedProject]{}, fieldError("sourceProcedureId", "改訂元が不正です")
	}

	now := u.now().UTC()
	id := u.newID()

	return u.repository.CreateRevision(ctx, ProjectCopyRecord{
		SourceProjectID: input.SourceProjectID, SourceProcedureID: input.SourceProcedureID,
		SourceProcedureRevision: input.SourceProcedureRevision,
		Name:                    input.Name, WorkspacePath: input.WorkspacePath, OperationID: input.OperationID,
		ProjectID: id, RequestHash: projectHash(input),
		Receipt: MutationReceipt{OperationID: input.OperationID, CommittedAt: now},
		Event: OutboxEvent{
			ID: u.newID(), Name: "project.changed", EmittedAt: now,
			AggregateType: "project", AggregateID: id, Correlation: input.OperationID,
		},
	})
}

func (u *ProjectUseCases) ArchiveProject(
	ctx context.Context,
	input ArchiveProjectInput,
) (MutationResult[ArchivedProject], error) {
	if input.ProjectID == "" || input.ExpectedRevision < 1 || input.OperationID == "" {
		return MutationResult[ArchivedProject]{}, fieldError("projectId", "アーカイブ対象が不正です")
	}

	now := u.now().UTC()

	return u.repository.ArchiveProject(ctx, ProjectArchiveRecord{
		ArchiveProjectInput: input, RequestHash: projectHash(input),
		Receipt: MutationReceipt{OperationID: input.OperationID, CommittedAt: now},
		Event: OutboxEvent{
			ID: u.newID(), Name: "project.changed", EmittedAt: now,
			AggregateType: "project", AggregateID: input.ProjectID, Correlation: input.OperationID,
		},
	})
}

func (u *ProjectUseCases) DeleteProject(
	ctx context.Context,
	input DeleteProjectInput,
) (MutationResult[DeletedProject], error) {
	if input.ProjectID == "" || input.ExpectedRevision < 1 || input.OperationID == "" {
		return MutationResult[DeletedProject]{}, fieldError("projectId", "削除対象が不正です")
	}

	now := u.now().UTC()

	return u.repository.DeleteProject(ctx, ProjectDeleteRecord{
		DeleteProjectInput: input, RequestHash: projectHash(input),
		Receipt: MutationReceipt{OperationID: input.OperationID, CommittedAt: now},
		Event: OutboxEvent{
			ID: u.newID(), Name: "project.deleted", EmittedAt: now,
			AggregateType: "project", AggregateID: input.ProjectID, Correlation: input.OperationID,
		},
	})
}

func validateProjectInput(name, description, workspacePath, operationID string) error {
	if strings.TrimSpace(name) == "" || utf8.RuneCountInString(name) > 200 {
		return fieldError("name", "名前は1から200文字で入力してください")
	}

	if len(description) > 20000 {
		return fieldError("description", "説明が長すぎます")
	}

	if workspacePath == "" || operationID == "" {
		return fieldError("workspacePath", "作業場所とoperationIdが必要です")
	}

	return nil
}

func fieldError(field, message string) error {
	return &shared.Error{Code: "validation_error", Message: message, FieldErrors: map[string]string{field: message}}
}

func projectHash(value any) string {
	encoded, _ := json.Marshal(value)
	digest := sha256.Sum256(append([]byte("project:v1:"), encoded...))

	return hex.EncodeToString(digest[:])
}
