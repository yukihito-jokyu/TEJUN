package sqlite

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestEvidenceFiles(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.png")

	source, err := os.Create(sourcePath)
	if err != nil {
		t.Fatal(err)
	}

	picture := image.NewRGBA(image.Rect(0, 0, 1, 1))
	picture.Set(0, 0, color.RGBA{R: 255, A: 255})

	if err := png.Encode(source, picture); err != nil {
		t.Fatal(err)
	}

	if err := source.Close(); err != nil {
		t.Fatal(err)
	}

	linkPath := filepath.Join(dir, "link.png")
	if err := os.Symlink(sourcePath, linkPath); err != nil {
		t.Fatal(err)
	}

	valid, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}

	truncatedPath := filepath.Join(dir, "truncated.png")
	if err := os.WriteFile(truncatedPath, valid[:len(valid)-8], 0o600); err != nil {
		t.Fatal(err)
	}

	store, err := OpenEvidenceFiles(filepath.Join(dir, "cas"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = store.Close() })

	tests := []struct {
		name, path string
		wantError  bool
	}{
		{name: "regular image", path: sourcePath},
		{name: "symlink", path: linkPath, wantError: true},
		{name: "relative path", path: "source.png", wantError: true},
		{name: "truncated image", path: truncatedPath, wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			staged, err := store.Stage(context.Background(), tt.path)
			if (err != nil) != tt.wantError {
				t.Fatalf("Stage() err=%v", err)
			}

			if tt.wantError {
				return
			}

			if err := store.Commit(context.Background(), staged); err != nil {
				t.Fatal(err)
			}

			valid, err := store.Verify(context.Background(), staged)
			if err != nil || !valid {
				t.Fatalf("Verify() valid=%t err=%v", valid, err)
			}
		})
	}
}
