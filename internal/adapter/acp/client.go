package acp

import (
	"context"
	"encoding/json"
	"errors"

	sdk "github.com/coder/acp-go-sdk"
	"github.com/yukihito-jokyu/TEJUN/internal/application"
)

var errUnsupportedClientOperation = errors.New("ACP client operation is not supported")

type client struct {
	manager    *Manager
	attemptID  string
	generation int64
}

func (client) ReadTextFile(context.Context, sdk.ReadTextFileRequest) (sdk.ReadTextFileResponse, error) {
	return sdk.ReadTextFileResponse{}, errUnsupportedClientOperation
}

func (client) WriteTextFile(context.Context, sdk.WriteTextFileRequest) (sdk.WriteTextFileResponse, error) {
	return sdk.WriteTextFileResponse{}, errUnsupportedClientOperation
}

func (c client) RequestPermission(
	ctx context.Context,
	request sdk.RequestPermissionRequest,
) (sdk.RequestPermissionResponse, error) {
	return c.requestPermission(ctx, request)
}

func (c client) SessionUpdate(ctx context.Context, notification sdk.SessionNotification) error {
	var (
		sessionID string
		sequence  int64
		modes     *application.SessionModes
		options   []application.SessionConfigOption
	)

	c.manager.mu.Lock()
	for appSessionID, session := range c.manager.sessions {
		if session.agentSessionID != notification.SessionId || session.probe.ProcessGeneration != c.generation {
			continue
		}

		if notification.Update.CurrentModeUpdate != nil && session.modes != nil {
			copyModes := *session.modes
			copyModes.CurrentModeID = string(notification.Update.CurrentModeUpdate.CurrentModeId)
			session.modes = &copyModes
			modes = &copyModes
		}

		if notification.Update.ConfigOptionUpdate != nil {
			session.configOptions = preparationConfigOptions(notification.Update.ConfigOptionUpdate.ConfigOptions)
			options = session.configOptions
		}

		if modes != nil || options != nil {
			session.receiveSequence++
			sequence = session.receiveSequence
			sessionID = appSessionID
		}

		if session.pendingTurnID == "" {
			break
		}

		if tool := notification.Update.ToolCall; tool != nil {
			session.activityPhase = "tool"
			session.activityMessageID = ""
			messageID := c.manager.newID()

			session.activityTools = append(session.activityTools, application.PreparationToolActivity{
				ID: string(tool.ToolCallId), MessageID: messageID, Title: tool.Title, Status: string(tool.Status),
			})
			session.activityLog = append(session.activityLog, application.ConversationItem{
				MessageID: messageID, TurnID: session.pendingTurnID,
				Role: "system", Status: "streaming", CreatedAt: c.manager.now(),
				Content: []application.ContentPart{{Type: "text", Text: tool.Title}},
			})

			break
		}

		if tool := notification.Update.ToolCallUpdate; tool != nil {
			for i := range session.activityTools {
				if session.activityTools[i].ID != string(tool.ToolCallId) {
					continue
				}

				logIndex := -1

				for j := range session.activityLog {
					if session.activityLog[j].MessageID == session.activityTools[i].MessageID {
						logIndex = j
						break
					}
				}

				if tool.Title != nil {
					session.activityTools[i].Title = *tool.Title
					if logIndex >= 0 {
						session.activityLog[logIndex].Content[0].Text = *tool.Title
					}
				}

				if tool.Status != nil {
					session.activityTools[i].Status = string(*tool.Status)
					if logIndex >= 0 {
						switch *tool.Status {
						case sdk.ToolCallStatusCompleted:
							session.activityLog[logIndex].Status = "completed"
						case sdk.ToolCallStatusFailed:
							session.activityLog[logIndex].Status = "failed"
						}
					}

					if *tool.Status == sdk.ToolCallStatusCompleted || *tool.Status == sdk.ToolCallStatusFailed {
						session.activityPhase = "thinking"
						for _, active := range session.activityTools {
							if active.Status == string(sdk.ToolCallStatusPending) ||
								active.Status == string(sdk.ToolCallStatusInProgress) {
								session.activityPhase = "tool"
								break
							}
						}
					}
				}

				break
			}

			break
		}

		role, chunk := "", ""
		if update := notification.Update.AgentMessageChunk; update != nil && update.Content.Text != nil {
			role, chunk = "agent", update.Content.Text.Text

			session.activityPhase = "responding"

			messageID := ""
			if update.MessageId != nil {
				messageID = *update.MessageId
			}

			last := len(session.activityLog) - 1
			if last >= 0 && session.activityLog[last].Role == "agent" &&
				(messageID == "" || messageID == session.activityMessageID) {
				session.activityLog[last].Content[0].Text += chunk
			} else {
				session.activityLog = append(session.activityLog, application.ConversationItem{
					MessageID: c.manager.newID(), TurnID: session.pendingTurnID,
					Role: "agent", Status: "streaming", CreatedAt: c.manager.now(),
					Content: []application.ContentPart{{Type: "text", Text: chunk}},
				})
			}

			session.activityMessageID = messageID
		}

		if update := notification.Update.AgentThoughtChunk; update != nil && update.Content.Text != nil {
			role, chunk = "thought", update.Content.Text.Text
			session.activityPhase = "thinking"
		}

		if role == "" {
			break
		}

		if len(session.messages) == 0 || session.messages[len(session.messages)-1].Role != role {
			session.messages = append(
				session.messages,
				application.ConversationItem{
					MessageID: c.manager.newID(),
					TurnID:    session.pendingTurnID,
					Role:      role,
					Status:    "completed",
					CreatedAt: c.manager.now(),
					Content:   []application.ContentPart{{Type: "text", Text: chunk}},
				},
			)
		} else {
			session.messages[len(session.messages)-1].Content[0].Text += chunk
		}

		break
	}
	c.manager.mu.Unlock()

	if sessionID != "" && c.manager.sink != nil {
		return c.manager.sink.UpdateSessionConfiguration(ctx, sessionID, sequence, modes, options, c.manager.now())
	}

	return nil
}

func (client) CreateTerminal(context.Context, sdk.CreateTerminalRequest) (sdk.CreateTerminalResponse, error) {
	return sdk.CreateTerminalResponse{}, errUnsupportedClientOperation
}

func (client) KillTerminal(context.Context, sdk.KillTerminalRequest) (sdk.KillTerminalResponse, error) {
	return sdk.KillTerminalResponse{}, errUnsupportedClientOperation
}

func (client) TerminalOutput(context.Context, sdk.TerminalOutputRequest) (sdk.TerminalOutputResponse, error) {
	return sdk.TerminalOutputResponse{}, errUnsupportedClientOperation
}

func (client) ReleaseTerminal(context.Context, sdk.ReleaseTerminalRequest) (sdk.ReleaseTerminalResponse, error) {
	return sdk.ReleaseTerminalResponse{}, errUnsupportedClientOperation
}

func (client) WaitForTerminalExit(
	context.Context,
	sdk.WaitForTerminalExitRequest,
) (sdk.WaitForTerminalExitResponse, error) {
	return sdk.WaitForTerminalExitResponse{}, errUnsupportedClientOperation
}

func (c client) UnstableCompleteElicitation(
	ctx context.Context,
	notice sdk.UnstableCompleteElicitationNotification,
) error {
	if c.manager.sink == nil {
		return errors.New("elicitation is not configured")
	}

	id, err := c.manager.sink.CompleteURLElicitation(
		ctx,
		string(notice.ElicitationId),
		c.attemptID,
		c.generation,
		c.manager.now(),
	)
	if err != nil {
		return err
	}

	c.manager.mu.Lock()
	response := c.manager.elicitations[id]
	delete(c.manager.elicitations, id)
	c.manager.mu.Unlock()

	if response != nil {
		response <- application.ElicitationResponse{ElicitationRequestID: id, Action: "accept", Content: "{}"}
	}

	return nil
}

func (c client) UnstableCreateElicitation(
	ctx context.Context,
	request sdk.UnstableCreateElicitationRequest,
) (sdk.UnstableCreateElicitationResponse, error) {
	id := c.manager.newID()
	mode, message := elicitationPrompt(request)
	sessionID := ""

	c.manager.mu.Lock()
	for appSessionID, session := range c.manager.sessions {
		if session.probe.ProcessGeneration == c.generation && session.agentSessionID != "" {
			sessionID = appSessionID
			break
		}
	}
	c.manager.mu.Unlock()

	incoming := application.IncomingElicitation{
		ID:                  id,
		SessionID:           sessionID,
		ConnectionAttemptID: c.attemptID,
		ProcessGeneration:   c.generation,
		Mode:                mode,
		Message:             message,
		RequestedAt:         c.manager.now(),
	}
	if sessionID != "" {
		incoming.Scope = map[string]any{"type": "session", "sessionId": sessionID}
	} else {
		incoming.Scope = map[string]any{"type": "request", "requestCorrelationId": id}
	}

	if request.Form != nil {
		encoded, _ := json.Marshal(request.Form.RequestedSchema)
		_ = json.Unmarshal(encoded, &incoming.RequestedSchema)
	}

	if request.Url != nil {
		incoming.URL = request.Url.Url
		incoming.ElicitationID = string(request.Url.ElicitationId)
	}

	response := make(chan application.ElicitationResponse, 1)

	c.manager.mu.Lock()
	c.manager.elicitations[id] = response
	c.manager.mu.Unlock()

	if c.manager.sink == nil {
		c.manager.mu.Lock()
		delete(c.manager.elicitations, id)
		c.manager.mu.Unlock()

		return sdk.UnstableCreateElicitationResponse{}, errors.New("elicitation is not configured")
	}

	if err := c.manager.sink.RegisterElicitation(ctx, incoming); err != nil {
		c.manager.mu.Lock()
		delete(c.manager.elicitations, id)
		c.manager.mu.Unlock()

		return sdk.UnstableCreateElicitationResponse{}, err
	}

	select {
	case <-ctx.Done():
		c.manager.mu.Lock()
		delete(c.manager.elicitations, id)
		c.manager.mu.Unlock()

		return sdk.UnstableCreateElicitationResponse{}, ctx.Err()
	case answer := <-response:
		return elicitationResponse(answer), nil
	}
}

func elicitationPrompt(request sdk.UnstableCreateElicitationRequest) (string, string) {
	if request.Form != nil {
		return "form", request.Form.Message
	}

	if request.Url != nil {
		return "url", request.Url.Message
	}

	return "unsupported", ""
}

func elicitationResponse(response application.ElicitationResponse) sdk.UnstableCreateElicitationResponse {
	switch response.Action {
	case "accept":
		content := make(map[string]any)
		if err := json.Unmarshal([]byte(response.Content), &content); err != nil {
			return sdk.UnstableCreateElicitationResponse{Cancel: &sdk.UnstableCreateElicitationCancel{Action: "cancel"}}
		}

		return sdk.UnstableCreateElicitationResponse{
			Accept: &sdk.UnstableCreateElicitationAccept{Action: "accept", Content: content},
		}
	case "decline":
		return sdk.UnstableCreateElicitationResponse{Decline: &sdk.UnstableCreateElicitationDecline{Action: "decline"}}
	default:
		return sdk.UnstableCreateElicitationResponse{Cancel: &sdk.UnstableCreateElicitationCancel{Action: "cancel"}}
	}
}
