package bootstrap

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	appacp "github.com/yukihito-jokyu/TEJUN/internal/adapter/acp"
	appexport "github.com/yukihito-jokyu/TEJUN/internal/adapter/export"
	appsqlite "github.com/yukihito-jokyu/TEJUN/internal/adapter/sqlite"
	appwails "github.com/yukihito-jokyu/TEJUN/internal/adapter/wails"
	appevents "github.com/yukihito-jokyu/TEJUN/internal/adapter/wails/events"
	appworkspace "github.com/yukihito-jokyu/TEJUN/internal/adapter/workspace"
	appusecase "github.com/yukihito-jokyu/TEJUN/internal/application"
	"github.com/yukihito-jokyu/TEJUN/internal/trace"
)

func Run(assets fs.FS) (runErr error) {
	dataDir, err := dataDirectory()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return err
	}

	traceWriter, err := trace.Open(dataDir)
	if err != nil {
		return err
	}

	defer func() { runErr = errors.Join(runErr, traceWriter.Close()) }()

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

	projectRepository := appsqlite.NewProjectRepository(db)

	projectExternalRepository := appsqlite.NewProjectExternalRepository(db)

	exporter := appexport.NewExporter()

	if appsqlite.SupportsEvidencePath() {
		evidence, err := appsqlite.NewEvidenceStore(db, dataDir)
		if err != nil {
			return err
		}

		defer func() { runErr = errors.Join(runErr, evidence.Close()) }()

		projectRepository.SetEvidenceStore(evidence)

		if err := evidence.Reconcile(context.Background()); err != nil {
			return err
		}

		exporter.SetEvidenceReader(func(procedureID, evidenceID string) ([]byte, error) {
			return evidence.ReadForProcedure(context.Background(), procedureID, evidenceID)
		})
	}

	if err := projectExternalRepository.FailInterruptedProjectJobs(context.Background(), now()); err != nil {
		return err
	}

	defer func() { runErr = errors.Join(runErr, exporter.Close()) }()

	manager := appacp.NewManager(repository, now, newID)
	startup := appusecase.NewStartup(repository, appacp.NewCodexScanner(dataDir, now), manager, now, newID)
	agentControl := appusecase.NewAgentControl(repository, manager, manager, now, newID)
	projects := appusecase.NewProjectUseCases(projectRepository, appworkspace.Validator{}, now, newID)
	projectExternal := appusecase.NewProjectExternal(projectExternalRepository, exporter, now, newID)
	projectService := appwails.NewProjectService(projects, projectExternal)
	projectService.SetTrace(traceWriter)

	serviceOptions := application.ServiceOptions{MarshalError: appwails.MarshalError}
	app := application.New(application.Options{
		Name: "TEJUN",
		Services: []application.Service{
			application.NewServiceWithOptions(appwails.NewStartupService(startup), serviceOptions),
			application.NewServiceWithOptions(projectService, serviceOptions),
			application.NewServiceWithOptions(&appwails.PreparationService{}, serviceOptions),
			application.NewServiceWithOptions(&appwails.ExecutionService{}, serviceOptions),
			application.NewServiceWithOptions(&appwails.ProcedureService{}, serviceOptions),
			application.NewServiceWithOptions(appwails.NewAgentControlService(agentControl), serviceOptions),
		},
		Assets: application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
		Mac:    application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: true},
	})
	workerContext, cancelWorkers := context.WithCancel(trace.WithWriter(context.Background(), traceWriter))

	var workers sync.WaitGroup

	workers.Add(3)
	go runAgentWorker(workerContext, &workers, repository, manager, now)
	go runProjectWorker(workerContext, &workers, projectExternalRepository, manager, exporter, now)
	go runEventDispatcher(workerContext, &workers, repository, app.Event.Emit, now)

	app.OnShutdown(func() {
		cancelWorkers()

		_ = manager.Close()

		workers.Wait()
	})
	app.Window.NewWithOptions(application.WebviewWindowOptions{Title: "TEJUN", Width: 1200, Height: 800, URL: "/"})

	return app.Run()
}

func runProjectWorker(
	ctx context.Context,
	wg *sync.WaitGroup,
	repository *appsqlite.ProjectExternalRepository,
	manager *appacp.Manager,
	exporter *appexport.Exporter,
	now func() time.Time,
) {
	defer wg.Done()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		_, _ = appusecase.RunProjectJob(ctx, repository, manager, exporter, now)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
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
	emit func(string, ...any) bool,
	now func() time.Time,
) {
	defer wg.Done()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		events, err := repository.PendingEvents(ctx)
		if err == nil {
			for _, event := range events {
				var (
					correlation appevents.Correlation
					payload     map[string]any
				)
				if json.Unmarshal([]byte(event.Correlation), &correlation) != nil ||
					json.Unmarshal([]byte(event.Payload), &payload) != nil ||
					payload == nil {
					rejectEvent(ctx, repository, event, now)
					continue
				}

				envelope := appevents.AppEvent{
					EventID: event.ID, Name: event.Name, EmittedAt: event.EmittedAt,
					AggregateType: event.AggregateType, AggregateID: event.AggregateID,
					ChangeSequence: event.ChangeSequence, StreamKey: event.StreamKey,
					StreamRevision: event.StreamRevision, Correlation: correlation,
					Payload: payload,
				}
				if appevents.Validate(envelope) != nil {
					rejectEvent(ctx, repository, event, now)
					continue
				}

				if !emit(appevents.Name, envelope) {
					trace.Record(ctx, trace.Entry{
						Phase: "event_dispatch", OperationID: event.OperationID, EventID: event.ID,
						AggregateID: event.AggregateID, ChangeSequence: event.ChangeSequence,
						Status: "succeeded",
					})

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

func rejectEvent(
	ctx context.Context,
	repository *appsqlite.AgentConnectionRepository,
	event appsqlite.PendingEvent,
	now func() time.Time,
) {
	digest := sha256.Sum256([]byte(event.Payload))
	slog.Warn("不正なAppEvent", "causeId", newID(), "payloadDigest", hex.EncodeToString(digest[:]))

	_ = repository.MarkEventDispatched(ctx, event.ID, now())
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
