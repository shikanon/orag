package kb

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
)

type DenseRetriever struct {
	Store ChunkSource
}

func (r DenseRetriever) Retrieve(_ context.Context, req SearchRequest) ([]SearchResult, error) {
	chunks := r.Store.Chunks(req.TenantID, req.KnowledgeBaseID)
	results := make([]SearchResult, 0, len(chunks))
	for _, chunk := range chunks {
		score := cosine(req.Vector, chunk.Vector)
		if len(req.Vector) == 0 || len(chunk.Vector) == 0 {
			score = lexicalScore(req.Query, chunk.Content) * 0.5
		}
		if score > 0 {
			results = append(results, SearchResult{Chunk: chunk, Score: score, From: "dense"})
		}
	}
	return top(results, denseLimit(req)), nil
}

type SparseRetriever struct {
	Store ChunkSource
}

func (r SparseRetriever) Retrieve(_ context.Context, req SearchRequest) ([]SearchResult, error) {
	chunks := r.Store.Chunks(req.TenantID, req.KnowledgeBaseID)
	results := make([]SearchResult, 0, len(chunks))
	for _, chunk := range chunks {
		score := lexicalScore(req.Query, chunk.SearchText())
		if score > 0 {
			results = append(results, SearchResult{Chunk: chunk, Score: score, From: "sparse"})
		}
	}
	return top(results, sparseLimit(req)), nil
}

type HybridRetriever struct {
	Dense      Retriever
	Sparse     Retriever
	RRFK       int
	TopN       int
	DenseTopK  int
	SparseTopK int
}

func (r HybridRetriever) Retrieve(ctx context.Context, req SearchRequest) ([]SearchResult, error) {
	results, _, err := r.RetrieveWithWarnings(ctx, req)
	return results, err
}

func (r HybridRetriever) RetrieveWithWarnings(ctx context.Context, req SearchRequest) ([]SearchResult, []string, error) {
	denseReq := req
	if r.DenseTopK > 0 {
		denseReq.TopK = r.DenseTopK
		denseReq.DenseTopK = r.DenseTopK
	} else if req.DenseTopK > 0 {
		denseReq.TopK = req.DenseTopK
	}
	sparseReq := req
	if r.SparseTopK > 0 {
		sparseReq.TopK = r.SparseTopK
		sparseReq.SparseTopK = r.SparseTopK
	} else if req.SparseTopK > 0 {
		sparseReq.TopK = req.SparseTopK
	}

	var wg sync.WaitGroup
	var dense, sparse []SearchResult
	var denseErr, sparseErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		if r.Dense == nil {
			denseErr = fmt.Errorf("dense retriever is nil")
			return
		}
		dense, denseErr = r.Dense.Retrieve(ctx, denseReq)
	}()
	go func() {
		defer wg.Done()
		if r.Sparse == nil {
			sparseErr = fmt.Errorf("sparse retriever is nil")
			return
		}
		sparse, sparseErr = r.Sparse.Retrieve(ctx, sparseReq)
	}()
	wg.Wait()

	var warnings []string
	if denseErr != nil {
		warnings = append(warnings, "dense retrieval failed: "+denseErr.Error())
	}
	if sparseErr != nil {
		warnings = append(warnings, "sparse retrieval failed: "+sparseErr.Error())
	}
	if denseErr != nil && sparseErr != nil {
		return nil, warnings, fmt.Errorf("hybrid retrieval failed: dense: %v; sparse: %v", denseErr, sparseErr)
	}
	return RRF([][]SearchResult{dense, sparse}, r.RRFK, r.rrfTopN(req)), warnings, nil
}

func (r HybridRetriever) rrfTopN(req SearchRequest) int {
	if req.TopK > 0 {
		return req.TopK
	}
	return r.TopN
}

func denseLimit(req SearchRequest) int {
	if req.DenseTopK > 0 {
		return req.DenseTopK
	}
	return req.TopK
}

func sparseLimit(req SearchRequest) int {
	if req.SparseTopK > 0 {
		return req.SparseTopK
	}
	return req.TopK
}

func top(results []SearchResult, n int) []SearchResult {
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			return results[i].Chunk.ID < results[j].Chunk.ID
		}
		return results[i].Score > results[j].Score
	})
	for i := range results {
		results[i].Rank = i + 1
	}
	if n > 0 && len(results) > n {
		return results[:n]
	}
	return results
}

func lexicalScore(query, content string) float64 {
	terms := strings.Fields(strings.ToLower(query))
	if len(terms) == 0 {
		return 0
	}
	text := strings.ToLower(content)
	hits := 0
	for _, term := range terms {
		if strings.Contains(text, term) {
			hits++
		}
	}
	return float64(hits) / float64(len(terms))
}

func cosine(a, b []float64) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}
