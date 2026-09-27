package wails

import (
	"context"
	"errors"
	"testing"

	"github.com/yukihito-jokyu/TEJUN/internal/application"
)

type procedureEvidenceStub struct {
	row application.ProcedureEvidenceRecord
}

func (s procedureEvidenceStub) GetEvidence(
	context.Context,
	string,
	string,
) (application.ProcedureEvidenceRecord, error) {
	return s.row, nil
}

func TestProcedureImageNeedsBlobVerification(t *testing.T) {
	service := &ProcedureService{
		evidence: procedureEvidenceStub{row: application.ProcedureEvidenceRecord{
			EvidenceID:  "e",
			ProjectID:   "p",
			ExecutionID: "x",
			CheckID:     "c",
			Kind:        "image",
			BlobHash:    "hash",
			BlobStatus:  "available",
		}},
		previewURL:  func(string, string, string, string) string { return "/preview" },
		verifyImage: func(context.Context, string, string, string, string) error { return errors.New("corrupt") },
	}

	got, err := service.GetEvidence(context.Background(), GetEvidenceInput{ProcedureID: "procedure", EvidenceID: "e"})
	if err != nil || got.Integrity != "missing" || got.Image != nil {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

func TestProcedureBindingRejectsMissingIDs(t *testing.T) {
	service := &ProcedureService{}
	ctx := context.Background()

	tests := []struct {
		name string
		call func() error
	}{
		{"get", func() error { _, err := service.GetProcedure(ctx, ProcedureViewQuery{}); return err }},
		{"save", func() error { _, err := service.SaveProcedureDraft(ctx, SaveProcedureDraftInput{}); return err }},
		{"complete", func() error { _, err := service.CompleteProcedure(ctx, CompleteProcedureInput{}); return err }},
		{"evidence", func() error { _, err := service.GetEvidence(ctx, GetEvidenceInput{}); return err }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(); err == nil {
				t.Fatal("invalid input accepted")
			}
		})
	}
}

func TestProcedureTextPage(t *testing.T) {
	for _, tc := range []struct {
		name, cursor, want string
		limit              int
		truncated          bool
	}{{"first", "", "あい", 2, true}, {"next", "2", "うえ", 2, true}, {"last", "4", "お", 2, false}} {
		t.Run(tc.name, func(t *testing.T) {
			got, _, truncated, err := procedureTextPage("あいうえお", tc.cursor, tc.limit)
			if err != nil || got != tc.want || truncated != tc.truncated {
				t.Fatalf("got=%q truncated=%v err=%v", got, truncated, err)
			}
		})
	}
}
