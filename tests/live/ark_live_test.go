package live

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/shikanon/orag/internal/config"
	"github.com/shikanon/orag/internal/llm/ark"
)

func TestArkSmoke(t *testing.T) {
	if os.Getenv("LIVE_ARK_TESTS") != "1" {
		t.Skip("set LIVE_ARK_TESTS=1 to run live Ark smoke tests")
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Ark.APIKey == "" {
		t.Fatal("ARK_API_KEY is required for live Ark smoke tests")
	}

	timeout := cfg.Ark.Timeout
	if timeout < 3*time.Minute {
		timeout = 3 * time.Minute
	}
	client := ark.NewClient(ark.Config{
		APIKey:              cfg.Ark.APIKey,
		BaseURL:             cfg.Ark.BaseURL,
		ChatModel:           cfg.Ark.ChatModel,
		EmbeddingModel:      cfg.Ark.EmbeddingModel,
		EmbeddingDimensions: cfg.Ark.EmbeddingDimensions,
		RerankProvider:      cfg.Ark.RerankProvider,
		RerankBaseURL:       cfg.Ark.RerankBaseURL,
		RerankModel:         cfg.Ark.RerankModel,
		RerankAPIKey:        cfg.Ark.RerankAPIKey,
		RerankInstruct:      cfg.Ark.RerankInstruct,
		MultimodalModel:     cfg.Ark.MultimodalModel,
		Timeout:             timeout,
		RetryTimes:          cfg.Ark.RetryTimes,
	}, nil)

	t.Logf("running live Ark smoke with chat=%s embedding=%s rerank_provider=%s rerank=%s multimodal=%s", cfg.Ark.ChatModel, cfg.Ark.EmbeddingModel, cfg.Ark.RerankProvider, cfg.Ark.RerankModel, cfg.Ark.MultimodalModel)

	t.Run("chat", func(t *testing.T) {
		ctx, cancel := liveContext(t)
		defer cancel()
		answer, err := client.Chat(ctx, []ark.ChatMessage{
			{Role: "system", Content: "你是一个用于连通性验证的助手。"},
			{Role: "user", Content: "请只用中文简短回复：Ark live test ok"},
		})
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(answer) == "" {
			t.Fatal("empty chat answer")
		}
	})

	t.Run("chat_stream", func(t *testing.T) {
		ctx, cancel := liveContext(t)
		defer cancel()
		chunks, errs := client.ChatStream(ctx, []ark.ChatMessage{
			{Role: "user", Content: "请分两三个短词回复：stream ok"},
		})
		var b strings.Builder
		done := false
		for chunk := range chunks {
			if chunk.Done {
				done = true
			}
			b.WriteString(chunk.Content)
		}
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
		if !done {
			t.Fatal("stream did not send done chunk")
		}
		if strings.TrimSpace(b.String()) == "" {
			t.Fatal("empty stream content")
		}
	})

	t.Run("embedding", func(t *testing.T) {
		if explicitEnv("ARK_EMBEDDING_MODEL") == "" {
			t.Skip("ARK_EMBEDDING_MODEL is not explicitly configured for this Ark account")
		}
		ctx, cancel := liveContext(t)
		defer cancel()
		vectors, err := client.Embed(ctx, []string{"ORAG 使用 Qdrant 做向量检索", "PostgreSQL 提供 sparse retrieval"})
		if err != nil {
			t.Fatal(err)
		}
		if len(vectors) != 2 {
			t.Fatalf("expected 2 embedding vectors, got %d", len(vectors))
		}
		for i, vector := range vectors {
			if len(vector) == 0 {
				t.Fatalf("embedding %d is empty", i)
			}
		}
		t.Logf("embedding dimension=%d", len(vectors[0]))
	})

	t.Run("rerank", func(t *testing.T) {
		if cfg.Ark.RerankProvider == "aliyun" && cfg.Ark.RerankAPIKey == "" {
			t.Skip("ALIYUN_RERANK_API_KEY is not configured")
		}
		if cfg.Ark.RerankProvider == "volcengine" && explicitEnv("ARK_RERANK_MODEL") == "" {
			t.Skip("ARK_RERANK_MODEL is not explicitly configured for this Ark account")
		}
		ctx, cancel := liveContext(t)
		defer cancel()
		results, err := client.Rerank(ctx, "Qdrant 向量检索", []ark.RerankDocument{
			{ID: "qdrant", Content: "Qdrant stores dense vectors and supports similarity search."},
			{ID: "postgres", Content: "PostgreSQL stores metadata and full text search indexes."},
		}, 2)
		if err != nil {
			t.Fatal(err)
		}
		if len(results) == 0 {
			t.Fatal("empty rerank results")
		}
		for _, result := range results {
			if result.Index < 0 || result.Index > 1 {
				t.Fatalf("rerank index out of range: %+v", result)
			}
		}
	})

	t.Run("multimodal_parse", func(t *testing.T) {
		ctx, cancel := liveContext(t)
		defer cancel()
		parsed, err := client.MultimodalParse(ctx, "ark-live-test.png", tinyPNG(t))
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(parsed) == "" {
			t.Fatal("empty multimodal parse result")
		}
	})
}

func liveContext(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(context.Background(), 3*time.Minute)
}

func explicitEnv(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}

func tinyPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			img.Set(x, y, color.RGBA{R: uint8(64 + x), G: uint8(96 + y), B: 180, A: 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
