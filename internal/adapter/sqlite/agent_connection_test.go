package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/yukihito-jokyu/TEJUN/internal/application"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/agentconnection"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/shared"
)

type countingElicitationResponder struct{ calls int }

func (r *countingElicitationResponder) Respond(context.Context, application.ElicitationResponse) error {
	r.calls++

	return nil
}

type fixedAuthMethodResolver struct{}

func (fixedAuthMethodResolver) ResolveAuthMethod(context.Context, string, string) (string, error) {
	return "", nil
}

func TestCompleteInitialSetupIsAtomicAndDeduplicated(t *testing.T) {
	ctx := context.Background()

	db, err := Open(ctx, t.TempDir()+"/test.db")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	repository := NewAgentConnectionRepository(db)
	now := time.Date(2026, 9, 26, 1, 2, 3, 0, time.UTC)
	record := application.CompleteSetupRecord{
		OperationID: "operation-1",
		RequestHash: "hash-1",
		Connection: agentconnection.Connection{
			ID: "connection-1", DisplayName: "Agent", Command: "agent", Args: []string{"serve"},
			Transport: "stdio", ResolvedExecutablePath: "/usr/local/bin/agent",
			LastVerifiedAt: now, AuthState: agentconnection.AuthNotRequired,
			SchemaArtifactVersion: "1", Revision: 1,
		},
		Receipt: application.MutationReceipt{OperationID: "operation-1", CommittedAt: now},
		Event: application.OutboxEvent{
			ID: "event-1", Name: "agent.connection.changed",
			EmittedAt: now, AggregateType: "agent_connection",
		},
	}

	changed := record
	changed.RequestHash = "hash-2"
	tests := []struct {
		name         string
		record       application.CompleteSetupRecord
		wantConflict bool
	}{
		{name: "first commit", record: record},
		{name: "same operation", record: record},
		{name: "different payload", record: changed, wantConflict: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var appErr *shared.Error

			result, err := repository.CompleteInitialSetup(ctx, tt.record)
			if tt.wantConflict {
				if !errors.As(err, &appErr) || appErr.Code != "operation_id_conflict" {
					t.Fatalf("error=%v, want operation_id_conflict", err)
				}

				return
			}

			if err != nil {
				t.Fatal(err)
			}

			if result.Data.NextRoute != "#/projects" || result.Data.ChangeSequence != 1 {
				t.Fatalf("result=%+v", result.Data)
			}
		})
	}

	var connections, receipts, events int
	if err := db.QueryRow(`SELECT
(SELECT COUNT(*) FROM agent_connections),
(SELECT COUNT(*) FROM operation_receipts),
(SELECT COUNT(*) FROM event_outbox)`).Scan(&connections, &receipts, &events); err != nil {
		t.Fatal(err)
	}

	if connections != 1 || receipts != 1 || events != 1 {
		t.Fatalf("connections=%d receipts=%d events=%d", connections, receipts, events)
	}

	state, err := repository.GetStartupState(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if state.InitialSetupRequired || state.NextRoute != "#/projects" || state.DefaultConnection == nil {
		t.Fatalf("state=%+v", state)
	}

	if state.DefaultConnection.ResolvedExecutablePath != record.Connection.ResolvedExecutablePath {
		t.Fatalf("resolved path=%q", state.DefaultConnection.ResolvedExecutablePath)
	}
}

func TestFailInterruptedAgentJobs(t *testing.T) {
	ctx := context.Background()

	db, err := Open(ctx, t.TempDir()+"/test.db")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec(`INSERT INTO agent_jobs
(job_id, kind, target_id, auth_method_id, method_type, state, accepted_at)
VALUES ('job-1', 'logout', 'connection-1', '', '', 'running', '2026-09-26T00:00:00Z')`)
	if err != nil {
		t.Fatal(err)
	}

	completedAt := time.Date(2026, 9, 26, 1, 2, 3, 0, time.UTC)
	if err := NewAgentConnectionRepository(db).FailInterruptedAgentJobs(ctx, completedAt); err != nil {
		t.Fatal(err)
	}

	var state, gotCompletedAt string
	if err := db.QueryRow(`SELECT state, completed_at FROM agent_jobs WHERE job_id = 'job-1'`).
		Scan(&state, &gotCompletedAt); err != nil {
		t.Fatal(err)
	}

	if state != "failed" || gotCompletedAt != completedAt.Format(time.RFC3339Nano) {
		t.Fatalf("state=%q completed_at=%q", state, gotCompletedAt)
	}
}

func TestFailedElicitationCanBeRetried(t *testing.T) {
	ctx := context.Background()

	db, err := Open(ctx, t.TempDir()+"/test.db")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	repository := NewAgentConnectionRepository(db)
	requestedAt := time.Date(2026, 9, 26, 1, 2, 3, 0, time.UTC)

	if err := repository.RegisterElicitation(ctx, application.IncomingElicitation{
		ID: "elicitation-1", ConnectionAttemptID: "probe-1", ProcessGeneration: 1,
		Mode: "form", Message: "continue?", RequestedAt: requestedAt,
	}); err != nil {
		t.Fatal(err)
	}

	claim := application.ElicitationClaim{
		ElicitationRequestID: "elicitation-1", Action: "decline", OperationID: "operation-1",
		RequestHash: "hash-1", ClaimedAt: requestedAt,
		Receipt: application.MutationReceipt{OperationID: "operation-1", CommittedAt: requestedAt},
	}
	if _, _, err := repository.ClaimElicitation(ctx, claim); err != nil {
		t.Fatal(err)
	}

	if err := repository.CompleteElicitation(ctx, "elicitation-1", "operation-1", false, requestedAt); err != nil {
		t.Fatal(err)
	}

	if _, _, err := repository.ClaimElicitation(ctx, claim); err != nil {
		t.Fatalf("retry failed: %v", err)
	}

	if err := repository.CompleteElicitation(
		ctx,
		"elicitation-1",
		"operation-1",
		true,
		requestedAt.Add(time.Second),
	); err != nil {
		t.Fatalf("retry completion failed: %v", err)
	}
}

func TestRespondToElicitationIsIdempotent(t *testing.T) {
	ctx := context.Background()

	db, err := Open(ctx, t.TempDir()+"/test.db")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	repository := NewAgentConnectionRepository(db)
	requestedAt := time.Date(2026, 9, 26, 1, 2, 3, 0, time.UTC)

	if err := repository.RegisterElicitation(ctx, application.IncomingElicitation{
		ID: "elicitation-1", ConnectionAttemptID: "probe-1", ProcessGeneration: 1,
		Mode: "form", Message: "continue?", RequestedAt: requestedAt,
	}); err != nil {
		t.Fatal(err)
	}

	responder := &countingElicitationResponder{}
	now := requestedAt
	control := application.NewAgentControl(
		repository,
		responder,
		fixedAuthMethodResolver{},
		func() time.Time {
			now = now.Add(time.Second)

			return now
		},
		func() string { return "unused" },
	)
	input := application.RespondToElicitationInput{
		ElicitationRequestID: "elicitation-1", Action: "decline", OperationID: "operation-1",
	}

	first, err := control.RespondToElicitation(ctx, input)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name         string
		input        application.RespondToElicitationInput
		wantConflict bool
	}{
		{name: "same payload returns stored result", input: input},
		{
			name: "different payload conflicts",
			input: application.RespondToElicitationInput{
				ElicitationRequestID: "elicitation-1", Action: "accept", OperationID: "operation-1",
			},
			wantConflict: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := control.RespondToElicitation(ctx, tt.input)
			if tt.wantConflict {
				var appErr *shared.Error
				if !errors.As(err, &appErr) || appErr.Code != "operation_id_conflict" {
					t.Fatalf("error=%v, want operation_id_conflict", err)
				}

				return
			}

			if err != nil {
				t.Fatal(err)
			}

			if result != first {
				t.Fatalf("result=%+v, want %+v", result, first)
			}
		})
	}

	if responder.calls != 1 {
		t.Fatalf("responder calls=%d, want 1", responder.calls)
	}
}

func TestFailInterruptedElicitationsAllowsRetry(t *testing.T) {
	ctx := context.Background()

	db, err := Open(ctx, t.TempDir()+"/test.db")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	repository := NewAgentConnectionRepository(db)
	requestedAt := time.Date(2026, 9, 26, 1, 2, 3, 0, time.UTC)

	if err := repository.RegisterElicitation(ctx, application.IncomingElicitation{
		ID: "elicitation-1", ConnectionAttemptID: "probe-1", ProcessGeneration: 1,
		Mode: "form", Message: "continue?", RequestedAt: requestedAt,
	}); err != nil {
		t.Fatal(err)
	}

	claim := application.ElicitationClaim{
		ElicitationRequestID: "elicitation-1", Action: "decline", OperationID: "operation-1",
		RequestHash: "hash-1", ClaimedAt: requestedAt,
		Receipt: application.MutationReceipt{OperationID: "operation-1", CommittedAt: requestedAt},
	}
	if _, _, err := repository.ClaimElicitation(ctx, claim); err != nil {
		t.Fatal(err)
	}

	if err := repository.FailInterruptedElicitations(ctx, requestedAt.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}

	if _, _, err := repository.ClaimElicitation(ctx, claim); err != nil {
		t.Fatalf("retry after recovery failed: %v", err)
	}
}
