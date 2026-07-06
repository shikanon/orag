package optimizer

import "testing"

func TestGenerateCandidatesWarnsLargeSearchSpaceAndUsesSeedStableRandom(t *testing.T) {
	space := SearchSpace{
		Prompts: []PromptCandidate{
			{Name: "strict", System: "Use context only."},
			{Name: "cite", System: "Cite every claim."},
		},
		Retrieval: RetrievalSpace{
			DenseTopK:               []int{20, 50, 80},
			SparseTopK:              []int{20, 50},
			RRFK:                    []int{30, 60},
			SemanticCacheThresholds: []float64{0.88, 0.92},
		},
		Reranker: RerankerSpace{
			Providers: []string{"volcengine", "aliyun"},
			Models:    []string{"m3-v2-rerank", "qwen3-rerank"},
			TopN:      []int{4, 8},
			ProviderModels: map[string][]string{
				"volcengine": {"m3-v2-rerank"},
				"aliyun":     {"qwen3-rerank"},
			},
		},
		Graph: GraphSpace{
			QueryRewriteEnabled: []bool{true, false},
			HyDEEnabled:         []bool{true, false},
			MultiQueryCount:     []int{1, 3},
		},
	}
	spec := SearchSpec{Strategy: SearchStrategySeededRandom, MaxCandidates: 6, Seed: 42, LargeSpaceWarningThreshold: 10}

	first, err := GenerateCandidates(space, spec)
	if err != nil {
		t.Fatalf("GenerateCandidates() error = %v", err)
	}
	second, err := GenerateCandidates(space, spec)
	if err != nil {
		t.Fatalf("GenerateCandidates() second error = %v", err)
	}
	if first.SearchSpaceSize <= spec.LargeSpaceWarningThreshold {
		t.Fatalf("search_space_size = %d, want > %d", first.SearchSpaceSize, spec.LargeSpaceWarningThreshold)
	}
	if !first.Warning.LargeSearchSpace {
		t.Fatalf("warning = %#v, want large search space warning", first.Warning)
	}
	if len(first.Candidates) != spec.MaxCandidates {
		t.Fatalf("candidates = %d, want %d", len(first.Candidates), spec.MaxCandidates)
	}
	if len(second.Candidates) != len(first.Candidates) {
		t.Fatalf("second candidates = %d, want %d", len(second.Candidates), len(first.Candidates))
	}
	for i := range first.Candidates {
		if first.Candidates[i].ID != second.Candidates[i].ID {
			t.Fatalf("seeded order differs at %d: %s != %s", i, first.Candidates[i].ID, second.Candidates[i].ID)
		}
		if first.Candidates[i].Reranker.Provider == "aliyun" && first.Candidates[i].Reranker.Model != "qwen3-rerank" {
			t.Fatalf("unpruned aliyun reranker = %#v", first.Candidates[i].Reranker)
		}
		if first.Candidates[i].Reranker.Provider == "volcengine" && first.Candidates[i].Reranker.Model != "m3-v2-rerank" {
			t.Fatalf("unpruned volcengine reranker = %#v", first.Candidates[i].Reranker)
		}
	}
}

func TestGenerateCandidatesUsesGridForSmallSpace(t *testing.T) {
	space := SearchSpace{
		Prompts: []PromptCandidate{{Name: "strict"}, {Name: "cite"}},
		Retrieval: RetrievalSpace{
			DenseTopK: []int{10, 20},
		},
	}

	result, err := GenerateCandidates(space, SearchSpec{Strategy: SearchStrategyGrid, MaxCandidates: 10})
	if err != nil {
		t.Fatalf("GenerateCandidates() error = %v", err)
	}
	if len(result.Candidates) != 4 {
		t.Fatalf("candidates = %d, want full grid of 4", len(result.Candidates))
	}
	if result.Strategy != SearchStrategyGrid {
		t.Fatalf("strategy = %q, want grid", result.Strategy)
	}
}

func TestGenerateCandidatesPreservesOmittedGraphBooleanDimensions(t *testing.T) {
	result, err := GenerateCandidates(SearchSpace{
		Retrieval: RetrievalSpace{DenseTopK: []int{10}},
	}, SearchSpec{Strategy: SearchStrategyGrid, MaxCandidates: 10})
	if err != nil {
		t.Fatalf("GenerateCandidates() error = %v", err)
	}
	if len(result.Candidates) != 1 {
		t.Fatalf("candidates = %d, want one candidate", len(result.Candidates))
	}
	if result.Candidates[0].Graph.QueryRewriteEnabled != nil || result.Candidates[0].Graph.HyDEEnabled != nil {
		t.Fatalf("graph candidate = %#v, want omitted boolean dimensions to remain nil", result.Candidates[0].Graph)
	}

	result, err = GenerateCandidates(SearchSpace{
		Graph: GraphSpace{
			QueryRewriteEnabled: []bool{false},
			HyDEEnabled:         []bool{false},
		},
	}, SearchSpec{Strategy: SearchStrategyGrid, MaxCandidates: 10})
	if err != nil {
		t.Fatalf("GenerateCandidates() explicit false error = %v", err)
	}
	if len(result.Candidates) != 1 {
		t.Fatalf("explicit false candidates = %d, want one candidate", len(result.Candidates))
	}
	graph := result.Candidates[0].Graph
	if graph.QueryRewriteEnabled == nil || *graph.QueryRewriteEnabled || graph.HyDEEnabled == nil || *graph.HyDEEnabled {
		t.Fatalf("graph candidate = %#v, want explicit false boolean dimensions", graph)
	}
}

func TestGenerateCandidatesPrunesDisabledIndexAffectingDimensions(t *testing.T) {
	space := SearchSpace{
		Chunking: ChunkingSpace{
			Enabled:       false,
			SizeTokens:    []int{500, 800},
			OverlapTokens: []int{80, 120},
			ParserMethods: []string{"basic", "docling"},
		},
		Embedding: EmbeddingSpace{
			Enabled:    false,
			Models:     []string{"embed-a", "embed-b"},
			Dimensions: []int{512, 1024},
		},
		Reranker: RerankerSpace{
			Providers: []string{"aliyun"},
			Models:    []string{"m3-v2-rerank", "qwen3-rerank"},
			TopN:      []int{4},
			ProviderModels: map[string][]string{
				"aliyun": {"qwen3-rerank"},
			},
		},
	}

	result, err := GenerateCandidates(space, SearchSpec{Strategy: SearchStrategyGrid, MaxCandidates: 10})
	if err != nil {
		t.Fatalf("GenerateCandidates() error = %v", err)
	}
	if len(result.Candidates) != 1 {
		t.Fatalf("candidates = %d, want one pruned candidate", len(result.Candidates))
	}
	candidate := result.Candidates[0]
	if candidate.Chunking.Enabled || candidate.Chunking.SizeTokens != 0 || candidate.Chunking.ParserMethod != "" {
		t.Fatalf("chunking candidate = %#v, want disabled zero-value knobs", candidate.Chunking)
	}
	if candidate.Embedding.Enabled || candidate.Embedding.Model != "" || candidate.Embedding.Dimensions != 0 {
		t.Fatalf("embedding candidate = %#v, want disabled zero-value knobs", candidate.Embedding)
	}
	if candidate.Reranker.Model != "qwen3-rerank" {
		t.Fatalf("reranker = %#v, want aliyun/qwen3-rerank only", candidate.Reranker)
	}
}

func TestCandidateIDIsDeterministicFromCanonicalConfig(t *testing.T) {
	config := CandidateConfig{
		Prompt:    PromptCandidate{Name: "strict", System: "Use context only."},
		Retrieval: RetrievalCandidate{DenseTopK: 20, SparseTopK: 20, RRFK: 60},
		Reranker:  RerankerCandidate{Provider: "volcengine", Model: "m3-v2-rerank", TopN: 8},
	}
	first := config.WithDeterministicID("opt_123")
	second := config.WithDeterministicID("opt_123")
	otherRun := config.WithDeterministicID("opt_456")

	if first.ID == "" || first.Hash == "" {
		t.Fatalf("candidate id/hash not populated: %#v", first)
	}
	if first.ID != second.ID || first.Hash != second.Hash {
		t.Fatalf("candidate id/hash not idempotent: %#v != %#v", first, second)
	}
	if first.ID == otherRun.ID {
		t.Fatalf("candidate ids should include run scope: %q", first.ID)
	}
}

func TestSuccessiveHalvingPlanPromotesDeterministicBudget(t *testing.T) {
	space := SearchSpace{
		Prompts: []PromptCandidate{{Name: "p1"}, {Name: "p2"}, {Name: "p3"}, {Name: "p4"}},
	}

	result, err := GenerateCandidates(space, SearchSpec{
		Strategy:      SearchStrategySuccessiveHalving,
		MaxCandidates: 4,
		Seed:          7,
		Halving:       HalvingSpec{InitialDatasetFraction: 0.25, ReductionFactor: 2, MinSurvivors: 1},
	})
	if err != nil {
		t.Fatalf("GenerateCandidates() error = %v", err)
	}
	if len(result.Stages) != 3 {
		t.Fatalf("stages = %#v, want 3-stage halving plan", result.Stages)
	}
	if result.Stages[0].CandidateCount != 4 || result.Stages[0].DatasetFraction != 0.25 {
		t.Fatalf("stage 0 = %#v, want 4 candidates at 0.25 fraction", result.Stages[0])
	}
	if result.Stages[1].CandidateCount != 2 || result.Stages[2].CandidateCount != 1 {
		t.Fatalf("stage counts = %#v, want 4 -> 2 -> 1", result.Stages)
	}
}
