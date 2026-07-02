package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shikanon/orag/internal/llm/ark"
	fakeark "github.com/shikanon/orag/internal/testing/fakeark"
)

func TestClientRoutesOpenAICompatibleCapabilities(t *testing.T) {
	server := fakeark.NewServer()
	defer server.Close()

	client, err := NewClient(Config{
		ChatProvider:        OpenAI,
		EmbeddingProvider:   OpenAI,
		RerankProvider:      OpenAI,
		MultimodalProvider:  OpenAI,
		APIKeys:             map[Name]string{OpenAI: "openai-test-key"},
		BaseURLs:            map[Name]string{OpenAI: server.URL},
		ChatModel:           "gpt-test",
		EmbeddingModel:      "text-embedding-test",
		EmbeddingDimensions: 4,
		RerankModel:         "rerank-test",
		MultimodalModel:     "vision-test",
	}, server.Client())
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	answer, err := client.Chat(context.Background(), []ark.ChatMessage{{Role: "user", Content: "hello"}})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if answer != "fake ark answer" {
		t.Fatalf("answer = %q", answer)
	}

	vectors, err := client.Embed(context.Background(), []string{"hello"})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if len(vectors) != 1 || len(vectors[0]) != 4 {
		t.Fatalf("vectors = %#v", vectors)
	}

	reranked, err := client.Rerank(context.Background(), "rag", []ark.RerankDocument{{Content: "rag framework"}}, 1)
	if err != nil {
		t.Fatalf("Rerank() error = %v", err)
	}
	if len(reranked) != 1 || reranked[0].Index != 0 {
		t.Fatalf("reranked = %#v", reranked)
	}

	parsed, err := client.MultimodalParse(context.Background(), "doc.pdf", []byte("pdf bytes"))
	if err != nil {
		t.Fatalf("MultimodalParse() error = %v", err)
	}
	if parsed != "fake ark answer" {
		t.Fatalf("parsed = %q", parsed)
	}
}

func TestClientRoutesAzureOpenAINativeDeployments(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("api-version") != defaultAzureAPIVersion {
			t.Fatalf("api-version = %q", r.URL.RawQuery)
		}
		if r.Header.Get("api-key") != "azure-test-key" {
			t.Fatalf("missing azure api key header: %q", r.Header.Get("api-key"))
		}
		switch r.URL.Path {
		case "/openai/deployments/chat-deployment/chat/completions":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": "azure answer"}}},
			})
		case "/openai/deployments/embedding-deployment/embeddings":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{{"embedding": []float64{1, 0, 0, 0}}},
			})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewClient(Config{
		ChatProvider:           AzureOpenAI,
		EmbeddingProvider:      AzureOpenAI,
		RerankProvider:         Mock,
		MultimodalProvider:     Mock,
		AllowDeterministicMock: true,
		APIKeys:                map[Name]string{AzureOpenAI: "azure-test-key"},
		BaseURLs:               map[Name]string{AzureOpenAI: server.URL},
		ChatModel:              "chat-deployment",
		EmbeddingModel:         "embedding-deployment",
		EmbeddingDimensions:    4,
	}, server.Client())
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	answer, err := client.Chat(context.Background(), []ark.ChatMessage{{Role: "user", Content: "hello"}})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if answer != "azure answer" {
		t.Fatalf("answer = %q", answer)
	}
	vectors, err := client.Embed(context.Background(), []string{"hello"})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if len(vectors) != 1 || len(vectors[0]) != 4 {
		t.Fatalf("vectors = %#v", vectors)
	}
}

func TestClientAllowsDeterministicMockOnlyWhenExplicit(t *testing.T) {
	if _, err := NewClient(Config{
		ChatProvider:       Mock,
		EmbeddingProvider:  Mock,
		RerankProvider:     Mock,
		MultimodalProvider: Mock,
	}, nil); err == nil {
		t.Fatal("expected explicit mock mode error")
	}

	client, err := NewClient(Config{
		ChatProvider:           Mock,
		EmbeddingProvider:      Mock,
		RerankProvider:         Mock,
		MultimodalProvider:     Mock,
		AllowDeterministicMock: true,
		EmbeddingDimensions:    4,
	}, nil)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	answer, err := client.Chat(context.Background(), []ark.ChatMessage{{Role: "user", Content: "hello"}})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if answer == "" {
		t.Fatal("mock answer is empty")
	}
	vectors, err := client.Embed(context.Background(), []string{"hello"})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if len(vectors) != 1 || len(vectors[0]) != 4 {
		t.Fatalf("vectors = %#v", vectors)
	}
}

func TestClientRoutesCapabilitySpecificProvider(t *testing.T) {
	server := fakeark.NewServer()
	defer server.Close()

	client, err := NewClient(Config{
		ChatProvider:           Mock,
		EmbeddingProvider:      Jina,
		RerankProvider:         Jina,
		MultimodalProvider:     Mock,
		AllowDeterministicMock: true,
		APIKeys:                map[Name]string{Jina: "jina-test-key"},
		BaseURLs:               map[Name]string{Jina: server.URL},
		EmbeddingModel:         "jina-embeddings-v3",
		EmbeddingDimensions:    4,
		RerankModel:            "jina-reranker-v2-base-multilingual",
	}, server.Client())
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	vectors, err := client.Embed(context.Background(), []string{"hello"})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if len(vectors) != 1 || len(vectors[0]) != 4 {
		t.Fatalf("vectors = %#v", vectors)
	}

	reranked, err := client.Rerank(context.Background(), "rag", []ark.RerankDocument{{Content: "rag framework"}}, 1)
	if err != nil {
		t.Fatalf("Rerank() error = %v", err)
	}
	if len(reranked) != 1 || reranked[0].Index != 0 {
		t.Fatalf("reranked = %#v", reranked)
	}
}

func TestClientRoutesVoyageNativeProtocol(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer voyage-test-key" {
			t.Fatalf("missing voyage auth header: %q", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/embeddings":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{{"embedding": []float64{0, 1, 0, 0}}},
			})
		case "/rerank":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{{"index": 1, "relevance_score": 0.91}},
			})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewClient(Config{
		ChatProvider:           Mock,
		EmbeddingProvider:      VoyageAI,
		RerankProvider:         VoyageAI,
		MultimodalProvider:     Mock,
		AllowDeterministicMock: true,
		APIKeys:                map[Name]string{VoyageAI: "voyage-test-key"},
		BaseURLs:               map[Name]string{VoyageAI: server.URL},
		EmbeddingModel:         "voyage-3.5",
		EmbeddingDimensions:    4,
		RerankModel:            "rerank-2",
	}, server.Client())
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	vectors, err := client.Embed(context.Background(), []string{"hello"})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if len(vectors) != 1 || vectors[0][1] != 1 {
		t.Fatalf("vectors = %#v", vectors)
	}

	reranked, err := client.Rerank(context.Background(), "rag", []ark.RerankDocument{
		{Content: "nothing"},
		{Content: "rag framework"},
	}, 2)
	if err != nil {
		t.Fatalf("Rerank() error = %v", err)
	}
	if len(reranked) != 1 || reranked[0].Index != 1 || reranked[0].Score != 0.91 {
		t.Fatalf("reranked = %#v", reranked)
	}
}

func TestClientRoutesCohereNativeProtocol(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/chat":
			if r.Header.Get("Authorization") != "Bearer cohere-test-key" {
				t.Fatalf("missing cohere auth header: %q", r.Header.Get("Authorization"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"message": map[string]any{
					"content": []map[string]string{{"type": "text", "text": "cohere answer"}},
				},
			})
		case "/v2/embed":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"embeddings": map[string]any{
					"float": [][]float64{{1, 0, 0, 0}},
				},
			})
		case "/v2/rerank":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{{"index": 1, "relevance_score": 0.77}},
			})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewClient(Config{
		ChatProvider:           Cohere,
		EmbeddingProvider:      Cohere,
		RerankProvider:         Cohere,
		MultimodalProvider:     Mock,
		AllowDeterministicMock: true,
		APIKeys:                map[Name]string{Cohere: "cohere-test-key"},
		BaseURLs:               map[Name]string{Cohere: server.URL},
		ChatModel:              "command-r-plus",
		EmbeddingModel:         "embed-v4.0",
		EmbeddingDimensions:    4,
		RerankModel:            "rerank-v3.5",
	}, server.Client())
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	answer, err := client.Chat(context.Background(), []ark.ChatMessage{{Role: "user", Content: "hello"}})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if answer != "cohere answer" {
		t.Fatalf("answer = %q", answer)
	}

	vectors, err := client.Embed(context.Background(), []string{"hello"})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if len(vectors) != 1 || len(vectors[0]) != 4 {
		t.Fatalf("vectors = %#v", vectors)
	}

	reranked, err := client.Rerank(context.Background(), "rag", []ark.RerankDocument{
		{Content: "nothing"},
		{Content: "rag framework"},
	}, 2)
	if err != nil {
		t.Fatalf("Rerank() error = %v", err)
	}
	if len(reranked) != 1 || reranked[0].Index != 1 {
		t.Fatalf("reranked = %#v", reranked)
	}
}

func TestClientRoutesAnthropicNativeChat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "anthropic-test-key" {
			t.Fatalf("missing anthropic api key: %q", r.Header.Get("x-api-key"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]string{{"type": "text", "text": "anthropic answer"}},
		})
	}))
	defer server.Close()

	client, err := NewClient(Config{
		ChatProvider:           Anthropic,
		EmbeddingProvider:      Mock,
		RerankProvider:         Mock,
		MultimodalProvider:     Mock,
		AllowDeterministicMock: true,
		APIKeys:                map[Name]string{Anthropic: "anthropic-test-key"},
		BaseURLs:               map[Name]string{Anthropic: server.URL},
		ChatModel:              "claude-test",
		EmbeddingDimensions:    4,
	}, server.Client())
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	answer, err := client.Chat(context.Background(), []ark.ChatMessage{{Role: "user", Content: "hello"}})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if answer != "anthropic answer" {
		t.Fatalf("answer = %q", answer)
	}
}

func TestClientRoutesGeminiNativeChat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models/gemini-test:generateContent" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.URL.Query().Get("key") != "gemini-test-key" {
			t.Fatalf("missing gemini key: %q", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"candidates": []map[string]any{{
				"content": map[string]any{
					"parts": []map[string]string{{"text": "gemini answer"}},
				},
			}},
		})
	}))
	defer server.Close()

	client, err := NewClient(Config{
		ChatProvider:           Gemini,
		EmbeddingProvider:      Mock,
		RerankProvider:         Mock,
		MultimodalProvider:     Mock,
		AllowDeterministicMock: true,
		APIKeys:                map[Name]string{Gemini: "gemini-test-key"},
		BaseURLs:               map[Name]string{Gemini: server.URL},
		ChatModel:              "gemini-test",
		EmbeddingDimensions:    4,
	}, server.Client())
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	answer, err := client.Chat(context.Background(), []ark.ChatMessage{{Role: "user", Content: "hello"}})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if answer != "gemini answer" {
		t.Fatalf("answer = %q", answer)
	}
}

func TestClientRoutesGeminiNativeEmbedding(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models/gemini-embedding-test:embedContent" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.URL.Query().Get("key") != "gemini-test-key" {
			t.Fatalf("missing gemini key: %q", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"embedding": map[string]any{"values": []float64{0, 0, 1, 0}},
		})
	}))
	defer server.Close()

	client, err := NewClient(Config{
		ChatProvider:           Mock,
		EmbeddingProvider:      Gemini,
		RerankProvider:         Mock,
		MultimodalProvider:     Mock,
		AllowDeterministicMock: true,
		APIKeys:                map[Name]string{Gemini: "gemini-test-key"},
		BaseURLs:               map[Name]string{Gemini: server.URL},
		EmbeddingModel:         "gemini-embedding-test",
		EmbeddingDimensions:    4,
	}, server.Client())
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	vectors, err := client.Embed(context.Background(), []string{"hello"})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if len(vectors) != 1 || vectors[0][2] != 1 {
		t.Fatalf("vectors = %#v", vectors)
	}
}

func TestClientRoutesGeminiNativeMultimodal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models/gemini-vision-test:generateContent" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.URL.Query().Get("key") != "gemini-test-key" {
			t.Fatalf("missing gemini key: %q", r.URL.RawQuery)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		contents := body["contents"].([]any)
		parts := contents[0].(map[string]any)["parts"].([]any)
		inline, ok := parts[1].(map[string]any)["inline_data"].(map[string]any)
		if !ok {
			t.Fatalf("expected inline_data part, body = %#v", body)
		}
		if inline["mime_type"] != "application/vnd.openxmlformats-officedocument.wordprocessingml.document" {
			t.Fatalf("unexpected inline mime type: %#v", inline)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"candidates": []map[string]any{{
				"content": map[string]any{
					"parts": []map[string]string{{"text": "gemini parsed markdown"}},
				},
			}},
		})
	}))
	defer server.Close()

	client, err := NewClient(Config{
		ChatProvider:           Mock,
		EmbeddingProvider:      Mock,
		RerankProvider:         Mock,
		MultimodalProvider:     Gemini,
		AllowDeterministicMock: true,
		APIKeys:                map[Name]string{Gemini: "gemini-test-key"},
		BaseURLs:               map[Name]string{Gemini: server.URL},
		MultimodalModel:        "gemini-vision-test",
		EmbeddingDimensions:    4,
	}, server.Client())
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	parsed, err := client.MultimodalParse(context.Background(), "doc.docx", []byte("docx bytes"))
	if err != nil {
		t.Fatalf("MultimodalParse() error = %v", err)
	}
	if parsed != "gemini parsed markdown" {
		t.Fatalf("parsed = %q", parsed)
	}
}

func TestClientRoutesAnthropicNativeMultimodal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "anthropic-test-key" {
			t.Fatalf("missing anthropic api key: %q", r.Header.Get("x-api-key"))
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		messages := body["messages"].([]any)
		content := messages[0].(map[string]any)["content"].([]any)
		if _, ok := content[1].(map[string]any)["source"]; !ok {
			t.Fatalf("expected source part, body = %#v", body)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]string{{"type": "text", "text": "anthropic parsed markdown"}},
		})
	}))
	defer server.Close()

	client, err := NewClient(Config{
		ChatProvider:           Mock,
		EmbeddingProvider:      Mock,
		RerankProvider:         Mock,
		MultimodalProvider:     Anthropic,
		AllowDeterministicMock: true,
		APIKeys:                map[Name]string{Anthropic: "anthropic-test-key"},
		BaseURLs:               map[Name]string{Anthropic: server.URL},
		MultimodalModel:        "claude-vision-test",
		EmbeddingDimensions:    4,
	}, server.Client())
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	parsed, err := client.MultimodalParse(context.Background(), "scan.png", []byte("image bytes"))
	if err != nil {
		t.Fatalf("MultimodalParse() error = %v", err)
	}
	if parsed != "anthropic parsed markdown" {
		t.Fatalf("parsed = %q", parsed)
	}
}
