package http

import (
	"strings"
	"testing"

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
