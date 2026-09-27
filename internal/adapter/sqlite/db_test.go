package sqlite

import (
	"context"
	"database/sql"
	"io/fs"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
)

func TestDataSourceNameUsesLocalFileURI(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")

	parsed, err := url.Parse(dataSourceName(path))
	if err != nil {
		t.Fatal(err)
	}

	wantPath := filepath.ToSlash(path)
	if volume := filepath.VolumeName(path); len(volume) == 2 && volume[1] == ':' {
		wantPath = "/" + wantPath
	}

	if parsed.Scheme != "file" || parsed.Host != "" || parsed.Path != wantPath {
		t.Fatalf("database URI = %s, want local file path %s", parsed.String(), path)
	}
}

func TestOpenUpgradesPreparationTurnColumns(t *testing.T) {
	ctx := context.Background()
	path := t.TempDir() + "/test.db"

	db, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}

	migrationFS, err := fs.Sub(migrations, "migration")
	if err != nil {
		t.Fatal(err)
	}

	provider, err := goose.NewProvider(goose.DialectSQLite3, db, migrationFS, goose.WithLogger(goose.NopLogger()))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := provider.Down(ctx); err != nil {
		t.Fatal(err)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	rows, err := db.QueryContext(ctx, `PRAGMA table_info(preparation_turns)`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var (
			cid, notNull, primaryKey int
			name, columnType         string
			defaultValue             sql.NullString
		)
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatal(err)
		}

		if name == "plan_revision" {
			return
		}
	}

	t.Fatal("plan_revision column missing after upgrade")
}

func TestCommandEvidenceMigrationUpgradesExistingChecks(t *testing.T) {
	for _, tc := range []struct {
		name, command, want string
	}{
		{name: "command", command: "go version", want: "text_or_image"},
		{name: "no command", command: " ", want: "none"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			repo, _ := executionFixture(t)

			migrationFS, err := fs.Sub(migrations, "migration")
			if err != nil {
				t.Fatal(err)
			}

			provider, err := goose.NewProvider(goose.DialectSQLite3, repo.db, migrationFS,
				goose.WithLogger(goose.NopLogger()))
			if err != nil {
				t.Fatal(err)
			}

			for range 2 {
				if _, err := provider.Down(ctx); err != nil {
					t.Fatal(err)
				}
			}

			for _, table := range []string{"check_items", "execution_checks"} {
				if _, err := repo.db.ExecContext(
					ctx,
					`UPDATE `+table+` SET suggested_command=?,human_evidence_requirement='none'`,
					tc.command,
				); err != nil {
					t.Fatal(err)
				}
			}

			if _, err := provider.Up(ctx); err != nil {
				t.Fatal(err)
			}

			for _, table := range []string{"check_items", "execution_checks"} {
				var requirement string
				if err := repo.db.QueryRowContext(ctx, `SELECT human_evidence_requirement FROM `+table).
					Scan(&requirement); err != nil ||
					requirement != tc.want {
					t.Fatalf("%s requirement=%q err=%v", table, requirement, err)
				}
			}
		})
	}
}

func TestProcedureCommandMigrationRestoresGeneratedDraft(t *testing.T) {
	ctx := context.Background()
	repo, _ := executionFixture(t)

	migrationFS, err := fs.Sub(migrations, "migration")
	if err != nil {
		t.Fatal(err)
	}

	provider, err := goose.NewProvider(goose.DialectSQLite3, repo.db, migrationFS,
		goose.WithLogger(goose.NopLogger()))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := provider.Down(ctx); err != nil {
		t.Fatal(err)
	}

	if _, err := repo.db.ExecContext(ctx, `INSERT INTO procedures
(procedure_id,project_id,revision,status,document_json,created_at,updated_at)
VALUES ('draft','p',1,'draft',?,1,1)`, `{"title":"Draft","steps":[{"stepId":"c","clientKey":"c","title":"Check","description":"Do it","notes":[],"evidenceRefs":[]}]}`); err != nil {
		t.Fatal(err)
	}

	if _, err := repo.db.ExecContext(ctx, `INSERT INTO procedure_sources(procedure_id,execution_id)
VALUES ('draft','e')`); err != nil {
		t.Fatal(err)
	}

	if _, err := provider.Up(ctx); err != nil {
		t.Fatal(err)
	}

	var command string
	if err := repo.db.QueryRowContext(ctx,
		`SELECT json_extract(document_json,'$.steps[0].command') FROM procedures WHERE procedure_id='draft'`).
		Scan(&command); err != nil || command != "go version" {
		t.Fatalf("restored command=%q err=%v", command, err)
	}
}

func TestOpenAppliesMigrationsAndPragmas(t *testing.T) {
	tests := []struct {
		name    string
		reopens bool
	}{
		{name: "fresh database", reopens: false},
		{name: "existing database", reopens: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := t.TempDir() + "/test.db"
			if tt.reopens {
				db, err := Open(context.Background(), path)
				if err != nil {
					t.Fatal(err)
				}

				if err := db.Close(); err != nil {
					t.Fatal(err)
				}
			}

			db, err := Open(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}

			t.Cleanup(func() {
				if err := db.Close(); err != nil {
					t.Error(err)
				}
			})

			var version int
			if err := db.QueryRow("SELECT MAX(version_id) FROM goose_db_version WHERE is_applied = 1").
				Scan(&version); err != nil {
				t.Fatal(err)
			}

			if version != 13 {
				t.Fatalf("version=%d, want 13", version)
			}

			assertPragmasOnTwoConnections(t, db)
			assertForeignKeyConstraint(t, db)
		})
	}
}

func assertPragmasOnTwoConnections(t *testing.T, db *sql.DB) {
	t.Helper()

	connections := make([]*sql.Conn, 0, 2)

	for range 2 {
		conn, err := db.Conn(context.Background())
		if err != nil {
			t.Fatal(err)
		}

		connections = append(connections, conn)

		t.Cleanup(func() {
			if err := conn.Close(); err != nil {
				t.Error(err)
			}
		})
	}

	tests := []struct {
		name string
		want string
	}{
		{name: "foreign_keys", want: "1"},
		{name: "journal_mode", want: "wal"},
		{name: "synchronous", want: "2"},
		{name: "busy_timeout", want: "5000"},
	}

	for connectionIndex, conn := range connections {
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				var got string
				if err := conn.QueryRowContext(context.Background(), "PRAGMA "+tt.name).Scan(&got); err != nil {
					t.Fatal(err)
				}

				if got != tt.want {
					t.Fatalf("connection=%d got %s, want %s", connectionIndex, got, tt.want)
				}
			})
		}
	}
}

func assertForeignKeyConstraint(t *testing.T, db *sql.DB) {
	t.Helper()

	statements := []struct {
		name  string
		query string
	}{
		{name: "parent", query: "CREATE TABLE parent (id INTEGER PRIMARY KEY)"},
		{name: "child", query: "CREATE TABLE child (parent_id INTEGER REFERENCES parent(id))"},
	}

	for _, tt := range statements {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := db.Exec(tt.query); err != nil {
				t.Fatal(err)
			}
		})
	}

	if _, err := db.Exec("INSERT INTO child(parent_id) VALUES (1)"); err == nil {
		t.Fatal("foreign key violation was accepted")
	}
}

func TestOpenUpgradesExistingExecution(t *testing.T) {
	ctx, db, _ := seededPreparation(t)

	migrationFS, err := fs.Sub(migrations, "migration")
	if err != nil {
		t.Fatal(err)
	}

	provider, err := goose.NewProvider(goose.DialectSQLite3, db, migrationFS, goose.WithLogger(goose.NopLogger()))
	if err != nil {
		t.Fatal(err)
	}

	for range 5 {
		if _, err := provider.Down(ctx); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := db.ExecContext(ctx, `INSERT INTO executions(execution_id, project_id, session_id, revision, started_at)
VALUES ('legacy', 'p', 's', 1, '')`); err != nil {
		t.Fatal(err)
	}

	for range 5 {
		if _, err := provider.Up(ctx); err != nil {
			t.Fatal(err)
		}
	}

	var status, startedAt string
	if err := db.QueryRowContext(ctx, `SELECT status, started_at FROM executions WHERE execution_id = 'legacy'`).
		Scan(&status, &startedAt); err != nil {
		t.Fatal(err)
	}

	if status != "active" || startedAt == "" {
		t.Fatalf("status=%q started_at=%q", status, startedAt)
	}
}
