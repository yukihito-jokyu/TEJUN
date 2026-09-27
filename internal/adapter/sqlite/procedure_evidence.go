package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/yukihito-jokyu/TEJUN/internal/application"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/shared"
)

func (r *ProcedureRepository) GetEvidence(
	ctx context.Context,
	procedureID, evidenceID string,
) (application.ProcedureEvidenceRecord, error) {
	var (
		out          application.ProcedureEvidenceRecord
		at           string
		hash, status sql.NullString
	)

	err := r.db.QueryRowContext(ctx, `SELECT e.evidence_id,x.project_id,e.execution_id,e.check_id,e.actor,e.kind,e.text,e.display_name,e.created_at,e.blob_hash,b.status
 FROM procedure_source_evidence pe JOIN execution_evidence e ON e.evidence_id=pe.evidence_id
 JOIN executions x ON x.execution_id=e.execution_id LEFT JOIN evidence_blobs b ON b.hash=e.blob_hash
 WHERE pe.procedure_id=? AND pe.evidence_id=?`, procedureID, evidenceID).
		Scan(&out.EvidenceID, &out.ProjectID, &out.ExecutionID, &out.CheckID, &out.Actor, &out.Kind, &out.Text, &out.DisplayName, &at, &hash, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return out, &shared.Error{Code: "not_found", Message: "証跡が見つかりません"}
	}

	if err != nil {
		return out, err
	}

	out.BlobHash, out.BlobStatus = hash.String, status.String
	out.CreatedAt, err = time.Parse(time.RFC3339Nano, at)

	return out, err
}
