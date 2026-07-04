package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	raggraph "github.com/shikanon/orag/internal/graph"
	"github.com/shikanon/orag/internal/rag"
	"github.com/shikanon/orag/internal/storage/postgres"
)

const (
	memoryTraceDefaultListLimit = 50
	memoryTraceMaxListLimit     = 500
)

type memoryTraceRepository struct {
	mu     sync.RWMutex
	traces map[string]postgres.TraceRecord
}

func newMemoryTraceRepository() *memoryTraceRepository {
	return &memoryTraceRepository{traces: map[string]postgres.TraceRecord{}}
}

func (r *memoryTraceRepository) StoreTrace(_ context.Context, tenantID, traceID, _ string, profile rag.Profile, latencyMS int64, spans []raggraph.NodeSpan) error {
	if traceID == "" {
		return nil
	}
	now := time.Now().UTC()
	r.mu.Lock()
	defer r.mu.Unlock()

	record, ok := r.traces[traceID]
	if !ok {
		record = postgres.TraceRecord{
			ID:        traceID,
			TenantID:  tenantID,
			Profile:   profile,
			LatencyMS: latencyMS,
			CreatedAt: now,
		}
	}
	spanBySequence := make(map[int]postgres.TraceNodeSpan, len(record.NodeSpans)+len(spans))
	for _, span := range record.NodeSpans {
		spanBySequence[span.Sequence] = span
	}
	for i, span := range spans {
		seq := span.Sequence
		if seq <= 0 {
			seq = i + 1
		}
		startedAt, endedAt := memoryTraceSpanTimes(span, now)
		createdAt := now
		if existing, exists := spanBySequence[seq]; exists && !existing.CreatedAt.IsZero() {
			createdAt = existing.CreatedAt
		}
		spanBySequence[seq] = postgres.TraceNodeSpan{
			ID:        memoryTraceStableSpanID(traceID, seq, span.NodeName),
			NodeName:  span.NodeName,
			Sequence:  seq,
			LatencyMS: span.LatencyMS,
			Error:     span.Error,
			StartedAt: startedAt,
			EndedAt:   endedAt,
			CreatedAt: createdAt,
		}
	}
	record.NodeSpans = sortedMemoryTraceSpans(spanBySequence)
	record.HasError, record.ErrorCount = memoryTraceErrorSummary(record.NodeSpans)
	r.traces[traceID] = record
	return nil
}

func (r *memoryTraceRepository) GetTrace(_ context.Context, traceID string) (postgres.TraceRecord, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	record, ok := r.traces[traceID]
	if !ok {
		return postgres.TraceRecord{}, false, nil
	}
	return copyMemoryTraceRecord(record, true), true, nil
}

func (r *memoryTraceRepository) ListTraces(_ context.Context, filter postgres.TraceListFilter) ([]postgres.TraceRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]postgres.TraceRecord, 0, len(r.traces))
	for _, record := range r.traces {
		if !memoryTraceMatchesFilter(record, filter) {
			continue
		}
		items = append(items, copyMemoryTraceRecord(record, false))
	}
	sort.Slice(items, func(i, j int) bool {
		if !items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].CreatedAt.After(items[j].CreatedAt)
		}
		return items[i].ID > items[j].ID
	})
	limit := memoryTraceListLimit(filter.Limit)
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (r *memoryTraceRepository) TraceNodeStats(_ context.Context, filter postgres.TraceListFilter) ([]postgres.TraceNodeStat, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	grouped := map[string][]postgres.TraceNodeSpan{}
	for _, record := range r.traces {
		if !memoryTraceMatchesFilter(record, filter) {
			continue
		}
		for _, span := range record.NodeSpans {
			grouped[span.NodeName] = append(grouped[span.NodeName], span)
		}
	}
	stats := make([]postgres.TraceNodeStat, 0, len(grouped))
	for nodeName, spans := range grouped {
		stats = append(stats, memoryTraceNodeStat(nodeName, spans))
	}
	sort.Slice(stats, func(i, j int) bool {
		if stats[i].AvgLatencyMS != stats[j].AvgLatencyMS {
			return stats[i].AvgLatencyMS > stats[j].AvgLatencyMS
		}
		return stats[i].NodeName < stats[j].NodeName
	})
	return stats, nil
}

func memoryTraceMatchesFilter(record postgres.TraceRecord, filter postgres.TraceListFilter) bool {
	if filter.TenantID != "" && record.TenantID != filter.TenantID {
		return false
	}
	if filter.Profile != "" && record.Profile != filter.Profile {
		return false
	}
	if !filter.Since.IsZero() && record.CreatedAt.Before(filter.Since) {
		return false
	}
	if !filter.Until.IsZero() && record.CreatedAt.After(filter.Until) {
		return false
	}
	if filter.HasError != nil && record.HasError != *filter.HasError {
		return false
	}
	if filter.SlowMS > 0 && record.LatencyMS < filter.SlowMS {
		return false
	}
	return true
}

func memoryTraceNodeStat(nodeName string, spans []postgres.TraceNodeSpan) postgres.TraceNodeStat {
	latencies := make([]float64, 0, len(spans))
	var total float64
	var errors int64
	for _, span := range spans {
		latency := float64(span.LatencyMS)
		total += latency
		latencies = append(latencies, latency)
		if span.Error != "" {
			errors++
		}
	}
	sort.Float64s(latencies)
	count := int64(len(latencies))
	avg := 0.0
	if count > 0 {
		avg = total / float64(count)
	}
	return postgres.TraceNodeStat{
		NodeName:     nodeName,
		Count:        count,
		AvgLatencyMS: avg,
		P95LatencyMS: memoryTracePercentileCont(latencies, 0.95),
		P99LatencyMS: memoryTracePercentileCont(latencies, 0.99),
		ErrorCount:   errors,
	}
}

func memoryTracePercentileCont(values []float64, percentile float64) float64 {
	if len(values) == 0 {
		return 0
	}
	if len(values) == 1 {
		return values[0]
	}
	position := percentile * float64(len(values)-1)
	lower := int(math.Floor(position))
	upper := int(math.Ceil(position))
	if lower == upper {
		return values[lower]
	}
	weight := position - float64(lower)
	return values[lower] + (values[upper]-values[lower])*weight
}

func copyMemoryTraceRecord(record postgres.TraceRecord, includeSpans bool) postgres.TraceRecord {
	out := record
	if includeSpans {
		out.NodeSpans = append([]postgres.TraceNodeSpan(nil), record.NodeSpans...)
	} else {
		out.NodeSpans = nil
	}
	return out
}

func sortedMemoryTraceSpans(spans map[int]postgres.TraceNodeSpan) []postgres.TraceNodeSpan {
	out := make([]postgres.TraceNodeSpan, 0, len(spans))
	for _, span := range spans {
		out = append(out, span)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Sequence != out[j].Sequence {
			return out[i].Sequence < out[j].Sequence
		}
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func memoryTraceErrorSummary(spans []postgres.TraceNodeSpan) (bool, int) {
	var count int
	for _, span := range spans {
		if span.Error != "" {
			count++
		}
	}
	return count > 0, count
}

func memoryTraceSpanTimes(span raggraph.NodeSpan, now time.Time) (time.Time, time.Time) {
	startedAt := span.StartedAt
	if startedAt.IsZero() {
		startedAt = now
	}
	endedAt := span.EndedAt
	if endedAt.IsZero() {
		endedAt = startedAt
		if span.LatencyMS > 0 {
			endedAt = startedAt.Add(time.Duration(span.LatencyMS) * time.Millisecond)
		}
	}
	return startedAt, endedAt
}

func memoryTraceStableSpanID(traceID string, sequence int, nodeName string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s/%d/%s", traceID, sequence, nodeName)))
	return "span_" + hex.EncodeToString(sum[:])[:24]
}

func memoryTraceListLimit(limit int) int {
	if limit <= 0 {
		return memoryTraceDefaultListLimit
	}
	if limit > memoryTraceMaxListLimit {
		return memoryTraceMaxListLimit
	}
	return limit
}
