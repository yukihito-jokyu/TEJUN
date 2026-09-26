package export

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/yukihito-jokyu/TEJUN/internal/application"
	"github.com/yukihito-jokyu/TEJUN/internal/domain/shared"
)

type selection struct {
	root              *os.Root
	path              string
	original          string
	name              string
	procedureID       string
	procedureRevision int64
	expires           time.Time
	timer             *time.Timer
	retained          bool
}

type Exporter struct {
	readEvidence func(string, string) ([]byte, error)
	mu           sync.Mutex
	selections   map[string]selection
	pdfSlot      chan struct{}
	syncRoot     func(*os.Root) error
}

func (e *Exporter) SetEvidenceReader(read func(string, string) ([]byte, error)) {
	e.readEvidence = read
}

func NewExporter() *Exporter {
	return &Exporter{selections: make(map[string]selection), pdfSlot: make(chan struct{}, 1), syncRoot: syncRoot}
}

func (e *Exporter) Verify(
	path, procedureID string,
	procedureRevision int64,
) (application.VerifiedPathSelection, *application.OverwriteIdentity, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return application.VerifiedPathSelection{}, nil, invalidDestination()
	}

	name := filepath.Base(path)
	if name == "." || name == string(filepath.Separator) {
		return application.VerifiedPathSelection{}, nil, invalidDestination()
	}

	parent, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return application.VerifiedPathSelection{}, nil, invalidDestination()
	}

	root, err := os.OpenRoot(parent)
	if err != nil {
		return application.VerifiedPathSelection{}, nil, invalidDestination()
	}

	identity, err := rootIdentity(root, name)
	if err != nil {
		_ = root.Close()
		return application.VerifiedPathSelection{}, nil, err
	}

	var token [32]byte
	if _, err := rand.Read(token[:]); err != nil {
		_ = root.Close()
		return application.VerifiedPathSelection{}, nil, err
	}

	id := hex.EncodeToString(token[:])
	resolved := filepath.Join(parent, name)

	e.mu.Lock()
	e.selections[id] = selection{
		root:              root,
		path:              resolved,
		original:          path,
		name:              name,
		procedureID:       procedureID,
		procedureRevision: procedureRevision,
		expires:           time.Now().Add(5 * time.Minute),
	}
	selected := e.selections[id]
	selected.timer = time.AfterFunc(5*time.Minute, func() { e.expire(id) })
	e.selections[id] = selected
	e.mu.Unlock()

	return application.VerifiedPathSelection{
		AbsolutePath: path, ResolvedPath: resolved, VerifiedRootID: id,
	}, identity, nil
}

func (e *Exporter) VerifyPrepared(
	input application.VerifiedPathSelection,
	procedureID string,
	procedureRevision int64,
) (*application.OverwriteIdentity, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	selected, ok := e.selections[input.VerifiedRootID]

	if !ok || time.Now().After(selected.expires) || selected.path != input.ResolvedPath ||
		filepath.Join(filepath.Dir(selected.path), selected.name) != selected.path ||
		selected.original != input.AbsolutePath || selected.procedureID != procedureID || selected.procedureRevision != procedureRevision {
		return nil, invalidDestination()
	}

	return rootIdentity(selected.root, selected.name)
}

func (e *Exporter) Retain(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	selected, ok := e.selections[id]
	if !ok || time.Now().After(selected.expires) {
		return invalidDestination()
	}

	selected.timer.Stop()
	selected.retained = true
	e.selections[id] = selected

	return nil
}

func (e *Exporter) Export(ctx context.Context, job application.ClaimedProjectJob, markReady func(string) error) error {
	if job.Kind != "export" {
		return errors.New("not an export job")
	}

	e.mu.Lock()
	selected, ok := e.selections[job.Destination.VerifiedRootID]
	delete(e.selections, job.Destination.VerifiedRootID)
	e.mu.Unlock()

	if ok {
		selected.timer.Stop()
	}

	if !ok || selected.path != job.Destination.ResolvedPath {
		return invalidDestination()
	}

	defer func() { _ = selected.root.Close() }()

	if job.Format == "pdf" {
		select {
		case e.pdfSlot <- struct{}{}:
			defer func() { <-e.pdfSlot }()
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return err
	}

	staging := ".tejun-" + hex.EncodeToString(token[:]) + ".tmp"

	file, err := selected.root.OpenFile(staging, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = selected.root.Remove(staging) }()

	hash := sha256.New()
	if err := RenderWithEvidence(
		job.Format,
		job.DocumentJSON,
		job.ProcedureID,
		e.readEvidence,
		io.MultiWriter(file, hash),
	); err != nil {
		_ = file.Close()
		return err
	}

	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}

	if err := file.Close(); err != nil {
		return err
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	if err := markReady(hex.EncodeToString(hash.Sum(nil))); err != nil {
		return err
	}

	if job.OverwriteConfirmed {
		if job.OverwriteIdentity == nil {
			return invalidDestination()
		}

		if err := exchangeChecked(selected.root, staging, selected.name, *job.OverwriteIdentity); err != nil {
			return err
		}
	} else {
		if err := selected.root.Link(staging, selected.name); err != nil {
			return &shared.Error{Code: "export_source_changed", Message: "出力先が変更されました", Retryable: true}
		}
	}

	if err := e.syncRoot(selected.root); err != nil {
		return fmt.Errorf("%w: %v", application.ErrExportPublishUncertain, err)
	}

	return nil
}

func (e *Exporter) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	var err error

	for id, selection := range e.selections {
		selection.timer.Stop()
		err = errors.Join(err, selection.root.Close())

		delete(e.selections, id)
	}

	return err
}

func (e *Exporter) Release(id string) {
	e.mu.Lock()
	selected, ok := e.selections[id]
	delete(e.selections, id)
	e.mu.Unlock()

	if ok {
		selected.timer.Stop()
		_ = selected.root.Close()
	}
}

func (e *Exporter) expire(id string) {
	e.mu.Lock()

	selected, ok := e.selections[id]
	if ok && !selected.retained {
		delete(e.selections, id)
	}
	e.mu.Unlock()

	if ok && !selected.retained {
		_ = selected.root.Close()
	}
}

func rootIdentity(root *os.Root, name string) (*application.OverwriteIdentity, error) {
	info, err := root.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}

	if err != nil || !info.Mode().IsRegular() {
		return nil, invalidDestination()
	}

	return identityForInfo(root, name, info)
}

func identityForInfo(root *os.Root, name string, info os.FileInfo) (*application.OverwriteIdentity, error) {
	file, err := openIdentityNoFollow(root, name)
	if err != nil {
		return nil, invalidDestination()
	}
	defer func() { _ = file.Close() }()

	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return nil, invalidDestination()
	}

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return nil, err
	}

	device, inode := fileID(info)

	return &application.OverwriteIdentity{
		Size:   info.Size(),
		SHA256: hex.EncodeToString(hash.Sum(nil)),
		Device: device,
		Inode:  inode,
	}, nil
}

func syncRoot(root *os.Root) error {
	directory, err := root.Open(".")
	if err != nil {
		return err
	}
	defer func() { _ = directory.Close() }()

	return directory.Sync()
}

func sameIdentity(actual, want *application.OverwriteIdentity) bool {
	return actual != nil && want != nil && actual.Size == want.Size && actual.SHA256 == want.SHA256 &&
		actual.Device == want.Device &&
		actual.Inode == want.Inode
}

func invalidDestination() error {
	return &shared.Error{
		Code:        "validation_error",
		Message:     "出力先を検証できません",
		FieldErrors: map[string]string{"destination": "出力先を検証できません"},
	}
}

var _ application.ProcedureExporter = (*Exporter)(nil)
