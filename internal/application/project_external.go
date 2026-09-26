package application

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/yukihito-jokyu/TEJUN/internal/domain/agentconnection"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/shared"
	"github.com/yukihito-jokyu/TEJUN/internal/trace"
)

type ProjectExternalRepository interface {
	ProcedureExportable(context.Context, string, int64) (bool, error)
	ExistingExport(context.Context, string, string) (MutationResult[ExportAccepted], bool, error)
	AcceptReconnect(context.Context, ProjectReconnectRecord) (MutationResult[SessionConnectionAccepted], error)
	AcceptExport(context.Context, ProjectExportRecord) (MutationResult[ExportAccepted], error)
	ClaimProjectJob(context.Context) (*ClaimedProjectJob, error)
	MarkExportReady(context.Context, string, string) error
	CompleteProjectJob(context.Context, ProjectJobCompletion) error
}

type ProjectSessionConnector interface {
	ConnectProject(context.Context, ClaimedProjectJob) (ProjectSessionConnected, error)
}

type ProcedureExporter interface {
	Verify(string, string, int64) (VerifiedPathSelection, *OverwriteIdentity, error)
	VerifyPrepared(VerifiedPathSelection, string, int64) (*OverwriteIdentity, error)
	Retain(string) error
	Release(string)
	Export(context.Context, ClaimedProjectJob, func(string) error) error
}

var ErrExportPublishUncertain = errors.New("export publish durability is uncertain")

type ProjectExternal struct {
	repository ProjectExternalRepository
	exporter   ProcedureExporter
	now        func() time.Time
	newID      func() string
}

func NewProjectExternal(
	repository ProjectExternalRepository,
	exporter ProcedureExporter,
	now func() time.Time,
	newID func() string,
) *ProjectExternal {
	return &ProjectExternal{repository: repository, exporter: exporter, now: now, newID: newID}
}

type ReconnectSessionInput struct {
	ProjectID        string
	ExpectedRevision int64
	Strategy         string
	OperationID      string
}

type SessionConnectionAccepted struct {
	ProjectID    string
	SessionID    string
	JobID        string
	State        string
	RecoveryMode string
	AcceptedAt   time.Time
}

type ProjectReconnectRecord struct {
	ReconnectSessionInput
	SessionID, JobID, RequestHash string
	Receipt                       MutationReceipt
	Event                         OutboxEvent
}

type VerifiedPathSelection struct {
	AbsolutePath   string `json:"absolutePath"`
	ResolvedPath   string `json:"resolvedPath"`
	VerifiedRootID string `json:"verifiedRootId"`
}

type OverwriteIdentity struct {
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
	Device uint64 `json:"device"`
	Inode  uint64 `json:"inode"`
}

type ExportProcedureInput struct {
	ProcedureID        string
	ProcedureRevision  int64
	Format             string
	Destination        VerifiedPathSelection
	OverwriteConfirmed bool
	OverwriteIdentity  *OverwriteIdentity
	OperationID        string
}

type PrepareExportProcedureInput struct {
	ProcedureID       string
	ProcedureRevision int64
	AbsolutePath      string
}

type PreparedExportProcedure struct {
	Destination            VerifiedPathSelection
	OverwriteIdentity      *OverwriteIdentity
	DestinationDisplayName string
	OverwriteRequired      bool
}

func (u *ProjectExternal) PrepareExportProcedure(
	ctx context.Context,
	input PrepareExportProcedureInput,
) (PreparedExportProcedure, error) {
	if input.ProcedureID == "" || input.ProcedureRevision < 1 || !filepath.IsAbs(input.AbsolutePath) {
		return PreparedExportProcedure{}, fieldError("destination", "出力対象または保存先が不正です")
	}

	exportable, err := u.repository.ProcedureExportable(ctx, input.ProcedureID, input.ProcedureRevision)
	if err != nil {
		return PreparedExportProcedure{}, err
	}

	if !exportable {
		return PreparedExportProcedure{}, &shared.Error{Code: "invalid_state", Message: "完成した手順書が見つかりません"}
	}

	selection, identity, err := u.exporter.Verify(input.AbsolutePath, input.ProcedureID, input.ProcedureRevision)
	if err != nil {
		return PreparedExportProcedure{}, err
	}

	if err := ctx.Err(); err != nil {
		u.exporter.Release(selection.VerifiedRootID)
		return PreparedExportProcedure{}, err
	}

	return PreparedExportProcedure{
		Destination: selection, OverwriteIdentity: identity,
		DestinationDisplayName: filepath.Base(selection.AbsolutePath), OverwriteRequired: identity != nil,
	}, nil
}

type ExportAccepted struct {
	ExportID               string
	JobID                  string
	ProcedureID            string
	Format                 string
	DestinationDisplayName string
	AcceptedAt             time.Time
}

type ProjectExportRecord struct {
	ExportProcedureInput
	ExportID, JobID, RequestHash string
	Receipt                      MutationReceipt
	Event                        OutboxEvent
}

type ClaimedProjectJob struct {
	OperationID            string
	JobID                  string
	Kind                   string
	ProjectID              string
	TargetID               string
	SessionID              string
	Strategy               string
	RecoveryMode           string
	PreviousAgentSessionID string
	Connection             agentconnection.ConnectionInput
	WorkspacePath          string
	ProcedureID            string
	ProcedureRevision      int64
	DocumentJSON           []byte
	Format                 string
	Destination            VerifiedPathSelection
	OverwriteConfirmed     bool
	OverwriteIdentity      *OverwriteIdentity
}

type ProjectSessionConnected struct {
	AgentSessionID    string
	RecoveryMode      string
	ResumeSupported   bool
	LoadSupported     bool
	ProcessGeneration int64
}

type ProjectJobCompletion struct {
	OperationID string
	JobID       string
	Kind        string
	TargetID    string
	Success     bool
	ErrorCode   string
	Session     ProjectSessionConnected
	CompletedAt time.Time
}

func (u *ProjectExternal) ReconnectProjectSession(
	ctx context.Context,
	input ReconnectSessionInput,
) (MutationResult[SessionConnectionAccepted], error) {
	if input.ProjectID == "" || input.ExpectedRevision < 1 || input.OperationID == "" {
		return MutationResult[SessionConnectionAccepted]{}, fieldError("projectId", "再接続対象が不正です")
	}

	if input.Strategy != "auto" && input.Strategy != "new_session" && input.Strategy != "resume_existing" {
		return MutationResult[SessionConnectionAccepted]{}, fieldError("strategy", "再接続方法が不正です")
	}

	now := u.now().UTC()

	return u.repository.AcceptReconnect(ctx, ProjectReconnectRecord{
		ReconnectSessionInput: input,
		SessionID:             u.newID(),
		JobID:                 u.newID(),
		RequestHash: requestDigest(
			"reconnect-v1",
			input.ProjectID,
			strconv.FormatInt(input.ExpectedRevision, 10),
			input.Strategy,
		),
		Receipt: MutationReceipt{OperationID: input.OperationID, CommittedAt: now},
		Event: OutboxEvent{
			ID:            u.newID(),
			Name:          "session.connection_changed",
			AggregateType: "session",
			AggregateID:   input.ProjectID,
			EmittedAt:     now,
			Correlation:   input.OperationID,
		},
	})
}

func (u *ProjectExternal) ExportProcedure(
	ctx context.Context,
	input ExportProcedureInput,
) (MutationResult[ExportAccepted], error) {
	if input.ProcedureID == "" || input.ProcedureRevision < 1 || input.OperationID == "" {
		return MutationResult[ExportAccepted]{}, fieldError("procedureId", "出力対象が不正です")
	}

	if input.Format != "markdown" && input.Format != "pdf" {
		return MutationResult[ExportAccepted]{}, fieldError("format", "出力形式が不正です")
	}

	if !filepath.IsAbs(input.Destination.AbsolutePath) || filepath.Base(input.Destination.AbsolutePath) == "." {
		return MutationResult[ExportAccepted]{}, fieldError("destination", "出力先が不正です")
	}

	extension := ".md"
	if input.Format == "pdf" {
		extension = ".pdf"
	}

	if !strings.EqualFold(filepath.Ext(input.Destination.AbsolutePath), extension) {
		return MutationResult[ExportAccepted]{}, fieldError("destination", "拡張子または出力先が不正です")
	}

	if input.OverwriteConfirmed != (input.OverwriteIdentity != nil) {
		return MutationResult[ExportAccepted]{}, fieldError("overwriteIdentity", "上書き確認情報が不正です")
	}

	if input.OverwriteIdentity != nil &&
		(input.OverwriteIdentity.Size < 0 || len(input.OverwriteIdentity.SHA256) != 64) {
		return MutationResult[ExportAccepted]{}, fieldError("overwriteIdentity", "上書き確認情報が不正です")
	}

	encoded, _ := json.Marshal(struct {
		ProcedureID        string
		ProcedureRevision  int64
		Format             string
		AbsolutePath       string
		OverwriteConfirmed bool
		OverwriteIdentity  *OverwriteIdentity
	}{
		input.ProcedureID, input.ProcedureRevision, input.Format, input.Destination.AbsolutePath,
		input.OverwriteConfirmed, input.OverwriteIdentity,
	})
	requestHash := requestDigest(string(encoded))

	if result, found, err := u.repository.ExistingExport(ctx, input.OperationID, requestHash); err != nil || found {
		return result, err
	}

	selection := input.Destination

	var existing *OverwriteIdentity

	var err error
	if selection.VerifiedRootID != "" {
		existing, err = u.exporter.VerifyPrepared(selection, input.ProcedureID, input.ProcedureRevision)
	} else {
		selection, existing, err = u.exporter.Verify(selection.AbsolutePath, input.ProcedureID, input.ProcedureRevision)
	}

	if err != nil {
		return MutationResult[ExportAccepted]{}, err
	}

	if input.OverwriteConfirmed {
		if existing == nil || existing.Size != input.OverwriteIdentity.Size ||
			existing.Device != input.OverwriteIdentity.Device ||
			existing.Inode != input.OverwriteIdentity.Inode ||
			subtle.ConstantTimeCompare([]byte(existing.SHA256), []byte(input.OverwriteIdentity.SHA256)) != 1 {
			u.exporter.Release(selection.VerifiedRootID)
			return MutationResult[ExportAccepted]{}, fieldError("overwriteIdentity", "上書き対象が変更されました")
		}
	} else if existing != nil {
		u.exporter.Release(selection.VerifiedRootID)
		return MutationResult[ExportAccepted]{}, fieldError("overwriteConfirmed", "既存fileの上書き確認が必要です")
	}

	input.Destination = selection
	if err := u.exporter.Retain(selection.VerifiedRootID); err != nil {
		return MutationResult[ExportAccepted]{}, err
	}

	now := u.now().UTC()
	exportID := u.newID()

	result, err := u.repository.AcceptExport(ctx, ProjectExportRecord{
		ExportProcedureInput: input,
		ExportID:             exportID,
		JobID:                u.newID(),
		RequestHash:          requestHash,
		Receipt:              MutationReceipt{OperationID: input.OperationID, CommittedAt: now},
		Event: OutboxEvent{
			ID:            u.newID(),
			Name:          "export.updated",
			AggregateType: "export",
			AggregateID:   exportID,
			EmittedAt:     now,
			Correlation:   input.OperationID,
		},
	})
	if err != nil || result.Data.ExportID != exportID {
		u.exporter.Release(selection.VerifiedRootID)
	}

	return result, err
}

func RunProjectJob(
	ctx context.Context,
	repository ProjectExternalRepository,
	connector ProjectSessionConnector,
	exporter ProcedureExporter,
	now func() time.Time,
) (bool, error) {
	job, err := repository.ClaimProjectJob(ctx)
	if err != nil || job == nil {
		return false, err
	}

	trace.Record(ctx, trace.Entry{
		Phase: "worker_claim", OperationID: job.OperationID, JobID: job.JobID,
		AggregateID: job.TargetID, Status: "succeeded",
	})

	completion := ProjectJobCompletion{
		OperationID: job.OperationID, JobID: job.JobID, Kind: job.Kind, TargetID: job.TargetID,
	}
	if job.Kind == "reconnect" {
		completion.Session.RecoveryMode = job.RecoveryMode
	}

	switch job.Kind {
	case "reconnect":
		trace.Record(ctx, trace.Entry{
			Phase: "external_io_start", Method: "ReconnectProjectSession",
			OperationID: job.OperationID, JobID: job.JobID, AggregateID: job.TargetID,
		})

		connected, connectErr := connector.ConnectProject(ctx, *job)

		trace.Record(ctx, trace.Entry{
			Phase: "external_io_end", Method: "ReconnectProjectSession",
			OperationID: job.OperationID, JobID: job.JobID, AggregateID: job.TargetID,
			Status: traceStatus(connectErr), ProcessGeneration: connected.ProcessGeneration,
		})

		err = connectErr
		if err == nil {
			completion.Session = connected
		}
	case "export":
		trace.Record(ctx, trace.Entry{
			Phase: "external_io_start", Method: "ExportProcedure",
			OperationID: job.OperationID, JobID: job.JobID, AggregateID: job.TargetID,
		})

		err = exporter.Export(ctx, *job, func(hash string) error {
			return repository.MarkExportReady(ctx, job.TargetID, hash)
		})

		trace.Record(ctx, trace.Entry{
			Phase: "external_io_end", Method: "ExportProcedure",
			OperationID: job.OperationID, JobID: job.JobID, AggregateID: job.TargetID,
			Status: traceStatus(err),
		})
	default:
		err = fieldError("kind", "job種別が不正です")
	}

	if errors.Is(err, ErrExportPublishUncertain) {
		return true, err
	}

	completion.Success = err == nil

	completion.CompletedAt = now().UTC()
	if err != nil {
		completion.ErrorCode = "external_operation_failed"

		var appErr *shared.Error
		if errors.As(err, &appErr) {
			completion.ErrorCode = appErr.Code
		}
	}

	completionContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()

	if completeErr := repository.CompleteProjectJob(completionContext, completion); completeErr != nil {
		return true, completeErr
	}

	return true, err
}

func traceStatus(err error) string {
	if err != nil {
		return "failed"
	}

	return "succeeded"
}
