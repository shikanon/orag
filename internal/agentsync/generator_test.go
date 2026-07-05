package agentsync

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateFromOpenAPIProducesMCPAndSkillArtifacts(t *testing.T) {
	files, err := GenerateFromOpenAPI(context.Background(), filepath.Join("..", "..", "api", "openapi.yaml"))
	if err != nil {
		t.Fatalf("GenerateFromOpenAPI() error = %v", err)
	}

	byPath := map[string]GeneratedFile{}
	for _, file := range files {
		byPath[file.Path] = file
	}
	for _, path := range []string{
		".mcp/tools/ralph-loop.json",
		".codex/skills/ralph-loop/SKILL.md",
		".claude/skills/ralph-loop/SKILL.md",
		".trae/skills/ralph-loop/SKILL.md",
	} {
		if _, ok := byPath[path]; !ok {
			t.Fatalf("missing generated file %s in %#v", path, files)
		}
	}

	mcpTools := byPath[".mcp/tools/ralph-loop.json"].Content
	for _, want := range []string{
		`"manifest_extension": "x-orag-agent-capabilities"`,
		`"protocol_version": "2024-11-05"`,
		`"name": "ralph_loop_run"`,
		`"task_spec_path"`,
		`"trace_id"`,
	} {
		if !strings.Contains(mcpTools, want) {
			t.Fatalf("MCP tools artifact missing %q\n%s", want, mcpTools)
		}
	}
}

func TestWriteAndCheckFiles(t *testing.T) {
	dir := t.TempDir()
	files := []GeneratedFile{
		{Target: "mcp", Path: ".mcp/tools/ralph-loop.json", Content: "{}\n"},
		{Target: "trae", Path: ".trae/skills/ralph-loop/SKILL.md", Content: "# Trae\n"},
	}

	if err := WriteFiles(dir, files); err != nil {
		t.Fatalf("WriteFiles() error = %v", err)
	}
	if err := CheckFiles(dir, files); err != nil {
		t.Fatalf("CheckFiles() error = %v", err)
	}

	path := filepath.Join(dir, ".trae", "skills", "ralph-loop", "SKILL.md")
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
