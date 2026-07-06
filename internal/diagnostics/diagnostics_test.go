package diagnostics

import (
	"strings"
	"testing"

	"github.com/shikanon/orag/internal/selfcheck"
)

func TestTraceLookupMissingTraceIsBlocked(t *testing.T) {
	result := NewExecutor().TraceLookup(TraceLookupRequest{})

	if result.SchemaVersion != selfcheck.SchemaVersion || result.Verdict != selfcheck.VerdictBlocked || result.Found {
		t.Fatalf("unexpected trace lookup result: %#v", result)
	}
	if len(result.Findings) != 1 || result.Findings[0].ID != "orag.diagnostics.trace.missing" {
		t.Fatalf("missing stable finding: %#v", result.Findings)
	}
	if !result.ReadOnly {
		t.Fatalf("trace lookup must be read-only: %#v", result)
	}
}

func TestDiagnoseFailedCommandEvidence(t *testing.T) {
	result := NewExecutor().Diagnose(DiagnoseRequest{
		Scope:                 "agent_sync",
		Symptom:               "make agent-sync-check failed",
		TraceID:               "trace_req",
		FailedCommand:         "make agent-sync-check",
		FailedCommandExitCode: 1,
		FailedCommandOutput:   ".mcp/tools/orag-self-diagnose.json: generated content differs",
		AllowCommands:         true,
	})

	if result.Verdict != selfcheck.VerdictFail || result.Severity != selfcheck.SeverityCritical {
		t.Fatalf("unexpected diagnosis verdict: %#v", result)
	}
	if len(result.Findings) == 0 || result.Findings[0].ID != "orag.diagnostics.agent_sync.failed_command" {
		t.Fatalf("unexpected findings: %#v", result.Findings)
	}
	if !contains(result.VerificationCommands, "make agent-sync-check") {
		t.Fatalf("missing verification command: %#v", result.VerificationCommands)
	}
	if !result.ReadOnly || !strings.Contains(result.RecommendedActions[0], "不执行") {
		t.Fatalf("diagnosis must preserve read-only boundary: %#v", result)
	}
}

func TestDiagnoseUnknownScopeReturnsInvalid(t *testing.T) {
	result := NewExecutor().Diagnose(DiagnoseRequest{Scope: "unknown", Symptom: "bad scope"})

	if result.Verdict != selfcheck.VerdictInvalid || result.Severity != selfcheck.SeverityWarning {
		t.Fatalf("unexpected unknown scope diagnosis: %#v", result)
	}
	if len(result.Findings) != 1 || result.Findings[0].ID != "orag.diagnostics.scope.unknown" {
		t.Fatalf("missing unknown scope finding: %#v", result.Findings)
	}
}

func TestRunbookSuggestMapsStorageScope(t *testing.T) {
	result := NewExecutor().RunbookSuggest(RunbookSuggestRequest{Scope: "storage", Verdict: string(selfcheck.VerdictBlocked)})

	if result.Verdict != selfcheck.VerdictPass || result.Runbook != "docs/operations/troubleshooting.md" {
		t.Fatalf("unexpected runbook suggestion: %#v", result)
	}
	if !contains(result.VerificationCommands, "oragctl trace --trace-id <trace_id>") {
		t.Fatalf("missing trace verification command: %#v", result.VerificationCommands)
	}
	if !result.ReadOnly {
		t.Fatalf("runbook suggestion must be read-only: %#v", result)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
