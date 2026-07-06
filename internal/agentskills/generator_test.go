package agentskills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shikanon/orag/internal/capabilities"
)

func TestGenerateFromManifestProducesAgentSkillTargets(t *testing.T) {
	files, err := GenerateFromManifest(capabilities.MustBuiltinManifest())
	if err != nil {
		t.Fatalf("GenerateFromManifest() error = %v", err)
	}

	byPath := map[string]GeneratedFile{}
	for _, file := range files {
		byPath[file.Path] = file
	}
	for _, path := range []string{
		"agent/skills/codex/ralph-loop/SKILL.md",
		"agent/skills/claude-code/ralph-loop/SKILL.md",
		"agent/skills/trae/ralph-loop/SKILL.md",
		"agent/skills/codex/orag-self-check/SKILL.md",
		"agent/skills/claude-code/orag-self-diagnose/SKILL.md",
		"agent/skills/trae/orag-self-ops/SKILL.md",
	} {
		if _, ok := byPath[path]; !ok {
			t.Fatalf("missing generated file %s in %#v", path, files)
		}
	}

	ralph := byPath["agent/skills/trae/ralph-loop/SKILL.md"].Content
	for _, want := range []string{
		"Generated from `orag.capabilities.v1` version `2026-07-05`",
		"ORAG_API_BASE_URL",
		"ORAG_API_TOKEN",
		"ORAG_TENANT_ID",
		"`POST /v1/ralph-loop`",
		"`ralph_loop_run`",
		"`#/components/schemas/RalphLoopRequest`",
		"Never print bearer tokens",
		"Task 1",
	} {
		if !strings.Contains(ralph, want) {
			t.Fatalf("ralph Skill missing %q\n%s", want, ralph)
		}
	}

	selfCheck := byPath["agent/skills/codex/orag-self-check/SKILL.md"].Content
	for _, want := range []string{
		"`orag_check`",
		"make agent-sync-check remains the authoritative release gate",
		"Key: `self-check`",
	} {
		if !strings.Contains(selfCheck, want) {
			t.Fatalf("self-check Skill missing %q\n%s", want, selfCheck)
		}
	}

	diagnose := byPath["agent/skills/claude-code/orag-self-diagnose/SKILL.md"].Content
	for _, want := range []string{"`orag_diagnose`", "`orag_trace_lookup`", "`orag_runbook_suggest`", "read-only"} {
		if !strings.Contains(diagnose, want) {
			t.Fatalf("diagnose Skill missing %q\n%s", want, diagnose)
		}
	}

	ops := byPath["agent/skills/trae/orag-self-ops/SKILL.md"].Content
	for _, want := range []string{"`orag_maintenance_plan`", "`orag_apply_low_risk_action`", "`orag_create_remediation_issue`", "Default to dry-run"} {
		if !strings.Contains(ops, want) {
			t.Fatalf("ops Skill missing %q\n%s", want, ops)
		}
	}
}

func TestWriteFilesCreatesSkillDirectories(t *testing.T) {
	dir := t.TempDir()
	files := []GeneratedFile{
		{Target: "codex", Path: "agent/skills/codex/orag-self-check/SKILL.md", Content: "# Codex\n"},
		{Target: "claude-code", Path: "agent/skills/claude-code/orag-self-diagnose/SKILL.md", Content: "# Claude\n"},
		{Target: "trae", Path: "agent/skills/trae/orag-self-ops/SKILL.md", Content: "# Trae\n"},
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
