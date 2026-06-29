package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
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
	case "trace":
		if err := traceCmd(cfg, os.Args[2:], os.Stdout); err != nil {
			log.Fatalf("trace: %v", err)
		}
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

type traceGetter interface {
	GetTrace(ctx context.Context, traceID string) (postgres.TraceRecord, bool, error)
}

type traceLookupResult struct {
	Found   bool                  `json:"found"`
	TraceID string                `json:"trace_id,omitempty"`
	Trace   *postgres.TraceRecord `json:"trace,omitempty"`
}

func traceCmd(cfg config.Config, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("trace", flag.ExitOnError)
	traceID := fs.String("trace-id", "", "trace id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *traceID == "" && fs.NArg() > 0 {
		*traceID = fs.Arg(0)
	}
	if *traceID == "" {
		return fmt.Errorf("trace id required")
	}

	pool, err := postgres.Open(context.Background(), cfg.Database.URL)
	if err != nil {
		return err
	}
	defer pool.Close()
	return runTraceLookup(context.Background(), postgres.NewRepository(pool), *traceID, out)
}

func runTraceLookup(ctx context.Context, getter traceGetter, traceID string, out io.Writer) error {
	trace, found, err := getter.GetTrace(ctx, traceID)
	if err != nil {
		return err
	}
	result := traceLookupResult{Found: found, TraceID: traceID}
	if found {
		result.TraceID = ""
		result.Trace = &trace
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

func usage() {
	fmt.Println("usage: oragctl [migrate|eval|token|trace]")
}
