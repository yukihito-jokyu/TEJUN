package sqlite

import (
	"context"
	"database/sql"
	"testing"
)

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

			if version != 5 {
				t.Fatalf("version=%d, want 5", version)
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
