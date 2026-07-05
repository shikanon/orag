package agentskills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateFromOpenAPIProducesAgentSkillTargets(t *testing.T) {
	files, err := GenerateFromOpenAPI(filepath.Join("..", "..", "api", "openapi.yaml"))
	if err != nil {
		t.Fatalf("GenerateFromOpenAPI() error = %v", err)
	}

	byPath := map[string]GeneratedFile{}
	for _, file := range files {
		byPath[file.Path] = file
	}
	for _, path := range []string{
		".codex/skills/ralph-loop/SKILL.md",
		".claude/skills/ralph-loop/SKILL.md",
		".trae/skills/ralph-loop/SKILL.md",
	} {
		if _, ok := byPath[path]; !ok {
			t.Fatalf("missing generated file %s in %#v", path, files)
		}
	}

	for _, file := range files {
		for _, want := range []string{
			"Generated from `x-orag-agent-capabilities` version `1`",
			"ORAG_API_BASE_URL",
			"ORAG_API_TOKEN",
			"ORAG_TENANT_ID",
			"`X-ORAG-Tenant-ID: ${ORAG_TENANT_ID}`",
			"`POST /v1/ralph-loop`",
			"`ralph_loop_run`",
			"`#/components/schemas/RalphLoopRequest`",
			"`#/components/schemas/RalphLoopResponse`",
			"Never print bearer tokens",
			"trace_id",
			"Task 1",
		} {
			if !strings.Contains(file.Content, want) {
				t.Fatalf("%s missing %q\n%s", file.Path, want, file.Content)
			}
		}
		if strings.Contains(file.Content, "X-Tenant-ID") {
			t.Fatalf("%s contains deprecated tenant header\n%s", file.Path, file.Content)
		}
	}

	claude := byPath[".claude/skills/ralph-loop/SKILL.md"].Content
	for _, want := range []string{
		"---\nname: ralph-loop\n",
		"allowed-tools: Read, Bash(curl:*)",
		"# Ralph Loop Claude Code Skill",
	} {
		if !strings.Contains(claude, want) {
			t.Fatalf("Claude Skill missing %q\n%s", want, claude)
		}
	}

	trae := byPath[".trae/skills/ralph-loop/SKILL.md"].Content
	for _, want := range []string{
		"---\nname: ralph-loop\n",
		"# Ralph Loop Trae Skill",
		"Invoke this Skill when the user asks to run Ralph Loop verification",
	} {
		if !strings.Contains(trae, want) {
			t.Fatalf("Trae Skill missing %q\n%s", want, trae)
		}
	}
}

func TestWriteFilesCreatesSkillDirectories(t *testing.T) {
	dir := t.TempDir()
	files := []GeneratedFile{
		{Target: "codex", Path: ".codex/skills/ralph-loop/SKILL.md", Content: "# Codex\n"},
		{Target: "claude-code", Path: ".claude/skills/ralph-loop/SKILL.md", Content: "# Claude\n"},
		{Target: "trae", Path: ".trae/skills/ralph-loop/SKILL.md", Content: "# Trae\n"},
	}

	if err := WriteFiles(dir, files); err != nil {
		t.Fatalf("WriteFiles() error = %v", err)
	}
	for _, file := range files {
		body, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(file.Path)))
		if err != nil {
			t.Fatalf("read generated %s: %v", file.Path, err)
		}
		if string(body) != file.Content {
			t.Fatalf("%s content = %q, want %q", file.Path, string(body), file.Content)
		}
	}
}
