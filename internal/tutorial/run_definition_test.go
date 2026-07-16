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
