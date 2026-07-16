package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/shikanon/orag/internal/agentsync"
	core "github.com/shikanon/orag/internal/app"
	"github.com/shikanon/orag/internal/capabilities"
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
	switch os.Args[1] {
	case "migrate":
		cfg := mustConfig()
		if err := migrateCmd(cfg, os.Args[2:], os.Stdout); err != nil {
			log.Fatalf("migrate: %v", err)
		}
		fmt.Println("migrations completed")
	case "eval":
		cfg := mustConfig()
		app := mustApp(cfg)
		defer func() {
			if err := app.Close(); err != nil {
				log.Printf("close app: %v", err)
			}
		}()
		evalCmd(app, os.Args[2:])
	case "token":
		cfg := mustConfig()
		app := mustApp(cfg)
		defer func() {
			if err := app.Close(); err != nil {
				log.Printf("close app: %v", err)
			}
		}()
		fmt.Println(app.BootstrapToken())
	case "trace":
		cfg := mustConfig()
		if err := traceCmd(cfg, os.Args[2:], os.Stdout); err != nil {
			log.Fatalf("trace: %v", err)
		}
	case "generate-agent-artifacts", "generate-skills":
		if err := generateAgentArtifactsCmd(os.Args[2:], os.Stdout); err != nil {
			log.Fatalf("%s: %v", os.Args[1], err)
		}
	default:
		usage()
	}
}

func mustConfig() config.Config {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	return cfg
}

func migrateCmd(cfg config.Config, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("migrate", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	status := fs.Bool("status", false, "report local migration state without applying changes")
	if err := fs.Parse(args); err != nil {
		return err
	}
	pool, err := postgres.Open(context.Background(), cfg.Database.URL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if *status {
		entries, err := postgres.MigrationStatuses(context.Background(), pool, "migrations")
		if err != nil {
			return err
		}
		for _, entry := range entries {
			state := "pending"
			at := ""
			if entry.AppliedAt != nil {
				state = "applied"
				at = entry.AppliedAt.Format(time.RFC3339)
			}
			fmt.Fprintf(out, "%s\t%s\t%s\n", entry.Version, state, at)
		}
		return nil
	}
	return postgres.Migrate(context.Background(), pool, "migrations")
}

func generateAgentArtifactsCmd(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("generate-agent-artifacts", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	openAPIPath := fs.String("openapi", filepath.Join("api", "openapi.yaml"), "OpenAPI document path")
	manifestPath := fs.String("manifest", "builtin", "capability manifest JSON path or builtin")
	outputDir := fs.String("out", ".", "directory where generated MCP and Skill files are written")
	check := fs.Bool("check", false, "verify generated files are in sync without writing")
	if err := fs.Parse(args); err != nil {
		return err
	}
	_ = openAPIPath // kept for CLI compatibility; manifest is the Task 2 SSOT.
	manifest, err := loadCapabilityManifest(*manifestPath)
	if err != nil {
		return err
	}
	files, err := agentsync.GenerateFromManifest(manifest)
	if err != nil {
		return err
	}
	if *check {
		if err := agentsync.CheckFiles(*outputDir, files); err != nil {
			return err
		}
		for _, file := range files {
			fmt.Fprintf(out, "checked %s %s\n", file.Target, file.Path)
		}
		return nil
	}
	if err := agentsync.WriteFiles(*outputDir, files); err != nil {
		return err
	}
	for _, file := range files {
		fmt.Fprintf(out, "generated %s %s\n", file.Target, file.Path)
	}
	return nil
}

func loadCapabilityManifest(path string) (capabilities.Manifest, error) {
	if strings.TrimSpace(path) == "" || path == "builtin" {
		return capabilities.MustBuiltinManifest(), nil
	}
	file, err := os.Open(path)
	if err != nil {
		return capabilities.Manifest{}, err
	}
	defer file.Close()
	return capabilities.LoadJSON(file)
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

type traceReader interface {
	traceGetter
	ListTraces(ctx context.Context, filter postgres.TraceListFilter) ([]postgres.TraceRecord, error)
	TraceNodeStats(ctx context.Context, filter postgres.TraceListFilter) ([]postgres.TraceNodeStat, error)
}

type traceLookupResult struct {
	Found   bool                  `json:"found"`
	TraceID string                `json:"trace_id,omitempty"`
	Trace   *postgres.TraceRecord `json:"trace,omitempty"`
}

type traceListResult struct {
	Traces []postgres.TraceRecord `json:"traces"`
}

type traceStatsResult struct {
	NodeStats []postgres.TraceNodeStat `json:"node_stats"`
}

type traceOptions struct {
	TraceID string
	Stats   bool
	Filter  postgres.TraceListFilter
}

func traceCmd(cfg config.Config, args []string, out io.Writer) error {
	opts, err := parseTraceOptions(args)
	if err != nil {
		return err
	}

	pool, err := postgres.Open(context.Background(), cfg.Database.URL)
	if err != nil {
		return err
	}
	defer pool.Close()
	return runTraceCommand(context.Background(), postgres.NewRepository(pool), opts, out)
}

func parseTraceOptions(args []string) (traceOptions, error) {
	fs := flag.NewFlagSet("trace", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var opts traceOptions
	var since string
	var until string
	var profile string
	var hasError optionalBoolFlag
	fs.StringVar(&opts.TraceID, "trace-id", "", "trace id")
	fs.StringVar(&opts.Filter.TenantID, "tenant-id", "", "tenant id")
	fs.StringVar(&since, "since", "", "inclusive trace creation time in RFC3339 format")
	fs.StringVar(&until, "until", "", "inclusive trace creation time in RFC3339 format")
	fs.StringVar(&profile, "profile", "", "rag profile")
	fs.Var(&hasError, "has-error", "filter by traces with node errors")
	fs.Int64Var(&opts.Filter.SlowMS, "slow-ms", 0, "minimum trace latency in milliseconds")
	fs.IntVar(&opts.Filter.Limit, "limit", 0, "maximum traces to return")
	fs.BoolVar(&opts.Stats, "stats", false, "aggregate trace node latency statistics")
	if err := fs.Parse(args); err != nil {
		return traceOptions{}, err
	}
	if opts.TraceID == "" && fs.NArg() > 0 {
		opts.TraceID = fs.Arg(0)
	}
	if profile != "" {
		opts.Filter.Profile = rag.Profile(profile)
	}
	if hasError.IsSet {
		value := hasError.Value
		opts.Filter.HasError = &value
	}
	parsedSince, err := parseTraceTime("since", since)
	if err != nil {
		return traceOptions{}, err
	}
	parsedUntil, err := parseTraceTime("until", until)
	if err != nil {
		return traceOptions{}, err
	}
	opts.Filter.Since = parsedSince
	opts.Filter.Until = parsedUntil
	return opts, nil
}

func parseTraceTime(name, value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid %s %q: expected RFC3339 time", name, value)
	}
	return parsed, nil
}

func runTraceCommand(ctx context.Context, reader traceReader, opts traceOptions, out io.Writer) error {
	if opts.TraceID != "" {
		return runTraceLookup(ctx, reader, opts.TraceID, out)
	}
	if opts.Stats {
		return runTraceStats(ctx, reader, opts.Filter, out)
	}
	return runTraceList(ctx, reader, opts.Filter, out)
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

func runTraceList(ctx context.Context, reader traceReader, filter postgres.TraceListFilter, out io.Writer) error {
	traces, err := reader.ListTraces(ctx, filter)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(traceListResult{Traces: traces})
}

func runTraceStats(ctx context.Context, reader traceReader, filter postgres.TraceListFilter, out io.Writer) error {
	stats, err := reader.TraceNodeStats(ctx, filter)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(traceStatsResult{NodeStats: stats})
}

type optionalBoolFlag struct {
	IsSet bool
	Value bool
}

func (f *optionalBoolFlag) Set(value string) error {
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fmt.Errorf("invalid boolean %q", value)
	}
	f.IsSet = true
	f.Value = parsed
	return nil
}

func (f optionalBoolFlag) String() string {
	if !f.IsSet {
		return ""
	}
	return strconv.FormatBool(f.Value)
}

func (f optionalBoolFlag) IsBoolFlag() bool {
	return true
}

func usage() {
	fmt.Println("usage: oragctl [migrate [--status]|eval|token|trace|generate-agent-artifacts|generate-skills]")
}
