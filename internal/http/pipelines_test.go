package http

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/shikanon/orag/internal/pipeline"
	"github.com/shikanon/orag/internal/project"
)

func TestPipelineDraftRoutesEnforceValidationAndRevision(t *testing.T) {
	h, application, closeApp := newTestHertzWithApp(t)
	defer closeApp()
	token := issueToken(t, application, "tenant_a")
	projectResponse := performJSON(h, "POST", "/v1/projects", `{"name":"Debug project"}`, token)
	if projectResponse.Code != 201 {
		t.Fatalf("project status=%d body=%s", projectResponse.Code, projectResponse.Body)
	}
	var projectID string
	if err := json.Unmarshal([]byte(projectResponse.Body), &struct {
		ID *string `json:"id"`
	}{ID: &projectID}); err != nil || projectID == "" {
		t.Fatalf("project body=%s", projectResponse.Body)
	}
	created := performJSON(h, "POST", "/v1/projects/"+projectID+"/pipelines", `{"name":"Support"}`, token)
	if created.Code != 201 || !strings.Contains(created.Body, `"project_id":"`+projectID+`"`) {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body)
	}
	var pipelineID string
	if err := json.Unmarshal([]byte(created.Body), &struct {
		ID *string `json:"id"`
	}{ID: &pipelineID}); err != nil || pipelineID == "" {
		t.Fatalf("pipeline body=%s", created.Body)
	}
	draft := performJSON(h, "GET", "/v1/projects/"+projectID+"/pipelines/"+pipelineID+"/draft", "", token)
	if draft.Code != 200 || !strings.Contains(draft.Body, `"revision":0`) {
		t.Fatalf("draft status=%d body=%s", draft.Code, draft.Body)
	}
	valid := `{"expected_revision":0,"definition":{"nodes":[{"id":"input","type":"init"},{"id":"query_route","type":"query_route"},{"id":"semantic_cache_lookup","type":"semantic_cache_lookup"},{"id":"query_rewrite","type":"query_rewrite"},{"id":"multi_query","type":"multi_query"},{"id":"hybrid_retrieve","type":"hybrid_retrieve"},{"id":"ark_rerank","type":"ark_rerank"},{"id":"context_pack","type":"context_pack"},{"id":"prompt_prefix_cache","type":"prompt_prefix_cache"},{"id":"ark_generate","type":"ark_generate"},{"id":"semantic_cache_write","type":"semantic_cache_write"}],"edges":[]}}`
	// The graph validator should reject disconnected drafts before persistence.
	rejected := performJSON(h, "PUT", "/v1/projects/"+projectID+"/pipelines/"+pipelineID+"/draft", valid, token)
	if rejected.Code != 422 {
		t.Fatalf("invalid draft status=%d body=%s", rejected.Code, rejected.Body)
	}
}

func TestPipelineDebugRejectsNonDevelopmentDrafts(t *testing.T) {
	h, application, closeApp := newTestHertzWithApp(t)
	defer closeApp()
	token := issueToken(t, application, "tenant_a")
	projectID := project.LegacyDefaultID("tenant_a")
	created := performJSON(h, "POST", "/v1/projects/"+projectID+"/pipelines", `{"name":"Support"}`, token)
	if created.Code != 201 {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body)
	}
	var pipelineID string
	if err := json.Unmarshal([]byte(created.Body), &struct {
		ID *string `json:"id"`
	}{ID: &pipelineID}); err != nil || pipelineID == "" {
		t.Fatalf("pipeline body=%s", created.Body)
	}
	response := performJSON(h, "POST", "/v1/projects/"+projectID+"/query:debug", `{"pipeline_id":"`+pipelineID+`","expected_revision":0,"environment":"production","query":{"knowledge_base_id":"kb_default","query":"hello"}}`, token)
	if response.Code != 409 || !strings.Contains(response.Body, `"pipeline_draft_environment_forbidden"`) {
		t.Fatalf("debug status=%d body=%s", response.Code, response.Body)
	}
}

func TestSaveDebugRunAsEvaluationCase(t *testing.T) {
	h, application, closeApp := newTestHertzWithApp(t)
	defer closeApp()
	token := issueToken(t, application, "tenant_a")
	projectResponse := performJSON(h, "POST", "/v1/projects", `{"name":"Debug project"}`, token)
	if projectResponse.Code != 201 {
		t.Fatalf("project status=%d body=%s", projectResponse.Code, projectResponse.Body)
	}
	var projectID string
	if err := json.Unmarshal([]byte(projectResponse.Body), &struct {
		ID *string `json:"id"`
	}{ID: &projectID}); err != nil || projectID == "" {
		t.Fatalf("project body=%s", projectResponse.Body)
	}
	datasetResponse := performJSON(h, "POST", "/v1/datasets", `{"name":"Debug cases","project_id":"`+projectID+`"}`, token)
	if datasetResponse.Code != 201 {
		t.Fatalf("dataset status=%d body=%s", datasetResponse.Code, datasetResponse.Body)
	}
	var datasetID string
	if err := json.Unmarshal([]byte(datasetResponse.Body), &struct {
		ID *string `json:"id"`
	}{ID: &datasetID}); err != nil || datasetID == "" {
		t.Fatalf("dataset body=%s", datasetResponse.Body)
	}
	response := performJSON(h, "POST", "/v1/projects/"+projectID+"/debug-runs/trace_debug/save-case", `{"dataset_id":"`+datasetID+`","query":"how?","ground_truth":"this","expected_evidence":["chunk_1"]}`, token)
	if response.Code != 201 || !strings.Contains(response.Body, `"run_id":"trace_debug"`) || !strings.Contains(response.Body, datasetID) {
		t.Fatalf("save case status=%d body=%s", response.Code, response.Body)
	}
}

func TestCreatePipelineVersionFromDraftFreezesRevision(t *testing.T) {
	h, application, closeApp := newTestHertzWithApp(t)
	defer closeApp()
	token := issueToken(t, application, "tenant_a")
	projectID := project.LegacyDefaultID("tenant_a")
	created := performJSON(h, "POST", "/v1/projects/"+projectID+"/pipelines", `{"name":"Versioned"}`, token)
	if created.Code != 201 {
		t.Fatalf("pipeline status=%d body=%s", created.Code, created.Body)
	}
	var pipelineID string
	if err := json.Unmarshal([]byte(created.Body), &struct {
		ID *string `json:"id"`
	}{ID: &pipelineID}); err != nil || pipelineID == "" {
		t.Fatalf("pipeline body=%s", created.Body)
	}
	response := performJSON(h, "POST", "/v1/projects/"+projectID+"/pipelines/"+pipelineID+"/versions", `{"expected_revision":0}`, token)
	if response.Code != 422 || !strings.Contains(response.Body, `pipeline_invalid_definition`) {
		t.Fatalf("version status=%d body=%s", response.Code, response.Body)
	}

	definition := versionablePipelineDefinition()
	draftBody, err := json.Marshal(struct {
		ExpectedRevision int64               `json:"expected_revision"`
		Definition       pipeline.Definition `json:"definition"`
	}{ExpectedRevision: 0, Definition: definition})
	if err != nil {
		t.Fatal(err)
	}
	saved := performJSON(h, "PUT", "/v1/projects/"+projectID+"/pipelines/"+pipelineID+"/draft", string(draftBody), token)
	if saved.Code != 200 || !strings.Contains(saved.Body, `"revision":1`) {
		t.Fatalf("save draft status=%d body=%s", saved.Code, saved.Body)
	}

	createdVersion := performJSON(h, "POST", "/v1/projects/"+projectID+"/pipelines/"+pipelineID+"/versions", `{"expected_revision":1}`, token)
	if createdVersion.Code != 201 || !strings.Contains(createdVersion.Body, `"pipeline_id":"`+pipelineID+`"`) {
		t.Fatalf("create version status=%d body=%s", createdVersion.Code, createdVersion.Body)
	}
	var createdPayload struct {
		Version struct {
			ID          string `json:"id"`
			PipelineID  string `json:"pipeline_id"`
			ContentHash string `json:"content_hash"`
		} `json:"version"`
	}
	if err := json.Unmarshal([]byte(createdVersion.Body), &createdPayload); err != nil || createdPayload.Version.ID == "" || createdPayload.Version.PipelineID != pipelineID {
		t.Fatalf("version body=%s err=%v", createdVersion.Body, err)
	}
	payload, err := json.Marshal(definition)
	if err != nil {
		t.Fatal(err)
	}
	wantHash := fmt.Sprintf("%x", sha256.Sum256(payload))
	if createdPayload.Version.ContentHash != wantHash {
		t.Fatalf("content hash = %q, want %q", createdPayload.Version.ContentHash, wantHash)
	}
	manualValidation := performJSON(h, "POST", "/v1/projects/"+projectID+"/versions/"+createdPayload.Version.ID+"/validations", `{"environment":"staging","passed":true,"content_hash":"`+wantHash+`"}`, token)
	if manualValidation.Code != 422 || !strings.Contains(manualValidation.Body, `server_derived_evidence_required`) {
		t.Fatalf("manual validation status=%d body=%s", manualValidation.Code, manualValidation.Body)
	}
	version, err := application.Release.Versions(t.Context(), projectID)
	if err != nil || len(version) != 1 || string(version[0].Definition) != string(payload) || version[0].PipelineID != pipelineID {
		t.Fatalf("stored versions = %#v, err = %v", version, err)
	}
}

func versionablePipelineDefinition() pipeline.Definition {
	types := []string{"init", "query_route", "semantic_cache_lookup", "query_rewrite", "multi_query", "hybrid_retrieve", "ark_rerank", "context_pack", "prompt_prefix_cache", "ark_generate", "semantic_cache_write"}
	nodes := make([]pipeline.Node, len(types))
	edges := make([]pipeline.Edge, 0, len(types)-1)
	for i, nodeType := range types {
		nodes[i] = pipeline.Node{ID: nodeType, Type: nodeType}
		if i > 0 {
			edges = append(edges, pipeline.Edge{ID: fmt.Sprintf("edge_%d", i), SourceNodeID: types[i-1], SourcePort: "out", TargetNodeID: nodeType, TargetPort: "in"})
		}
	}
	return pipeline.Definition{Nodes: nodes, Edges: edges}
}
