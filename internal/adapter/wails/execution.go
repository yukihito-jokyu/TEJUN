package wails

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/yukihito-jokyu/TEJUN/internal/application"
)

type ExecutionViewQuery struct {
	ProjectID          string `json:"projectId"`
	ConversationCursor *int64 `json:"conversationCursor,omitempty"`
	ConversationLimit  int    `json:"conversationLimit"`
}

func (s *ExecutionService) GetExecution(ctx context.Context, in ExecutionViewQuery) (application.ExecutionView, error) {
	if err := required("projectId", in.ProjectID); err != nil {
		return application.ExecutionView{}, err
	}

	if in.ConversationLimit < 1 || in.ConversationLimit > 100 {
		return application.ExecutionView{}, validation(map[string]string{"conversationLimit": "1から100を指定してください"})
	}

	if in.ConversationCursor != nil && *in.ConversationCursor < 0 {
		return application.ExecutionView{}, validation(map[string]string{"conversationCursor": "0以上を指定してください"})
	}

	view, err := s.execution.GetView(ctx, application.ExecutionViewQuery{
		ProjectID: in.ProjectID, ConversationCursor: in.ConversationCursor,
		ConversationLimit: in.ConversationLimit,
	})
	if err != nil {
		return application.ExecutionView{}, storageError(err)
	}

	view.Checks = NonNilSlice(view.Checks)
	view.PendingPermissions = NonNilSlice(view.PendingPermissions)
	view.Conversation.Items = NonNilSlice(view.Conversation.Items)
	view.Readiness.BlockingReasons = NonNilSlice(view.Readiness.BlockingReasons)

	view.Session.ConfigOptions = NonNilSlice(view.Session.ConfigOptions)
	if view.Session.Modes != nil {
		view.Session.Modes.Available = NonNilSlice(view.Session.Modes.Available)
	}

	if view.ActiveRun != nil {
		view.ActiveRun.TargetedCheckIDs = NonNilSlice(view.ActiveRun.TargetedCheckIDs)
	}

	for i := range view.Checks {
		view.Checks[i].Evidence.AI = NonNilSlice(view.Checks[i].Evidence.AI)

		view.Checks[i].Evidence.Human = NonNilSlice(view.Checks[i].Evidence.Human)
		if s.previewURL != nil {
			for j := range view.Checks[i].Evidence.AI {
				evidence := &view.Checks[i].Evidence.AI[j]
				if evidence.Kind == "image" {
					evidence.PreviewURL = s.previewURL(
						in.ProjectID,
						view.Execution.ExecutionID,
						view.Checks[i].CheckID,
						evidence.EvidenceID,
					)
				}
			}

			for j := range view.Checks[i].Evidence.Human {
				evidence := &view.Checks[i].Evidence.Human[j]
				if evidence.Kind == "image" {
					evidence.PreviewURL = s.previewURL(
						in.ProjectID,
						view.Execution.ExecutionID,
						view.Checks[i].CheckID,
						evidence.EvidenceID,
					)
				}
			}
		}
	}

	for i := range view.PendingPermissions {
		view.PendingPermissions[i].Options = NonNilSlice(view.PendingPermissions[i].Options)
		view.PendingPermissions[i].ToolCall.Locations = NonNilSlice(view.PendingPermissions[i].ToolCall.Locations)
		view.PendingPermissions[i].ToolCall.Details = NonNilSlice(view.PendingPermissions[i].ToolCall.Details)
	}

	for i := range view.Conversation.Items {
		view.Conversation.Items[i].Content = NonNilSlice(view.Conversation.Items[i].Content)
	}

	return view, nil
}

type RunPendingChecksInput struct {
	ExecutionID      string   `json:"executionId"`
	ExpectedRevision int64    `json:"expectedRevision"`
	CheckIDs         []string `json:"checkIds,omitempty"`
	OperationID      string   `json:"operationId"`
}
type SendExecutionMessageInput struct {
	ExecutionID string                    `json:"executionId"`
	Content     []application.ContentPart `json:"content"`
	OperationID string                    `json:"operationId"`
}
type PermissionResponseInput struct {
	SessionID           string `json:"sessionId"`
	PermissionRequestID string `json:"permissionRequestId"`
	OptionID            string `json:"optionId"`
	OperationID         string `json:"operationId"`
}
type AttachHumanEvidenceInput struct {
	ExecutionID      string `json:"executionId"`
	CheckID          string `json:"checkId"`
	ExpectedRevision int64  `json:"expectedRevision"`
	Kind             string `json:"kind"`
	Text             string `json:"text,omitempty"`
	SourcePath       string `json:"sourcePath,omitempty"`
	DisplayName      string `json:"displayName,omitempty"`
	OperationID      string `json:"operationId"`
}
type SetHumanCheckInput struct {
	ExecutionID      string `json:"executionId"`
	CheckID          string `json:"checkId"`
	Checked          bool   `json:"checked"`
	ExpectedRevision int64  `json:"expectedRevision"`
	OperationID      string `json:"operationId"`
}
type GenerateProcedureDraftInput struct {
	ExecutionID      string `json:"executionId"`
	ExpectedRevision int64  `json:"expectedRevision"`
	OperationID      string `json:"operationId"`
}
type ExecutionJobAccepted struct {
	ExecutionID      string   `json:"executionId"`
	RunID            string   `json:"runId,omitempty"`
	TurnID           string   `json:"turnId,omitempty"`
	SessionID        string   `json:"sessionId"`
	JobID            string   `json:"jobId"`
	AcceptedAt       string   `json:"acceptedAt"`
	TargetedCheckIDs []string `json:"targetedCheckIds"`
}
type PermissionResponseResult struct {
	PermissionRequestID string `json:"permissionRequestId"`
	Status              string `json:"status"`
	SelectedOptionID    string `json:"selectedOptionId"`
	RespondedAt         string `json:"respondedAt"`
}
type EvidenceAttached struct {
	ExecutionID       string                      `json:"executionId"`
	CheckID           string                      `json:"checkId"`
	Evidence          application.EvidenceSummary `json:"evidence"`
	ExecutionRevision int64                       `json:"executionRevision"`
	Readiness         ExecutionReadiness          `json:"readiness"`
}
type ExecutionReadiness struct {
	CanGenerateProcedure bool     `json:"canGenerateProcedure"`
	BlockingReasons      []string `json:"blockingReasons"`
}
type CheckUpdate struct {
	ExecutionID       string                         `json:"executionId"`
	Check             application.ExecutionCheckView `json:"check"`
	ExecutionRevision int64                          `json:"executionRevision"`
	Readiness         ExecutionReadiness             `json:"readiness"`
}
type ProcedureGenerationAccepted struct {
	ProcedureID string `json:"procedureId"`
	NextRoute   string `json:"nextRoute"`
	AcceptedAt  string `json:"acceptedAt"`
}

func (s *ExecutionService) RunPendingChecks(
	ctx context.Context,
	in RunPendingChecksInput,
) (MutationResult[ExecutionJobAccepted], error) {
	if err := requireFields(
		map[string]string{"executionId": in.ExecutionID, "operationId": in.OperationID},
	); err != nil {
		return MutationResult[ExecutionJobAccepted]{}, err
	}

	for _, id := range in.CheckIDs {
		if strings.TrimSpace(id) == "" {
			return MutationResult[ExecutionJobAccepted]{}, validation(
				map[string]string{"checkIds": "空のcheckIdは指定できません"},
			)
		}
	}

	result, err := s.runner.RunPendingChecks(
		ctx,
		application.RunPendingChecksInput{
			ExecutionID:      in.ExecutionID,
			ExpectedRevision: in.ExpectedRevision,
			CheckIDs:         in.CheckIDs,
			OperationID:      in.OperationID,
		},
	)
	if err != nil {
		return MutationResult[ExecutionJobAccepted]{}, err
	}

	return executionJobResult(result), nil
}

func (s *ExecutionService) SendExecutionMessage(
	ctx context.Context,
	in SendExecutionMessageInput,
) (MutationResult[ExecutionJobAccepted], error) {
	if err := requireFields(
		map[string]string{"executionId": in.ExecutionID, "operationId": in.OperationID},
	); err != nil {
		return MutationResult[ExecutionJobAccepted]{}, err
	}

	if len(in.Content) != 1 || in.Content[0].Type != "text" || strings.TrimSpace(in.Content[0].Text) == "" {
		return MutationResult[ExecutionJobAccepted]{}, validation(map[string]string{"content": "空ではないtextを1件指定してください"})
	}

	result, err := s.runner.SendMessage(
		ctx,
		application.SendExecutionMessageInput{
			ExecutionID: in.ExecutionID,
			Text:        in.Content[0].Text,
			OperationID: in.OperationID,
		},
	)
	if err != nil {
		return MutationResult[ExecutionJobAccepted]{}, err
	}

	return executionJobResult(result), nil
}

func executionJobResult(
	result application.MutationResult[application.ExecutionJobAccepted],
) MutationResult[ExecutionJobAccepted] {
	d := result.Data

	return MutationResult[ExecutionJobAccepted]{
		Data: ExecutionJobAccepted{
			ExecutionID:      d.ExecutionID,
			RunID:            d.RunID,
			TurnID:           d.TurnID,
			SessionID:        d.SessionID,
			JobID:            d.JobID,
			AcceptedAt:       d.AcceptedAt.Format(time.RFC3339Nano),
			TargetedCheckIDs: NonNilSlice(d.TargetedCheckIDs),
		},
		Receipt: receipt(result.Receipt),
	}
}

func (s *ExecutionService) RespondToPermissionRequest(
	ctx context.Context,
	in PermissionResponseInput,
) (MutationResult[PermissionResponseResult], error) {
	if err := requireFields(
		map[string]string{
			"sessionId":           in.SessionID,
			"permissionRequestId": in.PermissionRequestID,
			"optionId":            in.OptionID,
			"operationId":         in.OperationID,
		},
	); err != nil {
		return MutationResult[PermissionResponseResult]{}, err
	}

	result, err := s.permission.Respond(
		ctx,
		application.PermissionClaim{
			SessionID:           in.SessionID,
			PermissionRequestID: in.PermissionRequestID,
			OptionID:            in.OptionID,
			OperationID:         in.OperationID,
		},
	)
	if err != nil {
		return MutationResult[PermissionResponseResult]{}, err
	}

	return MutationResult[PermissionResponseResult]{
		Data: PermissionResponseResult{
			PermissionRequestID: result.Data.PermissionRequestID,
			Status:              result.Data.Status,
			SelectedOptionID:    result.Data.SelectedOptionID,
			RespondedAt:         result.Data.RespondedAt.Format(time.RFC3339Nano),
		},
		Receipt: receipt(result.Receipt),
	}, nil
}

func (s *ExecutionService) AttachHumanEvidence(
	ctx context.Context,
	in AttachHumanEvidenceInput,
) (MutationResult[EvidenceAttached], error) {
	if err := requireFields(
		map[string]string{
			"executionId": in.ExecutionID,
			"checkId":     in.CheckID,
			"operationId": in.OperationID,
			"kind":        in.Kind,
		},
	); err != nil {
		return MutationResult[EvidenceAttached]{}, err
	}

	switch in.Kind {
	case "text":
		if strings.TrimSpace(in.Text) == "" || in.SourcePath != "" {
			return MutationResult[EvidenceAttached]{}, validation(map[string]string{"text": "textとsourcePathを確認してください"})
		}
	case "image":
		if !filepath.IsAbs(in.SourcePath) || in.Text != "" {
			return MutationResult[EvidenceAttached]{}, validation(map[string]string{"sourcePath": "画像の絶対pathを指定してください"})
		}
	default:
		return MutationResult[EvidenceAttached]{}, validation(map[string]string{"kind": "textかimageを指定してください"})
	}

	result, err := s.evidence.Attach(
		ctx,
		application.AttachHumanEvidenceInput{
			ExecutionID:      in.ExecutionID,
			CheckID:          in.CheckID,
			ExpectedRevision: in.ExpectedRevision,
			Kind:             in.Kind,
			Text:             in.Text,
			SourcePath:       in.SourcePath,
			DisplayName:      in.DisplayName,
			OperationID:      in.OperationID,
		},
	)
	if err != nil {
		return MutationResult[EvidenceAttached]{}, err
	}

	d := result.Data
	if d.Evidence.Kind == "image" && s.previewURL != nil {
		projectID, err := s.execution.ProjectIDForExecution(ctx, d.ExecutionID)
		if err != nil {
			return MutationResult[EvidenceAttached]{}, storageError(err)
		}

		d.Evidence.PreviewURL = s.previewURL(projectID, d.ExecutionID, d.CheckID, d.Evidence.EvidenceID)
	}

	return MutationResult[EvidenceAttached]{
		Data: EvidenceAttached{
			ExecutionID:       d.ExecutionID,
			CheckID:           d.CheckID,
			Evidence:          d.Evidence,
			ExecutionRevision: d.ExecutionRevision,
			Readiness: ExecutionReadiness{
				CanGenerateProcedure: d.CanGenerate,
				BlockingReasons:      NonNilSlice(d.BlockingReasons),
			},
		},
		Receipt: receipt(result.Receipt),
	}, nil
}

func (s *ExecutionService) SetHumanCheck(
	ctx context.Context,
	in SetHumanCheckInput,
) (MutationResult[CheckUpdate], error) {
	if err := requireFields(
		map[string]string{"executionId": in.ExecutionID, "checkId": in.CheckID, "operationId": in.OperationID},
	); err != nil {
		return MutationResult[CheckUpdate]{}, err
	}

	result, err := s.execution.SetHumanCheck(
		ctx,
		application.HumanCheckRecord{
			ExecutionID:      in.ExecutionID,
			CheckID:          in.CheckID,
			Checked:          in.Checked,
			ExpectedRevision: in.ExpectedRevision,
			OperationID:      in.OperationID,
		},
	)
	if err != nil {
		return MutationResult[CheckUpdate]{}, err
	}

	d := result.Data
	d.Check.Evidence.AI = NonNilSlice(d.Check.Evidence.AI)
	d.Check.Evidence.Human = NonNilSlice(d.Check.Evidence.Human)

	return MutationResult[CheckUpdate]{
		Data: CheckUpdate{
			ExecutionID:       d.ExecutionID,
			Check:             d.Check,
			ExecutionRevision: d.ExecutionRevision,
			Readiness: ExecutionReadiness{
				CanGenerateProcedure: d.CanGenerate,
				BlockingReasons:      NonNilSlice(d.BlockingReasons),
			},
		},
		Receipt: receipt(result.Receipt),
	}, nil
}

func (s *ExecutionService) GenerateProcedureDraft(
	ctx context.Context,
	in GenerateProcedureDraftInput,
) (MutationResult[ProcedureGenerationAccepted], error) {
	if err := requireFields(
		map[string]string{"executionId": in.ExecutionID, "operationId": in.OperationID},
	); err != nil {
		return MutationResult[ProcedureGenerationAccepted]{}, err
	}

	result, err := s.execution.GenerateProcedureDraft(
		ctx,
		application.ProcedureDraftRecord{
			ExecutionID:      in.ExecutionID,
			ExpectedRevision: in.ExpectedRevision,
			OperationID:      in.OperationID,
		},
	)
	if err != nil {
		return MutationResult[ProcedureGenerationAccepted]{}, err
	}

	d := result.Data

	return MutationResult[ProcedureGenerationAccepted]{
		Data: ProcedureGenerationAccepted{
			ProcedureID: d.ProcedureID,
			NextRoute:   d.NextRoute,
			AcceptedAt:  d.AcceptedAt.Format(time.RFC3339Nano),
		},
		Receipt: receipt(result.Receipt),
	}, nil
}
