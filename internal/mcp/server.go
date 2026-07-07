package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"

	"github.com/shikanon/orag/internal/diagnostics"
	"github.com/shikanon/orag/internal/selfcheck"
	"github.com/shikanon/orag/internal/selfops"
)

const (
	ProtocolVersion = "2024-11-05"

	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternalError  = -32603
	codeConfigError    = -32010
	codeHTTPError      = -32020
	codeTimeoutError   = -32030
)

type Server struct {
	tools       []ToolDefinition
	byName      map[string]ToolDefinition
	client      ToolClient
	config      RuntimeConfig
	checks      selfCheckExecutor
	diagnostics diagnosticsExecutor
	ops         selfOpsExecutor
}

type ToolClient interface {
	CallTool(ctx context.Context, cfg RuntimeConfig, tool ToolDefinition, args map[string]any, meta map[string]any) (ToolResult, error)
}

type ToolResult struct {
	Payload any
	TraceID string
	Status  int
}

type selfCheckExecutor interface {
	Execute(ctx context.Context, req selfcheck.Request) (selfcheck.Envelope, error)
}

type diagnosticsExecutor interface {
	TraceLookup(ctx context.Context, req diagnostics.TraceLookupRequest) diagnostics.TraceLookupResponse
	Diagnose(req diagnostics.DiagnoseRequest) diagnostics.DiagnoseResult
	RunbookSuggest(req diagnostics.RunbookSuggestRequest) diagnostics.RunbookSuggestResponse
}

type selfOpsExecutor interface {
	Plan(ctx context.Context, req selfops.PlanRequest) (selfops.Plan, error)
	Apply(ctx context.Context, req selfops.ApplyRequest) (selfops.ApplyResult, error)
}

func NewServer(tools []ToolDefinition, cfg RuntimeConfig, client ToolClient) (*Server, error) {
	if client == nil {
		client = NewHTTPToolClient(http.DefaultClient)
	}
	byName := make(map[string]ToolDefinition, len(tools))
	for _, tool := range tools {
		if strings.TrimSpace(tool.Name) == "" {
			return nil, errors.New("tool name is required")
		}
		if _, exists := byName[tool.Name]; exists {
			return nil, fmt.Errorf("duplicate tool %q", tool.Name)
		}
		byName[tool.Name] = tool
	}
	return &Server{
		tools:       tools,
		byName:      byName,
		client:      client,
		config:      cfg,
		checks:      selfcheck.NewExecutor(selfcheck.Options{}),
		diagnostics: newDiagnosticsExecutor(cfg, client),
		ops:         selfops.NewExecutor(selfops.Options{}),
	}, nil
}

func newDiagnosticsExecutor(cfg RuntimeConfig, client ToolClient) *diagnostics.Executor {
	if getter := traceGetterFromRuntimeConfig(cfg, client); getter != nil {
		return diagnostics.NewExecutor(diagnostics.WithTraceGetter(getter))
	}
	return diagnostics.NewExecutor()
}

func traceGetterFromRuntimeConfig(cfg RuntimeConfig, client ToolClient) diagnostics.TraceGetter {
	if !runtimeConfigAvailable(cfg) {
		return nil
	}
	if _, err := joinURL(cfg.BaseURL, "/v1/traces/trace_config_probe"); err != nil {
		return nil
	}
	return NewHTTPTraceGetter(httpClientFromToolClient(client), cfg)
}

func httpClientFromToolClient(client ToolClient) *http.Client {
	httpClient, ok := client.(*HTTPToolClient)
	if !ok || httpClient == nil || httpClient.client == nil {
		return http.DefaultClient
	}
	return httpClient.client
}

func (s *Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		response, ok := s.Handle(ctx, line)
		if !ok {
			continue
		}
		if _, err := out.Write(response); err != nil {
			return err
		}
		if _, err := out.Write([]byte("\n")); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func (s *Server) Handle(ctx context.Context, data []byte) ([]byte, bool) {
	var req jsonRPCRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return mustMarshal(jsonRPCResponse{
			JSONRPC: "2.0",
			Error:   newRPCError(codeParseError, "parse_error", "invalid JSON-RPC request", nil),
		}), true
	}
	if req.JSONRPC != "2.0" || strings.TrimSpace(req.Method) == "" {
		return s.errorResponse(req.ID, codeInvalidRequest, "invalid_request", "JSON-RPC 2.0 method is required", nil), hasID(req.ID)
	}
	if !hasID(req.ID) {
		return nil, false
	}

	switch req.Method {
	case "initialize":
		return s.resultResponse(req.ID, map[string]any{
			"protocolVersion": ProtocolVersion,
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
			"serverInfo": map[string]any{
				"name":    "orag-mcp",
				"version": "0.1.0",
			},
		}), true
	case "tools/list":
		return s.resultResponse(req.ID, map[string]any{"tools": s.publicTools()}), true
	case "tools/call":
		return s.handleToolCall(ctx, req)
	default:
		return s.errorResponse(req.ID, codeMethodNotFound, "method_not_found", "method is not supported", nil), true
	}
}

func (s *Server) handleToolCall(ctx context.Context, req jsonRPCRequest) ([]byte, bool) {
	var params toolCallParams
	if len(req.Params) > 0 {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return s.errorResponse(req.ID, codeInvalidParams, "invalid_params", "tools/call params must be an object", nil), true
		}
	}
	tool, ok := s.byName[params.Name]
	if !ok {
		return s.errorResponse(req.ID, codeInvalidParams, "unknown_tool", "requested tool is not available", map[string]any{"tool": params.Name}), true
	}
	if params.Arguments == nil {
		params.Arguments = map[string]any{}
	}
	if err := validateObject(tool.InputSchema, params.Arguments); err != nil {
		return s.errorResponse(req.ID, codeInvalidParams, "invalid_tool_arguments", err.Error(), map[string]any{"tool": params.Name}), true
	}
	if params.Name == "orag_check" {
		return s.handleSelfCheck(ctx, req.ID, params), true
	}
	if isDiagnosticTool(params.Name) {
		return s.handleDiagnostics(ctx, req.ID, params), true
	}
	if isSelfOpsTool(params.Name) {
		return s.handleSelfOps(ctx, req.ID, params), true
	}

	result, err := s.client.CallTool(ctx, s.config, tool, params.Arguments, params.Meta)
	if err != nil {
		var rpcErr *RPCError
		if errors.As(err, &rpcErr) {
			return s.response(req.ID, nil, rpcErr), true
		}
		return s.errorResponse(req.ID, codeInternalError, "tool_call_failed", "tool call failed", nil), true
	}

	text, err := json.Marshal(result.Payload)
	if err != nil {
		return s.errorResponse(req.ID, codeInternalError, "marshal_result_failed", "failed to encode tool result", nil), true
	}
	mcpResult := map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": string(text)},
		},
		"structuredContent": result.Payload,
		"isError":           false,
	}
	if result.TraceID != "" {
		mcpResult["_meta"] = map[string]any{"trace_id": result.TraceID, "http_status": result.Status}
	}
	return s.resultResponse(req.ID, mcpResult), true
}

func (s *Server) handleSelfCheck(ctx context.Context, id json.RawMessage, params toolCallParams) []byte {
	checkReq, err := selfCheckRequestFromArgs(params.Arguments, params.Meta)
	if err != nil {
		return s.errorResponse(id, codeInvalidParams, "invalid_tool_arguments", err.Error(), map[string]any{"tool": params.Name})
	}
	result, err := s.checks.Execute(ctx, checkReq)
	if err != nil {
		return s.errorResponse(id, codeInvalidParams, "invalid_tool_arguments", err.Error(), map[string]any{"tool": params.Name})
	}
	text, err := json.Marshal(result)
	if err != nil {
		return s.errorResponse(id, codeInternalError, "marshal_result_failed", "failed to encode tool result", nil)
	}
	mcpResult := map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": string(text)},
		},
		"structuredContent": result,
		"isError":           result.Verdict != selfcheck.VerdictPass,
		"_meta":             map[string]any{"trace_id": result.TraceID, "exit_code": result.ExitCode},
	}
	return s.resultResponse(id, mcpResult)
}

func (s *Server) handleDiagnostics(ctx context.Context, id json.RawMessage, params toolCallParams) []byte {
	var payload any
	var verdict selfcheck.Verdict
	var traceID string
	switch params.Name {
	case "orag_trace_lookup":
		result := s.diagnostics.TraceLookup(ctx, diagnostics.TraceLookupRequest{TraceID: stringArg(params.Arguments, "trace_id")})
		payload = result
		verdict = result.Verdict
		traceID = result.TraceID
	case "orag_diagnose":
		exitCode, err := optionalIntArg(params.Arguments, "failed_command_exit_code")
		if err != nil {
			return s.errorResponse(id, codeInvalidParams, "invalid_tool_arguments", err.Error(), map[string]any{"tool": params.Name})
		}
		traceID = stringArg(params.Arguments, "trace_id")
		if traceID == "" {
			traceID = traceIDFromMeta(params.Meta)
		}
		result := s.diagnostics.Diagnose(diagnostics.DiagnoseRequest{
			Scope:                 stringArg(params.Arguments, "scope"),
			Symptom:               stringArg(params.Arguments, "symptom"),
			TraceID:               traceID,
			FailedCommand:         stringArg(params.Arguments, "failed_command"),
			FailedCommandExitCode: exitCode,
			FailedCommandOutput:   stringArg(params.Arguments, "failed_command_output"),
			AllowCommands:         boolArg(params.Arguments, "allow_commands"),
		})
		payload = result
		verdict = result.Verdict
		traceID = result.TraceID
	case "orag_runbook_suggest":
		result := s.diagnostics.RunbookSuggest(diagnostics.RunbookSuggestRequest{
			Scope:   stringArg(params.Arguments, "scope"),
			Verdict: stringArg(params.Arguments, "verdict"),
		})
		payload = result
		verdict = result.Verdict
	default:
		return s.errorResponse(id, codeInvalidParams, "unknown_tool", "requested diagnostic tool is not available", map[string]any{"tool": params.Name})
	}
	text, err := json.Marshal(payload)
	if err != nil {
		return s.errorResponse(id, codeInternalError, "marshal_result_failed", "failed to encode tool result", nil)
	}
	mcpResult := map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": string(text)},
		},
		"structuredContent": payload,
		"isError":           verdict != selfcheck.VerdictPass,
		"_meta":             map[string]any{"trace_id": traceID, "exit_code": selfcheck.ExitCode(verdict)},
	}
	return s.resultResponse(id, mcpResult)
}

func (s *Server) handleSelfOps(ctx context.Context, id json.RawMessage, params toolCallParams) []byte {
	var payload any
	var verdict string
	var traceID string
	var planID string
	var err error
	switch params.Name {
	case "orag_maintenance_plan":
		result, planErr := s.ops.Plan(ctx, selfOpsPlanRequestFromArgs(params.Arguments, params.Meta))
		payload = result
		verdict = result.Verdict
		traceID = result.TraceID
		planID = result.PlanID
		err = planErr
	case "orag_apply_low_risk_action":
		result, applyErr := s.ops.Apply(ctx, selfOpsApplyRequestFromArgs(params.Arguments, params.Meta))
		payload = result
		verdict = result.Verdict
		traceID = result.TraceID
		planID = result.PlanID
		err = applyErr
	case "orag_create_remediation_issue":
		traceID = traceIDFromMeta(params.Meta)
		if traceID == "" {
			traceID = "selfops_remediation_issue_blocked"
		}
		payload = selfops.ApplyResult{
			SchemaVersion: selfops.SchemaVersion,
			TraceID:       traceID,
			Verdict:       selfops.VerdictBlocked,
			Status:        selfops.StatusBlocked,
			BlockedReason: "remediation issue backend is not configured; create the issue manually from diagnosis findings",
		}
		verdict = selfops.VerdictBlocked
	default:
		err = fmt.Errorf("requested self-ops tool is not available")
	}
	if err != nil {
		return s.errorResponse(id, codeInvalidParams, "invalid_tool_arguments", err.Error(), map[string]any{"tool": params.Name})
	}
	text, err := json.Marshal(payload)
	if err != nil {
		return s.errorResponse(id, codeInternalError, "marshal_result_failed", "failed to encode tool result", nil)
	}
	mcpResult := map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": string(text)},
		},
		"structuredContent": payload,
		"isError":           verdict != selfops.VerdictPass,
		"_meta":             map[string]any{"trace_id": traceID, "plan_id": planID},
	}
	return s.resultResponse(id, mcpResult)
}

func (s *Server) publicTools() []ToolDefinition {
	tools := make([]ToolDefinition, 0, len(s.tools))
	for _, tool := range s.tools {
		tools = append(tools, ToolDefinition{
			Name:         tool.Name,
			Description:  tool.Description,
			InputSchema:  tool.InputSchema,
			OutputSchema: tool.OutputSchema,
			Annotations:  tool.Annotations,
		})
	}
	return tools
}

func (s *Server) resultResponse(id json.RawMessage, result any) []byte {
	return s.response(id, result, nil)
}

func (s *Server) errorResponse(id json.RawMessage, code int, kind, message string, data map[string]any) []byte {
	return s.response(id, nil, newRPCError(code, kind, message, data))
}

func (s *Server) response(id json.RawMessage, result any, rpcErr *RPCError) []byte {
	return mustMarshal(jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
		Error:   rpcErr,
	})
}

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

type RPCError struct {
	Code    int            `json:"code"`
	Message string         `json:"message"`
	Data    map[string]any `json:"data,omitempty"`
}

func (e *RPCError) Error() string {
	return e.Message
}

func newRPCError(code int, kind, message string, data map[string]any) *RPCError {
	if data == nil {
		data = map[string]any{}
	}
	data["code"] = kind
	return &RPCError{Code: code, Message: message, Data: data}
}

type toolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
	Meta      map[string]any `json:"_meta"`
}

func hasID(id json.RawMessage) bool {
	return len(bytes.TrimSpace(id)) > 0
}

func mustMarshal(value any) []byte {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return data
}

func validateObject(schema map[string]any, value map[string]any) error {
	required, _ := schema["required"].([]any)
	for _, raw := range required {
		name, ok := raw.(string)
		if ok && strings.TrimSpace(name) != "" {
			if _, exists := value[name]; !exists {
				return fmt.Errorf("missing required argument %q", name)
			}
		}
	}

	properties, _ := schema["properties"].(map[string]any)
	for name, rawValue := range value {
		rawSchema, ok := properties[name]
		if !ok {
			continue
		}
		propSchema, ok := rawSchema.(map[string]any)
		if !ok {
			continue
		}
		if err := validateValue(name, propSchema, rawValue); err != nil {
			return err
		}
	}
	return nil
}

func validateValue(name string, schema map[string]any, value any) error {
	switch schema["type"] {
	case "string":
		s, ok := value.(string)
		if !ok {
			return fmt.Errorf("argument %q must be a string", name)
		}
		if enum, ok := schema["enum"].([]any); ok && len(enum) > 0 && !enumContains(enum, s) {
			return fmt.Errorf("argument %q must be one of %v", name, enum)
		}
	case "integer":
		number, ok := numberValue(value)
		if !ok || math.Trunc(number) != number {
			return fmt.Errorf("argument %q must be an integer", name)
		}
		if min, ok := numberValue(schema["minimum"]); ok && number < min {
			return fmt.Errorf("argument %q must be >= %v", name, min)
		}
		if max, ok := numberValue(schema["maximum"]); ok && number > max {
			return fmt.Errorf("argument %q must be <= %v", name, max)
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("argument %q must be a boolean", name)
		}
	}
	return nil
}

func enumContains(enum []any, value string) bool {
	for _, item := range enum {
		if item == value {
			return true
		}
	}
	return false
}

func numberValue(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	default:
		return 0, false
	}
}

func selfCheckRequestFromArgs(args map[string]any, meta map[string]any) (selfcheck.Request, error) {
	req := selfcheck.Request{
		Scope:   selfcheck.Scope(stringArg(args, "scope")),
		Mode:    selfcheck.Mode(stringArg(args, "mode")),
		TraceID: traceIDFromMeta(meta),
	}
	var err error
	if req.OverallDeadlineSeconds, err = optionalIntArg(args, "overall_deadline_seconds"); err != nil {
		return selfcheck.Request{}, err
	}
	if req.PerCheckTimeoutSeconds, err = optionalIntArg(args, "per_check_timeout_seconds"); err != nil {
		return selfcheck.Request{}, err
	}
	return req, nil
}

func stringArg(args map[string]any, key string) string {
	value, _ := args[key].(string)
	return strings.TrimSpace(value)
}

func optionalIntArg(args map[string]any, key string) (int, error) {
	value, ok := args[key]
	if !ok || value == nil {
		return 0, nil
	}
	number, ok := numberValue(value)
	if !ok || math.Trunc(number) != number {
		return 0, fmt.Errorf("argument %q must be an integer", key)
	}
	return int(number), nil
}

func boolArg(args map[string]any, key string) bool {
	value, _ := args[key].(bool)
	return value
}

func selfOpsPlanRequestFromArgs(args map[string]any, meta map[string]any) selfops.PlanRequest {
	return selfops.PlanRequest{
		Scope:   stringArg(args, "scope"),
		DryRun:  boolArg(args, "dry_run"),
		TraceID: traceIDFromMeta(meta),
	}
}

func selfOpsApplyRequestFromArgs(args map[string]any, meta map[string]any) selfops.ApplyRequest {
	return selfops.ApplyRequest{
		PlanID:   stringArg(args, "plan_id"),
		Approved: boolArg(args, "approved"),
		TraceID:  traceIDFromMeta(meta),
	}
}

func isDiagnosticTool(name string) bool {
	switch name {
	case "orag_trace_lookup", "orag_diagnose", "orag_runbook_suggest":
		return true
	default:
		return false
	}
}

func isSelfOpsTool(name string) bool {
	switch name {
	case "orag_maintenance_plan", "orag_apply_low_risk_action", "orag_create_remediation_issue":
		return true
	default:
		return false
	}
}
