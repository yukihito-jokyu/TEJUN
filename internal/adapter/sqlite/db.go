package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

//go:embed migration/*.sql
var migrations embed.FS

func Open(ctx context.Context, path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dataSourceName(path))
	if err != nil {
		return nil, err
	}

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}

	migrationFS, err := fs.Sub(migrations, "migration")
	if err != nil {
		_ = db.Close()

		return nil, err
	}

	provider, err := goose.NewProvider(
		goose.DialectSQLite3,
		db,
		migrationFS,
		goose.WithLogger(goose.NopLogger()),
	)
	if err != nil {
		_ = db.Close()

		return nil, err
	}

	if _, err := provider.Up(ctx); err != nil {
		_ = db.Close()

		return nil, fmt.Errorf("migrationに失敗しました: %w", err)
	}

	return db, nil
}

func dataSourceName(path string) string {
	fileURL := (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()

	return fileURL + "?" + strings.Join([]string{
		"_pragma=foreign_keys(1)",
		"_pragma=journal_mode(WAL)",
		"_pragma=synchronous(FULL)",
		"_pragma=busy_timeout(5000)",
	}, "&")
}
