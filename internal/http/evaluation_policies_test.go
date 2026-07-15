package http

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/shikanon/orag/internal/dataset"
	"github.com/shikanon/orag/internal/project"
)

func TestProjectEvaluationPolicyRoutesAreProjectScoped(t *testing.T) {
	h, application, closeApp := newTestHertzWithApp(t)
	defer closeApp()
	token := issueToken(t, application, "tenant_a")
	projectID := project.LegacyDefaultID("tenant_a")
	dataset, err := application.Datasets.CreateInProject(t.Context(), "tenant_a", projectID, "Release holdout", "evaluation")
	if err != nil {
		t.Fatal(err)
	}
	body := `{"name":"Release quality","dataset_id":"` + dataset.ID + `","gates":[{"metric":"answer_accuracy","comparator":"gte","threshold":0.8},{"metric":"latency_p95_ms","comparator":"lte","threshold":400}]}`
	created := performJSON(h, "POST", "/v1/projects/"+projectID+"/evaluation-policies", body, token)
	if created.Code != 201 || !strings.Contains(created.Body, `"version":1`) || !strings.Contains(created.Body, `"answer_accuracy"`) {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body)
	}
	listed := performJSON(h, "GET", "/v1/projects/"+projectID+"/evaluation-policies", "", token)
	if listed.Code != 200 || !strings.Contains(listed.Body, "Release quality") {
		t.Fatalf("list status=%d body=%s", listed.Code, listed.Body)
	}
	foreignDataset, err := application.Datasets.CreateInProject(t.Context(), "tenant_a", "prj_foreign", "Foreign", "evaluation")
	if err != nil {
		t.Fatal(err)
	}
	foreign := performJSON(h, "POST", "/v1/projects/"+projectID+"/evaluation-policies", `{"name":"Foreign","dataset_id":"`+foreignDataset.ID+`","gates":[{"metric":"answer_accuracy","comparator":"gte","threshold":0.8}]}`, token)
	if foreign.Code != 404 || !strings.Contains(foreign.Body, "evaluation_policy_dataset_not_found") {
		t.Fatalf("foreign dataset status=%d body=%s", foreign.Code, foreign.Body)
	}
}

func TestProjectEvaluationEvidenceIsDerivedFromStoredRun(t *testing.T) {
	h, application, closeApp := newTestHertzWithApp(t)
	defer closeApp()
	application.RAG.Pipeline = &recordingPipeline{answer: "ground truth"}
	token := issueToken(t, application, "tenant_a")
	projectResponse := performJSON(h, "POST", "/v1/projects", `{"name":"Evidence project"}`, token)
	if projectResponse.Code != 201 {
		t.Fatalf("project status=%d body=%s", projectResponse.Code, projectResponse.Body)
	}
	var projectRecord struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(projectResponse.Body), &projectRecord); err != nil || projectRecord.ID == "" {
		t.Fatalf("project body=%s err=%v", projectResponse.Body, err)
	}
	projectID := projectRecord.ID
	datasetRecord, err := application.Datasets.CreateInProject(t.Context(), "tenant_a", projectID, "Release gate", "evaluation")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.Datasets.AddItem(t.Context(), "tenant_a", datasetRecord.ID, dataset.Item{Query: "question", GroundTruth: "ground truth"}); err != nil {
		t.Fatal(err)
	}
	policyResponse := performJSON(h, "POST", "/v1/projects/"+projectID+"/evaluation-policies", `{"name":"Release quality","dataset_id":"`+datasetRecord.ID+`","gates":[{"metric":"answer_accuracy","comparator":"gte","threshold":1}]}`, token)
	if policyResponse.Code != 201 {
		t.Fatalf("policy status=%d body=%s", policyResponse.Code, policyResponse.Body)
	}
	var policy struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(policyResponse.Body), &policy); err != nil || policy.ID == "" {
		t.Fatalf("policy body=%s err=%v", policyResponse.Body, err)
	}
	versionResponse := performJSON(h, "POST", "/v1/projects/"+projectID+"/versions", `{"id":"pv_evidence","content_hash":"sha256:version"}`, token)
	if versionResponse.Code != 201 {
		t.Fatalf("version status=%d body=%s", versionResponse.Code, versionResponse.Body)
	}
	knowledgeBaseResponse := performJSON(h, "POST", "/v1/knowledge-bases", `{"name":"Release docs","project_id":"`+projectID+`"}`, token)
	if knowledgeBaseResponse.Code != 201 {
		t.Fatalf("knowledge base status=%d body=%s", knowledgeBaseResponse.Code, knowledgeBaseResponse.Body)
	}
	var knowledgeBase struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(knowledgeBaseResponse.Body), &knowledgeBase); err != nil || knowledgeBase.ID == "" {
		t.Fatalf("knowledge base body=%s err=%v", knowledgeBaseResponse.Body, err)
	}
	runResponse := performJSON(h, "POST", "/v1/evaluations", `{"dataset_id":"`+datasetRecord.ID+`","knowledge_base_id":"`+knowledgeBase.ID+`","profile":"realtime"}`, token)
	if runResponse.Code != 202 {
		t.Fatalf("evaluation status=%d body=%s", runResponse.Code, runResponse.Body)
	}
	var run struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(runResponse.Body), &run); err != nil || run.ID == "" {
		t.Fatalf("evaluation body=%s err=%v", runResponse.Body, err)
	}
	evidence := performJSON(h, "POST", "/v1/projects/"+projectID+"/versions/pv_evidence/evaluation-evidence", `{"policy_id":"`+policy.ID+`","evaluation_run_id":"`+run.ID+`","environment":"staging"}`, token)
	if evidence.Code != 201 || !strings.Contains(evidence.Body, `"passed":true`) || !strings.Contains(evidence.Body, `"pipeline_version_id":"pv_evidence"`) || !strings.Contains(evidence.Body, `"environment":"staging"`) {
		t.Fatalf("evidence status=%d body=%s", evidence.Code, evidence.Body)
	}
	missing := performJSON(h, "POST", "/v1/projects/"+projectID+"/versions/pv_evidence/evaluation-evidence", `{"policy_id":"`+policy.ID+`","evaluation_run_id":"eval_missing","environment":"staging"}`, token)
	if missing.Code != 404 || !strings.Contains(missing.Body, `"code":"evaluation_not_found"`) {
		t.Fatalf("missing evaluation status=%d body=%s", missing.Code, missing.Body)
	}
}
