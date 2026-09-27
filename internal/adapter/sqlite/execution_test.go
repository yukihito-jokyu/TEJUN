package sqlite

import (
	"context"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yukihito-jokyu/TEJUN/internal/application"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/execution"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/shared"
)

func executionFixture(t *testing.T) (*ExecutionRepository, time.Time) {
	t.Helper()

	ctx := context.Background()

	projectRepo := projectTestDB(t)
	if _, err := projectRepo.CreateProject(ctx, projectRecord("p", "create:p")); err != nil {
		t.Fatal(err)
	}

	db := projectRepo.db
	if _, err := db.ExecContext(ctx, `INSERT INTO preparation_sessions(session_id, project_id, state, started_at)
VALUES ('s', 'p', 'ready', '2026-09-26T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}

	at := time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)

	for _, statement := range []string{
		`INSERT INTO check_items(check_id, project_id, sequence, title, instruction, expected_result, ai_required, human_required, human_evidence_requirement)
VALUES ('c', 'p', 1, 'Check', 'Do it', 'Done', 1, 1, 'text')`,
		`INSERT INTO executions(execution_id, project_id, session_id, status, revision, started_at)
VALUES ('e', 'p', 's', 'active', 1, '2026-09-26T00:00:00Z')`,
		`INSERT INTO execution_checks(check_id, execution_id, sequence, title, instruction,
expected_result, suggested_command, ai_required, human_required,
ai_status, human_status, human_evidence_requirement)
VALUES ('c', 'e', 1, 'Check', 'Do it', 'Done', '', 1, 1, 'completed', 'pending', 'text')`,
	} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}

	return NewExecutionRepository(db), at
}

func TestExecutionSnapshotAndHumanCheck(t *testing.T) {
	tests := []struct {
		name          string
		withEvidence  bool
		expectFailure bool
	}{
		{name: "evidence required", expectFailure: true},
		{name: "evidence available", withEvidence: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, at := executionFixture(t)

			ctx := context.Background()
			if tt.withEvidence {
				if _, err := repo.db.ExecContext(ctx, `INSERT INTO execution_evidence
(evidence_id, execution_id, check_id, actor, kind, text, created_at)
VALUES ('v', 'e', 'c', 'human', 'text', 'observed', '2026-09-26T00:00:00Z')`); err != nil {
					t.Fatal(err)
				}
			}

			before, err := repo.GetExecution(ctx, "p", 10)
			if err != nil || before.Revision != 1 || len(before.Checks) != 1 || before.CanGenerate {
				t.Fatalf("before=%+v err=%v", before, err)
			}

			result, err := repo.SetHumanCheck(ctx, application.HumanCheckRecord{
				ExecutionID: "e", CheckID: "c", Checked: true, ExpectedRevision: 1,
				OperationID: "op", At: at, Event: application.OutboxEvent{
					ID: "event", Name: "execution.updated", EmittedAt: at,
					AggregateType: "execution", AggregateID: "e",
				},
			})

			if tt.expectFailure {
				var business *shared.Error
				if !errors.As(err, &business) || business.Code != "evidence_required" {
					t.Fatalf("err=%v", err)
				}

				return
			}

			if err != nil || !result.Data.CanGenerate || result.Data.ExecutionRevision != 2 {
				t.Fatalf("result=%+v err=%v", result, err)
			}

			again, err := repo.SetHumanCheck(ctx, application.HumanCheckRecord{
				ExecutionID: "e", CheckID: "c", Checked: true, ExpectedRevision: 1,
				OperationID: "op", At: at,
			})
			if err != nil || again.Data.ExecutionRevision != result.Data.ExecutionRevision {
				t.Fatalf("dedup=%+v err=%v", again, err)
			}

			after, err := repo.GetExecution(ctx, "p", 10)
			if err != nil || after.Revision != 2 || !after.CanGenerate {
				t.Fatalf("after=%+v err=%v", after, err)
			}
		})
	}
}

func TestPermissionClaim(t *testing.T) {
	tests := []struct {
		name, option string
		generation   int64
		wantClaim    bool
	}{
		{name: "valid", option: "allow", generation: 3, wantClaim: true},
		{name: "unknown option", option: "other", generation: 3},
		{name: "stale process", option: "allow", generation: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, at := executionFixture(t)

			ctx := context.Background()
			if err := repo.RegisterPermission(ctx, application.IncomingPermission{
				ID:                "permission",
				ExecutionID:       "e",
				SessionID:         "s",
				ToolCallID:        "tool",
				Title:             "Run",
				AgentSessionID:    "agent-session",
				RPCRequestIDJSON:  `"rpc-1"`,
				ProcessGeneration: 3,
				Options:           []execution.PermissionOption{{ID: "allow", Name: "Allow", Kind: "allow_once"}},
				RequestedAt:       at,
			}); err != nil {
				t.Fatal(err)
			}

			_, claimed, err := repo.ClaimPermission(ctx, application.PermissionClaim{
				SessionID: "s", PermissionRequestID: "permission", OptionID: tt.option,
				ProcessGeneration: tt.generation, OperationID: "op", At: at,
			})
			if claimed != tt.wantClaim || (err != nil) == tt.wantClaim {
				t.Fatalf("claimed=%t err=%v", claimed, err)
			}

			if !tt.wantClaim {
				return
			}

			_, claimed, err = repo.ClaimPermission(ctx, application.PermissionClaim{
				SessionID: "s", PermissionRequestID: "permission", OptionID: "allow",
				ProcessGeneration: 3, OperationID: "different", At: at,
			})
			if claimed || err == nil {
				t.Fatalf("second claim claimed=%t err=%v", claimed, err)
			}
		})
	}
}

func TestEvidenceSagaAndProcedure(t *testing.T) {
	repo, at := executionFixture(t)
	ctx := context.Background()
	input := application.AttachHumanEvidenceInput{
		ExecutionID: "e", CheckID: "c", Kind: "text", Text: "observed",
		ExpectedRevision: 1, OperationID: "evidence-op",
	}

	attached, err := repo.AttachTextEvidence(ctx, application.EvidenceRecord{
		Input: input, EvidenceID: "v", At: at,
		Event: application.OutboxEvent{
			ID: "evidence-event", Name: "execution.updated",
			EmittedAt: at, AggregateType: "execution", AggregateID: "e",
		},
	})
	if err != nil || attached.Data.Evidence.Actor != "human" || attached.Data.ExecutionRevision != 2 {
		t.Fatalf("attach=%+v err=%v", attached, err)
	}

	_, err = repo.GenerateProcedureDraft(ctx, application.ProcedureDraftRecord{
		ExecutionID: "e", OperationID: "procedure-op", ProcedureID: "procedure",
		ExpectedRevision: 2, At: at,
	})

	var business *shared.Error
	if !errors.As(err, &business) || business.Code != "invalid_state" {
		t.Fatalf("premature generation err=%v", err)
	}

	_, err = repo.SetHumanCheck(ctx, application.HumanCheckRecord{
		ExecutionID: "e", CheckID: "c", Checked: true,
		ExpectedRevision: 2, OperationID: "check-op", At: at,
		Event: application.OutboxEvent{
			ID: "check-event", Name: "execution.updated",
			EmittedAt: at, AggregateType: "execution", AggregateID: "e",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	generated, err := repo.GenerateProcedureDraft(ctx, application.ProcedureDraftRecord{
		ExecutionID: "e", OperationID: "procedure-op", ProcedureID: "procedure",
		ExpectedRevision: 3, At: at,
		Event: application.OutboxEvent{
			ID: "procedure-event", Name: "procedure.updated",
			EmittedAt: at, AggregateType: "procedure", AggregateID: "procedure",
		},
	})
	if err != nil || generated.Data.ProcedureID != "procedure" {
		t.Fatalf("generated=%+v err=%v", generated, err)
	}

	_, err = repo.SetHumanCheck(ctx, application.HumanCheckRecord{
		ExecutionID: "e", CheckID: "c", Checked: false,
		ExpectedRevision: 4, OperationID: "uncheck-op", At: at,
	})
	if !errors.As(err, &business) || business.Code != "invalid_state" {
		t.Fatalf("uncheck after generation err=%v", err)
	}
}

func TestImageEvidencePendingIsNotAvailable(t *testing.T) {
	repo, at := executionFixture(t)

	ctx := context.Background()
	if _, err := repo.db.ExecContext(ctx, `UPDATE execution_checks SET human_evidence_requirement = 'image'
WHERE check_id = 'c'`); err != nil {
		t.Fatal(err)
	}

	input := application.AttachHumanEvidenceInput{
		ExecutionID: "e", CheckID: "c", Kind: "image", SourcePath: "/tmp/image.png",
		ExpectedRevision: 1, OperationID: "image-op",
	}

	staged := application.StagedEvidence{
		Hash: requestHash("blob"), StagingName: "staging/blob",
		MIME: "image/png", Size: 12,
	}
	if err := repo.BeginImageEvidence(ctx, application.EvidenceRecord{
		Input: input, EvidenceID: "image", Staged: &staged, At: at,
	}); err != nil {
		t.Fatal(err)
	}

	before, err := repo.GetExecution(ctx, "p", 1)
	if err != nil || before.Revision != 1 || before.Checks[0].Evidence[0].Status != "pending" {
		t.Fatalf("before=%+v err=%v", before, err)
	}

	_, found, pending, err := repo.FindEvidenceOperation(ctx, input)
	if err != nil || found || !pending {
		t.Fatalf("found=%t pending=%t err=%v", found, pending, err)
	}

	completed, err := repo.CompleteImageEvidence(ctx, "image", true, at)
	if err != nil || completed.Data.Evidence.EvidenceID != "image" || completed.Data.ExecutionRevision != 2 {
		t.Fatalf("completed=%+v err=%v", completed, err)
	}

	replayed, found, pending, err := repo.FindEvidenceOperation(ctx, input)
	if err != nil || !found || pending || replayed.Data.Evidence.EvidenceID != "image" {
		t.Fatalf("replayed=%+v found=%t pending=%t err=%v", replayed, found, pending, err)
	}
}

func TestInterruptedPermissionRecovery(t *testing.T) {
	for _, tt := range []struct {
		name    string
		recover bool
	}{
		{name: "recovery", recover: true}, {name: "idempotent", recover: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo, at := executionFixture(t)

			ctx := context.Background()
			if err := repo.RegisterPermission(ctx, application.IncomingPermission{
				ID: "permission", ExecutionID: "e", SessionID: "s", ProcessGeneration: 1,
				Options: []execution.PermissionOption{{ID: "allow", Name: "Allow"}}, RequestedAt: at,
			}); err != nil {
				t.Fatal(err)
			}

			if _, claimed, err := repo.ClaimPermission(ctx, application.PermissionClaim{
				SessionID: "s", PermissionRequestID: "permission", OptionID: "allow",
				ProcessGeneration: 1, OperationID: "op", At: at,
			}); err != nil || !claimed {
				t.Fatalf("claim=%t err=%v", claimed, err)
			}

			before, err := repo.GetExecution(ctx, "p", 1)
			if err != nil {
				t.Fatal(err)
			}

			if err := repo.FailInterruptedPermissions(ctx, at); err != nil {
				t.Fatal(err)
			}

			if !tt.recover {
				if err := repo.FailInterruptedPermissions(ctx, at); err != nil {
					t.Fatal(err)
				}
			}

			after, err := repo.GetExecution(ctx, "p", 1)
			if err != nil || after.ChangeSequence != before.ChangeSequence+1 {
				t.Fatalf("after=%+v err=%v", after, err)
			}

			var state string
			if err := repo.db.QueryRowContext(ctx, `SELECT state FROM execution_permissions WHERE permission_request_id = 'permission'`).
				Scan(&state); err != nil ||
				state != "failed" {
				t.Fatalf("state=%s err=%v", state, err)
			}

			var receipts int
			if err := repo.db.QueryRowContext(ctx, `SELECT count(*) FROM operation_receipts WHERE scope = 'permission_response'`).
				Scan(&receipts); err != nil ||
				receipts != 0 {
				t.Fatalf("receipts=%d err=%v", receipts, err)
			}
		})
	}
}

func TestInterruptedJobPublishesChange(t *testing.T) {
	for _, tt := range []struct {
		name    string
		running bool
	}{
		{name: "running", running: true}, {name: "none"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo, at := executionFixture(t)

			ctx := context.Background()
			if tt.running {
				if _, err := repo.db.ExecContext(ctx, `INSERT INTO execution_agent_jobs
(job_id, execution_id, session_id, kind, state, prompt_text, accepted_at)
VALUES ('j', 'e', 's', 'message', 'running', 'hello', '2026-09-26T00:00:00Z')`); err != nil {
					t.Fatal(err)
				}
			}

			before, err := repo.GetExecution(ctx, "p", 1)
			if err != nil {
				t.Fatal(err)
			}

			if err := repo.FailInterruptedExecutionJobs(ctx, at); err != nil {
				t.Fatal(err)
			}

			after, err := repo.GetExecution(ctx, "p", 1)
			if err != nil {
				t.Fatal(err)
			}

			want := before.ChangeSequence
			if tt.running {
				want++
			}

			if after.ChangeSequence != want {
				t.Fatalf("sequence=%d want=%d", after.ChangeSequence, want)
			}

			var events int
			if err := repo.db.QueryRowContext(ctx, `SELECT count(*) FROM event_outbox WHERE name = 'execution.updated'`).
				Scan(&events); err != nil {
				t.Fatal(err)
			}

			if tt.running && events != 1 || !tt.running && events != 0 {
				t.Fatalf("events=%d", events)
			}
		})
	}
}

func TestDuplicateImageReplacesMissingStaging(t *testing.T) {
	for _, tt := range []struct {
		name        string
		firstStatus string
	}{
		{name: "pending", firstStatus: "pending"}, {name: "corrupt", firstStatus: "corrupt"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo, at := executionFixture(t)

			ctx := context.Background()
			dir := t.TempDir()
			path := filepath.Join(dir, "image.png")

			imageFile, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}

			if err := png.Encode(imageFile, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
				t.Fatal(err)
			}

			if err := imageFile.Close(); err != nil {
				t.Fatal(err)
			}

			store, err := OpenEvidenceFiles(filepath.Join(dir, "cas"))
			if err != nil {
				t.Fatal(err)
			}

			t.Cleanup(func() { _ = store.Close() })

			if _, err := repo.db.ExecContext(
				ctx,
				`UPDATE execution_checks SET human_evidence_requirement = 'image' WHERE check_id = 'c'`,
			); err != nil {
				t.Fatal(err)
			}

			var replacement application.StagedEvidence

			for i, name := range []string{"missing", "replacement"} {
				staged, err := store.Stage(ctx, path)
				if err != nil {
					t.Fatal(err)
				}

				if i == 0 {
					if err := store.root.Remove(staged.StagingName); err != nil {
						t.Fatal(err)
					}
				} else {
					replacement = staged
				}

				record := application.EvidenceRecord{
					Input: application.AttachHumanEvidenceInput{
						ExecutionID:      "e",
						CheckID:          "c",
						Kind:             "image",
						SourcePath:       path,
						ExpectedRevision: 1,
						OperationID:      name,
					},
					EvidenceID: name,
					Staged:     &staged,
					At:         at,
				}
				if err := repo.BeginImageEvidence(ctx, record); err != nil {
					t.Fatal(err)
				}

				if i == 0 && tt.firstStatus == "corrupt" {
					if _, err := repo.db.ExecContext(ctx, `UPDATE evidence_blobs SET status = 'corrupt'`); err != nil {
						t.Fatal(err)
					}
				}
			}

			pending, err := repo.PendingImageEvidence(ctx)
			if err != nil || len(pending) != 2 {
				t.Fatalf("pending=%+v err=%v", pending, err)
			}

			if pending[0].Staged.StagingName == pending[1].Staged.StagingName {
				t.Fatal("distinct evidence shared staging name")
			}

			for _, item := range pending {
				if item.EvidenceID == "replacement" && item.Staged.StagingName != replacement.StagingName {
					t.Fatalf("staging=%s", item.Staged.StagingName)
				}
			}

			evidence := application.NewExecutionEvidence(
				repo,
				store,
				func() time.Time { return at },
				func() string { return "new" },
			)
			if err := evidence.Reconcile(ctx); err != nil {
				t.Fatal(err)
			}

			var count int
			if err := repo.db.QueryRowContext(ctx, `SELECT count(*) FROM operation_receipts WHERE scope = 'attach_evidence'`).
				Scan(&count); err != nil ||
				count != 2 {
				t.Fatalf("count=%d err=%v", count, err)
			}

			if err := repo.db.QueryRowContext(ctx, `SELECT count(*) FROM evidence_records r
JOIN check_evidence c ON c.evidence_id = r.evidence_id
JOIN evidence_blobs b ON b.hash = r.blob_hash
WHERE b.status = 'available' AND b.relative_path = ?`, blobPath(replacement.Hash)).Scan(&count); err != nil || count != 2 {
				t.Fatalf("canonical evidence count=%d err=%v", count, err)
			}
		})
	}
}

func TestMissingImageReconcileEndsOperation(t *testing.T) {
	for _, name := range []string{"missing", "corrupt"} {
		t.Run(name, func(t *testing.T) {
			repo, at := executionFixture(t)
			ctx := context.Background()
			dir := t.TempDir()
			path := filepath.Join(dir, "image.png")

			file, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}

			if err := png.Encode(file, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
				t.Fatal(err)
			}

			if err := file.Close(); err != nil {
				t.Fatal(err)
			}

			store, err := OpenEvidenceFiles(filepath.Join(dir, "cas"))
			if err != nil {
				t.Fatal(err)
			}

			t.Cleanup(func() { _ = store.Close() })

			if _, err := repo.db.ExecContext(
				ctx,
				`UPDATE execution_checks SET human_evidence_requirement = 'image' WHERE check_id = 'c'`,
			); err != nil {
				t.Fatal(err)
			}

			staged, err := store.Stage(ctx, path)
			if err != nil {
				t.Fatal(err)
			}

			input := application.AttachHumanEvidenceInput{
				ExecutionID:      "e",
				CheckID:          "c",
				Kind:             "image",
				SourcePath:       path,
				ExpectedRevision: 1,
				OperationID:      "missing",
			}
			if err := repo.BeginImageEvidence(
				ctx,
				application.EvidenceRecord{Input: input, EvidenceID: "missing", Staged: &staged, At: at},
			); err != nil {
				t.Fatal(err)
			}

			if err := store.root.Remove(staged.StagingName); err != nil {
				t.Fatal(err)
			}

			if name == "corrupt" {
				if _, err := repo.db.ExecContext(ctx, `UPDATE evidence_blobs SET status = 'corrupt'`); err != nil {
					t.Fatal(err)
				}
			}

			evidence := application.NewExecutionEvidence(
				repo,
				store,
				func() time.Time { return at },
				func() string { return "new" },
			)
			if err := evidence.Reconcile(ctx); err != nil {
				t.Fatal(err)
			}

			var status, scope string
			if err := repo.db.QueryRowContext(ctx, `SELECT status FROM evidence_blobs WHERE hash = ?`, staged.Hash).
				Scan(&status); err != nil {
				t.Fatal(err)
			}

			if err := repo.db.QueryRowContext(ctx, `SELECT scope FROM operation_receipts WHERE operation_id = ?`, input.OperationID).
				Scan(&scope); err != nil {
				t.Fatal(err)
			}

			if status != "corrupt" || scope != "evidence_corrupt" {
				t.Fatalf("status=%s scope=%s", status, scope)
			}

			if _, err := evidence.Attach(ctx, input); err == nil || !strings.Contains(err.Error(), "evidence_corrupt") {
				t.Fatalf("retry err=%v", err)
			}
		})
	}
}

type failingEvidenceCommit struct {
	*EvidenceFiles
	err error
}

func (s *failingEvidenceCommit) Commit(ctx context.Context, staged application.StagedEvidence) error {
	if s.err != nil {
		return s.err
	}

	return s.EvidenceFiles.Commit(ctx, staged)
}

func TestImageReconcileCommitFailure(t *testing.T) {
	for _, tt := range []struct {
		name       string
		commitErr  error
		wantStatus string
		wantErr    bool
	}{
		{name: "temporary error stays pending", commitErr: errors.New("disk full"), wantStatus: "pending", wantErr: true},
		{name: "missing staging becomes corrupt", commitErr: os.ErrNotExist, wantStatus: "corrupt"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo, at := executionFixture(t)
			ctx := context.Background()
			dir := t.TempDir()
			path := filepath.Join(dir, "image.png")

			file, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}

			if err := png.Encode(file, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
				t.Fatal(err)
			}

			if err := file.Close(); err != nil {
				t.Fatal(err)
			}

			store, err := OpenEvidenceFiles(filepath.Join(dir, "cas"))
			if err != nil {
				t.Fatal(err)
			}

			t.Cleanup(func() { _ = store.Close() })

			if _, err := repo.db.ExecContext(
				ctx,
				`UPDATE execution_checks SET human_evidence_requirement = 'image' WHERE check_id = 'c'`,
			); err != nil {
				t.Fatal(err)
			}

			staged, err := store.Stage(ctx, path)
			if err != nil {
				t.Fatal(err)
			}

			input := application.AttachHumanEvidenceInput{
				ExecutionID:      "e",
				CheckID:          "c",
				Kind:             "image",
				SourcePath:       path,
				ExpectedRevision: 1,
				OperationID:      "op",
			}
			if err := repo.BeginImageEvidence(
				ctx,
				application.EvidenceRecord{Input: input, EvidenceID: "image", Staged: &staged, At: at},
			); err != nil {
				t.Fatal(err)
			}

			if !tt.wantErr {
				if err := store.root.Remove(staged.StagingName); err != nil {
					t.Fatal(err)
				}
			}

			evidence := application.NewExecutionEvidence(
				repo,
				&failingEvidenceCommit{EvidenceFiles: store, err: tt.commitErr},
				func() time.Time { return at },
				func() string { return "new" },
			)

			err = evidence.Reconcile(ctx)
			if (err != nil) != tt.wantErr {
				t.Fatalf("reconcile err=%v wantErr=%v", err, tt.wantErr)
			}

			var status string
			if err := repo.db.QueryRowContext(ctx, `SELECT status FROM evidence_blobs WHERE hash = ?`, staged.Hash).
				Scan(&status); err != nil {
				t.Fatal(err)
			}

			if status != tt.wantStatus {
				t.Fatalf("status=%q want=%q", status, tt.wantStatus)
			}

			if tt.wantErr {
				evidence = application.NewExecutionEvidence(
					repo,
					store,
					func() time.Time { return at },
					func() string { return "new" },
				)
				if err := evidence.Reconcile(ctx); err != nil {
					t.Fatal(err)
				}

				if err := repo.db.QueryRowContext(ctx, `SELECT status FROM evidence_blobs WHERE hash = ?`, staged.Hash).
					Scan(&status); err != nil ||
					status != "available" {
					t.Fatalf("retry status=%q err=%v", status, err)
				}
			}
		})
	}
}

func TestExecutionCheckUpdatesStayInExecution(t *testing.T) {
	repo, at := executionFixture(t)

	ctx := context.Background()
	for _, statement := range []string{
		`INSERT INTO executions(execution_id, project_id, session_id, status, revision, started_at)
VALUES ('e2', 'p', 's', 'active', 1, '2026-09-26T00:00:00Z')`,
		`INSERT INTO execution_checks(check_id, execution_id, sequence, title, instruction,
expected_result, suggested_command, ai_required, human_required,
ai_status, human_status, human_evidence_requirement)
VALUES ('c', 'e2', 1, 'Check', 'Do it', 'Done', '', 1, 1, 'pending', 'pending', 'text')`,
		`INSERT INTO execution_evidence(evidence_id, execution_id, check_id, actor, kind, text, created_at)
VALUES ('v', 'e', 'c', 'human', 'text', 'observed', '2026-09-26T00:00:00Z')`,
	} {
		if _, err := repo.db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}

	latest, err := repo.GetExecution(ctx, "p", 1)
	if err != nil || latest.ExecutionID != "e2" {
		t.Fatalf("latest=%+v err=%v", latest, err)
	}

	_, err = repo.SetHumanCheck(ctx, application.HumanCheckRecord{
		ExecutionID: "e", CheckID: "c", Checked: true, ExpectedRevision: 1,
		OperationID: "isolated", At: at,
	})
	if err != nil {
		t.Fatal(err)
	}

	var status string
	if err := repo.db.QueryRowContext(ctx, `SELECT human_status FROM execution_checks WHERE execution_id = 'e2' AND check_id = 'c'`).
		Scan(&status); err != nil ||
		status != "pending" {
		t.Fatalf("other execution human status=%s err=%v", status, err)
	}
}

func TestImageCommitBeforeDatabaseFailureReconciles(t *testing.T) {
	repo, at := executionFixture(t)
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "image.png")

	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := png.Encode(file, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}

	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := OpenEvidenceFiles(filepath.Join(t.TempDir(), "cas"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = store.Close() })

	if _, err := repo.db.ExecContext(
		ctx,
		`UPDATE execution_checks SET human_evidence_requirement = 'image' WHERE execution_id = 'e'`,
	); err != nil {
		t.Fatal(err)
	}

	staged, err := store.Stage(ctx, path)
	if err != nil {
		t.Fatal(err)
	}

	input := application.AttachHumanEvidenceInput{
		ExecutionID:      "e",
		CheckID:          "c",
		Kind:             "image",
		SourcePath:       path,
		ExpectedRevision: 1,
		OperationID:      "op",
	}
	if err := repo.BeginImageEvidence(
		ctx,
		application.EvidenceRecord{Input: input, EvidenceID: "v", Staged: &staged, At: at},
	); err != nil {
		t.Fatal(err)
	}

	if err := store.Commit(ctx, staged); err != nil {
		t.Fatal(err)
	}

	if _, err := repo.db.ExecContext(
		ctx,
		`CREATE TRIGGER reject_receipt BEFORE INSERT ON operation_receipts WHEN NEW.scope = 'attach_evidence' BEGIN SELECT RAISE(ABORT, 'injected failure'); END`,
	); err != nil {
		t.Fatal(err)
	}

	if _, err := repo.CompleteImageEvidence(ctx, "v", true, at); err == nil {
		t.Fatal("expected database failure")
	}

	if _, err := repo.db.ExecContext(ctx, `DROP TRIGGER reject_receipt`); err != nil {
		t.Fatal(err)
	}

	evidence := application.NewExecutionEvidence(
		repo,
		store,
		func() time.Time { return at },
		func() string { return "new" },
	)
	if err := evidence.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}

	result, found, pending, err := repo.FindEvidenceOperation(ctx, input)
	if err != nil || !found || pending || result.Data.Evidence.EvidenceID != "v" {
		t.Fatalf("result=%+v found=%t pending=%t err=%v", result, found, pending, err)
	}
}

func TestExecutionJobResultAndCancellation(t *testing.T) {
	tests := []struct {
		name         string
		output       application.ExecutionJobResult
		cancel       bool
		wantState    string
		wantCheck    string
		wantEvidence int
	}{
		{name: "generic prompt success", wantState: "failed", wantCheck: "failed"},
		{
			name: "individual success",
			output: application.ExecutionJobResult{
				Checks: []application.ExecutionCheckResult{{CheckID: "c", Status: "completed", Evidence: "observed"}},
			},
			wantState:    "completed",
			wantCheck:    "completed",
			wantEvidence: 1,
		},
		{
			name: "individual failure",
			output: application.ExecutionJobResult{
				Checks: []application.ExecutionCheckResult{{CheckID: "c", Status: "failed", Evidence: "mismatch"}},
			},
			wantState:    "failed",
			wantCheck:    "failed",
			wantEvidence: 1,
		},
		{
			name: "unknown result",
			output: application.ExecutionJobResult{
				Checks: []application.ExecutionCheckResult{
					{CheckID: "other", Status: "completed", Evidence: "observed"},
				},
			},
			wantState: "failed",
			wantCheck: "failed",
		},
		{
			name:      "cancel confirmed",
			output:    application.ExecutionJobResult{Cancelled: true},
			cancel:    true,
			wantState: "cancelled",
			wantCheck: "pending",
		},
		{
			name: "completion beats cancel",
			output: application.ExecutionJobResult{
				Checks: []application.ExecutionCheckResult{{CheckID: "c", Status: "completed", Evidence: "observed"}},
			},
			cancel:       true,
			wantState:    "completed",
			wantCheck:    "completed",
			wantEvidence: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, at := executionFixture(t)

			ctx := context.Background()
			if _, err := repo.db.ExecContext(
				ctx,
				`UPDATE execution_checks SET ai_status = 'pending' WHERE check_id = 'c'`,
			); err != nil {
				t.Fatal(err)
			}

			_, err := repo.AcceptExecutionJob(ctx, application.ExecutionJobRecord{
				ExecutionID:      "e",
				JobID:            "j",
				RunID:            "r",
				Kind:             "checks",
				ExpectedRevision: 1,
				CheckIDs:         []string{"c"},
				OperationID:      "run-op",
				AcceptedAt:       at,
				Event: application.OutboxEvent{
					ID:            "run-event",
					Name:          "execution.updated",
					AggregateType: "execution",
					AggregateID:   "e",
					EmittedAt:     at,
				},
			})
			if err != nil {
				t.Fatal(err)
			}

			job, err := repo.ClaimExecutionJob(ctx)
			if err != nil || job == nil || len(job.Checks) != 1 || job.Checks[0].Instruction != "Do it" {
				t.Fatalf("job=%+v err=%v", job, err)
			}

			if tt.cancel {
				running, err := repo.CancelExecutionJob(ctx, "j", "cancel-op", at)
				if err != nil || !running {
					t.Fatalf("cancel running=%t err=%v", running, err)
				}
			}

			if err := repo.CompleteExecutionJob(ctx, *job, tt.output, nil, at); err != nil {
				t.Fatal(err)
			}

			var state, check string

			var evidence int

			if err := repo.db.QueryRowContext(ctx, `SELECT state FROM execution_runs WHERE run_id = 'r'`).
				Scan(&state); err != nil {
				t.Fatal(err)
			}

			if err := repo.db.QueryRowContext(ctx, `SELECT ai_status FROM execution_checks WHERE check_id = 'c' AND execution_id = 'e'`).
				Scan(&check); err != nil {
				t.Fatal(err)
			}

			if err := repo.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM execution_evidence WHERE actor = 'ai'`).
				Scan(&evidence); err != nil {
				t.Fatal(err)
			}

			if state != tt.wantState || check != tt.wantCheck || evidence != tt.wantEvidence {
				t.Fatalf("state=%s check=%s evidence=%d", state, check, evidence)
			}
		})
	}
}

func TestQueuedExecutionJobCancellation(t *testing.T) {
	repo, at := executionFixture(t)
	ctx := context.Background()

	if _, err := repo.db.ExecContext(
		ctx,
		`UPDATE execution_checks SET ai_status = 'pending' WHERE check_id = 'c'`,
	); err != nil {
		t.Fatal(err)
	}

	_, err := repo.AcceptExecutionJob(ctx, application.ExecutionJobRecord{
		ExecutionID:      "e",
		JobID:            "j",
		RunID:            "r",
		Kind:             "checks",
		ExpectedRevision: 1,
		CheckIDs:         []string{"c"},
		OperationID:      "run-op",
		AcceptedAt:       at,
		Event: application.OutboxEvent{
			ID:            "run-event",
			Name:          "execution.updated",
			AggregateType: "execution",
			AggregateID:   "e",
			EmittedAt:     at,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, operationID := range []string{"cancel-op", "cancel-op"} {
		running, err := repo.CancelExecutionJob(ctx, "j", operationID, at)
		if err != nil || running {
			t.Fatalf("running=%t err=%v", running, err)
		}
	}

	job, err := repo.ClaimExecutionJob(ctx)
	if err != nil || job != nil {
		t.Fatalf("job=%+v err=%v", job, err)
	}

	var state, check string
	if err := repo.db.QueryRowContext(ctx, `SELECT state FROM execution_runs WHERE run_id = 'r'`).
		Scan(&state); err != nil {
		t.Fatal(err)
	}

	if err := repo.db.QueryRowContext(ctx, `SELECT ai_status FROM execution_checks WHERE check_id = 'c' AND execution_id = 'e'`).
		Scan(&check); err != nil {
		t.Fatal(err)
	}

	if state != "cancelled" || check != "pending" {
		t.Fatalf("state=%s check=%s", state, check)
	}
}
