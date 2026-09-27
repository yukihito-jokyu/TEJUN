package acp

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/yukihito-jokyu/TEJUN/internal/application"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/shared"
)

func TestExecutionJobRejectsUnattributedCheckCompletion(t *testing.T) {
	m := NewManager(nil, time.Now, func() string { return "unused" })
	defer func() { _ = m.Close() }()

	for _, job := range []application.ClaimedExecutionJob{
		{Kind: "checks", SessionID: "missing"},
		{Kind: "unknown", SessionID: "missing"},
	} {
		t.Run(job.Kind, func(t *testing.T) {
			_, err := m.ExecuteExecutionJob(context.Background(), job)

			var appErr *shared.Error
			if !errors.As(err, &appErr) || appErr.Code != "invalid_state" {
				t.Fatalf("error = %v, want invalid_state", err)
			}
		})
	}
}

func TestParseCheckResults(t *testing.T) {
	targets := []application.ExecutionCheckTarget{{CheckID: "a"}, {CheckID: "b"}}

	for _, tc := range []struct {
		name, answer string
		valid        bool
	}{
		{"valid", `{"results":[{"checkId":"a","status":"completed","evidence":"ok"},{"checkId":"b","status":"failed","evidence":"error"}]}`, true},
		{"fenced", "確認しました。\n```json\n" + `{"results":[{"checkId":"a","status":"completed","evidence":"ok"},{"checkId":"b","status":"failed","evidence":"error"}]}` + "\n```", true},
		{"plain fence", "```\n" + `{"results":[{"checkId":"a","status":"completed","evidence":"ok"},{"checkId":"b","status":"failed","evidence":"error"}]}` + "\n```", true},
		{"missing", `{"results":[{"checkId":"a","status":"completed","evidence":"ok"}]}`, false},
		{"duplicate", `{"results":[{"checkId":"a","status":"completed","evidence":"ok"},{"checkId":"a","status":"failed","evidence":"error"}]}`, false},
		{"unknown", `{"results":[{"checkId":"a","status":"completed","evidence":"ok"},{"checkId":"c","status":"failed","evidence":"error"}]}`, false},
		{"empty evidence", `{"results":[{"checkId":"a","status":"completed","evidence":" "},{"checkId":"b","status":"failed","evidence":"error"}]}`, false},
		{"extra field", `{"results":[{"checkId":"a","status":"completed","evidence":"ok","other":1},{"checkId":"b","status":"failed","evidence":"error"}]}`, false},
		{"trailing", `{"results":[{"checkId":"a","status":"completed","evidence":"ok"},{"checkId":"b","status":"failed","evidence":"error"}]} {}`, false},
		{"prose", `done`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			results, err := parseCheckResults(tc.answer, targets)
			if (err == nil) != tc.valid || (tc.valid && len(results) != len(targets)) {
				t.Fatalf("results = %+v, error = %v", results, err)
			}
		})
	}
}
