package eval

import (
	"sort"
	"strings"
	"unicode"

	"github.com/shikanon/orag/internal/dataset"
	"github.com/shikanon/orag/internal/kb"
	"github.com/shikanon/orag/internal/rag"
)

type ScoreOptions struct {
	TopK int
}

type weightedLatency struct {
	value  int64
	weight float64
}

func ScoreItem(item dataset.Item, resp rag.QueryResponse) map[string]float64 {
	return ScoreItemWithOptions(item, resp, ScoreOptions{})
}

func ScoreItemWithOptions(item dataset.Item, resp rag.QueryResponse, opts ScoreOptions) map[string]float64 {
	answerAccuracy := boolScore(matches(resp.Answer, item.GroundTruth))
	citationHit := boolScore(len(resp.Citations) > 0)
	metrics := map[string]float64{
		"answer_accuracy":    answerAccuracy,
		"accuracy":           answerAccuracy,
		"citation_hit_rate":  citationHit,
		"context_recall":     contextRecall(item.RelevantDocIDs, resp),
		"citation_precision": citationPrecision(item.RelevantDocIDs, resp),
		"latency_ms":         float64(resp.LatencyMS),
		"cache_hit":          boolScore(resp.CacheStatus == "hit"),
	}
	for name, value := range retrievalMetrics(item.RelevantDocIDs, resp.RetrievedChunks, opts.TopK) {
		metrics[name] = value
	}
	for name, value := range redundancyMetrics(resp.RetrievedChunks) {
		metrics[name] = value
	}
	if diversity, ok := ScoreDiversity(resp.RetrievedChunks, diversityAnnotations(item.DiversityAnnotations), DiversityOptions{K: opts.TopK}); ok {
		for name, value := range diversity {
			metrics[name] = value
		}
	}
	return metrics
}

func diversityAnnotations(annotations []dataset.DiversityAnnotation) []DiversityAnnotation {
	out := make([]DiversityAnnotation, 0, len(annotations))
	for _, annotation := range annotations {
		out = append(out, DiversityAnnotation{
			Aspect:      annotation.Aspect,
			Subquestion: annotation.Subquestion,
			ChunkID:     annotation.ChunkID,
			ChunkIDs:    append([]string(nil), annotation.ChunkIDs...),
			DocumentID:  annotation.DocumentID,
			DocumentIDs: append([]string(nil), annotation.DocumentIDs...),
			SourceURI:   annotation.SourceURI,
			SourceURIs:  append([]string(nil), annotation.SourceURIs...),
		})
	}
	return out
}

func redundancyMetrics(results []kb.SearchResult) map[string]float64 {
	total := len(results)
	duplicateCount := duplicateChunkCount(results)
	return map[string]float64{
		"redundancy_rate":     average(float64(duplicateCount), total),
		"duplicate_count":     float64(duplicateCount),
		"deduped_top_k_count": float64(total - duplicateCount),
	}
}

func duplicateChunkCount(results []kb.SearchResult) int {
	seen := map[string]struct{}{}
	duplicates := 0
	for _, result := range results {
		key := duplicateKey(result.Chunk)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			duplicates++
			continue
		}
		seen[key] = struct{}{}
	}
	return duplicates
}

func duplicateKey(chunk kb.Chunk) string {
	if chunk.ID != "" {
		return "chunk:" + chunk.ID
	}
	if key := firstMetadataValue(chunk.Metadata, "content_hash", "chunk_hash", "hash", "dedupe_key"); key != "" {
		return "metadata:" + key
	}
	text := normalizeDuplicateText(chunk.Content)
	if text == "" {
		return ""
	}
	if chunk.DocumentID != "" {
		return "doc_text:" + chunk.DocumentID + "\x00" + text
	}
	if chunk.SourceURI != "" {
		return "source_text:" + chunk.SourceURI + "\x00" + text
	}
	return "text:" + text
}

func firstMetadataValue(metadata map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(metadata[key]); value != "" {
			return value
		}
	}
	return ""
}

func normalizeDuplicateText(text string) string {
	fields := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsPunct(r)
	})
	return strings.Join(fields, " ")
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

func weightedAverage(sum, weight float64) float64 {
	if weight <= 0 {
		return 0
	}
	return sum / weight
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

func weightedP95(values []weightedLatency) int64 {
	if len(values) == 0 {
		return 0
	}
	cp := append([]weightedLatency(nil), values...)
	sort.Slice(cp, func(i, j int) bool { return cp[i].value < cp[j].value })
	var total float64
	for _, value := range cp {
		if value.weight > 0 {
			total += value.weight
		}
	}
	if total <= 0 {
		return 0
	}
	threshold := total * 0.95
	var seen float64
	for _, value := range cp {
		if value.weight <= 0 {
			continue
		}
		seen += value.weight
		if seen >= threshold {
			return value.value
		}
	}
	return cp[len(cp)-1].value
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

func itemWeight(item dataset.Item) float64 {
	item = dataset.NormalizeItemMetadata(item)
	return item.Weight
}

// metricEligible prevents unlabeled cases from silently behaving as zero-score
// cases for metrics that require annotations. Compatibility metrics are still
// emitted, while MetricSummary reports the evidence coverage explicitly.
func metricEligible(name string, item dataset.Item) bool {
	item = dataset.NormalizeItemMetadata(item)
	switch name {
	case "answer_accuracy", "accuracy", PrimaryMetricDeterministicAnswerMatch, "hit_rate":
		return strings.TrimSpace(item.GroundTruth) != ""
	case "context_recall", "citation_precision", "ndcg_at_k", "recall_at_k", "mrr", "map", "coverage", "retrieval_failure_rate":
		return len(item.RelevantDocIDs) > 0
	case "alpha_ndcg", "aspect_coverage":
		return len(item.DiversityAnnotations) > 0
	case "qag_claim_coverage":
		return len(item.ExpectedEvidence) > 0
	default:
		return true
	}
}

func summarizeMetrics(observations map[string][]metricObservation, total int, runID string) map[string]MetricSummary {
	if len(observations) == 0 {
		return map[string]MetricSummary{}
	}
	out := make(map[string]MetricSummary, len(observations))
	for name, values := range observations {
		out[name] = summarizeMetric(values, total, int64(stableMetricSeed(runID, name)))
	}
	return out
}

func stableMetricSeed(runID, name string) uint64 {
	value := stableJSONHash([]string{runID, name})
	var seed uint64
	for _, char := range value {
		seed = seed*131 + uint64(char)
	}
	return seed
}

func copyMetricSummary(summaries map[string]MetricSummary, source string, targets []string) {
	summary, ok := summaries[source]
	if !ok {
		return
	}
	for _, target := range targets {
		summaries[target] = summary
	}
}

func effectiveSampleCount(items []dataset.Item) float64 {
	var total, squared float64
	for _, item := range items {
		weight := itemWeight(item)
		total += weight
		squared += weight * weight
	}
	if squared == 0 {
		return 0
	}
	return total * total / squared
}

func splitWeight(items []dataset.Item) float64 {
	var total float64
	for _, item := range items {
		total += itemWeight(item)
	}
	return total
}

func summarizeSplits(items []dataset.Item) map[string]SplitSummary {
	if len(items) == 0 {
		return nil
	}
	summary := map[string]SplitSummary{}
	for _, item := range items {
		item = dataset.NormalizeItemMetadata(item)
		key := string(item.Split)
		entry := summary[key]
		entry.UnweightedSampleCount++
		entry.WeightedSampleCount += item.Weight
		summary[key] = entry
	}
	return summary
}
