package optimizer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shikanon/orag/internal/eval"
	"github.com/shikanon/orag/internal/kb"
	"github.com/shikanon/orag/internal/rag"
)

func TestInternalRAGRunnerAppliesCandidateToClonedService(t *testing.T) {
	baseRetriever := &kb.HybridRetriever{DenseTopK: 3, SparseTopK: 4, RRFK: 30}
	base := &rag.Service{
		Retriever:              baseRetriever,
		Packer:                 rag.ContextPacker{TopN: 2},
		TopK:                   5,
		SemanticCacheThreshold: 0.88,
		SemanticCacheNamespace: "production",
		RRFK:                   60,
		MultiQueryCount:        1,
	}
	candidate := CandidateConfig{
		ID:       "cand_internal",
		Chunking: ChunkingCandidate{Enabled: true, SizeTokens: 800},
		Reranker: RerankerCandidate{TopN: 8},
		Retrieval: RetrievalCandidate{
			DenseTopK:              20,
			SparseTopK:             10,
			RRFK:                   90,
			SemanticCacheThreshold: 0.96,
		},
		Graph: GraphCandidate{QueryRewriteEnabled: boolPtr(true), HyDEEnabled: boolPtr(true), MultiQueryCount: 3},
	}
	now := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)
	namespaces := NewTempNamespaceManager(nil)
	namespaces.now = func() time.Time { return now }

	var capturedService *rag.Service
	var capturedRequest eval.RunRequest
	runner := InternalRAGRunner{
		BaseRAG:    base,
		Namespaces: namespaces,
		BuildEvaluationRunner: func(service *rag.Service) EvaluationRunner {
			capturedService = service
			return fakeEvaluationRunner{
				run: eval.RunResult{
					ID:      "eval_candidate",
					Metrics: map[string]float64{"accuracy": 0.8, eval.PrimaryMetricDeterministicAnswerMatch: 0.8},
				},
				req: &capturedRequest,
			}
		},
	}

	result, err := runner.RunCandidate(context.Background(), CandidateRunRequest{
		TenantID:        "tenant_a",
		DatasetID:       "ds_1",
		KnowledgeBaseID: "kb_1",
		Candidate:       candidate,
		Profile:         rag.ProfileHighPrecision,
		NamespaceTTL:    30 * time.Minute,
	})
	if err != nil {
		t.Fatalf("RunCandidate() error = %v", err)
	}

	if result.CandidateID != candidate.ID {
		t.Fatalf("candidate id = %q, want %q", result.CandidateID, candidate.ID)
	}
	if result.CleanupStatus != CleanupPending || len(result.TempNamespaces) != 1 {
		t.Fatalf("temp namespaces = %#v, cleanup = %q; want one pending namespace", result.TempNamespaces, result.CleanupStatus)
	}
	if result.TempNamespaces[0].OwnerID != candidate.ID || !result.TempNamespaces[0].ExpiresAt.Equal(now.Add(30*time.Minute)) {
		t.Fatalf("namespace metadata = %#v, want owner and ttl", result.TempNamespaces[0])
	}
	if capturedRequest.TopK != 20 || capturedRequest.Profile != rag.ProfileHighPrecision {
		t.Fatalf("eval request = %#v, want candidate top_k and requested profile", capturedRequest)
	}
	if capturedService == nil {
		t.Fatal("candidate service was not captured")
	}
	if capturedService == base {
		t.Fatal("candidate service reused production service pointer")
	}
	if capturedService.Packer.TopN != 8 || capturedService.TopK != 20 || capturedService.RRFK != 90 {
		t.Fatalf("candidate service = %#v, want reranker/retrieval overrides", capturedService)
	}
	if capturedService.SemanticCacheThreshold != 0.96 || !capturedService.QueryRewriteEnabled || !capturedService.HyDEEnabled || capturedService.MultiQueryCount != 3 {
		t.Fatalf("candidate graph/cache settings not applied: %#v", capturedService)
	}
	if capturedService.SemanticCacheNamespace != "optimizer_candidate:"+candidate.ID {
		t.Fatalf("candidate semantic cache namespace = %q, want candidate namespace", capturedService.SemanticCacheNamespace)
	}
	clonedHybrid, ok := capturedService.Retriever.(*kb.HybridRetriever)
	if !ok {
		t.Fatalf("candidate retriever type = %T, want cloned hybrid retriever", capturedService.Retriever)
	}
	if clonedHybrid == baseRetriever {
		t.Fatal("candidate retriever reused production hybrid retriever pointer")
	}
	if clonedHybrid.DenseTopK != 20 || clonedHybrid.SparseTopK != 10 || clonedHybrid.RRFK != 90 {
		t.Fatalf("candidate retriever = %#v, want retrieval overrides", clonedHybrid)
	}

	if base.Packer.TopN != 2 || base.TopK != 5 || base.SemanticCacheThreshold != 0.88 || base.SemanticCacheNamespace != "production" || base.RRFK != 60 || base.QueryRewriteEnabled || base.HyDEEnabled || base.MultiQueryCount != 1 {
		t.Fatalf("base RAG was mutated: %#v", base)
	}
	if baseRetriever.DenseTopK != 3 || baseRetriever.SparseTopK != 4 || baseRetriever.RRFK != 30 {
		t.Fatalf("base retriever was mutated: %#v", baseRetriever)
	}
}

func TestInternalRAGRunnerPreservesGraphEnhancementsWhenCandidateOmitsGraphBooleans(t *testing.T) {
	base := &rag.Service{
		TopK:                   5,
		QueryRewriteEnabled:    true,
		HyDEEnabled:            true,
		SemanticCacheThreshold: 0.88,
	}
	runner := InternalRAGRunner{BaseRAG: base}

	cloned := runner.configureCandidateService(CandidateConfig{
		Retrieval: RetrievalCandidate{DenseTopK: 12},
	})

	if cloned == base {
		t.Fatal("candidate service reused production service pointer")
	}
	if !cloned.QueryRewriteEnabled || !cloned.HyDEEnabled {
		t.Fatalf("graph enhancements = rewrite:%t hyde:%t, want preserved true values", cloned.QueryRewriteEnabled, cloned.HyDEEnabled)
	}
	if cloned.TopK != 12 {
		t.Fatalf("top_k = %d, want retrieval candidate override", cloned.TopK)
	}
	if !base.QueryRewriteEnabled || !base.HyDEEnabled || base.TopK != 5 {
		t.Fatalf("base RAG was mutated: %#v", base)
	}
}

func TestInternalRAGRunnerAppliesExplicitFalseGraphCandidate(t *testing.T) {
	base := &rag.Service{
		QueryRewriteEnabled: true,
		HyDEEnabled:         true,
	}
	runner := InternalRAGRunner{BaseRAG: base}

	cloned := runner.configureCandidateService(CandidateConfig{
		Graph: GraphCandidate{
			QueryRewriteEnabled: boolPtr(false),
			HyDEEnabled:         boolPtr(false),
		},
	})

	if cloned.QueryRewriteEnabled || cloned.HyDEEnabled {
		t.Fatalf("graph enhancements = rewrite:%t hyde:%t, want explicit false overrides", cloned.QueryRewriteEnabled, cloned.HyDEEnabled)
	}
	if !base.QueryRewriteEnabled || !base.HyDEEnabled {
		t.Fatalf("base RAG was mutated: %#v", base)
	}
}

func TestInternalRAGRunnerAssignsDeterministicSemanticCacheNamespace(t *testing.T) {
	candidate := CandidateConfig{
		Retrieval: RetrievalCandidate{RRFK: 90},
	}
	wantCandidate := candidate.WithDeterministicID("internal_rag")
	base := &rag.Service{}
	var capturedService *rag.Service
	runner := InternalRAGRunner{
		BaseRAG: base,
		BuildEvaluationRunner: func(service *rag.Service) EvaluationRunner {
			capturedService = service
			return fakeEvaluationRunner{run: eval.RunResult{ID: "eval_candidate"}}
		},
	}

	result, err := runner.RunCandidate(context.Background(), CandidateRunRequest{
		TenantID:        "tenant_a",
		DatasetID:       "ds_1",
		KnowledgeBaseID: "kb_1",
		Candidate:       candidate,
	})
	if err != nil {
		t.Fatalf("RunCandidate() error = %v", err)
	}
	if result.CandidateID != wantCandidate.ID {
		t.Fatalf("candidate id = %q, want deterministic %q", result.CandidateID, wantCandidate.ID)
	}
	if capturedService == nil {
		t.Fatal("candidate service was not captured")
	}
	if capturedService.SemanticCacheNamespace != "optimizer_candidate:"+wantCandidate.ID {
		t.Fatalf("candidate semantic cache namespace = %q, want deterministic candidate namespace", capturedService.SemanticCacheNamespace)
	}
	if base.SemanticCacheNamespace != "" {
		t.Fatalf("base semantic cache namespace = %q, want unchanged empty namespace", base.SemanticCacheNamespace)
	}
}

func TestInternalRAGRunnerClearsBasePipelineForCandidateClone(t *testing.T) {
	base := &rag.Service{
		TopK:     5,
		Pipeline: fakePipeline{},
	}
	runner := InternalRAGRunner{BaseRAG: base}

	cloned := runner.configureCandidateService(CandidateConfig{
		Retrieval: RetrievalCandidate{DenseTopK: 12},
	})
	if cloned == base {
		t.Fatal("candidate service reused base pointer")
	}
	if cloned.Pipeline != nil {
		t.Fatalf("candidate pipeline = %#v, want nil to apply candidate overrides", cloned.Pipeline)
	}
	if base.Pipeline == nil {
		t.Fatal("base pipeline was mutated")
	}
}

func TestTempNamespaceManagerGCAndOwnerCleanup(t *testing.T) {
	now := time.Date(2026, 7, 4, 11, 0, 0, 0, time.UTC)
	cleaner := &recordingNamespaceCleaner{}
	manager := NewTempNamespaceManager(cleaner)
	manager.now = func() time.Time { return now }

	expired := manager.Register("cand_a", "index", "ns_expired", time.Minute)
	future := manager.Register("cand_b", "index", "ns_future", time.Hour)
	manager.now = func() time.Time { return now.Add(2 * time.Minute) }

	cleaned, err := manager.GC(context.Background())
	if err != nil {
		t.Fatalf("GC() error = %v", err)
	}
	if len(cleaned) != 1 || cleaned[0].Name != expired.Name || cleaned[0].Status != CleanupDone {
		t.Fatalf("GC cleaned = %#v, want only expired namespace done", cleaned)
	}
	if len(cleaner.deleted) != 1 || cleaner.deleted[0] != expired.Name {
		t.Fatalf("deleted namespaces = %#v, want expired namespace", cleaner.deleted)
	}
	for _, namespace := range manager.ListByOwner("cand_b") {
		if namespace.Name == future.Name && namespace.Status != CleanupPending {
			t.Fatalf("future namespace status = %q, want pending", namespace.Status)
		}
	}

	cleaned, err = manager.CleanupOwner(context.Background(), "cand_b")
	if err != nil {
		t.Fatalf("CleanupOwner() error = %v", err)
	}
	if len(cleaned) != 1 || cleaned[0].Name != future.Name || cleaned[0].Status != CleanupDone {
		t.Fatalf("owner cleanup = %#v, want future namespace done", cleaned)
	}
}

type fakePipeline struct{}

func (fakePipeline) Invoke(context.Context, rag.QueryRequest) (rag.QueryResponse, error) {
	return rag.QueryResponse{}, nil
}

func TestTempNamespaceManagerRecordsCleanupFailure(t *testing.T) {
	manager := NewTempNamespaceManager(failingNamespaceCleaner{})
	manager.now = func() time.Time { return time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC) }
	manager.Register("cand_failed", "index", "ns_failed", time.Minute)
	manager.now = func() time.Time { return time.Date(2026, 7, 4, 12, 2, 0, 0, time.UTC) }

	cleaned, err := manager.GC(context.Background())
	if err == nil {
		t.Fatal("GC() error = nil, want cleaner failure")
	}
	if len(cleaned) != 1 || cleaned[0].Status != CleanupFailed || cleaned[0].Error == "" {
		t.Fatalf("failed cleanup = %#v, want failed status with error", cleaned)
	}
}

type fakeEvaluationRunner struct {
	run eval.RunResult
	req *eval.RunRequest
}

func (r fakeEvaluationRunner) Run(_ context.Context, req eval.RunRequest) (eval.RunResult, error) {
	if r.req != nil {
		*r.req = req
	}
	return r.run, nil
}

type recordingNamespaceCleaner struct {
	deleted []string
}

func (c *recordingNamespaceCleaner) DeleteTempNamespace(_ context.Context, namespace TempNamespace) error {
	c.deleted = append(c.deleted, namespace.Name)
	return nil
}

type failingNamespaceCleaner struct{}

func (failingNamespaceCleaner) DeleteTempNamespace(context.Context, TempNamespace) error {
	return errors.New("delete failed")
}

func boolPtr(value bool) *bool {
	return &value
}
