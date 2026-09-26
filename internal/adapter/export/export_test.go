package export

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yukihito-jokyu/TEJUN/internal/application"
)

func TestExportPreservesExistingOutput(t *testing.T) {
	document := []byte(`{"title":"新しい手順","steps":[]}`)

	tests := []struct {
		name              string
		initial           string
		changeAfterVerify string
		overwrite         bool
		wantError         bool
		wantContent       string
		markFails         bool
	}{
		{name: "new", wantContent: "# 新しい手順\n\n"},
		{name: "overwrite", initial: "old", overwrite: true, wantContent: "# 新しい手順\n\n"},
		{
			name:              "changed",
			initial:           "old",
			changeAfterVerify: "another",
			overwrite:         true,
			wantError:         true,
			wantContent:       "another",
		},
		{name: "no confirmation", initial: "old", wantError: true, wantContent: "old"},
		{
			name:        "DB prepare fails",
			initial:     "old",
			overwrite:   true,
			markFails:   true,
			wantError:   true,
			wantContent: "old",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()

			path := filepath.Join(dir, "procedure.md")
			if test.initial != "" {
				if err := os.WriteFile(path, []byte(test.initial), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			exporter := NewExporter()
			defer func() { _ = exporter.Close() }()

			selection, identity, err := exporter.Verify(path, "procedure", 1)
			if err != nil {
				t.Fatal(err)
			}

			if test.changeAfterVerify != "" {
				if err := os.WriteFile(path, []byte(test.changeAfterVerify), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			job := application.ClaimedProjectJob{
				Kind:               "export",
				Format:             "markdown",
				DocumentJSON:       document,
				Destination:        selection,
				OverwriteConfirmed: test.overwrite,
				OverwriteIdentity:  identity,
			}

			err = exporter.Export(context.Background(), job, func(string) error {
				if test.markFails {
					return errors.New("DB commit failed")
				}

				return nil
			})
			if (err != nil) != test.wantError {
				t.Fatalf("Export error = %v, wantError %t", err, test.wantError)
			}

			content, readErr := os.ReadFile(path)
			if readErr != nil || string(content) != test.wantContent {
				t.Fatalf("content = %q, err = %v, want %q", content, readErr, test.wantContent)
			}
		})
	}
}

func TestPreparedSelectionValidation(t *testing.T) {
	tests := []struct {
		name        string
		change      func(*application.VerifiedPathSelection)
		procedureID string
		revision    int64
		wantError   bool
	}{
		{name: "same selection", procedureID: "procedure", revision: 1},
		{
			name:        "path changed",
			procedureID: "procedure",
			revision:    1,
			change:      func(s *application.VerifiedPathSelection) { s.AbsolutePath += ".other" },
			wantError:   true,
		},
		{name: "different procedure", procedureID: "other", revision: 1, wantError: true},
		{name: "different revision", procedureID: "procedure", revision: 2, wantError: true},
		{
			name:        "unknown token",
			procedureID: "procedure",
			revision:    1,
			change:      func(s *application.VerifiedPathSelection) { s.VerifiedRootID = "unknown" },
			wantError:   true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			exporter := NewExporter()
			defer func() { _ = exporter.Close() }()

			selection, _, err := exporter.Verify(filepath.Join(t.TempDir(), "procedure.md"), "procedure", 1)
			if err != nil {
				t.Fatal(err)
			}

			if test.change != nil {
				test.change(&selection)
			}

			_, err = exporter.VerifyPrepared(selection, test.procedureID, test.revision)
			if (err != nil) != test.wantError {
				t.Fatalf("VerifyPrepared error = %v, wantError %t", err, test.wantError)
			}
		})
	}
}

func TestRetainedSelectionSurvivesExpiry(t *testing.T) {
	for _, test := range []struct {
		name, retain string
		wantValid    bool
	}{
		{name: "unaccepted expires"},
		{name: "accepted remains", retain: "yes", wantValid: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			exporter := NewExporter()
			defer func() { _ = exporter.Close() }()

			selection, _, err := exporter.Verify(filepath.Join(t.TempDir(), "procedure.md"), "procedure", 1)
			if err != nil {
				t.Fatal(err)
			}

			if test.retain != "" {
				if err := exporter.Retain(selection.VerifiedRootID); err != nil {
					t.Fatal(err)
				}
			}

			exporter.mu.Lock()
			prepared := exporter.selections[selection.VerifiedRootID]
			prepared.expires = time.Now().Add(-time.Minute)
			exporter.selections[selection.VerifiedRootID] = prepared
			exporter.mu.Unlock()
			exporter.expire(selection.VerifiedRootID)

			exporter.mu.Lock()
			_, valid := exporter.selections[selection.VerifiedRootID]
			exporter.mu.Unlock()

			if valid != test.wantValid {
				t.Fatalf("selection exists = %t, want %t", valid, test.wantValid)
			}
		})
	}
}

func TestExportDirectorySyncFailureIsUncertain(t *testing.T) {
	for _, test := range []struct {
		name, initial string
	}{
		{name: "new file"},
		{name: "overwrite", initial: "old output"},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "procedure.md")
			if test.initial != "" {
				if err := os.WriteFile(path, []byte(test.initial), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			exporter := NewExporter()
			defer func() { _ = exporter.Close() }()

			selection, identity, err := exporter.Verify(path, "procedure", 1)
			if err != nil {
				t.Fatal(err)
			}

			if err := exporter.Retain(selection.VerifiedRootID); err != nil {
				t.Fatal(err)
			}

			exporter.syncRoot = func(*os.Root) error { return errors.New("sync failpoint") }

			err = exporter.Export(context.Background(), application.ClaimedProjectJob{
				Kind: "export", Format: "markdown", DocumentJSON: []byte(`{"title":"new","steps":[]}`),
				Destination: selection, OverwriteConfirmed: identity != nil, OverwriteIdentity: identity,
			}, func(string) error { return nil })
			if !errors.Is(err, application.ErrExportPublishUncertain) {
				t.Fatalf("error = %v", err)
			}

			content, err := os.ReadFile(path)
			if err != nil || string(content) != "# new\n\n" {
				t.Fatalf("published content = %q, error = %v", content, err)
			}
		})
	}
}
