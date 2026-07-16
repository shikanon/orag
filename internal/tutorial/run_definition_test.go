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
