package eval

import (
	"math"

	"github.com/shikanon/orag/internal/kb"
)

func retrievalMetrics(relevantDocIDs []string, results []kb.SearchResult, k int) map[string]float64 {
	metrics := map[string]float64{
		"ndcg_at_k":              0,
		"recall_at_k":            0,
		"mrr":                    0,
		"map":                    0,
		"coverage":               0,
		"retrieval_failure_rate": 0,
	}

	relevant := stringSet(relevantDocIDs)
	if len(relevant) == 0 {
		return metrics
	}

	limit := retrievalLimit(len(results), k)
	seenRelevant := map[string]struct{}{}
	var dcg, precisionSum float64
	for i := 0; i < limit; i++ {
		docID := results[i].Chunk.DocumentID
		if _, ok := relevant[docID]; !ok {
			continue
		}
		if _, ok := seenRelevant[docID]; ok {
			continue
		}

		rank := i + 1
		seenRelevant[docID] = struct{}{}
		dcg += rankDiscount(rank)
		precisionSum += float64(len(seenRelevant)) / float64(rank)
		if metrics["mrr"] == 0 {
			metrics["mrr"] = 1 / float64(rank)
		}
	}

	hits := len(seenRelevant)
	if hits == 0 {
		metrics["retrieval_failure_rate"] = 1
		return metrics
	}

	metrics["coverage"] = 1
	metrics["recall_at_k"] = float64(hits) / float64(len(relevant))
	metrics["map"] = precisionSum / float64(len(relevant))
	metrics["ndcg_at_k"] = dcg / idealRetrievalDCG(len(relevant), limit)
	return metrics
}

func retrievalLimit(resultCount, k int) int {
	if k <= 0 || k > resultCount {
		return resultCount
	}
	return k
}

func rankDiscount(rank int) float64 {
	return 1 / math.Log2(float64(rank+1))
}

func idealRetrievalDCG(relevantCount, k int) float64 {
	limit := retrievalLimit(relevantCount, k)
	var ideal float64
	for rank := 1; rank <= limit; rank++ {
		ideal += rankDiscount(rank)
	}
	if ideal == 0 {
		return 1
	}
	return ideal
}
