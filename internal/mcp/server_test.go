package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/shikanon/orag/internal/selfcheck"
	"github.com/shikanon/orag/internal/selfops"
)

func TestLoadToolsFromOpenAPI(t *testing.T) {
	tools, err := LoadToolsFromOpenAPI(context.Background(), "../../api/openapi.yaml")
	if err != nil {
		t.Fatalf("LoadToolsFromOpenAPI() error = %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("tools len = %d, want 1", len(tools))
	}
	tool := tools[0]
	if tool.Name != "ralph_loop_run" {
		t.Fatalf("tool name = %q", tool.Name)
	}
	if tool.Description == "" || tool.Capability.Path != "/v1/ralph-loop" || tool.Capability.AuthScheme != "bearerAuth" {
		t.Fatalf("unexpected tool metadata: %#v", tool)
	}
	if got := schemaType(t, tool.InputSchema, "task_spec_path"); got != "string" {
		t.Fatalf("task_spec_path type = %q, want string", got)
	}
	if got := schemaType(t, tool.InputSchema, "max_rounds"); got != "integer" {
		t.Fatalf("max_rounds type = %q, want integer", got)
	}
	if _, ok := tool.OutputSchema["properties"].(map[string]any)["trace_id"]; !ok {
		t.Fatalf("output schema missing trace_id: %#v", tool.OutputSchema)
	}
}

func TestServerInitializeAndListTools(t *testing.T) {
	server := newTestServer(t, fakeToolClient{})

	initResp := handleResponse(t, server, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	if initResp.Error != nil {
		t.Fatalf("initialize error = %#v", initResp.Error)
	}
	result := initResp.Result.(map[string]any)
	if result["protocolVersion"] != ProtocolVersion {
		t.Fatalf("protocolVersion = %#v", result["protocolVersion"])
	}

	listResp := handleResponse(t, server, `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
	if listResp.Error != nil {
		t.Fatalf("tools/list error = %#v", listResp.Error)
	}
	tools := listResp.Result.(map[string]any)["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("tools len = %d, want 1", len(tools))
	}
	tool := tools[0].(map[string]any)
	if tool["name"] != "ralph_loop_run" || tool["inputSchema"] == nil || tool["outputSchema"] == nil || tool["annotations"] == nil {
		t.Fatalf("unexpected tool list entry: %#v", tool)
	}
}

func TestServerCallToolValidatesArguments(t *testing.T) {
	server := newTestServer(t, fakeToolClient{})
	body := `{"jsonrpc":"2.0","id":"call-1","method":"tools/call","params":{"name":"ralph_loop_run","arguments":{"task_spec_path":"tasks.md","task_id":"Task 2","mode":"invalid","max_rounds":1}}}`

	resp := handleResponse(t, server, body)
	if resp.Error == nil {
		t.Fatal("tools/call error = nil")
	}
	if resp.Error.Code != codeInvalidParams || resp.Error.Data["code"] != "invalid_tool_arguments" {
		t.Fatalf("unexpected error: %#v", resp.Error)
	}
	if !strings.Contains(resp.Error.Message, "mode") {
		t.Fatalf("error message = %q, want mode", resp.Error.Message)
	}
}

func TestServerCallToolReturnsStructuredContent(t *testing.T) {
	client := fakeToolClient{
		result: ToolResult{
			Payload: map[string]any{"status": "completed", "verdict": "pass", "trace_id": "trace_1"},
			TraceID: "trace_1",
			Status:  http.StatusOK,
		},
	}
	server := newTestServer(t, client)
	body := `{"jsonrpc":"2.0","id":"call-2","method":"tools/call","params":{"name":"ralph_loop_run","arguments":{"task_spec_path":"tasks.md","task_id":"Task 2","mode":"focused","max_rounds":1},"_meta":{"trace_id":"trace_req"}}}`

	resp := handleResponse(t, server, body)
	if resp.Error != nil {
		t.Fatalf("tools/call error = %#v", resp.Error)
	}
	result := resp.Result.(map[string]any)
	if result["isError"] != false {
		t.Fatalf("isError = %#v", result["isError"])
	}
	content := result["content"].([]any)[0].(map[string]any)
	if !strings.Contains(content["text"].(string), `"verdict":"pass"`) {
		t.Fatalf("content text = %s", content["text"])
	}
	meta := result["_meta"].(map[string]any)
	if meta["trace_id"] != "trace_1" {
		t.Fatalf("trace meta = %#v", meta)
	}
}

func TestServerListsAndRunsSelfCheckTool(t *testing.T) {
	tools, err := LoadToolsFromArtifacts("../../agent/mcp/tools/orag-self-check.json")
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(tools, RuntimeConfig{}, fakeToolClient{})
	if err != nil {
		t.Fatal(err)
	}
	server.checks = fakeSelfCheckExecutor{
		envelope: selfcheck.Envelope{
			SchemaVersion:      selfcheck.SchemaVersion,
			TraceID:            "trace_selfcheck",
			Scope:              selfcheck.ScopeAgentSync,
			Mode:               selfcheck.ModeFocused,
			Verdict:            selfcheck.VerdictPass,
			ExitCode:           0,
			RuntimeGateWarning: selfcheck.RuntimeGateWarning,
			Results: []selfcheck.CheckResult{{
				ID:       "orag.selfcheck.agent_sync.artifacts",
				Scope:    selfcheck.ScopeAgentSync,
				Name:     "Agent artifact drift",
				Severity: selfcheck.SeverityCritical,
				Status:   selfcheck.StatusPass,
				Verdict:  selfcheck.VerdictPass,
			}},
		},
	}

	listResp := handleResponse(t, server, `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)
	if listResp.Error != nil {
		t.Fatalf("tools/list error = %#v", listResp.Error)
	}
	listed := listResp.Result.(map[string]any)["tools"].([]any)[0].(map[string]any)
	if listed["name"] != "orag_check" {
		t.Fatalf("listed tool = %#v, want orag_check", listed)
	}

	callResp := handleResponse(t, server, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"orag_check","arguments":{"scope":"agent_sync","mode":"focused"},"_meta":{"trace_id":"trace_req"}}}`)
	if callResp.Error != nil {
		t.Fatalf("tools/call error = %#v", callResp.Error)
	}
	result := callResp.Result.(map[string]any)
	if result["isError"] != false {
		t.Fatalf("isError = %#v", result["isError"])
	}
	content := result["structuredContent"].(map[string]any)
	if content["schema_version"] != selfcheck.SchemaVersion || content["verdict"] != "pass" {
		t.Fatalf("unexpected structured content: %#v", content)
	}
	if !strings.Contains(content["runtime_gate_warning"].(string), "authoritative release gate") {
		t.Fatalf("missing gate warning: %#v", content)
	}
}

func TestServerRunsSelfOpsPlanAndApplyTools(t *testing.T) {
	tools, err := LoadToolsFromArtifacts("../../agent/mcp/tools/orag-self-ops.json")
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(tools, RuntimeConfig{}, fakeToolClient{err: errors.New("http client should not be called")})
	if err != nil {
		t.Fatal(err)
	}
	server.ops = fakeSelfOpsExecutor{
		plan: selfops.Plan{
			SchemaVersion:  selfops.SchemaVersion,
			TraceID:        "trace_plan",
			PlanID:         "plan_1",
			Scope:          selfops.ScopeAgentArtifacts,
			Verdict:        selfops.VerdictPass,
			Status:         selfops.StatusPlanned,
			DryRun:         true,
			IdempotencyKey: "selfops:key",
			LockKey:        "selfops:agent-artifacts",
		},
		apply: selfops.ApplyResult{
			SchemaVersion:  selfops.SchemaVersion,
			TraceID:        "trace_apply",
			PlanID:         "plan_1",
			Verdict:        selfops.VerdictPass,
			Status:         selfops.StatusCompleted,
			IdempotencyKey: "selfops:key",
			LockKey:        "selfops:agent-artifacts",
		},
	}

	planResp := handleResponse(t, server, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"orag_maintenance_plan","arguments":{"scope":"agent_artifacts","dry_run":true},"_meta":{"trace_id":"trace_req"}}}`)
	if planResp.Error != nil {
		t.Fatalf("maintenance plan error = %#v", planResp.Error)
	}
	planResult := planResp.Result.(map[string]any)
	if planResult["isError"] != false {
		t.Fatalf("plan isError = %#v", planResult["isError"])
	}
	planContent := planResult["structuredContent"].(map[string]any)
	if planContent["plan_id"] != "plan_1" || planContent["lock_key"] != "selfops:agent-artifacts" {
		t.Fatalf("unexpected plan content: %#v", planContent)
	}

	applyResp := handleResponse(t, server, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"orag_apply_low_risk_action","arguments":{"plan_id":"plan_1","approved":true},"_meta":{"trace_id":"trace_req"}}}`)
	if applyResp.Error != nil {
		t.Fatalf("apply error = %#v", applyResp.Error)
	}
	applyResult := applyResp.Result.(map[string]any)
	applyContent := applyResult["structuredContent"].(map[string]any)
	if applyResult["isError"] != false || applyContent["status"] != selfops.StatusCompleted {
		t.Fatalf("unexpected apply content: %#v", applyResult)
	}
}

func TestServerListsAndRunsDiagnosticTools(t *testing.T) {
	tools, err := LoadToolsFromArtifacts("../../agent/mcp/tools/orag-self-diagnose.json")
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(tools, RuntimeConfig{}, fakeToolClient{err: errors.New("diagnostics must not call HTTP client")})
	if err != nil {
		t.Fatal(err)
	}

	listResp := handleResponse(t, server, `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)
	if listResp.Error != nil {
		t.Fatalf("tools/list error = %#v", listResp.Error)
	}
	listed := listResp.Result.(map[string]any)["tools"].([]any)
	if len(listed) != 3 {
		t.Fatalf("diagnostic tools len = %d, want 3", len(listed))
	}

	traceResp := handleResponse(t, server, `{"jsonrpc":"2.0","id":"trace-lookup","method":"tools/call","params":{"name":"orag_trace_lookup","arguments":{"trace_id":"trace_missing"}}}`)
	if traceResp.Error != nil {
		t.Fatalf("trace lookup error = %#v", traceResp.Error)
	}
	traceResult := traceResp.Result.(map[string]any)
	if traceResult["isError"] != true {
		t.Fatalf("trace lookup isError = %#v, want true for unavailable trace store", traceResult["isError"])
	}
	traceContent := traceResult["structuredContent"].(map[string]any)
	if traceContent["schema_version"] != selfcheck.SchemaVersion || traceContent["verdict"] != "blocked" || traceContent["found"] != false || traceContent["read_only"] != true {
		t.Fatalf("unexpected trace lookup structured content: %#v", traceContent)
	}
	traceFindings := traceContent["findings"].([]any)
	if len(traceFindings) != 1 || traceFindings[0].(map[string]any)["id"] != "orag.diagnostics.trace.store_unavailable" {
		t.Fatalf("unexpected trace lookup findings: %#v", traceFindings)
	}

	callResp := handleResponse(t, server, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"orag_diagnose","arguments":{"scope":"agent_sync","symptom":"make agent-sync-check failed","failed_command":"make agent-sync-check","failed_command_exit_code":1,"failed_command_output":"generated content differs","allow_commands":true},"_meta":{"trace_id":"trace_req"}}}`)
	if callResp.Error != nil {
		t.Fatalf("tools/call error = %#v", callResp.Error)
	}
	result := callResp.Result.(map[string]any)
	if result["isError"] != true {
		t.Fatalf("isError = %#v, want true for failed diagnosis", result["isError"])
	}
	content := result["structuredContent"].(map[string]any)
	if content["schema_version"] != selfcheck.SchemaVersion || content["verdict"] != "fail" || content["read_only"] != true {
		t.Fatalf("unexpected diagnostic structured content: %#v", content)
	}
	meta := result["_meta"].(map[string]any)
	if meta["trace_id"] != "trace_req" || meta["exit_code"].(float64) != 1 {
		t.Fatalf("unexpected diagnostic meta: %#v", meta)
	}
}

func TestServerTraceLookupFoundTrace(t *testing.T) {
	var gotAuth, gotTenant string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotTenant = r.Header.Get("X-ORAG-Tenant-ID")
		if r.Method != http.MethodGet || r.URL.Path != "/v1/traces/trace_found" {
			t.Fatalf("trace request method=%s path=%s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{
			"trace_id":"trace_found",
			"tenant_id":"tenant_1",
			"profile":"realtime",
			"latency_ms":42,
			"created_at":"2026-07-06T00:00:00Z",
			"has_error":true,
			"error_count":1,
			"node_spans":[{
				"id":"span_1",
				"node_name":"retrieve",
				"sequence":1,
				"latency_ms":40,
				"error":"timeout",
				"started_at":"2026-07-06T00:00:00Z",
				"ended_at":"2026-07-06T00:00:01Z",
				"created_at":"2026-07-06T00:00:01Z"
			}]
		}`))
	}))
	defer api.Close()

	server := newDiagnosticServer(t, RuntimeConfig{
		BaseURL: api.URL, Token: "token_secret", TenantID: "tenant_1", Timeout: time.Second,
	}, NewHTTPToolClient(api.Client()))

	resp := handleResponse(t, server, `{"jsonrpc":"2.0","id":"trace-found","method":"tools/call","params":{"name":"orag_trace_lookup","arguments":{"trace_id":"trace_found"}}}`)
	if resp.Error != nil {
		t.Fatalf("trace lookup error = %#v", resp.Error)
	}
	result := resp.Result.(map[string]any)
	if result["isError"] != false {
		t.Fatalf("trace lookup isError = %#v", result["isError"])
	}
	content := result["structuredContent"].(map[string]any)
	if content["verdict"] != "pass" || content["found"] != true || content["trace_id"] != "trace_found" {
		t.Fatalf("unexpected found trace content: %#v", content)
	}
	findings := content["findings"].([]any)
	if len(findings) != 1 || findings[0].(map[string]any)["id"] != "orag.diagnostics.trace.found" {
		t.Fatalf("unexpected found trace findings: %#v", findings)
	}
	evidence := content["evidence"].([]any)
	output := evidence[0].(map[string]any)["output"].(string)
	if !strings.Contains(output, "retrieve") || strings.Contains(output, "connect a trace repository") {
		t.Fatalf("unexpected found trace evidence: %#v", evidence)
	}
	if gotAuth != "Bearer token_secret" || gotTenant != "tenant_1" {
		t.Fatalf("trace headers auth=%q tenant=%q", gotAuth, gotTenant)
	}
}

func TestServerTraceLookupMissingTraceIsBlocked(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/traces/trace_missing" {
			t.Fatalf("trace request method=%s path=%s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":"trace_not_found","message":"trace not found","trace_id":"trace_missing"}}`))
	}))
	defer api.Close()

	server := newDiagnosticServer(t, RuntimeConfig{
		BaseURL: api.URL, Token: "token_secret", TenantID: "tenant_1", Timeout: time.Second,
	}, NewHTTPToolClient(api.Client()))

	resp := handleResponse(t, server, `{"jsonrpc":"2.0","id":"trace-missing","method":"tools/call","params":{"name":"orag_trace_lookup","arguments":{"trace_id":"trace_missing"}}}`)
	if resp.Error != nil {
		t.Fatalf("trace lookup error = %#v", resp.Error)
	}
	result := resp.Result.(map[string]any)
	content := result["structuredContent"].(map[string]any)
	if result["isError"] != true || content["verdict"] != "blocked" || content["found"] != false {
		t.Fatalf("unexpected missing trace content: %#v", result)
	}
	findings := content["findings"].([]any)
	if len(findings) != 1 || findings[0].(map[string]any)["id"] != "orag.diagnostics.trace.not_found" {
		t.Fatalf("unexpected missing trace findings: %#v", findings)
	}
}

func TestServerTraceLookupWithoutRuntimeConfigIsBlocked(t *testing.T) {
	server := newDiagnosticServer(t, RuntimeConfig{}, fakeToolClient{err: errors.New("trace lookup must not call generic tool client")})

	resp := handleResponse(t, server, `{"jsonrpc":"2.0","id":"trace-unavailable","method":"tools/call","params":{"name":"orag_trace_lookup","arguments":{"trace_id":"trace_missing"}}}`)
	if resp.Error != nil {
		t.Fatalf("trace lookup error = %#v", resp.Error)
	}
	result := resp.Result.(map[string]any)
	content := result["structuredContent"].(map[string]any)
	if result["isError"] != true || content["verdict"] != "blocked" || content["found"] != false || content["read_only"] != true {
		t.Fatalf("unexpected unavailable trace content: %#v", result)
	}
	findings := content["findings"].([]any)
	if len(findings) != 1 || findings[0].(map[string]any)["id"] != "orag.diagnostics.trace.store_unavailable" {
		t.Fatalf("unexpected unavailable trace findings: %#v", findings)
	}
}

func TestHTTPToolClientSendsConfigAndTrace(t *testing.T) {
	var gotAuth, gotTenant, gotTrace string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotTenant = r.Header.Get("X-ORAG-Tenant-ID")
		gotTrace = r.Header.Get("X-Trace-ID")
		if r.URL.Path != "/v1/ralph-loop" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		w.Header().Set("X-Trace-ID", "trace_resp")
		_, _ = w.Write([]byte(`{"run_id":"run_1","status":"completed","verdict":"pass","summary":"ok","trace_id":"trace_body","artifacts":[]}`))
	}))
	defer api.Close()

	tools, err := LoadToolsFromOpenAPI(context.Background(), "../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	client := NewHTTPToolClient(api.Client())
	result, err := client.CallTool(context.Background(), RuntimeConfig{
		BaseURL:  api.URL,
		Token:    "token_secret",
		TenantID: "tenant_1",
		Timeout:  time.Second,
	}, tools[0], validArgs(), map[string]any{"trace_id": "trace_req"})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if gotAuth != "Bearer token_secret" || gotTenant != "tenant_1" || gotTrace != "trace_req" {
		t.Fatalf("headers auth=%q tenant=%q trace=%q", gotAuth, gotTenant, gotTrace)
	}
	if result.TraceID != "trace_resp" || result.Status != http.StatusOK {
		t.Fatalf("result = %#v", result)
	}
}

func TestHTTPToolClientMissingConfig(t *testing.T) {
	tools, err := LoadToolsFromOpenAPI(context.Background(), "../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewHTTPToolClient(http.DefaultClient).CallTool(context.Background(), RuntimeConfig{BaseURL: "http://localhost:8080"}, tools[0], validArgs(), nil)
	var rpcErr *RPCError
	if !errors.As(err, &rpcErr) {
		t.Fatalf("error = %T %v, want RPCError", err, err)
	}
	if rpcErr.Code != codeConfigError || rpcErr.Data["code"] != "missing_config" {
		t.Fatalf("unexpected error: %#v", rpcErr)
	}
}

func TestHTTPToolClientMapsDownstreamErrors(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Trace-ID", "trace_auth")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":"invalid_bearer_token","message":"invalid bearer token","trace_id":"trace_body"}}`))
	}))
	defer api.Close()

	tools, err := LoadToolsFromOpenAPI(context.Background(), "../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewHTTPToolClient(api.Client()).CallTool(context.Background(), RuntimeConfig{
		BaseURL: api.URL, Token: "bad", TenantID: "tenant_1", Timeout: time.Second,
	}, tools[0], validArgs(), nil)
	var rpcErr *RPCError
	if !errors.As(err, &rpcErr) {
		t.Fatalf("error = %T %v, want RPCError", err, err)
	}
	if rpcErr.Code != codeHTTPError || rpcErr.Data["code"] != "invalid_bearer_token" || rpcErr.Data["trace_id"] != "trace_auth" {
		t.Fatalf("unexpected error: %#v", rpcErr)
	}
}

func TestHTTPToolClientMapsTimeout(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		time.Sleep(50 * time.Millisecond)
	}))
	defer api.Close()

	tools, err := LoadToolsFromOpenAPI(context.Background(), "../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewHTTPToolClient(api.Client()).CallTool(context.Background(), RuntimeConfig{
		BaseURL: api.URL, Token: "token", TenantID: "tenant_1", Timeout: time.Nanosecond,
	}, tools[0], validArgs(), nil)
	var rpcErr *RPCError
	if !errors.As(err, &rpcErr) {
		t.Fatalf("error = %T %v, want RPCError", err, err)
	}
	if rpcErr.Code != codeTimeoutError || rpcErr.Data["code"] != "downstream_timeout" {
		t.Fatalf("unexpected error: %#v", rpcErr)
	}
}

func schemaType(t *testing.T, schema map[string]any, property string) string {
	t.Helper()
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema properties missing: %#v", schema)
	}
	field, ok := properties[property].(map[string]any)
	if !ok {
		t.Fatalf("property %q missing: %#v", property, properties)
	}
	got, _ := field["type"].(string)
	return got
}

func newTestServer(t *testing.T, client ToolClient) *Server {
	t.Helper()
	tools, err := LoadToolsFromOpenAPI(context.Background(), "../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(tools, RuntimeConfig{
		BaseURL:  "http://localhost:8080",
		Token:    "token",
		TenantID: "tenant_1",
		Timeout:  time.Second,
	}, client)
	if err != nil {
		t.Fatal(err)
	}
	return server
}

func newDiagnosticServer(t *testing.T, cfg RuntimeConfig, client ToolClient) *Server {
	t.Helper()
	tools, err := LoadToolsFromArtifacts("../../agent/mcp/tools/orag-self-diagnose.json")
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(tools, cfg, client)
	if err != nil {
		t.Fatal(err)
	}
	return server
}

func decodeResponse(t *testing.T, data []byte, ok bool) jsonRPCResponse {
	t.Helper()
	if !ok {
		t.Fatal("Handle() returned no response")
	}
	var resp jsonRPCResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatalf("unmarshal response: %v\n%s", err, string(data))
	}
	return resp
}

func handleResponse(t *testing.T, server *Server, body string) jsonRPCResponse {
	t.Helper()
	data, ok := server.Handle(context.Background(), []byte(body))
	return decodeResponse(t, data, ok)
}

func validArgs() map[string]any {
	return map[string]any{
		"task_spec_path": ".trae/specs/add-ralph-loop-mcp-skills/tasks.md",
		"task_id":        "Task 2",
		"mode":           "focused",
		"max_rounds":     1,
	}
}

type fakeToolClient struct {
	result ToolResult
	err    error
}

type fakeSelfCheckExecutor struct {
	envelope selfcheck.Envelope
	err      error
}

type fakeSelfOpsExecutor struct {
	plan  selfops.Plan
	apply selfops.ApplyResult
	err   error
}

func (f fakeSelfCheckExecutor) Execute(context.Context, selfcheck.Request) (selfcheck.Envelope, error) {
	if f.err != nil {
		return selfcheck.Envelope{}, f.err
	}
	return f.envelope, nil
}

func (f fakeSelfOpsExecutor) Plan(context.Context, selfops.PlanRequest) (selfops.Plan, error) {
	if f.err != nil {
		return selfops.Plan{}, f.err
	}
	return f.plan, nil
}

func (f fakeSelfOpsExecutor) Apply(context.Context, selfops.ApplyRequest) (selfops.ApplyResult, error) {
	if f.err != nil {
		return selfops.ApplyResult{}, f.err
	}
	return f.apply, nil
}

func (f fakeToolClient) CallTool(context.Context, RuntimeConfig, ToolDefinition, map[string]any, map[string]any) (ToolResult, error) {
	if f.err != nil {
		return ToolResult{}, f.err
	}
	return f.result, nil
}
