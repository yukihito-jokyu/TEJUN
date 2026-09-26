package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/yukihito-jokyu/TEJUN/internal/application"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/shared"
)

func seededPreparation(t *testing.T) (context.Context, *sql.DB, *application.Preparation) {
	t.Helper()

	ctx := context.Background()
	repo := projectTestDB(t)

	db := repo.db
	if _, err := repo.CreateProject(ctx, projectRecord("p", "create:p")); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	at := now.Format(time.RFC3339Nano)

	for _, q := range []string{
		`UPDATE projects SET current_session_id='s' WHERE project_id='p'`,
		`UPDATE preparations SET purpose='Purpose',completion_criteria_json='["Done"]',intended_users='User' WHERE project_id='p'`,
		`INSERT INTO preparation_sessions(session_id,project_id,state,started_at) VALUES('s','p','ready',?)`,
	} {
		args := []any{}
		if q == `INSERT INTO preparation_sessions(session_id,project_id,state,started_at) VALUES('s','p','ready',?)` {
			args = []any{at}
		}

		if _, err := db.ExecContext(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}

	n := 0
	id := func() string { n++; return "id-" + string(rune('a'+n)) }

	return ctx, db, application.NewPreparation(NewPreparationRepository(db), func() time.Time { return now }, id)
}

func TestConnectCompletionPreservesNewerACPConfiguration(t *testing.T) {
	ctx, db, _ := seededPreparation(t)
	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO preparation_jobs(job_id,session_id,kind,state,accepted_at) VALUES('connect','s','connect','running','2026-09-26T12:00:00Z')`,
	); err != nil {
		t.Fatal(err)
	}

	if _, err := db.ExecContext(
		ctx,
		`UPDATE preparation_sessions SET modes_json='{"currentModeId":"new"}',config_options_json='[{"configId":"new"}]',acp_receive_sequence=1 WHERE session_id='s'`,
	); err != nil {
		t.Fatal(err)
	}

	if err := NewPreparationRepository(db).CompletePreparationJob(ctx, application.PreparationJobCompletion{
		JobID: "connect", SessionID: "s", AgentSessionID: "agent", Success: true,
		Modes: &application.SessionModes{CurrentModeID: "old"}, CompletedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	var modes, options string
	if err := db.QueryRowContext(ctx, `SELECT modes_json,config_options_json FROM preparation_sessions WHERE session_id='s'`).
		Scan(&modes, &options); err != nil {
		t.Fatal(err)
	}

	if modes != `{"currentModeId":"new"}` || options != `[{"configId":"new"}]` {
		t.Fatalf("ACP update overwritten: modes=%s options=%s", modes, options)
	}
}

func TestAgentBriefSuggestionRespectsManualRevision(t *testing.T) {
	for _, tc := range []struct {
		name, manual, want string
	}{
		{"applies", "", "Agent purpose"},
		{"manual change wins", "Manual purpose", "Manual purpose"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, db, p := seededPreparation(t)

			accepted, err := p.SendPreparationMessage(ctx, application.SendPreparationMessageInput{
				ProjectID:   "p",
				SessionID:   "s",
				OperationID: "suggestion",
				Content:     []application.ContentPart{{Type: "text", Text: "作りたい"}},
			})
			if err != nil {
				t.Fatal(err)
			}

			if _, err = db.ExecContext(
				ctx,
				`UPDATE preparation_jobs SET state='running' WHERE job_id=?`,
				accepted.Data.JobID,
			); err != nil {
				t.Fatal(err)
			}

			if _, err = db.ExecContext(
				ctx,
				`UPDATE preparation_turns SET state='running' WHERE turn_id=?`,
				accepted.Data.TurnID,
			); err != nil {
				t.Fatal(err)
			}

			if tc.manual != "" {
				if _, err = db.ExecContext(
					ctx,
					`UPDATE preparations SET purpose=?,revision=revision+1 WHERE project_id='p'`,
					tc.manual,
				); err != nil {
					t.Fatal(err)
				}
			}

			err = NewPreparationRepository(db).CompletePreparationJob(ctx, application.PreparationJobCompletion{
				JobID:       accepted.Data.JobID,
				SessionID:   "s",
				Success:     true,
				CompletedAt: time.Now(),
				BriefSuggestion: &application.PreparationBriefSuggestion{
					Purpose: "Agent purpose",
					CheckItems: []application.PreparationCheckSuggestion{
						{Title: "起動する", Instruction: "アプリを起動", ExpectedResult: "起動できる"},
					},
				},
			})
			if err != nil {
				t.Fatal(err)
			}

			var purpose string
			if err = db.QueryRowContext(ctx, `SELECT purpose FROM preparations WHERE project_id='p'`).
				Scan(&purpose); err != nil {
				t.Fatal(err)
			}

			if purpose != tc.want {
				t.Fatalf("purpose=%q, want %q", purpose, tc.want)
			}

			var checkTitle string
			if err = db.QueryRowContext(ctx, `SELECT title FROM check_items WHERE project_id='p'`).
				Scan(&checkTitle); err != nil {
				t.Fatal(err)
			}

			if checkTitle != "起動する" {
				t.Fatalf("check title=%q", checkTitle)
			}
		})
	}
}

func TestPreparationChatHistoryAfterNewSession(t *testing.T) {
	ctx, db, p := seededPreparation(t)
	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO preparation_messages(message_id,session_id,turn_id,role,content_json,status,created_at,sequence) VALUES('old-message','s','old-turn','user','[{"type":"text","text":"古い手順書"}]','completed','2026-09-26T12:00:00Z',1)`,
	); err != nil {
		t.Fatal(err)
	}

	before, err := p.GetPreparation(ctx, application.PreparationViewQuery{ProjectID: "p"})
	if err != nil {
		t.Fatal(err)
	}

	_, err = NewPreparationRepository(db).ChangeProjectWorkspace(ctx, application.ChangeProjectWorkspaceRecord{
		ChangeProjectWorkspaceInput: application.ChangeProjectWorkspaceInput{
			ProjectID:               "p",
			NewWorkspacePath:        before.Project.WorkspacePath,
			ExpectedProjectRevision: before.Project.Revision,
			ConfirmSessionReset:     true,
			OperationID:             "new-chat",
		},
		SessionID:  "new-chat",
		JobID:      "new-connect",
		EventID:    "new-event",
		AcceptedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}

	current, err := p.GetPreparation(ctx, application.PreparationViewQuery{ProjectID: "p"})
	if err != nil {
		t.Fatal(err)
	}

	if current.Session.SessionID != "new-chat" || len(current.Conversation.Items) != 0 || len(current.Chats) != 2 {
		t.Fatalf("current chat=%+v", current)
	}

	archived, err := p.GetPreparation(ctx, application.PreparationViewQuery{ProjectID: "p", ChatID: "s"})
	if err != nil {
		t.Fatal(err)
	}

	if len(archived.Conversation.Items) != 1 || archived.Chats[1].Title != "古い手順書" {
		t.Fatalf("archived chat=%+v", archived)
	}

	if _, err = p.GetPreparation(
		ctx,
		application.PreparationViewQuery{ProjectID: "p", ChatID: "other-project-chat"},
	); err == nil {
		t.Fatal("foreign chat should be rejected")
	}
}

func TestPreparationReadinessBlocksPendingTurn(t *testing.T) {
	ctx, db, p := seededPreparation(t)
	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO check_items(check_id,project_id,sequence,title,instruction,expected_result,ai_required,human_required,human_evidence_requirement) VALUES('check','p',1,'Check','Do','Done',0,1,'none')`,
	); err != nil {
		t.Fatal(err)
	}

	ready, err := p.GetPreparation(ctx, application.PreparationViewQuery{ProjectID: "p"})
	if err != nil || !ready.Readiness.CanStartExecution {
		t.Fatalf("expected ready: %v %+v", err, ready.Readiness)
	}

	if _, err = p.SendPreparationMessage(ctx, application.SendPreparationMessageInput{
		ProjectID:   "p",
		SessionID:   "s",
		OperationID: "turn",
		Content:     []application.ContentPart{{Type: "text", Text: "hello"}},
	}); err != nil {
		t.Fatal(err)
	}

	blocked, err := p.GetPreparation(ctx, application.PreparationViewQuery{ProjectID: "p"})
	if err != nil || blocked.Readiness.CanStartExecution {
		t.Fatalf("pending turn should block execution: %v %+v", err, blocked.Readiness)
	}
}

func TestCreatedProjectOpensPreparation(t *testing.T) {
	repo := projectTestDB(t)

	ctx := context.Background()
	if _, err := repo.CreateProject(ctx, projectRecord("new", "create:new")); err != nil {
		t.Fatal(err)
	}

	view, err := NewPreparationRepository(
		repo.db,
	).GetPreparation(ctx, application.PreparationViewQuery{ProjectID: "new"})
	if err != nil {
		t.Fatal(err)
	}

	if view.Project.CurrentStage != "preparation" || view.CheckPlan.PlanID != "new" || view.Session != nil ||
		view.Readiness.CanStartExecution {
		t.Fatalf("new project preparation = %+v", view)
	}
}

func TestDeleteProjectRemovesPreparationSession(t *testing.T) {
	ctx, db, _ := seededPreparation(t)
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)

	_, err := NewProjectRepository(db).DeleteProject(ctx, application.ProjectDeleteRecord{
		DeleteProjectInput: application.DeleteProjectInput{
			ProjectID:        "p",
			ExpectedRevision: 1,
			OperationID:      "delete:p",
		},
		RequestHash: "delete:p",
		Receipt:     application.MutationReceipt{OperationID: "delete:p", CommittedAt: now},
		Event: application.OutboxEvent{
			ID:            "delete-event",
			Name:          "project.deleted",
			EmittedAt:     now,
			AggregateType: "project",
			AggregateID:   "p",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM preparation_sessions WHERE project_id='p'`).
		Scan(&count); err != nil ||
		count != 0 {
		t.Fatalf("remaining sessions=%d, error=%v", count, err)
	}
}

func TestPreparationMutations(t *testing.T) {
	cases := []struct {
		name string
		run  func(context.Context, *sql.DB, *application.Preparation) error
	}{
		{"cancel target and replay", func(ctx context.Context, db *sql.DB, p *application.Preparation) error {
			accepted, err := p.SendPreparationMessage(
				ctx,
				application.SendPreparationMessageInput{
					ProjectID:   "p",
					SessionID:   "s",
					OperationID: "turn",
					Content:     []application.ContentPart{{Type: "text", Text: "hello"}},
				},
			)
			if err != nil {
				return err
			}

			repo := NewPreparationRepository(db)
			in := application.CancelAgentOperationRecord{
				CancelAgentOperationInput: application.CancelAgentOperationInput{
					SessionID:   "s",
					TurnID:      accepted.Data.TurnID,
					OperationID: "cancel",
				},
				CancellationJobID: "cancel-job",
				EventID:           "cancel-event",
				RequestedAt:       time.Now(),
			}

			first, err := repo.AcceptCancellation(ctx, in)
			if err != nil {
				return err
			}

			again, err := repo.AcceptCancellation(ctx, in)
			if err != nil {
				return err
			}

			if first.Data.JobID != "cancel-job" || again.Data.JobID != first.Data.JobID {
				return errors.New("cancel replay mismatch")
			}

			if first.Data.TargetStatus != "cancelled" || first.Data.Mechanism != "local_cancel" {
				return errors.New("pending turn was not cancelled locally")
			}

			var status string
			if err := db.QueryRowContext(ctx, `SELECT status FROM preparation_messages WHERE turn_id=? AND role='user'`, accepted.Data.TurnID).
				Scan(&status); err != nil ||
				status != "cancelled" {
				return errors.New("pending message was not cancelled")
			}

			if job, err := repo.ClaimPreparationJob(ctx); err != nil || job != nil {
				return errors.New("cancelled turn was claimed")
			}

			in.OperationID = "other"

			in.TurnID = "unknown"
			if _, err := repo.AcceptCancellation(ctx, in); err == nil {
				return errors.New("foreign turn accepted")
			}

			return nil
		}},
		{"configuration claim and revision", func(ctx context.Context, db *sql.DB, _ *application.Preparation) error {
			_, err := db.ExecContext(
				ctx,
				`UPDATE preparation_sessions SET agent_session_id='agent-s',modes_json='{"currentModeId":"safe","available":[{"modeId":"safe","name":"Safe"},{"modeId":"fast","name":"Fast"}]}' WHERE session_id='s'`,
			)
			if err != nil {
				return err
			}

			repo := NewPreparationRepository(db)
			claim := application.SessionConfigurationClaim{
				SetAgentSessionConfigurationInput: application.SetAgentSessionConfigurationInput{
					SessionID:        "s",
					ExpectedRevision: 1,
					OperationID:      "config",
					Change:           application.SessionConfigurationChange{Kind: "mode", ModeID: "fast"},
				},
				EventID:   "config-event",
				ClaimedAt: time.Now(),
			}

			target, _, claimed, err := repo.ClaimSessionConfiguration(ctx, claim)
			if err != nil {
				return err
			}

			if !claimed || target.AgentSessionID != "agent-s" {
				return errors.New("configuration claim mismatch")
			}

			out, err := repo.CompleteSessionConfiguration(
				ctx,
				claim,
				application.SessionConfigurationResult{
					Modes: &application.SessionModes{
						CurrentModeID: "fast",
						Available: []application.SessionMode{
							{ModeID: "safe", Name: "Safe"},
							{ModeID: "fast", Name: "Fast"},
						},
					},
					ConfigOptions: []application.SessionConfigOption{},
				},
			)
			if err != nil {
				return err
			}

			if out.Data.Revision != 2 || out.Data.Modes.CurrentModeID != "fast" {
				return errors.New("configuration result mismatch")
			}

			_, replay, claimed, err := repo.ClaimSessionConfiguration(ctx, claim)
			if err != nil {
				return err
			}

			if claimed || replay.Data.Revision != 2 {
				return errors.New("configuration replay mismatch")
			}

			return nil
		}},
		{
			"newer ACP update survives older response",
			func(ctx context.Context, db *sql.DB, _ *application.Preparation) error {
				_, err := db.ExecContext(
					ctx,
					`UPDATE preparation_sessions SET agent_session_id='agent-s',modes_json='{"currentModeId":"safe","available":[{"modeId":"safe"},{"modeId":"fast"}]}' WHERE session_id='s'`,
				)
				if err != nil {
					return err
				}

				repo := NewPreparationRepository(db)
				claim := application.SessionConfigurationClaim{
					SetAgentSessionConfigurationInput: application.SetAgentSessionConfigurationInput{
						SessionID: "s", ExpectedRevision: 1, OperationID: "config",
						Change: application.SessionConfigurationChange{Kind: "mode", ModeID: "fast"},
					},
					EventID: "config-event", ClaimedAt: time.Now(),
				}

				target, _, claimed, err := repo.ClaimSessionConfiguration(ctx, claim)
				if err != nil || !claimed {
					return errors.New("configuration claim failed")
				}

				claim.ReceiveSequence = target.ReceiveSequence

				newer := &application.SessionModes{
					CurrentModeID: "safe",
					Available:     []application.SessionMode{{ModeID: "safe"}, {ModeID: "fast"}},
				}
				if err := NewAgentConnectionRepository(
					db,
				).UpdateSessionConfiguration(ctx, "s", 1, newer, nil, time.Now()); err != nil {
					return err
				}

				out, err := repo.CompleteSessionConfiguration(ctx, claim, application.SessionConfigurationResult{
					Modes: &application.SessionModes{CurrentModeID: "fast", Available: newer.Available},
				})
				if err != nil {
					return err
				}

				if out.Data.Modes.CurrentModeID != "safe" {
					return errors.New("older response overwrote ACP update")
				}

				return nil
			},
		},
		{"worker claim and completion", func(ctx context.Context, db *sql.DB, p *application.Preparation) error {
			_, err := db.ExecContext(
				ctx,
				`INSERT INTO agent_connections(connection_id,display_name,command,args_json,transport,resolved_executable_path,last_verified_at,protocol_version,auth_state,schema_artifact_version,revision) VALUES('conn','Agent','agent','[]','stdio','/bin/agent','2026-09-26T12:00:00Z','1','authenticated','1',1)`,
			)
			if err != nil {
				return err
			}

			_, err = db.ExecContext(ctx, `UPDATE app_settings SET default_connection_id='conn' WHERE singleton=1`)
			if err != nil {
				return err
			}

			accepted, err := p.SendPreparationMessage(
				ctx,
				application.SendPreparationMessageInput{
					ProjectID:   "p",
					SessionID:   "s",
					OperationID: "turn",
					Content:     []application.ContentPart{{Type: "text", Text: "hello"}},
				},
			)
			if err != nil {
				return err
			}

			repo := NewPreparationRepository(db)

			job, err := repo.ClaimPreparationJob(ctx)
			if err != nil {
				return err
			}

			if job == nil || job.Kind != "turn" || job.TurnID != accepted.Data.TurnID || len(job.Content) != 1 {
				return errors.New("claim mismatch")
			}

			var messageStatus, sessionState string
			if err = db.QueryRowContext(ctx, `SELECT status FROM preparation_messages WHERE turn_id=? AND role='user'`, job.TurnID).
				Scan(&messageStatus); err != nil {
				return err
			}

			if err = db.QueryRowContext(ctx, `SELECT state FROM preparation_sessions WHERE session_id=?`, job.SessionID).
				Scan(&sessionState); err != nil {
				return err
			}

			if messageStatus != "streaming" || sessionState != "busy" {
				return errors.New("running turn is not cancellable in snapshot")
			}

			if err = repo.CompletePreparationJob(
				ctx,
				application.PreparationJobCompletion{
					JobID:       job.JobID,
					SessionID:   job.SessionID,
					Success:     true,
					StopReason:  "end_turn",
					CompletedAt: time.Now(),
				},
			); err != nil {
				return err
			}

			var state string
			if err = db.QueryRowContext(ctx, `SELECT state FROM preparation_turns WHERE turn_id=?`, job.TurnID).
				Scan(&state); err != nil {
				return err
			}

			if state != "completed" {
				return errors.New("turn completion mismatch")
			}

			if err = db.QueryRowContext(ctx, `SELECT status FROM preparation_messages WHERE turn_id=? AND role='user'`, job.TurnID).
				Scan(&messageStatus); err != nil {
				return err
			}

			if messageStatus != "completed" {
				return errors.New("message completion mismatch")
			}

			return nil
		}},
		{"brief dedup and revision", func(ctx context.Context, db *sql.DB, p *application.Preparation) error {
			in := application.SavePreparationBriefInput{
				ProjectID:                   "p",
				ExpectedPreparationRevision: 1,
				OperationID:                 "op",
				Brief: application.PreparationBrief{
					Purpose:            "Changed",
					CompletionCriteria: []string{"Done"},
					IntendedUsers:      "User",
				},
			}

			first, err := p.SavePreparationBrief(ctx, in)
			if err != nil {
				return err
			}

			again, err := p.SavePreparationBrief(ctx, in)
			if err != nil {
				return err
			}

			if first.Data.PreparationRevision != 2 || again.Receipt != first.Receipt {
				return errors.New("dedup mismatch")
			}

			in.OperationID = "op2"
			_, err = p.SavePreparationBrief(ctx, in)

			var conflict *shared.Error
			if !errors.As(err, &conflict) || conflict.Code != "revision_conflict" || *conflict.CurrentRevision != 2 {
				return errors.New("missing revision conflict")
			}

			var count int
			if err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM event_outbox WHERE name='preparation.changed'`).
				Scan(&count); err != nil {
				return err
			}

			if count != 1 {
				return errors.New("outbox not atomic")
			}

			return nil
		}},
		{
			"plan ordering and execution snapshot",
			func(ctx context.Context, db *sql.DB, p *application.Preparation) error {
				in := application.SaveCheckPlanInput{
					ProjectID:                   "p",
					ExpectedPreparationRevision: 1,
					ExpectedPlanRevision:        1,
					OperationID:                 "plan-op",
					Items: []application.CheckItemInput{
						{ClientKey: "second", Title: "B", ExpectedResult: "ok", HumanEvidenceRequirement: "none"},
						{ClientKey: "first", Title: "A", ExpectedResult: "ok", HumanEvidenceRequirement: "none"},
					},
				}

				saved, err := p.SaveCheckPlan(ctx, in)
				if err != nil {
					return err
				}

				if saved.Data.Items[0].Sequence != 1 || saved.Data.Items[1].Sequence != 2 ||
					len(saved.Data.AssignedIDs) != 2 {
					return errors.New("plan sequence mismatch")
				}

				started, err := p.StartExecution(
					ctx,
					application.StartExecutionInput{
						ProjectID:                   "p",
						ExpectedPreparationRevision: 1,
						ExpectedPlanRevision:        2,
						OperationID:                 "start",
					},
				)
				if err != nil {
					return err
				}

				if started.Data.ExecutionRevision != 1 {
					return errors.New("execution revision mismatch")
				}

				var count int
				if err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM execution_checks WHERE execution_id=?`, started.Data.ExecutionID).
					Scan(&count); err != nil {
					return err
				}

				if count != 2 {
					return errors.New("snapshot missing checks")
				}

				return nil
			},
		},
		{"read snapshot empty arrays", func(ctx context.Context, _ *sql.DB, p *application.Preparation) error {
			v, err := p.GetPreparation(ctx, application.PreparationViewQuery{ProjectID: "p"})
			if err != nil {
				return err
			}

			if v.CheckPlan.Items == nil || v.Conversation.Items == nil || v.Elicitations == nil ||
				v.Readiness.BlockingReasons == nil {
				return errors.New("nil collection")
			}

			return nil
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, db, p := seededPreparation(t)
			if err := tc.run(ctx, db, p); err != nil {
				t.Fatal(err)
			}
		})
	}
}
