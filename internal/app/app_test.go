package app

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/shikanon/orag/internal/config"
	raggraph "github.com/shikanon/orag/internal/graph"
	"github.com/shikanon/orag/internal/rag"
	"github.com/shikanon/orag/internal/storage/postgres"
)

func TestMemoryTraceRepositoryStoreListAndStats(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryTraceRepository()
	startedAt := time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC)
	spans := []raggraph.NodeSpan{
		{NodeName: "retrieve", Sequence: 1, LatencyMS: 10, StartedAt: startedAt, EndedAt: startedAt.Add(10 * time.Millisecond)},
		{NodeName: "generate", LatencyMS: 30, Error: "llm timeout", StartedAt: startedAt.Add(20 * time.Millisecond)},
	}

	for i := 0; i < 2; i++ {
		if err := repo.StoreTrace(ctx, "tenant_1", "trace_1", "query", rag.ProfileRealtime, 40, spans); err != nil {
			t.Fatalf("StoreTrace() call %d error = %v", i+1, err)
		}
	}
	if err := repo.StoreTrace(ctx, "tenant_1", "trace_2", "query", rag.ProfileRealtime, 20, []raggraph.NodeSpan{
		{NodeName: "retrieve", Sequence: 1, LatencyMS: 20, StartedAt: startedAt.Add(time.Minute)},
		{NodeName: "generate", Sequence: 2, LatencyMS: 10, StartedAt: startedAt.Add(time.Minute)},
	}); err != nil {
		t.Fatalf("StoreTrace() trace_2 error = %v", err)
	}
	if err := repo.StoreTrace(ctx, "tenant_2", "trace_other", "query", rag.ProfileRealtime, 100, []raggraph.NodeSpan{
		{NodeName: "generate", Sequence: 1, LatencyMS: 100, Error: "other tenant"},
	}); err != nil {
		t.Fatalf("StoreTrace() trace_other error = %v", err)
	}

	got, found, err := repo.GetTrace(ctx, "trace_1")
	if err != nil {
		t.Fatalf("GetTrace() error = %v", err)
	}
	if !found {
		t.Fatal("GetTrace() found = false, want true")
	}
	if got.ID != "trace_1" || got.TenantID != "tenant_1" || got.Profile != rag.ProfileRealtime || got.LatencyMS != 40 {
		t.Fatalf("GetTrace() metadata = %#v", got)
	}
	if !got.HasError || got.ErrorCount != 1 {
		t.Fatalf("GetTrace() error summary = has_error:%v error_count:%d", got.HasError, got.ErrorCount)
	}
	if len(got.NodeSpans) != 2 || got.NodeSpans[0].NodeName != "retrieve" || got.NodeSpans[1].Sequence != 2 {
		t.Fatalf("GetTrace() spans = %#v", got.NodeSpans)
	}

	hasError := true
	traces, err := repo.ListTraces(ctx, postgres.TraceListFilter{
		TenantID: "tenant_1",
		Profile:  rag.ProfileRealtime,
		HasError: &hasError,
		SlowMS:   30,
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("ListTraces() error = %v", err)
	}
	if len(traces) != 1 || traces[0].ID != "trace_1" || len(traces[0].NodeSpans) != 0 {
		t.Fatalf("ListTraces() = %#v, want trace_1 summary only", traces)
	}

	stats, err := repo.TraceNodeStats(ctx, postgres.TraceListFilter{
		TenantID: "tenant_1",
		Profile:  rag.ProfileRealtime,
	})
	if err != nil {
		t.Fatalf("TraceNodeStats() error = %v", err)
	}
	if len(stats) != 2 {
		t.Fatalf("TraceNodeStats() len = %d, want 2", len(stats))
	}
	if stats[0].NodeName != "generate" || stats[0].Count != 2 || stats[0].AvgLatencyMS != 20 || stats[0].ErrorCount != 1 {
		t.Fatalf("TraceNodeStats() first = %#v", stats[0])
	}
	if stats[1].NodeName != "retrieve" || stats[1].Count != 2 || stats[1].AvgLatencyMS != 15 || stats[1].ErrorCount != 0 {
		t.Fatalf("TraceNodeStats() second = %#v", stats[1])
	}
}

func TestNewMemoryBackendWiresTraceStoreToGraph(t *testing.T) {
	ctx := context.Background()
	app, err := New(ctx, memoryAppConfig(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() {
		if err := app.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})
	graph, ok := app.RAG.Pipeline.(*raggraph.RAGGraph)
	if !ok {
		t.Fatalf("RAG.Pipeline = %T, want *graph.RAGGraph", app.RAG.Pipeline)
	}
	if graph.TraceStore == nil || app.Traces == nil {
		t.Fatalf("memory backend trace wiring missing: graph=%v app=%v", graph.TraceStore, app.Traces)
	}
	if err := graph.TraceStore.StoreTrace(ctx, "tenant_default", "trace_memory", "query", rag.ProfileRealtime, 12, []raggraph.NodeSpan{
		{NodeName: "init", Sequence: 1, LatencyMS: 1},
	}); err != nil {
		t.Fatalf("StoreTrace() error = %v", err)
	}
	trace, found, err := app.Traces.GetTrace(ctx, "trace_memory")
	if err != nil {
		t.Fatalf("GetTrace() error = %v", err)
	}
	if !found || trace.ID != "trace_memory" || len(trace.NodeSpans) != 1 {
		t.Fatalf("GetTrace() = %#v, found=%v", trace, found)
	}
	stats, err := app.Traces.TraceNodeStats(ctx, postgres.TraceListFilter{TenantID: "tenant_default"})
	if err != nil {
		t.Fatalf("TraceNodeStats() error = %v", err)
	}
	if len(stats) != 1 || stats[0].NodeName != "init" {
		t.Fatalf("TraceNodeStats() = %#v", stats)
	}
}

func memoryAppConfig() config.Config {
	return config.Config{
		Storage: config.StorageConfig{Backend: "memory"},
		Auth:    config.AuthConfig{JWTSecret: "secret", TokenTTL: time.Hour},
		Ark: config.ArkConfig{
			BaseURL:             "http://127.0.0.1:1",
			ChatModel:           "test-chat",
			EmbeddingModel:      "test-embedding",
			EmbeddingDimensions: 3,
			RerankProvider:      "volcengine",
			RerankBaseURL:       "http://127.0.0.1:1",
			RerankModel:         "test-rerank",
			MultimodalModel:     "test-multimodal",
			Timeout:             time.Second,
		},
		RAG: config.RAGConfig{
			DefaultProfile:          string(rag.ProfileRealtime),
			DenseTopK:               3,
			SparseTopK:              3,
			RRFK:                    60,
			RerankTopN:              3,
			ContextTopN:             3,
			MaxContextTokens:        1000,
			SemanticCacheThreshold:  0.92,
			SemanticCacheMaxEntries: 16,
			NoContextAnswer:         "no context",
			PromptCacheMode:         "auto",
		},
		Ingestion: config.IngestionConfig{
			ChunkSizeTokens:    800,
			ChunkOverlapTokens: 120,
			MaxDocumentBytes:   1024,
			ParserMethod:       "basic",
		},
	}
}
