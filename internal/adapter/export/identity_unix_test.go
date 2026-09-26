//go:build darwin || linux

package export

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIdentityForInfoRejectsSameInodeSymlinkSwap(t *testing.T) {
	for _, test := range []struct {
		name      string
		swap      bool
		wantError bool
	}{
		{name: "unchanged regular file"},
		{name: "same inode symlink swap", swap: true, wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()

			path := filepath.Join(dir, "procedure.md")
			if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
				t.Fatal(err)
			}

			root, err := os.OpenRoot(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = root.Close() }()

			info, err := root.Lstat("procedure.md")
			if err != nil {
				t.Fatal(err)
			}

			if test.swap {
				alias := filepath.Join(dir, "same-inode.md")
				if err := os.Link(path, alias); err != nil {
					t.Fatal(err)
				}

				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}

				if err := os.Symlink(alias, path); err != nil {
					t.Fatal(err)
				}
			}

			_, err = identityForInfo(root, "procedure.md", info)
			if (err != nil) != test.wantError {
				t.Fatalf("identityForInfo error = %v, wantError %t", err, test.wantError)
			}
		})
	}
}
