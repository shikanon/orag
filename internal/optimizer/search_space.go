package optimizer

import (
	"math"
	"math/rand"
)

type SearchStrategy string

const (
	SearchStrategyGrid              SearchStrategy = "grid"
	SearchStrategySeededRandom      SearchStrategy = "seeded_random"
	SearchStrategySuccessiveHalving SearchStrategy = "successive_halving"
)

type SearchSpace struct {
	Prompts   []PromptCandidate  `json:"prompts,omitempty"`
	Chunking  ChunkingSpace      `json:"chunking,omitempty"`
	Embedding EmbeddingSpace     `json:"embedding,omitempty"`
	Reranker  RerankerSpace      `json:"reranker,omitempty"`
	Retrieval RetrievalSpace     `json:"retrieval,omitempty"`
	Graph     GraphSpace         `json:"graph,omitempty"`
	Harness   []HarnessCandidate `json:"harness,omitempty"`
}

type ChunkingSpace struct {
	Enabled       bool     `json:"enabled"`
	SizeTokens    []int    `json:"size_tokens,omitempty"`
	OverlapTokens []int    `json:"overlap_tokens,omitempty"`
	ParserMethods []string `json:"parser_methods,omitempty"`
}

type EmbeddingSpace struct {
	Enabled    bool     `json:"enabled"`
	Models     []string `json:"models,omitempty"`
	Dimensions []int    `json:"dimensions,omitempty"`
}

type RerankerSpace struct {
	Providers      []string            `json:"providers,omitempty"`
	Models         []string            `json:"models,omitempty"`
	TopN           []int               `json:"top_n,omitempty"`
	ProviderModels map[string][]string `json:"provider_models,omitempty"`
}

type RetrievalSpace struct {
	DenseTopK               []int     `json:"dense_top_k,omitempty"`
	SparseTopK              []int     `json:"sparse_top_k,omitempty"`
	RRFK                    []int     `json:"rrf_k,omitempty"`
	SemanticCacheThresholds []float64 `json:"semantic_cache_thresholds,omitempty"`
}

type GraphSpace struct {
	QueryRewriteEnabled []bool     `json:"query_rewrite_enabled,omitempty"`
	HyDEEnabled         []bool     `json:"hyde_enabled,omitempty"`
	MultiQueryCount     []int      `json:"multi_query_count,omitempty"`
	Modules             [][]string `json:"modules,omitempty"`
}

type SearchSpec struct {
	Strategy                   SearchStrategy `json:"strategy,omitempty"`
	MaxCandidates              int            `json:"max_candidates,omitempty"`
	Seed                       int64          `json:"seed,omitempty"`
	RunID                      string         `json:"run_id,omitempty"`
	LargeSpaceWarningThreshold int64          `json:"large_space_warning_threshold,omitempty"`
	Halving                    HalvingSpec    `json:"halving,omitempty"`
}

type HalvingSpec struct {
	InitialDatasetFraction float64 `json:"initial_dataset_fraction,omitempty"`
	ReductionFactor        int     `json:"reduction_factor,omitempty"`
	MinSurvivors           int     `json:"min_survivors,omitempty"`
}

type SearchResult struct {
	Strategy        SearchStrategy     `json:"strategy"`
	SearchSpaceSize int64              `json:"search_space_size"`
	Warning         SearchWarning      `json:"warning,omitempty"`
	Candidates      []CandidateConfig  `json:"candidates"`
	Stages          []HalvingStagePlan `json:"stages,omitempty"`
}

type SearchWarning struct {
	LargeSearchSpace bool   `json:"large_search_space,omitempty"`
	Message          string `json:"message,omitempty"`
}

type HalvingStagePlan struct {
	Stage           int     `json:"stage"`
	CandidateCount  int     `json:"candidate_count"`
	DatasetFraction float64 `json:"dataset_fraction"`
}

func GenerateCandidates(space SearchSpace, spec SearchSpec) (SearchResult, error) {
	if spec.MaxCandidates <= 0 {
		spec.MaxCandidates = 50
	}
	if spec.RunID == "" {
		spec.RunID = "candidate_search"
	}

	allCandidates := expandSearchSpace(space)
	for i := range allCandidates {
		allCandidates[i] = allCandidates[i].WithDeterministicID(spec.RunID)
	}
	size := int64(len(allCandidates))
	strategy := chooseStrategy(spec.Strategy, size, spec.MaxCandidates)
	warning := searchWarning(size, spec)

	selected := selectCandidates(allCandidates, strategy, spec)
	result := SearchResult{
		Strategy:        strategy,
		SearchSpaceSize: size,
		Warning:         warning,
		Candidates:      selected,
	}
	if strategy == SearchStrategySuccessiveHalving {
		result.Stages = buildHalvingPlan(len(selected), spec.Halving)
	}
	return result, nil
}

func chooseStrategy(requested SearchStrategy, size int64, maxCandidates int) SearchStrategy {
	if requested != "" {
		return requested
	}
	if size <= int64(maxCandidates) {
		return SearchStrategyGrid
	}
	return SearchStrategySeededRandom
}

func searchWarning(size int64, spec SearchSpec) SearchWarning {
	threshold := spec.LargeSpaceWarningThreshold
	if threshold <= 0 {
		threshold = int64(spec.MaxCandidates)
	}
	if size <= threshold {
		return SearchWarning{}
	}
	return SearchWarning{
		LargeSearchSpace: true,
		Message:          "search_space_size exceeds candidate budget; use seeded random or staged sampling for reproducible partial coverage",
	}
}

func selectCandidates(candidates []CandidateConfig, strategy SearchStrategy, spec SearchSpec) []CandidateConfig {
	if len(candidates) <= spec.MaxCandidates {
		out := make([]CandidateConfig, len(candidates))
		copy(out, candidates)
		return out
	}
	if strategy == SearchStrategyGrid {
		out := make([]CandidateConfig, spec.MaxCandidates)
		copy(out, candidates[:spec.MaxCandidates])
		return out
	}

	order := rand.New(rand.NewSource(spec.Seed)).Perm(len(candidates))
	out := make([]CandidateConfig, 0, spec.MaxCandidates)
	for _, idx := range order[:spec.MaxCandidates] {
		out = append(out, candidates[idx])
	}
	return out
}

func buildHalvingPlan(candidateCount int, spec HalvingSpec) []HalvingStagePlan {
	if candidateCount == 0 {
		return nil
	}
	if spec.ReductionFactor < 2 {
		spec.ReductionFactor = 2
	}
	if spec.MinSurvivors <= 0 {
		spec.MinSurvivors = 1
	}
	if spec.InitialDatasetFraction <= 0 || spec.InitialDatasetFraction > 1 {
		spec.InitialDatasetFraction = 1
	}

	var stages []HalvingStagePlan
	count := candidateCount
	fraction := spec.InitialDatasetFraction
	for stage := 0; ; stage++ {
		stages = append(stages, HalvingStagePlan{
			Stage:           stage,
			CandidateCount:  count,
			DatasetFraction: roundSearchFloat(fraction),
		})
		if count <= spec.MinSurvivors && fraction >= 1 {
			break
		}
		nextCount := int(math.Ceil(float64(count) / float64(spec.ReductionFactor)))
		if nextCount < spec.MinSurvivors {
			nextCount = spec.MinSurvivors
		}
		if nextCount == count && fraction >= 1 {
			break
		}
		count = nextCount
		fraction *= float64(spec.ReductionFactor)
		if fraction > 1 {
			fraction = 1
		}
	}
	return stages
}

func expandSearchSpace(space SearchSpace) []CandidateConfig {
	var candidates []CandidateConfig
	for _, prompt := range promptChoices(space.Prompts) {
		for _, chunking := range chunkingChoices(space.Chunking) {
			for _, embedding := range embeddingChoices(space.Embedding) {
				for _, reranker := range rerankerChoices(space.Reranker) {
					for _, retrieval := range retrievalChoices(space.Retrieval) {
						for _, graph := range graphChoices(space.Graph) {
							for _, harness := range harnessChoices(space.Harness) {
								candidates = append(candidates, CandidateConfig{
									Prompt:    prompt,
									Chunking:  chunking,
									Embedding: embedding,
									Reranker:  reranker,
									Retrieval: retrieval,
									Graph:     graph,
									Harness:   harness,
								})
							}
						}
					}
				}
			}
		}
	}
	return candidates
}

func promptChoices(values []PromptCandidate) []PromptCandidate {
	if len(values) == 0 {
		return []PromptCandidate{{}}
	}
	return values
}

func chunkingChoices(space ChunkingSpace) []ChunkingCandidate {
	if !space.Enabled {
		return []ChunkingCandidate{{Enabled: false}}
	}
	var out []ChunkingCandidate
	for _, size := range intChoices(space.SizeTokens) {
		for _, overlap := range intChoices(space.OverlapTokens) {
			for _, parser := range stringChoices(space.ParserMethods) {
				out = append(out, ChunkingCandidate{Enabled: true, SizeTokens: size, OverlapTokens: overlap, ParserMethod: parser})
			}
		}
	}
	return out
}

func embeddingChoices(space EmbeddingSpace) []EmbeddingCandidate {
	if !space.Enabled {
		return []EmbeddingCandidate{{Enabled: false}}
	}
	var out []EmbeddingCandidate
	for _, model := range stringChoices(space.Models) {
		for _, dimensions := range intChoices(space.Dimensions) {
			out = append(out, EmbeddingCandidate{Enabled: true, Model: model, Dimensions: dimensions})
		}
	}
	return out
}

func rerankerChoices(space RerankerSpace) []RerankerCandidate {
	providers := stringChoices(space.Providers)
	models := stringChoices(space.Models)
	topNs := intChoices(space.TopN)
	out := make([]RerankerCandidate, 0, len(providers)*len(models)*len(topNs))
	for _, provider := range providers {
		for _, model := range models {
			if !rerankerModelAllowed(provider, model, space.ProviderModels) {
				continue
			}
			for _, topN := range topNs {
				out = append(out, RerankerCandidate{Provider: provider, Model: model, TopN: topN})
			}
		}
	}
	if len(out) == 0 {
		return []RerankerCandidate{{}}
	}
	return out
}

func rerankerModelAllowed(provider, model string, allowed map[string][]string) bool {
	if provider == "" || model == "" {
		return true
	}
	if len(allowed) == 0 {
		return true
	}
	models, ok := allowed[provider]
	if !ok {
		return false
	}
	for _, candidate := range models {
		if candidate == model {
			return true
		}
	}
	return false
}

func retrievalChoices(space RetrievalSpace) []RetrievalCandidate {
	var out []RetrievalCandidate
	for _, denseTopK := range intChoices(space.DenseTopK) {
		for _, sparseTopK := range intChoices(space.SparseTopK) {
			for _, rrfK := range intChoices(space.RRFK) {
				for _, threshold := range floatChoices(space.SemanticCacheThresholds) {
					out = append(out, RetrievalCandidate{
						DenseTopK:              denseTopK,
						SparseTopK:             sparseTopK,
						RRFK:                   rrfK,
						SemanticCacheThreshold: threshold,
					})
				}
			}
		}
	}
	return out
}

func graphChoices(space GraphSpace) []GraphCandidate {
	var out []GraphCandidate
	for _, rewrite := range boolPtrChoices(space.QueryRewriteEnabled) {
		for _, hyde := range boolPtrChoices(space.HyDEEnabled) {
			for _, multiQuery := range intChoices(space.MultiQueryCount) {
				for _, modules := range moduleChoices(space.Modules) {
					out = append(out, GraphCandidate{
						QueryRewriteEnabled: rewrite,
						HyDEEnabled:         hyde,
						MultiQueryCount:     multiQuery,
						Modules:             modules,
					})
				}
			}
		}
	}
	return out
}

func harnessChoices(values []HarnessCandidate) []HarnessCandidate {
	if len(values) == 0 {
		return []HarnessCandidate{{}}
	}
	return values
}

func intChoices(values []int) []int {
	if len(values) == 0 {
		return []int{0}
	}
	return values
}

func stringChoices(values []string) []string {
	if len(values) == 0 {
		return []string{""}
	}
	return values
}

func boolPtrChoices(values []bool) []*bool {
	if len(values) == 0 {
		return []*bool{nil}
	}
	out := make([]*bool, 0, len(values))
	for _, value := range values {
		candidate := value
		out = append(out, &candidate)
	}
	return out
}

func floatChoices(values []float64) []float64 {
	if len(values) == 0 {
		return []float64{0}
	}
	return values
}

func moduleChoices(values [][]string) [][]string {
	if len(values) == 0 {
		return [][]string{{}}
	}
	return values
}

func roundSearchFloat(value float64) float64 {
	return math.Round(value*1e12) / 1e12
}
