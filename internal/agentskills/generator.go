package agentskills

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/shikanon/orag/internal/capabilities"
)

// GeneratedFile is a deterministic Skill artifact rendered from the capability manifest.
type GeneratedFile struct {
	Target  string
	Path    string
	Content string
}

// GenerateFromOpenAPI is kept as a compatibility wrapper for older callers.
// Skill behavior is no longer parsed from OpenAPI; the builtin capability
// manifest is the source of truth.
func GenerateFromOpenAPI(openAPIPath string) ([]GeneratedFile, error) {
	if strings.TrimSpace(openAPIPath) != "" {
		if _, err := os.Stat(openAPIPath); err != nil {
			return nil, err
		}
	}
	return GenerateFromManifest(capabilities.MustBuiltinManifest())
}

// GenerateFromManifest renders Codex, Claude Code, and Trae Skill artifacts.
func GenerateFromManifest(manifest capabilities.Manifest) ([]GeneratedFile, error) {
	if err := capabilities.Validate(manifest); err != nil {
		return nil, err
	}
	return Render(manifest)
}

type skillBundle struct {
	Name         string
	Description  string
	Behavior     capabilities.SkillBehavior
	Capabilities []capabilities.Capability
}

// Render creates one Skill per manifest_name per target. Multiple MCP tools can
// intentionally share one Skill when their trigger boundary is the same.
func Render(manifest capabilities.Manifest) ([]GeneratedFile, error) {
	bundles, err := groupSkillBundles(manifest)
	if err != nil {
		return nil, err
	}
	var files []GeneratedFile
	for _, bundle := range bundles {
		files = append(files,
			GeneratedFile{
				Target:  "codex",
				Path:    filepath.ToSlash(filepath.Join("agent", "skills", "codex", bundle.Name, "SKILL.md")),
				Content: renderCodexSkill(manifest, bundle),
			},
			GeneratedFile{
				Target:  "claude-code",
				Path:    filepath.ToSlash(filepath.Join("agent", "skills", "claude-code", bundle.Name, "SKILL.md")),
				Content: renderClaudeSkill(manifest, bundle),
			},
			GeneratedFile{
				Target:  "trae",
				Path:    filepath.ToSlash(filepath.Join("agent", "skills", "trae", bundle.Name, "SKILL.md")),
				Content: renderTraeSkill(manifest, bundle),
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

func groupSkillBundles(manifest capabilities.Manifest) ([]skillBundle, error) {
	byName := map[string]*skillBundle{}
	for _, capability := range manifest.Capabilities {
		name := strings.TrimSpace(capability.Skill.ManifestName)
		if name == "" {
			return nil, fmt.Errorf("capability %s missing skill.manifest_name", capability.ID)
		}
		bundle := byName[name]
		if bundle == nil {
			bundle = &skillBundle{
				Name:        name,
				Description: capability.Skill.Description,
				Behavior:    capability.Skill,
			}
			byName[name] = bundle
		}
		if bundle.Behavior.MutualExclusionKey != capability.Skill.MutualExclusionKey {
			return nil, fmt.Errorf("skill %s mixes mutual exclusion keys %q and %q", name, bundle.Behavior.MutualExclusionKey, capability.Skill.MutualExclusionKey)
		}
		bundle.Capabilities = append(bundle.Capabilities, capability)
	}
	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)
	bundles := make([]skillBundle, 0, len(names))
	for _, name := range names {
		bundle := byName[name]
		sort.Slice(bundle.Capabilities, func(i, j int) bool {
			return bundle.Capabilities[i].ID < bundle.Capabilities[j].ID
		})
		bundles = append(bundles, *bundle)
	}
	return bundles, nil
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

func renderCodexSkill(manifest capabilities.Manifest, bundle skillBundle) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s Codex Skill\n\n", skillTitle(bundle))
	writeSharedSections(&b, manifest, bundle, "Codex")
	fmt.Fprintf(&b, "## Codex Usage\n")
	fmt.Fprintf(&b, "- Read local task/spec or evidence files before invoking ORAG tools.\n")
	fmt.Fprintf(&b, "- Prefer MCP tools when available; use HTTP only when the matching `%s` facet is implemented.\n", manifest.Generation.OpenAPIFacetPath)
	fmt.Fprintf(&b, "- Return verdict, summary, artifacts, and `trace_id` when present.\n")
	return ensureTrailingNewline(b.String())
}

func renderClaudeSkill(manifest capabilities.Manifest, bundle skillBundle) string {
	var b strings.Builder
	fmt.Fprintf(&b, "---\n")
	fmt.Fprintf(&b, "name: %s\n", bundle.Name)
	fmt.Fprintf(&b, "description: %s\n", yamlScalar(bundle.Description))
	fmt.Fprintf(&b, "allowed-tools: Read, Bash(curl:*)\n")
	fmt.Fprintf(&b, "---\n\n")
	fmt.Fprintf(&b, "# %s Claude Code Skill\n\n", skillTitle(bundle))
	writeSharedSections(&b, manifest, bundle, "Claude Code")
	fmt.Fprintf(&b, "## Claude Code Usage\n")
	fmt.Fprintf(&b, "- Prefer `Read` for local context and MCP/HTTP calls only for the listed ORAG capabilities.\n")
	fmt.Fprintf(&b, "- Do not modify repository files unless the user explicitly asks for implementation work.\n")
	return ensureTrailingNewline(b.String())
}

func renderTraeSkill(manifest capabilities.Manifest, bundle skillBundle) string {
	var b strings.Builder
	fmt.Fprintf(&b, "---\n")
	fmt.Fprintf(&b, "name: %s\n", bundle.Name)
	fmt.Fprintf(&b, "description: %s\n", yamlScalar(bundle.Description))
	fmt.Fprintf(&b, "---\n\n")
	fmt.Fprintf(&b, "# %s Trae Skill\n\n", skillTitle(bundle))
	writeSharedSections(&b, manifest, bundle, "Trae")
	fmt.Fprintf(&b, "## Trae Usage\n")
	fmt.Fprintf(&b, "- Invoke this Skill only when the user request matches the trigger conditions.\n")
	fmt.Fprintf(&b, "- Keep actions inside the listed safety boundaries and ask before expanding scope.\n")
	return ensureTrailingNewline(b.String())
}

func writeSharedSections(b *strings.Builder, manifest capabilities.Manifest, bundle skillBundle, target string) {
	fmt.Fprintf(b, "Generated from `%s` version `%s` with generator `%s` for %s.\n\n", capabilities.SchemaVersion, manifest.CapabilityVersion, manifest.GeneratorVersion, target)
	fmt.Fprintf(b, "## Purpose\n")
	fmt.Fprintf(b, "%s\n\n", bundle.Description)
	fmt.Fprintf(b, "## Trigger Conditions\n")
	writeBullets(b, bundle.Behavior.TriggerConditions)
	fmt.Fprintf(b, "\n## Anti-Triggers\n")
	writeBullets(b, bundle.Behavior.AntiTriggers)
	fmt.Fprintf(b, "\n## Mutual Exclusion\n")
	fmt.Fprintf(b, "- Key: `%s`\n", bundle.Behavior.MutualExclusionKey)
	if bundle.Behavior.MutualExclusionNote != "" {
		fmt.Fprintf(b, "- %s\n", bundle.Behavior.MutualExclusionNote)
	}
	fmt.Fprintf(b, "\n## Capabilities\n")
	for _, capability := range bundle.Capabilities {
		fmt.Fprintf(b, "- `%s`: `%s` via `%s %s`, input `%s`, output `%s`, risk `%s`, side effect `%s`\n",
			capability.MCP.ToolName,
			capability.ID,
			capability.HTTP.Method,
			capability.HTTP.Path,
			capability.HTTP.RequestSchema,
			capability.HTTP.ResponseSchema,
			capability.RiskLevel,
			capability.Operations.SideEffect,
		)
	}
	fmt.Fprintf(b, "\n## Environment\n")
	fmt.Fprintf(b, "- `ORAG_API_BASE_URL`\n")
	fmt.Fprintf(b, "- `ORAG_API_TOKEN`\n")
	fmt.Fprintf(b, "- `ORAG_TENANT_ID`\n")
	fmt.Fprintf(b, "\n## Call Steps\n")
	writeNumbered(b, bundle.Behavior.CallOrder)
	writeExamples(b, bundle)
	fmt.Fprintf(b, "## Safety Boundaries\n")
	writeBullets(b, bundle.Behavior.SafetyBoundaries)
	fmt.Fprintf(b, "\n## Failure Handling\n")
	writeBullets(b, bundle.Behavior.FailureHandling)
	fmt.Fprintf(b, "\n")
}

func writeBullets(b *strings.Builder, items []string) {
	for _, item := range items {
		fmt.Fprintf(b, "- %s\n", item)
	}
}

func writeNumbered(b *strings.Builder, items []string) {
	for i, item := range items {
		fmt.Fprintf(b, "%d. %s\n", i+1, item)
	}
	fmt.Fprintf(b, "\n")
}

func writeExamples(b *strings.Builder, bundle skillBundle) {
	fmt.Fprintf(b, "## Example Prompts\n")
	writeBullets(b, bundle.Behavior.ExamplePrompts)
	for _, capability := range bundle.Capabilities {
		if len(capability.Examples) == 0 {
			continue
		}
		example := capability.Examples[0]
		fmt.Fprintf(b, "\n## Example Request: `%s`\n", capability.MCP.ToolName)
		fmt.Fprintf(b, "%s\n\n", example.Prompt)
		fmt.Fprintf(b, "```json\n%s\n```\n\n", marshalJSON(example.Input))
		fmt.Fprintf(b, "## Expected Output Shape: `%s`\n", capability.MCP.ToolName)
		fmt.Fprintf(b, "```json\n%s\n```\n\n", marshalJSON(example.ExpectedOutput))
	}
}

func skillTitle(bundle skillBundle) string {
	if len(bundle.Capabilities) == 1 {
		return bundle.Capabilities[0].DisplayName
	}
	return strings.TrimPrefix(strings.ReplaceAll(strings.Title(strings.ReplaceAll(bundle.Name, "-", " ")), "Orag", "ORAG"), "")
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

func yamlScalar(value string) string {
	escaped := strings.ReplaceAll(value, `"`, `\"`)
	return `"` + escaped + `"`
}

func ensureTrailingNewline(value string) string {
	return strings.TrimRight(value, "\n") + "\n"
}
