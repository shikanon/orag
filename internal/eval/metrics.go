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
