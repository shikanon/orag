package offlineknowledge

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestValidatorRejectsInvalidItems(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*OptimizationItem, *fakeSourceReader, *fakeConclusionJudge)
		wantErr error
	}{
		{
			name: "missing quote",
			mutate: func(item *OptimizationItem, _ *fakeSourceReader, _ *fakeConclusionJudge) {
				item.Evidence[0].Quote = ""
			},
			wantErr: ErrMissingQuote,
		},
		{
			name: "wrong tenant",
			mutate: func(item *OptimizationItem, _ *fakeSourceReader, _ *fakeConclusionJudge) {
				item.TenantID = "tenant_b"
			},
			wantErr: ErrTenantMismatch,
		},
		{
			name: "wrong knowledge base",
			mutate: func(item *OptimizationItem, _ *fakeSourceReader, _ *fakeConclusionJudge) {
				item.KBID = "kb_other"
			},
			wantErr: ErrKBMismatch,
		},
		{
			name: "stale source hash",
			mutate: func(item *OptimizationItem, _ *fakeSourceReader, _ *fakeConclusionJudge) {
				item.SourceFingerprints[0].ChunkContentHash = "sha256:stale"
			},
			wantErr: ErrStaleFingerprint,
		},
		{
			name: "low confidence",
			mutate: func(item *OptimizationItem, _ *fakeSourceReader, _ *fakeConclusionJudge) {
				item.Confidence = 0.79
			},
			wantErr: ErrLowConfidence,
		},
		{
			name: "conclusion judge fails",
			mutate: func(_ *OptimizationItem, _ *fakeSourceReader, judge *fakeConclusionJudge) {
				judge.accepted = false
			},
			wantErr: ErrConclusionRejected,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := validValidatorItem()
			source := newFakeSourceReader(validSourceChunk())
			judge := &fakeConclusionJudge{accepted: true}
			tt.mutate(&item, source, judge)
			validator := NewValidator(source, judge, ValidatorOptions{MinConfidence: 0.8})

			err := validator.ValidateItem(context.Background(), "tenant_1", "kb_1", item)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ValidateItem() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidatorAcceptsEvidenceGroundedAnswerItem(t *testing.T) {
	item := validValidatorItem()
	source := newFakeSourceReader(validSourceChunk())
	judge := &fakeConclusionJudge{accepted: true}
	validator := NewValidator(source, judge, ValidatorOptions{MinConfidence: 0.8})

	if err := validator.ValidateItem(context.Background(), "tenant_1", "kb_1", item); err != nil {
		t.Fatalf("ValidateItem() error = %v, want nil", err)
	}
	if judge.calls != 1 {
		t.Fatalf("ConclusionJudge calls = %d, want 1", judge.calls)
	}
	if judge.gotItem.ID != item.ID {
		t.Fatalf("ConclusionJudge item ID = %q, want %q", judge.gotItem.ID, item.ID)
	}
}

func TestMemoryRepositoryBasicCRUDAndFilters(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	now := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)

	if err := repo.CreateRun(ctx, OfflineKnowledgeRun{
		ID:        "run_1",
		TenantID:  "tenant_1",
		KBID:      "kb_1",
		Status:    RunStatusPending,
		StartedAt: now,
		ConfigJSON: map[string]any{
			"lookback_days": float64(7),
		},
	}); err != nil {
		t.Fatal(err)
	}
	run, found, err := repo.GetRun(ctx, "tenant_1", "run_1")
	if err != nil || !found {
		t.Fatalf("GetRun() found=%v err=%v", found, err)
	}
	run.ConfigJSON["lookback_days"] = float64(30)
	runs, err := repo.ListRuns(ctx, RunFilter{TenantID: "tenant_1", KBID: "kb_1", Status: RunStatusPending})
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].ConfigJSON["lookback_days"] != float64(7) {
		t.Fatalf("ListRuns() = %#v, want isolated stored run copy", runs)
	}

	if err := repo.UpsertQuestionCluster(ctx, QuestionCluster{
		ID:           "cluster_1",
		TenantID:     "tenant_1",
		RunID:        "run_1",
		KBID:         "kb_1",
		QuestionHash: "hash_1",
		CreatedAt:    now,
	}); err != nil {
		t.Fatal(err)
	}
	clusters, err := repo.ListQuestionClusters(ctx, QuestionClusterFilter{TenantID: "tenant_1", RunID: "run_1", QuestionHash: "hash_1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(clusters) != 1 || clusters[0].ID != "cluster_1" {
		t.Fatalf("ListQuestionClusters() = %#v", clusters)
	}

	item := validValidatorItem()
	if err := repo.CreateOptimizationItem(ctx, item); err != nil {
		t.Fatal(err)
	}
	if updated, err := repo.UpdateOptimizationItemStatus(ctx, "tenant_1", item.ID, ItemStatusVerified, now.Add(time.Minute)); err != nil || !updated {
		t.Fatalf("UpdateOptimizationItemStatus() updated=%v err=%v", updated, err)
	}
	items, err := repo.ListOptimizationItems(ctx, OptimizationItemFilter{TenantID: "tenant_1", KBID: "kb_1", Status: ItemStatusVerified, ItemType: ItemTypeAnswer})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Status != ItemStatusVerified {
		t.Fatalf("ListOptimizationItems() = %#v", items)
	}

	if err := repo.AppendItemEvent(ctx, OptimizationItemEvent{
		ID:        "event_1",
		TenantID:  "tenant_1",
		ItemID:    item.ID,
		EventType: "status_changed",
		Payload:   map[string]any{"status": string(ItemStatusVerified)},
		CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	events, err := repo.ListItemEvents(ctx, OptimizationItemEventFilter{TenantID: "tenant_1", ItemID: item.ID, EventType: "status_changed"})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Payload["status"] != string(ItemStatusVerified) {
		t.Fatalf("ListItemEvents() = %#v", events)
	}

	if err := repo.RecordShadowEvent(ctx, ShadowRetrievalEvent{
		ID:        "shadow_1",
		TenantID:  "tenant_1",
		KBID:      "kb_1",
		ItemID:    item.ID,
		TraceID:   "trace_1",
		Matched:   true,
		CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	shadow, err := repo.ListShadowEvents(ctx, ShadowRetrievalEventFilter{TenantID: "tenant_1", KBID: "kb_1", TraceID: "trace_1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(shadow) != 1 || !shadow[0].Matched {
		t.Fatalf("ListShadowEvents() = %#v", shadow)
	}

	if err := repo.RecordCodexToolAudit(ctx, CodexToolAuditEvent{
		ID:         "audit_1",
		SessionID:  "session_1",
		TenantID:   "tenant_1",
		KBID:       "kb_1",
		Tool:       ReadOnlyToolSearchChunksByText,
		Rows:       2,
		Steps:      1,
		Allowed:    true,
		StartedAt:  now,
		FinishedAt: now.Add(time.Millisecond),
	}); err != nil {
		t.Fatal(err)
	}
	audits, err := repo.ListCodexToolAuditEvents(ctx, CodexToolAuditFilter{
		TenantID:  "tenant_1",
		KBID:      "kb_1",
		SessionID: "session_1",
		Tool:      ReadOnlyToolSearchChunksByText,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(audits) != 1 || !audits[0].Allowed || audits[0].Rows != 2 {
		t.Fatalf("ListCodexToolAuditEvents() = %#v", audits)
	}
}

func validValidatorItem() OptimizationItem {
	now := time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)
	return OptimizationItem{
		ID:                "item_1",
		TenantID:          "tenant_1",
		RunID:             "run_1",
		KBID:              "kb_1",
		QuestionClusterID: "cluster_1",
		ItemType:          ItemTypeAnswer,
		Status:            ItemStatusCandidate,
		CanonicalQuestion: "What is ORAG?",
		FinalAnswer:       "ORAG is a retrieval augmented generation framework.",
		RecallQuality:     RecallQualityMiss,
		FailureType:       FailureTypeSemanticGap,
		Confidence:        0.91,
		SourceFingerprints: []SourceFingerprint{
			{DocID: "doc_1", DocVersion: "v1", ChunkID: "chunk_1", ChunkContentHash: "sha256:chunk_1"},
		},
		Evidence: []Evidence{
			{ChunkID: "chunk_1", DocID: "doc_1", Quote: "ORAG is a retrieval augmented generation framework", Supports: "definition"},
		},
		DeepSearchSteps: []DeepSearchStep{
			{Step: 1, Tool: "search_chunks_by_text", Query: "ORAG", Observation: "definition found", Decision: "keep"},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func validSourceChunk() SourceChunk {
	return SourceChunk{
		TenantID:         "tenant_1",
		KBID:             "kb_1",
		DocID:            "doc_1",
		DocVersion:       "v1",
		ChunkID:          "chunk_1",
		ChunkContentHash: "sha256:chunk_1",
		Text:             "ORAG is a retrieval augmented generation framework for multimodal knowledge workflows.",
	}
}

type fakeSourceReader struct {
	chunks map[string]SourceChunk
}

func newFakeSourceReader(chunks ...SourceChunk) *fakeSourceReader {
	out := &fakeSourceReader{chunks: make(map[string]SourceChunk, len(chunks))}
	for _, chunk := range chunks {
		out.chunks[chunk.ChunkID] = chunk
	}
	return out
}

func (r *fakeSourceReader) ReadSourceChunk(_ context.Context, _, _, chunkID string) (SourceChunk, bool, error) {
	chunk, ok := r.chunks[chunkID]
	return chunk, ok, nil
}

type fakeConclusionJudge struct {
	accepted bool
	calls    int
	gotItem  OptimizationItem
}

func (j *fakeConclusionJudge) JudgeConclusion(_ context.Context, item OptimizationItem, _ []Evidence) (bool, error) {
	j.calls++
	j.gotItem = item
	return j.accepted, nil
}
