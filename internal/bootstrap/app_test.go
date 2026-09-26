package bootstrap

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDataDirectory(t *testing.T) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}

	override := filepath.Join(t.TempDir(), "e2e")
	tests := []struct {
		name     string
		override string
		want     string
	}{
		{name: "default", override: "", want: filepath.Join(configDir, "TEJUN")},
		{name: "override", override: override, want: override},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TEJUN_DATA_DIR", tt.override)

			got, err := dataDirectory()
			if err != nil {
				t.Fatal(err)
			}

			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}
