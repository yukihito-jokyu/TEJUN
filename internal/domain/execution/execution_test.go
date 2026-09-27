package execution

import "testing"

func TestSnapshotReadiness(t *testing.T) {
	tests := []struct {
		name string
		view Snapshot
		want bool
	}{
		{name: "empty", view: Snapshot{Status: "active"}, want: false},
		{name: "missing evidence", view: Snapshot{Status: "active", Checks: []Check{
			{
				ID:                       "c",
				AIRequired:               true,
				HumanRequired:            true,
				AIStatus:                 "completed",
				HumanStatus:              "completed",
				HumanEvidenceRequirement: "image",
			},
		}}, want: false},
		{name: "pending evidence", view: Snapshot{Status: "active", Checks: []Check{
			{
				ID:                       "c",
				AIRequired:               true,
				HumanRequired:            true,
				AIStatus:                 "completed",
				HumanStatus:              "completed",
				HumanEvidenceRequirement: "image",
				Evidence:                 []Evidence{{Actor: "human", Kind: "image", Status: "pending"}},
			},
		}}, want: false},
		{
			name: "optional sides",
			view: Snapshot{Status: "active", Checks: []Check{{ID: "c", HumanEvidenceRequirement: "none"}}},
			want: true,
		},
		{name: "ready", view: Snapshot{Status: "active", Checks: []Check{
			{
				ID:                       "c",
				AIRequired:               true,
				HumanRequired:            true,
				AIStatus:                 "completed",
				HumanStatus:              "completed",
				HumanEvidenceRequirement: "image",
				Evidence:                 []Evidence{{Actor: "human", Kind: "image", Status: "available"}},
			},
		}}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := tt.view.Readiness()
			if got != tt.want {
				t.Fatalf("Readiness() = %t, want %t", got, tt.want)
			}
		})
	}
}
