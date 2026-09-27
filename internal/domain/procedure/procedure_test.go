package procedure

import (
	"reflect"
	"testing"
)

func TestEvaluate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		document Document
		source   map[string]bool
		want     Integrity
	}{
		{
			name: "valid source evidence",
			document: Document{
				Title: "手順",
				Steps: []Step{
					{ClientKey: "one", Title: "実行", EvidenceRefs: []EvidenceRef{{EvidenceID: "ev-1", Included: true}}},
				},
			},
			source: map[string]bool{"ev-1": true},
			want:   Integrity{Status: "valid", Issues: []Issue{}},
		},
		{
			name: "unrelated evidence",
			document: Document{
				Title: "手順",
				Steps: []Step{
					{ID: "step-1", ClientKey: "one", Title: "実行", EvidenceRefs: []EvidenceRef{{EvidenceID: "other"}}},
				},
			},
			source: map[string]bool{"ev-1": true},
			want: Integrity{Status: "blocked", Issues: []Issue{
				{
					Code:       "invalid_evidence_ref",
					Severity:   "blocking",
					Message:    "invalid_evidence_ref",
					StepID:     "step-1",
					EvidenceID: "other",
				},
			}},
		},
		{
			name:     "missing required content",
			document: Document{Steps: []Step{{ClientKey: "one"}, {ClientKey: "one"}}},
			want: Integrity{Status: "blocked", Issues: []Issue{
				{Code: "title_required", Severity: "blocking", Message: "title_required"},
				{Code: "step_title_required", Severity: "blocking", Message: "step_title_required"},
				{Code: "invalid_client_key", Severity: "blocking", Message: "invalid_client_key"},
				{Code: "step_title_required", Severity: "blocking", Message: "step_title_required"},
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := Evaluate(tt.document, tt.source)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Evaluate() = %#v; want %#v", got, tt.want)
			}
		})
	}
}
