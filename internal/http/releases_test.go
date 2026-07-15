package http

import (
	"strings"
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

func TestPipelineVersionRoutesRequireMatchingEvidence(t *testing.T) {
	h, application, closeApp := newTestHertzWithApp(t)
	defer closeApp()
	projectID := project.LegacyDefaultID("tenant_a")
	repo := release.NewMemoryRepository(projectID)
	application.Release = release.NewService(repo)
	token := issueToken(t, application, "tenant_a")
	created := performJSON(h, "POST", "/v1/projects/"+projectID+"/versions", `{"id":"pv_api","content_hash":"sha256:abc"}`, token)
	if created.Code != 201 || !strings.Contains(created.Body, `"id":"pv_api"`) {
		t.Fatalf("create version status=%d body=%s", created.Code, created.Body)
	}
	mismatch := performJSON(h, "POST", "/v1/projects/"+projectID+"/versions/pv_api/validations", `{"environment":"staging","passed":true,"content_hash":"sha256:wrong"}`, token)
	if mismatch.Code != 422 {
		t.Fatalf("mismatch status=%d body=%s", mismatch.Code, mismatch.Body)
	}
	valid := performJSON(h, "POST", "/v1/projects/"+projectID+"/versions/pv_api/validations", `{"environment":"staging","passed":true,"content_hash":"sha256:abc"}`, token)
	if valid.Code != 201 {
		t.Fatalf("validation status=%d body=%s", valid.Code, valid.Body)
	}
	versions := performJSON(h, "GET", "/v1/projects/"+projectID+"/versions", "", token)
	if versions.Code != 200 || !strings.Contains(versions.Body, `"pv_api"`) {
		t.Fatalf("versions status=%d body=%s", versions.Code, versions.Body)
	}
}
