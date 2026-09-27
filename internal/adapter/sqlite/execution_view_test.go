package sqlite

import (
	"context"
	"testing"

	"github.com/yukihito-jokyu/TEJUN/internal/application"
)

func TestExecutionViewSnapshot(t *testing.T) {
	tests := []struct {
		name      string
		cursor    *int64
		wantItems int
	}{
		{name: "first page", wantItems: 1},
		{name: "previous page", cursor: ptrInt64(2), wantItems: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, _ := executionFixture(t)

			ctx := context.Background()
			for _, sqlText := range []string{
				`INSERT INTO execution_turns(turn_id,execution_id,role,text,status,created_at) VALUES ('t1','e','user','one','completed','2026-09-26T00:00:01Z')`,
				`INSERT INTO execution_turns(turn_id,execution_id,role,text,status,created_at) VALUES ('t2','e','agent','two','completed','2026-09-26T00:00:02Z')`,
			} {
				if _, err := repo.db.ExecContext(ctx, sqlText); err != nil {
					t.Fatal(err)
				}
			}

			view, err := repo.GetExecutionView(
				ctx,
				application.ExecutionViewQuery{ProjectID: "p", ConversationCursor: tt.cursor, ConversationLimit: 1},
			)
			if err != nil {
				t.Fatal(err)
			}

			if view.Project.ProjectID != "p" || view.Session.SessionID != "s" || view.Execution.ExecutionID != "e" ||
				len(view.Checks) != 1 ||
				view.Checks[0].SuggestedCommand != "go version" ||
				len(view.Conversation.Items) != tt.wantItems {
				t.Fatalf("view=%+v", view)
			}

			if view.Checks[0].Evidence.AI == nil || view.Checks[0].Evidence.Human == nil ||
				view.PendingPermissions == nil ||
				view.Readiness.BlockingReasons == nil {
				t.Fatalf("nil collection: %+v", view)
			}

			if tt.cursor == nil && (!view.Conversation.HasPrevious || view.Conversation.PreviousCursor == nil) {
				t.Fatalf("pagination=%+v", view.Conversation)
			}
		})
	}
}

func ptrInt64(value int64) *int64 { return &value }

func TestExecutionViewRestoresCheckDetails(t *testing.T) {
	repo, at := executionFixture(t)

	ctx := context.Background()
	for _, statement := range []string{
		`UPDATE execution_checks SET human_evidence_requirement='none' WHERE check_id='c'`,
		`INSERT INTO execution_checks(check_id,execution_id,sequence,title,instruction,expected_result,suggested_command,ai_required,human_required,ai_status,human_status,human_evidence_requirement,ai_failure_summary) VALUES ('c2','e',2,'Second','','','',1,0,'failed','pending','none','agent failed')`,
		`INSERT INTO execution_evidence(evidence_id,execution_id,check_id,actor,kind,text,created_at) VALUES ('text','e','c','ai','text','done','2026-09-26T00:00:00Z')`,
		`INSERT INTO evidence_blobs(hash,status,size,mime,relative_path,ref_count) VALUES (lower(hex(zeroblob(32))),'pending',1,'image/png','pending',1),(lower(hex(randomblob(32))),'corrupt',1,'image/png','corrupt',1)`,
		`INSERT INTO execution_evidence(evidence_id,execution_id,check_id,actor,kind,blob_hash,created_at) SELECT 'pending','e','c','human','image',hash,'2026-09-26T00:00:00Z' FROM evidence_blobs WHERE status='pending'`,
		`INSERT INTO execution_evidence(evidence_id,execution_id,check_id,actor,kind,blob_hash,created_at) SELECT 'corrupt','e','c','human','image',hash,'2026-09-26T00:00:00Z' FROM evidence_blobs WHERE status='corrupt'`,
	} {
		if _, err := repo.db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}

	result, err := repo.SetHumanCheck(
		ctx,
		application.HumanCheckRecord{
			ExecutionID:      "e",
			CheckID:          "c",
			Checked:          true,
			ExpectedRevision: 1,
			OperationID:      "checked",
			At:               at,
			Event: application.OutboxEvent{
				ID:            "checked-event",
				Name:          "execution.updated",
				AggregateType: "execution",
				AggregateID:   "e",
				EmittedAt:     at,
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if result.Data.Check.Human.CheckedAt == nil || !result.Data.Check.Human.CheckedAt.Equal(at) {
		t.Fatalf("mutation=%+v", result.Data.Check.Human)
	}

	view, err := repo.GetExecutionView(ctx, application.ExecutionViewQuery{ProjectID: "p"})
	if err != nil {
		t.Fatal(err)
	}

	if view.Execution.Status != "failed" || len(view.Checks) != 2 || view.Checks[0].Human.CheckedAt == nil ||
		!view.Checks[0].Human.CheckedAt.Equal(at) ||
		len(view.Checks[0].Evidence.Human) != 0 ||
		len(view.Checks[0].Evidence.AI) != 1 ||
		view.Checks[0].Evidence.AI[0].Text != "done" ||
		view.Checks[1].AI.FailureSummary != "agent failed" {
		t.Fatalf("view=%+v", view)
	}
}

func TestProjectIDForExecution(t *testing.T) {
	repo, _ := executionFixture(t)

	tests := []struct {
		id, want  string
		wantError bool
	}{{id: "e", want: "p"}, {id: "missing", wantError: true}}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			got, err := repo.ProjectIDForExecution(context.Background(), tt.id)
			if (err != nil) != tt.wantError || got != tt.want {
				t.Fatalf("got=%q err=%v", got, err)
			}
		})
	}
}
