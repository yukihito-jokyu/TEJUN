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

		prompt = "次の1件のcheckを実行し、結果と観測した証跡をJSONオブジェクトのみで返してください。" +
			`形式: {"results":[{"checkId":"...","status":"completed|failed","evidence":"具体的な証跡"}]}` +
			"。対象check: " + string(payload)
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
	s.activityTools = nil
	s.activityLog = nil
	s.activityPhase = "thinking"
	s.activityMessageID = ""
	m.mu.Unlock()

	result := application.ExecutionJobResult{Logs: []string{}}

	var runErr error

	for attempt := 0; attempt < 3; attempt++ {
		response, err := s.connection.Prompt(ctx, sdk.PromptRequest{
			SessionId: s.agentSessionID,
			Prompt:    []sdk.ContentBlock{{Text: &sdk.ContentBlockText{Type: "text", Text: prompt}}},
		})

		m.mu.Lock()

		messages := append([]application.ConversationItem(nil), s.messages...)
		tools := append([]application.PreparationToolActivity(nil), s.activityTools...)
		s.messages = nil
		s.activityTools = nil
		s.activityMessageID = ""
		m.mu.Unlock()

		for _, tool := range tools {
			if strings.TrimSpace(tool.Title) != "" {
				result.Logs = append(result.Logs, tool.Title+" ("+tool.Status+")")
			}
		}

		if err != nil {
			runErr = acpError(err, "prompt")
			break
		}

		if response.StopReason == sdk.StopReasonCancelled {
			result.Cancelled = true
			break
		}

		if response.StopReason != sdk.StopReasonEndTurn {
			runErr = errors.New("agent turn did not complete")
			break
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
			result.Message = answer.String()
			break
		}

		result.Checks, runErr = parseCheckResults(answer.String(), job.Checks)
		if runErr == nil {
			break
		}

		result.Logs = append(result.Logs,
			fmt.Sprintf("AIの回答（形式エラー、%d回目）: %s", attempt+1, answer.String()))
		if attempt == 2 {
			break
		}

		prompt = "直前の回答はJSONとして読み取れませんでした（" + runErr.Error() +
			"）。コマンドを再実行せず、直前に観測した結果だけを次の形式のJSONオブジェクトで再出力してください。" +
			`{"results":[{"checkId":"` + job.Checks[0].CheckID +
			`","status":"completed|failed","evidence":"具体的な証跡"}]}` +
			"。Markdownのコードフェンスや説明文は付けないでください。"

		m.mu.Lock()
		s.activityLog = append(s.activityLog, application.ConversationItem{
			MessageID: m.newID(), TurnID: s.pendingTurnID, Role: "system", Status: "completed",
			CreatedAt: m.now(), Content: []application.ContentPart{{Type: "text", Text: "JSON形式を確認し、AIに再出力を依頼しています"}},
		})
		s.activityPhase = "thinking"
		m.mu.Unlock()
	}

	m.mu.Lock()
	close(s.pendingTurnDone)
	s.pendingTurnDone = nil
	s.pendingTurnID = ""
	s.pendingExecutionJobID = ""
	s.pendingExecutionID = ""
	s.pendingRunID = ""
	s.messages = nil
	m.mu.Unlock()

	return result, runErr
}

func (m *Manager) ExecutionActivity(sessionID string) *application.PreparationActivity {
	m.mu.Lock()
	defer m.mu.Unlock()

	s := m.sessions[sessionID]
	if s == nil || s.pendingExecutionJobID == "" {
		return nil
	}

	items := make([]application.ConversationItem, len(s.activityLog))
	for i, item := range s.activityLog {
		items[i] = item
		items[i].Content = append([]application.ContentPart{}, item.Content...)
	}

	return &application.PreparationActivity{
		TurnID: s.pendingTurnID,
		Phase:  s.activityPhase,
		Items:  items,
	}
}

func parseCheckResults(
	answer string,
	targets []application.ExecutionCheckTarget,
) ([]application.ExecutionCheckResult, error) {
	if _, fenced, ok := strings.Cut(answer, "```"); ok {
		if body, _, closed := strings.Cut(fenced, "```"); closed {
			body = strings.TrimSpace(body)

			body = strings.TrimPrefix(body, "json")
			if strings.HasPrefix(strings.TrimSpace(body), "{") {
				answer = body
			}
		}
	}

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
