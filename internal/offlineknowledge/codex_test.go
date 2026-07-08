package offlineknowledge

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/shikanon/orag/internal/kb"
)

func TestValidateCodexResponseAcceptsValidOutput(t *testing.T) {
	response := validCodexAnalyzeResponse()

	if err := ValidateCodexResponse(response, testCodexQuota()); err != nil {
		t.Fatalf("ValidateCodexResponse() error = %v, want nil", err)
	}
}

func TestValidateCodexResponseRejectsInvalidEnum(t *testing.T) {
	response := validCodexAnalyzeResponse()
	response.RecallQuality = RecallQuality("confident_guess")

	err := ValidateCodexResponse(response, testCodexQuota())
	if !errors.Is(err, ErrCodexInvalidEnum) {
		t.Fatalf("ValidateCodexResponse() error = %v, want %v", err, ErrCodexInvalidEnum)
	}
}

func TestValidateCodexResponseRejectsAnswerItemMissingEvidence(t *testing.T) {
	response := validCodexAnalyzeResponse()
	response.Evidence = nil

	err := ValidateCodexResponse(response, testCodexQuota())
	if !errors.Is(err, ErrCodexMissingEvidence) {
		t.Fatalf("ValidateCodexResponse() error = %v, want %v", err, ErrCodexMissingEvidence)
	}
}

func TestValidateCodexResponseRejectsOverBudgetDeepSearchSteps(t *testing.T) {
	response := validCodexAnalyzeResponse()
	response.DeepSearchSteps = append(response.DeepSearchSteps,
		DeepSearchStep{Step: 3, Tool: string(ReadOnlyToolGetDocumentChunks), Query: "doc_1", Observation: "more chunks", Decision: "compare"},
	)
	quota := testCodexQuota()
	quota.MaxDeepSearchSteps = 2

	err := ValidateCodexResponse(response, quota)
	if !errors.Is(err, ErrCodexStepBudgetExceeded) {
		t.Fatalf("ValidateCodexResponse() error = %v, want %v", err, ErrCodexStepBudgetExceeded)
	}
}

func TestValidateCodexResponseRejectsUnknownTool(t *testing.T) {
	response := validCodexAnalyzeResponse()
	response.DeepSearchSteps[0].Tool = "write_chunk"

	err := ValidateCodexResponse(response, testCodexQuota())
	if !errors.Is(err, ErrCodexUnknownTool) {
		t.Fatalf("ValidateCodexResponse() error = %v, want %v", err, ErrCodexUnknownTool)
	}
}

func TestValidateCodexResponseRejectsActionItemMismatch(t *testing.T) {
	tests := []struct {
		name   string
		action RecommendedAction
		item   ItemType
	}{
		{name: "answer action requires answer item", action: RecommendedActionCreateAnswerItem, item: ItemTypeQueryRewrite},
		{name: "rewrite action requires rewrite item", action: RecommendedActionCreateQueryRewriteItem, item: ItemTypeAnswer},
		{name: "knowledge gap action requires knowledge gap item", action: RecommendedActionCreateKnowledgeGapItem, item: ItemTypeAnswer},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := validCodexAnalyzeResponse()
			response.RecommendedAction = tt.action
			response.ItemType = tt.item

			err := ValidateCodexResponse(response, testCodexQuota())
			if !errors.Is(err, ErrCodexActionItemMismatch) {
				t.Fatalf("ValidateCodexResponse() error = %v, want %v", err, ErrCodexActionItemMismatch)
			}
		})
	}
}

func TestValidateCodexResponseAllowsUnconstrainedReviewActions(t *testing.T) {
	tests := []RecommendedAction{
		RecommendedActionNeedsReview,
		RecommendedActionReject,
	}

	for _, action := range tests {
		t.Run(string(action), func(t *testing.T) {
			response := validCodexAnalyzeResponse()
			response.RecommendedAction = action
			response.ItemType = ItemTypeQueryRewrite

			if err := ValidateCodexResponse(response, testCodexQuota()); err != nil {
				t.Fatalf("ValidateCodexResponse() error = %v, want nil", err)
			}
		})
	}
}

func TestCodexReadOnlyToolNamesMatchDesign(t *testing.T) {
	tests := []struct {
		name string
		tool ReadOnlyToolName
		want string
	}{
		{name: "text search", tool: ReadOnlyToolSearchChunksByText, want: "search_chunks_by_text"},
		{name: "vector search", tool: ReadOnlyToolSearchChunksVector, want: "search_chunks_by_vector"},
		{name: "neighbors", tool: ReadOnlyToolGetChunkNeighbors, want: "get_chunk_neighbors"},
		{name: "document chunks", tool: ReadOnlyToolGetDocumentChunks, want: "get_document_chunks"},
		{name: "graph chunks", tool: ReadOnlyToolGetGraphChunks, want: "get_related_graph_chunks"},
		{name: "eval results", tool: ReadOnlyToolLookupEvalResults, want: "get_eval_results_by_question"},
		{name: "existing items", tool: ReadOnlyToolLookupExistingItem, want: "get_existing_optimization_items"},
		{name: "replay recall", tool: ReadOnlyToolReplayRecall, want: "replay_recall_with_query"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.tool) != tt.want {
				t.Fatalf("tool value = %q, want %q", tt.tool, tt.want)
			}
			guard := NewToolGuard(testCodexQuota())
			err := guard.Validate(ToolUsage{Tool: tt.tool, Tokens: 1, Rows: 1, QPS: 1, Timeout: time.Second, Steps: 1})
			if err != nil {
				t.Fatalf("ToolGuard.Validate(%q) error = %v, want nil", tt.tool, err)
			}
		})
	}
}

func TestCodexRecommendedActionValuesMatchDesign(t *testing.T) {
	tests := []struct {
		name   string
		action RecommendedAction
		want   string
	}{
		{name: "answer item", action: RecommendedActionCreateAnswerItem, want: "create_answer_item"},
		{name: "query rewrite item", action: RecommendedActionCreateQueryRewriteItem, want: "create_query_rewrite_item"},
		{name: "knowledge gap item", action: RecommendedActionCreateKnowledgeGapItem, want: "create_knowledge_gap_item"},
		{name: "needs review", action: RecommendedActionNeedsReview, want: "needs_review"},
		{name: "reject", action: RecommendedActionReject, want: "reject"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.action) != tt.want {
				t.Fatalf("recommended action value = %q, want %q", tt.action, tt.want)
			}
		})
	}
}

func TestCodexToolGuardRejectsQuotaExceeded(t *testing.T) {
	tests := []struct {
		name  string
		usage ToolUsage
	}{
		{
			name:  "tokens",
			usage: ToolUsage{Tool: ReadOnlyToolSearchChunksByText, Tokens: 1001, Rows: 10, QPS: 2, Timeout: time.Second, Steps: 1},
		},
		{
			name:  "rows",
			usage: ToolUsage{Tool: ReadOnlyToolSearchChunksByText, Tokens: 100, Rows: 21, QPS: 2, Timeout: time.Second, Steps: 1},
		},
		{
			name:  "qps",
			usage: ToolUsage{Tool: ReadOnlyToolSearchChunksByText, Tokens: 100, Rows: 10, QPS: 6, Timeout: time.Second, Steps: 1},
		},
		{
			name:  "timeout",
			usage: ToolUsage{Tool: ReadOnlyToolSearchChunksByText, Tokens: 100, Rows: 10, QPS: 2, Timeout: 6 * time.Second, Steps: 1},
		},
		{
			name:  "steps",
			usage: ToolUsage{Tool: ReadOnlyToolSearchChunksByText, Tokens: 100, Rows: 10, QPS: 2, Timeout: time.Second, Steps: 5},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			guard := NewToolGuard(testCodexQuota())

			err := guard.Validate(tt.usage)
			if !errors.Is(err, ErrCodexQuotaExceeded) {
				t.Fatalf("ToolGuard.Validate() error = %v, want %v", err, ErrCodexQuotaExceeded)
			}
		})
	}
}

func TestCodexRunnerAdapterUnavailableAndDisabled(t *testing.T) {
	ctx := context.Background()
	req := validCodexAnalyzeRequest()

	_, err := NewCodexRunnerAdapter(CodexRunnerConfig{}).AnalyzeCodex(ctx, req)
	if !errors.Is(err, ErrCodexDisabled) {
		t.Fatalf("AnalyzeCodex() error = %v, want %v", err, ErrCodexDisabled)
	}

	_, err = NewCodexRunnerAdapter(CodexRunnerConfig{Enabled: true}).AnalyzeCodex(ctx, req)
	if !errors.Is(err, ErrCodexUnavailable) {
		t.Fatalf("AnalyzeCodex() error = %v, want %v", err, ErrCodexUnavailable)
	}
}

func TestCodexRunnerAdapterRejectsInvalidOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"item_type":"answer_item","recommended_action":"create_answer_item","recall_quality":"confident_guess","confidence":0.9,"evidence":[{"chunk_id":"chunk_1","doc_id":"doc_1","quote":"q","supports":"s"}]}`))
	}))
	defer server.Close()

	adapter := NewCodexRunnerAdapter(CodexRunnerConfig{Enabled: true, Endpoint: server.URL})
	_, err := adapter.AnalyzeCodex(context.Background(), validCodexAnalyzeRequest())
	if !errors.Is(err, ErrCodexInvalidEnum) {
		t.Fatalf("AnalyzeCodex() error = %v, want %v", err, ErrCodexInvalidEnum)
	}
}

func TestCodexRunnerAdapterRejectsInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`not-json`))
	}))
	defer server.Close()

	adapter := NewCodexRunnerAdapter(CodexRunnerConfig{Enabled: true, Endpoint: server.URL})
	_, err := adapter.AnalyzeCodex(context.Background(), validCodexAnalyzeRequest())
	if !errors.Is(err, ErrCodexInvalidJSON) {
		t.Fatalf("AnalyzeCodex() error = %v, want %v", err, ErrCodexInvalidJSON)
	}
}

func TestCodexToolRegistryRejectsUnknownToolAndAudits(t *testing.T) {
	audit := &codexToolAuditRecorder{}
	registry := NewCodexToolRegistry(CodexToolRegistryOptions{
		Quota: ToolQuota{MaxRowsPerCall: 5, MaxDeepSearchSteps: 2, MaxTimeout: time.Second},
		Audit: audit,
		Now:   fixedCodexToolNow,
	})

	_, err := registry.Execute(context.Background(), CodexToolCall{
		SessionID: "session_unknown",
		TenantID:  "tenant_1",
		KBID:      "kb_1",
		Tool:      ReadOnlyToolName("write_chunk"),
		MaxRows:   1,
	})
	if !errors.Is(err, ErrCodexUnknownTool) {
		t.Fatalf("Execute() error = %v, want %v", err, ErrCodexUnknownTool)
	}
	if len(audit.events) != 1 || audit.events[0].Allowed || audit.events[0].Error == "" {
		t.Fatalf("audit events = %#v, want denied unknown tool audit", audit.events)
	}
}

func TestCodexToolRegistryRejectsQuotaExceeded(t *testing.T) {
	audit := &codexToolAuditRecorder{}
	registry := NewCodexToolRegistry(CodexToolRegistryOptions{
		Quota: ToolQuota{MaxRowsPerCall: 1, MaxDeepSearchSteps: 3, MaxTimeout: time.Second},
		Audit: audit,
		Now:   fixedCodexToolNow,
	})

	_, err := registry.Execute(context.Background(), CodexToolCall{
		SessionID: "session_quota",
		TenantID:  "tenant_1",
		KBID:      "kb_1",
		Tool:      ReadOnlyToolSearchChunksByText,
		Query:     "ORAG",
		MaxRows:   2,
	})
	if !errors.Is(err, ErrCodexQuotaExceeded) {
		t.Fatalf("Execute() error = %v, want %v", err, ErrCodexQuotaExceeded)
	}
	if len(audit.events) != 1 || audit.events[0].Allowed || audit.events[0].Steps != 1 {
		t.Fatalf("audit events = %#v, want denied quota audit with one reserved step", audit.events)
	}
}

func TestCodexToolRegistryAuditsSuccessfulToolCall(t *testing.T) {
	ctx := context.Background()
	store := kb.NewMemoryStore()
	doc := kb.Document{
		ID:              "doc_1",
		TenantID:        "tenant_1",
		KnowledgeBaseID: "kb_1",
		SourceURI:       "memory://orag.md",
		Title:           "orag.md",
		ContentHash:     "doc-hash-v1",
		CreatedAt:       fixedCodexToolNow(),
	}
	if err := store.Store(ctx, doc, []kb.Chunk{{
		ID:              "chunk_1",
		TenantID:        "tenant_1",
		KnowledgeBaseID: "kb_1",
		DocumentID:      "doc_1",
		Content:         "ORAG supports offline knowledge optimization.",
		SourceURI:       doc.SourceURI,
	}}); err != nil {
		t.Fatal(err)
	}
	audit := &codexToolAuditRecorder{}
	registry := NewCodexToolRegistry(CodexToolRegistryOptions{
		Retriever:   kb.SparseRetriever{Store: store},
		ChunkSource: store,
		Quota:       ToolQuota{MaxRowsPerCall: 5, MaxDeepSearchSteps: 3, MaxTimeout: time.Second},
		Audit:       audit,
		Now:         fixedCodexToolNow,
	})

	got, err := registry.Execute(ctx, CodexToolCall{
		SessionID: "session_success",
		TenantID:  "tenant_1",
		KBID:      "kb_1",
		Tool:      ReadOnlyToolSearchChunksByText,
		Query:     "offline knowledge",
		MaxRows:   2,
		Timeout:   time.Second,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(got.Rows) != 1 || got.Rows[0].ChunkID != "chunk_1" || got.Rows[0].DocVersion != "doc-hash-v1" {
		t.Fatalf("Execute() rows = %#v, want scoped chunk with source metadata", got.Rows)
	}
	if len(audit.events) != 1 || !audit.events[0].Allowed || audit.events[0].Rows != 1 || audit.events[0].Steps != 1 {
		t.Fatalf("audit events = %#v, want successful tool audit", audit.events)
	}
}

func TestCodexToolRegistryPersistsSuccessAndDeniedAudits(t *testing.T) {
	ctx := context.Background()
	store := kb.NewMemoryStore()
	doc := kb.Document{
		ID:              "doc_audit",
		TenantID:        "tenant_1",
		KnowledgeBaseID: "kb_1",
		ContentHash:     "doc-hash-v1",
		CreatedAt:       fixedCodexToolNow(),
	}
	if err := store.Store(ctx, doc, []kb.Chunk{{
		ID:              "chunk_audit",
		TenantID:        "tenant_1",
		KnowledgeBaseID: "kb_1",
		DocumentID:      doc.ID,
		Content:         "ORAG audit sink persists tool calls.",
	}}); err != nil {
		t.Fatal(err)
	}
	repo := NewMemoryRepository()
	registry := NewCodexToolRegistry(CodexToolRegistryOptions{
		Retriever:   kb.SparseRetriever{Store: store},
		ChunkSource: store,
		Quota:       ToolQuota{MaxRowsPerCall: 5, MaxDeepSearchSteps: 3, MaxTimeout: time.Second},
		Audit:       repo,
		Now:         fixedCodexToolNow,
	})

	if _, err := registry.Execute(ctx, CodexToolCall{
		SessionID: "session_audit",
		TenantID:  "tenant_1",
		KBID:      "kb_1",
		Tool:      ReadOnlyToolSearchChunksByText,
		Query:     "audit sink",
		MaxRows:   2,
	}); err != nil {
		t.Fatalf("Execute() success call error = %v", err)
	}
	if _, err := registry.Execute(ctx, CodexToolCall{
		SessionID: "session_audit",
		TenantID:  "tenant_1",
		KBID:      "kb_1",
		Tool:      ReadOnlyToolSearchChunksByText,
		Query:     "audit sink",
		MaxRows:   2,
	}); err != nil {
		t.Fatalf("Execute() repeated success call error = %v", err)
	}
	_, err := registry.Execute(ctx, CodexToolCall{
		SessionID: "session_audit",
		TenantID:  "tenant_1",
		KBID:      "kb_1",
		Tool:      ReadOnlyToolName("write_chunk"),
		MaxRows:   1,
	})
	if !errors.Is(err, ErrCodexUnknownTool) {
		t.Fatalf("Execute() denied call error = %v, want %v", err, ErrCodexUnknownTool)
	}

	events, err := repo.ListCodexToolAuditEvents(ctx, CodexToolAuditFilter{
		TenantID:  "tenant_1",
		KBID:      "kb_1",
		SessionID: "session_audit",
		Limit:     10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("ListCodexToolAuditEvents() = %#v, want 3 audit events", events)
	}
	var allowedCount int
	seenIDs := map[string]bool{}
	var denied bool
	for _, event := range events {
		if seenIDs[event.ID] {
			t.Fatalf("duplicate audit ID %q in events %#v", event.ID, events)
		}
		seenIDs[event.ID] = true
		if event.Allowed && event.Rows == 1 && event.Steps > 0 {
			allowedCount++
		}
		if !event.Allowed && event.Error != "" && event.Tool == ReadOnlyToolName("write_chunk") {
			denied = true
		}
	}
	if allowedCount != 2 || !denied {
		t.Fatalf("audit events = %#v, want two allowed and one denied event", events)
	}
}

func TestCodexToolRegistryRejectsReplayScopeViolation(t *testing.T) {
	registry := NewCodexToolRegistry(CodexToolRegistryOptions{
		Replayer: &codexToolReplayStub{},
		Quota:    ToolQuota{MaxRowsPerCall: 5, MaxDeepSearchSteps: 3, MaxTimeout: time.Second},
	})

	_, err := registry.Execute(context.Background(), CodexToolCall{
		SessionID: "session_scope",
		TenantID:  "tenant_1",
		KBID:      "kb_1",
		Tool:      ReadOnlyToolReplayRecall,
		Cluster:   replayCluster("tenant_2", "kb_1", "ORAG"),
		MaxRows:   1,
	})
	if !errors.Is(err, ErrCodexToolScopeViolation) {
		t.Fatalf("Execute() error = %v, want %v", err, ErrCodexToolScopeViolation)
	}
}

func TestCodexToolRegistryRejectsSameTenantOverQPS(t *testing.T) {
	clock := &codexToolClock{now: fixedCodexToolNow()}
	audit := &codexToolAuditRecorder{}
	registry := NewCodexToolRegistry(CodexToolRegistryOptions{
		Replayer: &codexToolReplayStub{},
		Quota: ToolQuota{
			MaxRowsPerCall:     5,
			MaxDeepSearchSteps: 10,
			MaxTimeout:         time.Second,
			MaxQPSPerTenant:    2,
		},
		Audit: audit,
		Now:   clock.Now,
	})

	for i := 0; i < 2; i++ {
		if _, err := registry.Execute(context.Background(), codexReplayToolCall("tenant_1")); err != nil {
			t.Fatalf("Execute() call %d error = %v, want nil", i+1, err)
		}
	}
	_, err := registry.Execute(context.Background(), codexReplayToolCall("tenant_1"))
	if !errors.Is(err, ErrCodexQuotaExceeded) {
		t.Fatalf("Execute() error = %v, want %v", err, ErrCodexQuotaExceeded)
	}
	if len(audit.events) != 3 || audit.events[2].Allowed || audit.events[2].Error == "" {
		t.Fatalf("audit events = %#v, want third call denied by qps", audit.events)
	}
}

func TestCodexToolRegistryQPSTenantIsolation(t *testing.T) {
	clock := &codexToolClock{now: fixedCodexToolNow()}
	audit := &codexToolAuditRecorder{}
	registry := NewCodexToolRegistry(CodexToolRegistryOptions{
		Replayer: &codexToolReplayStub{},
		Quota: ToolQuota{
			MaxRowsPerCall:     5,
			MaxDeepSearchSteps: 10,
			MaxTimeout:         time.Second,
			MaxQPSPerTenant:    1,
		},
		Audit: audit,
		Now:   clock.Now,
	})

	if _, err := registry.Execute(context.Background(), codexReplayToolCall("tenant_1")); err != nil {
		t.Fatalf("Execute() tenant_1 error = %v, want nil", err)
	}
	if _, err := registry.Execute(context.Background(), codexReplayToolCall("tenant_2")); err != nil {
		t.Fatalf("Execute() tenant_2 error = %v, want nil", err)
	}
	_, err := registry.Execute(context.Background(), codexReplayToolCall("tenant_1"))
	if !errors.Is(err, ErrCodexQuotaExceeded) {
		t.Fatalf("Execute() tenant_1 second error = %v, want %v", err, ErrCodexQuotaExceeded)
	}
	if len(audit.events) != 3 || !audit.events[0].Allowed || !audit.events[1].Allowed || audit.events[2].Allowed {
		t.Fatalf("audit events = %#v, want only tenant_1 second call denied", audit.events)
	}
}

func TestCodexToolRegistryQPSResetsNextSecond(t *testing.T) {
	clock := &codexToolClock{now: fixedCodexToolNow()}
	audit := &codexToolAuditRecorder{}
	registry := NewCodexToolRegistry(CodexToolRegistryOptions{
		Replayer: &codexToolReplayStub{},
		Quota: ToolQuota{
			MaxRowsPerCall:     5,
			MaxDeepSearchSteps: 10,
			MaxTimeout:         time.Second,
			MaxQPSPerTenant:    1,
		},
		Audit: audit,
		Now:   clock.Now,
	})

	if _, err := registry.Execute(context.Background(), codexReplayToolCall("tenant_1")); err != nil {
		t.Fatalf("Execute() first error = %v, want nil", err)
	}
	_, err := registry.Execute(context.Background(), codexReplayToolCall("tenant_1"))
	if !errors.Is(err, ErrCodexQuotaExceeded) {
		t.Fatalf("Execute() same second error = %v, want %v", err, ErrCodexQuotaExceeded)
	}
	clock.Advance(time.Second)
	if _, err := registry.Execute(context.Background(), codexReplayToolCall("tenant_1")); err != nil {
		t.Fatalf("Execute() next second error = %v, want nil", err)
	}
	if len(audit.events) != 3 || !audit.events[0].Allowed || audit.events[1].Allowed || !audit.events[2].Allowed {
		t.Fatalf("audit events = %#v, want qps reset after second changes", audit.events)
	}
}

func validCodexAnalyzeResponse() CodexAnalyzeResponse {
	return CodexAnalyzeResponse{
		ItemType:          ItemTypeAnswer,
		RecommendedAction: RecommendedActionCreateAnswerItem,
		RecallQuality:     RecallQualityMiss,
		FailureType:       FailureTypeSemanticGap,
		Confidence:        0.92,
		FinalAnswer:       "ORAG is a retrieval augmented generation framework.",
		Evidence: []Evidence{
			{ChunkID: "chunk_1", DocID: "doc_1", Quote: "ORAG is a retrieval augmented generation framework", Supports: "definition"},
		},
		MissingEvidence: []string{"deployment checklist"},
		DeepSearchSteps: []DeepSearchStep{
			{Step: 1, Tool: string(ReadOnlyToolSearchChunksByText), Query: "ORAG", Observation: "definition found", Decision: "keep"},
			{Step: 2, Tool: string(ReadOnlyToolReplayRecall), Query: "What is ORAG?", Observation: "baseline missed doc_1", Decision: "create answer item"},
		},
		ToolUsage: []ToolUsage{
			{Tool: ReadOnlyToolSearchChunksByText, Tokens: 400, Rows: 8, QPS: 2, Timeout: time.Second, Steps: 1},
			{Tool: ReadOnlyToolReplayRecall, Tokens: 300, Rows: 5, QPS: 1, Timeout: 2 * time.Second, Steps: 1},
		},
	}
}

func validCodexAnalyzeRequest() CodexAnalyzeRequest {
	return CodexAnalyzeRequest{
		TenantID:              "tenant_1",
		KBID:                  "kb_1",
		CanonicalQuestion:     "What is ORAG?",
		SampleQuestions:       []string{"What is ORAG?"},
		BaselineRecallResults: []BaselineRecallItem{{ChunkID: "chunk_1", DocID: "doc_1", Rank: 1, Score: 0.9, Matched: true}},
		Constraints: CodexConstraints{
			ReadOnlyTools:      allowedReadOnlyTools(),
			Quota:              testCodexQuota(),
			RequireEvidence:    true,
			MaxDeepSearchSteps: 2,
			AllowedItemTypes:   []ItemType{ItemTypeAnswer, ItemTypeQueryRewrite, ItemTypeKnowledgeGap},
			AllowedActions:     []RecommendedAction{RecommendedActionCreateAnswerItem, RecommendedActionNeedsReview, RecommendedActionReject},
		},
	}
}

func testCodexQuota() ToolQuota {
	return ToolQuota{
		MaxTokens:          1000,
		MaxRowsPerCall:     20,
		MaxQPSPerTenant:    5,
		MaxTimeout:         5 * time.Second,
		MaxDeepSearchSteps: 4,
	}
}

type codexToolAuditRecorder struct {
	events []CodexToolAuditEvent
}

func (r *codexToolAuditRecorder) RecordCodexToolAudit(_ context.Context, event CodexToolAuditEvent) error {
	r.events = append(r.events, event)
	return nil
}

type codexToolReplayStub struct{}

func (*codexToolReplayStub) ReplayRecall(_ context.Context, cluster QuestionCluster) (RecallReplayResult, error) {
	return RecallReplayResult{
		BaselineRecallResults: []BaselineRecallItem{{ChunkID: "chunk_1", DocID: "doc_1", Rank: 1, Matched: true}},
		Metadata:              map[string]any{"query": cluster.CanonicalQuestion},
	}, nil
}

func fixedCodexToolNow() time.Time {
	return time.Date(2026, 7, 8, 15, 0, 0, 0, time.UTC)
}

type codexToolClock struct {
	now time.Time
}

func (c *codexToolClock) Now() time.Time {
	return c.now
}

func (c *codexToolClock) Advance(delta time.Duration) {
	c.now = c.now.Add(delta)
}

func codexReplayToolCall(tenantID string) CodexToolCall {
	return CodexToolCall{
		SessionID: tenantID + "_session",
		TenantID:  tenantID,
		KBID:      "kb_1",
		Tool:      ReadOnlyToolReplayRecall,
		Query:     "What is ORAG?",
		MaxRows:   1,
		Timeout:   time.Second,
	}
}
