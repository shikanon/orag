package agentsync

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shikanon/orag/internal/capabilities"
)

func TestGenerateFromManifestProducesMCPAndSkillArtifacts(t *testing.T) {
	files, err := GenerateFromManifest(capabilities.MustBuiltinManifest())
	if err != nil {
		t.Fatalf("GenerateFromManifest() error = %v", err)
	}

	byPath := map[string]GeneratedFile{}
	for _, file := range files {
		byPath[file.Path] = file
	}
	for _, path := range []string{
		".mcp/openapi-facet.json",
		".mcp/tools/ralph-loop.json",
		".mcp/tools/orag-self-check.json",
		".mcp/tools/orag-self-diagnose.json",
		".mcp/tools/orag-self-ops.json",
		".codex/skills/ralph-loop/SKILL.md",
		".claude/skills/ralph-loop/SKILL.md",
		".trae/skills/ralph-loop/SKILL.md",
		".codex/skills/orag-self-check/SKILL.md",
		".claude/skills/orag-self-diagnose/SKILL.md",
		".trae/skills/orag-self-ops/SKILL.md",
	} {
		if _, ok := byPath[path]; !ok {
			t.Fatalf("missing generated file %s in %#v", path, files)
		}
	}

	mcpTools := byPath[".mcp/tools/orag-self-diagnose.json"].Content
	for _, want := range []string{
		`"schema_version": "orag.capabilities.v1"`,
		`"protocol_version": "2024-11-05"`,
		`"name": "orag_diagnose"`,
		`"name": "orag_trace_lookup"`,
		`"name": "orag_runbook_suggest"`,
		`"runtime_gate_warning"`,
	} {
		if !strings.Contains(mcpTools, want) {
			t.Fatalf("MCP tools artifact missing %q\n%s", want, mcpTools)
		}
	}

	facet := byPath[".mcp/openapi-facet.json"].Content
	for _, want := range []string{
		`"id": "self-check"`,
		`"path": "/v1/self-check"`,
		`"request_schema": "#/components/schemas/SelfCheckRequest"`,
	} {
		if !strings.Contains(facet, want) {
			t.Fatalf("OpenAPI facet artifact missing %q\n%s", want, facet)
		}
	}
}

func TestGenerateFromOpenAPICompatibilityWrapperUsesManifest(t *testing.T) {
	files, err := GenerateFromOpenAPI(context.Background(), filepath.Join("..", "..", "api", "openapi.yaml"))
	if err != nil {
		t.Fatalf("GenerateFromOpenAPI() error = %v", err)
	}
	var found bool
	for _, file := range files {
		if file.Path == ".mcp/tools/orag-self-check.json" && strings.Contains(file.Content, `"name": "orag_check"`) {
			found = true
		}
	}
	if !found {
		t.Fatalf("compatibility wrapper did not generate manifest self-check tool: %#v", files)
	}
}

func TestWriteAndCheckFilesDetectsStaticDrift(t *testing.T) {
	dir := t.TempDir()
	files := []GeneratedFile{
		{Target: "mcp", Path: ".mcp/tools/ralph-loop.json", Content: "{}\n"},
		{Target: "openapi-facet", Path: ".mcp/openapi-facet.json", Content: "{}\n"},
		{Target: "trae", Path: ".trae/skills/orag-self-check/SKILL.md", Content: "# Trae\n"},
	}

	if err := WriteFiles(dir, files); err != nil {
		t.Fatalf("WriteFiles() error = %v", err)
	}
	if err := CheckFiles(dir, files); err != nil {
		t.Fatalf("CheckFiles() error = %v", err)
	}

	path := filepath.Join(dir, ".trae", "skills", "orag-self-check", "SKILL.md")
	if err := os.WriteFile(path, []byte("# stale\n"), 0o644); err != nil {
		t.Fatalf("write stale file: %v", err)
	}
	err := CheckFiles(dir, files)
	if err == nil {
		t.Fatal("CheckFiles() error = nil, want drift")
	}
	if !strings.Contains(err.Error(), "generated content differs") {
		t.Fatalf("CheckFiles() error = %q, want generated content differs", err.Error())
	}
}
