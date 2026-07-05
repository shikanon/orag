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

func (f fakeToolClient) CallTool(context.Context, RuntimeConfig, ToolDefinition, map[string]any, map[string]any) (ToolResult, error) {
	if f.err != nil {
		return ToolResult{}, f.err
	}
	return f.result, nil
}
