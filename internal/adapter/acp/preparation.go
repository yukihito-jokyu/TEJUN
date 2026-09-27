package acp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	sdk "github.com/coder/acp-go-sdk"
	"github.com/yukihito-jokyu/TEJUN/internal/application"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/shared"
)

func (m *Manager) ExecutePreparation(
	ctx context.Context,
	job application.ClaimedPreparationJob,
) (application.PreparationJobCompletion, error) {
	const cancellationGrace = 2 * time.Second

	completed := application.PreparationJobCompletion{JobID: job.JobID, SessionID: job.SessionID, CompletedAt: m.now()}
	switch job.Kind {
	case "connect":
		clean := filepath.Clean(job.WorkspacePath)

		resolved, err := filepath.EvalSymlinks(clean)
		if err != nil || !filepath.IsAbs(clean) || resolved != clean {
			return completed, &shared.Error{Code: "validation_failed", Message: "workspace pathが不正です"}
		}

		root, err := os.OpenRoot(clean)
		if err != nil {
			return completed, &shared.Error{Code: "validation_failed", Message: "workspaceを開けません"}
		}
		defer func() { _ = root.Close() }()

		info, err := root.Stat(".")
		if err != nil || !info.IsDir() {
			return completed, &shared.Error{Code: "validation_failed", Message: "workspace directoryが不正です"}
		}

		if job.PreviousSessionID != "" {
			_ = m.closeSession(job.PreviousSessionID)
		}

		probe, err := m.Check(
			ctx,
			application.CheckAuthenticationInput{Connection: job.Connection, OperationID: job.JobID},
		)
		if err != nil {
			return completed, err
		}

		m.mu.Lock()
		s := m.sessions[probe.Probe.ID]
		m.mu.Unlock()

		if s == nil {
			return completed, errors.New("ACP process exited before session/new")
		}

		response, err := s.connection.NewSession(
			ctx,
			sdk.NewSessionRequest{Cwd: job.WorkspacePath, McpServers: []sdk.McpServer{}},
		)
		if err != nil {
			_ = m.closeSession(probe.Probe.ID)
			return completed, acpError(err, "session/new")
		}

		m.mu.Lock()
		if m.sessions[probe.Probe.ID] != s {
			m.mu.Unlock()
			return completed, errors.New("ACP process ownership changed")
		}

		delete(m.sessions, probe.Probe.ID)
		delete(m.checks, job.JobID)

		s.agentSessionID = response.SessionId
		m.sessions[job.SessionID] = s
		m.mu.Unlock()

		completed.AgentSessionID = string(response.SessionId)
		if response.Modes != nil {
			modes := &application.SessionModes{
				CurrentModeID: string(response.Modes.CurrentModeId),
				Available:     make([]application.SessionMode, 0, len(response.Modes.AvailableModes)),
			}
			for _, mode := range response.Modes.AvailableModes {
				modes.Available = append(
					modes.Available,
					application.SessionMode{
						ModeID:      string(mode.Id),
						Name:        mode.Name,
						Description: value(mode.Description),
					},
				)
			}

			completed.Modes = modes

			m.mu.Lock()
			s.modes = modes
			m.mu.Unlock()
		}

		completed.ConfigOptions = preparationConfigOptions(response.ConfigOptions)

		m.mu.Lock()
		s.configOptions = completed.ConfigOptions
		m.mu.Unlock()

		completed.ProtocolVersion = s.probe.ProtocolVersion
		completed.Success = true

		return completed, nil
	case "turn":
		m.mu.Lock()

		s := m.sessions[job.SessionID]
		m.mu.Unlock()

		if s == nil || s.agentSessionID == "" {
			return completed, &shared.Error{
				Code:      "session_disconnected",
				Message:   "Agent sessionに接続されていません",
				Retryable: true,
			}
		}

		parts := make([]sdk.ContentBlock, 0, len(job.Content)+1)
		for _, part := range job.Content {
			switch part.Type {
			case "text":
				parts = append(parts, sdk.ContentBlock{Text: &sdk.ContentBlockText{Type: "text", Text: part.Text}})
			case "resource_link":
				parts = append(
					parts,
					sdk.ContentBlock{
						ResourceLink: &sdk.ContentBlockResourceLink{
							Type: "resource_link",
							Uri:  part.URL,
							Name: part.Name,
						},
					},
				)
			default:
				return completed, &shared.Error{Code: "validation_failed", Message: "このAgentで未対応のcontentです"}
			}
		}

		parts = append(parts, sdk.ContentBlock{Text: &sdk.ContentBlockText{Type: "text", Text: preparationInstruction}})

		m.mu.Lock()
		s.pendingTurnID = job.TurnID
		s.pendingTurnDone = make(chan struct{})
		s.cancelTimedOut = false
		s.messages = nil
		m.mu.Unlock()

		response, err := s.connection.Prompt(ctx, sdk.PromptRequest{SessionId: s.agentSessionID, Prompt: parts})

		m.mu.Lock()

		completed.Messages = append([]application.ConversationItem(nil), s.messages...)
		timedOut := s.cancelTimedOut
		close(s.pendingTurnDone)
		s.pendingTurnDone = nil
		s.pendingTurnID = ""
		s.messages = nil
		m.mu.Unlock()

		if err != nil {
			if timedOut {
				completed.StopReason = "interrupted"
			}

			return completed, acpError(err, "prompt")
		}

		completed.BriefSuggestion = takeBriefSuggestion(completed.Messages)
		completed.Messages = m.preparationReply(completed.Messages, completed.BriefSuggestion, job.TurnID)
		completed.AgentSessionID = string(s.agentSessionID)
		completed.StopReason = string(response.StopReason)
		completed.Success = true

		return completed, nil
	case "cancel":
		m.mu.Lock()
		s := m.sessions[job.SessionID]

		var done chan struct{}
		if s != nil {
			done = s.pendingTurnDone
		}
		m.mu.Unlock()

		if s == nil || s.agentSessionID == "" {
			return completed, &shared.Error{
				Code:      "session_disconnected",
				Message:   "Agent sessionに接続されていません",
				Retryable: true,
			}
		}

		if err := s.connection.Cancel(ctx, sdk.CancelNotification{SessionId: s.agentSessionID}); err != nil {
			return completed, acpError(err, "session/cancel")
		}

		if done != nil {
			select {
			case <-done:
			case <-time.After(cancellationGrace):
				m.mu.Lock()
				if m.sessions[job.SessionID] == s {
					s.cancelTimedOut = true
				}
				m.mu.Unlock()

				if err := m.closeOwnedSession(job.SessionID, s); err != nil {
					return completed, err
				}

				completed.StopReason = "interrupted"
			case <-ctx.Done():
				return completed, ctx.Err()
			}
		}

		completed.AgentSessionID = string(s.agentSessionID)

		completed.Success = true
		if completed.StopReason == "" {
			completed.StopReason = "cancelled"
		}

		return completed, nil
	default:
		return completed, errors.New("unsupported preparation job")
	}
}

const preparationInstruction = `あなたは手順書の準備担当です。利用者が作りたい手順書について、作業場所のREADME、Taskfile、依存関係、設定などを読み取り、必要なら公式資料も調べ、右側の目的・完了条件・想定利用者・動作チェック案を具体化してください。調査は読み取りに限り、インストール、build、起動、ファイル編集など手順書に書く作業を今ここで実行しないでください。
調査環境で既に導入済みのツールや依存関係を、手順書を使う人にもあると仮定しないでください。初めて構築する人がゼロから再現できるよう、必要なツールとその導入方法、依存関係の導入、設定、起動・検証を調べてください。OSや前提が資料から特定できなければ明示し、推測で埋めないでください。
目的は作る手順書の対象、完了条件は手順全体の最終的な到達状態です。動作チェック案は、その手順書を組み立てるための順序付きの確認項目であり、完了条件を一件のチェックへ言い換えたものではありません。前提ツール、導入、設定、起動など必要な段階を過不足なく分け、各項目のinstructionに実施内容、expectedResultに観測できる結果を書いてください。たとえばtask devが完了条件でも、それだけを唯一の確認項目にしないでください。
まず自分で調べ、分かった内容をまとめて提案してください。一問一答を進めず、資料から解決できない重要な不明点が残る場合だけ、関連する質問をまとめて尋ねてください。通常の説明の末尾には毎回、言語名をtejun-preparationとしたMarkdownのJSONコードブロックを付けてください。キーはpurpose、completionCriteria（文字列配列）、intendedUsers、checkItems（全項目の配列）、question（必要な質問）です。checkItemsの各要素はtitle、instruction、expectedResult、suggestedCommandを持ちます。変更のないキーは省略し、既存の案を変更する場合はcheckItemsを全件返してください。`

const preparationFallbackMessage = "AIの返答を準備項目へ反映できませんでした。もう一度依頼してください。"

func takeBriefSuggestion(messages []application.ConversationItem) *application.PreparationBriefSuggestion {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != "agent" || len(messages[i].Content) == 0 {
			continue
		}

		message := &messages[i].Content[0].Text

		start := strings.Index(*message, "```tejun-preparation")
		if start < 0 {
			continue
		}

		bodyStart := start + len("```tejun-preparation")

		lineEnd := strings.IndexByte((*message)[bodyStart:], '\n')
		if lineEnd < 0 {
			continue
		}

		bodyStart += lineEnd + 1

		end := strings.Index((*message)[bodyStart:], "```")
		if end < 0 || end > 16384 {
			continue
		}

		var suggestion application.PreparationBriefSuggestion
		if err := json.Unmarshal(
			[]byte(strings.TrimSpace((*message)[bodyStart:bodyStart+end])),
			&suggestion,
		); err != nil {
			continue
		}

		suggestion.Question = strings.TrimSpace(suggestion.Question)

		*message = strings.TrimSpace((*message)[:start] + (*message)[bodyStart+end+3:])

		if suggestion.Purpose != "" || suggestion.IntendedUsers != "" || len(suggestion.CompletionCriteria) > 0 ||
			len(suggestion.CheckItems) > 0 || suggestion.Question != "" {
			return &suggestion
		}
	}

	return nil
}

func (m *Manager) preparationReply(
	messages []application.ConversationItem,
	suggestion *application.PreparationBriefSuggestion,
	turnID string,
) []application.ConversationItem {
	for i := len(messages) - 1; i >= 0; i-- {
		message := messages[i]
		if message.Role != "agent" || len(message.Content) == 0 {
			continue
		}

		reply := preparationFallbackMessage
		if suggestion != nil {
			reply = strings.TrimSpace(message.Content[0].Text)
			if reply == "" {
				reply = strings.TrimSpace(suggestion.Question)
			}

			if reply == "" {
				reply = "準備内容の案を整理しました。"
			}
		}

		message.Content = []application.ContentPart{{Type: "text", Text: reply}}

		return []application.ConversationItem{message}
	}

	return []application.ConversationItem{{
		MessageID: m.newID(), TurnID: turnID, Role: "agent", Status: "completed", CreatedAt: m.now(),
		Content: []application.ContentPart{{Type: "text", Text: preparationFallbackMessage}},
	}}
}

func (m *Manager) SetConfiguration(
	ctx context.Context,
	target application.SessionConfigurationTarget,
) (application.SessionConfigurationResult, error) {
	m.mu.Lock()
	s := m.sessions[target.SessionID]

	var receiveSequence int64
	if s != nil {
		receiveSequence = s.receiveSequence
	}
	m.mu.Unlock()

	if s == nil || string(s.agentSessionID) != target.AgentSessionID {
		return application.SessionConfigurationResult{}, &shared.Error{
			Code:      "session_disconnected",
			Message:   "Agent sessionに接続されていません",
			Retryable: true,
		}
	}

	result := application.SessionConfigurationResult{}

	switch target.Change.Kind {
	case "mode":
		_, err := s.connection.SetSessionMode(
			ctx,
			sdk.SetSessionModeRequest{SessionId: s.agentSessionID, ModeId: sdk.SessionModeId(target.Change.ModeID)},
		)
		if err != nil {
			return result, acpError(err, "session/set_mode")
		}

		m.mu.Lock()
		if s.receiveSequence == receiveSequence && s.modes != nil {
			copyModes := *s.modes
			copyModes.CurrentModeID = target.Change.ModeID
			s.modes = &copyModes
			result.Modes = &copyModes
		} else {
			result.Modes = s.modes
		}
		m.mu.Unlock()
	case "config_option":
		request := sdk.SetSessionConfigOptionRequest{}

		switch value := target.Change.Value.(type) {
		case string:
			request.ValueId = &sdk.SetSessionConfigOptionValueId{
				SessionId: s.agentSessionID,
				ConfigId:  sdk.SessionConfigId(target.Change.ConfigID),
				Value:     sdk.SessionConfigValueId(value),
			}
		case bool:
			request.Boolean = &sdk.SetSessionConfigOptionBoolean{
				SessionId: s.agentSessionID,
				ConfigId:  sdk.SessionConfigId(target.Change.ConfigID),
				Type:      "boolean",
				Value:     value,
			}
		default:
			return result, &shared.Error{Code: "validation_failed", Message: "設定値が不正です"}
		}

		response, err := s.connection.SetSessionConfigOption(ctx, request)
		if err != nil {
			return result, acpError(err, "session/set_config_option")
		}

		result.ConfigOptions = preparationConfigOptions(response.ConfigOptions)

		m.mu.Lock()
		if s.receiveSequence == receiveSequence {
			s.configOptions = result.ConfigOptions
		} else {
			result.ConfigOptions = s.configOptions
		}
		m.mu.Unlock()
	default:
		return result, &shared.Error{Code: "validation_failed", Message: "変更種別が不正です"}
	}

	return result, nil
}

func preparationConfigOptions(options []sdk.SessionConfigOption) []application.SessionConfigOption {
	items := make([]application.SessionConfigOption, 0, len(options))
	for _, option := range options {
		if option.Select != nil {
			item := option.Select

			converted := application.SessionConfigOption{
				Type:         "select",
				ConfigID:     string(item.Id),
				Name:         item.Name,
				Description:  value(item.Description),
				CurrentValue: string(item.CurrentValue),
				Options:      &application.SessionConfigOptionChoices{},
			}
			if item.Category != nil {
				converted.Category = string(*item.Category)
			}

			if item.Options.Ungrouped != nil {
				converted.Options.Layout = "flat"

				converted.Options.Items = make([]application.SessionConfigChoice, 0, len(*item.Options.Ungrouped))
				for _, choice := range *item.Options.Ungrouped {
					converted.Options.Items = append(
						converted.Options.Items,
						application.SessionConfigChoice{
							Value:       string(choice.Value),
							Name:        choice.Name,
							Description: value(choice.Description),
						},
					)
				}
			}

			if item.Options.Grouped != nil {
				converted.Options.Layout = "grouped"

				converted.Options.Groups = make([]application.SessionConfigGroup, 0, len(*item.Options.Grouped))
				for _, group := range *item.Options.Grouped {
					g := application.SessionConfigGroup{
						GroupID: string(group.Group),
						Name:    group.Name,
						Options: make([]application.SessionConfigChoice, 0, len(group.Options)),
					}
					for _, choice := range group.Options {
						g.Options = append(
							g.Options,
							application.SessionConfigChoice{
								Value:       string(choice.Value),
								Name:        choice.Name,
								Description: value(choice.Description),
							},
						)
					}

					converted.Options.Groups = append(converted.Options.Groups, g)
				}
			}

			items = append(items, converted)
		} else if option.Boolean != nil {
			item := option.Boolean

			converted := application.SessionConfigOption{
				Type:         "boolean",
				ConfigID:     string(item.Id),
				Name:         item.Name,
				Description:  value(item.Description),
				CurrentValue: item.CurrentValue,
			}
			if item.Category != nil {
				converted.Category = string(*item.Category)
			}

			items = append(items, converted)
		}
	}

	return items
}
