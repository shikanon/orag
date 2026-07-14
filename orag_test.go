package orag_test

import (
	"context"
	"errors"
	"testing"

	orag "github.com/shikanon/orag"
)

func TestMockClientLifecycleDoesNotReadEnvironment(t *testing.T) {
	t.Setenv("LLM_CHAT_PROVIDER", "volcengine")
	t.Setenv("LLM_EMBEDDING_PROVIDER", "volcengine")
	t.Setenv("ARK_API_KEY", "")
	t.Setenv("VOLCENGINE_API_KEY", "")

	client, err := orag.New(context.Background(), orag.MockConfig())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	readiness, err := client.Readiness(context.Background())
	if err != nil {
		t.Fatalf("Readiness() error = %v", err)
	}
	if !readiness.Ready {
		t.Fatalf("Readiness() = %#v, want ready", readiness)
	}
	if got := readiness.Checks["model_provider"].Status; got != "mock" {
		t.Fatalf("model_provider status = %q, want mock", got)
	}

	if err := client.Close(); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}

func TestClosedClientRejectsNewOperations(t *testing.T) {
	client, err := orag.New(context.Background(), orag.MockConfig())
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = client.Readiness(context.Background())
	if !errors.Is(err, orag.ErrUnavailable) {
		t.Fatalf("Readiness() error = %v, want ErrUnavailable", err)
	}
}

func TestNilClientCloseIsSafe(t *testing.T) {
	var client *orag.Client
	if err := client.Close(); err != nil {
		t.Fatalf("nil Close() error = %v", err)
	}
}
