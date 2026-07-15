package http

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/shikanon/orag/internal/project"
)

func TestPipelineDraftRoutesEnforceValidationAndRevision(t *testing.T) {
	h, application, closeApp := newTestHertzWithApp(t)
	defer closeApp()
	token := issueToken(t, application, "tenant_a")
	projectID := project.LegacyDefaultID("tenant_a")
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
