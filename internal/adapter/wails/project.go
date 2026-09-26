package wails

import (
	"context"
	"time"

	"github.com/yukihito-jokyu/TEJUN/internal/application"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/project"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/shared"
	"github.com/yukihito-jokyu/TEJUN/internal/trace"
)

type ProjectService struct {
	projects *application.ProjectUseCases
	external *application.ProjectExternal
	trace    *trace.Writer
}

func NewProjectService(projects *application.ProjectUseCases, external *application.ProjectExternal) *ProjectService {
	return &ProjectService{projects: projects, external: external}
}

func (s *ProjectService) SetTrace(writer *trace.Writer) { s.trace = writer }

func (s *ProjectService) traceEntry(ctx context.Context, method, operationID, aggregateID string) context.Context {
	ctx = trace.WithWriter(ctx, s.trace)
	trace.Record(ctx, trace.Entry{
		Phase: "binding_entry", Method: method, OperationID: operationID, AggregateID: aggregateID,
	})

	return ctx
}

func traceAccepted(ctx context.Context, method, aggregateID, jobID string, result MutationReceipt) {
	trace.Record(ctx, trace.Entry{
		Phase: "accepted_response", Method: method, OperationID: result.OperationID, JobID: jobID,
		AggregateID: aggregateID, ChangeSequence: result.ChangeSequence, Status: "succeeded",
	})
}

type ProjectListQuery struct {
	Search   string   `json:"search"`
	Statuses []string `json:"statuses"`
	Sort     string   `json:"sort"`
	Cursor   string   `json:"cursor,omitempty"`
	Limit    int      `json:"limit"`
}

type ProjectProgress struct {
	Completed int `json:"completed"`
	Total     int `json:"total"`
}

type ProjectSummary struct {
	ProjectID                string          `json:"projectId"`
	Name                     string          `json:"name"`
	Description              string          `json:"description"`
	WorkspacePath            string          `json:"workspacePath"`
	Status                   string          `json:"status"`
	CurrentStage             string          `json:"currentStage"`
	Progress                 ProjectProgress `json:"progress"`
	AttentionRank            int             `json:"attentionRank"`
	AttentionReason          *string         `json:"attentionReason"`
	UpdatedAt                string          `json:"updatedAt"`
	CompletedAt              *string         `json:"completedAt"`
	ResumeRoute              string          `json:"resumeRoute"`
	CurrentProcedureID       *string         `json:"currentProcedureId"`
	CurrentProcedureRevision *int64          `json:"currentProcedureRevision,omitempty"`
	ConnectionState          string          `json:"connectionState"`
	ErrorSummary             *string         `json:"errorSummary"`
	Revision                 int64           `json:"revision"`
}

type ProjectListResult struct {
	Items          []ProjectSummary `json:"items"`
	Total          int              `json:"total"`
	NextCursor     *string          `json:"nextCursor"`
	GeneratedAt    string           `json:"generatedAt"`
	ChangeSequence int64            `json:"changeSequence"`
}

type CreateProjectInput struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	WorkspacePath string `json:"workspacePath"`
	ConnectionID  string `json:"connectionId"`
	OperationID   string `json:"operationId"`
}
type DuplicateProjectInput struct {
	SourceProjectID string `json:"sourceProjectId"`
	SourceRevision  int64  `json:"sourceRevision"`
	Name            string `json:"name"`
	WorkspacePath   string `json:"workspacePath"`
	OperationID     string `json:"operationId"`
}
type CreateRevisionInput struct {
	SourceProjectID         string `json:"sourceProjectId"`
	SourceProcedureID       string `json:"sourceProcedureId"`
	SourceProcedureRevision int64  `json:"sourceProcedureRevision"`
	Name                    string `json:"name"`
	WorkspacePath           string `json:"workspacePath"`
	OperationID             string `json:"operationId"`
}
type ArchiveProjectInput struct {
	ProjectID        string `json:"projectId"`
	ExpectedRevision int64  `json:"expectedRevision"`
	OperationID      string `json:"operationId"`
}
type DeleteProjectInput struct {
	ProjectID        string `json:"projectId"`
	ExpectedRevision int64  `json:"expectedRevision"`
	OperationID      string `json:"operationId"`
}
type ReconnectSessionInput struct {
	ProjectID        string `json:"projectId"`
	ExpectedRevision int64  `json:"expectedRevision"`
	Strategy         string `json:"strategy"`
	OperationID      string `json:"operationId"`
}
type VerifiedPathSelection struct {
	AbsolutePath   string `json:"absolutePath"`
	ResolvedPath   string `json:"resolvedPath"`
	VerifiedRootID string `json:"verifiedRootId"`
}
type OverwriteIdentity struct {
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
	Device uint64 `json:"device"`
	Inode  uint64 `json:"inode"`
}
type ExportProcedureInput struct {
	ProcedureID        string                `json:"procedureId"`
	ProcedureRevision  int64                 `json:"procedureRevision"`
	Format             string                `json:"format"`
	Destination        VerifiedPathSelection `json:"destination"`
	OverwriteConfirmed bool                  `json:"overwriteConfirmed"`
	OverwriteIdentity  *OverwriteIdentity    `json:"overwriteIdentity"`
	OperationID        string                `json:"operationId"`
}

type PrepareExportProcedureInput struct {
	ProcedureID       string `json:"procedureId"`
	ProcedureRevision int64  `json:"procedureRevision"`
	AbsolutePath      string `json:"absolutePath"`
}

type PreparedExportProcedure struct {
	Destination            VerifiedPathSelection `json:"destination"`
	OverwriteIdentity      *OverwriteIdentity    `json:"overwriteIdentity"`
	DestinationDisplayName string                `json:"destinationDisplayName"`
	OverwriteRequired      bool                  `json:"overwriteRequired"`
}

type CreatedProject struct {
	ProjectID    string `json:"projectId"`
	Revision     int64  `json:"revision"`
	Status       string `json:"status"`
	CurrentStage string `json:"currentStage"`
	NextRoute    string `json:"nextRoute"`
	CreatedAt    string `json:"createdAt"`
}
type ArchivedProject struct {
	ProjectID  string `json:"projectId"`
	Revision   int64  `json:"revision"`
	ArchivedAt string `json:"archivedAt"`
}
type DeletedProject struct {
	ProjectID string `json:"projectId"`
	DeletedAt string `json:"deletedAt"`
}
type SessionConnectionAccepted struct {
	ProjectID    string `json:"projectId"`
	SessionID    string `json:"sessionId"`
	JobID        string `json:"jobId"`
	State        string `json:"state"`
	RecoveryMode string `json:"recoveryMode"`
	AcceptedAt   string `json:"acceptedAt"`
}
type ExportAccepted struct {
	ExportID               string `json:"exportId"`
	JobID                  string `json:"jobId"`
	ProcedureID            string `json:"procedureId"`
	Format                 string `json:"format"`
	DestinationDisplayName string `json:"destinationDisplayName"`
	AcceptedAt             string `json:"acceptedAt"`
}

func (s *ProjectService) ListProjects(ctx context.Context, input ProjectListQuery) (ProjectListResult, error) {
	statuses := make([]project.Status, 0, len(input.Statuses))
	for _, status := range input.Statuses {
		statuses = append(statuses, project.Status(status))
	}

	result, err := s.projects.ListProjects(
		ctx,
		application.ProjectListQuery{
			Search:   input.Search,
			Statuses: statuses,
			Sort:     input.Sort,
			Cursor:   input.Cursor,
			Limit:    input.Limit,
		},
	)
	if err != nil {
		return ProjectListResult{}, storageError(err)
	}

	items := make([]ProjectSummary, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(
			items,
			ProjectSummary{
				ProjectID:     item.ProjectID,
				Name:          item.Name,
				Description:   item.Description,
				WorkspacePath: item.WorkspacePath,
				Status:        string(item.Status),
				CurrentStage:  item.CurrentStage,
				Progress: ProjectProgress{
					Completed: item.Progress.Completed,
					Total:     item.Progress.Total,
				},
				AttentionRank:            item.AttentionRank,
				AttentionReason:          item.AttentionReason,
				UpdatedAt:                item.UpdatedAt.Format(time.RFC3339Nano),
				CompletedAt:              projectTime(item.CompletedAt),
				ResumeRoute:              item.ResumeRoute,
				CurrentProcedureID:       item.CurrentProcedureID,
				CurrentProcedureRevision: item.CurrentProcedureRevision,
				ConnectionState:          item.ConnectionState,
				ErrorSummary:             item.ErrorSummary,
				Revision:                 item.Revision,
			},
		)
	}

	return ProjectListResult{
		Items:          items,
		Total:          result.Total,
		NextCursor:     result.NextCursor,
		GeneratedAt:    result.GeneratedAt.Format(time.RFC3339Nano),
		ChangeSequence: result.ChangeSequence,
	}, nil
}

func (s *ProjectService) CreateProject(
	ctx context.Context,
	input CreateProjectInput,
) (MutationResult[CreatedProject], error) {
	ctx = s.traceEntry(ctx, "CreateProject", input.OperationID, "")

	result, err := s.projects.CreateProject(
		ctx,
		application.CreateProjectInput{
			Name:          input.Name,
			Description:   input.Description,
			WorkspacePath: input.WorkspacePath,
			ConnectionID:  input.ConnectionID,
			OperationID:   input.OperationID,
		},
	)
	if err != nil {
		return MutationResult[CreatedProject]{}, storageError(err)
	}

	traceAccepted(ctx, "CreateProject", result.Data.ProjectID, "", receipt(result.Receipt))

	return createdProjectResult(result), nil
}

func (s *ProjectService) DuplicateProject(
	ctx context.Context,
	input DuplicateProjectInput,
) (MutationResult[CreatedProject], error) {
	ctx = s.traceEntry(ctx, "DuplicateProject", input.OperationID, input.SourceProjectID)

	result, err := s.projects.DuplicateProject(
		ctx,
		application.DuplicateProjectInput{
			SourceProjectID: input.SourceProjectID,
			SourceRevision:  input.SourceRevision,
			Name:            input.Name,
			WorkspacePath:   input.WorkspacePath,
			OperationID:     input.OperationID,
		},
	)
	if err != nil {
		return MutationResult[CreatedProject]{}, storageError(err)
	}

	traceAccepted(ctx, "DuplicateProject", result.Data.ProjectID, "", receipt(result.Receipt))

	return createdProjectResult(result), nil
}

func (s *ProjectService) CreateRevision(
	ctx context.Context,
	input CreateRevisionInput,
) (MutationResult[CreatedProject], error) {
	ctx = s.traceEntry(ctx, "CreateRevision", input.OperationID, input.SourceProjectID)

	result, err := s.projects.CreateRevision(
		ctx,
		application.CreateRevisionInput{
			SourceProjectID:         input.SourceProjectID,
			SourceProcedureID:       input.SourceProcedureID,
			SourceProcedureRevision: input.SourceProcedureRevision,
			Name:                    input.Name,
			WorkspacePath:           input.WorkspacePath,
			OperationID:             input.OperationID,
		},
	)
	if err != nil {
		return MutationResult[CreatedProject]{}, storageError(err)
	}

	traceAccepted(ctx, "CreateRevision", result.Data.ProjectID, "", receipt(result.Receipt))

	return createdProjectResult(result), nil
}

func (s *ProjectService) ArchiveProject(
	ctx context.Context,
	input ArchiveProjectInput,
) (MutationResult[ArchivedProject], error) {
	ctx = s.traceEntry(ctx, "ArchiveProject", input.OperationID, input.ProjectID)

	result, err := s.projects.ArchiveProject(
		ctx,
		application.ArchiveProjectInput{
			ProjectID:        input.ProjectID,
			ExpectedRevision: input.ExpectedRevision,
			OperationID:      input.OperationID,
		},
	)
	if err != nil {
		return MutationResult[ArchivedProject]{}, storageError(err)
	}

	traceAccepted(ctx, "ArchiveProject", result.Data.ProjectID, "", receipt(result.Receipt))

	return MutationResult[ArchivedProject]{
		Data: ArchivedProject{
			ProjectID:  result.Data.ProjectID,
			Revision:   result.Data.Revision,
			ArchivedAt: result.Data.ArchivedAt.Format(time.RFC3339Nano),
		},
		Receipt: receipt(result.Receipt),
	}, nil
}

func (s *ProjectService) DeleteProject(
	ctx context.Context,
	input DeleteProjectInput,
) (MutationResult[DeletedProject], error) {
	ctx = s.traceEntry(ctx, "DeleteProject", input.OperationID, input.ProjectID)

	result, err := s.projects.DeleteProject(ctx, application.DeleteProjectInput{
		ProjectID: input.ProjectID, ExpectedRevision: input.ExpectedRevision, OperationID: input.OperationID,
	})
	if err != nil {
		return MutationResult[DeletedProject]{}, storageError(err)
	}

	traceAccepted(ctx, "DeleteProject", result.Data.ProjectID, "", receipt(result.Receipt))

	return MutationResult[DeletedProject]{
		Data: DeletedProject{
			ProjectID: result.Data.ProjectID,
			DeletedAt: result.Data.DeletedAt.Format(time.RFC3339Nano),
		},
		Receipt: receipt(result.Receipt),
	}, nil
}

func (s *ProjectService) ReconnectProjectSession(
	ctx context.Context,
	input ReconnectSessionInput,
) (MutationResult[SessionConnectionAccepted], error) {
	ctx = s.traceEntry(ctx, "ReconnectProjectSession", input.OperationID, input.ProjectID)

	result, err := s.external.ReconnectProjectSession(
		ctx,
		application.ReconnectSessionInput{
			ProjectID:        input.ProjectID,
			ExpectedRevision: input.ExpectedRevision,
			Strategy:         input.Strategy,
			OperationID:      input.OperationID,
		},
	)
	if err != nil {
		return MutationResult[SessionConnectionAccepted]{}, storageError(err)
	}

	traceAccepted(ctx, "ReconnectProjectSession", result.Data.ProjectID, result.Data.JobID, receipt(result.Receipt))

	return MutationResult[SessionConnectionAccepted]{
		Data: SessionConnectionAccepted{
			ProjectID:    result.Data.ProjectID,
			SessionID:    result.Data.SessionID,
			JobID:        result.Data.JobID,
			State:        result.Data.State,
			RecoveryMode: result.Data.RecoveryMode,
			AcceptedAt:   result.Data.AcceptedAt.Format(time.RFC3339Nano),
		},
		Receipt: receipt(result.Receipt),
	}, nil
}

func (s *ProjectService) PrepareExportProcedure(
	ctx context.Context,
	input PrepareExportProcedureInput,
) (PreparedExportProcedure, error) {
	prepared, err := s.external.PrepareExportProcedure(ctx, application.PrepareExportProcedureInput{
		ProcedureID: input.ProcedureID, ProcedureRevision: input.ProcedureRevision, AbsolutePath: input.AbsolutePath,
	})
	if err != nil {
		return PreparedExportProcedure{}, storageError(err)
	}

	var identity *OverwriteIdentity
	if prepared.OverwriteIdentity != nil {
		identity = &OverwriteIdentity{
			Size: prepared.OverwriteIdentity.Size, SHA256: prepared.OverwriteIdentity.SHA256,
			Device: prepared.OverwriteIdentity.Device, Inode: prepared.OverwriteIdentity.Inode,
		}
	}

	return PreparedExportProcedure{
		Destination: VerifiedPathSelection{
			AbsolutePath:   prepared.Destination.AbsolutePath,
			ResolvedPath:   prepared.Destination.ResolvedPath,
			VerifiedRootID: prepared.Destination.VerifiedRootID,
		},
		OverwriteIdentity: identity, DestinationDisplayName: prepared.DestinationDisplayName,
		OverwriteRequired: prepared.OverwriteRequired,
	}, nil
}

func (s *ProjectService) ExportProcedure(
	ctx context.Context,
	input ExportProcedureInput,
) (MutationResult[ExportAccepted], error) {
	ctx = s.traceEntry(ctx, "ExportProcedure", input.OperationID, input.ProcedureID)
	if input.Destination.VerifiedRootID == "" {
		return MutationResult[ExportAccepted]{}, storageError(
			&shared.Error{
				Code:        "validation_error",
				Message:     "保存先の確認が必要です",
				FieldErrors: map[string]string{"destination": "保存先の確認が必要です"},
			},
		)
	}

	var identity *application.OverwriteIdentity
	if input.OverwriteIdentity != nil {
		identity = &application.OverwriteIdentity{
			Size:   input.OverwriteIdentity.Size,
			SHA256: input.OverwriteIdentity.SHA256,
			Device: input.OverwriteIdentity.Device,
			Inode:  input.OverwriteIdentity.Inode,
		}
	}

	result, err := s.external.ExportProcedure(
		ctx,
		application.ExportProcedureInput{
			ProcedureID:       input.ProcedureID,
			ProcedureRevision: input.ProcedureRevision,
			Format:            input.Format,
			Destination: application.VerifiedPathSelection{
				AbsolutePath:   input.Destination.AbsolutePath,
				ResolvedPath:   input.Destination.ResolvedPath,
				VerifiedRootID: input.Destination.VerifiedRootID,
			},
			OverwriteConfirmed: input.OverwriteConfirmed,
			OverwriteIdentity:  identity,
			OperationID:        input.OperationID,
		},
	)
	if err != nil {
		return MutationResult[ExportAccepted]{}, storageError(err)
	}

	traceAccepted(ctx, "ExportProcedure", result.Data.ExportID, result.Data.JobID, receipt(result.Receipt))

	return MutationResult[ExportAccepted]{
		Data: ExportAccepted{
			ExportID:               result.Data.ExportID,
			JobID:                  result.Data.JobID,
			ProcedureID:            result.Data.ProcedureID,
			Format:                 result.Data.Format,
			DestinationDisplayName: result.Data.DestinationDisplayName,
			AcceptedAt:             result.Data.AcceptedAt.Format(time.RFC3339Nano),
		},
		Receipt: receipt(result.Receipt),
	}, nil
}

func createdProjectResult(
	result application.MutationResult[application.CreatedProject],
) MutationResult[CreatedProject] {
	return MutationResult[CreatedProject]{
		Data: CreatedProject{
			ProjectID:    result.Data.ProjectID,
			Revision:     result.Data.Revision,
			Status:       string(result.Data.Status),
			CurrentStage: result.Data.CurrentStage,
			NextRoute:    result.Data.NextRoute,
			CreatedAt:    result.Data.CreatedAt.Format(time.RFC3339Nano),
		},
		Receipt: receipt(result.Receipt),
	}
}

func projectTime(value *time.Time) *string {
	if value == nil {
		return nil
	}

	formatted := value.Format(time.RFC3339Nano)

	return &formatted
}
