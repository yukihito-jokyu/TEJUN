package wails

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/yukihito-jokyu/TEJUN/internal/domain/shared"
)

func TestPreparationBindingMethods(t *testing.T) {
	tests := []struct {
		service any
		methods []string
	}{
		{
			service: &PreparationService{},
			methods: []string{
				"GetPreparation",
				"SendPreparationMessage",
				"SavePreparationBrief",
				"ChangeProjectWorkspace",
				"SaveSessionPermissionPolicy",
				"SaveCheckPlan",
				"StartExecution",
			},
		},
		{
			service: &AgentControlService{},
			methods: []string{"CancelAgentOperation", "RespondToElicitation", "SetAgentSessionConfiguration"},
		},
	}
	for _, tt := range tests {
		for _, name := range tt.methods {
			t.Run(name, func(t *testing.T) {
				if _, ok := reflect.TypeOf(tt.service).MethodByName(name); !ok {
					t.Fatalf("Binding %s missing", name)
				}
			})
		}
	}
}

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
	preparation := &PreparationService{}
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
		{name: "preparation rejects pagination", call: func() error {
			_, err := preparation.GetPreparation(
				context.Background(),
				PreparationViewQuery{ProjectID: "p", ConversationLimit: 101},
			)

			return err
		}},
		{name: "message rejects empty content", call: func() error {
			_, err := preparation.SendPreparationMessage(
				context.Background(),
				SendPreparationMessageInput{ProjectID: "p", OperationID: "op"},
			)

			return err
		}},
		{name: "workspace requires confirmation", call: func() error {
			_, err := preparation.ChangeProjectWorkspace(
				context.Background(),
				ChangeProjectWorkspaceInput{ProjectID: "p", NewWorkspacePath: "/tmp", OperationID: "op"},
			)

			return err
		}},
		{name: "policy rejects mode", call: func() error {
			_, err := preparation.SaveSessionPermissionPolicy(
				context.Background(),
				SaveSessionPermissionPolicyInput{SessionID: "s", OperationID: "op", Mode: "unknown"},
			)

			return err
		}},
		{name: "cancel requires target", call: func() error {
			_, err := control.CancelAgentOperation(
				context.Background(),
				CancelAgentOperationInput{SessionID: "s", OperationID: "op"},
			)

			return err
		}},
		{name: "config rejects change", call: func() error {
			_, err := control.SetAgentSessionConfiguration(
				context.Background(),
				SetAgentSessionConfigurationInput{SessionID: "s", OperationID: "op"},
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
