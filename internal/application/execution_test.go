package application

import (
	"testing"

	"github.com/yukihito-jokyu/TEJUN/internal/domain/execution"
)

func TestOptionalAICheckKeepsExecutionStatus(t *testing.T) {
	for _, tc := range []struct {
		name, stored, visible string
	}{
		{"not started", "pending", "not_required"},
		{"failed", "failed", "failed"},
		{"completed", "completed", "completed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			view := ExecutionCheckFromSnapshot(execution.Check{AIStatus: tc.stored})
			if view.AI.Status != tc.visible {
				t.Fatalf("AI status = %q, want %q", view.AI.Status, tc.visible)
			}
		})
	}
}
