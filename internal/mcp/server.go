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
	tools  []ToolDefinition
	byName map[string]ToolDefinition
	client ToolClient
	config RuntimeConfig
}

type ToolClient interface {
	CallTool(ctx context.Context, cfg RuntimeConfig, tool ToolDefinition, args map[string]any, meta map[string]any) (ToolResult, error)
}

type ToolResult struct {
	Payload any
	TraceID string
	Status  int
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
	return &Server{tools: tools, byName: byName, client: client, config: cfg}, nil
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
