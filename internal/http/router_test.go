package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/cloudwego/hertz/pkg/route"
	"github.com/getkin/kin-openapi/openapi3"
	core "github.com/shikanon/orag/internal/app"
	"github.com/shikanon/orag/internal/auth"
	"github.com/shikanon/orag/internal/config"
	"github.com/shikanon/orag/internal/dataset"
	evalpkg "github.com/shikanon/orag/internal/eval"
	"github.com/shikanon/orag/internal/ingest"
	"github.com/shikanon/orag/internal/kb"
	"github.com/shikanon/orag/internal/observability"
	"github.com/shikanon/orag/internal/offlineknowledge"
	"github.com/shikanon/orag/internal/optimizer"
	"github.com/shikanon/orag/internal/platform/logger"
	"github.com/shikanon/orag/internal/project"
	"github.com/shikanon/orag/internal/rag"
	"github.com/shikanon/orag/internal/tutorial"
	"time"
)

func TestTutorialCatalogRoutes(t *testing.T) {
	h, application, closeApp := newTestHertzWithApp(t)
	defer closeApp()
	token := issueToken(t, application, "tenant_a")

	listed := performJSON(h, "GET", "/v1/tutorials", "", token)
	if listed.Code != 200 {
		t.Fatalf("list status = %d body=%s", listed.Code, listed.Body)
	}
	var body struct {
		Tutorials []tutorialResponse `json:"tutorials"`
	}
	if err := json.Unmarshal([]byte(listed.Body), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Tutorials) != 3 {
		t.Fatalf("tutorials = %d, want 3", len(body.Tutorials))
	}
	if got := body.Tutorials[0].Packs[0].ManifestURL; !strings.HasPrefix(got, "https://lensrhyme.tos-cn-hongkong.volces.com/tutorial-packs/") {
		t.Fatalf("manifest URL = %q", got)
	}

	current := performJSON(h, "GET", "/v1/tutorials/text-rag", "", token)
	if current.Code != 200 {
		t.Fatalf("current status = %d body=%s", current.Code, current.Body)
	}
	var currentBody tutorialResponse
	if err := json.Unmarshal([]byte(current.Body), &currentBody); err != nil {
		t.Fatal(err)
	}
	if currentBody.ID != "text-rag" || currentBody.Version != "1.1.0" || currentBody.Modality != tutorial.ModalityText {
		t.Fatalf("current tutorial = %#v", currentBody)
	}

	versioned := performJSON(h, "GET", "/v1/tutorials/text-rag/versions/1.1.0", "", token)
	if versioned.Code != 200 || versioned.Body != current.Body {
		t.Fatalf("versioned status = %d body=%s, current=%s", versioned.Code, versioned.Body, current.Body)
	}

	replay := performJSON(h, "GET", "/v1/tutorials/text-rag/replay", "", token)
	if replay.Code != http.StatusOK || !strings.Contains(replay.Body, `"id":"text-rag/1.0.0/benchmark/replay-v1"`) || !strings.Contains(replay.Body, `"fingerprint":"`) || strings.Contains(strings.ToLower(replay.Body), "access_key") {
		t.Fatalf("replay status=%d body=%s", replay.Code, replay.Body)
	}
	if unavailable := performJSON(h, "GET", "/v1/tutorials/video-rag/replay", "", token); unavailable.Code != http.StatusNotFound || !strings.Contains(unavailable.Body, `"code":"tutorial_replay_not_found"`) {
		t.Fatalf("unavailable replay status=%d body=%s", unavailable.Code, unavailable.Body)
	}
}

func TestTutorialCloneRoutesCreatePollAndExposeNoStorageDetails(t *testing.T) {
	pack := []byte(`{"service":{"port":8080,"name":"ORAG"}}`)
	checksum := "bdb62ea22175c8ad0f316fb554a4e8884c2ea3ae0df9c1cdf8def49b523b79ce"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/packs/text-rag/1.0.0/quick/manifest.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"template_id":"text-rag","version":"1.0.0","tier":"quick","license":{"spdx":"CC-BY-4.0","source_url":"https://example.test/license","redistributable":true},"objects":[{"path":"corpus/service.json","sha256":"` + checksum + `","bytes":39,"content_type":"application/json"}],"runtime":{"baseline":{"profile":"realtime","top_k":3},"documents":[{"object_path":"corpus/service.json","name":"服务配置"}],"dataset":{"name":"教程基线评测","items":[{"query":"服务端口是多少？","ground_truth":"8080","split":"eval"}]},"candidates":[{"id":"p1_structured_json","chapter":"p1_document_parser","parser_method":"structured_json"},{"id":"p2_recursive_400_80","chapter":"p2_chunking","parser_method":"basic","chunk_size_tokens":400,"chunk_overlap_tokens":80}]}}`))
		case "/packs/text-rag/1.0.0/quick/corpus/service.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(pack)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("ORAG_TEST_MODE", "true")
	t.Setenv("TUTORIAL_CATALOG_BASE_URL", server.URL+"/packs")
	t.Setenv("TUTORIAL_PRIVATE_OUTPUT_DIR", t.TempDir())
	h, application, closeApp := newTestHertzWithApp(t)
	defer closeApp()
	token := issueToken(t, application, "tenant_a")

	created := performJSON(h, "POST", "/v1/tutorials/text-rag/clones", `{"version":"1.0.0","pack_tier":"quick","project":{"name":"Text lab","description":"server-owned install"},"idempotency_key":"http_clone_1","license_accepted":true}`, token)
	if created.Code != http.StatusAccepted {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body)
	}
	for _, forbidden := range []string{"bucket", "access_key", "object_key", "manifest_url", "ORAG"} {
		if strings.Contains(created.Body, forbidden) {
			t.Fatalf("create leaked %q: %s", forbidden, created.Body)
		}
	}
	var accepted tutorialCloneAcceptedResponse
	if err := json.Unmarshal([]byte(created.Body), &accepted); err != nil {
		t.Fatal(err)
	}
	if accepted.JobID == "" || accepted.ProjectID == "" || accepted.PollURL != "/v1/tutorial-clone-jobs/"+accepted.JobID {
		t.Fatalf("accepted = %#v", accepted)
	}

	var job tutorial.CloneJob
	for range 50 {
		polled := performJSON(h, "GET", accepted.PollURL, "", token)
		if polled.Code != http.StatusOK {
			t.Fatalf("poll status=%d body=%s", polled.Code, polled.Body)
		}
		if err := json.Unmarshal([]byte(polled.Body), &job); err != nil {
			t.Fatal(err)
		}
		if job.Status == tutorial.CloneStatusCompleted || job.Status == tutorial.CloneStatusFailed {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if job.Status != tutorial.CloneStatusCompleted || job.Stage != tutorial.CloneStagePackInstalled {
		t.Fatalf("clone job = %#v", job)
	}
	experiment := performJSON(h, "GET", "/v1/projects/"+accepted.ProjectID+"/tutorial-experiment", "", token)
	if experiment.Code != http.StatusOK || !strings.Contains(experiment.Body, `"pack_status":"pack_installed"`) || !strings.Contains(experiment.Body, `"runtime_status":"ready"`) {
		t.Fatalf("experiment status=%d body=%s", experiment.Code, experiment.Body)
	}
	var installed tutorial.Experiment
	if err := json.Unmarshal([]byte(experiment.Body), &installed); err != nil {
		t.Fatal(err)
	}
	if len(installed.Variants) != 3 || installed.Variants[1].ID != tutorial.TutorialP1StructuredJSONCandidateID || installed.Variants[2].ID != tutorial.TutorialP2RecursiveChunkCandidateID || installed.Variants[2].ChunkSizeTokens != tutorial.TutorialP2ChunkSizeTokens {
		t.Fatalf("experiment variants=%#v", installed.Variants)
	}
	spoofed := performJSON(h, "POST", "/v1/projects/"+accepted.ProjectID+"/tutorial-experiments/"+installed.ID+"/runs", `{"variant":"baseline","idempotency_key":"spoofed","knowledge_base_id":"browser-controlled"}`, token)
	if spoofed.Code != http.StatusBadRequest || !strings.Contains(spoofed.Body, `"code":"invalid_tutorial_experiment_run_request"`) {
		t.Fatalf("spoofed start status=%d body=%s", spoofed.Code, spoofed.Body)
	}
	beforeBaseline := performJSON(h, "POST", "/v1/projects/"+accepted.ProjectID+"/tutorial-experiments/"+installed.ID+"/runs", `{"variant":"p1_structured_json","idempotency_key":"p1-before-p0"}`, token)
	if beforeBaseline.Code != http.StatusConflict || !strings.Contains(beforeBaseline.Body, `"code":"tutorial_baseline_required"`) {
		t.Fatalf("P1 before P0 status=%d body=%s", beforeBaseline.Code, beforeBaseline.Body)
	}
	beforeP2Baseline := performJSON(h, "POST", "/v1/projects/"+accepted.ProjectID+"/tutorial-experiments/"+installed.ID+"/runs", `{"variant":"p2_recursive_400_80","idempotency_key":"p2-before-p0"}`, token)
	if beforeP2Baseline.Code != http.StatusConflict || !strings.Contains(beforeP2Baseline.Body, `"code":"tutorial_baseline_required"`) {
		t.Fatalf("P2 before P0 status=%d body=%s", beforeP2Baseline.Code, beforeP2Baseline.Body)
	}
	started := performJSON(h, "POST", "/v1/projects/"+accepted.ProjectID+"/tutorial-experiments/"+installed.ID+"/runs", `{"idempotency_key":"http_live_run_1"}`, token)
	if started.Code != http.StatusAccepted {
		t.Fatalf("live run start status=%d body=%s", started.Code, started.Body)
	}
	for _, forbidden := range []string{"bucket", "access_key", "object_key", "manifest_url", "tutorial data"} {
		if strings.Contains(started.Body, forbidden) {
			t.Fatalf("live run start leaked %q: %s", forbidden, started.Body)
		}
	}
	var runAccepted tutorialExperimentRunAcceptedResponse
	if err := json.Unmarshal([]byte(started.Body), &runAccepted); err != nil {
		t.Fatal(err)
	}
	var run tutorial.ExperimentRun
	for range 200 {
		polled := performJSON(h, "GET", runAccepted.PollURL, "", token)
		if polled.Code != http.StatusOK {
			t.Fatalf("live run poll status=%d body=%s", polled.Code, polled.Body)
		}
		if err := json.Unmarshal([]byte(polled.Body), &run); err != nil {
			t.Fatal(err)
		}
		if run.Status == tutorial.ExperimentRunCompleted || run.Status == tutorial.ExperimentRunFailed {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if run.Status != tutorial.ExperimentRunCompleted || run.EvaluationRunID == "" {
		t.Fatalf("live run = %#v", run)
	}
	p1Started := performJSON(h, "POST", "/v1/projects/"+accepted.ProjectID+"/tutorial-experiments/"+installed.ID+"/runs", `{"variant":"p1_structured_json","idempotency_key":"http_live_run_p1"}`, token)
	if p1Started.Code != http.StatusAccepted {
		t.Fatalf("P1 start status=%d body=%s", p1Started.Code, p1Started.Body)
	}
	var p1Accepted tutorialExperimentRunAcceptedResponse
	if err := json.Unmarshal([]byte(p1Started.Body), &p1Accepted); err != nil {
		t.Fatal(err)
	}
	var p1Run tutorial.ExperimentRun
	for range 200 {
		polled := performJSON(h, "GET", p1Accepted.PollURL, "", token)
		if polled.Code != http.StatusOK {
			t.Fatalf("P1 poll status=%d body=%s", polled.Code, polled.Body)
		}
		if err := json.Unmarshal([]byte(polled.Body), &p1Run); err != nil {
			t.Fatal(err)
		}
		if p1Run.Status == tutorial.ExperimentRunCompleted || p1Run.Status == tutorial.ExperimentRunFailed {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if p1Run.Status != tutorial.ExperimentRunCompleted || p1Run.BaselineRunID != run.ID || p1Run.KnowledgeBaseID == run.KnowledgeBaseID || p1Run.ParserMethod != tutorial.TutorialStructuredJSONParserMethod {
		t.Fatalf("P1 run=%#v baseline=%#v", p1Run, run)
	}
	comparison := performJSON(h, "GET", p1Accepted.PollURL+"/comparison", "", token)
	if comparison.Code != http.StatusOK || !strings.Contains(comparison.Body, `"comparable":true`) || !strings.Contains(comparison.Body, `"parser_method":"structured_json"`) {
		t.Fatalf("comparison status=%d body=%s", comparison.Code, comparison.Body)
	}
	p2Started := performJSON(h, "POST", "/v1/projects/"+accepted.ProjectID+"/tutorial-experiments/"+installed.ID+"/runs", `{"variant":"p2_recursive_400_80","idempotency_key":"http_live_run_p2"}`, token)
	if p2Started.Code != http.StatusAccepted {
		t.Fatalf("P2 start status=%d body=%s", p2Started.Code, p2Started.Body)
	}
	var p2Accepted tutorialExperimentRunAcceptedResponse
	if err := json.Unmarshal([]byte(p2Started.Body), &p2Accepted); err != nil {
		t.Fatal(err)
	}
	var p2Run tutorial.ExperimentRun
	for range 200 {
		polled := performJSON(h, "GET", p2Accepted.PollURL, "", token)
		if polled.Code != http.StatusOK {
			t.Fatalf("P2 poll status=%d body=%s", polled.Code, polled.Body)
		}
		if err := json.Unmarshal([]byte(polled.Body), &p2Run); err != nil {
			t.Fatal(err)
		}
		if p2Run.Status == tutorial.ExperimentRunCompleted || p2Run.Status == tutorial.ExperimentRunFailed {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if p2Run.Status != tutorial.ExperimentRunCompleted || p2Run.BaselineRunID != run.ID || p2Run.ParserMethod != "basic" || p2Run.ChunkSizeTokens != tutorial.TutorialP2ChunkSizeTokens || p2Run.IndexedChunkCount == 0 {
		t.Fatalf("P2 run=%#v baseline=%#v", p2Run, run)
	}
	p2Comparison := performJSON(h, "GET", p2Accepted.PollURL+"/comparison", "", token)
	if p2Comparison.Code != http.StatusOK || !strings.Contains(p2Comparison.Body, `"comparable":true`) || !strings.Contains(p2Comparison.Body, `"index_metrics"`) || !strings.Contains(p2Comparison.Body, `"chunk_count"`) {
		t.Fatalf("P2 comparison status=%d body=%s", p2Comparison.Code, p2Comparison.Body)
	}
	queued, _, err := application.TutorialRuns.Start(context.Background(), tutorial.Subject{TenantID: "tenant_a", ID: "http_test"}, accepted.ProjectID, "http_live_run_cancel")
	if err != nil {
		t.Fatalf("create queued live run: %v", err)
	}
	cancelled := performJSON(h, "POST", "/v1/projects/"+accepted.ProjectID+"/tutorial-experiments/"+installed.ID+"/runs/"+queued.ID+":cancel", "", token)
	if cancelled.Code != http.StatusAccepted || !strings.Contains(cancelled.Body, `"status":"cancelled"`) {
		t.Fatalf("live run cancel status=%d body=%s", cancelled.Code, cancelled.Body)
	}
	missingRetry := performJSON(h, "POST", "/v1/tutorial-clone-jobs/missing:retry", "", token)
	if missingRetry.Code != http.StatusNotFound {
		t.Fatalf("retry route status=%d body=%s", missingRetry.Code, missingRetry.Body)
	}
}

func TestVersionRoute(t *testing.T) {
	h, _, closeApp := newTestHertzWithApp(t)
	defer closeApp()
	response := performJSON(h, "GET", "/version", "", "")
	if response.Code != 200 || !strings.Contains(response.Body, `"version"`) || !strings.Contains(response.Body, `"commit"`) {
		t.Fatalf("version status=%d body=%s", response.Code, response.Body)
	}
}

func TestInteractiveDocumentationRoutes(t *testing.T) {
	h, _, closeApp := newTestHertzWithApp(t)
	defer closeApp()

	docs := performJSON(h, "GET", "/docs", "", "")
	if docs.Code != 200 || !strings.Contains(docs.Body, "SwaggerUIBundle") || !strings.Contains(docs.Body, "/openapi.yaml") {
		t.Fatalf("docs status=%d body=%s", docs.Code, docs.Body)
	}

	spec := performJSON(h, "GET", "/openapi.yaml", "", "")
	if spec.Code != 200 || !strings.Contains(spec.Body, "openapi: 3.") || !strings.Contains(spec.Body, "/v1/query:") {
		t.Fatalf("openapi status=%d body=%s", spec.Code, spec.Body)
	}

	for _, path := range []string{"/docs/assets/swagger-ui.css", "/docs/assets/swagger-ui-bundle.js"} {
		asset := performJSON(h, "GET", path, "", "")
		if asset.Code != 200 || len(asset.Body) < 1_000 {
			t.Fatalf("asset %s status=%d size=%d", path, asset.Code, len(asset.Body))
		}
	}
}

func TestTutorialCatalogRouteErrorsAndAuthentication(t *testing.T) {
	h, application, closeApp := newTestHertzWithApp(t)
	defer closeApp()
	token := issueToken(t, application, "tenant_a")

	unauthenticated := performJSON(h, "GET", "/v1/tutorials", "", "")
	if unauthenticated.Code != 401 {
		t.Fatalf("unauthenticated status = %d body=%s", unauthenticated.Code, unauthenticated.Body)
	}
	missingTemplate := performJSONWithTrace(h, "GET", "/v1/tutorials/missing", "", token, "trace_tutorial_missing")
	assertErrorResponse(t, missingTemplate, 404, "tutorial_not_found", "trace_tutorial_missing")
	missingVersion := performJSONWithTrace(h, "GET", "/v1/tutorials/text-rag/versions/9.9.9", "", token, "trace_tutorial_version_missing")
	assertErrorResponse(t, missingVersion, 404, "tutorial_version_not_found", "trace_tutorial_version_missing")

	application.Config.Tutorial.CatalogBaseURL = "http://insecure.example.test/packs"
	invalidCatalog := performJSONWithTrace(h, "GET", "/v1/tutorials", "", token, "trace_tutorial_catalog_failed")
	assertErrorResponse(t, invalidCatalog, 500, "tutorial_catalog_failed", "trace_tutorial_catalog_failed")
}

func TestResolveTutorialManifestURLRejectsEncodedTraversal(t *testing.T) {
	_, err := resolveTutorialManifestURL("https://example.test/packs", "%2e%2e/private/manifest.json")
	if err == nil {
		t.Fatal("resolveTutorialManifestURL() error = nil, want encoded traversal error")
	}
}

func TestProjectsAreTenantScoped(t *testing.T) {
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()
	app.Projects = project.NewService(newHTTPProjectRepository(), time.Now)
	tenantA := issueToken(t, app, "tenant_a")
	tenantB := issueToken(t, app, "tenant_b")
	created := performJSON(h, "POST", "/v1/projects", `{"name":"Support","description":"Customer support"}`, tenantA)
	if created.Code != 201 {
		t.Fatalf("create status = %d body=%s", created.Code, created.Body)
	}
	var item project.Project
	if err := json.Unmarshal([]byte(created.Body), &item); err != nil {
		t.Fatal(err)
	}
	foreign := performJSON(h, "GET", "/v1/projects/"+item.ID, "", tenantB)
	if foreign.Code != 404 || !strings.Contains(foreign.Body, `"code":"project_not_found"`) {
		t.Fatalf("foreign get status = %d body=%s", foreign.Code, foreign.Body)
	}
	listed := performJSON(h, "GET", "/v1/projects", "", tenantA)
	if listed.Code != 200 || !strings.Contains(listed.Body, `"projects"`) {
		t.Fatalf("list status = %d body=%s", listed.Code, listed.Body)
	}
	updated := performJSON(h, "PATCH", "/v1/projects/"+item.ID, `{"name":"Support Ops"}`, tenantA)
	if updated.Code != 200 || !strings.Contains(updated.Body, `"name":"Support Ops"`) {
		t.Fatalf("update status = %d body=%s", updated.Code, updated.Body)
	}
}

func TestAPIKeyLifecycleAndProjectAuthorization(t *testing.T) {
	h, application, closeApp := newTestHertzWithApp(t)
	defer closeApp()
	adminToken := issueToken(t, application, "tenant_a")

	firstProjectResponse := performJSON(h, "POST", "/v1/projects", `{"name":"First"}`, adminToken)
	secondProjectResponse := performJSON(h, "POST", "/v1/projects", `{"name":"Second"}`, adminToken)
	if firstProjectResponse.Code != 201 || secondProjectResponse.Code != 201 {
		t.Fatalf("project creation failed: first=%d second=%d", firstProjectResponse.Code, secondProjectResponse.Code)
	}
	var firstProject, secondProject project.Project
	if err := json.Unmarshal([]byte(firstProjectResponse.Body), &firstProject); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(secondProjectResponse.Body), &secondProject); err != nil {
		t.Fatal(err)
	}

	viewerResponse := performJSON(h, "POST", "/v1/api-keys", `{"name":"read-only robot","role":"project_viewer","project_id":"`+firstProject.ID+`"}`, adminToken)
	if viewerResponse.Code != 201 {
		t.Fatalf("viewer key status = %d body=%s", viewerResponse.Code, viewerResponse.Body)
	}
	var viewer auth.APIKeyCreateResult
	if err := json.Unmarshal([]byte(viewerResponse.Body), &viewer); err != nil {
		t.Fatal(err)
	}
	if viewer.Secret == "" || viewer.APIKey.KeyHash != "" {
		t.Fatalf("create response = %#v", viewer)
	}

	listed := performJSON(h, "GET", "/v1/api-keys", "", adminToken)
	if listed.Code != 200 || strings.Contains(listed.Body, viewer.Secret) || strings.Contains(listed.Body, "key_hash") {
		t.Fatalf("list status = %d body=%s", listed.Code, listed.Body)
	}
	ownProject := performJSON(h, "GET", "/v1/projects/"+firstProject.ID, "", viewer.Secret)
	if ownProject.Code != 200 {
		t.Fatalf("viewer own project status = %d body=%s", ownProject.Code, ownProject.Body)
	}
	assertErrorResponse(t, performJSONWithTrace(h, "GET", "/v1/projects", "", viewer.Secret, "trace_viewer_list"), 403, "forbidden", "trace_viewer_list")
	assertErrorResponse(t, performJSONWithTrace(h, "PATCH", "/v1/projects/"+firstProject.ID, `{"name":"Changed"}`, viewer.Secret, "trace_viewer_update"), 403, "forbidden", "trace_viewer_update")
	assertErrorResponse(t, performJSONWithTrace(h, "GET", "/v1/projects/"+secondProject.ID, "", viewer.Secret, "trace_cross_project"), 404, "project_not_found", "trace_cross_project")
	assertErrorResponse(t, performJSONWithTrace(h, "GET", "/v1/api-keys", "", viewer.Secret, "trace_viewer_keys"), 403, "forbidden", "trace_viewer_keys")
	if response := performJSON(h, "GET", "/v1/knowledge-bases", "", viewer.Secret); response.Code != 200 || !strings.Contains(response.Body, `"items":[]`) {
		t.Fatalf("empty project knowledge-base list status=%d body=%s", response.Code, response.Body)
	}
	assertErrorResponse(t, performJSONWithTrace(h, "GET", "/v1/tutorials", "", viewer.Secret, "trace_viewer_tutorials"), 403, "forbidden", "trace_viewer_tutorials")

	firstKBResponse := performJSON(h, "POST", "/v1/knowledge-bases", `{"name":"First docs","project_id":"`+firstProject.ID+`"}`, adminToken)
	secondKBResponse := performJSON(h, "POST", "/v1/knowledge-bases", `{"name":"Second docs","project_id":"`+secondProject.ID+`"}`, adminToken)
	if firstKBResponse.Code != 201 || secondKBResponse.Code != 201 {
		t.Fatalf("knowledge-base creation failed: first=%d second=%d", firstKBResponse.Code, secondKBResponse.Code)
	}
	var firstKB, secondKB kb.KnowledgeBase
	if err := json.Unmarshal([]byte(firstKBResponse.Body), &firstKB); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(secondKBResponse.Body), &secondKB); err != nil {
		t.Fatal(err)
	}
	viewerList := performJSON(h, "GET", "/v1/knowledge-bases", "", viewer.Secret)
	if viewerList.Code != 200 || !strings.Contains(viewerList.Body, firstKB.ID) || strings.Contains(viewerList.Body, secondKB.ID) {
		t.Fatalf("viewer project list status=%d body=%s", viewerList.Code, viewerList.Body)
	}
	if response := performJSON(h, "GET", "/v1/knowledge-bases/"+firstKB.ID, "", viewer.Secret); response.Code != 200 {
		t.Fatalf("viewer own knowledge base status=%d body=%s", response.Code, response.Body)
	}
	assertErrorResponse(t, performJSONWithTrace(h, "GET", "/v1/knowledge-bases/"+secondKB.ID, "", viewer.Secret, "trace_foreign_kb"), 404, "knowledge_base_not_found", "trace_foreign_kb")
	assertErrorResponse(t, performJSONWithTrace(h, "DELETE", "/v1/knowledge-bases/"+firstKB.ID, "", viewer.Secret, "trace_viewer_delete_kb"), 403, "forbidden", "trace_viewer_delete_kb")
	// Access is authorized, but production execution needs an evaluated,
	// activated version for this project.
	assertErrorResponse(t, performJSONWithTrace(h, "POST", "/v1/query", `{"knowledge_base_id":"`+firstKB.ID+`","query":"hello"}`, viewer.Secret, "trace_project_version_required"), 409, "production_version_unavailable", "trace_project_version_required")

	editorResponse := performJSON(h, "POST", "/v1/api-keys", `{"name":"project editor","role":"project_editor","project_id":"`+firstProject.ID+`"}`, adminToken)
	if editorResponse.Code != 201 {
		t.Fatalf("editor key status=%d body=%s", editorResponse.Code, editorResponse.Body)
	}
	var editor auth.APIKeyCreateResult
	if err := json.Unmarshal([]byte(editorResponse.Body), &editor); err != nil {
		t.Fatal(err)
	}
	editorKBResponse := performJSON(h, "POST", "/v1/knowledge-bases", `{"name":"Editor docs"}`, editor.Secret)
	if editorKBResponse.Code != 201 || !strings.Contains(editorKBResponse.Body, `"project_id":"`+firstProject.ID+`"`) {
		t.Fatalf("editor knowledge base status=%d body=%s", editorKBResponse.Code, editorKBResponse.Body)
	}
	editorDatasetResponse := performJSON(h, "POST", "/v1/datasets", `{"name":"Editor eval","kind":"golden"}`, editor.Secret)
	if editorDatasetResponse.Code != 201 || !strings.Contains(editorDatasetResponse.Body, `"project_id":"`+firstProject.ID+`"`) {
		t.Fatalf("editor dataset status=%d body=%s", editorDatasetResponse.Code, editorDatasetResponse.Body)
	}
	var editorDataset dataset.Dataset
	if err := json.Unmarshal([]byte(editorDatasetResponse.Body), &editorDataset); err != nil {
		t.Fatal(err)
	}
	if response := performJSON(h, "POST", "/v1/datasets/"+editorDataset.ID+"/items", `{"query":"hello","ground_truth":"Insufficient context."}`, editor.Secret); response.Code != 201 {
		t.Fatalf("editor dataset item status=%d body=%s", response.Code, response.Body)
	}
	evaluationResponse := performJSON(h, "POST", "/v1/evaluations", `{"dataset_id":"`+editorDataset.ID+`","knowledge_base_id":"`+firstKB.ID+`","profile":"realtime"}`, editor.Secret)
	if evaluationResponse.Code != 202 {
		t.Fatalf("editor evaluation status=%d body=%s", evaluationResponse.Code, evaluationResponse.Body)
	}
	var evaluation evalpkg.RunResult
	if err := json.Unmarshal([]byte(evaluationResponse.Body), &evaluation); err != nil {
		t.Fatal(err)
	}
	if evaluation.ProjectID != firstProject.ID {
		t.Fatalf("evaluation project=%q want=%q", evaluation.ProjectID, firstProject.ID)
	}
	if response := performJSON(h, "GET", "/v1/evaluations/"+evaluation.ID, "", viewer.Secret); response.Code != 200 {
		t.Fatalf("viewer evaluation status=%d body=%s", response.Code, response.Body)
	}
	mismatch := performJSONWithTrace(h, "POST", "/v1/evaluations", `{"dataset_id":"`+editorDataset.ID+`","knowledge_base_id":"`+secondKB.ID+`","profile":"realtime"}`, adminToken, "trace_evaluation_project_mismatch")
	assertErrorResponse(t, mismatch, 400, "project_mismatch", "trace_evaluation_project_mismatch")

	optimizationResponse := performJSON(h, "POST", "/v1/optimizations", `{"dataset_id":"`+editorDataset.ID+`","knowledge_base_id":"`+firstKB.ID+`","profiles":["realtime"],"top_ks":[1]}`, editor.Secret)
	if optimizationResponse.Code != 202 {
		t.Fatalf("editor optimization status=%d body=%s", optimizationResponse.Code, optimizationResponse.Body)
	}
	var optimization struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal([]byte(optimizationResponse.Body), &optimization); err != nil {
		t.Fatal(err)
	}
	if response := performJSON(h, "GET", "/v1/optimizations/"+optimization.RunID, "", viewer.Secret); response.Code != 200 || !strings.Contains(response.Body, `"project_id":"`+firstProject.ID+`"`) {
		t.Fatalf("viewer optimization status=%d body=%s", response.Code, response.Body)
	}
	secondViewerResponse := performJSON(h, "POST", "/v1/api-keys", `{"name":"second viewer","role":"project_viewer","project_id":"`+secondProject.ID+`"}`, adminToken)
	if secondViewerResponse.Code != 201 {
		t.Fatalf("second viewer status=%d body=%s", secondViewerResponse.Code, secondViewerResponse.Body)
	}
	var secondViewer auth.APIKeyCreateResult
	if err := json.Unmarshal([]byte(secondViewerResponse.Body), &secondViewer); err != nil {
		t.Fatal(err)
	}
	assertErrorResponse(t, performJSONWithTrace(h, "GET", "/v1/evaluations/"+evaluation.ID, "", secondViewer.Secret, "trace_foreign_evaluation"), 404, "evaluation_not_found", "trace_foreign_evaluation")
	assertErrorResponse(t, performJSONWithTrace(h, "GET", "/v1/optimizations/"+optimization.RunID, "", secondViewer.Secret, "trace_foreign_optimization"), 404, "optimization_not_found", "trace_foreign_optimization")
	assertErrorResponse(t, performJSONWithTrace(h, "POST", "/v1/datasets", `{"name":"Denied"}`, viewer.Secret, "trace_viewer_create_dataset"), 403, "forbidden", "trace_viewer_create_dataset")

	machineAdminResponse := performJSON(h, "POST", "/v1/api-keys", `{"name":"automation admin","role":"tenant_admin"}`, adminToken)
	if machineAdminResponse.Code != 201 {
		t.Fatalf("machine admin status = %d body=%s", machineAdminResponse.Code, machineAdminResponse.Body)
	}
	var machineAdmin auth.APIKeyCreateResult
	if err := json.Unmarshal([]byte(machineAdminResponse.Body), &machineAdmin); err != nil {
		t.Fatal(err)
	}
	if response := performJSON(h, "GET", "/v1/api-keys", "", machineAdmin.Secret); response.Code != 200 {
		t.Fatalf("machine admin list status = %d body=%s", response.Code, response.Body)
	}

	revoked := performJSON(h, "DELETE", "/v1/api-keys/"+viewer.APIKey.ID, "", adminToken)
	if revoked.Code != 204 {
		t.Fatalf("revoke status = %d body=%s", revoked.Code, revoked.Body)
	}
	assertErrorResponse(t, performJSONWithTrace(h, "GET", "/v1/projects/"+firstProject.ID, "", viewer.Secret, "trace_revoked_key"), 401, "invalid_bearer_token", "trace_revoked_key")
	assertErrorResponse(t, performJSONWithTrace(h, "GET", "/v1/projects/"+firstProject.ID, "", "orag_sk_invalid", "trace_invalid_key"), 401, "invalid_bearer_token", "trace_invalid_key")
}

func TestAPIKeyCreateValidatesRoleAndProject(t *testing.T) {
	h, application, closeApp := newTestHertzWithApp(t)
	defer closeApp()
	adminToken := issueToken(t, application, "tenant_a")

	missingProject := performJSONWithTrace(h, "POST", "/v1/api-keys", `{"name":"viewer","role":"project_viewer"}`, adminToken, "trace_key_missing_project")
	assertErrorResponse(t, missingProject, 400, "invalid_request", "trace_key_missing_project")
	foreignProject := performJSONWithTrace(h, "POST", "/v1/api-keys", `{"name":"viewer","role":"project_viewer","project_id":"prj_missing"}`, adminToken, "trace_key_foreign_project")
	assertErrorResponse(t, foreignProject, 404, "project_not_found", "trace_key_foreign_project")
	unknownRole := performJSONWithTrace(h, "POST", "/v1/api-keys", `{"name":"robot","role":"owner"}`, adminToken, "trace_key_role")
	assertErrorResponse(t, unknownRole, 400, "invalid_request", "trace_key_role")
	missingKey := performJSONWithTrace(h, "DELETE", "/v1/api-keys/key_missing", "", adminToken, "trace_key_missing")
	assertErrorResponse(t, missingKey, 404, "api_key_not_found", "trace_key_missing")
}

func TestProjectHandlersReturnStableErrors(t *testing.T) {
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()
	repo := newHTTPProjectRepository()
	app.Projects = project.NewService(repo, time.Now)
	token := issueToken(t, app, "tenant_a")
	invalid := performJSONWithTrace(h, "POST", "/v1/projects", `{"name":" "}`, token, "trace_project_invalid")
	assertErrorResponse(t, invalid, 400, "invalid_request", "trace_project_invalid")
	missing := performJSONWithTrace(h, "GET", "/v1/projects/prj_missing", "", token, "trace_project_missing")
	assertErrorResponse(t, missing, 404, "project_not_found", "trace_project_missing")
	repo.createErr = project.ErrConflict
	conflict := performJSONWithTrace(h, "POST", "/v1/projects", `{"name":"Support"}`, token, "trace_project_conflict")
	assertErrorResponse(t, conflict, 409, "project_conflict", "trace_project_conflict")

	repo.createErr = errors.New("database unavailable")
	failed := performJSONWithTrace(h, "POST", "/v1/projects", `{"name":"Support"}`, token, "trace_project_create_failed")
	assertErrorResponse(t, failed, 500, "project_create_failed", "trace_project_create_failed")
	repo.createErr = nil
	repo.listErr = errors.New("database unavailable")
	failed = performJSONWithTrace(h, "GET", "/v1/projects", "", token, "trace_project_list_failed")
	assertErrorResponse(t, failed, 500, "project_list_failed", "trace_project_list_failed")
	repo.listErr = nil
	repo.getErr = errors.New("database unavailable")
	failed = performJSONWithTrace(h, "GET", "/v1/projects/prj_1", "", token, "trace_project_lookup_failed")
	assertErrorResponse(t, failed, 500, "project_lookup_failed", "trace_project_lookup_failed")
	repo.getErr = nil
	repo.items["prj_1"] = project.Project{ID: "prj_1", TenantID: "tenant_a", Name: "Support"}
	repo.updateErr = errors.New("database unavailable")
	failed = performJSONWithTrace(h, "PATCH", "/v1/projects/prj_1", `{"name":"Support Ops"}`, token, "trace_project_update_failed")
	assertErrorResponse(t, failed, 500, "project_update_failed", "trace_project_update_failed")
}

func TestLoginValidatesPassword(t *testing.T) {
	h, closeApp := newTestHertz(t)
	defer closeApp()

	resp := performJSON(h, "POST", "/v1/auth/login", `{"username":"admin","password":"secret"}`, "")
	if resp.Code != 200 {
		t.Fatalf("login status = %d body=%s", resp.Code, resp.Body)
	}
	var body struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &body); err != nil {
		t.Fatal(err)
	}
	if body.AccessToken == "" {
		t.Fatal("missing access token")
	}

	resp = performJSON(h, "POST", "/v1/auth/login", `{"username":"admin","password":"wrong"}`, "")
	if resp.Code != 401 {
		t.Fatalf("wrong password status = %d body=%s", resp.Code, resp.Body)
	}
	if !strings.Contains(resp.Body, `"code":"invalid_credentials"`) {
		t.Fatalf("unexpected body: %s", resp.Body)
	}
}

func TestAuthMiddlewareAndInvalidJSON(t *testing.T) {
	h, closeApp := newTestHertz(t)
	defer closeApp()

	resp := performJSONWithTrace(h, "GET", "/v1/knowledge-bases", "", "", "trace_http_error")
	if resp.Code != 401 {
		t.Fatalf("missing token status = %d body=%s", resp.Code, resp.Body)
	}
	if !strings.Contains(resp.Body, `"trace_id":"trace_http_error"`) {
		t.Fatalf("error response missing request trace id: %s", resp.Body)
	}
	if got := resp.TraceIDHeader; got != "trace_http_error" {
		t.Fatalf("response trace header = %q, want trace_http_error", got)
	}

	resp = performJSON(h, "POST", "/v1/auth/login", `{`, "")
	if resp.Code != 400 {
		t.Fatalf("invalid json status = %d body=%s", resp.Code, resp.Body)
	}
	if !strings.Contains(resp.Body, `"code":"invalid_json"`) {
		t.Fatalf("unexpected body: %s", resp.Body)
	}
}

func TestRalphLoopHTTPRuntimeIsPlannedOnly(t *testing.T) {
	h, closeApp := newTestHertz(t)
	defer closeApp()

	token := loginToken(t, h)
	resp := performJSONWithTrace(h, "POST", "/v1/ralph-loop", `{"task_spec_path":"tasks.md","task_id":"Task 1","mode":"focused","max_rounds":1}`, token, "trace_ralph_loop_planned_only")
	if resp.Code != 404 {
		t.Fatalf("ralph-loop status = %d, want 404 for planned-only runtime boundary body=%s", resp.Code, resp.Body)
	}
	if strings.Contains(resp.Body, `"verdict"`) || strings.Contains(resp.Body, `"status":"completed"`) {
		t.Fatalf("planned-only endpoint returned runnable Ralph Loop payload: %s", resp.Body)
	}
}

func TestCreateKnowledgeBaseMemoryBackendReturnsCreated(t *testing.T) {
	h, closeApp := newTestHertz(t)
	defer closeApp()

	token := loginToken(t, h)
	resp := performJSON(h, "POST", "/v1/knowledge-bases", `{"name":"Docs","description":"Team docs","metadata":{"owner":"search"}}`, token)
	if resp.Code != 201 {
		t.Fatalf("create status = %d body=%s", resp.Code, resp.Body)
	}
	var created kb.KnowledgeBase
	if err := json.Unmarshal([]byte(resp.Body), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.TenantID != "tenant_default" || created.Name != "Docs" {
		t.Fatalf("unexpected create response: %#v", created)
	}
}

func TestKnowledgeBaseAndDatasetAcceptOwnedProject(t *testing.T) {
	h, closeApp := newTestHertz(t)
	defer closeApp()
	token := loginToken(t, h)
	projectID := project.LegacyDefaultID("tenant_default")

	kbResponse := performJSON(h, "POST", "/v1/knowledge-bases", `{"name":"Project docs","project_id":"`+projectID+`"}`, token)
	if kbResponse.Code != 201 {
		t.Fatalf("knowledge base status=%d body=%s", kbResponse.Code, kbResponse.Body)
	}
	var knowledgeBase kb.KnowledgeBase
	if err := json.Unmarshal([]byte(kbResponse.Body), &knowledgeBase); err != nil {
		t.Fatal(err)
	}
	if knowledgeBase.ProjectID != projectID {
		t.Fatalf("knowledge base project = %q, want %q", knowledgeBase.ProjectID, projectID)
	}

	datasetResponse := performJSON(h, "POST", "/v1/datasets", `{"name":"Project evaluation","kind":"golden","project_id":"`+projectID+`"}`, token)
	if datasetResponse.Code != 201 {
		t.Fatalf("dataset status=%d body=%s", datasetResponse.Code, datasetResponse.Body)
	}
	var createdDataset dataset.Dataset
	if err := json.Unmarshal([]byte(datasetResponse.Body), &createdDataset); err != nil {
		t.Fatal(err)
	}
	if createdDataset.ProjectID != projectID {
		t.Fatalf("dataset project = %q, want %q", createdDataset.ProjectID, projectID)
	}

	missing := performJSONWithTrace(h, "POST", "/v1/knowledge-bases", `{"name":"Missing","project_id":"prj_missing"}`, token, "trace_kb_project_missing")
	assertErrorResponse(t, missing, 404, "project_not_found", "trace_kb_project_missing")
}

func TestCreateKnowledgeBaseStorageError(t *testing.T) {
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()

	token := loginToken(t, h)
	app.KBStore = fakeKnowledgeBaseRepository{putErr: errors.New("insert failed")}

	resp := performJSONWithTrace(h, "POST", "/v1/knowledge-bases", `{"name":"Docs"}`, token, "trace_kb_create_error")
	if resp.Code == 201 {
		t.Fatalf("create status = 201, want storage error body=%s", resp.Body)
	}
	assertErrorResponse(t, resp, 500, "knowledge_base_create_failed", "trace_kb_create_error")
}

func TestKnowledgeBaseListStorageError(t *testing.T) {
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()

	token := loginToken(t, h)
	app.KBStore = fakeKnowledgeBaseRepository{listErr: errors.New("list failed")}

	resp := performJSONWithTrace(h, "GET", "/v1/knowledge-bases", "", token, "trace_kb_list_error")
	if resp.Code == 200 {
		t.Fatalf("list status = 200, want storage error body=%s", resp.Body)
	}
	if strings.Contains(resp.Body, `"items":[]`) {
		t.Fatalf("list storage error returned empty items: %s", resp.Body)
	}
	assertErrorResponse(t, resp, 500, "knowledge_base_list_failed", "trace_kb_list_error")
}

func TestKnowledgeBaseGetStorageError(t *testing.T) {
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()

	token := loginToken(t, h)
	app.KBStore = fakeKnowledgeBaseRepository{getErr: errors.New("lookup failed")}

	resp := performJSONWithTrace(h, "GET", "/v1/knowledge-bases/kb_default", "", token, "trace_kb_get_error")
	if resp.Code == 404 {
		t.Fatalf("get storage error status = 404, want 500 body=%s", resp.Body)
	}
	assertErrorResponse(t, resp, 500, "knowledge_base_lookup_failed", "trace_kb_get_error")
}

func TestKnowledgeBaseGetNotFound(t *testing.T) {
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()

	token := loginToken(t, h)
	app.KBStore = fakeKnowledgeBaseRepository{getFound: false}

	resp := performJSONWithTrace(h, "GET", "/v1/knowledge-bases/kb_missing", "", token, "trace_kb_not_found")
	assertErrorResponse(t, resp, 404, "knowledge_base_not_found", "trace_kb_not_found")
}

func TestHTTPCompletionLogIncludesTraceAndErrorCodeWithoutSensitiveBody(t *testing.T) {
	var logs bytes.Buffer
	h, closeApp := newTestHertzWithLogger(t, slog.New(slog.NewJSONHandler(&logs, nil)))
	defer closeApp()

	resp := performJSONWithTrace(h, "POST", "/v1/auth/login", `{"username":"admin","password":"raw-token prompt document model-response"`, "", "trace_http_log")
	if resp.Code != 400 {
		t.Fatalf("invalid json status = %d body=%s", resp.Code, resp.Body)
	}

	line := logs.String()
	for _, want := range []string{
		`"msg":"http_request_completed"`,
		`"method":"POST"`,
		`"route":"/v1/auth/login"`,
		`"path":"/v1/auth/login"`,
		`"status":400`,
		`"trace_id":"trace_http_log"`,
		`"error_code":"invalid_json"`,
		`"latency":`,
	} {
		if !strings.Contains(line, want) {
			t.Fatalf("completion log missing %s: %s", want, line)
		}
	}
	for _, forbidden := range []string{"raw-token", "prompt document", "model-response"} {
		if strings.Contains(line, forbidden) {
			t.Fatalf("completion log leaked %q: %s", forbidden, line)
		}
	}
}

func TestQueryUsesRequestTraceID(t *testing.T) {
	h, closeApp := newTestHertz(t)
	defer closeApp()

	token := loginToken(t, h)
	resp := performJSONWithTrace(h, "POST", "/v1/query", `{"knowledge_base_id":"kb_default","query":"hello"}`, token, "trace_query_success")
	if resp.Code != 200 {
		t.Fatalf("query status = %d body=%s", resp.Code, resp.Body)
	}
	var body rag.QueryResponse
	if err := json.Unmarshal([]byte(resp.Body), &body); err != nil {
		t.Fatal(err)
	}
	if body.TraceID != "trace_query_success" {
		t.Fatalf("query trace_id = %q, want trace_query_success", body.TraceID)
	}
}

func TestProjectQueryUsesServerOwnedProductionRunner(t *testing.T) {
	h, application, closeApp := newTestHertzWithApp(t)
	defer closeApp()
	token := loginToken(t, h)

	projectResponse := performJSON(h, "POST", "/v1/projects", `{"name":"Versioned support"}`, token)
	if projectResponse.Code != 201 {
		t.Fatalf("create project status=%d body=%s", projectResponse.Code, projectResponse.Body)
	}
	var projectItem project.Project
	if err := json.Unmarshal([]byte(projectResponse.Body), &projectItem); err != nil {
		t.Fatal(err)
	}
	kbResponse := performJSON(h, "POST", "/v1/knowledge-bases", `{"name":"Versioned docs","project_id":"`+projectItem.ID+`"}`, token)
	if kbResponse.Code != 201 {
		t.Fatalf("create knowledge base status=%d body=%s", kbResponse.Code, kbResponse.Body)
	}
	var knowledgeBase kb.KnowledgeBase
	if err := json.Unmarshal([]byte(kbResponse.Body), &knowledgeBase); err != nil {
		t.Fatal(err)
	}
	runner := &recordingProductionQuery{}
	application.ProductionQuery = runner

	response := performJSONWithTrace(h, "POST", "/v1/query", `{"knowledge_base_id":"`+knowledgeBase.ID+`","query":"hello","top_k":4}`, token, "trace_project_production")
	if response.Code != 200 {
		t.Fatalf("project query status=%d body=%s", response.Code, response.Body)
	}
	if len(runner.requests) != 1 {
		t.Fatalf("production runner calls=%d, want 1", len(runner.requests))
	}
	request := runner.requests[0]
	if request.ProjectID != projectItem.ID || request.KnowledgeBaseID != knowledgeBase.ID || request.TenantID != "tenant_default" || request.TraceID != "trace_project_production" || request.TopK != 4 {
		t.Fatalf("production runner request=%#v", request)
	}
}

func TestProjectQueryRejectsMissingProductionVersion(t *testing.T) {
	h, _, closeApp := newTestHertzWithApp(t)
	defer closeApp()
	token := loginToken(t, h)
	projectResponse := performJSON(h, "POST", "/v1/projects", `{"name":"Version required"}`, token)
	if projectResponse.Code != 201 {
		t.Fatalf("create project status=%d body=%s", projectResponse.Code, projectResponse.Body)
	}
	var projectItem project.Project
	if err := json.Unmarshal([]byte(projectResponse.Body), &projectItem); err != nil {
		t.Fatal(err)
	}
	kbResponse := performJSON(h, "POST", "/v1/knowledge-bases", `{"name":"Versioned docs","project_id":"`+projectItem.ID+`"}`, token)
	if kbResponse.Code != 201 {
		t.Fatalf("create knowledge base status=%d body=%s", kbResponse.Code, kbResponse.Body)
	}
	var knowledgeBase kb.KnowledgeBase
	if err := json.Unmarshal([]byte(kbResponse.Body), &knowledgeBase); err != nil {
		t.Fatal(err)
	}
	assertErrorResponse(t, performJSONWithTrace(h, "POST", "/v1/query", `{"knowledge_base_id":"`+knowledgeBase.ID+`","query":"hello"}`, token, "trace_missing_production_version"), 409, "production_version_unavailable", "trace_missing_production_version")
}

func TestQueryDirectRouteResponseMatchesOpenAPI(t *testing.T) {
	t.Setenv("RAG_QUERY_ROUTER_ENABLED", "true")
	h, closeApp := newTestHertz(t)
	defer closeApp()

	token := loginToken(t, h)
	resp := performJSONWithTrace(h, "POST", "/v1/query", `{"knowledge_base_id":"kb_default","query":"你好"}`, token, "trace_query_direct_route")
	if resp.Code != 200 {
		t.Fatalf("query status = %d body=%s", resp.Code, resp.Body)
	}

	var body rag.QueryResponse
	if err := json.Unmarshal([]byte(resp.Body), &body); err != nil {
		t.Fatal(err)
	}
	if body.CacheStatus != "bypass" {
		t.Fatalf("cache_status = %q, want bypass body=%s", body.CacheStatus, resp.Body)
	}
	if body.Route == nil || body.Route.Route != rag.QueryRouteDirect {
		t.Fatalf("route = %#v, want direct body=%s", body.Route, resp.Body)
	}

	var payload any
	if err := json.Unmarshal([]byte(resp.Body), &payload); err != nil {
		t.Fatal(err)
	}
	doc, err := openapi3.NewLoader().LoadFromFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatalf("openapi validation failed: %v", err)
	}
	schemaRef := doc.Components.Schemas["QueryResponse"]
	if schemaRef == nil || schemaRef.Value == nil {
		t.Fatal("OpenAPI missing QueryResponse schema")
	}
	if err := schemaRef.Value.VisitJSON(payload, openapi3.VisitAsResponse()); err != nil {
		t.Fatalf("direct route response does not match OpenAPI QueryResponse schema: %v body=%s", err, resp.Body)
	}
}

func TestQueryRepeatedTraceIDInvokesPipelinePerRequest(t *testing.T) {
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()
	pipeline := &recordingPipeline{}
	app.RAG.Pipeline = pipeline

	token := loginToken(t, h)
	traceID := "trace_query_reused"
	for _, query := range []string{"first repeated trace query", "second repeated trace query"} {
		resp := performJSONWithTrace(h, "POST", "/v1/query", `{"knowledge_base_id":"kb_default","query":"`+query+`"}`, token, traceID)
		if resp.Code != 200 {
			t.Fatalf("query status = %d body=%s", resp.Code, resp.Body)
		}
		var body rag.QueryResponse
		if err := json.Unmarshal([]byte(resp.Body), &body); err != nil {
			t.Fatal(err)
		}
		if body.TraceID != traceID {
			t.Fatalf("query trace_id = %q, want %q", body.TraceID, traceID)
		}
		if resp.TraceIDHeader != traceID {
			t.Fatalf("response trace header = %q, want %q", resp.TraceIDHeader, traceID)
		}
	}

	if len(pipeline.requests) != 2 {
		t.Fatalf("pipeline requests = %d, want 2", len(pipeline.requests))
	}
	for i, req := range pipeline.requests {
		if req.TraceID != traceID {
			t.Fatalf("pipeline request %d trace_id = %q, want %q", i+1, req.TraceID, traceID)
		}
	}
	if pipeline.requests[0].Query == pipeline.requests[1].Query {
		t.Fatalf("pipeline requests were not distinct: %#v", pipeline.requests)
	}
}

func TestTraceStatsReturnsTenantNodeStats(t *testing.T) {
	h, closeApp := newTestHertz(t)
	defer closeApp()

	token := loginToken(t, h)
	resp := performJSONWithTrace(h, "POST", "/v1/query", `{"knowledge_base_id":"kb_default","query":"trace stats"}`, token, "trace_stats_http")
	if resp.Code != 200 {
		t.Fatalf("query status = %d body=%s", resp.Code, resp.Body)
	}

	resp = performJSON(h, "GET", "/v1/traces:stats?limit=10", "", token)
	if resp.Code != 200 {
		t.Fatalf("trace stats status = %d body=%s", resp.Code, resp.Body)
	}
	if !strings.Contains(resp.Body, `"tenant_id":"tenant_default"`) || !strings.Contains(resp.Body, `"node_name":"init"`) {
		t.Fatalf("trace stats body = %s", resp.Body)
	}
}

type queryValidationCase struct {
	name    string
	body    string
	traceID string
}

func queryValidationCases(tracePrefix string) []queryValidationCase {
	return []queryValidationCase{
		{
			name:    "empty object",
			body:    `{}`,
			traceID: tracePrefix + "_empty_object",
		},
		{
			name:    "only query",
			body:    `{"query":"hello"}`,
			traceID: tracePrefix + "_only_query",
		},
		{
			name:    "only knowledge_base_id",
			body:    `{"knowledge_base_id":"kb_default"}`,
			traceID: tracePrefix + "_only_knowledge_base_id",
		},
		{
			name:    "blank knowledge_base_id",
			body:    `{"knowledge_base_id":"  ","query":"hello"}`,
			traceID: tracePrefix + "_blank_knowledge_base_id",
		},
		{
			name:    "blank query",
			body:    `{"knowledge_base_id":"kb_default","query":"  "}`,
			traceID: tracePrefix + "_blank_query",
		},
		{
			name:    "both blank strings",
			body:    `{"knowledge_base_id":"","query":""}`,
			traceID: tracePrefix + "_both_blank",
		},
		{
			name:    "invalid profile",
			body:    `{"knowledge_base_id":"kb_default","query":"hello","profile":"batch"}`,
			traceID: tracePrefix + "_invalid_profile",
		},
		{
			name:    "zero top_k",
			body:    `{"knowledge_base_id":"kb_default","query":"hello","top_k":0}`,
			traceID: tracePrefix + "_zero_top_k",
		},
		{
			name:    "negative top_k",
			body:    `{"knowledge_base_id":"kb_default","query":"hello","top_k":-1}`,
			traceID: tracePrefix + "_negative_top_k",
		},
		{
			name:    "too large top_k",
			body:    `{"knowledge_base_id":"kb_default","query":"hello","top_k":101}`,
			traceID: tracePrefix + "_too_large_top_k",
		},
	}
}

func TestQueryRejectsInvalidRequests(t *testing.T) {
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()
	pipeline := &countingPipeline{}
	app.RAG.Pipeline = pipeline

	token := loginToken(t, h)

	for _, tt := range queryValidationCases("trace_query_validation") {
		t.Run(tt.name, func(t *testing.T) {
			resp := performJSONWithTrace(h, "POST", "/v1/query", tt.body, token, tt.traceID)
			assertErrorResponse(t, resp, 400, "invalid_request", tt.traceID)

			var body ErrorResponse
			if err := json.Unmarshal([]byte(resp.Body), &body); err != nil {
				t.Fatal(err)
			}
			if body.Error.Code != "invalid_request" {
				t.Fatalf("error code = %q, want invalid_request body=%s", body.Error.Code, resp.Body)
			}
			if body.Error.TraceID != tt.traceID {
				t.Fatalf("error trace_id = %q, want %q body=%s", body.Error.TraceID, tt.traceID, resp.Body)
			}

			var topLevel map[string]json.RawMessage
			if err := json.Unmarshal([]byte(resp.Body), &topLevel); err != nil {
				t.Fatal(err)
			}
			if _, ok := topLevel["answer"]; ok {
				t.Fatalf("validation error returned query answer: %s", resp.Body)
			}
		})
	}

	if pipeline.calls != 0 {
		t.Fatalf("query pipeline called %d times for invalid requests", pipeline.calls)
	}
}

func TestQueryStreamRejectsInvalidRequests(t *testing.T) {
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()
	pipeline := &countingPipeline{}
	app.RAG.Pipeline = pipeline

	token := loginToken(t, h)
	for _, tt := range queryValidationCases("trace_query_stream_validation") {
		t.Run(tt.name, func(t *testing.T) {
			resp := performJSONWithTrace(h, "POST", "/v1/query:stream", tt.body, token, tt.traceID)
			assertErrorResponse(t, resp, 400, "invalid_request", tt.traceID)
			if strings.Contains(resp.ContentType, "text/event-stream") {
				t.Fatalf("pre-stream validation content type = %q, want non-SSE body=%s", resp.ContentType, resp.Body)
			}

			var body ErrorResponse
			if err := json.Unmarshal([]byte(resp.Body), &body); err != nil {
				t.Fatal(err)
			}
			if body.Error.Code != "invalid_request" {
				t.Fatalf("error code = %q, want invalid_request body=%s", body.Error.Code, resp.Body)
			}
			if body.Error.TraceID != tt.traceID {
				t.Fatalf("error trace_id = %q, want %q body=%s", body.Error.TraceID, tt.traceID, resp.Body)
			}
		})
	}

	if pipeline.calls != 0 {
		t.Fatalf("query stream pipeline called %d times for invalid requests", pipeline.calls)
	}
}

func TestQueryRequiresExistingKnowledgeBase(t *testing.T) {
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()

	token := loginToken(t, h)
	otherToken, err := app.Auth.IssueToken("tenant_other", "user_other")
	if err != nil {
		t.Fatal(err)
	}
	pipeline := &countingPipeline{}
	app.RAG.Pipeline = pipeline

	for _, tt := range []struct {
		name    string
		path    string
		body    string
		token   string
		traceID string
	}{
		{
			name:    "json missing knowledge base",
			path:    "/v1/query",
			body:    `{"knowledge_base_id":"kb_missing_query","query":"hello"}`,
			token:   token,
			traceID: "trace_query_missing_kb",
		},
		{
			name:    "stream missing knowledge base",
			path:    "/v1/query:stream",
			body:    `{"knowledge_base_id":"kb_missing_query","query":"hello"}`,
			token:   token,
			traceID: "trace_query_stream_missing_kb",
		},
		{
			name:    "json cross tenant knowledge base",
			path:    "/v1/query",
			body:    `{"knowledge_base_id":"kb_default","query":"hello"}`,
			token:   otherToken,
			traceID: "trace_query_cross_tenant_kb",
		},
		{
			name:    "stream cross tenant knowledge base",
			path:    "/v1/query:stream",
			body:    `{"knowledge_base_id":"kb_default","query":"hello"}`,
			token:   otherToken,
			traceID: "trace_query_stream_cross_tenant_kb",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			resp := performJSONWithTrace(h, "POST", tt.path, tt.body, tt.token, tt.traceID)
			assertErrorResponse(t, resp, 404, "knowledge_base_not_found", tt.traceID)
			if strings.Contains(resp.ContentType, "text/event-stream") {
				t.Fatalf("missing knowledge base response content type = %q, want JSON error body=%s", resp.ContentType, resp.Body)
			}
		})
	}

	if pipeline.calls != 0 {
		t.Fatalf("query pipeline called %d times for missing knowledge bases", pipeline.calls)
	}
}

func TestInvalidQueryRequestsDoNotIncrementRAGSuccessMetrics(t *testing.T) {
	h, closeApp := newTestHertz(t)
	defer closeApp()

	token := loginToken(t, h)
	tests := []struct {
		name string
		path string
		body string
	}{
		{
			name: "json",
			path: "/v1/query",
			body: `{"knowledge_base_id":"kb_default","query":""}`,
		},
		{
			name: "stream",
			path: "/v1/query:stream",
			body: `{"knowledge_base_id":"","query":"hello"}`,
		},
		{
			name: "invalid_profile",
			path: "/v1/query",
			body: `{"knowledge_base_id":"kb_default","query":"hello","profile":"batch"}`,
		},
		{
			name: "invalid_top_k",
			path: "/v1/query:stream",
			body: `{"knowledge_base_id":"kb_default","query":"hello","top_k":101}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			traceID := "trace_metrics_invalid_query_" + tt.name
			resp := performJSONWithTrace(h, "POST", tt.path, tt.body, token, traceID)
			assertErrorResponse(t, resp, 400, "invalid_request", traceID)
		})
	}

	resp := performJSON(h, "GET", "/metrics", "", "")
	if resp.Code != 200 {
		t.Fatalf("metrics status = %d body=%s", resp.Code, resp.Body)
	}
	if !strings.Contains(resp.Body, "orag_rag_queries_total 0\n") {
		t.Fatalf("invalid requests incremented total RAG queries: %s", resp.Body)
	}
	if strings.Contains(resp.Body, `outcome="success"`) {
		t.Fatalf("invalid requests incremented successful RAG query metrics: %s", resp.Body)
	}
}

func TestQueryStreamSSE(t *testing.T) {
	h, closeApp := newTestHertz(t)
	defer closeApp()

	token := loginToken(t, h)
	resp := performJSON(h, "POST", "/v1/query:stream", `{"knowledge_base_id":"kb_default","query":"hello"}`, token)
	if resp.Code != 200 {
		t.Fatalf("query stream status = %d body=%s", resp.Code, resp.Body)
	}
	if !strings.Contains(resp.ContentType, "text/event-stream") {
		t.Fatalf("content type = %q", resp.ContentType)
	}
	for _, event := range []string{"event: trace", "event: chunk", "event: citations", "event: done"} {
		if !strings.Contains(resp.Body, event) {
			t.Fatalf("sse body missing %q: %s", event, resp.Body)
		}
	}
}

func TestQueryStreamSSEErrorUsesRequestTraceID(t *testing.T) {
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()
	app.RAG.Pipeline = failingPipeline{err: errors.New("boom")}

	token := loginToken(t, h)
	resp := performJSONWithTrace(h, "POST", "/v1/query:stream", `{"knowledge_base_id":"kb_default","query":"hello"}`, token, "trace_sse_error")
	if resp.Code != 500 {
		t.Fatalf("query stream status = %d body=%s", resp.Code, resp.Body)
	}
	if !strings.Contains(resp.ContentType, "text/event-stream") {
		t.Fatalf("content type = %q", resp.ContentType)
	}
	if !strings.Contains(resp.Body, `"trace_id":"trace_sse_error"`) {
		t.Fatalf("sse error missing request trace id: %s", resp.Body)
	}
	resp = performJSON(h, "GET", "/metrics", "", "")
	if !strings.Contains(resp.Body, `orag_rag_errors_total{profile="default",error_code="query_failed"} 1`) {
		t.Fatalf("metrics missing rag error counter: %s", resp.Body)
	}
	if strings.Contains(resp.Body, "trace_sse_error") || strings.Contains(resp.Body, "query=") || strings.Contains(resp.Body, "session_id") {
		t.Fatalf("metrics contains high-cardinality request data: %s", resp.Body)
	}
}

func TestDatasetItemEvaluationAndOptimizationUseTokenTenant(t *testing.T) {
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()

	tenantAToken := loginToken(t, h)
	tenantBToken, err := app.Auth.IssueToken("tenant_b", "user_b")
	if err != nil {
		t.Fatal(err)
	}

	resp := performJSON(h, "POST", "/v1/datasets", `{"name":"regression","kind":"golden"}`, tenantAToken)
	if resp.Code != 201 {
		t.Fatalf("create dataset status = %d body=%s", resp.Code, resp.Body)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" {
		t.Fatalf("missing dataset id: %s", resp.Body)
	}

	evalBody := `{"dataset_id":"` + created.ID + `","knowledge_base_id":"kb_default","profile":"realtime"}`
	resp = performJSON(h, "POST", "/v1/evaluations", evalBody, tenantBToken)
	if resp.Code != 404 || !strings.Contains(resp.Body, `"code":"dataset_not_found"`) {
		t.Fatalf("cross-tenant evaluation status = %d body=%s", resp.Code, resp.Body)
	}
	optimizeBody := `{"dataset_id":"` + created.ID + `","knowledge_base_id":"kb_default","profiles":["realtime"],"top_ks":[1]}`
	resp = performJSON(h, "POST", "/v1/optimizations", optimizeBody, tenantBToken)
	if resp.Code != 404 || !strings.Contains(resp.Body, `"code":"dataset_not_found"`) {
		t.Fatalf("cross-tenant optimization status = %d body=%s", resp.Code, resp.Body)
	}

	itemBody := `{"query":"q","ground_truth":"a","relevant_doc_ids":["doc_1"]}`
	resp = performJSON(h, "POST", "/v1/datasets/"+created.ID+"/items", itemBody, tenantBToken)
	if resp.Code != 404 || !strings.Contains(resp.Body, `"code":"dataset_not_found"`) {
		t.Fatalf("cross-tenant item create status = %d body=%s", resp.Code, resp.Body)
	}
	resp = performJSON(h, "POST", "/v1/datasets/"+created.ID+"/items", itemBody, tenantAToken)
	if resp.Code != 201 {
		t.Fatalf("tenant item create status = %d body=%s", resp.Code, resp.Body)
	}

	resp = performJSON(h, "POST", "/v1/evaluations", evalBody, tenantAToken)
	if resp.Code != 202 {
		t.Fatalf("tenant evaluation status = %d body=%s", resp.Code, resp.Body)
	}
}

func TestRunEvaluationRejectsMissingKnowledgeBaseBeforeRAG(t *testing.T) {
	ctx := context.Background()
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()
	pipeline := &countingPipeline{resp: rag.QueryResponse{Answer: "should not run", CacheStatus: "miss"}}
	app.RAG.Pipeline = pipeline

	token := loginToken(t, h)
	ds, err := app.Datasets.Create(ctx, "tenant_default", "missing-kb-eval", "golden")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Datasets.AddItem(ctx, "tenant_default", ds.ID, dataset.Item{
		Query:       "q",
		GroundTruth: "a",
	}); err != nil {
		t.Fatal(err)
	}

	resp := performJSONWithTrace(h, "POST", "/v1/evaluations", `{"dataset_id":"`+ds.ID+`","knowledge_base_id":"kb_missing","profile":"realtime"}`, token, "trace_eval_missing_kb")
	assertErrorResponse(t, resp, 404, "knowledge_base_not_found", "trace_eval_missing_kb")
	if pipeline.calls != 0 {
		t.Fatalf("evaluation called RAG pipeline %d times for missing knowledge base", pipeline.calls)
	}
}

func TestRunEvaluationExposesSplitWeightsHoldoutAndMetricMigration(t *testing.T) {
	ctx := context.Background()
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()
	pipeline := &recordingPipeline{answer: "holdout answer"}
	app.RAG.Pipeline = pipeline

	token := loginToken(t, h)
	ds, err := app.Datasets.Create(ctx, "tenant_default", "weighted-holdout-http", "golden")
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []dataset.Item{
		{Query: "eval question", GroundTruth: "holdout answer", Split: dataset.DatasetSplitEval, Weight: 3},
		{Query: "holdout question", GroundTruth: "holdout answer", Split: dataset.DatasetSplitHoldout, Weight: 2},
	} {
		if _, err := app.Datasets.AddItem(ctx, "tenant_default", ds.ID, item); err != nil {
			t.Fatal(err)
		}
	}

	body := `{
		"dataset_id":"` + ds.ID + `",
		"knowledge_base_id":"kb_default",
		"profile":"realtime",
		"top_k":3,
		"split":"holdout",
		"scoped_shadow_item_id":"opt_scoped",
		"holdout_gate":{
			"enabled":true,
			"min_sample_count":1,
			"min_weighted_sample_count":2,
			"quality_metric":"deterministic_answer_match",
			"min_quality":1
		}
	}`
	resp := performJSON(h, "POST", "/v1/evaluations", body, token)
	if resp.Code != 202 {
		t.Fatalf("evaluation status = %d body=%s", resp.Code, resp.Body)
	}
	var result evalpkg.RunResult
	if err := json.Unmarshal([]byte(resp.Body), &result); err != nil {
		t.Fatal(err)
	}
	if result.Split != dataset.DatasetSplitHoldout || result.Total != 1 || result.UnweightedSampleCount != 1 || result.WeightedSampleCount != 2 {
		t.Fatalf("weighted split response = %#v", result)
	}
	if result.SplitSummary["eval"].WeightedSampleCount != 3 || result.SplitSummary["holdout"].WeightedSampleCount != 2 {
		t.Fatalf("split summary = %#v", result.SplitSummary)
	}
	if !result.HoldoutGate.Enabled || !result.HoldoutGate.Passed || result.HoldoutGate.QualityMetric != evalpkg.PrimaryMetricDeterministicAnswerMatch {
		t.Fatalf("holdout gate = %#v", result.HoldoutGate)
	}
	if result.Metrics[evalpkg.PrimaryMetricDeterministicAnswerMatch] != 1 {
		t.Fatalf("metrics missing deterministic answer match: %#v", result.Metrics)
	}
	if _, ok := result.Metrics[evalpkg.PrimaryMetricPairwiseAccuracy]; ok {
		t.Fatalf("rule-only HTTP evaluation wrote pairwise accuracy: %#v", result.Metrics)
	}
	if len(pipeline.requests) != 1 || pipeline.requests[0].ScopedShadowItemID != "opt_scoped" || pipeline.requests[0].TopK != 3 {
		t.Fatalf("pipeline requests = %#v", pipeline.requests)
	}
}

func TestGetEvaluationIncludesItemsJudgeAndPairwiseDetails(t *testing.T) {
	ctx := context.Background()
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()
	app.RAG.Pipeline = &countingPipeline{resp: rag.QueryResponse{
		Answer:      "qdrant answer",
		CacheStatus: "miss",
		LatencyMS:   5,
		RetrievedChunks: []kb.SearchResult{{
			Chunk: kb.Chunk{ID: "chk_1", DocumentID: "doc_1", Content: "qdrant evidence"},
		}},
		Citations: []rag.Citation{{ChunkID: "chk_1", DocumentID: "doc_1"}},
	}}
	app.Eval.Judge = httpFakeJudge{}
	app.Eval.QAG = httpFakeQAGJudge{}

	token := loginToken(t, h)
	ds, err := app.Datasets.Create(ctx, "tenant_default", "judge-http", "golden")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Datasets.AddItem(ctx, "tenant_default", ds.ID, dataset.Item{
		Query:            "qdrant",
		GroundTruth:      "qdrant",
		RelevantDocIDs:   []string{"doc_1"},
		ExpectedEvidence: []string{"qdrant evidence"},
	}); err != nil {
		t.Fatal(err)
	}

	resp := performJSON(h, "POST", "/v1/evaluations", `{"dataset_id":"`+ds.ID+`","knowledge_base_id":"kb_default","profile":"realtime","judge":{"provider":"test","model":"judge"},"qag":{"provider":"test","model":"qag"}}`, token)
	if resp.Code != 202 {
		t.Fatalf("evaluation status = %d body=%s", resp.Code, resp.Body)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" {
		t.Fatalf("evaluation response missing id: %s", resp.Body)
	}

	resp = performJSON(h, "GET", "/v1/evaluations/"+created.ID+"?include_items=true&include_judge=true&include_pairwise=true", "", token)
	if resp.Code != 200 {
		t.Fatalf("detail status = %d body=%s", resp.Code, resp.Body)
	}
	var detail evalpkg.EvaluationDetail
	if err := json.Unmarshal([]byte(resp.Body), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Run.ID != created.ID || len(detail.Items) != 1 {
		t.Fatalf("detail run/items = %#v", detail)
	}
	if len(detail.JudgeRuns) != 2 || len(detail.JudgeResults) != 2 {
		t.Fatalf("judge detail = runs:%#v results:%#v", detail.JudgeRuns, detail.JudgeResults)
	}
	if detail.JudgeResults[0].RawResponse == "" || detail.JudgeResults[0].TokenUsage.TotalTokens == 0 {
		t.Fatalf("judge result missing raw/token: %#v", detail.JudgeResults)
	}
	if detail.Items[0].Metrics["qag_score"] != 1 {
		t.Fatalf("item metrics = %#v, want qag_score", detail.Items[0].Metrics)
	}
}

func TestGetEvaluationDefaultReturnsSummaryOnly(t *testing.T) {
	ctx := context.Background()
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()
	token := loginToken(t, h)
	ds, err := app.Datasets.Create(ctx, "tenant_default", "summary-only", "golden")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Datasets.AddItem(ctx, "tenant_default", ds.ID, dataset.Item{Query: "q", GroundTruth: "a"}); err != nil {
		t.Fatal(err)
	}
	app.RAG.Pipeline = &countingPipeline{resp: rag.QueryResponse{Answer: "a", CacheStatus: "miss"}}

	resp := performJSON(h, "POST", "/v1/evaluations", `{"dataset_id":"`+ds.ID+`","knowledge_base_id":"kb_default","profile":"realtime"}`, token)
	if resp.Code != 202 {
		t.Fatalf("evaluation status = %d body=%s", resp.Code, resp.Body)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &created); err != nil {
		t.Fatal(err)
	}
	resp = performJSON(h, "GET", "/v1/evaluations/"+created.ID, "", token)
	if resp.Code != 200 {
		t.Fatalf("summary status = %d body=%s", resp.Code, resp.Body)
	}
	if strings.Contains(resp.Body, `"items"`) || strings.Contains(resp.Body, `"judge_results"`) {
		t.Fatalf("default evaluation response leaked details: %s", resp.Body)
	}
	var summary evalpkg.RunResult
	if err := json.Unmarshal([]byte(resp.Body), &summary); err != nil {
		t.Fatal(err)
	}
	if _, ok := summary.Metrics[evalpkg.PrimaryMetricDeterministicAnswerMatch]; !ok {
		t.Fatalf("summary metrics = %#v, want deterministic answer match metric", summary.Metrics)
	}
	if _, ok := summary.Metrics[evalpkg.PrimaryMetricPairwiseAccuracy]; ok {
		t.Fatalf("rule-only summary wrote pairwise accuracy: %s", resp.Body)
	}
}

func TestOptimizationAsyncHTTPLifecycle(t *testing.T) {
	ctx := context.Background()
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()
	app.Optimizer.DisableAutoStart = true
	app.RAG.Pipeline = &countingPipeline{resp: rag.QueryResponse{Answer: "Qdrant", CacheStatus: "miss", LatencyMS: 7}}

	token := loginToken(t, h)
	ds, err := app.Datasets.Create(ctx, "tenant_default", "async-optimizer", "golden")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Datasets.AddItem(ctx, "tenant_default", ds.ID, dataset.Item{Query: "vector store?", GroundTruth: "Qdrant"}); err != nil {
		t.Fatal(err)
	}

	body := `{
		"dataset_id":"` + ds.ID + `",
		"knowledge_base_id":"kb_default",
		"profile":"realtime",
		"objective":{"maximize":"pairwise_accuracy"},
		"search_space":{"retrieval":{"dense_top_k":[1,2]}},
		"search":{"strategy":"grid","max_candidates":2},
		"budget":{"max_judge_calls":3},
		"selection_split":"eval",
		"holdout_split":"holdout",
		"holdout_gate":{
			"enabled":true,
			"min_sample_count":1,
			"min_weighted_sample_count":1,
			"quality_metric":"deterministic_answer_match",
			"min_quality":0.8
		}
	}`
	resp := performJSON(h, "POST", "/v1/optimizations", body, token)
	if resp.Code != 202 {
		t.Fatalf("submit status = %d body=%s", resp.Code, resp.Body)
	}
	var accepted struct {
		RunID     string `json:"run_id"`
		Status    string `json:"status"`
		PollURL   string `json:"poll_url"`
		CancelURL string `json:"cancel_url"`
		ResumeURL string `json:"resume_url"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &accepted); err != nil {
		t.Fatal(err)
	}
	if accepted.RunID == "" || accepted.Status != "queued" || accepted.PollURL == "" || accepted.CancelURL == "" || accepted.ResumeURL == "" {
		t.Fatalf("unexpected accepted response: %#v body=%s", accepted, resp.Body)
	}
	storedStatus, ok, err := app.Optimizer.Get(ctx, "tenant_default", accepted.RunID)
	if err != nil || !ok {
		t.Fatalf("optimizer Get() ok=%v err=%v", ok, err)
	}
	storedReq := storedStatus.Run.StoredSubmitRequest()
	if !storedReq.HoldoutGate.Enabled ||
		storedReq.HoldoutGate.MinSampleCount != 1 ||
		storedReq.HoldoutGate.MinWeightedSampleCount != 1 ||
		storedReq.HoldoutGate.QualityMetric != evalpkg.PrimaryMetricDeterministicAnswerMatch ||
		storedReq.HoldoutGate.MinQuality != 0.8 {
		t.Fatalf("stored holdout gate = %#v, want request config", storedReq.HoldoutGate)
	}

	resp = performJSON(h, "GET", accepted.PollURL, "", token)
	if resp.Code != 200 {
		t.Fatalf("get status = %d body=%s", resp.Code, resp.Body)
	}
	type optimizationHTTPStatus struct {
		Run struct {
			ID                    string `json:"id"`
			Status                string `json:"status"`
			SampledCandidateCount int    `json:"sampled_candidate_count"`
			Objective             struct {
				Maximize string `json:"maximize"`
			} `json:"objective"`
			SearchSpace struct {
				Retrieval struct {
					DenseTopK []int `json:"dense_top_k"`
				} `json:"retrieval"`
			} `json:"search_space"`
			Runner map[string]any `json:"runner"`
		} `json:"run"`
		Candidates []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
			Config struct {
				Retrieval struct {
					DenseTopK int `json:"dense_top_k"`
				} `json:"retrieval"`
			} `json:"config"`
		} `json:"candidates"`
	}
	assertOriginalCandidateSet := func(t *testing.T, status optimizationHTTPStatus, body string) {
		t.Helper()
		if status.Run.SampledCandidateCount != 2 {
			t.Fatalf("sampled_candidate_count = %d, want 2 body=%s", status.Run.SampledCandidateCount, body)
		}
		if got := status.Run.SearchSpace.Retrieval.DenseTopK; len(got) != 2 || got[0] != 1 || got[1] != 2 {
			t.Fatalf("run search_space dense_top_k = %#v, want [1 2] body=%s", got, body)
		}
		if len(status.Candidates) != 2 {
			t.Fatalf("candidate count = %d, want 2 body=%s", len(status.Candidates), body)
		}
		topKCounts := map[int]int{}
		for _, candidate := range status.Candidates {
			topKCounts[candidate.Config.Retrieval.DenseTopK]++
		}
		if topKCounts[1] != 1 || topKCounts[2] != 1 || len(topKCounts) != 2 {
			t.Fatalf("candidate dense_top_k set = %#v, want exactly [1 2] body=%s", topKCounts, body)
		}
	}
	var status optimizationHTTPStatus
	if err := json.Unmarshal([]byte(resp.Body), &status); err != nil {
		t.Fatal(err)
	}
	if status.Run.ID != accepted.RunID || status.Run.Status != "queued" || status.Run.SampledCandidateCount != 2 || len(status.Candidates) != 2 {
		t.Fatalf("unexpected optimization status: %#v body=%s", status, resp.Body)
	}
	assertOriginalCandidateSet(t, status, resp.Body)
	if status.Run.Objective.Maximize != "pairwise_accuracy" || status.Run.Runner["type"] != "internal_rag" {
		t.Fatalf("missing objective/runner metadata: %#v", status.Run)
	}

	resp = performJSON(h, "POST", accepted.CancelURL, `{"reason":"user requested"}`, token)
	if resp.Code != 202 || !strings.Contains(resp.Body, `"status":"canceling"`) {
		t.Fatalf("cancel status = %d body=%s", resp.Code, resp.Body)
	}
	resp = performJSONWithTrace(h, "POST", accepted.ResumeURL, `{"search_space":{"retrieval":{"dense_top_k":[1,3]}}}`, token, "trace_resume_mutates_search_space")
	assertErrorResponse(t, resp, 400, "invalid_request", "trace_resume_mutates_search_space")

	resp = performJSON(h, "GET", accepted.PollURL, "", token)
	if resp.Code != 200 {
		t.Fatalf("get status after rejected resume = %d body=%s", resp.Code, resp.Body)
	}
	if err := json.Unmarshal([]byte(resp.Body), &status); err != nil {
		t.Fatal(err)
	}
	assertOriginalCandidateSet(t, status, resp.Body)

	resp = performJSONWithTrace(h, "POST", accepted.ResumeURL, `{}`, token, "trace_resume_conflict")
	assertErrorResponse(t, resp, 409, "optimization_state_conflict", "trace_resume_conflict")

	storedStatus, ok, err = app.Optimizer.Get(ctx, "tenant_default", accepted.RunID)
	if err != nil || !ok {
		t.Fatalf("optimizer Get() before terminal resume ok=%v err=%v", ok, err)
	}
	storedStatus.Run.Status = optimizer.RunStatusCanceled
	if err := app.Optimizer.Repository.UpdateOptimizationRun(ctx, storedStatus.Run); err != nil {
		t.Fatalf("mark optimization canceled: %v", err)
	}

	resp = performJSON(h, "POST", accepted.ResumeURL, `{}`, token)
	if resp.Code != 202 || !strings.Contains(resp.Body, `"status":"queued"`) {
		t.Fatalf("resume status = %d body=%s", resp.Code, resp.Body)
	}
}

func TestOptimizationAcceptsLegacyProfilesTopKsShortcut(t *testing.T) {
	ctx := context.Background()
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()
	app.Optimizer.DisableAutoStart = true

	token := loginToken(t, h)
	ds, err := app.Datasets.Create(ctx, "tenant_default", "legacy-optimizer", "golden")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Datasets.AddItem(ctx, "tenant_default", ds.ID, dataset.Item{Query: "q", GroundTruth: "a"}); err != nil {
		t.Fatal(err)
	}

	resp := performJSON(h, "POST", "/v1/optimizations", `{"dataset_id":"`+ds.ID+`","knowledge_base_id":"kb_default","profiles":["high_precision"],"top_ks":[3,5]}`, token)
	if resp.Code != 202 {
		t.Fatalf("legacy submit status = %d body=%s", resp.Code, resp.Body)
	}
	var accepted struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &accepted); err != nil {
		t.Fatal(err)
	}
	resp = performJSON(h, "GET", "/v1/optimizations/"+accepted.RunID, "", token)
	if resp.Code != 200 {
		t.Fatalf("legacy status = %d body=%s", resp.Code, resp.Body)
	}
	if !strings.Contains(resp.Body, `"profile":"high_precision"`) || !strings.Contains(resp.Body, `"dense_top_k":3`) || !strings.Contains(resp.Body, `"dense_top_k":5`) {
		t.Fatalf("legacy shortcut did not map profile/top_ks into async run: %s", resp.Body)
	}
}

func TestOfflineKnowledgeManualRunHTTP(t *testing.T) {
	ctx := context.Background()
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()
	repo := installOfflineKnowledgeService(app, offlineknowledge.ServiceOptions{Now: fixedOfflineKnowledgeNow})

	token := loginToken(t, h)
	body := `{
		"kb_id":"kb_default",
		"window_start":"2026-07-07T00:00:00Z",
		"window_end":"2026-07-08T00:00:00Z",
		"config_hash":"config_http"
	}`
	resp := performJSON(h, "POST", "/v1/offline-knowledge/runs", body, token)
	if resp.Code != 202 {
		t.Fatalf("create offline run status = %d body=%s", resp.Code, resp.Body)
	}
	var created struct {
		Run offlineknowledge.OfflineKnowledgeRun `json:"run"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &created); err != nil {
		t.Fatal(err)
	}
	if created.Run.ID == "" || created.Run.TenantID != "tenant_default" || created.Run.KBID != "kb_default" || created.Run.Status != offlineknowledge.RunStatusPending {
		t.Fatalf("unexpected created run: %#v body=%s", created.Run, resp.Body)
	}

	if err := repo.UpsertQuestionCluster(ctx, offlineknowledge.QuestionCluster{
		ID:                 "cluster_http",
		TenantID:           "tenant_default",
		RunID:              created.Run.ID,
		KBID:               "kb_default",
		CanonicalQuestion:  "What is ORAG?",
		NormalizedQuestion: "what is orag?",
		QuestionHash:       "hash_http",
		SampleQuestions:    []string{"What is ORAG?"},
		TraceIDs:           []string{"trace_http"},
		CreatedAt:          fixedOfflineKnowledgeNow(),
	}); err != nil {
		t.Fatal(err)
	}

	resp = performJSON(h, "GET", "/v1/offline-knowledge/runs?status=pending&kb_id=kb_default", "", token)
	if resp.Code != 200 || !strings.Contains(resp.Body, created.Run.ID) {
		t.Fatalf("list offline runs status = %d body=%s", resp.Code, resp.Body)
	}
	resp = performJSON(h, "GET", "/v1/offline-knowledge/runs/"+created.Run.ID, "", token)
	if resp.Code != 200 || !strings.Contains(resp.Body, `"config_hash":"config_http"`) {
		t.Fatalf("get offline run status = %d body=%s", resp.Code, resp.Body)
	}
	resp = performJSON(h, "GET", "/v1/offline-knowledge/runs/"+created.Run.ID+"/questions", "", token)
	if resp.Code != 200 || !strings.Contains(resp.Body, `"id":"cluster_http"`) {
		t.Fatalf("list offline questions status = %d body=%s", resp.Code, resp.Body)
	}
}

func TestOfflineKnowledgeRunExecuteHTTPStatuses(t *testing.T) {
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()
	installOfflineKnowledgeService(app, httpExecutableOfflineKnowledgeOptions())

	token := loginToken(t, h)
	body := `{
		"kb_id":"kb_default",
		"window_start":"2026-07-07T00:00:00Z",
		"window_end":"2026-07-08T00:00:00Z",
		"config_hash":"config_execute"
	}`
	resp := performJSON(h, "POST", "/v1/offline-knowledge/runs", body, token)
	if resp.Code != 202 {
		t.Fatalf("create run status = %d body=%s", resp.Code, resp.Body)
	}
	var created struct {
		Run offlineknowledge.OfflineKnowledgeRun `json:"run"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &created); err != nil {
		t.Fatal(err)
	}

	resp = performJSON(h, "POST", "/v1/offline-knowledge/runs/"+created.Run.ID+"/execute", `{}`, token)
	if resp.Code != 202 || !strings.Contains(resp.Body, `"status":"completed"`) {
		t.Fatalf("execute run status = %d body=%s", resp.Code, resp.Body)
	}
	resp = performJSON(h, "POST", "/v1/offline-knowledge/runs/"+created.Run.ID+"/execute", `{}`, token)
	if resp.Code != 409 || !strings.Contains(resp.Body, `"code":"offline_knowledge_run_execution_conflict"`) {
		t.Fatalf("second execute status = %d body=%s", resp.Code, resp.Body)
	}
}

func TestOfflineKnowledgeSchedulerTriggerDisabledReturns503(t *testing.T) {
	h, closeApp := newTestHertz(t)
	defer closeApp()

	token := loginToken(t, h)
	resp := performJSON(h, "POST", "/v1/offline-knowledge/scheduler:trigger", `{}`, token)
	if resp.Code != 503 || !strings.Contains(resp.Body, `"code":"offline_knowledge_scheduler_disabled"`) {
		t.Fatalf("scheduler trigger status = %d body=%s", resp.Code, resp.Body)
	}
}

func TestOfflineKnowledgeDisabledDependenciesReturn503(t *testing.T) {
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()
	repo := installOfflineKnowledgeService(app, offlineknowledge.ServiceOptions{
		RegressionRunner: offlineknowledge.DisabledRegressionRunner{},
		Now:              fixedOfflineKnowledgeNow,
	})
	if err := repo.CreateOptimizationItem(context.Background(), offlineKnowledgeItem("item_regression_disabled", "tenant_default", "kb_1", offlineknowledge.ItemStatusShadowEnabled, offlineknowledge.ItemTypeAnswer)); err != nil {
		t.Fatal(err)
	}

	token := loginToken(t, h)
	resp := performJSON(h, "POST", "/v1/optimization-items/item_regression_disabled/run-regression", `{}`, token)
	if resp.Code != 503 || !strings.Contains(resp.Body, `"code":"offline_knowledge_dependency_unavailable"`) {
		t.Fatalf("disabled regression status = %d body=%s", resp.Code, resp.Body)
	}

	resp = performJSON(h, "POST", "/v1/offline-knowledge/runs/run_missing/execute", `{}`, token)
	if resp.Code != 503 || !strings.Contains(resp.Body, `"code":"offline_knowledge_dependency_unavailable"`) {
		t.Fatalf("execute missing dependencies status = %d body=%s", resp.Code, resp.Body)
	}
}

func TestOfflineKnowledgeOptimizationItemListFiltersAndTenantIsolation(t *testing.T) {
	ctx := context.Background()
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()
	repo := installOfflineKnowledgeService(app, offlineknowledge.ServiceOptions{Now: fixedOfflineKnowledgeNow})
	for _, item := range []offlineknowledge.OptimizationItem{
		offlineKnowledgeItem("item_match", "tenant_default", "kb_1", offlineknowledge.ItemStatusVerified, offlineknowledge.ItemTypeAnswer),
		offlineKnowledgeItem("item_wrong_status", "tenant_default", "kb_1", offlineknowledge.ItemStatusRejected, offlineknowledge.ItemTypeAnswer),
		offlineKnowledgeItem("item_wrong_type", "tenant_default", "kb_1", offlineknowledge.ItemStatusVerified, offlineknowledge.ItemTypeQueryRewrite),
		offlineKnowledgeItem("item_other_tenant", "tenant_other", "kb_1", offlineknowledge.ItemStatusVerified, offlineknowledge.ItemTypeAnswer),
	} {
		if err := repo.CreateOptimizationItem(ctx, item); err != nil {
			t.Fatal(err)
		}
	}

	token := loginToken(t, h)
	resp := performJSON(h, "GET", "/v1/optimization-items?status=verified&kb_id=kb_1&item_type=answer_item", "", token)
	if resp.Code != 200 {
		t.Fatalf("list optimization items status = %d body=%s", resp.Code, resp.Body)
	}
	var listed struct {
		Items []offlineknowledge.OptimizationItem `json:"items"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Items) != 1 || listed.Items[0].ID != "item_match" {
		t.Fatalf("filtered optimization items = %#v body=%s", listed.Items, resp.Body)
	}

	otherToken, err := app.Auth.IssueToken("tenant_other", "user_other")
	if err != nil {
		t.Fatal(err)
	}
	resp = performJSON(h, "GET", "/v1/optimization-items/item_match", "", otherToken)
	if resp.Code != 404 || !strings.Contains(resp.Body, `"code":"optimization_item_not_found"`) {
		t.Fatalf("cross-tenant get item status = %d body=%s", resp.Code, resp.Body)
	}
}

func TestOfflineKnowledgeOptimizationItemDisable(t *testing.T) {
	ctx := context.Background()
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()
	repo := installOfflineKnowledgeService(app, offlineknowledge.ServiceOptions{Now: fixedOfflineKnowledgeNow})
	if err := repo.CreateOptimizationItem(ctx, offlineKnowledgeItem("item_disable", "tenant_default", "kb_1", offlineknowledge.ItemStatusPublished, offlineknowledge.ItemTypeAnswer)); err != nil {
		t.Fatal(err)
	}

	token := loginToken(t, h)
	resp := performJSON(h, "POST", "/v1/optimization-items/item_disable/disable", `{}`, token)
	if resp.Code != 202 || !strings.Contains(resp.Body, `"status":"deprecated"`) {
		t.Fatalf("disable item status = %d body=%s", resp.Code, resp.Body)
	}
	stored, found, err := repo.GetOptimizationItem(ctx, "tenant_default", "item_disable")
	if err != nil || !found {
		t.Fatalf("lookup disabled item found=%v err=%v", found, err)
	}
	if stored.Status != offlineknowledge.ItemStatusDeprecated {
		t.Fatalf("stored disabled status = %q, want deprecated", stored.Status)
	}
}

func TestOfflineKnowledgeOptimizationItemVerifyNeedsReview(t *testing.T) {
	ctx := context.Background()
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()
	repo := installOfflineKnowledgeService(app, offlineknowledge.ServiceOptions{Now: fixedOfflineKnowledgeNow})
	if err := repo.CreateOptimizationItem(ctx, offlineKnowledgeItem("item_review_verify", "tenant_default", "kb_1", offlineknowledge.ItemStatusNeedsReview, offlineknowledge.ItemTypeAnswer)); err != nil {
		t.Fatal(err)
	}

	token := loginToken(t, h)
	resp := performJSON(h, "POST", "/v1/optimization-items/item_review_verify/verify", `{}`, token)
	if resp.Code != 202 || !strings.Contains(resp.Body, `"status":"verified"`) {
		t.Fatalf("verify needs_review item status = %d body=%s", resp.Code, resp.Body)
	}
	stored, found, err := repo.GetOptimizationItem(ctx, "tenant_default", "item_review_verify")
	if err != nil || !found {
		t.Fatalf("lookup verified item found=%v err=%v", found, err)
	}
	if stored.Status != offlineknowledge.ItemStatusVerified {
		t.Fatalf("stored verified status = %q, want verified", stored.Status)
	}
}

func TestOfflineKnowledgeOptimizationItemRejectsIllegalPublish(t *testing.T) {
	ctx := context.Background()
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()
	repo := installOfflineKnowledgeService(app, offlineknowledge.ServiceOptions{Now: fixedOfflineKnowledgeNow})
	if err := repo.CreateOptimizationItem(ctx, offlineKnowledgeItem("item_illegal_publish", "tenant_default", "kb_1", offlineknowledge.ItemStatusVerified, offlineknowledge.ItemTypeAnswer)); err != nil {
		t.Fatal(err)
	}

	token := loginToken(t, h)
	resp := performJSON(h, "POST", "/v1/optimization-items/item_illegal_publish/publish", `{}`, token)
	if resp.Code != 409 || !strings.Contains(resp.Body, `"code":"invalid_optimization_item_transition"`) {
		t.Fatalf("illegal publish status = %d body=%s", resp.Code, resp.Body)
	}
	stored, found, err := repo.GetOptimizationItem(ctx, "tenant_default", "item_illegal_publish")
	if err != nil || !found {
		t.Fatalf("lookup illegal publish item found=%v err=%v", found, err)
	}
	if stored.Status != offlineknowledge.ItemStatusVerified {
		t.Fatalf("illegal publish mutated status = %q, want verified", stored.Status)
	}
}

func TestOfflineKnowledgeOptimizationItemRevalidate(t *testing.T) {
	ctx := context.Background()
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()
	validator := &httpOfflineKnowledgeValidator{}
	repo := installOfflineKnowledgeService(app, offlineknowledge.ServiceOptions{Validator: validator, Now: fixedOfflineKnowledgeNow})
	for _, item := range []offlineknowledge.OptimizationItem{
		offlineKnowledgeStaleItem("item_single_revalidate", "sha256:single"),
		offlineKnowledgeStaleItem("item_bulk_revalidate", "sha256:bulk"),
		offlineKnowledgeStaleItem("item_bulk_other_hash", "sha256:other"),
	} {
		if err := repo.CreateOptimizationItem(ctx, item); err != nil {
			t.Fatal(err)
		}
	}

	token := loginToken(t, h)
	resp := performJSON(h, "POST", "/v1/optimization-items/item_single_revalidate/revalidate", `{}`, token)
	if resp.Code != 202 || !strings.Contains(resp.Body, `"new_status":"verified"`) {
		t.Fatalf("single revalidate status = %d body=%s", resp.Code, resp.Body)
	}
	resp = performJSON(h, "POST", "/v1/optimization-items/revalidate", `{"kb_id":"kb_1","source_content_hash":"sha256:bulk"}`, token)
	if resp.Code != 202 || !strings.Contains(resp.Body, `"matched":1`) || !strings.Contains(resp.Body, `"updated":1`) {
		t.Fatalf("bulk revalidate status = %d body=%s", resp.Code, resp.Body)
	}
	if validator.calls != 2 {
		t.Fatalf("validator calls = %d, want 2", validator.calls)
	}
	bulk, _, _ := repo.GetOptimizationItem(ctx, "tenant_default", "item_bulk_revalidate")
	other, _, _ := repo.GetOptimizationItem(ctx, "tenant_default", "item_bulk_other_hash")
	if bulk.Status != offlineknowledge.ItemStatusVerified || other.Status != offlineknowledge.ItemStatusStale {
		t.Fatalf("bulk statuses = %q/%q, want verified/stale", bulk.Status, other.Status)
	}
}

func TestHealthReadyAndMetrics(t *testing.T) {
	h, closeApp := newTestHertz(t)
	defer closeApp()

	resp := performJSON(h, "GET", "/healthz", "", "")
	if resp.Code != 200 {
		t.Fatalf("health status = %d body=%s", resp.Code, resp.Body)
	}
	resp = performJSON(h, "GET", "/readyz", "", "")
	if resp.Code != 200 {
		t.Fatalf("ready status = %d body=%s", resp.Code, resp.Body)
	}
	if !strings.Contains(resp.Body, `"storage":{"status":"ready"}`) {
		t.Fatalf("ready body = %s", resp.Body)
	}
	resp = performJSON(h, "GET", "/metrics", "", "")
	if resp.Code != 200 {
		t.Fatalf("metrics status = %d body=%s", resp.Code, resp.Body)
	}
	if !strings.Contains(resp.Body, "orag_http_requests_total") || !strings.Contains(resp.Body, "orag_rag_queries_total") {
		t.Fatalf("metrics body = %s", resp.Body)
	}
}

func TestIngestionJobLookupReturnsResultSummary(t *testing.T) {
	h, closeApp := newTestHertz(t)
	defer closeApp()

	token := loginToken(t, h)
	resp := performJSON(h, "POST", "/v1/knowledge-bases/kb_default/documents:import", `{"name":"demo.md","source_uri":"example://demo","content":"ORAG supports Qdrant and PostgreSQL retrieval."}`, token)
	if resp.Code != 202 {
		t.Fatalf("import status = %d body=%s", resp.Code, resp.Body)
	}
	var imported struct {
		Document struct {
			ID string `json:"id"`
		} `json:"document"`
		Job struct {
			ID         string `json:"id"`
			Status     string `json:"status"`
			DocumentID string `json:"document_id"`
			ChunkCount int    `json:"chunk_count"`
		} `json:"job"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &imported); err != nil {
		t.Fatal(err)
	}
	if imported.Job.ID == "" || imported.Job.DocumentID != imported.Document.ID || imported.Job.ChunkCount == 0 {
		t.Fatalf("unexpected import response: %#v", imported)
	}

	resp = performJSON(h, "GET", "/v1/ingestion-jobs/"+imported.Job.ID, "", token)
	if resp.Code != 200 {
		t.Fatalf("job status = %d body=%s", resp.Code, resp.Body)
	}
	var job struct {
		ID         string `json:"id"`
		Status     string `json:"status"`
		DocumentID string `json:"document_id"`
		ChunkCount int    `json:"chunk_count"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &job); err != nil {
		t.Fatal(err)
	}
	if job.ID != imported.Job.ID || job.Status != "succeeded" || job.DocumentID != imported.Document.ID || job.ChunkCount == 0 {
		t.Fatalf("unexpected job response: %#v", job)
	}
}

func TestDeleteKnowledgeBaseRemovesItFromGetListAndMemoryChunks(t *testing.T) {
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()

	token := loginToken(t, h)
	resp := performJSON(h, "POST", "/v1/knowledge-bases", `{"name":"delete me","description":"temporary"}`, token)
	if resp.Code != 201 {
		t.Fatalf("create status = %d body=%s", resp.Code, resp.Body)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" {
		t.Fatalf("create response missing id: %s", resp.Body)
	}

	marker := "deleted_kb_http_contract_marker"
	resp = performJSON(h, "POST", "/v1/knowledge-bases/"+created.ID+"/documents:import", `{"name":"delete.md","source_uri":"example://delete","content":"This document should be removed with marker `+marker+`."}`, token)
	if resp.Code != 202 {
		t.Fatalf("import status = %d body=%s", resp.Code, resp.Body)
	}
	chunkSource, ok := app.KBStore.(interface {
		Chunks(tenantID, kbID string) []kb.Chunk
	})
	if !ok {
		t.Fatal("test KB store does not expose chunks")
	}
	if chunks := chunkSource.Chunks("tenant_default", created.ID); len(chunks) == 0 {
		t.Fatal("expected imported chunks before delete")
	}

	resp = performJSON(h, "DELETE", "/v1/knowledge-bases/"+created.ID, "", token)
	if resp.Code != 204 {
		t.Fatalf("delete status = %d body=%s", resp.Code, resp.Body)
	}
	resp = performJSON(h, "GET", "/v1/knowledge-bases/"+created.ID, "", token)
	if resp.Code != 404 {
		t.Fatalf("get after delete status = %d body=%s", resp.Code, resp.Body)
	}
	resp = performJSON(h, "GET", "/v1/knowledge-bases", "", token)
	if resp.Code != 200 {
		t.Fatalf("list status = %d body=%s", resp.Code, resp.Body)
	}
	var listed struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &listed); err != nil {
		t.Fatal(err)
	}
	for _, item := range listed.Items {
		if item.ID == created.ID {
			t.Fatalf("deleted knowledge base still listed: %s", resp.Body)
		}
	}
	if chunks := chunkSource.Chunks("tenant_default", created.ID); len(chunks) != 0 {
		t.Fatalf("deleted knowledge base still has chunks: %#v", chunks)
	}

	stalePipeline := &countingPipeline{resp: rag.QueryResponse{
		Answer:      "stale deleted content " + marker,
		CacheStatus: "miss",
		Profile:     rag.ProfileRealtime,
		RetrievedChunks: []kb.SearchResult{{
			Chunk: kb.Chunk{
				TenantID:        "tenant_default",
				KnowledgeBaseID: created.ID,
				Content:         "stale deleted content " + marker,
			},
		}},
	}}
	app.RAG.Pipeline = stalePipeline
	resp = performJSONWithTrace(h, "POST", "/v1/query", `{"knowledge_base_id":"`+created.ID+`","query":"`+marker+`"}`, token, "trace_deleted_kb_query")
	assertErrorResponse(t, resp, 404, "knowledge_base_not_found", "trace_deleted_kb_query")
	if stalePipeline.calls != 0 {
		t.Fatalf("query pipeline called %d times for deleted knowledge base", stalePipeline.calls)
	}

	resp = performJSON(h, "DELETE", "/v1/knowledge-bases/"+created.ID, "", token)
	if resp.Code != 404 {
		t.Fatalf("delete missing status = %d body=%s", resp.Code, resp.Body)
	}
}

func TestImportDocumentRejectsMissingContent(t *testing.T) {
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()

	token := loginToken(t, h)
	jobs := &countingJobStore{delegate: ingest.NewMemoryJobStore()}
	app.Ingest.Jobs = jobs

	for _, tt := range []struct {
		name    string
		body    string
		traceID string
	}{
		{
			name:    "missing content",
			body:    `{"name":"missing.md","source_uri":"test://missing-content"}`,
			traceID: "trace_import_missing_content",
		},
		{
			name:    "blank content",
			body:    `{"name":"blank.md","source_uri":"test://blank-content","content":"  "}`,
			traceID: "trace_import_blank_content",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			resp := performJSONWithTrace(h, "POST", "/v1/knowledge-bases/kb_default/documents:import", tt.body, token, tt.traceID)
			assertErrorResponse(t, resp, 400, "invalid_request", tt.traceID)
			assertNoChunks(t, app, "kb_default")
		})
	}
	if jobs.createCalls != 0 {
		t.Fatalf("import created %d ingestion jobs for invalid content", jobs.createCalls)
	}
}

func TestDocumentIngestionRequiresExistingKnowledgeBase(t *testing.T) {
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()

	token := loginToken(t, h)
	jobs := &countingJobStore{delegate: ingest.NewMemoryJobStore()}
	app.Ingest.Jobs = jobs
	missingKB := "kb_missing"

	resp := performJSON(h, "POST", "/v1/knowledge-bases/"+missingKB+"/documents:import", `{"name":"missing.md","source_uri":"test://missing","content":"orphan chunks must not be created"}`, token)
	assertMissingKnowledgeBaseResponse(t, resp)
	if jobs.createCalls != 0 {
		t.Fatalf("import created %d ingestion jobs for missing knowledge base", jobs.createCalls)
	}
	assertNoChunks(t, app, missingKB)

	resp = performMultipartUpload(t, h, "/v1/knowledge-bases/"+missingKB+"/documents", "missing.md", "orphan chunks must not be created", token)
	assertMissingKnowledgeBaseResponse(t, resp)
	if jobs.createCalls != 0 {
		t.Fatalf("upload created %d ingestion jobs for missing knowledge base", jobs.createCalls)
	}
	assertNoChunks(t, app, missingKB)
}

func TestImportDocumentRequiresExistingKnowledgeBase(t *testing.T) {
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()

	token := loginToken(t, h)
	jobs := &countingJobStore{delegate: ingest.NewMemoryJobStore()}
	app.Ingest.Jobs = jobs
	missingKB := "kb_missing_import"

	resp := performJSON(h, "POST", "/v1/knowledge-bases/"+missingKB+"/documents:import", `{"name":"missing.md","source_uri":"test://missing","content":"valid json must not be ingested"}`, token)
	assertMissingKnowledgeBaseResponse(t, resp)
	if strings.Contains(resp.Body, `"code":"ingest_failed"`) {
		t.Fatalf("missing knowledge base returned ingest_failed: %s", resp.Body)
	}
	if jobs.createCalls != 0 {
		t.Fatalf("import created %d ingestion jobs for missing knowledge base", jobs.createCalls)
	}
	assertNoChunks(t, app, missingKB)
}

func TestUploadDocumentRequiresExistingKnowledgeBase(t *testing.T) {
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()

	token := loginToken(t, h)
	jobs := &countingJobStore{delegate: ingest.NewMemoryJobStore()}
	app.Ingest.Jobs = jobs
	missingKB := "kb_missing_upload"

	resp := performMultipartUpload(t, h, "/v1/knowledge-bases/"+missingKB+"/documents", "missing.md", "orphan chunks must not be created", token)
	assertMissingKnowledgeBaseResponse(t, resp)
	if strings.Contains(resp.Body, `"code":"ingest_failed"`) {
		t.Fatalf("missing knowledge base returned ingest_failed: %s", resp.Body)
	}
	if jobs.createCalls != 0 {
		t.Fatalf("upload created %d ingestion jobs for missing knowledge base", jobs.createCalls)
	}
	assertNoChunks(t, app, missingKB)
}

func TestImportDocumentKnowledgeBaseLookupPrecedesInvalidJSON(t *testing.T) {
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()

	token := loginToken(t, h)
	jobs := &countingJobStore{delegate: ingest.NewMemoryJobStore()}
	app.Ingest.Jobs = jobs
	missingKB := "kb_missing_before_json_parse"

	resp := performJSONWithTrace(h, "POST", "/v1/knowledge-bases/"+missingKB+"/documents:import", `{`, token, "trace_import_invalid_json")
	assertErrorResponse(t, resp, 404, "knowledge_base_not_found", "trace_import_invalid_json")
	if jobs.createCalls != 0 {
		t.Fatalf("import created %d ingestion jobs for invalid json", jobs.createCalls)
	}
	assertNoChunks(t, app, missingKB)
}

func TestUploadDocumentKnowledgeBaseLookupPrecedesMissingFile(t *testing.T) {
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()

	token := loginToken(t, h)
	jobs := &countingJobStore{delegate: ingest.NewMemoryJobStore()}
	app.Ingest.Jobs = jobs
	missingKB := "kb_missing_before_multipart_parse"

	resp := performJSONWithTrace(h, "POST", "/v1/knowledge-bases/"+missingKB+"/documents", "", token, "trace_upload_missing_file")
	assertErrorResponse(t, resp, 404, "knowledge_base_not_found", "trace_upload_missing_file")
	if jobs.createCalls != 0 {
		t.Fatalf("upload created %d ingestion jobs for missing file", jobs.createCalls)
	}
	assertNoChunks(t, app, missingKB)
}

func TestDocumentIngestionMapsServiceMissingKnowledgeBaseTo404(t *testing.T) {
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()

	token := loginToken(t, h)
	jobs := &countingJobStore{delegate: ingest.NewMemoryJobStore()}
	app.Ingest.Jobs = jobs
	app.Ingest.KnowledgeBases = kb.NewMemoryStore()

	resp := performJSON(h, "POST", "/v1/knowledge-bases/kb_default/documents:import", `{"name":"missing.md","source_uri":"test://missing","content":"service guard should map to 404"}`, token)
	assertMissingKnowledgeBaseResponse(t, resp)
	if jobs.createCalls != 0 {
		t.Fatalf("import created %d ingestion jobs for service-level missing knowledge base", jobs.createCalls)
	}
	assertNoChunks(t, app, "kb_default")

	resp = performMultipartUpload(t, h, "/v1/knowledge-bases/kb_default/documents", "missing.md", "service guard should map to 404", token)
	assertMissingKnowledgeBaseResponse(t, resp)
	if jobs.createCalls != 0 {
		t.Fatalf("upload created %d ingestion jobs for service-level missing knowledge base", jobs.createCalls)
	}
	assertNoChunks(t, app, "kb_default")
}

func TestResumableUploadCanContinueFromLastOffsetAndComplete(t *testing.T) {
	h, closeApp := newTestHertz(t)
	defer closeApp()

	token := loginToken(t, h)
	content := "ORAG resumable upload test content with enough searchable words."
	createBody := `{"name":"resume.md","source_uri":"test://resume.md","total_bytes":` + strconv.Itoa(len(content)) + `}`
	resp := performJSON(h, "POST", "/v1/knowledge-bases/kb_default/uploads", createBody, token)
	if resp.Code != 201 {
		t.Fatalf("create upload status = %d body=%s", resp.Code, resp.Body)
	}
	var created struct {
		ID            string `json:"id"`
		UploadURL     string `json:"upload_url"`
		CompleteURL   string `json:"complete_url"`
		ReceivedBytes int64  `json:"received_bytes"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.UploadURL == "" || created.CompleteURL == "" || created.ReceivedBytes != 0 {
		t.Fatalf("unexpected upload create response: %#v body=%s", created, resp.Body)
	}

	first := content[:12]
	resp = performRaw(h, "PUT", created.UploadURL, first, token, ut.Header{Key: "Upload-Offset", Value: "0"})
	if resp.Code != 200 || !strings.Contains(resp.Body, `"received_bytes":12`) {
		t.Fatalf("first chunk status = %d body=%s", resp.Code, resp.Body)
	}
	resp = performJSON(h, "GET", created.UploadURL, "", token)
	if resp.Code != 200 || !strings.Contains(resp.Body, `"received_bytes":12`) {
		t.Fatalf("resume status lookup = %d body=%s", resp.Code, resp.Body)
	}
	resp = performRaw(h, "PUT", created.UploadURL, content[12:], token, ut.Header{Key: "Upload-Offset", Value: "12"})
	if resp.Code != 200 || !strings.Contains(resp.Body, `"received_bytes":`+strconv.Itoa(len(content))) {
		t.Fatalf("second chunk status = %d body=%s", resp.Code, resp.Body)
	}

	resp = performJSON(h, "POST", created.CompleteURL, `{}`, token)
	if resp.Code != 202 {
		t.Fatalf("complete status = %d body=%s", resp.Code, resp.Body)
	}
	if !strings.Contains(resp.Body, `"status":"completed"`) || !strings.Contains(resp.Body, `"document"`) || !strings.Contains(resp.Body, `"job"`) {
		t.Fatalf("complete response missing upload/document/job: %s", resp.Body)
	}
}

func TestResumableUploadRejectsWrongOffsetWithCurrentOffset(t *testing.T) {
	h, closeApp := newTestHertz(t)
	defer closeApp()

	token := loginToken(t, h)
	resp := performJSON(h, "POST", "/v1/knowledge-bases/kb_default/uploads", `{"name":"offset.md","total_bytes":6}`, token)
	if resp.Code != 201 {
		t.Fatalf("create upload status = %d body=%s", resp.Code, resp.Body)
	}
	var created struct {
		UploadURL string `json:"upload_url"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &created); err != nil {
		t.Fatal(err)
	}
	resp = performRaw(h, "PUT", created.UploadURL, "abc", token, ut.Header{Key: "Upload-Offset", Value: "0"})
	if resp.Code != 200 {
		t.Fatalf("first chunk status = %d body=%s", resp.Code, resp.Body)
	}

	resp = performRaw(h, "PUT", created.UploadURL, "def", token, ut.Header{Key: "Upload-Offset", Value: "0"})
	if resp.Code != 409 {
		t.Fatalf("wrong offset status = %d body=%s", resp.Code, resp.Body)
	}
	if !strings.Contains(resp.Body, `"code":"upload_offset_mismatch"`) || !strings.Contains(resp.Body, `"received_bytes":3`) {
		t.Fatalf("wrong offset response missing current offset: %s", resp.Body)
	}
}

func TestResumableUploadCancelRemovesSession(t *testing.T) {
	h, closeApp := newTestHertz(t)
	defer closeApp()

	token := loginToken(t, h)
	resp := performJSON(h, "POST", "/v1/knowledge-bases/kb_default/uploads", `{"name":"cancel.md","total_bytes":6}`, token)
	if resp.Code != 201 {
		t.Fatalf("create upload status = %d body=%s", resp.Code, resp.Body)
	}
	var created struct {
		UploadURL string `json:"upload_url"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &created); err != nil {
		t.Fatal(err)
	}

	resp = performJSON(h, "DELETE", created.UploadURL, "", token)
	if resp.Code != 204 {
		t.Fatalf("cancel status = %d body=%s", resp.Code, resp.Body)
	}
	resp = performJSON(h, "GET", created.UploadURL, "", token)
	if resp.Code != 404 || !strings.Contains(resp.Body, `"code":"upload_not_found"`) {
		t.Fatalf("lookup canceled status = %d body=%s", resp.Code, resp.Body)
	}
}

func TestDatasetItemAndEvaluationRequireTenantOwnership(t *testing.T) {
	ctx := context.Background()
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()

	ds, err := app.Datasets.Create(ctx, "tenant_default", "golden", "golden")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Datasets.AddItem(ctx, "tenant_default", ds.ID, dataset.Item{
		Query:       "qdrant vector",
		GroundTruth: "qdrant",
	}); err != nil {
		t.Fatal(err)
	}
	otherToken, err := app.Auth.IssueToken("tenant_other", "user_other")
	if err != nil {
		t.Fatal(err)
	}

	resp := performJSON(h, "POST", "/v1/datasets/"+ds.ID+"/items", `{"query":"cross tenant","ground_truth":"blocked"}`, otherToken)
	if resp.Code != 404 || !strings.Contains(resp.Body, `"code":"dataset_not_found"`) {
		t.Fatalf("cross-tenant dataset item status = %d body=%s", resp.Code, resp.Body)
	}
	items, err := app.Datasets.Items(ctx, "tenant_default", ds.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("dataset items = %d, want original item only", len(items))
	}

	resp = performJSON(h, "POST", "/v1/evaluations", `{"dataset_id":"`+ds.ID+`","knowledge_base_id":"kb_default","profile":"realtime"}`, otherToken)
	if resp.Code != 404 || !strings.Contains(resp.Body, `"code":"dataset_not_found"`) {
		t.Fatalf("cross-tenant evaluation status = %d body=%s", resp.Code, resp.Body)
	}
}

type testResponse struct {
	Code          int
	Body          string
	ContentType   string
	TraceIDHeader string
	Traceparent   string
}

func newTestHertz(t *testing.T) (*route.Engine, func()) {
	h, _, closeApp := newTestHertzWithApp(t)
	return h, closeApp
}

func newTestHertzWithApp(t *testing.T) (*route.Engine, *core.App, func()) {
	return newTestHertzWithLoggerAndApp(t, logger.New(false))
}

func newTestHertzWithLogger(t *testing.T, logg *slog.Logger) (*route.Engine, func()) {
	h, _, closeApp := newTestHertzWithLoggerAndApp(t, logg)
	return h, closeApp
}

func newTestHertzWithLoggerAndApp(t *testing.T, logg *slog.Logger) (*route.Engine, *core.App, func()) {
	t.Helper()
	t.Setenv("STORAGE_BACKEND", "memory")
	t.Setenv("ADMIN_DEFAULT_USERNAME", "admin")
	t.Setenv("ADMIN_DEFAULT_PASSWORD", "secret")
	t.Setenv("PORT", "0")
	t.Setenv("ALLOW_DETERMINISTIC_MOCK", "true")
	t.Setenv("LLM_CHAT_PROVIDER", "mock")
	t.Setenv("LLM_EMBEDDING_PROVIDER", "mock")
	t.Setenv("LLM_RERANK_PROVIDER", "mock")
	t.Setenv("LLM_MULTIMODAL_PROVIDER", "mock")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	app, err := core.New(context.Background(), cfg, logg)
	if err != nil {
		t.Fatal(err)
	}
	h := NewServer(app).Hertz()
	return h.Engine, app, func() { _ = app.Close() }
}

func installOfflineKnowledgeService(app *core.App, opts offlineknowledge.ServiceOptions) *offlineknowledge.MemoryRepository {
	repo := offlineknowledge.NewMemoryRepository()
	app.OfflineKnowledge = offlineknowledge.NewService(repo, opts)
	return repo
}

func httpExecutableOfflineKnowledgeOptions() offlineknowledge.ServiceOptions {
	return offlineknowledge.ServiceOptions{
		HistorySource:     httpOfflineKnowledgeHistory{},
		QuestionClusterer: httpOfflineKnowledgeClusterer{},
		RecallReplayer:    httpOfflineKnowledgeReplayer{},
		CodexAnalyzer:     httpOfflineKnowledgeCodex{},
		ToolQuota: offlineknowledge.ToolQuota{
			MaxDeepSearchSteps: 2,
		},
		Now: fixedOfflineKnowledgeNow,
	}
}

type httpOfflineKnowledgeHistory struct{}

func (httpOfflineKnowledgeHistory) ExtractHistory(context.Context, offlineknowledge.HistoryRequest) ([]offlineknowledge.HistorySignal, error) {
	return []offlineknowledge.HistorySignal{
		{
			TenantID: "tenant_default",
			KBID:     "kb_default",
			Query:    "What is ORAG?",
			TraceID:  "trace_http_execute",
		},
	}, nil
}

type httpOfflineKnowledgeClusterer struct{}

func (httpOfflineKnowledgeClusterer) ClusterQuestions(_ context.Context, req offlineknowledge.ClusterRequest) ([]offlineknowledge.QuestionCluster, error) {
	return []offlineknowledge.QuestionCluster{
		{
			ID:                 "cluster_http_execute",
			TenantID:           req.Run.TenantID,
			RunID:              req.Run.ID,
			KBID:               req.Run.KBID,
			CanonicalQuestion:  "What is ORAG?",
			NormalizedQuestion: "what is orag",
			QuestionHash:       "hash_http_execute",
			OccurrenceCount:    1,
			SampleQuestions:    []string{"What is ORAG?"},
			TraceIDs:           []string{"trace_http_execute"},
		},
	}, nil
}

type httpOfflineKnowledgeReplayer struct{}

func (httpOfflineKnowledgeReplayer) ReplayRecall(context.Context, offlineknowledge.QuestionCluster) (offlineknowledge.RecallReplayResult, error) {
	return offlineknowledge.RecallReplayResult{
		BaselineRecallResults: []offlineknowledge.BaselineRecallItem{
			{TraceID: "trace_http_execute", ChunkID: "chunk_1", DocID: "doc_1", Rank: 1, Score: 0.91, Matched: true},
		},
		TraceSummaries: []offlineknowledge.TraceSummary{
			{TraceID: "trace_http_execute", Query: "What is ORAG?"},
		},
		SourceFingerprints: []offlineknowledge.SourceFingerprint{
			{DocID: "doc_1", DocVersion: "v1", ChunkID: "chunk_1", ChunkContentHash: "sha256:chunk_1"},
		},
	}, nil
}

type httpOfflineKnowledgeCodex struct{}

func (httpOfflineKnowledgeCodex) AnalyzeCodex(context.Context, offlineknowledge.CodexAnalyzeRequest) (offlineknowledge.CodexAnalyzeResponse, error) {
	return offlineknowledge.CodexAnalyzeResponse{
		ItemType:          offlineknowledge.ItemTypeKnowledgeGap,
		RecommendedAction: offlineknowledge.RecommendedActionCreateKnowledgeGapItem,
		RecallQuality:     offlineknowledge.RecallQualityNoAnswerInKB,
		FailureType:       offlineknowledge.FailureTypeKnowledgeGap,
		Confidence:        0.9,
	}, nil
}

func fixedOfflineKnowledgeNow() time.Time {
	return time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
}

func offlineKnowledgeItem(id, tenantID, kbID string, status offlineknowledge.ItemStatus, itemType offlineknowledge.ItemType) offlineknowledge.OptimizationItem {
	now := fixedOfflineKnowledgeNow()
	return offlineknowledge.OptimizationItem{
		ID:                id,
		TenantID:          tenantID,
		RunID:             "run_http",
		KBID:              kbID,
		QuestionClusterID: "cluster_http",
		ItemType:          itemType,
		Status:            status,
		CanonicalQuestion: "What is ORAG?",
		FinalAnswer:       "ORAG is a RAG framework.",
		RecallQuality:     offlineknowledge.RecallQualityMiss,
		FailureType:       offlineknowledge.FailureTypeSemanticGap,
		Confidence:        0.9,
		SourceFingerprints: []offlineknowledge.SourceFingerprint{
			{DocID: "doc_1", DocVersion: "v1", ChunkID: "chunk_1", ChunkContentHash: "sha256:chunk_1"},
		},
		Evidence: []offlineknowledge.Evidence{
			{ChunkID: "chunk_1", DocID: "doc_1", Quote: "ORAG is a retrieval augmented generation framework", Supports: "definition"},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func offlineKnowledgeStaleItem(id, contentHash string) offlineknowledge.OptimizationItem {
	item := offlineKnowledgeItem(id, "tenant_default", "kb_1", offlineknowledge.ItemStatusStale, offlineknowledge.ItemTypeAnswer)
	item.SourceFingerprints[0].ChunkContentHash = contentHash
	return item
}

type httpOfflineKnowledgeValidator struct {
	calls int
	err   error
}

func (v *httpOfflineKnowledgeValidator) ValidateItem(context.Context, string, string, offlineknowledge.OptimizationItem) error {
	v.calls++
	return v.err
}

func loginToken(t *testing.T, h *route.Engine) string {
	t.Helper()
	resp := performJSON(h, "POST", "/v1/auth/login", `{"username":"admin","password":"secret"}`, "")
	if resp.Code != 200 {
		t.Fatalf("login status = %d body=%s", resp.Code, resp.Body)
	}
	var body struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &body); err != nil {
		t.Fatal(err)
	}
	return body.AccessToken
}

func issueToken(t *testing.T, app *core.App, tenant string) string {
	t.Helper()
	token, err := app.Auth.IssueToken(tenant, "user_test")
	if err != nil {
		t.Fatal(err)
	}
	return token
}

type httpProjectRepository struct {
	items     map[string]project.Project
	createErr error
	listErr   error
	getErr    error
	updateErr error
}

func newHTTPProjectRepository() *httpProjectRepository {
	return &httpProjectRepository{items: map[string]project.Project{}}
}
func (r *httpProjectRepository) CreateWithEnvironments(_ context.Context, p project.Project, _ []project.Environment) error {
	if r.createErr != nil {
		return r.createErr
	}
	r.items[p.ID] = p
	return nil
}
func (r *httpProjectRepository) List(_ context.Context, tenant string) ([]project.Project, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	items := []project.Project{}
	for _, p := range r.items {
		if p.TenantID == tenant {
			items = append(items, p)
		}
	}
	return items, nil
}
func (r *httpProjectRepository) Get(_ context.Context, tenant, id string) (project.Project, bool, error) {
	if r.getErr != nil {
		return project.Project{}, false, r.getErr
	}
	p, ok := r.items[id]
	return p, ok && p.TenantID == tenant, nil
}
func (r *httpProjectRepository) Update(_ context.Context, p project.Project) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	r.items[p.ID] = p
	return nil
}

func performJSON(h *route.Engine, method, path, body, token string) testResponse {
	return performJSONWithHeaders(h, method, path, body, token)
}

func performJSONWithTrace(h *route.Engine, method, path, body, token, traceID string) testResponse {
	return performJSONWithHeaders(h, method, path, body, token, ut.Header{Key: observability.TraceIDHeader, Value: traceID})
}

func performJSONWithHeaders(h *route.Engine, method, path, body, token string, extraHeaders ...ut.Header) testResponse {
	headers := []ut.Header{{Key: "Content-Type", Value: "application/json"}}
	if token != "" {
		headers = append(headers, ut.Header{Key: "Authorization", Value: "Bearer " + token})
	}
	headers = append(headers, extraHeaders...)
	var reqBody *ut.Body
	if body != "" {
		reqBody = &ut.Body{Body: bytes.NewBufferString(body), Len: len(body)}
	}
	w := ut.PerformRequest(h, method, path, reqBody, headers...)
	result := w.Result()
	return testResponse{
		Code:          result.StatusCode(),
		Body:          string(result.Body()),
		ContentType:   string(result.Header.ContentType()),
		TraceIDHeader: result.Header.Get(observability.TraceIDHeader),
		Traceparent:   result.Header.Get("traceparent"),
	}
}

func performRaw(h *route.Engine, method, path, body, token string, extraHeaders ...ut.Header) testResponse {
	headers := []ut.Header{{Key: "Content-Type", Value: "application/octet-stream"}}
	if token != "" {
		headers = append(headers, ut.Header{Key: "Authorization", Value: "Bearer " + token})
	}
	headers = append(headers, extraHeaders...)
	w := ut.PerformRequest(h, method, path, &ut.Body{Body: bytes.NewBufferString(body), Len: len(body)}, headers...)
	result := w.Result()
	return testResponse{
		Code:          result.StatusCode(),
		Body:          string(result.Body()),
		ContentType:   string(result.Header.ContentType()),
		TraceIDHeader: result.Header.Get(observability.TraceIDHeader),
	}
}

func performMultipartUpload(t *testing.T, h *route.Engine, path, filename, content, token string) testResponse {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	headers := []ut.Header{{Key: "Content-Type", Value: writer.FormDataContentType()}}
	if token != "" {
		headers = append(headers, ut.Header{Key: "Authorization", Value: "Bearer " + token})
	}
	w := ut.PerformRequest(h, "POST", path, &ut.Body{Body: bytes.NewReader(body.Bytes()), Len: body.Len()}, headers...)
	result := w.Result()
	return testResponse{
		Code:          result.StatusCode(),
		Body:          string(result.Body()),
		ContentType:   string(result.Header.ContentType()),
		TraceIDHeader: result.Header.Get(observability.TraceIDHeader),
	}
}

func assertMissingKnowledgeBaseResponse(t *testing.T, resp testResponse) {
	t.Helper()
	if resp.Code != 404 {
		t.Fatalf("missing knowledge base status = %d body=%s", resp.Code, resp.Body)
	}
	if !strings.Contains(resp.Body, `"code":"knowledge_base_not_found"`) {
		t.Fatalf("unexpected missing knowledge base body: %s", resp.Body)
	}
}

func assertErrorResponse(t *testing.T, resp testResponse, status int, code, traceID string) {
	t.Helper()
	if resp.Code != status {
		t.Fatalf("status = %d, want %d body=%s", resp.Code, status, resp.Body)
	}
	if !strings.Contains(resp.Body, `"code":"`+code+`"`) {
		t.Fatalf("error response missing code %q: %s", code, resp.Body)
	}
	if !strings.Contains(resp.Body, `"trace_id":"`+traceID+`"`) {
		t.Fatalf("error response missing trace %q: %s", traceID, resp.Body)
	}
	if resp.TraceIDHeader != traceID {
		t.Fatalf("trace header = %q, want %q", resp.TraceIDHeader, traceID)
	}
}

func assertNoChunks(t *testing.T, app *core.App, kbID string) {
	t.Helper()
	chunks, ok := app.KBStore.(kb.ChunkSource)
	if !ok {
		t.Fatalf("test knowledge base store does not expose chunks")
	}
	if got := chunks.Chunks("tenant_default", kbID); len(got) != 0 {
		t.Fatalf("chunks created for missing knowledge base: %#v", got)
	}
}

type fakeKnowledgeBaseRepository struct {
	putErr    error
	listItems []kb.KnowledgeBase
	listErr   error
	getItem   kb.KnowledgeBase
	getFound  bool
	getErr    error
}

func (r fakeKnowledgeBaseRepository) PutKnowledgeBase(context.Context, kb.KnowledgeBase) error {
	return r.putErr
}

func (r fakeKnowledgeBaseRepository) ListKnowledgeBases(context.Context, string) ([]kb.KnowledgeBase, error) {
	return r.listItems, r.listErr
}

func (r fakeKnowledgeBaseRepository) GetKnowledgeBase(context.Context, string, string) (kb.KnowledgeBase, bool, error) {
	return r.getItem, r.getFound, r.getErr
}

func (r fakeKnowledgeBaseRepository) DeleteKnowledgeBase(context.Context, string, string) (bool, error) {
	return r.getFound, r.getErr
}

type countingJobStore struct {
	delegate    ingest.JobStore
	createCalls int
	updateCalls int
}

func (s *countingJobStore) CreateJob(ctx context.Context, job ingest.Job) (ingest.Job, error) {
	s.createCalls++
	return s.delegate.CreateJob(ctx, job)
}

func (s *countingJobStore) UpdateJob(ctx context.Context, job ingest.Job) error {
	s.updateCalls++
	return s.delegate.UpdateJob(ctx, job)
}

func (s *countingJobStore) GetJob(ctx context.Context, tenantID, id string) (ingest.Job, bool, error) {
	return s.delegate.GetJob(ctx, tenantID, id)
}

type countingPipeline struct {
	calls int
	resp  rag.QueryResponse
	err   error
}

func (p *countingPipeline) Invoke(context.Context, rag.QueryRequest) (rag.QueryResponse, error) {
	p.calls++
	return p.resp, p.err
}

type recordingPipeline struct {
	requests []rag.QueryRequest
	answer   string
}

func (p *recordingPipeline) Invoke(_ context.Context, req rag.QueryRequest) (rag.QueryResponse, error) {
	p.requests = append(p.requests, req)
	answer := p.answer
	if answer == "" {
		answer = "ok"
	}
	return rag.QueryResponse{
		Answer:      answer,
		TraceID:     req.TraceID,
		CacheStatus: "miss",
		Profile:     rag.ProfileRealtime,
		LatencyMS:   1,
	}, nil
}

type recordingProductionQuery struct {
	requests []rag.QueryRequest
}

func (p *recordingProductionQuery) Query(_ context.Context, request rag.QueryRequest) (rag.QueryResponse, error) {
	p.requests = append(p.requests, request)
	return rag.QueryResponse{Answer: "released", TraceID: request.TraceID, CacheStatus: "miss", Profile: rag.ProfileRealtime, LatencyMS: 1}, nil
}

type failingPipeline struct {
	err error
}

func (p failingPipeline) Invoke(context.Context, rag.QueryRequest) (rag.QueryResponse, error) {
	return rag.QueryResponse{}, p.err
}

type httpFakeJudge struct{}

func (httpFakeJudge) Judge(_ context.Context, input evalpkg.JudgeInput) (evalpkg.JudgeOutput, error) {
	return evalpkg.JudgeOutput{
		Scores:      map[string]float64{"faithfulness": 1},
		Labels:      map[string]string{"faithfulness": "good"},
		Pass:        true,
		Rationale:   input.Query,
		RawResponse: `{"scores":{"faithfulness":1},"pass":true}`,
		ParsedJSON:  map[string]any{"scores": map[string]any{"faithfulness": float64(1)}},
		TokenUsage:  evalpkg.TokenUsage{PromptTokens: 2, CompletionTokens: 1, TotalTokens: 3},
		CostUSD:     0.01,
	}, nil
}

type httpFakeQAGJudge struct{}

func (httpFakeQAGJudge) ScoreQAG(_ context.Context, input evalpkg.JudgeInput) (evalpkg.QAGOutput, error) {
	return evalpkg.QAGOutput{
		Score:       1,
		Metrics:     map[string]float64{"qag_score": 1, "qag_claim_coverage": 1, "qag_question_count": 1, "qag_unverifiable_rate": 0},
		Claims:      []evalpkg.QAGClaim{{Claim: input.Answer, Verdict: "supported"}},
		RawResponse: `{"score":1}`,
		ParsedJSON:  map[string]any{"score": float64(1)},
		TokenUsage:  evalpkg.TokenUsage{PromptTokens: 2, CompletionTokens: 2, TotalTokens: 4},
		CostUSD:     0.02,
	}, nil
}
