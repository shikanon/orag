package offlineknowledge

import (
	"encoding/json"
	"testing"
)

func TestCanTransition(t *testing.T) {
	tests := []struct {
		name string
		from ItemStatus
		to   ItemStatus
		want bool
	}{
		{
			name: "candidate enters evidence validation",
			from: ItemStatusCandidate,
			to:   ItemStatusEvidenceValidating,
			want: true,
		},
		{
			name: "evidence validation can verify item",
			from: ItemStatusEvidenceValidating,
			to:   ItemStatusVerified,
			want: true,
		},
		{
			name: "evidence validation can require manual review",
			from: ItemStatusEvidenceValidating,
			to:   ItemStatusNeedsReview,
			want: true,
		},
		{
			name: "evidence validation can classify knowledge gap",
			from: ItemStatusEvidenceValidating,
			to:   ItemStatusKnowledgeGap,
			want: true,
		},
		{
			name: "evidence validation can reject item",
			from: ItemStatusEvidenceValidating,
			to:   ItemStatusRejected,
			want: true,
		},
		{
			name: "manual review can verify item",
			from: ItemStatusNeedsReview,
			to:   ItemStatusVerified,
			want: true,
		},
		{
			name: "manual review can reject item",
			from: ItemStatusNeedsReview,
			to:   ItemStatusRejected,
			want: true,
		},
		{
			name: "verified can enable shadow retrieval",
			from: ItemStatusVerified,
			to:   ItemStatusShadowEnabled,
			want: true,
		},
		{
			name: "verified can be manually rejected",
			from: ItemStatusVerified,
			to:   ItemStatusRejected,
			want: true,
		},
		{
			name: "shadow retrieval can pass regression",
			from: ItemStatusShadowEnabled,
			to:   ItemStatusRegressionPassed,
			want: true,
		},
		{
			name: "shadow retrieval can fail regression",
			from: ItemStatusShadowEnabled,
			to:   ItemStatusRegressionFailed,
			want: true,
		},
		{
			name: "regression failed can recover to verified",
			from: ItemStatusRegressionFailed,
			to:   ItemStatusVerified,
			want: true,
		},
		{
			name: "regression failed can go to review",
			from: ItemStatusRegressionFailed,
			to:   ItemStatusNeedsReview,
			want: true,
		},
		{
			name: "regression failed can reject item",
			from: ItemStatusRegressionFailed,
			to:   ItemStatusRejected,
			want: true,
		},
		{
			name: "regression passed can publish",
			from: ItemStatusRegressionPassed,
			to:   ItemStatusPublished,
			want: true,
		},
		{
			name: "regression passed can be rejected before publish",
			from: ItemStatusRegressionPassed,
			to:   ItemStatusRejected,
			want: true,
		},
		{
			name: "regression passed can be deprecated before publish",
			from: ItemStatusRegressionPassed,
			to:   ItemStatusDeprecated,
			want: true,
		},
		{
			name: "published can become stale",
			from: ItemStatusPublished,
			to:   ItemStatusStale,
			want: true,
		},
		{
			name: "published can be deprecated",
			from: ItemStatusPublished,
			to:   ItemStatusDeprecated,
			want: true,
		},
		{
			name: "stale can be revalidated",
			from: ItemStatusStale,
			to:   ItemStatusEvidenceValidating,
			want: true,
		},
		{
			name: "stale can be deprecated",
			from: ItemStatusStale,
			to:   ItemStatusDeprecated,
			want: true,
		},
		{
			name: "rejected can be manually reopened",
			from: ItemStatusRejected,
			to:   ItemStatusCandidate,
			want: true,
		},
		{
			name: "knowledge gap can reopen after documents are added",
			from: ItemStatusKnowledgeGap,
			to:   ItemStatusCandidate,
			want: true,
		},
		{
			name: "candidate cannot skip to verified",
			from: ItemStatusCandidate,
			to:   ItemStatusVerified,
			want: false,
		},
		{
			name: "verified cannot publish without shadow and regression",
			from: ItemStatusVerified,
			to:   ItemStatusPublished,
			want: false,
		},
		{
			name: "published cannot go back to candidate",
			from: ItemStatusPublished,
			to:   ItemStatusCandidate,
			want: false,
		},
		{
			name: "deprecated cannot recover automatically",
			from: ItemStatusDeprecated,
			to:   ItemStatusCandidate,
			want: false,
		},
		{
			name: "self transition is rejected",
			from: ItemStatusCandidate,
			to:   ItemStatusCandidate,
			want: false,
		},
		{
			name: "unknown source status is rejected",
			from: ItemStatus("unknown"),
			to:   ItemStatusCandidate,
			want: false,
		},
		{
			name: "unknown target status is rejected",
			from: ItemStatusCandidate,
			to:   ItemStatus("unknown"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CanTransition(tt.from, tt.to); got != tt.want {
				t.Fatalf("CanTransition(%q, %q) = %v, want %v", tt.from, tt.to, got, tt.want)
			}
		})
	}
}

func TestFailureTypeValuesMatchDesign(t *testing.T) {
	tests := []struct {
		name string
		got  FailureType
		want string
	}{
		{name: "keyword mismatch", got: FailureTypeKeywordMismatch, want: "keyword_mismatch"},
		{name: "semantic gap", got: FailureTypeSemanticGap, want: "semantic_gap"},
		{name: "chunk boundary", got: FailureTypeChunkBoundary, want: "chunk_boundary"},
		{name: "rerank error", got: FailureTypeRerankError, want: "rerank_error"},
		{name: "graph missing", got: FailureTypeGraphMissing, want: "graph_missing"},
		{name: "generation error", got: FailureTypeGenerationError, want: "generation_error"},
		{name: "knowledge gap", got: FailureTypeKnowledgeGap, want: "knowledge_gap"},
		{name: "unclear question", got: FailureTypeUnclearQuestion, want: "unclear_question"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.got) != tt.want {
				t.Fatalf("FailureType value = %q, want %q", tt.got, tt.want)
			}
		})
	}
}

func TestQuestionClusterEmbeddingJSON(t *testing.T) {
	cluster := QuestionCluster{
		ID:            "cluster-1",
		EmbeddingJSON: []float64{0.125, -0.5, 1},
	}

	data, err := json.Marshal(cluster)
	if err != nil {
		t.Fatalf("marshal QuestionCluster: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal QuestionCluster JSON: %v", err)
	}
	embedding, ok := got["embedding_json"].([]any)
	if !ok {
		t.Fatalf("embedding_json missing or has wrong type: %#v", got["embedding_json"])
	}
	if len(embedding) != 3 {
		t.Fatalf("embedding_json length = %d, want 3", len(embedding))
	}
	if embedding[0] != 0.125 || embedding[1] != -0.5 || embedding[2] != float64(1) {
		t.Fatalf("embedding_json = %#v, want [0.125 -0.5 1]", embedding)
	}
}

func TestOfflineKnowledgeRunConfigJSON(t *testing.T) {
	run := OfflineKnowledgeRun{
		ID:         "run_1",
		TenantID:   "tenant_1",
		KBID:       "kb_1",
		Status:     RunStatusPending,
		ConfigHash: "config_hash",
		ConfigJSON: map[string]any{
			"lookback_days": float64(7),
			"shadow":        map[string]any{"enabled": true},
		},
	}

	data, err := json.Marshal(run)
	if err != nil {
		t.Fatalf("marshal OfflineKnowledgeRun: %v", err)
	}
	var decoded OfflineKnowledgeRun
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal OfflineKnowledgeRun: %v", err)
	}
	if decoded.ConfigJSON["lookback_days"] != float64(7) {
		t.Fatalf("lookback_days = %#v, want 7", decoded.ConfigJSON["lookback_days"])
	}
	shadow, ok := decoded.ConfigJSON["shadow"].(map[string]any)
	if !ok || shadow["enabled"] != true {
		t.Fatalf("shadow config = %#v", decoded.ConfigJSON["shadow"])
	}
}
