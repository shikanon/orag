package integration

import (
	"context"
	"os"
	"testing"
	"time"

	core "github.com/shikanon/orag/internal/app"
	"github.com/shikanon/orag/internal/config"
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
