package fakeark

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

func TestMultimodalEmbeddingDimensionsAreBounded(t *testing.T) {
	server := NewServer()
	t.Cleanup(server.Close)

	t.Run("default", func(t *testing.T) {
		response := postMultimodalEmbedding(t, server.URL, 0)
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusOK)
		}

		var payload struct {
			Data struct {
				Embedding []float64 `json:"embedding"`
			} `json:"data"`
		}
		if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if got := len(payload.Data.Embedding); got != 4 {
			t.Fatalf("embedding length = %d, want 4", got)
		}
	})

	t.Run("maximum", func(t *testing.T) {
		response := postMultimodalEmbedding(t, server.URL, maxEmbeddingDimensions)
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusOK)
		}
	})

	t.Run("oversized", func(t *testing.T) {
		response := postMultimodalEmbedding(t, server.URL, maxEmbeddingDimensions+1)
		defer response.Body.Close()
		if response.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusBadRequest)
		}
	})
}

func TestFakeVectorRejectsInvalidDimensions(t *testing.T) {
	for _, dims := range []int{-1, 0, maxEmbeddingDimensions + 1} {
		if vector := fakeVector(1, dims); vector != nil {
			t.Fatalf("fakeVector(_, %d) = %v, want nil", dims, vector)
		}
	}
	if vector := fakeVector(1, maxEmbeddingDimensions); len(vector) != maxEmbeddingDimensions {
		t.Fatalf("maximum vector length = %d, want %d", len(vector), maxEmbeddingDimensions)
	}
}

func postMultimodalEmbedding(t *testing.T, baseURL string, dimensions int) *http.Response {
	t.Helper()
	body, err := json.Marshal(map[string]int{"dimensions": dimensions})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	response, err := http.Post(baseURL+"/embeddings/multimodal", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post request: %v", err)
	}
	return response
}
