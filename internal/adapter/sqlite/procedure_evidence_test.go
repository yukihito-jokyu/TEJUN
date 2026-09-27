package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/yukihito-jokyu/TEJUN/internal/application"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/shared"
)

func TestProcedureEvidenceRelationAndTextPage(t *testing.T) {
	ctx := context.Background()
	executionRepo, at := executionFixture(t)

	db := executionRepo.db
	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO execution_evidence(evidence_id,execution_id,check_id,actor,kind,text,display_name,created_at) VALUES ('evidence','e','c','human','text','あいうえお','Text','2026-09-26T00:00:00Z')`,
	); err != nil {
		t.Fatal(err)
	}

	if _, err := db.ExecContext(
		ctx,
		`UPDATE execution_checks SET human_status='completed' WHERE execution_id='e' AND check_id='c'`,
	); err != nil {
		t.Fatal(err)
	}

	_, err := executionRepo.GenerateProcedureDraft(
		ctx,
		application.ProcedureDraftRecord{
			ExecutionID:      "e",
			ProcedureID:      "procedure",
			OperationID:      "generate",
			ExpectedRevision: 1,
			At:               at,
			Event: application.OutboxEvent{
				ID:            "procedure-event",
				Name:          "procedure.updated",
				AggregateType: "procedure",
				AggregateID:   "procedure",
				EmittedAt:     at,
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	repo := NewProcedureRepository(db, nil)

	for _, tc := range []struct{ name, procedureID, code string }{{"related", "procedure", ""}, {"unrelated", "other", "not_found"}} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := repo.GetEvidence(ctx, tc.procedureID, "evidence")
			if tc.code == "" {
				if err != nil || result.Text != "あいうえお" {
					t.Fatalf("result=%+v err=%v", result, err)
				}

				return
			}

			var business *shared.Error
			if !errors.As(err, &business) || business.Code != tc.code {
				t.Fatalf("err=%v", err)
			}
		})
	}
}
