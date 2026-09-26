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

func (client) RequestPermission(context.Context, sdk.RequestPermissionRequest) (sdk.RequestPermissionResponse, error) {
	return sdk.RequestPermissionResponse{}, errUnsupportedClientOperation
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

		role, chunk := "", ""
		if update := notification.Update.AgentMessageChunk; update != nil && update.Content.Text != nil {
			role, chunk = "agent", update.Content.Text.Text
		}

		if update := notification.Update.AgentThoughtChunk; update != nil && update.Content.Text != nil {
			role, chunk = "thought", update.Content.Text.Text
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
