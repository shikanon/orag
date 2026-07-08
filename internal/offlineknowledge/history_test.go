package offlineknowledge

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func TestTraceHistoryExtractorExtractsTraceFieldsAndFeedback(t *testing.T) {
	ctx := context.Background()
	window := TimeWindow{
		Start: time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC),
		End:   time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC),
	}
	repo := &historyFakeTraceRepository{traces: []HistoryTrace{
		{
			TenantID:        "tenant_1",
			KBID:            "kb_1",
			TraceID:         "trace_1",
			Query:           "What is ORAG?",
			Answer:          "ORAG is a RAG framework.",
			RetrievedChunks: []string{"chunk_1", "chunk_2"},
			Latency:         123 * time.Millisecond,
			HasError:        true,
			Error:           "generation timeout",
			CreatedAt:       window.Start.Add(time.Hour),
		},
	}}
	feedback := &historyFakeNegativeFeedbackSource{items: []NegativeFeedback{
		{TenantID: "tenant_1", KBID: "kb_1", TraceID: "trace_1", Query: "What is ORAG?", Reason: "answer missed citation", CreatedAt: window.Start.Add(2 * time.Hour)},
	}}
	extractor := NewTraceHistoryExtractor(repo, feedback)

	got, err := extractor.ExtractHistory(ctx, HistoryRequest{
		TenantID: "tenant_1",
		KBID:     "kb_1",
		Window:   window,
		Limit:    25,
	})
	if err != nil {
		t.Fatalf("ExtractHistory() error = %v", err)
	}
	if repo.got.TenantID != "tenant_1" || repo.got.KBID != "kb_1" || !repo.got.Since.Equal(window.Start) || !repo.got.Until.Equal(window.End) || repo.got.Limit != 25 {
		t.Fatalf("trace filter = %#v, want tenant/kb/window/limit", repo.got)
	}
	if feedback.got.TenantID != "tenant_1" || feedback.got.KBID != "kb_1" || !reflect.DeepEqual(feedback.got.TraceIDs, []string{"trace_1"}) {
		t.Fatalf("feedback filter = %#v, want scoped trace feedback", feedback.got)
	}
	if len(got) != 1 {
		t.Fatalf("ExtractHistory() len = %d, want 1: %#v", len(got), got)
	}
	signal := got[0]
	if signal.Query != "What is ORAG?" || signal.Answer != "ORAG is a RAG framework." || signal.TraceID != "trace_1" {
		t.Fatalf("signal core fields = %#v", signal)
	}
	if !reflect.DeepEqual(signal.RetrievedChunks, []string{"chunk_1", "chunk_2"}) || signal.Latency != 123*time.Millisecond {
		t.Fatalf("signal retrieved/latency = %#v", signal)
	}
	if !signal.HasError || signal.Error != "generation timeout" {
		t.Fatalf("signal error fields = %#v", signal)
	}
	if !signal.ExplicitNegativeFeedback || signal.NegativeFeedbackReason != "answer missed citation" {
		t.Fatalf("signal feedback = %#v", signal)
	}
}

func TestTraceHistoryExtractorWithoutFeedbackSourceReturnsTraceSignalsOnly(t *testing.T) {
	window := TimeWindow{Start: time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)}
	repo := &historyFakeTraceRepository{traces: []HistoryTrace{
		{TenantID: "tenant_1", KBID: "kb_1", TraceID: "trace_1", Query: "How to deploy ORAG?", CreatedAt: window.Start.Add(time.Hour)},
	}}
	extractor := NewTraceHistoryExtractor(repo, nil)

	got, err := extractor.ExtractHistory(context.Background(), HistoryRequest{TenantID: "tenant_1", KBID: "kb_1", Window: window})
	if err != nil {
		t.Fatalf("ExtractHistory() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ExtractHistory() len = %d, want 1", len(got))
	}
	if got[0].ExplicitNegativeFeedback || got[0].NegativeFeedbackReason != "" {
		t.Fatalf("feedback fields = %#v, want empty when source is not configured", got[0])
	}
}

func TestTraceHistoryExtractorReadsRepositoryBackedNegativeFeedback(t *testing.T) {
	ctx := context.Background()
	window := TimeWindow{Start: time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)}
	traces := &historyFakeTraceRepository{traces: []HistoryTrace{
		{TenantID: "tenant_1", KBID: "kb_1", TraceID: "trace_keep", Query: "What is ORAG?", CreatedAt: window.Start.Add(time.Hour)},
	}}
	feedback := NewMemoryRepository()
	for _, item := range []NegativeFeedback{
		{TenantID: "tenant_1", KBID: "kb_1", TraceID: "trace_keep", Query: "What is ORAG?", Reason: "bad citation", CreatedAt: window.Start.Add(2 * time.Hour)},
		{TenantID: "tenant_2", KBID: "kb_1", TraceID: "trace_keep", Query: "drop tenant", Reason: "wrong tenant", CreatedAt: window.Start.Add(2 * time.Hour)},
		{TenantID: "tenant_1", KBID: "kb_2", TraceID: "trace_keep", Query: "drop kb", Reason: "wrong kb", CreatedAt: window.Start.Add(2 * time.Hour)},
		{TenantID: "tenant_1", KBID: "kb_1", TraceID: "trace_keep", Query: "drop old", Reason: "old", CreatedAt: window.Start.Add(-time.Second)},
	} {
		if err := feedback.AddNegativeFeedback(ctx, item); err != nil {
			t.Fatal(err)
		}
	}
	extractor := NewTraceHistoryExtractor(traces, feedback)

	got, err := extractor.ExtractHistory(ctx, HistoryRequest{TenantID: "tenant_1", KBID: "kb_1", Window: window, Limit: 10})
	if err != nil {
		t.Fatalf("ExtractHistory() error = %v", err)
	}
	if len(got) != 1 || !got[0].ExplicitNegativeFeedback || got[0].NegativeFeedbackReason != "bad citation" {
		t.Fatalf("ExtractHistory() = %#v, want repository-backed scoped negative feedback", got)
	}
}

func TestMemoryRepositoryListNegativeFeedbackFiltersTenantKBWindowAndLimit(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	base := time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
	for _, item := range []NegativeFeedback{
		{ID: "feedback_newest", TenantID: "tenant_1", KBID: "kb_1", TraceID: "trace_1", Query: "newest", Reason: "newest", CreatedAt: base.Add(2 * time.Hour)},
		{ID: "feedback_oldest", TenantID: "tenant_1", KBID: "kb_1", TraceID: "trace_2", Query: "oldest", Reason: "oldest", CreatedAt: base.Add(time.Hour)},
		{ID: "feedback_other_tenant", TenantID: "tenant_2", KBID: "kb_1", TraceID: "trace_1", Query: "drop", Reason: "tenant", CreatedAt: base.Add(2 * time.Hour)},
		{ID: "feedback_other_kb", TenantID: "tenant_1", KBID: "kb_2", TraceID: "trace_1", Query: "drop", Reason: "kb", CreatedAt: base.Add(2 * time.Hour)},
		{ID: "feedback_outside", TenantID: "tenant_1", KBID: "kb_1", TraceID: "trace_1", Query: "drop", Reason: "outside", CreatedAt: base.Add(-time.Hour)},
	} {
		if err := repo.AddNegativeFeedback(ctx, item); err != nil {
			t.Fatal(err)
		}
	}

	got, err := repo.ListNegativeFeedback(ctx, NegativeFeedbackFilter{
		TenantID: "tenant_1",
		KBID:     "kb_1",
		Since:    base,
		Until:    base.Add(3 * time.Hour),
		TraceIDs: []string{"trace_1", "trace_2"},
		Limit:    1,
	})
	if err != nil {
		t.Fatalf("ListNegativeFeedback() error = %v", err)
	}
	if len(got) != 1 || got[0].ID != "feedback_newest" {
		t.Fatalf("ListNegativeFeedback() = %#v, want newest scoped item only", got)
	}
}

func TestTraceHistoryExtractorTenantKBWindowIsolation(t *testing.T) {
	window := TimeWindow{
		Start: time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC),
		End:   time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC),
	}
	repo := &historyFakeTraceRepository{traces: []HistoryTrace{
		{TenantID: "tenant_1", KBID: "kb_1", TraceID: "trace_keep", Query: "keep", CreatedAt: window.Start.Add(time.Hour)},
		{TenantID: "tenant_2", KBID: "kb_1", TraceID: "trace_other_tenant", Query: "drop", CreatedAt: window.Start.Add(time.Hour)},
		{TenantID: "tenant_1", KBID: "kb_2", TraceID: "trace_other_kb", Query: "drop", CreatedAt: window.Start.Add(time.Hour)},
		{TenantID: "tenant_1", KBID: "kb_1", TraceID: "trace_outside", Query: "drop", CreatedAt: window.Start.Add(-time.Hour)},
	}}
	extractor := NewTraceHistoryExtractor(repo, nil)

	got, err := extractor.ExtractHistory(context.Background(), HistoryRequest{TenantID: "tenant_1", KBID: "kb_1", Window: window})
	if err != nil {
		t.Fatalf("ExtractHistory() error = %v", err)
	}
	if len(got) != 1 || got[0].TraceID != "trace_keep" {
		t.Fatalf("ExtractHistory() = %#v, want only scoped trace", got)
	}
}

func TestDeterministicQuestionClustererMergesDuplicatesAndMarksLongTail(t *testing.T) {
	run := OfflineKnowledgeRun{ID: "run_1", TenantID: "tenant_1", KBID: "kb_1"}
	clusterer := NewDeterministicQuestionClusterer(func() time.Time {
		return time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	})

	got, err := clusterer.ClusterQuestions(context.Background(), ClusterRequest{
		Run: run,
		Signals: []HistorySignal{
			{Query: "What is ORAG?", TraceID: "trace_1"},
			{Query: " what   is orag ", TraceID: "trace_2"},
			{Query: "What is ORAG?!", TraceID: "trace_2"},
			{Query: "Rare deployment edge case", TraceID: "trace_rare"},
		},
	})
	if err != nil {
		t.Fatalf("ClusterQuestions() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ClusterQuestions() len = %d, want 2: %#v", len(got), got)
	}
	frequent := got[0]
	if frequent.NormalizedQuestion != "what is orag" || frequent.OccurrenceCount != 3 || frequent.LongTail {
		t.Fatalf("frequent cluster = %#v", frequent)
	}
	if !reflect.DeepEqual(frequent.SampleQuestions, []string{"What is ORAG?", "what   is orag", "What is ORAG?!"}) {
		t.Fatalf("sample questions = %#v", frequent.SampleQuestions)
	}
	if !reflect.DeepEqual(frequent.TraceIDs, []string{"trace_1", "trace_2"}) {
		t.Fatalf("trace ids = %#v", frequent.TraceIDs)
	}
	longTail := got[1]
	if longTail.NormalizedQuestion != "rare deployment edge case" || longTail.OccurrenceCount != 1 || !longTail.LongTail {
		t.Fatalf("long-tail cluster = %#v", longTail)
	}
	if got[0].QuestionHash == "" || got[0].ID == "" || got[0].QuestionHash == got[1].QuestionHash {
		t.Fatalf("cluster hashes/ids are not deterministic enough: %#v", got)
	}
}

type historyFakeTraceRepository struct {
	traces []HistoryTrace
	got    HistoryTraceFilter
}

func (r *historyFakeTraceRepository) ListHistoryTraces(_ context.Context, filter HistoryTraceFilter) ([]HistoryTrace, error) {
	r.got = filter
	out := make([]HistoryTrace, 0, len(r.traces))
	for _, trace := range r.traces {
		if filter.TenantID != "" && trace.TenantID != filter.TenantID {
			continue
		}
		if filter.KBID != "" && trace.KBID != filter.KBID {
			continue
		}
		if !filter.Since.IsZero() && trace.CreatedAt.Before(filter.Since) {
			continue
		}
		if !filter.Until.IsZero() && !trace.CreatedAt.Before(filter.Until) {
			continue
		}
		out = append(out, trace)
	}
	if filter.Limit > 0 && len(out) > filter.Limit {
		out = out[:filter.Limit]
	}
	return out, nil
}

type historyFakeNegativeFeedbackSource struct {
	items []NegativeFeedback
	got   NegativeFeedbackFilter
}

func (s *historyFakeNegativeFeedbackSource) ListNegativeFeedback(_ context.Context, filter NegativeFeedbackFilter) ([]NegativeFeedback, error) {
	s.got = filter
	return append([]NegativeFeedback(nil), s.items...), nil
}
