//go:build windows

package sqlite

import (
	"errors"
	"testing"
)

func TestEvidencePathUnsupported(t *testing.T) {
	if SupportsEvidencePath() {
		t.Fatal("Windows evidence path must be unsupported")
	}

	store, err := NewEvidenceStore(nil, t.TempDir())
	if store != nil || !errors.Is(err, errEvidencePathUnsupported) {
		t.Fatalf("store = %v, error = %v", store, err)
	}
}
