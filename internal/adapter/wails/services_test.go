package wails

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/yukihito-jokyu/TEJUN/internal/domain/shared"
)

func TestEmptyCollectionIsNotNull(t *testing.T) {
	tests := []struct {
		name  string
		items []string
		want  string
	}{
		{name: "nil", items: nil, want: `[]`},
		{name: "empty", items: []string{}, want: `[]`},
		{name: "populated", items: []string{"item"}, want: `["item"]`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(NonNilSlice(tt.items))
			if err != nil {
				t.Fatal(err)
			}

			if string(got) != tt.want {
				t.Fatalf("got %s, want %s", got, tt.want)
			}
		})
	}
}

func TestBindingValidation(t *testing.T) {
	startup := &StartupService{}
	control := &AgentControlService{}
	tests := []struct {
		name string
		call func() error
	}{
		{name: "check requires connection", call: func() error {
			_, err := startup.CheckAuthentication(context.Background(), CheckAuthenticationInput{})
			return err
		}},
		{name: "complete requires probe", call: func() error {
			_, err := startup.CompleteInitialSetup(
				context.Background(),
				CompleteSetupInput{Connection: validConnection(), OperationID: "op"},
			)

			return err
		}},
		{name: "authenticate requires one target", call: func() error {
			_, err := control.AuthenticateAgent(
				context.Background(),
				AuthenticateAgentInput{ProbeID: "p", ConnectionID: "c", AuthMethodID: "m", OperationID: "op"},
			)

			return err
		}},
		{name: "logout requires operation", call: func() error {
			_, err := control.LogoutAgent(context.Background(), LogoutAgentInput{ConnectionID: "c"})
			return err
		}},
		{name: "elicitation rejects action", call: func() error {
			_, err := control.RespondToElicitation(
				context.Background(),
				RespondToElicitationInput{ElicitationRequestID: "e", Action: "unknown", OperationID: "op"},
			)

			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var appErr *shared.Error
			if err := tt.call(); !errors.As(err, &appErr) || appErr.Code != "validation_failed" {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func validConnection() AgentConnectionInput {
	return AgentConnectionInput{DisplayName: "Agent", Command: "agent", Transport: "stdio"}
}
