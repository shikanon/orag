package http

import (
	"testing"

	"github.com/shikanon/orag/internal/project"
	"github.com/shikanon/orag/internal/release"
)

func TestReleaseRoutesEnforcePromotionEvidenceAndConcurrency(t *testing.T) {
	h, application, closeApp := newTestHertzWithApp(t)
	defer closeApp()
	repo := release.NewMemoryRepository(project.LegacyDefaultID("tenant_a"))
	repo.PutVersion(release.Version{ID: "pv_1", ProjectID: project.LegacyDefaultID("tenant_a"), ContentHash: "hash"})
	repo.PutEvidence(release.Evidence{VersionID: "pv_1", EnvironmentID: string(release.Staging), Passed: true, ContentHash: "hash"})
	dev, _ := repo.Environment(t.Context(), project.LegacyDefaultID("tenant_a"), release.Development)
	dev.ActiveVersionID = "pv_1"
	repo.SetEnvironment(dev)
	application.Release = release.NewService(repo)
	token := issueToken(t, application, "tenant_a")
	projectID := project.LegacyDefaultID("tenant_a")
	promoted := performJSON(h, "POST", "/v1/projects/"+projectID+"/releases:promote", `{"source_environment":"development","target_environment":"staging","target_version_id":"pv_1"}`, token)
	if promoted.Code != 201 {
		t.Fatalf("promotion status=%d body=%s", promoted.Code, promoted.Body)
	}
	stale := performJSON(h, "POST", "/v1/projects/"+projectID+"/releases:promote", `{"source_environment":"development","target_environment":"staging","target_version_id":"pv_1","expected_active_version_id":"stale"}`, token)
	if stale.Code != 409 {
		t.Fatalf("stale promotion status=%d body=%s", stale.Code, stale.Body)
	}
	history := performJSON(h, "GET", "/v1/projects/"+projectID+"/releases", "", token)
	if history.Code != 200 || len(history.Body) < 20 {
		t.Fatalf("history status=%d body=%s", history.Code, history.Body)
	}
}
