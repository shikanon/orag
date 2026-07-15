package fakeark

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
)

const maxEmbeddingDimensions = 4096

func NewServer() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Stream bool `json:"stream"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"fake \"}}]}\n\n"))
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ark stream\"}}]}\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": "fake ark answer"}}},
		})
	})
	mux.HandleFunc("/embeddings", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Input []string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		data := make([]map[string]any, 0, len(req.Input))
		for i := range req.Input {
			data = append(data, map[string]any{"embedding": fakeVector(i, 4)})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": data,
		})
	})
	mux.HandleFunc("/embeddings/multimodal", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Dimensions int `json:"dimensions"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Dimensions <= 0 {
			req.Dimensions = 4
		}
		if req.Dimensions > maxEmbeddingDimensions {
			http.Error(w, "dimensions exceed fake provider limit", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"embedding": fakeVector(1, req.Dimensions)},
		})
	})
	mux.HandleFunc("/rerank", func(w http.ResponseWriter, r *http.Request) {
		results := fakeRerankResults(r)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": results,
		})
	})
	mux.HandleFunc("/reranks", func(w http.ResponseWriter, r *http.Request) {
		results := fakeRerankResults(r)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": results,
		})
	})
	return httptest.NewServer(mux)
}

func fakeVector(seed, dims int) []float64 {
	if dims <= 0 || dims > maxEmbeddingDimensions {
		return nil
	}
	vector := make([]float64, dims)
	vector[seed%dims] = 1
	return vector
}

func fakeRerankResults(r *http.Request) []map[string]any {
	var req struct {
		Query     string   `json:"query"`
		Documents []string `json:"documents"`
		TopN      int      `json:"top_n"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	limit := len(req.Documents)
	if req.TopN > 0 && req.TopN < limit {
		limit = req.TopN
	}
	type scored struct {
		index int
		score float64
	}
	scoredDocs := make([]scored, 0, len(req.Documents))
	for i, doc := range req.Documents {
		scoredDocs = append(scoredDocs, scored{index: i, score: fakeLexicalScore(req.Query, doc)})
	}
	sort.SliceStable(scoredDocs, func(i, j int) bool {
		return scoredDocs[i].score > scoredDocs[j].score
	})
	results := make([]map[string]any, 0, limit)
	for i := 0; i < limit; i++ {
		item := scoredDocs[i]
		results = append(results, map[string]any{
			"index":           item.index,
			"relevance_score": item.score,
		})
	}
	return results
}

func fakeLexicalScore(query, content string) float64 {
	terms := strings.Fields(strings.ToLower(query))
	text := strings.ToLower(content)
	if len(terms) == 0 {
		return 0
	}
	hits := 0
	for _, term := range terms {
		if strings.Contains(text, term) {
			hits++
		}
	}
	return float64(hits) / float64(len(terms))
}
