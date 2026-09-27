package acp

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sdk "github.com/coder/acp-go-sdk"
	"github.com/yukihito-jokyu/TEJUN/internal/adapter/sqlite"
	"github.com/yukihito-jokyu/TEJUN/internal/application"
	"github.com/yukihito-jokyu/TEJUN/internal/trace"
)

func TestProcedureRevisionLiveACP(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	directory := t.TempDir()

	writer, err := trace.Open(directory)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = writer.Close() })

	ctx = trace.WithWriter(ctx, writer)

	t.Setenv("GO_WANT_FAKE_PROCEDURE_ACP", "1")

	workspace, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	db, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "tejun.db"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })
	seedExecutionIntegration(t, db)

	for _, query := range []string{
		`INSERT INTO execution_evidence(evidence_id,execution_id,check_id,actor,kind,text,display_name,created_at) VALUES ('evidence','e','c','human','text','observed','Text','2026-09-26T00:00:00Z')`,
		`UPDATE execution_checks SET human_status='completed',ai_required=0,human_evidence_requirement='text' WHERE execution_id='e' AND check_id='c'`,
	} {
		if _, err := db.ExecContext(ctx, query); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := db.ExecContext(
		ctx,
		`UPDATE agent_connections SET command=?,args_json=? WHERE connection_id='connection'`,
		os.Args[0],
		`["-test.run=^TestFakeProcedureACPProcess$"]`,
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

	at := time.Now().UTC()

	_, err = sqlite.NewExecutionRepository(db).GenerateProcedureDraft(ctx, application.ProcedureDraftRecord{
		ExecutionID:      "e",
		ProcedureID:      "procedure",
		OperationID:      "generate",
		ExpectedRevision: 1,
		At:               at,
		Event: application.OutboxEvent{
			ID:            "generated",
			Name:          "procedure.updated",
			AggregateType: "procedure",
			AggregateID:   "procedure",
			EmittedAt:     at,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	repo := sqlite.NewProcedureRepository(db, nil)

	before, err := repo.GetProcedure(ctx, application.ProcedureViewQuery{ProjectID: "p"})
	if err != nil {
		t.Fatal(err)
	}

	document := before.Procedure.Document
	document.Overview = "ACPが更新した概要"

	answer, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("FAKE_PROCEDURE_ANSWER", string(answer))

	manager := NewManager(nil, time.Now, func() string { return "probe" })

	t.Cleanup(func() { _ = manager.Close() })

	id := 0
	worker := application.NewProcedureRevision(
		repo,
		manager,
		time.Now,
		func() string { id++; return fmt.Sprintf("procedure-%d", id) },
	)

	accepted, err := worker.Request(
		ctx,
		application.RequestProcedureRevisionInput{
			ProcedureID:      "procedure",
			ExpectedRevision: 1,
			Content:          []application.ContentPart{{Type: "text", Text: "概要を変更"}},
			OperationID:      "request",
		},
	)
	if err != nil || accepted.Data.JobID == "" {
		t.Fatalf("accepted=%+v err=%v", accepted, err)
	}

	pending, err := repo.GetProcedure(ctx, application.ProcedureViewQuery{ProjectID: "p"})
	if err != nil || pending.ActiveRevision == nil || pending.ActiveRevision.JobID != accepted.Data.JobID {
		t.Fatalf("pending=%+v err=%v", pending.ActiveRevision, err)
	}

	ran, err := worker.RunOne(ctx)
	if err != nil || !ran {
		t.Fatalf("run=%t err=%v", ran, err)
	}

	after, err := repo.GetProcedure(ctx, application.ProcedureViewQuery{ProjectID: "p"})
	if err != nil || after.Procedure.Revision != 2 || after.Procedure.Document.Overview != document.Overview ||
		after.ActiveRevision != nil {
		t.Fatalf("after=%+v err=%v", after, err)
	}

	if len(after.Conversation.Items) < 2 {
		t.Fatalf("conversation=%+v", after.Conversation)
	}

	var state string
	if err := db.QueryRowContext(ctx, `SELECT state FROM procedure_revision_jobs WHERE job_id=?`, accepted.Data.JobID).
		Scan(&state); err != nil ||
		state != "completed" {
		t.Fatalf("job state=%q err=%v", state, err)
	}

	file, err := os.Open(filepath.Join(directory, "trace.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()

	foundGeneration := false

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var row struct {
			Phase             string `json:"phase"`
			Method            string `json:"method"`
			JobID             string `json:"jobId"`
			ProcessGeneration int64  `json:"processGeneration"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
			t.Fatal(err)
		}

		if row.Phase == "external_io_end" && row.Method == "ConnectProcedureRevision" &&
			row.JobID == accepted.Data.JobID {
			foundGeneration = row.ProcessGeneration > 0
		}
	}

	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}

	if !foundGeneration {
		t.Fatal("ACP connection process generation was not traced")
	}
}

func TestProcedureRevisionLiveACPControl(t *testing.T) {
	for _, mode := range []string{"cancel", "elicitation"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			t.Setenv("GO_WANT_FAKE_PROCEDURE_ACP", "1")
			t.Setenv("FAKE_PROCEDURE_MODE", mode)
			marker := filepath.Join(t.TempDir(), "prompt")
			t.Setenv("FAKE_PROCEDURE_MARKER", marker)

			workspace, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}

			dbPath := filepath.Join(t.TempDir(), "tejun.db")

			db, err := sqlite.Open(ctx, dbPath)
			if err != nil {
				t.Fatal(err)
			}

			t.Cleanup(func() { _ = db.Close() })
			seedExecutionIntegration(t, db)

			for _, query := range []string{
				`INSERT INTO execution_evidence(evidence_id,execution_id,check_id,actor,kind,text,display_name,created_at) VALUES ('evidence','e','c','human','text','observed','Text','2026-09-26T00:00:00Z')`,
				`UPDATE execution_checks SET human_status='completed',ai_required=0,human_evidence_requirement='text' WHERE execution_id='e' AND check_id='c'`,
			} {
				if _, err := db.ExecContext(ctx, query); err != nil {
					t.Fatal(err)
				}
			}

			if _, err := db.ExecContext(
				ctx,
				`UPDATE agent_connections SET command=?,args_json=? WHERE connection_id='connection'`,
				os.Args[0],
				`["-test.run=^TestFakeProcedureACPProcess$"]`,
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

			at := time.Now().UTC()

			_, err = sqlite.NewExecutionRepository(db).
				GenerateProcedureDraft(ctx, application.ProcedureDraftRecord{ExecutionID: "e", ProcedureID: "procedure", OperationID: "generate", ExpectedRevision: 1, At: at, Event: application.OutboxEvent{ID: "generated", Name: "procedure.updated", AggregateType: "procedure", AggregateID: "procedure", EmittedAt: at}})
			if err != nil {
				t.Fatal(err)
			}

			repo := sqlite.NewProcedureRepository(db, nil)

			before, err := repo.GetProcedure(ctx, application.ProcedureViewQuery{ProjectID: "p"})
			if err != nil {
				t.Fatal(err)
			}

			document := before.Procedure.Document
			document.Overview = "ACP confirmed"

			answer, err := json.Marshal(document)
			if err != nil {
				t.Fatal(err)
			}

			t.Setenv("FAKE_PROCEDURE_ANSWER", string(answer))

			id := 0
			newID := func() string { id++; return fmt.Sprintf("control-%d", id) }
			agentRepo := sqlite.NewAgentConnectionRepository(db)
			manager := NewManager(agentRepo, time.Now, newID)

			t.Cleanup(func() { _ = manager.Close() })

			worker := application.NewProcedureRevision(repo, manager, time.Now, newID)

			accepted, err := worker.Request(
				ctx,
				application.RequestProcedureRevisionInput{
					ProcedureID:      "procedure",
					ExpectedRevision: 1,
					Content:          []application.ContentPart{{Type: "text", Text: "概要を変更"}},
					OperationID:      "request",
				},
			)
			if err != nil {
				t.Fatal(err)
			}

			done := make(chan error, 1)

			go func() { _, runErr := worker.RunOne(ctx); done <- runErr }()

			deadline := time.Now().Add(5 * time.Second)

			for {
				if _, err := os.Stat(marker); err == nil {
					break
				}

				if time.Now().After(deadline) {
					t.Fatal("prompt not started")
				}

				time.Sleep(time.Millisecond)
			}

			eventAfter := accepted.Receipt.ChangeSequence

			if mode == "cancel" {
				pending, err := repo.GetProcedure(ctx, application.ProcedureViewQuery{ProjectID: "p"})
				if err != nil || pending.ActiveRevision == nil {
					t.Fatalf("pending=%+v err=%v", pending.ActiveRevision, err)
				}

				cancelled, cancelErr := worker.Cancel(
					ctx,
					pending.ActiveRevision.SessionID,
					pending.ActiveRevision.TurnID,
					accepted.Data.JobID,
					"cancel",
				)

				err = cancelErr
				if err != nil {
					t.Fatal(err)
				}

				eventAfter = cancelled.Receipt.ChangeSequence
			} else {
				var requestID string
				for {
					err = db.QueryRowContext(ctx, `SELECT elicitation_request_id FROM elicitation_requests WHERE state='pending'`).
						Scan(&requestID)
					if err == nil {
						break
					}

					if err != sql.ErrNoRows || time.Now().After(deadline) {
						t.Fatalf("elicitation: %v", err)
					}

					time.Sleep(time.Millisecond)
				}

				control := application.NewAgentControl(agentRepo, manager, manager, time.Now, newID)

				result, err := control.RespondToElicitation(
					ctx,
					application.RespondToElicitationInput{
						ElicitationRequestID: requestID,
						Action:               "accept",
						Content:              `{}`,
						OperationID:          "respond",
					},
				)
				if err != nil || result.Data.Status != "responded" {
					t.Fatalf("response=%+v err=%v", result, err)
				}
			}

			select {
			case err := <-done:
				if mode == "elicitation" && err != nil {
					t.Fatal(err)
				}
			case <-ctx.Done():
				t.Fatal("worker did not finish")
			}

			if mode == "cancel" {
				if _, err := os.Stat(marker + ".cancel"); err != nil {
					t.Fatalf("ACP session/cancel not received: %v", err)
				}
			}

			after, err := repo.GetProcedure(ctx, application.ProcedureViewQuery{ProjectID: "p"})
			if err != nil {
				t.Fatal(err)
			}

			wantRevision, wantState := int64(1), "cancelled"
			if mode == "elicitation" {
				wantRevision, wantState = 2, "completed"
			}

			if after.Procedure.Revision != wantRevision || after.ActiveRevision != nil {
				t.Fatalf("view=%+v", after)
			}

			if mode == "cancel" && after.Procedure.Document.Overview != before.Procedure.Document.Overview {
				t.Fatal("cancel changed draft")
			}

			if mode == "elicitation" && after.Procedure.Document.Overview != document.Overview {
				t.Fatal("elicitation answer not committed")
			}

			var state string
			if err := db.QueryRowContext(ctx, `SELECT state FROM procedure_revision_jobs WHERE job_id=?`, accepted.Data.JobID).
				Scan(&state); err != nil ||
				state != wantState {
				t.Fatalf("state=%s err=%v", state, err)
			}

			var name, correlation, payload string
			if err := db.QueryRowContext(ctx, `SELECT name,correlation,payload_json FROM event_outbox WHERE aggregate_id='procedure' AND name='procedure.updated' AND change_sequence>? AND dispatched_at IS NULL ORDER BY change_sequence DESC LIMIT 1`, eventAfter).
				Scan(&name, &correlation, &payload); err != nil ||
				name != "procedure.updated" ||
				!strings.Contains(correlation, `"procedureId":"procedure"`) ||
				!strings.Contains(payload, fmt.Sprintf(`"revision":%d`, wantRevision)) {
				t.Fatalf("event=%s correlation=%s payload=%s err=%v", name, correlation, payload, err)
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

			reloaded, err := sqlite.NewProcedureRepository(reopened, nil).
				GetProcedure(ctx, application.ProcedureViewQuery{ProjectID: "p"})
			if err != nil || reloaded.Procedure.Revision != wantRevision || reloaded.ActiveRevision != nil {
				t.Fatalf("reloaded=%+v err=%v", reloaded, err)
			}
		})
	}
}

func TestFakeProcedureACPProcess(t *testing.T) {
	if os.Getenv("GO_WANT_FAKE_PROCEDURE_ACP") != "1" {
		return
	}

	reader := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)

	var pendingPrompt json.RawMessage

	for reader.Scan() {
		var request struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if json.Unmarshal(reader.Bytes(), &request) != nil {
			os.Exit(2)
		}

		if os.Getenv("FAKE_PROCEDURE_MODE") == "elicitation" && request.ID != nil && request.Method == "" {
			var response struct {
				Result struct {
					Action string `json:"action"`
				} `json:"result"`
			}

			_ = json.Unmarshal(reader.Bytes(), &response)
			if response.Result.Action != "accept" {
				os.Exit(5)
			}

			_ = encoder.Encode(
				map[string]any{
					"jsonrpc": "2.0",
					"method":  "session/update",
					"params": map[string]any{
						"sessionId": "agent-session",
						"update": map[string]any{
							"sessionUpdate": "agent_message_chunk",
							"content":       map[string]any{"type": "text", "text": os.Getenv("FAKE_PROCEDURE_ANSWER")},
						},
					},
				},
			)
			_ = encoder.Encode(
				map[string]any{
					"jsonrpc": "2.0",
					"id":      pendingPrompt,
					"result":  map[string]any{"stopReason": "end_turn"},
				},
			)
			pendingPrompt = nil

			continue
		}

		var result any = struct{}{}

		switch request.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": sdk.ProtocolVersionNumber, "authMethods": []any{}}
		case "session/new":
			result = map[string]any{"sessionId": "agent-session"}
		case "session/prompt":
			var prompt struct {
				Prompt []struct {
					Text string `json:"text"`
				} `json:"prompt"`
			}
			if json.Unmarshal(request.Params, &prompt) != nil || len(prompt.Prompt) != 1 ||
				!strings.Contains(prompt.Prompt[0].Text, "概要を変更") ||
				!strings.Contains(prompt.Prompt[0].Text, "手順書") {
				os.Exit(4)
			}

			if marker := os.Getenv("FAKE_PROCEDURE_MARKER"); marker != "" {
				_ = os.WriteFile(marker, []byte("prompt"), 0o600)
			}

			if mode := os.Getenv("FAKE_PROCEDURE_MODE"); mode == "cancel" || mode == "elicitation" {
				pendingPrompt = append([]byte(nil), request.ID...)

				if mode == "elicitation" {
					_ = encoder.Encode(
						map[string]any{
							"jsonrpc": "2.0",
							"id":      "question",
							"method":  "elicitation/create",
							"params": map[string]any{
								"mode":            "form",
								"message":         "Continue?",
								"requestedSchema": map[string]any{"type": "object", "properties": map[string]any{}},
							},
						},
					)
				}

				continue
			}

			if encoder.Encode(
				map[string]any{
					"jsonrpc": "2.0",
					"method":  "session/update",
					"params": map[string]any{
						"sessionId": "agent-session",
						"update": map[string]any{
							"sessionUpdate": "agent_message_chunk",
							"content":       map[string]any{"type": "text", "text": os.Getenv("FAKE_PROCEDURE_ANSWER")},
						},
					},
				},
			) != nil {
				os.Exit(3)
			}

			result = map[string]any{"stopReason": "end_turn"}
		case "session/cancel":
			if marker := os.Getenv("FAKE_PROCEDURE_MARKER"); marker != "" {
				if os.WriteFile(marker+".cancel", []byte("cancel"), 0o600) != nil {
					os.Exit(6)
				}
			}

			if pendingPrompt != nil {
				_ = encoder.Encode(
					map[string]any{
						"jsonrpc": "2.0",
						"id":      pendingPrompt,
						"result":  map[string]any{"stopReason": "cancelled"},
					},
				)
				pendingPrompt = nil
			}

			continue
		}

		if encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result}) != nil {
			os.Exit(3)
		}
	}

	os.Exit(0)
}
