package main

import (
	"context"
	"log"
	"log/slog"
	"os"

	core "github.com/shikanon/orag/internal/app"
	"github.com/shikanon/orag/internal/config"
	httpserver "github.com/shikanon/orag/internal/http"
	"github.com/shikanon/orag/internal/platform/logger"
)

type appBuilder func(context.Context, config.Config, *slog.Logger) (*core.App, error)
type serverStarter func(*core.App)

func main() {
	os.Exit(run(
		context.Background(),
		core.New,
		func(app *core.App) {
			httpserver.NewServer(app).Hertz().Spin()
		},
	))
}

func run(ctx context.Context, buildApp appBuilder, startServer serverStarter) int {
	cfg, err := config.Load()
	if err != nil {
		log.Printf("load config: %v", err)
		return 1
	}
	logg := logger.New(cfg.Server.Debug)
	app, err := buildApp(ctx, cfg, logg)
	if err != nil {
		logg.Error("init app failed", "error", err)
		return 1
	}
	defer func() {
		if err := app.Close(); err != nil {
			logg.Error("close app failed", "error", err)
		}
	}()
	logg.Info("starting orag api", "addr", cfg.Server.Addr(), "config", cfg.RedactedEnv())
	startServer(app)
	return 0
}
