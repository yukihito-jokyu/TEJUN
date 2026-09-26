package events

import (
	"bytes"
	"encoding/json"
	"reflect"
	"sort"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestAppEventSchema(t *testing.T) {
	valid := AppEvent{
		EventID: "event-1", Name: "project.changed", EmittedAt: "2026-09-26T00:00:00Z",
		AggregateType: "project", AggregateID: "project-1", ChangeSequence: 1,
		StreamKey: "project:project-1", StreamRevision: 0,
		Correlation: Correlation{}, Payload: map[string]any{},
	}

	tests := []struct {
		name    string
		mutate  func(*AppEvent)
		invalid bool
	}{
		{name: "valid"},
		{name: "missing field", mutate: func(event *AppEvent) { event.EventID = "" }, invalid: true},
		{
			name:    "wrong type",
			mutate:  func(event *AppEvent) { event.Payload = map[string]any{"nested": make(chan int)} },
			invalid: true,
		},
		{name: "invalid timestamp", mutate: func(event *AppEvent) { event.EmittedAt = "yesterday" }, invalid: true},
		{name: "invalid sequence", mutate: func(event *AppEvent) { event.ChangeSequence = 0 }, invalid: true},
		{
			name:    "unsafe integer",
			mutate:  func(event *AppEvent) { event.ChangeSequence = 9007199254740992 },
			invalid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := valid
			if tt.mutate != nil {
				tt.mutate(&event)
			}

			err := Validate(event)
			if (err != nil) != tt.invalid {
				t.Fatalf("invalid=%v, want %v", err != nil, tt.invalid)
			}
		})
	}

	for _, tt := range []struct {
		name string
		json string
	}{
		{name: "unknown field", json: `{"eventId":"e","name":"n","emittedAt":"2026-09-26T00:00:00Z","aggregateType":"a","aggregateId":"i","changeSequence":1,"streamKey":"a:i","streamRevision":0,"correlation":{},"payload":{},"extra":1}`},
		{name: "wrong correlation type", json: `{"eventId":"e","name":"n","emittedAt":"2026-09-26T00:00:00Z","aggregateType":"a","aggregateId":"i","changeSequence":1,"streamKey":"a:i","streamRevision":0,"correlation":{"projectId":1},"payload":{}}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			instance, err := jsonschema.UnmarshalJSON(bytes.NewBufferString(tt.json))
			if err != nil {
				t.Fatal(err)
			}

			if err := appEventSchema.Validate(instance); err == nil {
				t.Fatal("expected schema rejection")
			}
		})
	}
}

func TestAppEventRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		want AppEvent
	}{
		{
			name: "project update",
			want: AppEvent{
				EventID:        "event-1",
				Name:           "project.updated",
				EmittedAt:      "2026-09-26T00:00:00Z",
				AggregateType:  "project",
				AggregateID:    "project-1",
				ChangeSequence: 1,
				StreamKey:      "project:project-1",
				StreamRevision: 2,
				Correlation:    Correlation{ProjectID: "project-1", JobID: "job-1"},
				Payload:        map[string]any{"status": "preparing"},
			},
		},
		{
			name: "empty correlation",
			want: AppEvent{
				EventID:        "event-2",
				Name:           "startup.updated",
				EmittedAt:      "2026-09-26T00:00:01Z",
				AggregateType:  "startup",
				AggregateID:    "startup-1",
				ChangeSequence: 2,
				StreamKey:      "startup:startup-1",
				StreamRevision: 0,
				Correlation:    Correlation{},
				Payload:        map[string]any{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := json.Marshal(tt.want)
			if err != nil {
				t.Fatal(err)
			}

			var got AppEvent
			if err := json.Unmarshal(b, &got); err != nil {
				t.Fatal(err)
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %#v, want %#v", got, tt.want)
			}

			var fields map[string]json.RawMessage
			if err := json.Unmarshal(b, &fields); err != nil {
				t.Fatal(err)
			}

			gotFields := make([]string, 0, len(fields))
			for field := range fields {
				gotFields = append(gotFields, field)
			}

			sort.Strings(gotFields)

			wantFields := []string{
				"aggregateId",
				"aggregateType",
				"changeSequence",
				"correlation",
				"emittedAt",
				"eventId",
				"name",
				"payload",
				"streamKey",
				"streamRevision",
			}
			if !reflect.DeepEqual(gotFields, wantFields) {
				t.Fatalf("fields=%v, want %v", gotFields, wantFields)
			}
		})
	}
}
