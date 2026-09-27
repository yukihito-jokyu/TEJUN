package application

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/yukihito-jokyu/TEJUN/internal/domain/shared"
)

type AttachHumanEvidenceInput struct {
	ExecutionID, CheckID, Kind, Text, SourcePath, DisplayName, OperationID string
	ExpectedRevision                                                       int64
}

type StagedEvidence struct {
	Hash, StagingName, MIME string
	Size                    int64
}

type EvidenceStore interface {
	Stage(context.Context, string) (StagedEvidence, error)
	Commit(context.Context, StagedEvidence) error
	Verify(context.Context, StagedEvidence) (bool, error)
}

type EvidenceRecord struct {
	Input      AttachHumanEvidenceInput
	EvidenceID string
	Staged     *StagedEvidence
	At         time.Time
	Event      OutboxEvent
}

type EvidenceAttached struct {
	ExecutionID, CheckID string
	Evidence             EvidenceSummary
	ExecutionRevision    int64
	CanGenerate          bool
	BlockingReasons      []string
}

type PendingEvidence struct {
	EvidenceID string
	Staged     StagedEvidence
}

type EvidenceRepository interface {
	FindEvidenceOperation(
		context.Context,
		AttachHumanEvidenceInput,
	) (MutationResult[EvidenceAttached], bool, bool, error)
	AttachTextEvidence(context.Context, EvidenceRecord) (MutationResult[EvidenceAttached], error)
	BeginImageEvidence(context.Context, EvidenceRecord) error
	CompleteImageEvidence(context.Context, string, bool, time.Time) (MutationResult[EvidenceAttached], error)
	PendingImageEvidence(context.Context) ([]PendingEvidence, error)
}

type ExecutionEvidence struct {
	repository EvidenceRepository
	store      EvidenceStore
	now        func() time.Time
	newID      func() string
}

func NewExecutionEvidence(repository EvidenceRepository, store EvidenceStore,
	now func() time.Time, newID func() string,
) *ExecutionEvidence {
	return &ExecutionEvidence{repository: repository, store: store, now: now, newID: newID}
}

func (e *ExecutionEvidence) Attach(ctx context.Context,
	input AttachHumanEvidenceInput,
) (MutationResult[EvidenceAttached], error) {
	at := e.now()

	record := EvidenceRecord{
		Input: input, EvidenceID: e.newID(), At: at,
		Event: OutboxEvent{
			ID: e.newID(), Name: "execution.updated", EmittedAt: at,
			AggregateType: "execution", AggregateID: input.ExecutionID, Correlation: input.OperationID,
		},
	}
	if input.Kind == "text" {
		return e.repository.AttachTextEvidence(ctx, record)
	}

	prior, found, pending, err := e.repository.FindEvidenceOperation(ctx, input)
	if err != nil {
		return MutationResult[EvidenceAttached]{}, err
	}

	if found {
		return prior, nil
	}

	if pending {
		return MutationResult[EvidenceAttached]{}, &shared.Error{
			Code: "operation_pending", Message: "画像証跡を確定中です", Retryable: true,
		}
	}

	staged, err := e.store.Stage(ctx, input.SourcePath)
	if err != nil {
		return MutationResult[EvidenceAttached]{}, err
	}

	record.Staged = &staged
	if err := e.repository.BeginImageEvidence(ctx, record); err != nil {
		return MutationResult[EvidenceAttached]{}, err
	}

	if err := e.store.Commit(ctx, staged); err != nil {
		return MutationResult[EvidenceAttached]{}, err
	}

	return e.repository.CompleteImageEvidence(ctx, record.EvidenceID, true, e.now())
}

func (e *ExecutionEvidence) Reconcile(ctx context.Context) error {
	pending, err := e.repository.PendingImageEvidence(ctx)
	if err != nil {
		return err
	}

	for _, item := range pending {
		valid, err := e.store.Verify(ctx, item.Staged)
		if err != nil {
			return err
		}

		if !valid {
			if err := e.store.Commit(ctx, item.Staged); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}

	for _, item := range pending {
		valid, err := e.store.Verify(ctx, item.Staged)
		if err != nil {
			return err
		}

		if _, err := e.repository.CompleteImageEvidence(ctx, item.EvidenceID, valid, e.now()); err != nil {
			return err
		}
	}

	return nil
}
