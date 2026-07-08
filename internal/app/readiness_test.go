package app

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	qdrant "github.com/qdrant/go-client/qdrant"
	"github.com/shikanon/orag/internal/config"
	qdrantstore "github.com/shikanon/orag/internal/storage/qdrant"
	"google.golang.org/grpc"
)

func TestReadinessReportsModelProviderConfigured(t *testing.T) {
	t.Setenv("STORAGE_BACKEND", "memory")
	t.Setenv("ARK_API_KEY", "ark-test-key")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	app, err := New(context.Background(), cfg, slog.Default())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer app.Close()

	checks, ready := app.Readiness(context.Background())
	if !ready {
		t.Fatalf("ready = false checks=%#v", checks)
	}
	if _, ok := checks["ark"]; ok {
		t.Fatalf("legacy ark readiness should not be reported: %#v", checks)
	}
	if checks["model_provider"].Status != "configured" {
		t.Fatalf("model provider readiness = %#v", checks["model_provider"])
	}
}

func TestReadinessReportsExplicitMockModelProvider(t *testing.T) {
	t.Setenv("STORAGE_BACKEND", "memory")
	t.Setenv("ALLOW_DETERMINISTIC_MOCK", "true")
	t.Setenv("LLM_CHAT_PROVIDER", "mock")
	t.Setenv("LLM_EMBEDDING_PROVIDER", "mock")
	t.Setenv("LLM_RERANK_PROVIDER", "mock")
	t.Setenv("LLM_MULTIMODAL_PROVIDER", "mock")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	app, err := New(context.Background(), cfg, slog.Default())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer app.Close()

	checks, ready := app.Readiness(context.Background())
	if !ready {
		t.Fatalf("ready = false checks=%#v", checks)
	}
	if checks["model_provider"].Status != "mock" {
		t.Fatalf("model provider readiness = %#v", checks["model_provider"])
	}
}

func TestReadinessReportsOfflineKnowledgeDisabledAndConfiguredRegression(t *testing.T) {
	t.Setenv("STORAGE_BACKEND", "memory")
	t.Setenv("ARK_API_KEY", "ark-test-key")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	app, err := New(context.Background(), cfg, slog.Default())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer app.Close()

	checks, ready := app.Readiness(context.Background())
	if !ready {
		t.Fatalf("ready = false checks=%#v", checks)
	}
	if checks["offline_knowledge.service"].Status != "ready" {
		t.Fatalf("offline knowledge service readiness = %#v", checks["offline_knowledge.service"])
	}
	if checks["offline_knowledge.codex"].Status != "disabled" {
		t.Fatalf("offline knowledge codex readiness = %#v", checks["offline_knowledge.codex"])
	}
	if checks["offline_knowledge.regression"].Status != "configured" {
		t.Fatalf("offline knowledge regression readiness = %#v", checks["offline_knowledge.regression"])
	}
}

func TestReadinessFailsWhenEnabledOfflineKnowledgeCodexUnavailable(t *testing.T) {
	t.Setenv("STORAGE_BACKEND", "memory")
	t.Setenv("ARK_API_KEY", "ark-test-key")
	t.Setenv("OFFLINE_KNOWLEDGE_ORGANIZER_ENABLED", "true")
	t.Setenv("OFFLINE_KNOWLEDGE_ORGANIZER_TARGETS", "tenant_default:kb_default")
	t.Setenv("OFFLINE_KNOWLEDGE_ORGANIZER_CODEX_ENABLED", "true")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	app, err := New(context.Background(), cfg, slog.Default())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer app.Close()

	checks, ready := app.Readiness(context.Background())
	if ready {
		t.Fatalf("ready = true checks=%#v", checks)
	}
	if checks["offline_knowledge.codex"].Status != "unavailable" {
		t.Fatalf("offline knowledge codex readiness = %#v", checks["offline_knowledge.codex"])
	}
}

func TestReadinessReportsQdrantMainCollectionVectorConfigMismatch(t *testing.T) {
	app := &App{
		Config: readinessQdrantConfig(),
		Qdrant: &qdrantstore.Client{Collections: &readinessCollectionsClient{infos: map[string]*qdrant.CollectionInfo{
			"chunks":         readinessCollectionInfo(768, qdrant.Distance_Cosine),
			"semantic_cache": readinessCollectionInfo(1024, qdrant.Distance_Cosine),
		}}},
	}

	checks, ready := app.Readiness(context.Background())
	if ready {
		t.Fatalf("ready = true checks=%#v", checks)
	}
	check := checks["qdrant"]
	if check.Status != "error" {
		t.Fatalf("qdrant readiness = %#v", check)
	}
	assertReadinessErrorContains(t, check.Error, "qdrant collection \"chunks\" vector config mismatch", "size=768")
}

func TestReadinessReportsQdrantSemanticCacheCollectionVectorConfigMismatch(t *testing.T) {
	app := &App{
		Config: readinessQdrantConfig(),
		Qdrant: &qdrantstore.Client{Collections: &readinessCollectionsClient{infos: map[string]*qdrant.CollectionInfo{
			"chunks":         readinessCollectionInfo(1024, qdrant.Distance_Cosine),
			"semantic_cache": readinessCollectionInfo(768, qdrant.Distance_Cosine),
		}}},
	}

	checks, ready := app.Readiness(context.Background())
	if ready {
		t.Fatalf("ready = true checks=%#v", checks)
	}
	check := checks["qdrant"]
	if check.Status != "error" {
		t.Fatalf("qdrant readiness = %#v", check)
	}
	assertReadinessErrorContains(t, check.Error, "qdrant collection \"semantic_cache\" vector config mismatch", "size=768")
}

func readinessQdrantConfig() config.Config {
	return config.Config{
		Storage: config.StorageConfig{Backend: "qdrant_postgres"},
		Qdrant: config.QdrantConfig{
			Collection:              "chunks",
			SemanticCacheCollection: "semantic_cache",
		},
		Ark: config.ArkConfig{EmbeddingDimensions: 1024},
	}
}

func readinessCollectionInfo(size uint64, distance qdrant.Distance) *qdrant.CollectionInfo {
	return &qdrant.CollectionInfo{Config: &qdrant.CollectionConfig{Params: &qdrant.CollectionParams{
		VectorsConfig: &qdrant.VectorsConfig{Config: &qdrant.VectorsConfig_Params{
			Params: &qdrant.VectorParams{
				Size:     size,
				Distance: distance,
			},
		}},
	}}}
}

func assertReadinessErrorContains(t *testing.T, msg string, parts ...string) {
	t.Helper()
	for _, part := range parts {
		if !strings.Contains(msg, part) {
			t.Fatalf("readiness error %q does not contain %q", msg, part)
		}
	}
}

type readinessCollectionsClient struct {
	infos map[string]*qdrant.CollectionInfo
}

func (c *readinessCollectionsClient) CollectionExists(_ context.Context, req *qdrant.CollectionExistsRequest, _ ...grpc.CallOption) (*qdrant.CollectionExistsResponse, error) {
	_, exists := c.infos[req.GetCollectionName()]
	return &qdrant.CollectionExistsResponse{Result: &qdrant.CollectionExists{Exists: exists}}, nil
}

func (c *readinessCollectionsClient) Get(_ context.Context, req *qdrant.GetCollectionInfoRequest, _ ...grpc.CallOption) (*qdrant.GetCollectionInfoResponse, error) {
	return &qdrant.GetCollectionInfoResponse{Result: c.infos[req.GetCollectionName()]}, nil
}

func (c *readinessCollectionsClient) Create(context.Context, *qdrant.CreateCollection, ...grpc.CallOption) (*qdrant.CollectionOperationResponse, error) {
	return &qdrant.CollectionOperationResponse{}, nil
}
