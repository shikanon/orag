package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

const agentCapabilitiesExtension = "x-orag-agent-capabilities"

type ToolDefinition struct {
	Name         string         `json:"name"`
	Description  string         `json:"description"`
	InputSchema  map[string]any `json:"inputSchema"`
	OutputSchema map[string]any `json:"outputSchema,omitempty"`
	Annotations  map[string]any `json:"annotations,omitempty"`

	Capability Capability `json:"-"`
}

type generatedToolFile struct {
	Tools []ToolDefinition `json:"tools"`
}

func LoadToolsFromArtifacts(paths ...string) ([]ToolDefinition, error) {
	var tools []ToolDefinition
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read MCP artifact %s: %w", path, err)
		}
		var file generatedToolFile
		if err := json.Unmarshal(data, &file); err != nil {
			return nil, fmt.Errorf("decode MCP artifact %s: %w", path, err)
		}
		for _, tool := range file.Tools {
			if strings.TrimSpace(tool.Name) == "" {
				return nil, fmt.Errorf("MCP artifact %s contains tool with empty name", path)
			}
			tools = append(tools, tool)
		}
	}
	return tools, nil
}

type Capability struct {
	ID                  string
	DisplayName         string
	ToolName            string
	ToolDescription     string
	Method              string
	Path                string
	AuthRequired        bool
	AuthScheme          string
	AuthEnv             []string
	TraceRequestHeader  string
	TraceResponseHeader string
	TraceResponseField  string
	Examples            []any
}

func LoadToolsFromOpenAPI(ctx context.Context, openAPIPath string) ([]ToolDefinition, error) {
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile(openAPIPath)
	if err != nil {
		return nil, fmt.Errorf("load openapi: %w", err)
	}
	if err := doc.Validate(ctx); err != nil {
		return nil, fmt.Errorf("validate openapi: %w", err)
	}

	manifest, ok := doc.Extensions[agentCapabilitiesExtension].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("openapi missing %s extension", agentCapabilitiesExtension)
	}
	rawCapabilities, ok := manifest["capabilities"].([]any)
	if !ok || len(rawCapabilities) == 0 {
		return nil, fmt.Errorf("%s.capabilities must be a non-empty list", agentCapabilitiesExtension)
	}

	tools := make([]ToolDefinition, 0, len(rawCapabilities))
	for _, raw := range rawCapabilities {
		capability, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("capability entry has type %T, want map", raw)
		}
		tool, err := toolFromCapability(doc, capability)
		if err != nil {
			return nil, err
		}
		tools = append(tools, tool)
	}
	return tools, nil
}

func toolFromCapability(doc *openapi3.T, raw map[string]any) (ToolDefinition, error) {
	id := stringValue(raw, "id")
	if id == "" {
		return ToolDefinition{}, fmt.Errorf("capability missing id")
	}
	source := mapValue(raw, "source")
	auth := mapValue(raw, "auth")
	trace := mapValue(raw, "trace")
	mcp := mapValue(raw, "mcp")
	if len(source) == 0 || len(auth) == 0 || len(trace) == 0 || len(mcp) == 0 {
		return ToolDefinition{}, fmt.Errorf("capability %s missing source/auth/trace/mcp metadata", id)
	}

	inputSchema, err := schemaMap(doc, stringValue(mcp, "input_schema"))
	if err != nil {
		return ToolDefinition{}, fmt.Errorf("capability %s input schema: %w", id, err)
	}
	outputSchema, err := schemaMap(doc, stringValue(mcp, "output_schema"))
	if err != nil {
		return ToolDefinition{}, fmt.Errorf("capability %s output schema: %w", id, err)
	}

	cap := Capability{
		ID:                  id,
		DisplayName:         stringValue(raw, "name"),
		ToolName:            stringValue(mcp, "tool_name"),
		ToolDescription:     stringValue(mcp, "description"),
		Method:              stringValue(source, "method"),
		Path:                stringValue(source, "path"),
		AuthRequired:        boolValue(auth, "required"),
		AuthScheme:          stringValue(auth, "scheme"),
		AuthEnv:             stringSlice(auth["env"]),
		TraceRequestHeader:  stringValue(trace, "request_header"),
		TraceResponseHeader: stringValue(trace, "response_header"),
		TraceResponseField:  stringValue(trace, "response_field"),
		Examples:            anySlice(raw["examples"]),
	}
	if cap.ToolName == "" || cap.ToolDescription == "" {
		return ToolDefinition{}, fmt.Errorf("capability %s missing MCP tool metadata", id)
	}
	if cap.Method == "" {
		cap.Method = http.MethodPost
	}
	if cap.Path == "" {
		return ToolDefinition{}, fmt.Errorf("capability %s missing source.path", id)
	}

	return ToolDefinition{
		Name:         cap.ToolName,
		Description:  cap.ToolDescription,
		InputSchema:  inputSchema,
		OutputSchema: outputSchema,
		Annotations: map[string]any{
			"orag_capability_id": cap.ID,
			"orag_display_name":  cap.DisplayName,
			"auth": map[string]any{
				"required": cap.AuthRequired,
				"scheme":   cap.AuthScheme,
				"env":      cap.AuthEnv,
			},
			"trace": map[string]any{
				"request_header":  cap.TraceRequestHeader,
				"response_header": cap.TraceResponseHeader,
				"response_field":  cap.TraceResponseField,
			},
			"examples": cap.Examples,
		},
		Capability: cap,
	}, nil
}

func schemaMap(doc *openapi3.T, ref string) (map[string]any, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, fmt.Errorf("empty schema ref")
	}
	name := strings.TrimPrefix(ref, "#/components/schemas/")
	schemaRef, ok := doc.Components.Schemas[name]
	if !ok || schemaRef == nil {
		return nil, fmt.Errorf("schema %q not found", ref)
	}
	data, err := json.Marshal(schemaRef)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func mapValue(m map[string]any, key string) map[string]any {
	value, ok := m[key].(map[string]any)
	if !ok {
		return nil
	}
	return value
}

func stringValue(m map[string]any, key string) string {
	value, _ := m[key].(string)
	return strings.TrimSpace(value)
}

func boolValue(m map[string]any, key string) bool {
	value, _ := m[key].(bool)
	return value
}

func stringSlice(value any) []string {
	raw, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, strings.TrimSpace(s))
		}
	}
	return out
}

func anySlice(value any) []any {
	raw, ok := value.([]any)
	if !ok {
		return nil
	}
	return raw
}
