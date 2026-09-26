package events

import "github.com/wailsapp/wails/v3/pkg/application"

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
