package wails

import (
	"context"

	"github.com/yukihito-jokyu/TEJUN/internal/application"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/shared"
	"github.com/yukihito-jokyu/TEJUN/internal/trace"
)

func (s *ProcedureService) traceEntry(ctx context.Context, method, operationID, procedureID string) context.Context {
	if s.projects != nil {
		ctx = trace.WithWriter(ctx, s.projects.trace)
	}

	trace.Record(
		ctx,
		trace.Entry{Phase: "binding_entry", Method: method, OperationID: operationID, AggregateID: procedureID},
	)

	return ctx
}

func NewProcedureService(
	p *application.Procedure,
	evidence application.ProcedureEvidenceReader,
	projects *ProjectService,
	revision *application.ProcedureRevision,
	previewURL func(string, string, string, string) string,
	verifyImage func(context.Context, string, string, string, string) error,
) *ProcedureService {
	return &ProcedureService{
		procedure:   p,
		revision:    revision,
		evidence:    evidence,
		projects:    projects,
		previewURL:  previewURL,
		verifyImage: verifyImage,
	}
}

type (
	ProcedureViewQuery      = application.ProcedureViewQuery
	SaveProcedureDraftInput = application.ProcedureSaveInput
	CompleteProcedureInput  = application.ProcedureCompleteInput
)

type GetEvidenceInput struct {
	ProcedureID string `json:"procedureId"`
	EvidenceID  string `json:"evidenceId"`
	TextCursor  string `json:"textCursor,omitempty"`
	TextLimit   int    `json:"textLimit,omitempty"`
}
type ProcedureEvidenceDetail struct {
	Summary   application.ProcedureEvidenceSummary `json:"summary"`
	Source    any                                  `json:"source"`
	TextPage  *ProcedureTextSegment                `json:"textPage,omitempty"`
	Image     *ProcedureEvidenceImage              `json:"image,omitempty"`
	Integrity string                               `json:"integrity"`
}
type ProcedureTextSegment struct {
	Content    string  `json:"content"`
	NextCursor *string `json:"nextCursor,omitempty"`
	Truncated  bool    `json:"truncated"`
}
type ProcedureEvidenceImage struct {
	PreviewURL string `json:"previewUrl"`
	Alt        string `json:"alt"`
}

func (s *ProcedureService) GetProcedure(
	ctx context.Context,
	query ProcedureViewQuery,
) (application.ProcedureView, error) {
	ctx = s.traceEntry(ctx, "GetProcedure", "", query.ProjectID)
	if err := required("projectId", query.ProjectID); err != nil {
		return application.ProcedureView{}, err
	}

	v, err := s.procedure.Get(ctx, query)
	if err != nil {
		return v, storageError(err)
	}

	return v, nil
}

func (s *ProcedureService) SaveProcedureDraft(
	ctx context.Context,
	in SaveProcedureDraftInput,
) (MutationResult[application.ProcedureSaved], error) {
	ctx = s.traceEntry(ctx, "SaveProcedureDraft", in.OperationID, in.ProcedureID)
	if err := required("procedureId", in.ProcedureID); err != nil {
		return MutationResult[application.ProcedureSaved]{}, err
	}

	if err := required("operationId", in.OperationID); err != nil {
		return MutationResult[application.ProcedureSaved]{}, err
	}

	r, err := s.procedure.SaveDraft(ctx, in)
	if err != nil {
		return MutationResult[application.ProcedureSaved]{}, storageError(err)
	}

	traceAccepted(ctx, "SaveProcedureDraft", in.ProcedureID, "", receipt(r.Receipt))

	return MutationResult[application.ProcedureSaved]{Data: r.Data, Receipt: receipt(r.Receipt)}, nil
}

func (s *ProcedureService) CompleteProcedure(
	ctx context.Context,
	in CompleteProcedureInput,
) (MutationResult[application.ProcedureCompleted], error) {
	ctx = s.traceEntry(ctx, "CompleteProcedure", in.OperationID, in.ProcedureID)
	if err := required("procedureId", in.ProcedureID); err != nil {
		return MutationResult[application.ProcedureCompleted]{}, err
	}

	if err := required("operationId", in.OperationID); err != nil {
		return MutationResult[application.ProcedureCompleted]{}, err
	}

	r, err := s.procedure.Complete(ctx, in)
	if err != nil {
		return MutationResult[application.ProcedureCompleted]{}, storageError(err)
	}

	traceAccepted(ctx, "CompleteProcedure", in.ProcedureID, "", receipt(r.Receipt))

	return MutationResult[application.ProcedureCompleted]{Data: r.Data, Receipt: receipt(r.Receipt)}, nil
}

func (s *ProcedureService) GetEvidence(ctx context.Context, in GetEvidenceInput) (ProcedureEvidenceDetail, error) {
	ctx = s.traceEntry(ctx, "GetEvidence", "", in.ProcedureID)
	if err := required("procedureId", in.ProcedureID); err != nil {
		return ProcedureEvidenceDetail{}, err
	}

	if err := required("evidenceId", in.EvidenceID); err != nil {
		return ProcedureEvidenceDetail{}, err
	}

	row, err := s.evidence.GetEvidence(ctx, in.ProcedureID, in.EvidenceID)
	if err != nil {
		return ProcedureEvidenceDetail{}, storageError(err)
	}

	detail := ProcedureEvidenceDetail{
		Summary: application.ProcedureEvidenceSummary{
			EvidenceID:  row.EvidenceID,
			Actor:       row.Actor,
			Kind:        row.Kind,
			DisplayName: row.DisplayName,
			CreatedAt:   row.CreatedAt,
		},
		Integrity: "verified",
	}
	if row.Actor == "ai" {
		detail.Source = map[string]string{"actor": "ai", "runId": ""}
	} else {
		detail.Source = map[string]string{"actor": "human"}
	}

	switch row.Kind {
	case "text":
		content, next, truncated, e := procedureTextPage(row.Text, in.TextCursor, in.TextLimit)
		if e != nil {
			return ProcedureEvidenceDetail{}, e
		}

		detail.TextPage = &ProcedureTextSegment{Content: content, NextCursor: next, Truncated: truncated}
	case "image":
		if row.BlobHash == "" || row.BlobStatus != "available" {
			detail.Integrity = "missing"
			break
		}

		if s.previewURL == nil {
			return ProcedureEvidenceDetail{}, &shared.Error{Code: "unavailable", Message: "画像の表示に対応していません"}
		}

		if s.verifyImage == nil ||
			s.verifyImage(ctx, row.ProjectID, row.ExecutionID, row.CheckID, row.EvidenceID) != nil {
			detail.Integrity = "missing"
			break
		}

		detail.Image = &ProcedureEvidenceImage{
			PreviewURL: s.previewURL(row.ProjectID, row.ExecutionID, row.CheckID, row.EvidenceID),
			Alt:        row.DisplayName,
		}
	default:
		return ProcedureEvidenceDetail{}, &shared.Error{Code: "invalid_state", Message: "証跡の種類が不正です"}
	}

	return detail, nil
}

func (s *ProcedureService) PrepareExportProcedure(
	ctx context.Context,
	in PrepareExportProcedureInput,
) (PreparedExportProcedure, error) {
	ctx = s.traceEntry(ctx, "PrepareExportProcedure", "", in.ProcedureID)
	return s.projects.PrepareExportProcedure(ctx, in)
}

func (s *ProcedureService) ExportProcedure(
	ctx context.Context,
	in ExportProcedureInput,
) (MutationResult[ExportAccepted], error) {
	ctx = s.traceEntry(ctx, "ExportProcedure", in.OperationID, in.ProcedureID)
	return s.projects.ExportProcedure(ctx, in)
}

func (s *ProcedureService) RequestProcedureRevision(
	ctx context.Context,
	in application.RequestProcedureRevisionInput,
) (MutationResult[application.AcceptedTurn], error) {
	ctx = s.traceEntry(ctx, "RequestProcedureRevision", in.OperationID, in.ProcedureID)
	if s.revision == nil {
		return MutationResult[application.AcceptedTurn]{}, &shared.Error{Code: "unavailable", Message: "修正依頼を利用できません"}
	}

	result, err := s.revision.Request(ctx, in)
	if err != nil {
		return MutationResult[application.AcceptedTurn]{}, storageError(err)
	}

	traceAccepted(ctx, "RequestProcedureRevision", in.ProcedureID, result.Data.JobID, receipt(result.Receipt))

	return MutationResult[application.AcceptedTurn]{Data: result.Data, Receipt: receipt(result.Receipt)}, nil
}
