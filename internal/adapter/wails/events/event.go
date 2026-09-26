package events

import (
	"bytes"
	"embed"
	"encoding/json"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/wailsapp/wails/v3/pkg/application"
)

const Name = "app:event"

type Correlation struct {
	ProjectID   string `json:"projectId,omitempty"`
	SessionID   string `json:"sessionId,omitempty"`
	TurnID      string `json:"turnId,omitempty"`
	ExecutionID string `json:"executionId,omitempty"`
	RunID       string `json:"runId,omitempty"`
	ProcedureID string `json:"procedureId,omitempty"`
	JobID       string `json:"jobId,omitempty"`
}

type AppEvent struct {
	EventID        string         `json:"eventId"`
	Name           string         `json:"name"`
	EmittedAt      string         `json:"emittedAt"`
	AggregateType  string         `json:"aggregateType"`
	AggregateID    string         `json:"aggregateId"`
	ChangeSequence int64          `json:"changeSequence"`
	StreamKey      string         `json:"streamKey"`
	StreamRevision int64          `json:"streamRevision"`
	Correlation    Correlation    `json:"correlation"`
	Payload        map[string]any `json:"payload"`
}

func init() { application.RegisterEvent[AppEvent](Name) }

//go:embed app_event.schema.json
var schemaFiles embed.FS

var appEventSchema = compileAppEventSchema()

func compileAppEventSchema() *jsonschema.Schema {
	content, err := schemaFiles.ReadFile("app_event.schema.json")
	if err != nil {
		panic(err)
	}

	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(content))
	if err != nil {
		panic(err)
	}

	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()

	if err := compiler.AddResource("app_event.schema.json", document); err != nil {
		panic(err)
	}

	schema, err := compiler.Compile("app_event.schema.json")
	if err != nil {
		panic(err)
	}

	return schema
}

func Validate(value AppEvent) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}

	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(encoded))
	if err != nil {
		return err
	}

	return appEventSchema.Validate(instance)
}
