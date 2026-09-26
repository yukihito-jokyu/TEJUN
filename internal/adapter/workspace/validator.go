package workspace

import (
	"errors"
	"os"
	"path/filepath"
)

var ErrInvalid = errors.New("作業場所を開けません")

type Validator struct{}

func (Validator) Validate(path string) error { return Validate(path) }

// Validateは選択されたdirectoryをrootとして開けることを確認する。
func Validate(path string) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return ErrInvalid
	}

	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() {
		return ErrInvalid
	}

	root, err := os.OpenRoot(path)
	if err != nil {
		return ErrInvalid
	}
	defer func() { _ = root.Close() }()

	return nil
}
