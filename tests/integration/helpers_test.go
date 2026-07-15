package integration

import (
	"context"
	"os"
	"testing"
	"time"

	qdrant "github.com/qdrant/go-client/qdrant"
	core "github.com/shikanon/orag/internal/app"
	"github.com/shikanon/orag/internal/config"
	"github.com/shikanon/orag/internal/kb"
	"github.com/shikanon/orag/internal/platform/logger"
	"github.com/shikanon/orag/internal/storage/postgres"
	qdrantstore "github.com/shikanon/orag/internal/storage/qdrant"
)

const (
	testTenantID = "tenant_default"
	testKBID     = "kb_default"
)

func newIntegrationApp(t *testing.T) *core.App {
	t.Helper()
	if os.Getenv("ORAG_INTEGRATION_TESTS") != "1" {
		t.Skip("integration tests require ORAG_INTEGRATION_TESTS=1 and docker compose test services")
	}

	setenvDefault(t, "STORAGE_BACKEND", "qdrant_postgres")
	setenvDefault(t, "DATABASE_URL", "postgres://orag:orag@localhost:55432/orag_test?sslmode=disable")
	setenvDefault(t, "QDRANT_HOST", "localhost")
	setenvDefault(t, "QDRANT_GRPC_PORT", "6634")
	setenvDefault(t, "QDRANT_COLLECTION", "orag_chunks_test")
	setenvDefault(t, "QDRANT_SEMANTIC_CACHE_COLLECTION", "orag_semantic_cache_test")
	setenvDefault(t, "QDRANT_AUTO_CREATE_COLLECTIONS", "true")
	setenvDefault(t, "REQUIRE_EXTERNAL_PROVIDERS", "false")
	t.Setenv("ALLOW_DETERMINISTIC_MOCK", "true")
	t.Setenv("LLM_CHAT_PROVIDER", "mock")
	t.Setenv("LLM_EMBEDDING_PROVIDER", "mock")
	t.Setenv("LLM_RERANK_PROVIDER", "mock")
	t.Setenv("LLM_MULTIMODAL_PROVIDER", "mock")
	t.Setenv("ARK_API_KEY", "")
	t.Setenv("PORT", "0")

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	waitForPostgres(t, cfg.Database.URL)
	waitForQdrant(t, cfg)
	migrate(t, cfg.Database.URL)

	app, err := core.New(context.Background(), cfg, logger.New(false))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	return app
}

func setenvDefault(t *testing.T, key, value string) {
	t.Helper()
	if os.Getenv(key) == "" {
		t.Setenv(key, value)
	}
}

func waitForPostgres(t *testing.T, url string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		pool, err := postgres.Open(ctx, url)
		if err == nil {
			lastErr = pool.Ping(ctx)
			pool.Close()
			cancel()
			if lastErr == nil {
				return
			}
		} else {
			lastErr = err
			cancel()
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Fatalf("postgres is not ready: %v", lastErr)
}

func waitForQdrant(t *testing.T, cfg config.Config) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		client, err := qdrantstore.Open(ctx, qdrantstore.Config{
			Host:   cfg.Qdrant.Host,
			Port:   cfg.Qdrant.GRPCPort,
			APIKey: cfg.Qdrant.APIKey,
			UseTLS: cfg.Qdrant.UseTLS,
		})
		cancel()
		if err == nil {
			_ = client.Conn.Close()
			return
		}
		lastErr = err
		time.Sleep(300 * time.Millisecond)
	}
	t.Fatalf("qdrant is not ready: %v", lastErr)
}

func migrate(t *testing.T, url string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := postgres.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := postgres.Migrate(ctx, pool, "../../migrations"); err != nil {
		t.Fatal(err)
	}
}

func integrationQueryVector(t *testing.T, ctx context.Context, app *core.App, text string) []float64 {
	t.Helper()
	vectors, err := app.Ingest.Embedder.Embed(ctx, []string{text})
	if err != nil {
		t.Fatal(err)
	}
	if len(vectors) != 1 {
		t.Fatalf("embedding count = %d, want 1", len(vectors))
	}
	return vectors[0]
}

func integrationVectorStore(app *core.App, repo *postgres.Repository) qdrantstore.VectorStore {
	return qdrantstore.VectorStore{
		Client:     app.Qdrant,
		Collection: app.Config.Qdrant.Collection,
		Visibility: repo,
	}
}

func countSearchableSourceChunks(t *testing.T, ctx context.Context, app *core.App, kbID, sourceURI string) int {
	t.Helper()
	var count int
	if err := app.Postgres.QueryRow(ctx, `
		SELECT count(*) FROM chunks
		WHERE tenant_id=$1 AND knowledge_base_id=$2 AND source_uri=$3 AND searchable`,
		testTenantID, kbID, sourceURI).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func denseDocumentIDs(t *testing.T, ctx context.Context, store qdrantstore.VectorStore, req kb.SearchRequest) []string {
	t.Helper()
	results, err := store.Retrieve(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(results))
	for _, result := range results {
		ids = append(ids, result.Chunk.DocumentID)
	}
	return ids
}

func setQdrantDocumentPayload(t *testing.T, ctx context.Context, app *core.App, kbID, documentID string, payload map[string]*qdrant.Value) {
	t.Helper()
	wait := true
	_, err := app.Qdrant.Points.SetPayload(ctx, &qdrant.SetPayloadPoints{
		CollectionName: app.Config.Qdrant.Collection,
		Wait:           &wait,
		Payload:        payload,
		PointsSelector: &qdrant.PointsSelector{PointsSelectorOneOf: &qdrant.PointsSelector_Filter{Filter: &qdrant.Filter{Must: []*qdrant.Condition{
			integrationMatchKeyword("tenant_id", testTenantID),
			integrationMatchKeyword("knowledge_base_id", kbID),
			integrationMatchKeyword("document_id", documentID),
		}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func deleteQdrantDocumentPayloadKey(t *testing.T, ctx context.Context, app *core.App, kbID, documentID, key string) {
	t.Helper()
	wait := true
	_, err := qdrant.NewPointsClient(app.Qdrant.Conn).DeletePayload(ctx, &qdrant.DeletePayloadPoints{
		CollectionName: app.Config.Qdrant.Collection,
		Wait:           &wait,
		Keys:           []string{key},
		PointsSelector: &qdrant.PointsSelector{PointsSelectorOneOf: &qdrant.PointsSelector_Filter{Filter: &qdrant.Filter{Must: []*qdrant.Condition{
			integrationMatchKeyword("tenant_id", testTenantID),
			integrationMatchKeyword("knowledge_base_id", kbID),
			integrationMatchKeyword("document_id", documentID),
		}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
}
