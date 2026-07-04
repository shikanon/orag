package eval

import (
	"math"
	"testing"

	"github.com/shikanon/orag/internal/kb"
)

func TestRetrievalMetrics(t *testing.T) {
	tests := []struct {
		name     string
		relevant []string
		results  []string
		k        int
		want     map[string]float64
	}{
		{
			name:     "no annotations returns zero without failure",
			relevant: nil,
			results:  []string{"doc_1"},
			k:        3,
			want: map[string]float64{
				"ndcg_at_k":              0,
				"recall_at_k":            0,
				"mrr":                    0,
				"map":                    0,
				"coverage":               0,
				"retrieval_failure_rate": 0,
			},
		},
		{
			name:     "no retrieved relevant document is a retrieval failure",
			relevant: []string{"doc_1"},
			results:  []string{"doc_2", "doc_3"},
			k:        2,
			want: map[string]float64{
				"ndcg_at_k":              0,
				"recall_at_k":            0,
				"mrr":                    0,
				"map":                    0,
				"coverage":               0,
				"retrieval_failure_rate": 1,
			},
		},
		{
			name:     "top ranked relevant document scores perfectly",
			relevant: []string{"doc_1"},
			results:  []string{"doc_1", "doc_2"},
			k:        2,
			want: map[string]float64{
				"ndcg_at_k":              1,
				"recall_at_k":            1,
				"mrr":                    1,
				"map":                    1,
				"coverage":               1,
				"retrieval_failure_rate": 0,
			},
		},
		{
			name:     "lower ranked relevant document is discounted",
			relevant: []string{"doc_1"},
			results:  []string{"doc_2", "doc_3", "doc_1"},
			k:        3,
			want: map[string]float64{
				"ndcg_at_k":              0.5,
				"recall_at_k":            1,
				"mrr":                    1.0 / 3.0,
				"map":                    1.0 / 3.0,
				"coverage":               1,
				"retrieval_failure_rate": 0,
			},
		},
		{
			name:     "partial recall with multiple relevant documents",
			relevant: []string{"doc_1", "doc_2", "doc_3"},
			results:  []string{"doc_4", "doc_2", "doc_5", "doc_1"},
			k:        4,
			want: map[string]float64{
				"ndcg_at_k":              (rankDiscount(2) + rankDiscount(4)) / (rankDiscount(1) + rankDiscount(2) + rankDiscount(3)),
				"recall_at_k":            2.0 / 3.0,
				"mrr":                    0.5,
				"map":                    (1.0/2.0 + 2.0/4.0) / 3.0,
				"coverage":               1,
				"retrieval_failure_rate": 0,
			},
		},
		{
			name:     "k cutoff excludes later relevant document",
			relevant: []string{"doc_1", "doc_2"},
			results:  []string{"doc_2", "doc_4", "doc_1"},
			k:        2,
			want: map[string]float64{
				"ndcg_at_k":              1 / (rankDiscount(1) + rankDiscount(2)),
				"recall_at_k":            0.5,
				"mrr":                    1,
				"map":                    0.5,
				"coverage":               1,
				"retrieval_failure_rate": 0,
			},
		},
		{
			name:     "duplicate relevant document only counts once",
			relevant: []string{"doc_1", "doc_2"},
			results:  []string{"doc_1", "doc_1", "doc_2"},
			k:        3,
			want: map[string]float64{
				"ndcg_at_k":              (rankDiscount(1) + rankDiscount(3)) / (rankDiscount(1) + rankDiscount(2)),
				"recall_at_k":            1,
				"mrr":                    1,
				"map":                    (1.0 + 2.0/3.0) / 2.0,
				"coverage":               1,
				"retrieval_failure_rate": 0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := retrievalMetrics(tt.relevant, retrievalSearchResults(tt.results), tt.k)
			for key, want := range tt.want {
				if diff := math.Abs(got[key] - want); diff > 1e-9 {
					t.Fatalf("%s=%f, want %f; metrics=%#v", key, got[key], want, got)
				}
			}
		})
	}
}

func retrievalSearchResults(docIDs []string) []kb.SearchResult {
	results := make([]kb.SearchResult, len(docIDs))
	for i, docID := range docIDs {
		results[i] = kb.SearchResult{
			Chunk: kb.Chunk{
				ID:         docID + "_chunk",
				DocumentID: docID,
			},
			Rank: i + 1,
		}
	}
	return results
}
