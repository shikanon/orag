package ark

import (
	"context"
	"io"
	"testing"

	"github.com/cloudwego/eino/schema"
	fakeark "github.com/shikanon/orag/internal/testing/fakeark"
)

func TestEinoAdaptersUseArkClient(t *testing.T) {
	client := NewClient(Config{EmbeddingDimensions: 8}, nil)
	chat := NewEinoChatModel(client)
	msg, err := chat.Generate(context.Background(), []*schema.Message{schema.UserMessage("hello")})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if msg.Content == "" {
		t.Fatalf("Generate() returned empty content")
	}

	stream, err := chat.Stream(context.Background(), []*schema.Message{schema.UserMessage("hello")})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	chunk, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv() error = %v", err)
	}
	if chunk.Content == "" {
		t.Fatalf("stream chunk content is empty")
	}
	if _, err = stream.Recv(); err != io.EOF {
		t.Fatalf("second Recv() error = %v, want EOF", err)
	}

	embedder := NewEinoEmbedder(client)
	vectors, err := embedder.EmbedStrings(context.Background(), []string{"hello"})
	if err != nil {
		t.Fatalf("EmbedStrings() error = %v", err)
	}
	if len(vectors) != 1 || len(vectors[0]) != 8 {
		t.Fatalf("unexpected vectors shape: %#v", vectors)
	}
}

func TestMockEmbeddingIsDeterministic(t *testing.T) {
	client := NewClient(Config{EmbeddingDimensions: 8}, nil)
	first, err := client.Embed(context.Background(), []string{"same"})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	second, err := client.Embed(context.Background(), []string{"same"})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	for i := range first[0] {
		if first[0][i] != second[0][i] {
			t.Fatalf("embedding differs at %d", i)
		}
	}
}

func TestMockRerankOrdersByLexicalScore(t *testing.T) {
	client := NewClient(Config{}, nil)
	results, err := client.Rerank(context.Background(), "qdrant vector", []RerankDocument{
		{ID: "a", Content: "unrelated"},
		{ID: "b", Content: "qdrant vector database"},
	}, 1)
	if err != nil {
		t.Fatalf("Rerank() error = %v", err)
	}
	if len(results) != 1 || results[0].Index != 1 {
		t.Fatalf("unexpected results: %#v", results)
	}
}

func TestMockEmbeddingDeterministic(t *testing.T) {
	client := NewClient(Config{EmbeddingDimensions: 8}, nil)
	a, err := client.Embed(context.Background(), []string{"hello"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := client.Embed(context.Background(), []string{"hello"})
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 1 || len(a[0]) != 8 {
		t.Fatalf("unexpected embedding shape: %#v", a)
	}
	for i := range a[0] {
		if a[0][i] != b[0][i] {
			t.Fatal("embedding is not deterministic")
		}
	}
}

func TestMockRerank(t *testing.T) {
	client := NewClient(Config{}, nil)
	out, err := client.Rerank(context.Background(), "rag", []RerankDocument{
		{Content: "nothing"},
		{Content: "rag framework"},
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].Index != 1 {
		t.Fatalf("unexpected rerank result: %#v", out)
	}
}

func TestMultimodalEmbeddingUsesVisionEndpoint(t *testing.T) {
	server := fakeark.NewServer()
	defer server.Close()

	client := NewClient(Config{
		APIKey:              "test-key",
		BaseURL:             server.URL,
		EmbeddingModel:      "doubao-embedding-vision-251215",
		EmbeddingDimensions: 4,
	}, server.Client())
	out, err := client.Embed(context.Background(), []string{"hello"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || len(out[0]) != 4 || out[0][1] != 1 {
		t.Fatalf("unexpected multimodal embedding result: %#v", out)
	}
}

func TestAliyunRerank(t *testing.T) {
	server := fakeark.NewServer()
	defer server.Close()

	client := NewClient(Config{
		RerankProvider: "aliyun",
		RerankBaseURL:  server.URL,
		RerankModel:    "qwen3-rerank",
		RerankAPIKey:   "test-key",
	}, server.Client())
	out, err := client.Rerank(context.Background(), "rag", []RerankDocument{
		{Content: "nothing"},
		{Content: "rag framework"},
	}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].Index != 1 {
		t.Fatalf("unexpected aliyun rerank result: %#v", out)
	}
}

func TestClientUsesFakeArkServer(t *testing.T) {
	server := fakeark.NewServer()
	defer server.Close()

	client := NewClient(Config{
		APIKey:              "test-key",
		BaseURL:             server.URL,
		RerankBaseURL:       server.URL,
		ChatModel:           "chat",
		EmbeddingModel:      "embedding",
		EmbeddingDimensions: 4,
		RerankModel:         "rerank",
		MultimodalModel:     "vision",
	}, server.Client())

	answer, err := client.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "hello"}})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if answer != "fake ark answer" {
		t.Fatalf("answer = %q", answer)
	}

	parsed, err := client.MultimodalParse(context.Background(), "doc.pdf", []byte("pdf bytes"))
	if err != nil {
		t.Fatalf("MultimodalParse() error = %v", err)
	}
	if parsed != "fake ark answer" {
		t.Fatalf("parsed = %q", parsed)
	}

	chunks, errs := client.ChatStream(context.Background(), []ChatMessage{{Role: "user", Content: "hello"}})
	var streamed string
	for chunk := range chunks {
		streamed += chunk.Content
	}
	if err := <-errs; err != nil {
		t.Fatalf("ChatStream() error = %v", err)
	}
	if streamed != "fake ark stream" {
		t.Fatalf("streamed = %q", streamed)
	}
}
