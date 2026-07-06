package agentsync

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/shikanon/orag/internal/agentskills"
	"github.com/shikanon/orag/internal/capabilities"
	"github.com/shikanon/orag/internal/mcp"
)

const openAPIFacetPath = ".mcp/openapi-facet.json"

// GeneratedFile is a deterministic artifact rendered from the capability manifest.
type GeneratedFile struct {
	Target  string
	Path    string
	Content string
}

// GenerateFromOpenAPI keeps the old command/API surface while using the builtin
// capability manifest as the source of truth.
func GenerateFromOpenAPI(ctx context.Context, openAPIPath string) ([]GeneratedFile, error) {
	_ = ctx
	if strings.TrimSpace(openAPIPath) != "" {
		if _, err := os.Stat(openAPIPath); err != nil {
			return nil, err
		}
	}
	return GenerateFromManifest(capabilities.MustBuiltinManifest())
}

// GenerateFromManifest renders MCP tool schemas, Skill artifacts, and the
// static OpenAPI facet check artifact from a capability manifest.
func GenerateFromManifest(manifest capabilities.Manifest) ([]GeneratedFile, error) {
	if err := capabilities.Validate(manifest); err != nil {
		return nil, err
	}
	mcpFiles, err := renderMCPToolFiles(manifest)
	if err != nil {
		return nil, err
	}
	files := append([]GeneratedFile{}, mcpFiles...)
	facet, err := renderOpenAPIFacet(manifest)
	if err != nil {
		return nil, err
	}
	files = append(files, GeneratedFile{Target: "openapi-facet", Path: openAPIFacetPath, Content: facet})

	skillFiles, err := agentskills.GenerateFromManifest(manifest)
	if err != nil {
		return nil, err
	}
	for _, file := range skillFiles {
		files = append(files, GeneratedFile{Target: file.Target, Path: file.Path, Content: file.Content})
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].Target == files[j].Target {
			return files[i].Path < files[j].Path
		}
		return files[i].Target < files[j].Target
	})
	return files, nil
}

// WriteFiles writes generated artifacts below outputDir.
func WriteFiles(outputDir string, files []GeneratedFile) error {
	for _, file := range files {
		targetPath := filepath.Join(outputDir, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(targetPath, []byte(file.Content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// CheckFiles verifies generated artifacts match files on disk without modifying them.
func CheckFiles(outputDir string, files []GeneratedFile) error {
	var drifted []string
	for _, file := range files {
		targetPath := filepath.Join(outputDir, filepath.FromSlash(file.Path))
		body, err := os.ReadFile(targetPath)
		if err != nil {
			drifted = append(drifted, fmt.Sprintf("%s: %v", file.Path, err))
			continue
		}
		if string(body) != file.Content {
			drifted = append(drifted, fmt.Sprintf("%s: generated content differs", file.Path))
		}
	}
	if len(drifted) > 0 {
		return DriftError{Files: drifted}
	}
	return nil
}

// DriftError reports files that are missing or no longer match generated output.
type DriftError struct {
	Files []string
}

func (e DriftError) Error() string {
	return fmt.Sprintf("agent artifacts are out of sync: %v", e.Files)
}

type mcpToolsDocument struct {
	GeneratedFrom      string               `json:"generated_from"`
	SchemaVersion      string               `json:"schema_version"`
	CapabilityVersion  string               `json:"capability_version"`
	GeneratorVersion   string               `json:"generator_version"`
	ProtocolVersion    string               `json:"protocol_version"`
	RuntimeGateWarning string               `json:"runtime_gate_warning"`
	Tools              []mcp.ToolDefinition `json:"tools"`
}

func renderMCPToolFiles(manifest capabilities.Manifest) ([]GeneratedFile, error) {
	byPath := map[string][]mcp.ToolDefinition{}
	for _, capability := range manifest.Capabilities {
		tool, err := toolFromCapability(capability)
		if err != nil {
			return nil, err
		}
		path := strings.TrimSpace(capability.Generation.MCPArtifact)
		if path == "" {
			path = filepath.ToSlash(filepath.Join(".mcp", "tools", capability.Skill.ManifestName+".json"))
		}
		byPath[path] = append(byPath[path], tool)
	}
	paths := make([]string, 0, len(byPath))
	for path := range byPath {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	files := make([]GeneratedFile, 0, len(paths))
	for _, path := range paths {
		tools := byPath[path]
		sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
		doc := mcpToolsDocument{
			GeneratedFrom:      capabilities.SchemaVersion,
			SchemaVersion:      manifest.SchemaVersion,
			CapabilityVersion:  manifest.CapabilityVersion,
			GeneratorVersion:   manifest.GeneratorVersion,
			ProtocolVersion:    mcp.ProtocolVersion,
			RuntimeGateWarning: "Static make agent-sync-check is the authoritative drift gate; runtime MCP probes are convenience checks only.",
			Tools:              tools,
		}
		content, err := marshalIndented(doc)
		if err != nil {
			return nil, err
		}
		files = append(files, GeneratedFile{Target: "mcp", Path: path, Content: content})
	}
	return files, nil
}

func toolFromCapability(capability capabilities.Capability) (mcp.ToolDefinition, error) {
	if strings.TrimSpace(capability.MCP.ToolName) == "" {
		return mcp.ToolDefinition{}, fmt.Errorf("capability %s missing MCP tool name", capability.ID)
	}
	return mcp.ToolDefinition{
		Name:         capability.MCP.ToolName,
		Description:  capability.MCP.Description,
		InputSchema:  schemaForCapability(capability, true),
		OutputSchema: schemaForCapability(capability, false),
		Annotations:  annotationsForCapability(capability),
		Capability: mcp.Capability{
			ID:                  capability.ID,
			DisplayName:         capability.DisplayName,
			ToolName:            capability.MCP.ToolName,
			ToolDescription:     capability.MCP.Description,
			Method:              capability.HTTP.Method,
			Path:                capability.HTTP.Path,
			AuthRequired:        capability.HTTP.AuthScheme != "",
			AuthScheme:          capability.HTTP.AuthScheme,
			AuthEnv:             []string{"ORAG_API_BASE_URL", "ORAG_API_TOKEN", "ORAG_TENANT_ID"},
			TraceRequestHeader:  traceString(capability.MCP.Annotations, "request_header"),
			TraceResponseHeader: traceString(capability.MCP.Annotations, "response_header"),
			TraceResponseField:  traceString(capability.MCP.Annotations, "response_field"),
		},
	}, nil
}

func annotationsForCapability(capability capabilities.Capability) map[string]any {
	annotations := map[string]any{}
	for k, v := range capability.MCP.Annotations {
		annotations[k] = v
	}
	annotations["auth"] = map[string]any{
		"required": capability.HTTP.AuthScheme != "",
		"scheme":   capability.HTTP.AuthScheme,
		"env":      []string{"ORAG_API_BASE_URL", "ORAG_API_TOKEN", "ORAG_TENANT_ID"},
	}
	annotations["risk_level"] = capability.RiskLevel
	annotations["operations"] = capability.Operations
	annotations["skill_manifest_name"] = capability.Skill.ManifestName
	annotations["runtime_gate_warning"] = "Runtime probes do not replace static make agent-sync-check."
	annotations["examples"] = capability.Examples
	return annotations
}

func schemaForCapability(capability capabilities.Capability, input bool) map[string]any {
	ref := capability.HTTP.ResponseSchema
	example := map[string]any{}
	description := capability.Description
	if input {
		ref = capability.HTTP.RequestSchema
		if len(capability.Examples) > 0 {
			example = capability.Examples[0].Input
		}
		description = "Input contract for " + capability.DisplayName + "."
	} else if len(capability.Examples) > 0 {
		example = capability.Examples[0].ExpectedOutput
		description = "Output contract for " + capability.DisplayName + "."
	}
	if capability.ID == "ralph-loop" && input {
		return ralphLoopInputSchema(ref)
	}
	if capability.ID == "self-check" && input {
		return selfCheckInputSchema(ref)
	}
	return schemaFromExample(ref, description, example)
}

func ralphLoopInputSchema(ref string) map[string]any {
	return map[string]any{
		"type":              "object",
		"description":       "Input contract for the Ralph Loop agent capability.",
		"x-orag-schema-ref": ref,
		"required":          []any{"task_spec_path", "task_id", "mode", "max_rounds"},
		"properties": map[string]any{
			"task_spec_path": map[string]any{"type": "string", "description": "Repository-relative path to the spec tasks file or task directory."},
			"task_id":        map[string]any{"type": "string", "description": "Task label to verify, for example Task 1."},
			"mode":           map[string]any{"type": "string", "enum": []any{"focused", "broad"}},
			"max_rounds":     map[string]any{"type": "integer", "minimum": 1, "maximum": 20},
			"base_ref":       map[string]any{"type": "string"},
			"notes":          map[string]any{"type": "string"},
		},
	}
}

func selfCheckInputSchema(ref string) map[string]any {
	return map[string]any{
		"type":              "object",
		"description":       "Input contract for read-only ORAG self-checks.",
		"x-orag-schema-ref": ref,
		"required":          []any{"scope", "mode"},
		"properties": map[string]any{
			"scope":                     map[string]any{"type": "string", "enum": []any{"health", "contract", "agent_sync", "smoke", "storage", "config", "release", "all"}},
			"mode":                      map[string]any{"type": "string", "enum": []any{"focused", "broad"}},
			"overall_deadline_seconds":  map[string]any{"type": "integer", "minimum": 1},
			"per_check_timeout_seconds": map[string]any{"type": "integer", "minimum": 1},
		},
	}
}

func schemaFromExample(ref, description string, example map[string]any) map[string]any {
	properties := map[string]any{}
	keys := make([]string, 0, len(example))
	for key := range example {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		properties[key] = map[string]any{"type": jsonType(example[key])}
	}
	return map[string]any{
		"type":              "object",
		"description":       description,
		"x-orag-schema-ref": ref,
		"properties":        properties,
	}
}

func jsonType(value any) string {
	switch value.(type) {
	case bool:
		return "boolean"
	case int, int32, int64, float32, float64:
		return "integer"
	case []any, []string:
		return "array"
	case map[string]any:
		return "object"
	default:
		return "string"
	}
}

type openAPIFacetDocument struct {
	GeneratedFrom     string         `json:"generated_from"`
	SchemaVersion     string         `json:"schema_version"`
	CapabilityVersion string         `json:"capability_version"`
	GeneratorVersion  string         `json:"generator_version"`
	OpenAPIFacetPath  string         `json:"openapi_facet_path"`
	Capabilities      []openAPIFacet `json:"capabilities"`
}

type openAPIFacet struct {
	ID              string   `json:"id"`
	Status          string   `json:"status"`
	Method          string   `json:"method"`
	Path            string   `json:"path"`
	OperationID     string   `json:"operation_id"`
	AuthScheme      string   `json:"auth_scheme"`
	RequestSchema   string   `json:"request_schema"`
	ResponseSchema  string   `json:"response_schema"`
	ErrorSchema     string   `json:"error_schema"`
	BackingServices []string `json:"backing_services"`
}

func renderOpenAPIFacet(manifest capabilities.Manifest) (string, error) {
	doc := openAPIFacetDocument{
		GeneratedFrom:     capabilities.SchemaVersion,
		SchemaVersion:     manifest.SchemaVersion,
		CapabilityVersion: manifest.CapabilityVersion,
		GeneratorVersion:  manifest.GeneratorVersion,
		OpenAPIFacetPath:  manifest.Generation.OpenAPIFacetPath,
	}
	for _, capability := range manifest.Capabilities {
		doc.Capabilities = append(doc.Capabilities, openAPIFacet{
			ID:              capability.ID,
			Status:          capability.Status,
			Method:          capability.HTTP.Method,
			Path:            capability.HTTP.Path,
			OperationID:     capability.HTTP.OperationID,
			AuthScheme:      capability.HTTP.AuthScheme,
			RequestSchema:   capability.HTTP.RequestSchema,
			ResponseSchema:  capability.HTTP.ResponseSchema,
			ErrorSchema:     capability.HTTP.ErrorSchema,
			BackingServices: capability.HTTP.BackingServices,
		})
	}
	sort.Slice(doc.Capabilities, func(i, j int) bool { return doc.Capabilities[i].ID < doc.Capabilities[j].ID })
	return marshalIndented(doc)
}

func traceString(annotations map[string]any, key string) string {
	trace, _ := annotations["trace"].(map[string]any)
	value, _ := trace[key].(string)
	return value
}

func marshalIndented(value any) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(value); err != nil {
		return "", err
	}
	return buf.String(), nil
}
