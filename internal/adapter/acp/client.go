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

func (client) SessionUpdate(context.Context, sdk.SessionNotification) error { return nil }

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

func (c client) UnstableCompleteElicitation(context.Context, sdk.UnstableCompleteElicitationNotification) error {
	return nil
}

func (c client) UnstableCreateElicitation(
	ctx context.Context,
	request sdk.UnstableCreateElicitationRequest,
) (sdk.UnstableCreateElicitationResponse, error) {
	id := c.manager.newID()
	mode, message := elicitationPrompt(request)

	if c.manager.sink == nil {
		return sdk.UnstableCreateElicitationResponse{}, errors.New("elicitation is not configured")
	}

	if err := c.manager.sink.RegisterElicitation(ctx, application.IncomingElicitation{
		ID: id, ConnectionAttemptID: c.attemptID, ProcessGeneration: c.generation,
		Mode: mode, Message: message, RequestedAt: c.manager.now(),
	}); err != nil {
		return sdk.UnstableCreateElicitationResponse{}, err
	}

	response := make(chan application.ElicitationResponse, 1)

	c.manager.mu.Lock()
	c.manager.elicitations[id] = response
	c.manager.mu.Unlock()

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

	return "", ""
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
