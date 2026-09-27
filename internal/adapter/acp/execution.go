package acp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	sdk "github.com/coder/acp-go-sdk"
	"github.com/yukihito-jokyu/TEJUN/internal/application"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/shared"
)

var _ application.ExecutionJobExecutor = (*Manager)(nil)

func (m *Manager) ExecutionSessionConnected(sessionID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	s := m.sessions[sessionID]

	return s != nil && s.agentSessionID != ""
}

func (m *Manager) ConnectExecutionSession(ctx context.Context,
	job application.ClaimedExecutionJob, sessionID string,
) (string, error) {
	completed, err := m.ExecutePreparation(ctx, application.ClaimedPreparationJob{
		Kind: "connect", JobID: job.JobID + ":reconnect", SessionID: sessionID,
		WorkspacePath: job.WorkspacePath, Connection: job.Connection,
	})

	return completed.AgentSessionID, err
}

func (m *Manager) DisconnectExecutionSession(sessionID string) error {
	return m.closeSession(sessionID)
}

func (m *Manager) ExecuteExecutionJob(
	ctx context.Context,
	job application.ClaimedExecutionJob,
) (application.ExecutionJobResult, error) {
	if job.Kind != "message" && job.Kind != "checks" {
		return application.ExecutionJobResult{}, &shared.Error{Code: "invalid_state", Message: "実行種別が不正です"}
	}

	prompt := job.PromptText
	if job.Kind == "checks" {
		if len(job.Checks) == 0 {
			return application.ExecutionJobResult{}, &shared.Error{Code: "invalid_state", Message: "対象checkがありません"}
		}

		payload, err := json.Marshal(job.Checks)
		if err != nil {
			return application.ExecutionJobResult{}, err
		}

		prompt = "次の各checkを実行し、結果をJSONオブジェクトのみで返してください。" +
			`形式: {"results":[{"checkId":"...","status":"completed|failed","evidence":"具体的な証跡"}]}` +
			"。すべてのcheckを一度ずつ含めてください。対象check: " + string(payload)
	}

	m.mu.Lock()

	s := m.sessions[job.SessionID]
	if s == nil || s.agentSessionID == "" || s.pendingTurnID != "" {
		m.mu.Unlock()

		return application.ExecutionJobResult{}, &shared.Error{
			Code: "session_disconnected", Message: "Agent sessionを使用できません", Retryable: true,
		}
	}

	s.pendingTurnID = job.TurnID
	if s.pendingTurnID == "" {
		s.pendingTurnID = job.RunID
	}

	s.pendingExecutionJobID = job.JobID
	s.pendingExecutionID = job.ExecutionID
	s.pendingRunID = job.RunID
	s.pendingTurnDone = make(chan struct{})
	s.messages = nil
	m.mu.Unlock()

	response, err := s.connection.Prompt(ctx, sdk.PromptRequest{
		SessionId: s.agentSessionID,
		Prompt:    []sdk.ContentBlock{{Text: &sdk.ContentBlockText{Type: "text", Text: prompt}}},
	})

	m.mu.Lock()

	messages := append([]application.ConversationItem(nil), s.messages...)
	close(s.pendingTurnDone)
	s.pendingTurnDone = nil
	s.pendingTurnID = ""
	s.pendingExecutionJobID = ""
	s.pendingExecutionID = ""
	s.pendingRunID = ""
	s.messages = nil
	m.mu.Unlock()

	if err != nil {
		return application.ExecutionJobResult{}, acpError(err, "prompt")
	}

	if response.StopReason == sdk.StopReasonCancelled {
		return application.ExecutionJobResult{Cancelled: true}, nil
	}

	if response.StopReason != sdk.StopReasonEndTurn {
		return application.ExecutionJobResult{}, errors.New("agent turn did not complete")
	}

	var answer strings.Builder

	for _, message := range messages {
		if message.Role == "agent" {
			for _, content := range message.Content {
				if content.Type == "text" {
					answer.WriteString(content.Text)
				}
			}
		}
	}

	if job.Kind == "message" {
		return application.ExecutionJobResult{Message: answer.String()}, nil
	}

	checks, err := parseCheckResults(answer.String(), job.Checks)

	return application.ExecutionJobResult{Checks: checks}, err
}

func parseCheckResults(
	answer string,
	targets []application.ExecutionCheckTarget,
) ([]application.ExecutionCheckResult, error) {
	var envelope struct {
		Results []application.ExecutionCheckResult `json:"results"`
	}

	decoder := json.NewDecoder(strings.NewReader(answer))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&envelope); err != nil {
		return nil, fmt.Errorf("invalid check results: %w", err)
	}

	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return nil, errors.New("invalid check results: trailing data")
	}

	if len(envelope.Results) != len(targets) {
		return nil, errors.New("invalid check results: count mismatch")
	}

	wanted := make(map[string]bool, len(targets))
	for _, target := range targets {
		wanted[target.CheckID] = true
	}

	for _, result := range envelope.Results {
		if !wanted[result.CheckID] ||
			(result.Status != "completed" && result.Status != "failed") ||
			strings.TrimSpace(result.Evidence) == "" {
			return nil, errors.New("invalid check results: unknown check, status or empty evidence")
		}

		delete(wanted, result.CheckID)
	}

	if len(wanted) != 0 {
		return nil, errors.New("invalid check results: missing check")
	}

	return envelope.Results, nil
}

func (m *Manager) CancelExecutionOperation(ctx context.Context, jobID string) error {
	m.mu.Lock()

	var s *session

	for _, current := range m.sessions {
		if current.pendingExecutionJobID == jobID {
			s = current
			break
		}
	}
	m.mu.Unlock()

	if s == nil {
		return nil
	}

	if err := s.connection.Cancel(ctx, sdk.CancelNotification{SessionId: s.agentSessionID}); err != nil {
		return acpError(err, "session/cancel")
	}

	return nil
}
