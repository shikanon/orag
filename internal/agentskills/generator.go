package agentskills

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

const manifestExtension = "x-orag-agent-capabilities"

// GeneratedFile is a deterministic Skill artifact rendered from the OpenAPI capability manifest.
type GeneratedFile struct {
	Target  string
	Path    string
	Content string
}

type Manifest struct {
	Version      string
	Source       string
	Capabilities []Capability
}

type Capability struct {
	ID          string
	Name        string
	Description string
	Status      string
	Source      CapabilitySource
	Auth        CapabilityAuth
	Trace       CapabilityTrace
	Schemas     CapabilitySchemas
	MCP         CapabilityMCP
	Skills      CapabilitySkills
	Examples    []CapabilityExample
}

type CapabilitySource struct {
	Kind            string
	Method          string
	Path            string
	OperationID     string
	BackingServices []string
}

type CapabilityAuth struct {
	Required bool
	Scheme   string
	Env      []string
}

type CapabilityTrace struct {
	RequestHeader  string
	ResponseHeader string
	ResponseField  string
}

type CapabilitySchemas struct {
	Input  string
	Output string
	Error  string
}

type CapabilityMCP struct {
	ToolName     string
	Description  string
	InputSchema  string
	OutputSchema string
}

type CapabilitySkills struct {
	ManifestName string
	Description  string
}

type CapabilityExample struct {
	Name           string
	Input          map[string]any
	ExpectedOutput map[string]any
}

// GenerateFromOpenAPI loads the Task 1 capability manifest and renders all Skill variants.
func GenerateFromOpenAPI(openAPIPath string) ([]GeneratedFile, error) {
	manifest, err := LoadManifest(openAPIPath)
	if err != nil {
		return nil, err
	}
	return Render(manifest)
}

// LoadManifest extracts x-orag-agent-capabilities from an OpenAPI document.
func LoadManifest(openAPIPath string) (Manifest, error) {
	doc, err := openapi3.NewLoader().LoadFromFile(openAPIPath)
	if err != nil {
		return Manifest{}, err
	}
	if err := doc.Validate(context.Background()); err != nil {
		return Manifest{}, fmt.Errorf("validate openapi: %w", err)
	}

	raw, ok := doc.Extensions[manifestExtension]
	if !ok {
		return Manifest{}, fmt.Errorf("missing %s extension", manifestExtension)
	}
	root, ok := raw.(map[string]any)
	if !ok {
		return Manifest{}, fmt.Errorf("%s has type %T, want map", manifestExtension, raw)
	}

	capabilitiesRaw, ok := root["capabilities"].([]any)
	if !ok || len(capabilitiesRaw) == 0 {
		return Manifest{}, fmt.Errorf("%s.capabilities must be a non-empty list", manifestExtension)
	}

	manifest := Manifest{
		Version: fmt.Sprint(root["version"]),
		Source:  stringValue(root, "source"),
	}
	for i, item := range capabilitiesRaw {
		capabilityMap, ok := item.(map[string]any)
		if !ok {
			return Manifest{}, fmt.Errorf("capabilities[%d] has type %T, want map", i, item)
		}
		capability, err := parseCapability(capabilityMap)
		if err != nil {
			return Manifest{}, fmt.Errorf("capability %d: %w", i, err)
		}
		manifest.Capabilities = append(manifest.Capabilities, capability)
	}
	return manifest, nil
}

func parseCapability(raw map[string]any) (Capability, error) {
	capability := Capability{
		ID:          stringValue(raw, "id"),
		Name:        stringValue(raw, "name"),
		Description: stringValue(raw, "description"),
		Status:      stringValue(raw, "status"),
	}
	if capability.ID == "" || capability.Name == "" {
		return Capability{}, fmt.Errorf("id and name are required")
	}

	source := mapValue(raw, "source")
	capability.Source = CapabilitySource{
		Kind:            stringValue(source, "kind"),
		Method:          stringValue(source, "method"),
		Path:            stringValue(source, "path"),
		OperationID:     stringValue(source, "operation_id"),
		BackingServices: stringList(source, "backing_services"),
	}
	auth := mapValue(raw, "auth")
	capability.Auth = CapabilityAuth{
		Required: boolValue(auth, "required"),
		Scheme:   stringValue(auth, "scheme"),
		Env:      stringList(auth, "env"),
	}
	trace := mapValue(raw, "trace")
	capability.Trace = CapabilityTrace{
		RequestHeader:  stringValue(trace, "request_header"),
		ResponseHeader: stringValue(trace, "response_header"),
		ResponseField:  stringValue(trace, "response_field"),
	}
	schemas := mapValue(raw, "schemas")
	capability.Schemas = CapabilitySchemas{
		Input:  stringValue(schemas, "input"),
		Output: stringValue(schemas, "output"),
		Error:  stringValue(schemas, "error"),
	}
	mcp := mapValue(raw, "mcp")
	capability.MCP = CapabilityMCP{
		ToolName:     stringValue(mcp, "tool_name"),
		Description:  stringValue(mcp, "description"),
		InputSchema:  stringValue(mcp, "input_schema"),
		OutputSchema: stringValue(mcp, "output_schema"),
	}
	skills := mapValue(raw, "skills")
	capability.Skills = CapabilitySkills{
		ManifestName: stringValue(skills, "manifest_name"),
		Description:  stringValue(skills, "description"),
	}
	if capability.Skills.ManifestName == "" {
		capability.Skills.ManifestName = capability.ID
	}

	for _, example := range listValue(raw, "examples") {
		exampleMap, ok := example.(map[string]any)
		if !ok {
			continue
		}
		capability.Examples = append(capability.Examples, CapabilityExample{
			Name:           stringValue(exampleMap, "name"),
			Input:          mapValue(exampleMap, "input"),
			ExpectedOutput: mapValue(exampleMap, "expected_output"),
		})
	}
	return capability, nil
}

// Render creates Codex, Claude Code, and Trae workspace Skill artifacts.
func Render(manifest Manifest) ([]GeneratedFile, error) {
	var files []GeneratedFile
	for _, capability := range manifest.Capabilities {
		if capability.Skills.ManifestName == "" {
			return nil, fmt.Errorf("capability %s missing skills.manifest_name", capability.ID)
		}
		files = append(files,
			GeneratedFile{
				Target:  "codex",
				Path:    filepath.ToSlash(filepath.Join(".codex", "skills", capability.Skills.ManifestName, "SKILL.md")),
				Content: renderCodexSkill(manifest, capability),
			},
			GeneratedFile{
				Target:  "claude-code",
				Path:    filepath.ToSlash(filepath.Join(".claude", "skills", capability.Skills.ManifestName, "SKILL.md")),
				Content: renderClaudeSkill(manifest, capability),
			},
			GeneratedFile{
				Target:  "trae",
				Path:    filepath.ToSlash(filepath.Join(".trae", "skills", capability.Skills.ManifestName, "SKILL.md")),
				Content: renderTraeSkill(manifest, capability),
			},
		)
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

func renderCodexSkill(manifest Manifest, capability Capability) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s Codex Skill\n\n", capability.Name)
	writeSharedSections(&b, manifest, capability, "Codex")
	fmt.Fprintf(&b, "## Codex Usage\n")
	fmt.Fprintf(&b, "- Read the task/spec file before invoking the API.\n")
	fmt.Fprintf(&b, "- Use `curl` or an equivalent HTTP client against `${ORAG_API_BASE_URL}%s`.\n", capability.Source.Path)
	fmt.Fprintf(&b, "- Return the verdict, summary, artifacts, and `%s` as evidence.\n", capability.Trace.ResponseField)
	return ensureTrailingNewline(b.String())
}

func renderClaudeSkill(manifest Manifest, capability Capability) string {
	var b strings.Builder
	fmt.Fprintf(&b, "---\n")
	fmt.Fprintf(&b, "name: %s\n", capability.Skills.ManifestName)
	fmt.Fprintf(&b, "description: %s\n", yamlScalar(capability.Skills.Description))
	fmt.Fprintf(&b, "allowed-tools: Read, Bash(curl:*)\n")
	fmt.Fprintf(&b, "---\n\n")
	fmt.Fprintf(&b, "# %s Claude Code Skill\n\n", capability.Name)
	writeSharedSections(&b, manifest, capability, "Claude Code")
	fmt.Fprintf(&b, "## Claude Code Usage\n")
	fmt.Fprintf(&b, "- Prefer `Read` for local task/spec context and `Bash(curl:*)` only for the ORAG API call.\n")
	fmt.Fprintf(&b, "- Do not modify repository files unless the user explicitly asks for implementation work.\n")
	return ensureTrailingNewline(b.String())
}

func renderTraeSkill(manifest Manifest, capability Capability) string {
	var b strings.Builder
	fmt.Fprintf(&b, "---\n")
	fmt.Fprintf(&b, "name: %s\n", capability.Skills.ManifestName)
	fmt.Fprintf(&b, "description: %s\n", yamlScalar(capability.Skills.Description))
	fmt.Fprintf(&b, "---\n\n")
	fmt.Fprintf(&b, "# %s Trae Skill\n\n", capability.Name)
	writeSharedSections(&b, manifest, capability, "Trae")
	fmt.Fprintf(&b, "## Trae Usage\n")
	fmt.Fprintf(&b, "- Invoke this Skill when the user asks to run Ralph Loop verification for an ORAG task/spec.\n")
	fmt.Fprintf(&b, "- Keep the run bounded with `max_rounds`; ask the user before exceeding the requested scope.\n")
	return ensureTrailingNewline(b.String())
}

func writeSharedSections(b *strings.Builder, manifest Manifest, capability Capability, target string) {
	fmt.Fprintf(b, "Generated from `%s` version `%s` for %s.\n\n", manifestExtension, manifest.Version, target)
	fmt.Fprintf(b, "## Purpose\n")
	fmt.Fprintf(b, "%s\n\n", capability.Skills.Description)
	fmt.Fprintf(b, "## Trigger Conditions\n")
	fmt.Fprintf(b, "- Use when an agent needs a bounded ORAG Ralph Loop verification run from a task/spec path.\n")
	fmt.Fprintf(b, "- Use when the expected answer must include a PASS/FAIL verdict, artifacts, and trace evidence.\n")
	fmt.Fprintf(b, "- Do not use for general RAG queries, ingestion, or unbounded autonomous code changes.\n\n")
	fmt.Fprintf(b, "## Parameters\n")
	fmt.Fprintf(b, "- API endpoint: `%s %s`\n", capability.Source.Method, capability.Source.Path)
	fmt.Fprintf(b, "- Operation ID: `%s`\n", capability.Source.OperationID)
	fmt.Fprintf(b, "- MCP tool: `%s`\n", capability.MCP.ToolName)
	fmt.Fprintf(b, "- Input schema: `%s`\n", capability.Schemas.Input)
	fmt.Fprintf(b, "- Output schema: `%s`\n", capability.Schemas.Output)
	fmt.Fprintf(b, "- Error schema: `%s`\n\n", capability.Schemas.Error)
	fmt.Fprintf(b, "## Environment\n")
	for _, env := range capability.Auth.Env {
		fmt.Fprintf(b, "- `%s`\n", env)
	}
	fmt.Fprintf(b, "\n")
	fmt.Fprintf(b, "## Call Steps\n")
	fmt.Fprintf(b, "1. Confirm `ORAG_API_BASE_URL`, `ORAG_API_TOKEN`, and `ORAG_TENANT_ID` are available.\n")
	fmt.Fprintf(b, "2. Build a request body with `task_spec_path`, `task_id`, `mode`, and `max_rounds`.\n")
	fmt.Fprintf(b, "3. Send `Authorization: Bearer ${ORAG_API_TOKEN}`, `X-ORAG-Tenant-ID: ${ORAG_TENANT_ID}`, and optional `%s`.\n", capability.Trace.RequestHeader)
	fmt.Fprintf(b, "4. Report `status`, `verdict`, `summary`, `artifacts`, and `%s` from the response.\n\n", capability.Trace.ResponseField)
	writeExample(b, capability)
	fmt.Fprintf(b, "## Safety Boundaries\n")
	fmt.Fprintf(b, "- Treat this Skill as an API client description; it does not implement the Ralph Loop runtime handler.\n")
	fmt.Fprintf(b, "- Never print bearer tokens, tenant secrets, or full request headers in the final answer.\n")
	fmt.Fprintf(b, "- Stop and surface the API error when `%s` or HTTP status indicates failure.\n\n", capability.Schemas.Error)
}

func writeExample(b *strings.Builder, capability Capability) {
	if len(capability.Examples) == 0 {
		return
	}
	example := capability.Examples[0]
	input := marshalJSON(example.Input)
	output := marshalJSON(example.ExpectedOutput)
	fmt.Fprintf(b, "## Example Prompt\n")
	fmt.Fprintf(b, "Run Ralph Loop for `%s` in `%s` mode with at most `%v` round(s), then report the verdict and trace ID.\n\n",
		example.Input["task_id"], example.Input["mode"], example.Input["max_rounds"])
	fmt.Fprintf(b, "## Example Request\n")
	fmt.Fprintf(b, "```json\n%s\n```\n\n", input)
	fmt.Fprintf(b, "## Expected Output Shape\n")
	fmt.Fprintf(b, "```json\n%s\n```\n\n", output)
}

func marshalJSON(value map[string]any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(value); err != nil {
		return "{}"
	}
	return strings.TrimRight(buf.String(), "\n")
}

func mapValue(raw map[string]any, key string) map[string]any {
	value, ok := raw[key].(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return value
}

func listValue(raw map[string]any, key string) []any {
	value, ok := raw[key].([]any)
	if !ok {
		return nil
	}
	return value
}

func stringValue(raw map[string]any, key string) string {
	value, ok := raw[key]
	if !ok || value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func boolValue(raw map[string]any, key string) bool {
	value, ok := raw[key].(bool)
	return ok && value
}

func stringList(raw map[string]any, key string) []string {
	items := listValue(raw, key)
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, fmt.Sprint(item))
	}
	return result
}

func yamlScalar(value string) string {
	escaped := strings.ReplaceAll(value, `"`, `\"`)
	return `"` + escaped + `"`
}

func ensureTrailingNewline(value string) string {
	return strings.TrimRight(value, "\n") + "\n"
}
