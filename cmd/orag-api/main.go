package main

import (
	"context"
	"log"

	core "github.com/shikanon/orag/internal/app"
	"github.com/shikanon/orag/internal/config"
	httpserver "github.com/shikanon/orag/internal/http"
	"github.com/shikanon/orag/internal/platform/logger"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	logg := logger.New(cfg.Server.Debug)
	app, err := core.New(context.Background(), cfg, logg)
	if err != nil {
		logg.Error("init app failed", "error", err)
		return
	}
	defer func() {
		if err := app.Close(); err != nil {
			logg.Error("close app failed", "error", err)
		}
	}()
	logg.Info("starting orag api", "addr", cfg.Server.Addr(), "config", cfg.RedactedEnv())
	httpserver.NewServer(app).Hertz().Spin()
}
