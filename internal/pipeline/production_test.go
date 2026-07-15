package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/cloudwego/eino/compose"
	"github.com/shikanon/orag/internal/graph"
	"github.com/shikanon/orag/internal/kb"
	"github.com/shikanon/orag/internal/llm/ark"
	"github.com/shikanon/orag/internal/prompt"
	"github.com/shikanon/orag/internal/rag"
	"github.com/shikanon/orag/internal/release"
)

func TestProductionRunnerResolvesActiveFrozenVersionAndLineage(t *testing.T) {
	ctx := context.Background()
	repo := release.NewMemoryRepository("prj_1")
	definition, err := json.Marshal(validDefinition())
	if err != nil {
		t.Fatal(err)
	}
	repo.PutVersion(release.Version{ID: "pver_1", ProjectID: "prj_1", PipelineID: "pipe_1", ContentHash: "hash_1", Definition: definition})
	production, err := repo.Environment(ctx, "prj_1", release.Production)
	if err != nil {
		t.Fatal(err)
	}
	production.ActiveVersionID = "pver_1"
	production.ActiveReleaseID = "rel_1"
	production.Revision++
	repo.SetEnvironment(production)
	executor := &recordingCompiledExecutor{}
	runner := ProductionRunner{
		Release:  release.NewService(repo),
		Compiler: NewCompiler(&rag.Service{}, BuiltinRegistry()),
		Executor: executor,
	}

	response, err := runner.Query(ctx, rag.QueryRequest{ProjectID: "prj_1", TenantID: "tenant_1", KnowledgeBaseID: "kb_1", TraceID: "trace_1", Query: "How do I deploy?", TopK: 7, Profile: rag.ProfileHighPrecision})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if response.Answer != "executed" || executor.runnable == nil {
		t.Fatalf("Query() response/executor = %#v %#v", response, executor)
	}
	got := executor.request
	if got.ProjectID != "prj_1" || got.PipelineID != "pipe_1" || got.PipelineVersionID != "pver_1" || got.ReleaseID != "rel_1" || got.Environment != "production" || got.TopK != 7 {
		t.Fatalf("execution lineage = %#v", got)
	}
}

func TestProductionRunnerRejectsMissingOrLegacyActiveVersion(t *testing.T) {
	ctx := context.Background()
	repo := release.NewMemoryRepository("prj_1")
	runner := ProductionRunner{Release: release.NewService(repo), Compiler: NewCompiler(&rag.Service{}, BuiltinRegistry()), Executor: &recordingCompiledExecutor{}}
	_, err := runner.Query(ctx, rag.QueryRequest{ProjectID: "prj_1"})
	if !errors.Is(err, ErrProductionVersionUnavailable) {
		t.Fatalf("missing active version error = %v", err)
	}

	production, _ := repo.Environment(ctx, "prj_1", release.Production)
	production.ActiveVersionID = "legacy"
	repo.SetEnvironment(production)
	repo.PutVersion(release.Version{ID: "legacy", ProjectID: "prj_1", ContentHash: "hash"})
	_, err = runner.Query(ctx, rag.QueryRequest{ProjectID: "prj_1"})
	if !errors.Is(err, ErrFrozenVersionInvalid) {
		t.Fatalf("legacy active version error = %v", err)
	}
}

func TestProductionRunnerExecutesFrozenPipelineAndPersistsLineage(t *testing.T) {
	ctx := context.Background()
	svc := productionTestService(t)
	graphRunner, err := graph.NewRAGGraph(ctx, svc)
	if err != nil {
		t.Fatal(err)
	}
	traceStore := &recordingProductionTraceStore{}
	graphRunner.TraceStore = traceStore

	repo := release.NewMemoryRepository("prj_1")
	definition, err := json.Marshal(validDefinition())
	if err != nil {
		t.Fatal(err)
	}
	repo.PutVersion(release.Version{ID: "pver_1", ProjectID: "prj_1", PipelineID: "pipe_1", ContentHash: "hash_1", Definition: definition})
	production, _ := repo.Environment(ctx, "prj_1", release.Production)
	production.ActiveVersionID = "pver_1"
	production.ActiveReleaseID = "rel_1"
	repo.SetEnvironment(production)
	repo.PutEvidence(release.Evidence{ProjectID: "prj_1", VersionID: "pver_1", EnvironmentID: string(release.Production), Passed: true, ContentHash: "hash_1", DatasetID: "ds_1", EvaluationRunID: "eval_1"})
	runner := ProductionRunner{Release: release.NewService(repo), Compiler: NewCompiler(svc, BuiltinRegistry()), Executor: graphRunner}

	response, err := runner.Query(ctx, rag.QueryRequest{TenantID: "tenant_1", TraceID: "trace_frozen_execution", ProjectID: "prj_1", KnowledgeBaseID: "kb_1", Query: "Where are the deployment docs?", TopK: 3})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if response.Answer == "" || response.TraceID != "trace_frozen_execution" {
		t.Fatalf("Query() response = %#v", response)
	}
	if traceStore.input.PipelineVersionID != "pver_1" || traceStore.input.ReleaseID != "rel_1" || traceStore.input.Environment != "production" || traceStore.input.ProjectID != "prj_1" || traceStore.input.DatasetID != "ds_1" || traceStore.input.EvaluationRunID != "eval_1" || traceStore.input.RequestedTopK != 3 {
		t.Fatalf("stored lineage = %#v", traceStore.input)
	}
}

type recordingCompiledExecutor struct {
	runnable compose.Runnable[graph.State, graph.State]
	request  rag.QueryRequest
}

func (e *recordingCompiledExecutor) InvokeCompiled(_ context.Context, runnable compose.Runnable[graph.State, graph.State], request rag.QueryRequest) (rag.QueryResponse, error) {
	e.runnable = runnable
	e.request = request
	return rag.QueryResponse{Answer: "executed", TraceID: request.TraceID}, nil
}

type recordingProductionTraceStore struct {
	input graph.TraceInput
}

func (s *recordingProductionTraceStore) StoreTrace(_ context.Context, input graph.TraceInput) error {
	s.input = input
	return nil
}

func productionTestService(t *testing.T) *rag.Service {
	t.Helper()
	ctx := context.Background()
	store := kb.NewMemoryStore()
	if err := store.Store(ctx, kb.Document{ID: "doc_1", TenantID: "tenant_1", KnowledgeBaseID: "kb_1", SourceURI: "memory://deployment", Title: "Deployment"}, []kb.Chunk{{ID: "chk_1", TenantID: "tenant_1", KnowledgeBaseID: "kb_1", DocumentID: "doc_1", Content: "Deployment documentation is in the runbook.", SourceURI: "memory://deployment"}}); err != nil {
		t.Fatal(err)
	}
	return &rag.Service{
		Retriever:           kb.HybridRetriever{Dense: kb.DenseRetriever{Store: store}, Sparse: kb.SparseRetriever{Store: store}, RRFK: 60, TopN: 8},
		Model:               ark.NewClient(ark.Config{EmbeddingDimensions: 8}, nil),
		Cache:               rag.NewSemanticCache(10),
		Packer:              rag.ContextPacker{MaxTokens: 512, TopN: 4},
		PromptStrategy:      prompt.NewStrategy("auto"),
		DefaultProfile:      rag.ProfileRealtime,
		NoContextAnswer:     "no context",
		TopK:                8,
		QueryRewriteEnabled: true,
		MultiQueryCount:     2,
	}
}
