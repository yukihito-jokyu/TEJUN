package wails

import (
	"context"
	"time"

	"github.com/yukihito-jokyu/TEJUN/internal/application"
)

func (s *AgentControlService) CancelAgentOperation(
	ctx context.Context,
	input CancelAgentOperationInput,
) (MutationResult[CancellationAccepted], error) {
	if err := requireFields(
		map[string]string{"sessionId": input.SessionID, "operationId": input.OperationID},
	); err != nil {
		return MutationResult[CancellationAccepted]{}, err
	}

	if input.TurnID == "" && input.RunID == "" && input.JobID == "" {
		return MutationResult[CancellationAccepted]{}, validation(map[string]string{"turnId": "取消対象を指定してください"})
	}

	result, err := s.preparation.CancelAgentOperation(
		ctx,
		application.CancelAgentOperationInput{
			SessionID:   input.SessionID,
			TurnID:      input.TurnID,
			RunID:       input.RunID,
			JobID:       input.JobID,
			OperationID: input.OperationID,
		},
	)
	if err != nil {
		return MutationResult[CancellationAccepted]{}, err
	}

	return MutationResult[CancellationAccepted]{
		Data: CancellationAccepted{
			JobID:        result.Data.JobID,
			TargetStatus: result.Data.TargetStatus,
			RequestedAt:  result.Data.RequestedAt.Format(time.RFC3339Nano),
			Mechanism:    result.Data.Mechanism,
		},
		Receipt: receipt(result.Receipt),
	}, nil
}

func (s *AgentControlService) SetAgentSessionConfiguration(
	ctx context.Context,
	input SetAgentSessionConfigurationInput,
) (MutationResult[application.SessionSummary], error) {
	if err := requireFields(
		map[string]string{"sessionId": input.SessionID, "operationId": input.OperationID},
	); err != nil {
		return MutationResult[application.SessionSummary]{}, err
	}

	if input.Change.Kind != "mode" && input.Change.Kind != "config_option" {
		return MutationResult[application.SessionSummary]{}, validation(map[string]string{"change.kind": "不正な変更です"})
	}

	result, err := s.preparation.SetAgentSessionConfiguration(
		ctx,
		application.SetAgentSessionConfigurationInput{
			SessionID:        input.SessionID,
			ExpectedRevision: input.ExpectedRevision,
			OperationID:      input.OperationID,
			Change: application.SessionConfigurationChange{
				Kind:     input.Change.Kind,
				ModeID:   input.Change.ModeID,
				ConfigID: input.Change.ConfigID,
				Value:    input.Change.Value,
			},
		},
	)
	if err != nil {
		return MutationResult[application.SessionSummary]{}, err
	}

	result.Data.ConfigOptions = NonNilSlice(result.Data.ConfigOptions)

	return MutationResult[application.SessionSummary]{Data: result.Data, Receipt: receipt(result.Receipt)}, nil
}
