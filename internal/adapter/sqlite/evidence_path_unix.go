//go:build darwin || linux

package sqlite

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

func SupportsEvidencePath() bool { return true }

func evidenceParts(name string) ([]string, error) {
	if name == "" || filepath.IsAbs(name) || filepath.Clean(name) != name {
		return nil, errors.New("証跡pathが不正です")
	}

	parts := strings.Split(name, string(filepath.Separator))
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return nil, errors.New("証跡pathが不正です")
		}
	}

	return parts, nil
}

func (s *EvidenceStore) directory(name string, create bool) (*os.File, error) {
	var parts []string

	if name != "." {
		var err error

		parts, err = evidenceParts(name)
		if err != nil {
			return nil, err
		}
	}

	dir, err := s.root.Open(".")
	if err != nil {
		return nil, err
	}

	for _, part := range parts {
		if create {
			if err := unix.Mkdirat(int(dir.Fd()), part, 0o700); err != nil && !errors.Is(err, unix.EEXIST) {
				_ = dir.Close()
				return nil, err
			}
		}

		fd, err := unix.Openat(int(dir.Fd()), part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		_ = dir.Close()

		if err != nil {
			return nil, err
		}

		dir = os.NewFile(uintptr(fd), part)
	}

	return dir, nil
}

func (s *EvidenceStore) openDir(name string) (*os.File, error) { return s.directory(name, false) }

func (s *EvidenceStore) mkdirAll(name string) error {
	dir, err := s.directory(name, true)
	if err != nil {
		return err
	}

	return dir.Close()
}

func (s *EvidenceStore) parent(name string) (*os.File, string, error) {
	parts, err := evidenceParts(name)
	if err != nil {
		return nil, "", err
	}

	dir, err := s.directory(filepath.Dir(name), false)

	return dir, parts[len(parts)-1], err
}

func (s *EvidenceStore) openFile(name string, flags int, perm os.FileMode) (*os.File, error) {
	dir, base, err := s.parent(name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = dir.Close() }()

	fd, err := unix.Openat(
		int(dir.Fd()),
		base,
		flags|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK,
		uint32(perm.Perm()),
	)
	if err != nil {
		return nil, err
	}

	return os.NewFile(uintptr(fd), name), nil
}

func (s *EvidenceStore) link(source, target string) error {
	from, fromBase, err := s.parent(source)
	if err != nil {
		return err
	}
	defer func() { _ = from.Close() }()

	to, toBase, err := s.parent(target)
	if err != nil {
		return err
	}

	defer func() { _ = to.Close() }()

	return unix.Linkat(int(from.Fd()), fromBase, int(to.Fd()), toBase, 0)
}

func (s *EvidenceStore) remove(name string) error {
	dir, base, err := s.parent(name)
	if err != nil {
		return err
	}
	defer func() { _ = dir.Close() }()

	return unix.Unlinkat(int(dir.Fd()), base, 0)
}
