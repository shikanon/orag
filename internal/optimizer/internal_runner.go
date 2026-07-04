package optimizer

import (
	"context"
	"time"

	"github.com/shikanon/orag/internal/dataset"
	"github.com/shikanon/orag/internal/eval"
	"github.com/shikanon/orag/internal/kb"
	"github.com/shikanon/orag/internal/rag"
)

type InternalRAGRunner struct {
	BaseRAG               *rag.Service
	Datasets              *dataset.Service
	Repository            eval.Repository
	Namespaces            *TempNamespaceManager
	BuildEvaluationRunner func(*rag.Service) EvaluationRunner
}

func (r InternalRAGRunner) RunCandidate(ctx context.Context, req CandidateRunRequest) (CandidateRunResult, error) {
	candidate := req.Candidate
	if candidate.ID == "" {
		candidate = candidate.WithDeterministicID("internal_rag")
	}
	candidateRAG := r.configureCandidateService(candidate)
	namespaces := r.registerTempNamespaces(candidate, req.NamespaceTTL)

	runner := r.evaluationRunner(candidateRAG)
	topK := req.TopK
	if topK <= 0 {
		topK = candidateRAG.TopK
	}
	run, err := runner.Run(ctx, eval.RunRequest{
		TenantID:        req.TenantID,
		DatasetID:       req.DatasetID,
		KnowledgeBaseID: req.KnowledgeBaseID,
		Profile:         req.Profile,
		TopK:            topK,
	})
	if err != nil {
		return CandidateRunResult{}, err
	}

	cleanupStatus := CleanupNotRequired
	if len(namespaces) > 0 {
		cleanupStatus = CleanupPending
	}
	return CandidateRunResult{
		CandidateID:    candidate.ID,
		EvaluationRun:  run,
		Metrics:        cloneMetrics(run.Metrics),
		TempNamespaces: namespaces,
		CleanupStatus:  cleanupStatus,
	}, nil
}

func (r InternalRAGRunner) CleanupCandidateNamespaces(ctx context.Context, candidateID string) ([]TempNamespace, error) {
	if r.Namespaces == nil {
		return nil, nil
	}
	return r.Namespaces.CleanupOwner(ctx, candidateID)
}

func (r InternalRAGRunner) GCNamespaces(ctx context.Context) ([]TempNamespace, error) {
	if r.Namespaces == nil {
		return nil, nil
	}
	return r.Namespaces.GC(ctx)
}

func (r InternalRAGRunner) configureCandidateService(candidate CandidateConfig) *rag.Service {
	if r.BaseRAG == nil {
		return &rag.Service{}
	}
	cloned := *r.BaseRAG
	applyRetrievalCandidate(&cloned, candidate.Retrieval)
	applyRerankerCandidate(&cloned, candidate.Reranker)
	applyGraphCandidate(&cloned, candidate.Graph)
	return &cloned
}

func (r InternalRAGRunner) evaluationRunner(candidateRAG *rag.Service) EvaluationRunner {
	if r.BuildEvaluationRunner != nil {
		return r.BuildEvaluationRunner(candidateRAG)
	}
	return eval.Runner{RAG: candidateRAG, Datasets: r.Datasets, Repository: r.Repository}
}

func (r InternalRAGRunner) registerTempNamespaces(candidate CandidateConfig, ttl time.Duration) []TempNamespace {
	if r.Namespaces == nil || !requiresTempNamespace(candidate) {
		return nil
	}
	name := candidate.Indexing.Namespace
	if name == "" {
		name = defaultNamespaceName(candidate.ID, "index")
	}
	return []TempNamespace{r.Namespaces.Register(candidate.ID, "index", name, ttl)}
}

func requiresTempNamespace(candidate CandidateConfig) bool {
	return candidate.Indexing.Namespace != "" || candidate.Chunking.Enabled || candidate.Embedding.Enabled
}

func applyRetrievalCandidate(service *rag.Service, candidate RetrievalCandidate) {
	if candidate.SemanticCacheThreshold > 0 {
		service.SemanticCacheThreshold = candidate.SemanticCacheThreshold
	}
	if candidate.RRFK > 0 {
		service.RRFK = candidate.RRFK
	}
	if candidate.DenseTopK > 0 || candidate.SparseTopK > 0 {
		service.Retriever = candidateRetriever(service.Retriever, candidate)
	}
	if candidateTopK := maxPositive(candidate.DenseTopK, candidate.SparseTopK); candidateTopK > 0 {
		service.TopK = candidateTopK
	}
}

func applyRerankerCandidate(service *rag.Service, candidate RerankerCandidate) {
	if candidate.TopN <= 0 {
		return
	}
	packer := service.Packer
	packer.TopN = candidate.TopN
	service.Packer = packer
}

func applyGraphCandidate(service *rag.Service, candidate GraphCandidate) {
	service.QueryRewriteEnabled = candidate.QueryRewriteEnabled
	service.HyDEEnabled = candidate.HyDEEnabled
	if candidate.MultiQueryCount > 0 {
		service.MultiQueryCount = candidate.MultiQueryCount
	}
}

func candidateRetriever(base kb.Retriever, candidate RetrievalCandidate) kb.Retriever {
	switch retriever := base.(type) {
	case kb.HybridRetriever:
		retriever.DenseTopK = firstPositive(candidate.DenseTopK, retriever.DenseTopK)
		retriever.SparseTopK = firstPositive(candidate.SparseTopK, retriever.SparseTopK)
		retriever.RRFK = firstPositive(candidate.RRFK, retriever.RRFK)
		return retriever
	case *kb.HybridRetriever:
		cloned := *retriever
		cloned.DenseTopK = firstPositive(candidate.DenseTopK, cloned.DenseTopK)
		cloned.SparseTopK = firstPositive(candidate.SparseTopK, cloned.SparseTopK)
		cloned.RRFK = firstPositive(candidate.RRFK, cloned.RRFK)
		return &cloned
	default:
		return retrievalOverride{base: base, candidate: candidate}
	}
}

type retrievalOverride struct {
	base      kb.Retriever
	candidate RetrievalCandidate
}

func (r retrievalOverride) Retrieve(ctx context.Context, req kb.SearchRequest) ([]kb.SearchResult, error) {
	req = r.apply(req)
	return r.base.Retrieve(ctx, req)
}

func (r retrievalOverride) RetrieveWithWarnings(ctx context.Context, req kb.SearchRequest) ([]kb.SearchResult, []string, error) {
	req = r.apply(req)
	if retriever, ok := r.base.(interface {
		RetrieveWithWarnings(context.Context, kb.SearchRequest) ([]kb.SearchResult, []string, error)
	}); ok {
		return retriever.RetrieveWithWarnings(ctx, req)
	}
	results, err := r.base.Retrieve(ctx, req)
	return results, nil, err
}

func (r retrievalOverride) apply(req kb.SearchRequest) kb.SearchRequest {
	if r.candidate.DenseTopK > 0 {
		req.DenseTopK = r.candidate.DenseTopK
	}
	if r.candidate.SparseTopK > 0 {
		req.SparseTopK = r.candidate.SparseTopK
	}
	if req.TopK <= 0 {
		req.TopK = firstPositive(r.candidate.DenseTopK, r.candidate.SparseTopK)
	}
	return req
}

func firstPositive(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func maxPositive(values ...int) int {
	maximum := 0
	for _, value := range values {
		if value > maximum {
			maximum = value
		}
	}
	return maximum
}
