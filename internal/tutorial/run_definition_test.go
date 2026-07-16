package tutorial

import "testing"

func TestRuntimeDefinitionBindsP3ContextualContract(t *testing.T) {
	service := NewLiveRunService(nil, nil, nil)
	service.ConfigureCandidateIngestors(RuntimeEnvironment{ChatModel: "chat"}, map[string]RuntimeIngestor{
		TutorialP3ContextualCandidateID: &recordingRuntimeIngestor{},
	})
	experiment := Experiment{
		ProjectID: "prj_1", CloneJobID: "tclj_1", TemplateID: "text-rag", TemplateVersion: "1.0.3", Tier: "quick",
		RuntimeStatus: "ready", KnowledgeBaseID: "tkb_p0", DatasetID: "tds_1", BaselineProfile: "realtime", BaselineTopK: 5,
		PackManifest: Manifest{Runtime: &RuntimeManifest{Candidates: []RuntimeCandidate{{
			ID: TutorialP3ContextualCandidateID, Chapter: TutorialP3ContextualChapter, ParserMethod: "basic",
			ChunkSizeTokens: TutorialBaselineChunkSizeTokens, ChunkOverlapTokens: TutorialBaselineChunkOverlapTokens, ContextualRetrieval: true,
		}}}},
	}
	baseline, err := service.runtimeDefinition(experiment, "baseline")
	if err != nil {
		t.Fatal(err)
	}
	p3, err := service.runtimeDefinition(experiment, TutorialP3ContextualCandidateID)
	if err != nil {
		t.Fatal(err)
	}
	if !p3.contextualRetrievalEnabled || p3.contextualPromptVersion != TutorialP3ContextualPromptVersion || p3.chunkSizeTokens != TutorialBaselineChunkSizeTokens || p3.chunkOverlapTokens != TutorialBaselineChunkOverlapTokens {
		t.Fatalf("P3 definition=%#v", p3)
	}
	if baseline.contextualRetrievalEnabled || baseline.contextualPromptVersion != "" || baseline.definitionFingerprint == p3.definitionFingerprint || baseline.comparisonFingerprint != p3.comparisonFingerprint {
		t.Fatalf("baseline=%#v P3=%#v", baseline, p3)
	}
}

func TestRuntimeDefinitionBindsP4SparseReuseContract(t *testing.T) {
	service := NewLiveRunService(nil, nil, nil)
	service.ConfigureCandidateEvaluators(map[string]RuntimeEvaluator{TutorialP4SparseCandidateID: &recordingRuntimeEvaluator{}})
	experiment := Experiment{
		ProjectID: "prj_1", CloneJobID: "tclj_1", TemplateID: "text-rag", TemplateVersion: "1.0.4", Tier: "quick",
		RuntimeStatus: "ready", KnowledgeBaseID: "tkb_p0", DatasetID: "tds_1", BaselineProfile: "realtime", BaselineTopK: 5,
		PackManifest: Manifest{Runtime: &RuntimeManifest{Candidates: []RuntimeCandidate{{
			ID: TutorialP4SparseCandidateID, Chapter: TutorialP4SparseChapter, ParserMethod: "basic",
			ChunkSizeTokens: TutorialBaselineChunkSizeTokens, ChunkOverlapTokens: TutorialBaselineChunkOverlapTokens,
			RetrievalStrategy: TutorialRetrievalStrategySparse, ReuseBaselineIndex: true,
		}}}},
	}
	baseline, err := service.runtimeDefinition(experiment, "baseline")
	if err != nil {
		t.Fatal(err)
	}
	p4, err := service.runtimeDefinition(experiment, TutorialP4SparseCandidateID)
	if err != nil {
		t.Fatal(err)
	}
	if p4.contextualRetrievalEnabled || p4.retrievalStrategy != TutorialRetrievalStrategySparse || !p4.reuseBaselineIndex || p4.knowledgeBaseID != baseline.knowledgeBaseID || p4.definitionFingerprint == baseline.definitionFingerprint || p4.comparisonFingerprint != baseline.comparisonFingerprint {
		t.Fatalf("baseline=%#v P4=%#v", baseline, p4)
	}
}

func TestRuntimeDefinitionBindsP5MultiQueryReuseContract(t *testing.T) {
	service := NewLiveRunService(nil, nil, nil)
	service.ConfigureCandidateEvaluators(map[string]RuntimeEvaluator{TutorialP5MultiQueryCandidateID: &recordingRuntimeEvaluator{}})
	experiment := Experiment{
		ProjectID: "prj_1", CloneJobID: "tclj_1", TemplateID: "text-rag", TemplateVersion: "1.0.5", Tier: "quick",
		RuntimeStatus: "ready", KnowledgeBaseID: "tkb_p0", DatasetID: "tds_1", BaselineProfile: "realtime", BaselineTopK: 5,
		PackManifest: Manifest{Runtime: &RuntimeManifest{Candidates: []RuntimeCandidate{{
			ID: TutorialP5MultiQueryCandidateID, Chapter: TutorialP5MultiQueryChapter, ParserMethod: "basic",
			ChunkSizeTokens: TutorialBaselineChunkSizeTokens, ChunkOverlapTokens: TutorialBaselineChunkOverlapTokens,
			RetrievalStrategy: TutorialRetrievalStrategyHybrid, ReuseBaselineIndex: true, MultiQueryCount: 3,
		}}}},
	}
	baseline, err := service.runtimeDefinition(experiment, "baseline")
	if err != nil {
		t.Fatal(err)
	}
	p5, err := service.runtimeDefinition(experiment, TutorialP5MultiQueryCandidateID)
	if err != nil {
		t.Fatal(err)
	}
	if p5.queryExpansionMode != TutorialQueryExpansionMultiQuery || p5.multiQueryCount != 3 || p5.retrievalStrategy != TutorialRetrievalStrategyHybrid || !p5.reuseBaselineIndex || p5.knowledgeBaseID != baseline.knowledgeBaseID || p5.definitionFingerprint == baseline.definitionFingerprint || p5.comparisonFingerprint != baseline.comparisonFingerprint {
		t.Fatalf("baseline=%#v P5=%#v", baseline, p5)
	}
}

func TestRuntimeDefinitionBindsP6RerankReuseContract(t *testing.T) {
	service := NewLiveRunService(nil, nil, nil)
	service.ConfigureCandidateEvaluators(map[string]RuntimeEvaluator{TutorialP6RerankCandidateID: &recordingRuntimeEvaluator{}})
	experiment := Experiment{
		ProjectID: "prj_1", CloneJobID: "tclj_1", TemplateID: "text-rag", TemplateVersion: "1.0.6", Tier: "quick",
		RuntimeStatus: "ready", KnowledgeBaseID: "tkb_p0", DatasetID: "tds_1", BaselineProfile: "realtime", BaselineTopK: 5,
		PackManifest: Manifest{Runtime: &RuntimeManifest{Candidates: []RuntimeCandidate{{
			ID: TutorialP6RerankCandidateID, Chapter: TutorialP6RerankChapter, ParserMethod: "basic",
			ChunkSizeTokens: TutorialBaselineChunkSizeTokens, ChunkOverlapTokens: TutorialBaselineChunkOverlapTokens,
			RetrievalStrategy: TutorialRetrievalStrategyHybrid, ReuseBaselineIndex: true, RerankEnabled: true,
		}}}},
	}
	baseline, err := service.runtimeDefinition(experiment, "baseline")
	if err != nil {
		t.Fatal(err)
	}
	p6, err := service.runtimeDefinition(experiment, TutorialP6RerankCandidateID)
	if err != nil {
		t.Fatal(err)
	}
	if !p6.rerankEnabled || p6.multiQueryCount != 0 || p6.queryExpansionMode != TutorialQueryExpansionNone || p6.retrievalStrategy != TutorialRetrievalStrategyHybrid || !p6.reuseBaselineIndex || p6.knowledgeBaseID != baseline.knowledgeBaseID || p6.definitionFingerprint == baseline.definitionFingerprint || p6.comparisonFingerprint != baseline.comparisonFingerprint {
		t.Fatalf("baseline=%#v P6=%#v", baseline, p6)
	}
}

func TestRuntimeDefinitionBindsP7GraphContract(t *testing.T) {
	service := NewLiveRunService(nil, nil, nil)
	service.ConfigureCandidateIngestors(RuntimeEnvironment{}, map[string]RuntimeIngestor{TutorialP7GraphCandidateID: &recordingRuntimeIngestor{}})
	experiment := Experiment{
		ProjectID: "prj_1", CloneJobID: "tclj_1", TemplateID: "text-rag", TemplateVersion: "1.0.7", Tier: "quick",
		RuntimeStatus: "ready", KnowledgeBaseID: "tkb_p0", DatasetID: "tds_1", BaselineProfile: "realtime", BaselineTopK: 5,
		PackManifest: Manifest{Runtime: &RuntimeManifest{Candidates: []RuntimeCandidate{{
			ID: TutorialP7GraphCandidateID, Chapter: TutorialP7GraphChapter, ParserMethod: "basic",
			ChunkSizeTokens: TutorialBaselineChunkSizeTokens, ChunkOverlapTokens: TutorialBaselineChunkOverlapTokens,
			RetrievalStrategy: TutorialRetrievalStrategyGraph, GraphRetrievalEnabled: true,
		}}}},
	}
	baseline, err := service.runtimeDefinition(experiment, "baseline")
	if err != nil {
		t.Fatal(err)
	}
	p7, err := service.runtimeDefinition(experiment, TutorialP7GraphCandidateID)
	if err != nil {
		t.Fatal(err)
	}
	if !p7.graphRetrievalEnabled || p7.rerankEnabled || p7.multiQueryCount != 0 || p7.queryExpansionMode != TutorialQueryExpansionNone || p7.retrievalStrategy != TutorialRetrievalStrategyGraph || p7.reuseBaselineIndex || p7.knowledgeBaseID == baseline.knowledgeBaseID || p7.definitionFingerprint == baseline.definitionFingerprint || p7.comparisonFingerprint != baseline.comparisonFingerprint {
		t.Fatalf("baseline=%#v P7=%#v", baseline, p7)
	}
}

func TestRuntimeDefinitionBindsP8ContextPackReuseContract(t *testing.T) {
	service := NewLiveRunService(nil, nil, nil)
	service.ConfigureCandidateEvaluators(map[string]RuntimeEvaluator{TutorialP8ContextPackCandidateID: &recordingRuntimeEvaluator{}})
	experiment := Experiment{
		ProjectID: "prj_1", CloneJobID: "tclj_1", TemplateID: "text-rag", TemplateVersion: "1.0.8", Tier: "quick",
		RuntimeStatus: "ready", KnowledgeBaseID: "tkb_p0", DatasetID: "tds_1", BaselineProfile: "realtime", BaselineTopK: 5,
		PackManifest: Manifest{Runtime: &RuntimeManifest{Candidates: []RuntimeCandidate{{
			ID: TutorialP8ContextPackCandidateID, Chapter: TutorialP8ContextPackChapter, ParserMethod: "basic",
			ChunkSizeTokens: TutorialBaselineChunkSizeTokens, ChunkOverlapTokens: TutorialBaselineChunkOverlapTokens,
			RetrievalStrategy: TutorialRetrievalStrategyHybrid, ReuseBaselineIndex: true,
			ContextPackTopN: TutorialP8ContextPackTopN, ContextPackMaxTokens: TutorialContextPackMaxTokens,
		}}}},
	}
	baseline, err := service.runtimeDefinition(experiment, "baseline")
	if err != nil {
		t.Fatal(err)
	}
	p8, err := service.runtimeDefinition(experiment, TutorialP8ContextPackCandidateID)
	if err != nil {
		t.Fatal(err)
	}
	if baseline.contextPackTopN != TutorialBaselineContextPackTopN || baseline.contextPackMaxTokens != TutorialContextPackMaxTokens || p8.contextPackTopN != TutorialP8ContextPackTopN || p8.contextPackMaxTokens != TutorialContextPackMaxTokens || !p8.reuseBaselineIndex || p8.knowledgeBaseID != baseline.knowledgeBaseID || p8.definitionFingerprint == baseline.definitionFingerprint || p8.comparisonFingerprint != baseline.comparisonFingerprint {
		t.Fatalf("baseline=%#v P8=%#v", baseline, p8)
	}
}
