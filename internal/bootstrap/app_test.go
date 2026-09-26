package bootstrap

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	appsqlite "github.com/yukihito-jokyu/TEJUN/internal/adapter/sqlite"
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

func TestRunEventDispatcherMarksOnlyDeliveredEvents(t *testing.T) {
	tests := []struct {
		name       string
		cancelled  bool
		dispatched bool
	}{
		{name: "delivered", dispatched: true},
		{name: "cancelled", cancelled: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())

			db, err := appsqlite.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
			if err != nil {
				t.Fatal(err)
			}

			t.Cleanup(func() { _ = db.Close() })

			_, err = db.ExecContext(ctx, `INSERT INTO event_outbox
(event_id,name,emitted_at,aggregate_type,aggregate_id,change_sequence,stream_key,stream_revision,correlation,payload_json)
VALUES ('event-1','agent.authentication.updated','2026-09-26T00:00:00Z','agent_job','job-1',1,'agent_job:job-1',1,'{"jobId":"job-1"}','{"authState":"succeeded"}')`)
			if err != nil {
				t.Fatal(err)
			}

			emitted := make(chan struct{}, 1)

			var workers sync.WaitGroup

			workers.Add(1)
			go runEventDispatcher(ctx, &workers, appsqlite.NewAgentConnectionRepository(db),
				func(string, ...any) bool {
					select {
					case emitted <- struct{}{}:
					default:
					}

					return tt.cancelled
				}, time.Now)

			t.Cleanup(func() {
				cancel()
				workers.Wait()
			})

			select {
			case <-emitted:
			case <-time.After(time.Second):
				t.Fatal("event was not emitted")
			}

			if tt.cancelled {
				cancel()
				workers.Wait()
			} else {
				deadline := time.Now().Add(time.Second)

				for {
					var dispatchedAt sql.NullString
					if err := db.QueryRow(`SELECT dispatched_at FROM event_outbox WHERE event_id='event-1'`).
						Scan(&dispatchedAt); err != nil {
						t.Fatal(err)
					}

					if dispatchedAt.Valid || time.Now().After(deadline) {
						break
					}

					time.Sleep(time.Millisecond)
				}
			}

			var dispatchedAt sql.NullString
			if err := db.QueryRow(`SELECT dispatched_at FROM event_outbox WHERE event_id='event-1'`).
				Scan(&dispatchedAt); err != nil {
				t.Fatal(err)
			}

			if dispatchedAt.Valid != tt.dispatched {
				t.Fatalf("dispatched=%v, want %v", dispatchedAt.Valid, tt.dispatched)
			}
		})
	}
}
