package main

import (
	"errors"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestDomainAndApplicationDoNotImportAdapters(t *testing.T) {
	tests := []struct {
		name         string
		root         string
		allowMissing bool
	}{
		{name: "domain", root: "internal/domain"},
		{name: "application", root: "internal/application", allowMissing: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := filepath.WalkDir(tt.root, func(path string, entry fs.DirEntry, walkErr error) error {
				if walkErr != nil || entry.IsDir() || filepath.Ext(path) != ".go" {
					return walkErr
				}

				file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
				if err != nil {
					t.Fatal(err)
				}

				for _, item := range file.Imports {
					name, _ := strconv.Unquote(item.Path.Value)
					if strings.Contains(name, "/internal/adapter/") {
						t.Errorf("%s imports adapter %s", path, name)
					}
				}

				return nil
			})
			if err != nil && (!tt.allowMissing || !errors.Is(err, fs.ErrNotExist)) {
				t.Fatal(err)
			}
		})
	}
}
