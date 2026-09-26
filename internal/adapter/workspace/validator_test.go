package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidate(t *testing.T) {
	workspace := t.TempDir()

	file := filepath.Join(workspace, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(workspace, link); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		path      string
		wantError bool
	}{
		{name: "directory", path: workspace},
		{name: "relative", path: "relative", wantError: true},
		{name: "file", path: file, wantError: true},
		{name: "symlink", path: link, wantError: true},
		{name: "missing", path: filepath.Join(workspace, "missing"), wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := Validate(test.path); (err != nil) != test.wantError {
				t.Fatalf("Validate(%q) = %v, wantError %t", test.path, err, test.wantError)
			}
		})
	}
}
