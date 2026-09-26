package project

import "testing"

func TestProjectTransitions(t *testing.T) {
	tests := []struct {
		name          string
		status        Status
		activeJob     bool
		canRevise     bool
		canArchive    bool
		resumeStage   string
		wantRouteTail string
	}{
		{"preparation", Preparing, false, false, true, "preparation", "/prepare"},
		{"completed", Completed, false, true, true, "completed", "/procedure"},
		{"active", AIRunning, true, false, false, "execution", "/check"},
		{"archived", Archived, false, false, false, "completed", "/procedure"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if (CanRevise(tt.status) == nil) != tt.canRevise {
				t.Fatal("revision rule mismatch")
			}

			if (CanArchive(tt.status, tt.activeJob) == nil) != tt.canArchive {
				t.Fatal("archive rule mismatch")
			}

			if got := ResumeRoute("id", tt.resumeStage); got != "#/projects/id"+tt.wantRouteTail {
				t.Fatalf("route=%s", got)
			}
		})
	}
}
