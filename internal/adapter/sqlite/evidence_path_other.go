//go:build !darwin && !linux

package sqlite

import (
	"errors"
	"os"
)

var errEvidencePathUnsupported = errors.New("このplatformでは証跡pathの安全性を保証できません")

func SupportsEvidencePath() bool { return false }

func (s *EvidenceStore) mkdirAll(string) error            { return errEvidencePathUnsupported }
func (s *EvidenceStore) openDir(string) (*os.File, error) { return nil, errEvidencePathUnsupported }
func (s *EvidenceStore) openFile(string, int, os.FileMode) (*os.File, error) {
	return nil, errEvidencePathUnsupported
}
func (s *EvidenceStore) link(string, string) error { return errEvidencePathUnsupported }
func (s *EvidenceStore) remove(string) error       { return errEvidencePathUnsupported }
