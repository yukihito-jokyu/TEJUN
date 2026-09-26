package wails

import (
	"context"
	"strings"

	"github.com/yukihito-jokyu/TEJUN/internal/application"
)

func NewPreparationService(preparation *application.Preparation) *PreparationService {
	return &PreparationService{preparation: preparation}
}

type PreparationViewQuery struct {
	ProjectID          string `json:"projectId"`
	ChatID             string `json:"chatId,omitempty"`
	ConversationCursor *int64 `json:"conversationCursor,omitempty"`
	ConversationLimit  int    `json:"conversationLimit"`
}

type SendPreparationMessageInput struct {
	ProjectID   string                    `json:"projectId"`
	SessionID   *string                   `json:"sessionId,omitempty"`
	Content     []application.ContentPart `json:"content"`
	OperationID string                    `json:"operationId"`
}

type SavePreparationBriefInput struct {
	ProjectID                   string                `json:"projectId"`
	ExpectedPreparationRevision int64                 `json:"expectedPreparationRevision"`
	OperationID                 string                `json:"operationId"`
	Brief                       PreparationBriefInput `json:"brief"`
}

type PreparationBriefInput struct {
	Purpose            string   `json:"purpose"`
	CompletionCriteria []string `json:"completionCriteria"`
	IntendedUsers      string   `json:"intendedUsers"`
}

type ChangeProjectWorkspaceInput struct {
	ProjectID               string `json:"projectId"`
	NewWorkspacePath        string `json:"newWorkspacePath"`
	ExpectedProjectRevision int64  `json:"expectedProjectRevision"`
	ConfirmSessionReset     bool   `json:"confirmSessionReset"`
	OperationID             string `json:"operationId"`
}

type SaveSessionPermissionPolicyInput struct {
	SessionID               string `json:"sessionId"`
	ExpectedSessionRevision int64  `json:"expectedSessionRevision"`
	OperationID             string `json:"operationId"`
	Mode                    string `json:"mode"`
}

type SaveCheckPlanInput struct {
	ProjectID                   string           `json:"projectId"`
	ExpectedPreparationRevision int64            `json:"expectedPreparationRevision"`
	ExpectedPlanRevision        int64            `json:"expectedPlanRevision"`
	OperationID                 string           `json:"operationId"`
	Items                       []CheckItemInput `json:"items"`
}

type CheckItemInput struct {
	CheckID                  *string `json:"checkId,omitempty"`
	ClientKey                string  `json:"clientKey"`
	Title                    string  `json:"title"`
	Instruction              string  `json:"instruction"`
	ExpectedResult           string  `json:"expectedResult"`
	SuggestedCommand         *string `json:"suggestedCommand,omitempty"`
	AIRequired               bool    `json:"aiRequired"`
	HumanRequired            bool    `json:"humanRequired"`
	HumanEvidenceRequirement string  `json:"humanEvidenceRequirement"`
}

type StartExecutionInput struct {
	ProjectID                   string `json:"projectId"`
	ExpectedPreparationRevision int64  `json:"expectedPreparationRevision"`
	ExpectedPlanRevision        int64  `json:"expectedPlanRevision"`
	OperationID                 string `json:"operationId"`
}

func (s *PreparationService) GetPreparation(
	ctx context.Context,
	input PreparationViewQuery,
) (application.PreparationView, error) {
	if err := required("projectId", input.ProjectID); err != nil {
		return application.PreparationView{}, err
	}

	if input.ConversationLimit < 1 || input.ConversationLimit > 100 {
		return application.PreparationView{}, validation(map[string]string{"conversationLimit": "1から100を指定してください"})
	}

	view, err := s.preparation.GetPreparation(
		ctx,
		application.PreparationViewQuery{
			ProjectID:          input.ProjectID,
			ChatID:             input.ChatID,
			ConversationCursor: input.ConversationCursor,
			ConversationLimit:  input.ConversationLimit,
		},
	)
	if err != nil {
		return application.PreparationView{}, storageError(err)
	}

	view.Preparation.CompletionCriteria = NonNilSlice(view.Preparation.CompletionCriteria)
	view.CheckPlan.Items = NonNilSlice(view.CheckPlan.Items)
	view.Conversation.Items = NonNilSlice(view.Conversation.Items)
	view.Chats = NonNilSlice(view.Chats)
	view.Elicitations = NonNilSlice(view.Elicitations)

	view.Readiness.BlockingReasons = NonNilSlice(view.Readiness.BlockingReasons)
	if view.Session != nil {
		view.Session.ConfigOptions = NonNilSlice(view.Session.ConfigOptions)
		if view.Session.Modes != nil {
			view.Session.Modes.Available = NonNilSlice(view.Session.Modes.Available)
		}
	}

	for i := range view.Conversation.Items {
		view.Conversation.Items[i].Content = NonNilSlice(view.Conversation.Items[i].Content)
	}

	return view, nil
}

func (s *PreparationService) SendPreparationMessage(
	ctx context.Context,
	input SendPreparationMessageInput,
) (MutationResult[application.AcceptedTurn], error) {
	if err := requireFields(
		map[string]string{"projectId": input.ProjectID, "operationId": input.OperationID},
	); err != nil {
		return MutationResult[application.AcceptedTurn]{}, err
	}

	if len(input.Content) == 0 {
		return MutationResult[application.AcceptedTurn]{}, validation(map[string]string{"content": "必須です"})
	}

	for _, part := range input.Content {
		switch part.Type {
		case "text":
			if strings.TrimSpace(part.Text) != "" {
				continue
			}
		case "resource_link":
			if strings.TrimSpace(part.URL) != "" && strings.TrimSpace(part.Name) != "" {
				continue
			}
		}

		return MutationResult[application.AcceptedTurn]{}, validation(map[string]string{"content": "content partが不正です"})
	}

	sessionID := ""
	if input.SessionID != nil {
		sessionID = *input.SessionID
	}

	result, err := s.preparation.SendPreparationMessage(
		ctx,
		application.SendPreparationMessageInput{
			ProjectID:   input.ProjectID,
			SessionID:   sessionID,
			Content:     input.Content,
			OperationID: input.OperationID,
		},
	)
	if err != nil {
		return MutationResult[application.AcceptedTurn]{}, err
	}

	return MutationResult[application.AcceptedTurn]{Data: result.Data, Receipt: receipt(result.Receipt)}, nil
}

func (s *PreparationService) SavePreparationBrief(
	ctx context.Context,
	input SavePreparationBriefInput,
) (MutationResult[application.PreparationUpdate], error) {
	if err := requireFields(
		map[string]string{
			"projectId":   input.ProjectID,
			"operationId": input.OperationID,
			"purpose":     input.Brief.Purpose,
		},
	); err != nil {
		return MutationResult[application.PreparationUpdate]{}, err
	}

	brief := application.PreparationBrief{
		Purpose:            input.Brief.Purpose,
		CompletionCriteria: NonNilSlice(input.Brief.CompletionCriteria),
		IntendedUsers:      input.Brief.IntendedUsers,
	}

	result, err := s.preparation.SavePreparationBrief(
		ctx,
		application.SavePreparationBriefInput{
			ProjectID:                   input.ProjectID,
			ExpectedPreparationRevision: input.ExpectedPreparationRevision,
			OperationID:                 input.OperationID,
			Brief:                       brief,
		},
	)
	if err != nil {
		return MutationResult[application.PreparationUpdate]{}, err
	}

	result.Data.Brief.CompletionCriteria = NonNilSlice(result.Data.Brief.CompletionCriteria)
	result.Data.AffectedCheckIDs = NonNilSlice(result.Data.AffectedCheckIDs)
	result.Data.Readiness.BlockingReasons = NonNilSlice(result.Data.Readiness.BlockingReasons)

	return MutationResult[application.PreparationUpdate]{Data: result.Data, Receipt: receipt(result.Receipt)}, nil
}

func (s *PreparationService) ChangeProjectWorkspace(
	ctx context.Context,
	input ChangeProjectWorkspaceInput,
) (MutationResult[application.WorkspaceChangeAccepted], error) {
	if err := requireFields(
		map[string]string{
			"projectId":        input.ProjectID,
			"newWorkspacePath": input.NewWorkspacePath,
			"operationId":      input.OperationID,
		},
	); err != nil {
		return MutationResult[application.WorkspaceChangeAccepted]{}, err
	}

	if !input.ConfirmSessionReset {
		return MutationResult[application.WorkspaceChangeAccepted]{}, validation(
			map[string]string{"confirmSessionReset": "確認が必要です"},
		)
	}

	result, err := s.preparation.ChangeProjectWorkspace(
		ctx,
		application.ChangeProjectWorkspaceInput{
			ProjectID:               input.ProjectID,
			NewWorkspacePath:        input.NewWorkspacePath,
			ExpectedProjectRevision: input.ExpectedProjectRevision,
			ConfirmSessionReset:     input.ConfirmSessionReset,
			OperationID:             input.OperationID,
		},
	)
	if err != nil {
		return MutationResult[application.WorkspaceChangeAccepted]{}, err
	}

	return MutationResult[application.WorkspaceChangeAccepted]{Data: result.Data, Receipt: receipt(result.Receipt)}, nil
}

func (s *PreparationService) SaveSessionPermissionPolicy(
	ctx context.Context,
	input SaveSessionPermissionPolicyInput,
) (MutationResult[application.SessionPermissionPolicy], error) {
	if err := requireFields(
		map[string]string{"sessionId": input.SessionID, "operationId": input.OperationID, "mode": input.Mode},
	); err != nil {
		return MutationResult[application.SessionPermissionPolicy]{}, err
	}

	if input.Mode != "ask_every_time" && input.Mode != "reuse_explicit_always_choice" {
		return MutationResult[application.SessionPermissionPolicy]{}, validation(map[string]string{"mode": "不正なmodeです"})
	}

	result, err := s.preparation.SaveSessionPermissionPolicy(
		ctx,
		application.SaveSessionPermissionPolicyInput{
			SessionID:               input.SessionID,
			ExpectedSessionRevision: input.ExpectedSessionRevision,
			OperationID:             input.OperationID,
			Mode:                    input.Mode,
		},
	)
	if err != nil {
		return MutationResult[application.SessionPermissionPolicy]{}, err
	}

	return MutationResult[application.SessionPermissionPolicy]{Data: result.Data, Receipt: receipt(result.Receipt)}, nil
}

func (s *PreparationService) SaveCheckPlan(
	ctx context.Context,
	input SaveCheckPlanInput,
) (MutationResult[application.SavedCheckPlan], error) {
	if err := requireFields(
		map[string]string{"projectId": input.ProjectID, "operationId": input.OperationID},
	); err != nil {
		return MutationResult[application.SavedCheckPlan]{}, err
	}

	for _, item := range input.Items {
		if strings.TrimSpace(item.Title) == "" || strings.TrimSpace(item.ClientKey) == "" {
			return MutationResult[application.SavedCheckPlan]{}, validation(
				map[string]string{"items": "titleとclientKeyは必須です"},
			)
		}
	}

	items := make([]application.CheckItemInput, 0, len(input.Items))
	for _, item := range input.Items {
		converted := application.CheckItemInput{
			ClientKey:                item.ClientKey,
			Title:                    item.Title,
			Instruction:              item.Instruction,
			ExpectedResult:           item.ExpectedResult,
			AIRequired:               item.AIRequired,
			HumanRequired:            item.HumanRequired,
			HumanEvidenceRequirement: item.HumanEvidenceRequirement,
		}
		if item.CheckID != nil {
			converted.CheckID = *item.CheckID
		}

		if item.SuggestedCommand != nil {
			converted.SuggestedCommand = *item.SuggestedCommand
		}

		items = append(items, converted)
	}

	result, err := s.preparation.SaveCheckPlan(
		ctx,
		application.SaveCheckPlanInput{
			ProjectID:                   input.ProjectID,
			ExpectedPreparationRevision: input.ExpectedPreparationRevision,
			ExpectedPlanRevision:        input.ExpectedPlanRevision,
			OperationID:                 input.OperationID,
			Items:                       items,
		},
	)
	if err != nil {
		return MutationResult[application.SavedCheckPlan]{}, err
	}

	result.Data.Items = NonNilSlice(result.Data.Items)
	result.Data.AssignedIDs = NonNilSlice(result.Data.AssignedIDs)
	result.Data.Readiness.BlockingReasons = NonNilSlice(result.Data.Readiness.BlockingReasons)

	return MutationResult[application.SavedCheckPlan]{Data: result.Data, Receipt: receipt(result.Receipt)}, nil
}

func (s *PreparationService) StartExecution(
	ctx context.Context,
	input StartExecutionInput,
) (MutationResult[application.ExecutionStarted], error) {
	if err := requireFields(
		map[string]string{"projectId": input.ProjectID, "operationId": input.OperationID},
	); err != nil {
		return MutationResult[application.ExecutionStarted]{}, err
	}

	result, err := s.preparation.StartExecution(
		ctx,
		application.StartExecutionInput{
			ProjectID:                   input.ProjectID,
			ExpectedPreparationRevision: input.ExpectedPreparationRevision,
			ExpectedPlanRevision:        input.ExpectedPlanRevision,
			OperationID:                 input.OperationID,
		},
	)
	if err != nil {
		return MutationResult[application.ExecutionStarted]{}, err
	}

	return MutationResult[application.ExecutionStarted]{Data: result.Data, Receipt: receipt(result.Receipt)}, nil
}
