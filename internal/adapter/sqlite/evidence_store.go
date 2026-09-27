package sqlite

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/yukihito-jokyu/TEJUN/internal/application"
)

const (
	maxEvidenceBytes  = 25 << 20
	maxEvidencePixels = 100_000_000
)

type EvidenceFiles struct{ root *os.Root }

func OpenEvidenceFiles(path string) (*EvidenceFiles, error) {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return nil, err
	}

	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, err
	}

	if err := root.MkdirAll("staging", 0o700); err != nil {
		_ = root.Close()
		return nil, err
	}

	if err := root.MkdirAll("blobs/sha256", 0o700); err != nil {
		_ = root.Close()
		return nil, err
	}

	return &EvidenceFiles{root: root}, nil
}

func (f *EvidenceFiles) Close() error { return f.root.Close() }

func (f *EvidenceFiles) Stage(ctx context.Context, sourcePath string) (application.StagedEvidence, error) {
	if !filepath.IsAbs(sourcePath) || filepath.Clean(sourcePath) != sourcePath {
		return application.StagedEvidence{}, errors.New("image path must be absolute and clean")
	}

	info, err := os.Lstat(sourcePath)
	if err != nil {
		return application.StagedEvidence{}, err
	}

	if !info.Mode().IsRegular() || info.Size() > maxEvidenceBytes {
		return application.StagedEvidence{}, errors.New("image source must be a regular file within size limit")
	}

	fd, err := syscall.Open(sourcePath, syscall.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return application.StagedEvidence{}, err
	}

	source := os.NewFile(uintptr(fd), sourcePath)
	defer func() { _ = source.Close() }()

	info, err = source.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxEvidenceBytes {
		return application.StagedEvidence{}, errors.New("image source changed or is too large")
	}

	config, format, err := image.DecodeConfig(source)
	if err != nil || config.Width <= 0 || config.Height <= 0 ||
		int64(config.Width)*int64(config.Height) > maxEvidencePixels {
		return application.StagedEvidence{}, errors.New("image has invalid dimensions")
	}

	mime := map[string]string{"png": "image/png", "jpeg": "image/jpeg"}[format]
	if mime == "" {
		return application.StagedEvidence{}, errors.New("unsupported image MIME")
	}

	if _, err := source.Seek(0, io.SeekStart); err != nil {
		return application.StagedEvidence{}, err
	}

	if _, _, err := image.Decode(
		&contextReader{ctx: ctx, reader: io.LimitReader(source, maxEvidenceBytes+1)},
	); err != nil {
		return application.StagedEvidence{}, errors.New("image data is corrupt")
	}

	if _, err := source.Seek(0, io.SeekStart); err != nil {
		return application.StagedEvidence{}, err
	}

	stagingName := "staging/" + rand.Text()

	staging, err := f.root.OpenFile(stagingName, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return application.StagedEvidence{}, err
	}

	defer func() { _ = staging.Close() }()

	hash := sha256.New()

	size, err := io.Copy(
		io.MultiWriter(staging, hash),
		&contextReader{ctx: ctx, reader: io.LimitReader(source, maxEvidenceBytes+1)},
	)
	if err != nil || size > maxEvidenceBytes {
		_ = f.root.Remove(stagingName)
		return application.StagedEvidence{}, errors.New("image copy failed or exceeded size limit")
	}

	if err := staging.Sync(); err != nil {
		_ = f.root.Remove(stagingName)
		return application.StagedEvidence{}, err
	}

	if err := syncRootDirectory(f.root, "staging"); err != nil {
		return application.StagedEvidence{}, err
	}

	return application.StagedEvidence{
		Hash: hex.EncodeToString(hash.Sum(nil)), StagingName: stagingName,
		MIME: mime, Size: size,
	}, nil
}

func (f *EvidenceFiles) Commit(ctx context.Context, staged application.StagedEvidence) error {
	if !validStaged(staged) {
		return errors.New("invalid staged image")
	}

	valid, err := f.Verify(ctx, staged)
	if err != nil {
		return err
	}

	if valid {
		_ = f.root.Remove(staged.StagingName)
		return nil
	}

	file, err := f.root.Open(staged.StagingName)
	if err != nil {
		return err
	}

	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != staged.Size {
		return errors.New("staged image is invalid")
	}

	hash := sha256.New()
	if _, err := io.Copy(hash, &contextReader{ctx: ctx, reader: file}); err != nil {
		return err
	}

	if hex.EncodeToString(hash.Sum(nil)) != staged.Hash {
		return errors.New("staged image hash mismatch")
	}

	if err := f.root.MkdirAll("blobs/sha256/"+staged.Hash[:2], 0o700); err != nil {
		return err
	}

	if err := f.root.Rename(staged.StagingName, blobPath(staged.Hash)); err != nil {
		return err
	}

	blob, err := f.root.Open(blobPath(staged.Hash))
	if err != nil {
		return err
	}

	if err := blob.Sync(); err != nil {
		_ = blob.Close()
		return err
	}

	_ = blob.Close()

	return syncRootDirectory(f.root, "blobs/sha256/"+staged.Hash[:2])
}

func (f *EvidenceFiles) Verify(ctx context.Context, staged application.StagedEvidence) (bool, error) {
	if !validStaged(staged) {
		return false, errors.New("invalid staged image")
	}

	file, err := f.root.Open(blobPath(staged.Hash))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}

	if err != nil {
		return false, err
	}

	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != staged.Size {
		return false, nil
	}

	hash := sha256.New()
	if _, err := io.Copy(hash, &contextReader{ctx: ctx, reader: file}); err != nil {
		return false, err
	}

	return hex.EncodeToString(hash.Sum(nil)) == staged.Hash, nil
}

func blobPath(hash string) string { return "blobs/sha256/" + hash[:2] + "/" + hash }

func validStaged(staged application.StagedEvidence) bool {
	if len(staged.Hash) != 64 || staged.Size <= 0 || staged.Size > maxEvidenceBytes ||
		!strings.HasPrefix(staged.StagingName, "staging/") ||
		filepath.Base(staged.StagingName) != strings.TrimPrefix(staged.StagingName, "staging/") {
		return false
	}

	_, err := hex.DecodeString(staged.Hash)

	return err == nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func syncRootDirectory(root *os.Root, name string) error {
	directory, err := root.Open(name)
	if err != nil {
		return err
	}
	defer func() { _ = directory.Close() }()

	return directory.Sync()
}

func (r *contextReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}

	return r.reader.Read(buffer)
}
