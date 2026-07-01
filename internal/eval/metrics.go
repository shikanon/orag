package eval

import (
	"sort"

	"github.com/shikanon/orag/internal/dataset"
	"github.com/shikanon/orag/internal/rag"
)

func ScoreItem(item dataset.Item, resp rag.QueryResponse) map[string]float64 {
	answerAccuracy := boolScore(matches(resp.Answer, item.GroundTruth))
	citationHit := boolScore(len(resp.Citations) > 0)
	return map[string]float64{
		"answer_accuracy":    answerAccuracy,
		"accuracy":           answerAccuracy,
		"citation_hit_rate":  citationHit,
		"context_recall":     contextRecall(item.RelevantDocIDs, resp),
		"citation_precision": citationPrecision(item.RelevantDocIDs, resp),
		"latency_ms":         float64(resp.LatencyMS),
		"cache_hit":          boolScore(resp.CacheStatus == "hit"),
	}
}

func contextRecall(relevantDocIDs []string, resp rag.QueryResponse) float64 {
	if len(relevantDocIDs) == 0 {
		if len(resp.RetrievedChunks) > 0 {
			return 1
		}
		return 0
	}
	relevant := stringSet(relevantDocIDs)
	seen := map[string]struct{}{}
	for _, result := range resp.RetrievedChunks {
		if _, ok := relevant[result.Chunk.DocumentID]; ok {
			seen[result.Chunk.DocumentID] = struct{}{}
		}
	}
	return float64(len(seen)) / float64(len(relevant))
}

func citationPrecision(relevantDocIDs []string, resp rag.QueryResponse) float64 {
	if len(resp.Citations) == 0 {
		return 0
	}
	if len(relevantDocIDs) == 0 {
		return 1
	}
	relevant := stringSet(relevantDocIDs)
	var hits int
	for _, citation := range resp.Citations {
		if _, ok := relevant[citation.DocumentID]; ok {
			hits++
		}
	}
	return float64(hits) / float64(len(resp.Citations))
}

func average(sum float64, total int) float64 {
	if total == 0 {
		return 0
	}
	return sum / float64(total)
}

func p95(values []int64) int64 {
	if len(values) == 0 {
		return 0
	}
	cp := append([]int64(nil), values...)
	sort.Slice(cp, func(i, j int) bool { return cp[i] < cp[j] })
	idx := int(float64(len(cp)-1) * 0.95)
	return cp[idx]
}

func boolScore(ok bool) float64 {
	if ok {
		return 1
	}
	return 0
}

func stringSet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}
