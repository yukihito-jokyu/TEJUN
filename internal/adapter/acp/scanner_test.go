package acp

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestScanner(t *testing.T) {
	now := time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		candidates []CandidateSpec
		want       int
	}{
		{
			name:       "found executable",
			candidates: []CandidateSpec{{Key: "test", DisplayName: "Test", Command: os.Args[0]}},
			want:       1,
		},
		{
			name:       "missing executable",
			candidates: []CandidateSpec{{Key: "missing", DisplayName: "Missing", Command: "tejun-definitely-missing"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := NewScanner(tt.candidates, func() time.Time { return now }).List(context.Background(), false)
			if err != nil {
				t.Fatal(err)
			}

			if len(result.Items) != tt.want || !result.ScannedAt.Equal(now) {
				t.Fatalf("got %+v", result)
			}
		})
	}
}

func TestCodexScannerUsesInstalledPackage(t *testing.T) {
	dataDir := t.TempDir()

	entrypoint := filepath.Join(
		dataDir,
		"agents",
		"codex-acp",
		codexACPVersion,
		"node_modules",
		"@agentclientprotocol",
		"codex-acp",
		"dist",
		"index.js",
	)
	if err := os.MkdirAll(filepath.Dir(entrypoint), 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(entrypoint, []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := NewCodexScanner(dataDir, time.Now).List(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Items) != 1 || len(result.Items[0].Args) != 1 || result.Items[0].Args[0] != entrypoint {
		t.Fatalf("got %+v", result.Items)
	}
}
