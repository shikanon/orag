package contract

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func TestOpenAPI(t *testing.T) {
	doc, err := openapi3.NewLoader().LoadFromFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatalf("openapi validation failed: %v", err)
	}

	for _, route := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/healthz"},
		{http.MethodGet, "/readyz"},
		{http.MethodGet, "/metrics"},
		{http.MethodGet, "/docs"},
		{http.MethodPost, "/v1/auth/login"},
		{http.MethodGet, "/v1/projects"},
		{http.MethodPost, "/v1/projects"},
		{http.MethodGet, "/v1/projects/{project_id}"},
		{http.MethodPatch, "/v1/projects/{project_id}"},
		{http.MethodGet, "/v1/knowledge-bases"},
		{http.MethodPost, "/v1/knowledge-bases"},
		{http.MethodGet, "/v1/knowledge-bases/{id}"},
		{http.MethodDelete, "/v1/knowledge-bases/{id}"},
		{http.MethodPost, "/v1/knowledge-bases/{id}/documents"},
		{http.MethodPost, "/v1/knowledge-bases/{id}/documents:import"},
		{http.MethodPost, "/v1/knowledge-bases/{id}/uploads"},
		{http.MethodGet, "/v1/uploads/{id}"},
		{http.MethodPut, "/v1/uploads/{id}"},
		{http.MethodDelete, "/v1/uploads/{id}"},
		{http.MethodPost, "/v1/uploads/{id}:complete"},
		{http.MethodGet, "/v1/ingestion-jobs/{id}"},
		{http.MethodPost, "/v1/query"},
		{http.MethodPost, "/v1/query:stream"},
		{http.MethodPost, "/v1/datasets"},
		{http.MethodPost, "/v1/datasets/{id}/items"},
		{http.MethodPost, "/v1/evaluations"},
		{http.MethodGet, "/v1/evaluations/{id}"},
		{http.MethodPost, "/v1/optimizations"},
		{http.MethodGet, "/v1/optimizations/{id}"},
		{http.MethodPost, "/v1/optimizations/{id}:cancel"},
		{http.MethodPost, "/v1/optimizations/{id}:resume"},
		{http.MethodGet, "/v1/offline-knowledge/runs"},
		{http.MethodPost, "/v1/offline-knowledge/runs"},
		{http.MethodPost, "/v1/offline-knowledge/scheduler:trigger"},
		{http.MethodGet, "/v1/offline-knowledge/runs/{id}"},
		{http.MethodPost, "/v1/offline-knowledge/runs/{id}/execute"},
		{http.MethodGet, "/v1/offline-knowledge/runs/{id}/questions"},
		{http.MethodGet, "/v1/optimization-items"},
		{http.MethodGet, "/v1/optimization-items/{id}"},
		{http.MethodPost, "/v1/optimization-items/{id}/{action}"},
		{http.MethodPost, "/v1/optimization-items/revalidate"},
	} {
		item := doc.Paths.Find(route.path)
		if item == nil {
			t.Fatalf("missing path %s", route.path)
		}
		if item.GetOperation(route.method) == nil {
			t.Fatalf("missing operation %s %s", route.method, route.path)
		}
	}

	for _, schema := range []string{
		"ErrorResponse",
		"LoginRequest",
		"LoginResponse",
		"Project",
		"CreateProjectRequest",
		"UpdateProjectRequest",
		"ProjectListResponse",
		"KnowledgeBase",
		"QueryRequest",
		"QueryResponse",
		"IngestionJob",
		"CreateUploadRequest",
		"UploadSession",
		"CompleteUploadResponse",
		"Dataset",
		"DatasetItem",
		"RunEvaluationRequest",
		"RunEvaluationResponse",
		"EvaluationMetrics",
		"SplitSummary",
		"HoldoutGateConfig",
		"HoldoutGateResult",
		"OptimizeRequest",
		"OptimizeResult",
		"OptimizationAcceptedResponse",
		"OptimizationStatusResponse",
		"OptimizationRun",
		"OptimizationCandidate",
		"JudgeConfig",
		"JudgeRubric",
		"SearchSpace",
		"ObjectiveSpec",
		"ReadinessResponse",
		"OfflineKnowledgeRun",
		"OfflineKnowledgeRunResult",
		"OfflineKnowledgeSchedulerTriggerResponse",
		"OptimizationItem",
		"SourceFingerprint",
		"EvalReport",
		"RegressionResult",
		"ProfileNeutrality",
		"ProfileExperiment",
	} {
		if doc.Components.Schemas[schema] == nil {
			t.Fatalf("missing schema %s", schema)
		}
	}

	for _, route := range []struct {
		method string
		path   string
		status int
	}{
		{http.MethodDelete, "/v1/knowledge-bases/{id}", http.StatusNoContent},
		{http.MethodGet, "/v1/knowledge-bases", http.StatusInternalServerError},
		{http.MethodGet, "/v1/knowledge-bases/{id}", http.StatusInternalServerError},
		{http.MethodDelete, "/v1/knowledge-bases/{id}", http.StatusNotFound},
		{http.MethodDelete, "/v1/knowledge-bases/{id}", http.StatusInternalServerError},
		{http.MethodPost, "/v1/knowledge-bases/{id}/documents", http.StatusNotFound},
		{http.MethodPost, "/v1/knowledge-bases/{id}/documents:import", http.StatusNotFound},
		{http.MethodPost, "/v1/query", http.StatusNotFound},
		{http.MethodPost, "/v1/query:stream", http.StatusNotFound},
		{http.MethodGet, "/v1/optimizations/{id}", http.StatusNotFound},
		{http.MethodPost, "/v1/optimizations/{id}:cancel", http.StatusNotFound},
		{http.MethodPost, "/v1/optimizations/{id}:resume", http.StatusNotFound},
		{http.MethodPost, "/v1/offline-knowledge/runs/{id}/execute", http.StatusConflict},
		{http.MethodPost, "/v1/offline-knowledge/runs/{id}/execute", http.StatusServiceUnavailable},
		{http.MethodPost, "/v1/offline-knowledge/scheduler:trigger", http.StatusServiceUnavailable},
		{http.MethodPost, "/v1/optimization-items/{id}/{action}", http.StatusConflict},
		{http.MethodPost, "/v1/optimization-items/{id}/{action}", http.StatusServiceUnavailable},
	} {
		op := doc.Paths.Find(route.path).GetOperation(route.method)
		if op.Responses.Get(route.status) == nil {
			t.Fatalf("%s %s missing %d response", route.method, route.path, route.status)
		}
	}

	for path, item := range doc.Paths {
		if path == "/v1/auth/login" || len(path) < len("/v1/") || path[:len("/v1/")] != "/v1/" {
			continue
		}
		for method, op := range item.Operations() {
			if op.Security == nil || !hasBearerAuth(*op.Security) {
				t.Fatalf("%s %s missing bearerAuth security", method, path)
			}
		}
	}
}

func TestOptimizationAndJudgeSchemasExposeAsyncContract(t *testing.T) {
	doc, err := openapi3.NewLoader().LoadFromFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatalf("openapi validation failed: %v", err)
	}

	evalReq := requireSchema(t, doc, "RunEvaluationRequest")
	for _, name := range []string{"split", "scoped_shadow_item_id", "holdout_gate"} {
		if _, ok := evalReq.Properties[name]; !ok {
			t.Fatalf("RunEvaluationRequest missing %s", name)
		}
	}
	for _, name := range []string{"judge", "qag", "pairwise"} {
		ref, ok := evalReq.Properties[name]
		if !ok {
			t.Fatalf("RunEvaluationRequest missing %s", name)
		}
		if ref.Ref != "#/components/schemas/JudgeConfig" {
			t.Fatalf("%s ref = %q, want JudgeConfig", name, ref.Ref)
		}
	}

	optReq := requireSchema(t, doc, "OptimizeRequest")
	for _, name := range []string{"objective", "search_space", "search", "budget", "profiles", "top_ks", "holdout_gate"} {
		if _, ok := optReq.Properties[name]; !ok {
			t.Fatalf("OptimizeRequest missing %s", name)
		}
	}
	if optReq.Properties["holdout_gate"].Ref != "#/components/schemas/HoldoutGateConfig" {
		t.Fatalf("OptimizeRequest.holdout_gate ref = %q, want HoldoutGateConfig", optReq.Properties["holdout_gate"].Ref)
	}

	postOp := doc.Paths.Find("/v1/optimizations").Post
	if got := postOp.Responses.Get(http.StatusAccepted).Value.Content.Get("application/json").Schema.Ref; got != "#/components/schemas/OptimizationAcceptedResponse" {
		t.Fatalf("POST /v1/optimizations 202 schema = %q", got)
	}
	accepted := requireSchema(t, doc, "OptimizationAcceptedResponse")
	for _, name := range []string{"run_id", "poll_url", "cancel_url", "resume_url"} {
		if _, ok := accepted.Properties[name]; !ok {
			t.Fatalf("OptimizationAcceptedResponse missing %s", name)
		}
	}
	status := requireSchema(t, doc, "OptimizationStatusResponse")
	if status.Properties["run"].Ref != "#/components/schemas/OptimizationRun" {
		t.Fatalf("OptimizationStatusResponse.run ref = %q", status.Properties["run"].Ref)
	}
	optRun := requireSchema(t, doc, "OptimizationRun")
	if optRun.Properties["holdout_gate"].Ref != "#/components/schemas/HoldoutGateResult" {
		t.Fatalf("OptimizationRun.holdout_gate ref = %q, want HoldoutGateResult", optRun.Properties["holdout_gate"].Ref)
	}
}

func TestQueryResponseCacheStatusEnumCoversRuntimeStatuses(t *testing.T) {
	doc, err := openapi3.NewLoader().LoadFromFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatalf("openapi validation failed: %v", err)
	}

	queryResponse := requireSchema(t, doc, "QueryResponse")
	cacheStatusRef, ok := queryResponse.Properties["cache_status"]
	if !ok || cacheStatusRef == nil || cacheStatusRef.Value == nil {
		t.Fatal("QueryResponse.cache_status schema missing")
	}
	values := make(map[string]bool, len(cacheStatusRef.Value.Enum))
	for _, raw := range cacheStatusRef.Value.Enum {
		value, ok := raw.(string)
		if !ok {
			t.Fatalf("QueryResponse.cache_status enum value has type %T, want string", raw)
		}
		values[value] = true
	}
	for _, want := range []string{"hit", "miss", "error", "bypass"} {
		if !values[want] {
			t.Fatalf("QueryResponse.cache_status enum missing %q; enum=%#v", want, cacheStatusRef.Value.Enum)
		}
	}
}

func TestEvaluationSchemasExposeQualityMetrics(t *testing.T) {
	doc, err := openapi3.NewLoader().LoadFromFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatalf("openapi validation failed: %v", err)
	}

	evalResp := requireSchema(t, doc, "RunEvaluationResponse")
	metricsRef, ok := evalResp.Properties["metrics"]
	if !ok {
		t.Fatal("RunEvaluationResponse missing metrics")
	}
	if metricsRef.Ref != "#/components/schemas/EvaluationMetrics" {
		t.Fatalf("metrics ref = %q, want EvaluationMetrics", metricsRef.Ref)
	}
	if evalResp.Description == "" {
		t.Fatal("RunEvaluationResponse missing pairwise accuracy semantics description")
	}
	for _, name := range []string{
		"weighted_sample_count",
		"unweighted_sample_count",
		"split",
		"split_summary",
		"missing_split",
		"holdout_gate",
	} {
		if _, ok := evalResp.Properties[name]; !ok {
			t.Fatalf("RunEvaluationResponse missing %s", name)
		}
	}
	if evalResp.Properties["holdout_gate"].Ref != "#/components/schemas/HoldoutGateResult" {
		t.Fatalf("holdout_gate ref = %q, want HoldoutGateResult", evalResp.Properties["holdout_gate"].Ref)
	}

	evalMetrics := requireSchema(t, doc, "EvaluationMetrics")
	for _, name := range []string{
		"deterministic_answer_match",
		"pairwise_accuracy",
		"ndcg_at_k",
		"recall_at_k",
		"mrr",
		"map",
		"coverage",
		"retrieval_failure_rate",
		"redundancy_rate",
		"duplicate_count",
		"deduped_top_k_count",
		"alpha_ndcg",
		"aspect_coverage",
		"latency_p95_ms",
		"cache_hit_rate",
	} {
		if _, ok := evalMetrics.Properties[name]; !ok {
			t.Fatalf("EvaluationMetrics missing %s", name)
		}
	}
	pairwise := evalMetrics.Properties["pairwise_accuracy"]
	if pairwise.Value == nil || pairwise.Value.Description == "" || !pairwise.Value.Deprecated {
		t.Fatal("pairwise_accuracy missing migration compatibility description/deprecation")
	}
	deterministic := evalMetrics.Properties["deterministic_answer_match"]
	if deterministic.Value == nil || deterministic.Value.Description == "" {
		t.Fatal("deterministic_answer_match missing metric description")
	}

	candidate := requireSchema(t, doc, "CandidateResult")
	for _, name := range []string{
		"score",
		"score_metric",
		"deterministic_answer_match",
		"pairwise_accuracy",
		"ndcg_at_k",
		"recall_at_k",
		"mrr",
		"map",
		"retrieval_failure_rate",
		"redundancy_rate",
		"duplicate_count",
		"deduped_top_k_count",
		"alpha_ndcg",
		"aspect_coverage",
		"latency_p95_ms",
	} {
		if _, ok := candidate.Properties[name]; !ok {
			t.Fatalf("CandidateResult missing %s", name)
		}
	}
	score := candidate.Properties["score"]
	if score.Value == nil || score.Value.Description == "" {
		t.Fatal("CandidateResult.score missing primary metric description")
	}

	evalReport := requireSchema(t, doc, "EvalReport")
	for _, name := range []string{"scoped_item_id", "profile_neutrality", "profile_experiment", "holdout_gate"} {
		if _, ok := evalReport.Properties[name]; !ok {
			t.Fatalf("EvalReport missing %s", name)
		}
	}
	regression := requireSchema(t, doc, "RegressionResult")
	for _, name := range []string{"scoped_item_id", "profile_neutrality", "profile_experiment", "holdout_gate"} {
		if _, ok := regression.Properties[name]; !ok {
			t.Fatalf("RegressionResult missing %s", name)
		}
	}
}

func TestRalphLoopCapabilityManifest(t *testing.T) {
	doc, err := openapi3.NewLoader().LoadFromFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatalf("openapi validation failed: %v", err)
	}

	manifest := requireExtensionMap(t, doc.Extensions, "x-orag-agent-capabilities")
	if got := fmt.Sprint(manifest["version"]); got != "1" {
		t.Fatalf("capability manifest version = %q, want 1", got)
	}
	if got := requireString(t, manifest, "source"); got != "openapi" {
		t.Fatalf("capability manifest source = %q, want openapi", got)
	}

	boundary := requireMap(t, manifest, "generation_boundary")
	requireBool(t, boundary, "mcp_tool_schema", true)
	requireBool(t, boundary, "skill_manifests", true)
	requireBool(t, boundary, "runtime_handlers", false)
	if requireString(t, boundary, "note") == "" {
		t.Fatal("generation_boundary.note must explain the Task 1 runtime boundary")
	}

	capability := requireCapability(t, manifest, "ralph-loop")
	for _, field := range []string{"name", "description", "status"} {
		if requireString(t, capability, field) == "" {
			t.Fatalf("ralph-loop capability missing %s", field)
		}
	}
	if got := requireString(t, capability, "status"); got != "planned" {
		t.Fatalf("ralph-loop status = %q, want planned for Task 1 boundary", got)
	}
	if doc.Paths.Find("/v1/ralph-loop") != nil {
		t.Fatal("ralph-loop is planned-only: /v1/ralph-loop must not be declared as a runnable OpenAPI path until a real handler exists")
	}

	source := requireMap(t, capability, "source")
	for key, want := range map[string]string{
		"kind":         "planned_http",
		"method":       http.MethodPost,
		"path":         "/v1/ralph-loop",
		"operation_id": "runRalphLoop",
	} {
		if got := requireString(t, source, key); got != want {
			t.Fatalf("ralph-loop source.%s = %q, want %q", key, got, want)
		}
	}
	requireNonEmptyList(t, source, "backing_services")

	auth := requireMap(t, capability, "auth")
	requireBool(t, auth, "required", true)
	if got := requireString(t, auth, "scheme"); got != "bearerAuth" {
		t.Fatalf("ralph-loop auth.scheme = %q, want bearerAuth", got)
	}
	requireListContains(t, auth, "env", "ORAG_API_BASE_URL", "ORAG_API_TOKEN", "ORAG_TENANT_ID")

	trace := requireMap(t, capability, "trace")
	for key, want := range map[string]string{
		"request_header":  "X-Trace-ID",
		"response_header": "X-Trace-ID",
		"response_field":  "trace_id",
	} {
		if got := requireString(t, trace, key); got != want {
			t.Fatalf("ralph-loop trace.%s = %q, want %q", key, got, want)
		}
	}

	schemas := requireMap(t, capability, "schemas")
	for key, want := range map[string]string{
		"input":  "#/components/schemas/RalphLoopRequest",
		"output": "#/components/schemas/RalphLoopResponse",
		"error":  "#/components/schemas/ErrorResponse",
	} {
		ref := requireString(t, schemas, key)
		if ref != want {
			t.Fatalf("ralph-loop schemas.%s = %q, want %q", key, ref, want)
		}
		requireSchemaRefExists(t, doc, ref)
	}

	mcp := requireMap(t, capability, "mcp")
	for _, field := range []string{"tool_name", "description", "input_schema", "output_schema"} {
		if requireString(t, mcp, field) == "" {
			t.Fatalf("ralph-loop mcp missing %s", field)
		}
	}
	if got := requireString(t, mcp, "tool_name"); got != "ralph_loop_run" {
		t.Fatalf("ralph-loop mcp.tool_name = %q, want ralph_loop_run", got)
	}
	requireSchemaRefExists(t, doc, requireString(t, mcp, "input_schema"))
	requireSchemaRefExists(t, doc, requireString(t, mcp, "output_schema"))

	skills := requireMap(t, capability, "skills")
	for _, field := range []string{"manifest_name", "description"} {
		if requireString(t, skills, field) == "" {
			t.Fatalf("ralph-loop skills missing %s", field)
		}
	}

	examples := requireNonEmptyList(t, capability, "examples")
	example, ok := examples[0].(map[string]interface{})
	if !ok {
		t.Fatalf("ralph-loop example has type %T, want map", examples[0])
	}
	for _, field := range []string{"name", "input", "expected_output"} {
		if _, ok := example[field]; !ok {
			t.Fatalf("ralph-loop example missing %s", field)
		}
	}

	request := requireSchema(t, doc, "RalphLoopRequest")
	for _, field := range []string{"task_spec_path", "task_id", "mode", "max_rounds"} {
		if _, ok := request.Properties[field]; !ok {
			t.Fatalf("RalphLoopRequest missing %s", field)
		}
	}
	response := requireSchema(t, doc, "RalphLoopResponse")
	for _, field := range []string{"run_id", "status", "verdict", "summary", "trace_id", "artifacts"} {
		if _, ok := response.Properties[field]; !ok {
			t.Fatalf("RalphLoopResponse missing %s", field)
		}
	}
}

func requireSchema(t *testing.T, doc *openapi3.T, name string) *openapi3.Schema {
	t.Helper()

	ref := doc.Components.Schemas[name]
	if ref == nil || ref.Value == nil {
		t.Fatalf("missing schema %s", name)
	}
	return ref.Value
}

func hasBearerAuth(requirements openapi3.SecurityRequirements) bool {
	for _, req := range requirements {
		if _, ok := req["bearerAuth"]; ok {
			return true
		}
	}
	return false
}

func requireExtensionMap(t *testing.T, extensions map[string]interface{}, key string) map[string]interface{} {
	t.Helper()

	raw, ok := extensions[key]
	if !ok {
		t.Fatalf("missing OpenAPI extension %s", key)
	}
	out, ok := raw.(map[string]interface{})
	if !ok {
		t.Fatalf("OpenAPI extension %s has type %T, want map", key, raw)
	}
	return out
}

func requireMap(t *testing.T, parent map[string]interface{}, key string) map[string]interface{} {
	t.Helper()

	raw, ok := parent[key]
	if !ok {
		t.Fatalf("missing map field %s", key)
	}
	out, ok := raw.(map[string]interface{})
	if !ok {
		t.Fatalf("field %s has type %T, want map", key, raw)
	}
	return out
}

func requireString(t *testing.T, parent map[string]interface{}, key string) string {
	t.Helper()

	raw, ok := parent[key]
	if !ok {
		t.Fatalf("missing string field %s", key)
	}
	value, ok := raw.(string)
	if !ok {
		t.Fatalf("field %s has type %T, want string", key, raw)
	}
	return value
}

func requireBool(t *testing.T, parent map[string]interface{}, key string, want bool) {
	t.Helper()

	raw, ok := parent[key]
	if !ok {
		t.Fatalf("missing bool field %s", key)
	}
	got, ok := raw.(bool)
	if !ok {
		t.Fatalf("field %s has type %T, want bool", key, raw)
	}
	if got != want {
		t.Fatalf("field %s = %t, want %t", key, got, want)
	}
}

func requireCapability(t *testing.T, manifest map[string]interface{}, id string) map[string]interface{} {
	t.Helper()

	for _, raw := range requireNonEmptyList(t, manifest, "capabilities") {
		capability, ok := raw.(map[string]interface{})
		if !ok {
			t.Fatalf("capability has type %T, want map", raw)
		}
		if capability["id"] == id {
			return capability
		}
	}
	t.Fatalf("missing capability %s", id)
	return nil
}

func requireNonEmptyList(t *testing.T, parent map[string]interface{}, key string) []interface{} {
	t.Helper()

	raw, ok := parent[key]
	if !ok {
		t.Fatalf("missing list field %s", key)
	}
	items, ok := raw.([]interface{})
	if !ok {
		t.Fatalf("field %s has type %T, want list", key, raw)
	}
	if len(items) == 0 {
		t.Fatalf("field %s must not be empty", key)
	}
	return items
}

func requireListContains(t *testing.T, parent map[string]interface{}, key string, wants ...string) {
	t.Helper()

	items := requireNonEmptyList(t, parent, key)
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		value, ok := item.(string)
		if !ok {
			t.Fatalf("field %s item has type %T, want string", key, item)
		}
		seen[value] = true
	}
	for _, want := range wants {
		if !seen[want] {
			t.Fatalf("field %s missing %q", key, want)
		}
	}
}

func requireSchemaRefExists(t *testing.T, doc *openapi3.T, ref string) {
	t.Helper()

	const prefix = "#/components/schemas/"
	if len(ref) <= len(prefix) || ref[:len(prefix)] != prefix {
		t.Fatalf("schema ref %q must use %s", ref, prefix)
	}
	requireSchema(t, doc, ref[len(prefix):])
}
