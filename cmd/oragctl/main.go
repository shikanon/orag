package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	core "github.com/shikanon/orag/internal/app"
	"github.com/shikanon/orag/internal/config"
	evalpkg "github.com/shikanon/orag/internal/eval"
	"github.com/shikanon/orag/internal/platform/logger"
	"github.com/shikanon/orag/internal/rag"
	"github.com/shikanon/orag/internal/storage/postgres"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		return
	}
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	switch os.Args[1] {
	case "migrate":
		if err := migrateCmd(cfg); err != nil {
			log.Fatalf("migrate: %v", err)
		}
		fmt.Println("migrations completed")
	case "eval":
		app := mustApp(cfg)
		defer func() {
			if err := app.Close(); err != nil {
				log.Printf("close app: %v", err)
			}
		}()
		evalCmd(app, os.Args[2:])
	case "token":
		app := mustApp(cfg)
		defer func() {
			if err := app.Close(); err != nil {
				log.Printf("close app: %v", err)
			}
		}()
		fmt.Println(app.BootstrapToken())
	default:
		usage()
	}
}

func migrateCmd(cfg config.Config) error {
	pool, err := postgres.Open(context.Background(), cfg.Database.URL)
	if err != nil {
		return err
	}
	defer pool.Close()
	return postgres.Migrate(context.Background(), pool, "migrations")
}

func mustApp(cfg config.Config) *core.App {
	app, err := core.New(context.Background(), cfg, logger.New(cfg.Server.Debug))
	if err != nil {
		log.Fatalf("init app: %v", err)
	}
	return app
}

func evalCmd(app *core.App, args []string) {
	fs := flag.NewFlagSet("eval", flag.ExitOnError)
	datasetID := fs.String("dataset", "", "dataset id")
	kbID := fs.String("knowledge-base", "kb_default", "knowledge base id")
	profile := fs.String("profile", "realtime", "rag profile")
	_ = fs.Parse(args)
	result, err := app.Eval.Run(context.Background(), evalpkg.RunRequest{
		TenantID:        "tenant_default",
		DatasetID:       *datasetID,
		KnowledgeBaseID: *kbID,
		Profile:         rag.Profile(*profile),
	})
	if err != nil {
		log.Fatalf("eval: %v", err)
		return
	}
	body, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		log.Fatalf("marshal eval result: %v", err)
	}
	fmt.Println(string(body))
}

func usage() {
	fmt.Println("usage: oragctl [migrate|eval|token]")
}
