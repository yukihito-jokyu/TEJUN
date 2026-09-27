package acp

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yukihito-jokyu/TEJUN/internal/adapter/sqlite"
	"github.com/yukihito-jokyu/TEJUN/internal/application"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/agentconnection"
)

func TestExecutionWorkerSQLiteAndProcessRecovery(t *testing.T) {
	for _, tc := range []struct {
		name, mode        string
		interrupt, cancel bool
	}{
		{name: "worker result persists", mode: "execution_checks"},
		{name: "invalid JSON is retried", mode: "execution_retry_json"},
		{name: "invalid JSON attempts remain visible", mode: "execution_invalid_json"},
		{name: "permission response persists", mode: "execution_permission"},
		{name: "late chunk cannot complete check", mode: "execution_late_chunk"},
		{name: "running job is not replayed after restart", mode: "block_prompt", interrupt: true},
		{name: "cancelled prompt is not replayed", mode: "block_prompt", cancel: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			dataDir := t.TempDir()
			dbPath := filepath.Join(dataDir, "tejun.db")
			promptMarker := filepath.Join(dataDir, "prompt-received")

			db, err := sqlite.Open(ctx, dbPath)
			if err != nil {
				t.Fatal(err)
			}

			t.Cleanup(func() { _ = db.Close() })
			seedExecutionIntegration(t, db)

			manager := NewManager(nil, time.Now, func() string { return "probe" })

			t.Cleanup(func() { _ = manager.Close() })

			workspace, err := filepath.EvalSymlinks(dataDir)
			if err != nil {
				t.Fatal(err)
			}

			_, err = manager.ExecutePreparation(ctx, application.ClaimedPreparationJob{
				Kind: "connect", JobID: "connect", SessionID: "s", WorkspacePath: workspace,
				Connection: agentconnection.ConnectionInput{
					Command: os.Args[0], Args: []string{"-test.run=TestFakeACPProcess"},
					EnvironmentOverrides: []agentconnection.EnvironmentVariable{
						{Name: "GO_WANT_FAKE_ACP", Value: "1"},
						{Name: "FAKE_ACP_MODE", Value: tc.mode},
						{Name: "FAKE_ACP_PROMPT_MARKER", Value: promptMarker},
					},
				},
			})
			if err != nil {
				t.Fatal(err)
			}

			repository := sqlite.NewExecutionRepository(db)
			manager.SetPermissionRepository(repository)

			if tc.mode == "execution_late_chunk" {
				message, promptErr := manager.ExecuteExecutionJob(ctx, application.ClaimedExecutionJob{
					Kind: "message", SessionID: "s", JobID: "earlier", TurnID: "earlier", PromptText: "first",
				})
				if promptErr != nil || message.Message != "reply" {
					t.Fatalf("first prompt=%+v err=%v", message, promptErr)
				}
			}

			ids := 0
			runner := application.NewExecutionRunner(repository, manager, time.Now, func() string {
				ids++
				return "generated-" + string(rune('0'+ids))
			})

			accepted, err := runner.RunPendingChecks(ctx, application.RunPendingChecksInput{
				ExecutionID: "e", ExpectedRevision: 1, OperationID: "run",
			})
			if err != nil || accepted.Data.JobID == "" {
				t.Fatalf("accept=%+v err=%v", accepted, err)
			}

			if tc.cancel || tc.mode == "execution_permission" {
				done := make(chan error, 1)

				go func() {
					_, runErr := runner.RunOne(ctx)
					done <- runErr
				}()

				deadline := time.Now().Add(time.Second)

				for {
					if _, statErr := os.Stat(promptMarker); statErr == nil {
						break
					}

					if time.Now().After(deadline) {
						t.Fatal("prompt did not start")
					}

					time.Sleep(time.Millisecond)
				}

				if tc.mode == "execution_permission" {
					var permissionID string

					deadline := time.Now().Add(time.Second)

					for {
						err = db.QueryRowContext(ctx, `SELECT permission_request_id FROM execution_permissions WHERE execution_id='e'`).
							Scan(&permissionID)
						if err == nil {
							break
						}

						if err != sql.ErrNoRows || time.Now().After(deadline) {
							t.Fatalf("permission not persisted: %v", err)
						}

						time.Sleep(time.Millisecond)
					}

					permission := application.NewExecutionPermission(repository, manager, time.Now)

					result, respondErr := permission.Respond(ctx, application.PermissionClaim{
						SessionID: "s", PermissionRequestID: permissionID, OptionID: "allow", OperationID: "respond",
					})
					if respondErr != nil || result.Data.Status != "sent" {
						t.Fatalf("permission response=%+v err=%v", result, respondErr)
					}

					if _, respondErr := permission.Respond(ctx, application.PermissionClaim{
						SessionID:           "s",
						PermissionRequestID: permissionID,
						OptionID:            "allow",
						OperationID:         "respond-again",
					}); respondErr == nil {
						t.Fatal("permission accepted a second response")
					}
				} else {
					_, err = runner.CancelAgentOperation(ctx, application.CancelAgentOperationInput{
						SessionID: "s", RunID: accepted.Data.RunID, JobID: accepted.Data.JobID,
						OperationID: "cancel",
					})
					if err != nil {
						t.Fatal(err)
					}
				}

				select {
				case err := <-done:
					if err != nil {
						t.Fatal(err)
					}
				case <-ctx.Done():
					t.Fatal("cancelled prompt did not stop")
				}

				if err := manager.Close(); err != nil {
					t.Fatal(err)
				}

				if err := db.Close(); err != nil {
					t.Fatal(err)
				}

				reopened, err := sqlite.Open(ctx, dbPath)
				if err != nil {
					t.Fatal(err)
				}

				t.Cleanup(func() { _ = reopened.Close() })

				restartedRepository := sqlite.NewExecutionRepository(reopened)
				if err := restartedRepository.FailInterruptedExecutionJobs(ctx, time.Now()); err != nil {
					t.Fatal(err)
				}

				if err := restartedRepository.FailInterruptedPermissions(ctx, time.Now()); err != nil {
					t.Fatal(err)
				}

				var state string

				want := "cancelled"
				if tc.mode == "execution_permission" {
					want = "completed"
				}

				if err := reopened.QueryRowContext(ctx, `SELECT state FROM execution_agent_jobs WHERE job_id=?`,
					accepted.Data.JobID).Scan(&state); err != nil || state != want {
					t.Fatalf("reopened job state=%q want=%q err=%v", state, want, err)
				}

				if tc.mode == "execution_permission" {
					var permissionState, rpcID string
					if err := reopened.QueryRowContext(ctx, `SELECT state,rpc_request_id_json FROM execution_permissions WHERE execution_id='e'`).
						Scan(&permissionState, &rpcID); err != nil ||
						permissionState != "sent" ||
						rpcID != `"permission-request"` {
						t.Fatalf("reopened permission state=%q rpc=%q err=%v", permissionState, rpcID, err)
					}
				}

				restartedManager := NewManager(nil, time.Now, func() string { return "unused" })
				defer func() { _ = restartedManager.Close() }()

				restartedRunner := application.NewExecutionRunner(restartedRepository, restartedManager, time.Now,
					func() string { return "unused" })
				if ran, err := restartedRunner.RunOne(ctx); err != nil || ran {
					t.Fatalf("restarted job replayed: ran=%t err=%v", ran, err)
				}

				return
			}

			if !tc.interrupt {
				ran, err := runner.RunOne(ctx)

				wantError := tc.mode == "execution_late_chunk" || tc.mode == "execution_invalid_json"
				if !ran || (err != nil) != wantError {
					t.Fatalf("run=%t err=%v", ran, err)
				}

				if tc.mode == "execution_late_chunk" {
					var state string
					if err := db.QueryRowContext(ctx, `SELECT state FROM execution_agent_jobs WHERE job_id=?`,
						accepted.Data.JobID).Scan(&state); err != nil || state != "failed" {
						t.Fatalf("late chunk job state=%q err=%v", state, err)
					}

					return
				}

				if tc.mode == "execution_invalid_json" {
					view, err := repository.GetExecution(ctx, "p", 10)
					if err != nil || len(view.Conversation) < 5 ||
						view.Checks[0].AIStatus != "failed" ||
						!strings.Contains(view.Conversation[1].Text, "形式エラー") {
						t.Fatalf("failed view=%+v err=%v", view, err)
					}

					return
				}

				view, err := repository.GetExecution(ctx, "p", 10)
				if err != nil || len(view.Checks) != 1 || view.Checks[0].AIStatus != "completed" ||
					len(view.Checks[0].Evidence) != 1 {
					t.Fatalf("saved view=%+v err=%v", view, err)
				}

				return
			}

			job, err := repository.ClaimExecutionJob(ctx)
			if err != nil || job == nil || job.JobID != accepted.Data.JobID {
				t.Fatalf("claim=%+v err=%v", job, err)
			}

			// A process exit leaves the claimed job uncertain; bootstrap marks it failed before workers run.
			if err := manager.Close(); err != nil {
				t.Fatal(err)
			}

			if err := db.Close(); err != nil {
				t.Fatal(err)
			}

			reopened, err := sqlite.Open(ctx, dbPath)
			if err != nil {
				t.Fatal(err)
			}

			t.Cleanup(func() { _ = reopened.Close() })

			restartedRepository := sqlite.NewExecutionRepository(reopened)
			if err := restartedRepository.FailInterruptedExecutionJobs(ctx, time.Now()); err != nil {
				t.Fatal(err)
			}

			restartedManager := NewManager(nil, time.Now, func() string { return "unused" })
			defer func() { _ = restartedManager.Close() }()

			restartedRunner := application.NewExecutionRunner(restartedRepository, restartedManager, time.Now,
				func() string { return "unused" })

			ran, err := restartedRunner.RunOne(ctx)
			if err != nil || ran {
				t.Fatalf("restarted worker replayed job: ran=%t err=%v", ran, err)
			}

			var state string
			if err := reopened.QueryRowContext(ctx, `SELECT state FROM execution_agent_jobs WHERE job_id=?`,
				accepted.Data.JobID).Scan(&state); err != nil || state != "failed" {
				t.Fatalf("recovered job state=%q err=%v", state, err)
			}
		})
	}
}

func TestExecutionReconnectsAndRetriesFailedChecks(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	t.Setenv("GO_WANT_FAKE_ACP", "1")
	t.Setenv("FAKE_ACP_MODE", "execution_checks")

	workspace, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	db, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "tejun.db"))
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = db.Close() }()

	seedExecutionIntegration(t, db)

	if _, err := db.ExecContext(ctx, `UPDATE execution_checks SET ai_required=0 WHERE check_id='c'`); err != nil {
		t.Fatal(err)
	}

	if _, err := db.ExecContext(
		ctx,
		`UPDATE agent_connections SET command=?,args_json=? WHERE connection_id='connection'`,
		os.Args[0],
		`["-test.run=TestFakeACPProcess"]`,
	); err != nil {
		t.Fatal(err)
	}

	if _, err := db.ExecContext(
		ctx,
		`UPDATE projects SET workspace_path=? WHERE project_id='p'`,
		workspace,
	); err != nil {
		t.Fatal(err)
	}

	repository := sqlite.NewExecutionRepository(db)

	manager := NewManager(nil, time.Now, func() string { return "probe" })
	defer func() { _ = manager.Close() }()

	ids := 0

	runner := application.NewExecutionRunner(repository, manager, time.Now, func() string {
		ids++
		return fmt.Sprintf("id-%d", ids)
	})
	for _, expectedRevision := range []int64{1, 3} {
		if _, err := runner.RunPendingChecks(ctx, application.RunPendingChecksInput{
			ExecutionID: "e", ExpectedRevision: expectedRevision,
			OperationID: fmt.Sprintf("run-%d", expectedRevision),
		}); err != nil {
			t.Fatal(err)
		}

		if ran, err := runner.RunOne(ctx); !ran || err != nil {
			t.Fatalf("run=%t err=%v", ran, err)
		}

		if expectedRevision == 1 {
			if _, err := db.ExecContext(
				ctx,
				`UPDATE execution_checks SET ai_status='failed',ai_checked_at=NULL WHERE check_id='c'`,
			); err != nil {
				t.Fatal(err)
			}
		}
	}

	var status string
	if err := db.QueryRowContext(ctx, `SELECT ai_status FROM execution_checks WHERE check_id='c'`).
		Scan(&status); err != nil ||
		status != "completed" {
		t.Fatalf("check status=%q err=%v", status, err)
	}

	var newSessionID, previousSessionID, permissionMode string
	if err := db.QueryRowContext(ctx, `SELECT e.session_id,s.previous_session_id,s.permission_mode
FROM executions e JOIN preparation_sessions s ON s.session_id=e.session_id WHERE e.execution_id='e'`).
		Scan(&newSessionID, &previousSessionID, &permissionMode); err != nil ||
		newSessionID == "s" || previousSessionID != "s" || permissionMode != "ask_every_time" {
		t.Fatalf("reconnected session=%q previous=%q permission=%q err=%v",
			newSessionID, previousSessionID, permissionMode, err)
	}
}

func seedExecutionIntegration(t *testing.T, db *sql.DB) {
	t.Helper()

	for _, statement := range []string{
		`INSERT INTO agent_connections(connection_id,display_name,command,args_json,transport,resolved_executable_path,last_verified_at,protocol_version,auth_state,schema_artifact_version,revision)
VALUES('connection','Fake','fake','[]','stdio','/bin/fake','2026-09-26T00:00:00Z','1','not_required','1',1)`,
		`INSERT INTO projects(project_id,lineage_id,connection_id,name,workspace_path,status,current_stage,revision,created_at,updated_at)
VALUES('p','p','connection','Project','/tmp','human_waiting','execution',1,1,1)`,
		`INSERT INTO check_plans(project_id,revision) VALUES('p',1)`,
		`INSERT INTO preparation_sessions(session_id,project_id,state,started_at)
VALUES('s','p','ready','2026-09-26T00:00:00Z')`,
		`INSERT INTO executions(execution_id,project_id,session_id,status,revision,started_at)
VALUES('e','p','s','active',1,'2026-09-26T00:00:00Z')`,
		`INSERT INTO execution_checks(check_id,execution_id,sequence,title,instruction,expected_result,suggested_command,ai_required,human_required,human_evidence_requirement)
VALUES('c','e',1,'Check','Do it','Done','',1,1,'image')`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
}
