package tutorial

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// RuntimeEnvironment contains the non-secret runtime dimensions that must be
// unchanged for a baseline/candidate result to be comparable. App wiring owns
// this snapshot; it is never accepted from an API request.
type RuntimeEnvironment struct {
	ChatProvider       string `json:"chat_provider"`
	ChatModel          string `json:"chat_model"`
	EmbeddingProvider  string `json:"embedding_provider"`
	EmbeddingModel     string `json:"embedding_model"`
	RerankProvider     string `json:"rerank_provider"`
	RerankModel        string `json:"rerank_model"`
	MultimodalProvider string `json:"multimodal_provider"`
	MultimodalModel    string `json:"multimodal_model"`
	PromptCacheMode    string `json:"prompt_cache_mode"`
	EvaluatorVersion   string `json:"evaluator_version"`
}

type runtimeDefinition struct {
	knowledgeBaseID            string
	datasetID                  string
	profile                    string
	topK                       int
	parserMethod               string
	chunkSizeTokens            int
	chunkOverlapTokens         int
	contextualRetrievalEnabled bool
	contextualPromptVersion    string
	retrievalStrategy          string
	reuseBaselineIndex         bool
	comparisonFingerprint      string
	definitionFingerprint      string
}

func (s *LiveRunService) runtimeDefinition(experiment Experiment, variant string) (runtimeDefinition, error) {
	if !supportsTextQuickBaseline(experiment.TemplateID, experiment.Tier) || experiment.RuntimeStatus != "ready" || experiment.KnowledgeBaseID == "" || experiment.DatasetID == "" || experiment.CloneJobID == "" || experiment.PackManifest.Runtime == nil {
		return runtimeDefinition{}, ErrRuntimeUnavailable
	}
	definition := runtimeDefinition{
		knowledgeBaseID:    experiment.KnowledgeBaseID,
		datasetID:          experiment.DatasetID,
		profile:            experiment.BaselineProfile,
		topK:               experiment.BaselineTopK,
		parserMethod:       "basic",
		chunkSizeTokens:    TutorialBaselineChunkSizeTokens,
		chunkOverlapTokens: TutorialBaselineChunkOverlapTokens,
		retrievalStrategy:  TutorialRetrievalStrategyHybrid,
	}
	if variant != "baseline" {
		candidate, found := runtimeCandidate(experiment.PackManifest.Runtime.Candidates, variant)
		if !found {
			return runtimeDefinition{}, ErrExperimentRunVariant
		}
		if !candidate.ReuseBaselineIndex && s.candidateIngestors[candidate.ID] == nil {
			return runtimeDefinition{}, ErrRuntimeUnavailable
		}
		if candidate.ReuseBaselineIndex && s.candidateEvaluators[candidate.ID] == nil {
			return runtimeDefinition{}, ErrRuntimeUnavailable
		}
		if !candidate.ReuseBaselineIndex {
			definition.knowledgeBaseID = tutorialCandidateKnowledgeBaseIDFor(experiment.ProjectID, experiment.TemplateID, experiment.TemplateVersion, candidate.ID)
		}
		definition.parserMethod = candidate.ParserMethod
		if candidate.ChunkSizeTokens > 0 {
			definition.chunkSizeTokens = candidate.ChunkSizeTokens
			definition.chunkOverlapTokens = candidate.ChunkOverlapTokens
		}
		definition.contextualRetrievalEnabled = candidate.ContextualRetrieval
		definition.retrievalStrategy = candidateRetrievalStrategy(candidate)
		definition.reuseBaselineIndex = candidate.ReuseBaselineIndex
		if candidate.ContextualRetrieval {
			definition.contextualPromptVersion = TutorialP3ContextualPromptVersion
		}
	}
	comparisonInput := struct {
		TemplateID      string             `json:"template_id"`
		TemplateVersion string             `json:"template_version"`
		Tier            string             `json:"tier"`
		ManifestSHA256  string             `json:"manifest_sha256"`
		DatasetID       string             `json:"dataset_id"`
		Profile         string             `json:"profile"`
		TopK            int                `json:"top_k"`
		Environment     RuntimeEnvironment `json:"environment"`
	}{
		TemplateID: experiment.TemplateID, TemplateVersion: experiment.TemplateVersion, Tier: experiment.Tier,
		ManifestSHA256: manifestSHA256(experiment.PackManifest), DatasetID: definition.datasetID,
		Profile: definition.profile, TopK: definition.topK, Environment: s.runtimeEnvironment,
	}
	definition.comparisonFingerprint = jsonSHA256(comparisonInput)
	definition.definitionFingerprint = jsonSHA256(struct {
		ComparisonFingerprint      string `json:"comparison_fingerprint"`
		Variant                    string `json:"variant"`
		ParserMethod               string `json:"parser_method"`
		ChunkSizeTokens            int    `json:"chunk_size_tokens"`
		ChunkOverlapTokens         int    `json:"chunk_overlap_tokens"`
		ContextualRetrievalEnabled bool   `json:"contextual_retrieval_enabled"`
		ContextualPromptVersion    string `json:"contextual_prompt_version"`
		RetrievalStrategy          string `json:"retrieval_strategy"`
		ReuseBaselineIndex         bool   `json:"reuse_baseline_index"`
		KnowledgeBaseID            string `json:"knowledge_base_id"`
	}{
		ComparisonFingerprint: definition.comparisonFingerprint, Variant: variant,
		ParserMethod: definition.parserMethod, ChunkSizeTokens: definition.chunkSizeTokens,
		ChunkOverlapTokens: definition.chunkOverlapTokens, ContextualRetrievalEnabled: definition.contextualRetrievalEnabled,
		ContextualPromptVersion: definition.contextualPromptVersion, KnowledgeBaseID: definition.knowledgeBaseID,
		RetrievalStrategy: definition.retrievalStrategy, ReuseBaselineIndex: definition.reuseBaselineIndex,
	})
	return definition, nil
}

func runtimeCandidate(candidates []RuntimeCandidate, id string) (RuntimeCandidate, bool) {
	for _, candidate := range candidates {
		if candidate.ID == id {
			return candidate, true
		}
	}
	return RuntimeCandidate{}, false
}

func (d runtimeDefinition) matches(run ExperimentRun) bool {
	return run.KnowledgeBaseID == d.knowledgeBaseID &&
		run.DatasetID == d.datasetID &&
		run.Profile == d.profile &&
		run.TopK == d.topK &&
		run.ParserMethod == d.parserMethod &&
		run.ChunkSizeTokens == d.chunkSizeTokens &&
		run.ChunkOverlapTokens == d.chunkOverlapTokens &&
		run.ContextualRetrievalEnabled == d.contextualRetrievalEnabled &&
		run.RetrievalStrategy == d.retrievalStrategy &&
		run.ReusedBaselineIndex == d.reuseBaselineIndex &&
		run.ComparisonFingerprint == d.comparisonFingerprint &&
		run.DefinitionFingerprint == d.definitionFingerprint
}

func (r ExperimentRun) isLegacyBaseline() bool {
	return r.Variant == "baseline" && r.KnowledgeBaseID == "" && r.DatasetID == "" && r.Profile == "" && r.TopK == 0 && r.ParserMethod == "" && r.ChunkSizeTokens == 0 && r.ChunkOverlapTokens == 0 && !r.ContextualRetrievalEnabled && (r.RetrievalStrategy == "" || r.RetrievalStrategy == TutorialRetrievalStrategyHybrid) && !r.ReusedBaselineIndex && r.ComparisonFingerprint == "" && r.DefinitionFingerprint == ""
}

func manifestSHA256(manifest Manifest) string { return jsonSHA256(manifest) }

func jsonSHA256(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
