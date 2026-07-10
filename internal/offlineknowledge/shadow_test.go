package offlineknowledge

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestShadowRetrieverFiltersEligibleStatuses(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	for _, item := range []OptimizationItem{
		shadowTestItem("verified", ItemStatusVerified, "What is ORAG?"),
		shadowTestItem("shadow", ItemStatusShadowEnabled, "What is ORAG?"),
		shadowTestItem("regression", ItemStatusRegressionPassed, "What is ORAG?"),
		shadowTestItem("published", ItemStatusPublished, "What is ORAG?"),
		shadowTestItem("candidate", ItemStatusCandidate, "What is ORAG?"),
		shadowTestItem("rejected", ItemStatusRejected, "What is ORAG?"),
	} {
		if err := repo.CreateOptimizationItem(ctx, item); err != nil {
			t.Fatal(err)
		}
	}
	retriever := NewShadowRetriever(repo, ShadowRetrieverOptions{Now: fixedShadowNow})

	matches, err := retriever.Retrieve(ctx, ShadowRetrieveRequest{TenantID: "tenant_1", KBID: "kb_1", Query: "What is ORAG?", TraceID: "trace_1"})
	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}
	if len(matches) != 4 {
		t.Fatalf("Retrieve() matches = %d, want 4: %#v", len(matches), matches)
	}
	for _, match := range matches {
		switch match.ItemID {
		case "verified", "shadow", "regression", "published":
		default:
			t.Fatalf("Retrieve() returned ineligible item %q", match.ItemID)
		}
		if match.Source != ShadowSourceOptimizationLibrary {
			t.Fatalf("match source = %q, want %q", match.Source, ShadowSourceOptimizationLibrary)
		}
	}
}

func TestShadowRetrieverAnswerItemDoesNotExposeFinalAnswer(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	item := shadowTestItem("answer_item", ItemStatusVerified, "What is ORAG?")
	item.FinalAnswer = "SECRET FINAL ANSWER"
	if err := repo.CreateOptimizationItem(ctx, item); err != nil {
		t.Fatal(err)
	}
	retriever := NewShadowRetriever(repo, ShadowRetrieverOptions{Now: fixedShadowNow})

	matches, err := retriever.Retrieve(ctx, ShadowRetrieveRequest{TenantID: "tenant_1", KBID: "kb_1", Query: "What is ORAG?", TraceID: "trace_1"})
	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("Retrieve() matches = %d, want 1", len(matches))
	}
	answer := matches[0].AnswerItem
	if answer == nil {
		t.Fatal("Retrieve() AnswerItem = nil, want source guidance")
	}
	if len(answer.SourceFingerprints) != 1 || answer.SourceFingerprints[0].ChunkID != "chunk_answer_item" {
		t.Fatalf("AnswerItem.SourceFingerprints = %#v", answer.SourceFingerprints)
	}
	if len(answer.Evidence) != 1 || answer.Evidence[0].Quote != "ORAG is a retrieval augmented generation framework" {
		t.Fatalf("AnswerItem.Evidence = %#v", answer.Evidence)
	}
	assertNoFinalAnswerPayload(t, matches[0].Metadata)
	assertNoFinalAnswerPayload(t, answer.GuidanceMetadata)
}

func TestShadowRetrieverRecordEventWriteFailureDegrades(t *testing.T) {
	ctx := context.Background()
	base := NewMemoryRepository()
	if err := base.CreateOptimizationItem(ctx, shadowTestItem("item_1", ItemStatusVerified, "What is ORAG?")); err != nil {
		t.Fatal(err)
	}
	writeErr := errors.New("shadow store down")
	repo := &shadowFailingRecordRepository{Repository: base, err: writeErr}
	metric := &shadowDropMetricSpy{}
	var dropped []ShadowEventDrop
	retriever := NewShadowRetriever(repo, ShadowRetrieverOptions{
		Now:            fixedShadowNow,
		DropMetric:     metric,
		OnEventDropped: func(drop ShadowEventDrop) { dropped = append(dropped, drop) },
	})

	matches, err := retriever.Retrieve(ctx, ShadowRetrieveRequest{TenantID: "tenant_1", KBID: "kb_1", Query: "What is ORAG?", TraceID: "trace_1"})
	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("Retrieve() matches = %d, want 1", len(matches))
	}
	if repo.recordCalls != 1 {
		t.Fatalf("RecordShadowEvent calls = %d, want 1", repo.recordCalls)
	}
	if metric.counts["write_failed"] != 1 {
		t.Fatalf("drop metric = %#v, want write_failed=1", metric.counts)
	}
	if len(dropped) != 1 || dropped[0].Reason != "write_failed" || !errors.Is(dropped[0].Err, writeErr) {
		t.Fatalf("dropped = %#v, want write failure with original error", dropped)
	}
}

func TestShadowRetrieverRecordEventSamplingDropped(t *testing.T) {
	ctx := context.Background()
	base := NewMemoryRepository()
	repo := &shadowFailingRecordRepository{Repository: base}
	metric := &shadowDropMetricSpy{}
	var dropped []ShadowEventDrop
	retriever := NewShadowRetriever(repo, ShadowRetrieverOptions{
		EventSamplingRate:    0.5,
		EventSamplingRateSet: true,
		RandFloat64:          func() float64 { return 0.9 },
		Now:                  fixedShadowNow,
		DropMetric:           metric,
		OnEventDropped:       func(drop ShadowEventDrop) { dropped = append(dropped, drop) },
	})

	retriever.RecordShadowEvent(ctx, ShadowRetrievalEvent{
		TenantID: "tenant_1",
		KBID:     "kb_1",
		ItemID:   "item_1",
		TraceID:  "trace_1",
		Query:    "What is ORAG?",
		Matched:  true,
	})

	if repo.recordCalls != 0 {
		t.Fatalf("RecordShadowEvent calls = %d, want 0 for sampled event", repo.recordCalls)
	}
	if metric.counts["sampled_out"] != 1 {
		t.Fatalf("drop metric = %#v, want sampled_out=1", metric.counts)
	}
	if len(dropped) != 1 || dropped[0].Reason != "sampled_out" || dropped[0].Event.ID == "" {
		t.Fatalf("dropped = %#v, want sampled_out with populated event ID", dropped)
	}
}

func TestShadowRetrieverExplicitZeroSamplingRateDropsAllEvents(t *testing.T) {
	ctx := context.Background()
	base := NewMemoryRepository()
	repo := &shadowFailingRecordRepository{Repository: base}
	metric := &shadowDropMetricSpy{}
	retriever := NewShadowRetriever(repo, ShadowRetrieverOptions{
		EventSamplingRate:    0,
		EventSamplingRateSet: true,
		RandFloat64:          func() float64 { return 0 },
		Now:                  fixedShadowNow,
		DropMetric:           metric,
	})

	retriever.RecordShadowEvent(ctx, ShadowRetrievalEvent{
		TenantID: "tenant_1",
		KBID:     "kb_1",
		ItemID:   "item_1",
		TraceID:  "trace_1",
		Query:    "What is ORAG?",
		Matched:  true,
	})

	if repo.recordCalls != 0 {
		t.Fatalf("RecordShadowEvent calls = %d, want 0 when explicit sampling rate is 0", repo.recordCalls)
	}
	if metric.counts["sampled_out"] != 1 {
		t.Fatalf("drop metric = %#v, want sampled_out=1", metric.counts)
	}
}

func TestShadowRetrieverDefaultSamplingRateRecordsEvents(t *testing.T) {
	ctx := context.Background()
	base := NewMemoryRepository()
	repo := &shadowFailingRecordRepository{Repository: base}
	retriever := NewShadowRetriever(repo, ShadowRetrieverOptions{
		EventSamplingRate: 0,
		RandFloat64:       func() float64 { return 0.99 },
		Now:               fixedShadowNow,
	})

	retriever.RecordShadowEvent(ctx, ShadowRetrievalEvent{
		TenantID: "tenant_1",
		KBID:     "kb_1",
		ItemID:   "item_1",
		TraceID:  "trace_1",
		Query:    "What is ORAG?",
		Matched:  true,
	})

	if repo.recordCalls != 1 {
		t.Fatalf("RecordShadowEvent calls = %d, want 1 when sampling rate is unset", repo.recordCalls)
	}
}

func TestShadowRetrieverSortsExactAndContainsMatchesOnlyByDefault(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	for _, item := range []OptimizationItem{
		shadowTestItem("contains", ItemStatusVerified, "What is ORAG deployment?"),
		shadowTestItem("default_a", ItemStatusVerified, "How to tune RRF?"),
		shadowTestItem("exact", ItemStatusVerified, "What is ORAG?"),
		shadowTestItem("default_b", ItemStatusVerified, "How to index documents?"),
	} {
		if err := repo.CreateOptimizationItem(ctx, item); err != nil {
			t.Fatal(err)
		}
	}
	retriever := NewShadowRetriever(repo, ShadowRetrieverOptions{Now: fixedShadowNow})

	matches, err := retriever.Retrieve(ctx, ShadowRetrieveRequest{TenantID: "tenant_1", KBID: "kb_1", Query: "What is ORAG?", TraceID: "trace_1"})
	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}
	wantOrder := []string{"exact", "contains"}
	if len(matches) != len(wantOrder) {
		t.Fatalf("Retrieve() matches = %d, want %d", len(matches), len(wantOrder))
	}
	for i, want := range wantOrder {
		if matches[i].ItemID != want {
			t.Fatalf("matches[%d].ItemID = %q, want %q; matches=%#v", i, matches[i].ItemID, want, matches)
		}
		if matches[i].Rank != i+1 {
			t.Fatalf("matches[%d].Rank = %d, want %d", i, matches[i].Rank, i+1)
		}
	}
	if matches[0].Score != 1 || matches[1].Score != 0.7 {
		t.Fatalf("scores = %.1f %.1f, want 1.0 0.7", matches[0].Score, matches[1].Score)
	}
}

func TestShadowRetrieverAllowsLowConfidenceFallbackWhenRequested(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	for _, item := range []OptimizationItem{
		shadowTestItem("contains", ItemStatusVerified, "What is ORAG deployment?"),
		shadowTestItem("default_a", ItemStatusVerified, "How to tune RRF?"),
		shadowTestItem("exact", ItemStatusVerified, "What is ORAG?"),
		shadowTestItem("default_b", ItemStatusVerified, "How to index documents?"),
	} {
		if err := repo.CreateOptimizationItem(ctx, item); err != nil {
			t.Fatal(err)
		}
	}
	retriever := NewShadowRetriever(repo, ShadowRetrieverOptions{Now: fixedShadowNow})

	matches, err := retriever.Retrieve(ctx, ShadowRetrieveRequest{
		TenantID:                   "tenant_1",
		KBID:                       "kb_1",
		Query:                      "What is ORAG?",
		TraceID:                    "trace_1",
		AllowLowConfidenceFallback: true,
	})
	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}
	wantOrder := []string{"exact", "contains", "default_a", "default_b"}
	if len(matches) != len(wantOrder) {
		t.Fatalf("Retrieve() matches = %d, want %d", len(matches), len(wantOrder))
	}
	for i, want := range wantOrder {
		if matches[i].ItemID != want {
			t.Fatalf("matches[%d].ItemID = %q, want %q; matches=%#v", i, matches[i].ItemID, want, matches)
		}
	}
	if matches[0].Score != 1 || matches[1].Score != 0.7 || matches[2].Score != 0.1 || matches[3].Score != 0.1 {
		t.Fatalf("scores = %.1f %.1f %.1f %.1f, want 1.0 0.7 0.1 0.1", matches[0].Score, matches[1].Score, matches[2].Score, matches[3].Score)
	}
}

func TestShadowRetrieverScopedItemOnlyUsesCurrentItem(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	for _, item := range []OptimizationItem{
		shadowTestItem("current", ItemStatusShadowEnabled, "What is ORAG?"),
		shadowTestItem("other", ItemStatusShadowEnabled, "What is ORAG?"),
	} {
		if err := repo.CreateOptimizationItem(ctx, item); err != nil {
			t.Fatal(err)
		}
	}
	retriever := NewShadowRetriever(repo, ShadowRetrieverOptions{Now: fixedShadowNow})

	matches, err := retriever.Retrieve(ctx, ShadowRetrieveRequest{
		TenantID:     "tenant_1",
		KBID:         "kb_1",
		Query:        "What is ORAG?",
		TraceID:      "trace_scoped",
		ScopedItemID: "current",
	})
	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}
	if len(matches) != 1 || matches[0].ItemID != "current" {
		t.Fatalf("Retrieve() matches = %#v, want only current item", matches)
	}
}

func TestShadowRetrieverScopedItemReturnsExplicitErrors(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	disabled := shadowTestItem("disabled", ItemStatusCandidate, "What is ORAG?")
	stale := shadowTestItem("stale", ItemStatusStale, "What is ORAG?")
	for _, item := range []OptimizationItem{disabled, stale} {
		if err := repo.CreateOptimizationItem(ctx, item); err != nil {
			t.Fatal(err)
		}
	}
	retriever := NewShadowRetriever(repo, ShadowRetrieverOptions{Now: fixedShadowNow})

	tests := []struct {
		name         string
		scopedItemID string
		wantErr      error
	}{
		{name: "missing", scopedItemID: "missing", wantErr: ErrScopedShadowItemMissing},
		{name: "disabled", scopedItemID: "disabled", wantErr: ErrScopedShadowItemDisabled},
		{name: "stale", scopedItemID: "stale", wantErr: ErrScopedShadowItemStale},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := retriever.Retrieve(ctx, ShadowRetrieveRequest{
				TenantID:     "tenant_1",
				KBID:         "kb_1",
				Query:        "What is ORAG?",
				TraceID:      "trace_" + tt.name,
				ScopedItemID: tt.scopedItemID,
			})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Retrieve() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func shadowTestItem(id string, status ItemStatus, question string) OptimizationItem {
	now := fixedShadowNow()
	return OptimizationItem{
		ID:                id,
		TenantID:          "tenant_1",
		RunID:             "run_1",
		KBID:              "kb_1",
		QuestionClusterID: "cluster_" + id,
		ItemType:          ItemTypeAnswer,
		Status:            status,
		CanonicalQuestion: question,
		FinalAnswer:       "ORAG is a retrieval augmented generation framework.",
		RecallQuality:     RecallQualityMiss,
		FailureType:       FailureTypeSemanticGap,
		Confidence:        0.91,
		SourceFingerprints: []SourceFingerprint{
			{DocID: "doc_" + id, DocVersion: "v1", ChunkID: "chunk_" + id, ChunkContentHash: "sha256:" + id},
		},
		Evidence: []Evidence{
			{ChunkID: "chunk_" + id, DocID: "doc_" + id, Quote: "ORAG is a retrieval augmented generation framework", Supports: "definition"},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func fixedShadowNow() time.Time {
	return time.Date(2026, 7, 8, 13, 0, 0, 0, time.UTC)
}

func assertNoFinalAnswerPayload(t *testing.T, metadata map[string]any) {
	t.Helper()
	if _, ok := metadata["final_answer"]; ok {
		t.Fatalf("metadata includes final_answer: %#v", metadata)
	}
	for key, value := range metadata {
		if text, ok := value.(string); ok && text == "SECRET FINAL ANSWER" {
			t.Fatalf("metadata[%q] exposes final answer payload", key)
		}
	}
}

type shadowFailingRecordRepository struct {
	Repository
	err         error
	recordCalls int
}

func (r *shadowFailingRecordRepository) RecordShadowEvent(ctx context.Context, event ShadowRetrievalEvent) error {
	r.recordCalls++
	if r.err != nil {
		return r.err
	}
	return r.Repository.RecordShadowEvent(ctx, event)
}

type shadowDropMetricSpy struct {
	counts map[string]int
}

func (m *shadowDropMetricSpy) RecordShadowEventDrop(reason string) {
	if m.counts == nil {
		m.counts = make(map[string]int)
	}
	m.counts[reason]++
}
