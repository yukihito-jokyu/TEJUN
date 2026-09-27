package application

import (
	"context"
	"time"
)

type ProcedureEvidenceRecord struct {
	EvidenceID  string
	ProjectID   string
	ExecutionID string
	CheckID     string
	Actor       string
	Kind        string
	Text        string
	DisplayName string
	CreatedAt   time.Time
	BlobHash    string
	BlobStatus  string
}

type ProcedureEvidenceReader interface {
	GetEvidence(context.Context, string, string) (ProcedureEvidenceRecord, error)
}
