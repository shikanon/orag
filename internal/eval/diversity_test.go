package eval

import (
	"testing"

	"github.com/shikanon/orag/internal/kb"
)

func TestScoreDiversityRewardsBalancedAspectCoverage(t *testing.T) {
	annotations := []DiversityAnnotation{
		{Aspect: "install", ChunkID: "chunk_install"},
		{Aspect: "config", ChunkID: "chunk_config"},
		{Aspect: "install", ChunkID: "chunk_install_2"},
	}
	balanced := searchResults("chunk_install", "chunk_config", "chunk_install_2")
	stacked := searchResults("chunk_install", "chunk_install_2", "chunk_config")

	balancedMetrics, ok := ScoreDiversity(balanced, annotations, DiversityOptions{K: 3})
	if !ok {
		t.Fatal("ScoreDiversity() ok=false")
	}
	stackedMetrics, ok := ScoreDiversity(stacked, annotations, DiversityOptions{K: 3})
	if !ok {
		t.Fatal("ScoreDiversity() ok=false for stacked results")
	}
	if balancedMetrics["alpha_ndcg"] <= stackedMetrics["alpha_ndcg"] {
		t.Fatalf("balanced alpha_ndcg=%f, stacked alpha_ndcg=%f", balancedMetrics["alpha_ndcg"], stackedMetrics["alpha_ndcg"])
	}
	if balancedMetrics["aspect_coverage"] != 1 {
		t.Fatalf("balanced aspect_coverage=%f", balancedMetrics["aspect_coverage"])
	}
}

func TestScoreDiversitySupportsSubquestionAnnotations(t *testing.T) {
	annotations := []DiversityAnnotation{
		{Subquestion: "how to install", DocumentID: "doc_install"},
		{Subquestion: "how to configure", SourceURI: "memory://config"},
	}
	results := []kb.SearchResult{
		{Chunk: kb.Chunk{ID: "chunk_1", DocumentID: "doc_install"}},
		{Chunk: kb.Chunk{ID: "chunk_2", SourceURI: "memory://config"}},
	}

	metrics, ok := ScoreDiversity(results, annotations, DiversityOptions{})
	if !ok {
		t.Fatal("ScoreDiversity() ok=false")
	}
	if metrics["alpha_ndcg"] != 1 || metrics["aspect_coverage"] != 1 {
		t.Fatalf("metrics = %#v", metrics)
	}
}

func TestScoreDiversitySkipsMissingAnnotations(t *testing.T) {
	tests := []struct {
		name        string
		annotations []DiversityAnnotation
	}{
		{name: "nil annotations"},
		{name: "missing label", annotations: []DiversityAnnotation{{ChunkID: "chunk_1"}}},
		{name: "missing reference", annotations: []DiversityAnnotation{{Aspect: "install"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics, ok := ScoreDiversity(searchResults("chunk_1"), tt.annotations, DiversityOptions{})
			if ok {
				t.Fatalf("ScoreDiversity() ok=true metrics=%#v", metrics)
			}
			if len(metrics) != 0 {
				t.Fatalf("metrics = %#v", metrics)
			}
		})
	}
}

func TestScoreDiversityScoresEmptyRetrieval(t *testing.T) {
	annotations := []DiversityAnnotation{
		{Aspect: "install", ChunkID: "chunk_install"},
		{Aspect: "config", ChunkID: "chunk_config"},
	}

	metrics, ok := ScoreDiversity(nil, annotations, DiversityOptions{K: 3})
	if !ok {
		t.Fatal("ScoreDiversity() ok=false")
	}
	if metrics["alpha_ndcg"] != 0 || metrics["aspect_coverage"] != 0 {
		t.Fatalf("metrics = %#v", metrics)
	}
}

func searchResults(chunkIDs ...string) []kb.SearchResult {
	results := make([]kb.SearchResult, 0, len(chunkIDs))
	for idx, chunkID := range chunkIDs {
		results = append(results, kb.SearchResult{
			Chunk: kb.Chunk{ID: chunkID},
			Rank:  idx + 1,
		})
	}
	return results
}
