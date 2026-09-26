package bootstrap

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"
	appsqlite "github.com/yukihito-jokyu/TEJUN/internal/adapter/sqlite"
	appwails "github.com/yukihito-jokyu/TEJUN/internal/adapter/wails"
	_ "github.com/yukihito-jokyu/TEJUN/internal/adapter/wails/events"
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

	serviceOptions := application.ServiceOptions{MarshalError: appwails.MarshalError}
	app := application.New(application.Options{
		Name: "TEJUN",
		Services: []application.Service{
			application.NewServiceWithOptions(&appwails.StartupService{}, serviceOptions),
			application.NewServiceWithOptions(&appwails.ProjectService{}, serviceOptions),
			application.NewServiceWithOptions(&appwails.PreparationService{}, serviceOptions),
			application.NewServiceWithOptions(&appwails.ExecutionService{}, serviceOptions),
			application.NewServiceWithOptions(&appwails.ProcedureService{}, serviceOptions),
			application.NewServiceWithOptions(&appwails.AgentControlService{}, serviceOptions),
		},
		Assets: application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
		Mac:    application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: true},
	})
	app.Window.NewWithOptions(application.WebviewWindowOptions{Title: "TEJUN", Width: 1200, Height: 800, URL: "/"})

	return app.Run()
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
