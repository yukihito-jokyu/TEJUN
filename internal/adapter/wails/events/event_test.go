package events

import (
	"encoding/json"
	"reflect"
	"sort"
	"testing"
)

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
