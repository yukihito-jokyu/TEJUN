package bootstrap

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	appacp "github.com/yukihito-jokyu/TEJUN/internal/adapter/acp"
	appsqlite "github.com/yukihito-jokyu/TEJUN/internal/adapter/sqlite"
	appwails "github.com/yukihito-jokyu/TEJUN/internal/adapter/wails"
	appevents "github.com/yukihito-jokyu/TEJUN/internal/adapter/wails/events"
	appusecase "github.com/yukihito-jokyu/TEJUN/internal/application"
)

func Run(assets fs.FS) (runErr error) {
	dataDir, err := dataDirectory()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return err
	}

	db, err := appsqlite.Open(context.Background(), filepath.Join(dataDir, "tejun.db"))
	if err != nil {
		return err
	}
	defer func() { runErr = errors.Join(runErr, db.Close()) }()

	repository := appsqlite.NewAgentConnectionRepository(db)

	now := func() time.Time { return time.Now().UTC() }
	if err := repository.FailInterruptedAgentJobs(context.Background(), now()); err != nil {
		return err
	}

	if err := repository.FailInterruptedElicitations(context.Background(), now()); err != nil {
		return err
	}

	manager := appacp.NewManager(repository, now, newID)
	startup := appusecase.NewStartup(repository, appacp.NewCodexScanner(dataDir, now), manager, now, newID)
	agentControl := appusecase.NewAgentControl(repository, manager, manager, now, newID)

	serviceOptions := application.ServiceOptions{MarshalError: appwails.MarshalError}
	app := application.New(application.Options{
		Name: "TEJUN",
		Services: []application.Service{
			application.NewServiceWithOptions(appwails.NewStartupService(startup), serviceOptions),
			application.NewServiceWithOptions(&appwails.ProjectService{}, serviceOptions),
			application.NewServiceWithOptions(&appwails.PreparationService{}, serviceOptions),
			application.NewServiceWithOptions(&appwails.ExecutionService{}, serviceOptions),
			application.NewServiceWithOptions(&appwails.ProcedureService{}, serviceOptions),
			application.NewServiceWithOptions(appwails.NewAgentControlService(agentControl), serviceOptions),
		},
		Assets: application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
		Mac:    application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: true},
	})
	workerContext, cancelWorkers := context.WithCancel(context.Background())

	var workers sync.WaitGroup

	workers.Add(2)
	go runAgentWorker(workerContext, &workers, repository, manager, now)
	go runEventDispatcher(workerContext, &workers, repository, app, now)

	app.OnShutdown(func() {
		cancelWorkers()

		_ = manager.Close()

		workers.Wait()
	})
	app.Window.NewWithOptions(application.WebviewWindowOptions{Title: "TEJUN", Width: 1200, Height: 800, URL: "/"})

	return app.Run()
}

func runAgentWorker(
	ctx context.Context,
	wg *sync.WaitGroup,
	repository *appsqlite.AgentConnectionRepository,
	manager *appacp.Manager,
	now func() time.Time,
) {
	defer wg.Done()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		_, _ = appusecase.RunAgentJob(ctx, repository, manager, now)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func runEventDispatcher(
	ctx context.Context,
	wg *sync.WaitGroup,
	repository *appsqlite.AgentConnectionRepository,
	app *application.App,
	now func() time.Time,
) {
	defer wg.Done()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		events, err := repository.PendingEvents(ctx)
		if err == nil {
			for _, event := range events {
				envelope := appevents.AppEvent{
					EventID: event.ID, Name: event.Name, EmittedAt: event.EmittedAt,
					AggregateType: event.AggregateType, AggregateID: event.AggregateID,
					ChangeSequence: event.ChangeSequence, StreamKey: event.StreamKey,
					StreamRevision: event.StreamRevision, Correlation: appevents.Correlation{JobID: event.Correlation},
					Payload: map[string]any{},
				}
				if app.Event.Emit(appevents.Name, envelope) {
					_ = repository.MarkEventDispatched(ctx, event.ID, now())
				}
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func newID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(err)
	}

	return hex.EncodeToString(value[:])
}

func dataDirectory() (string, error) {
	if dataDir := os.Getenv("TEJUN_DATA_DIR"); dataDir != "" {
		return dataDir, nil
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(configDir, "TEJUN"), nil
}
