package diagnostics

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/shikanon/orag/internal/rag"
	"github.com/shikanon/orag/internal/selfcheck"
	"github.com/shikanon/orag/internal/storage/postgres"
)

func TestTraceLookupMissingTraceIsBlocked(t *testing.T) {
	result := NewExecutor().TraceLookup(context.Background(), TraceLookupRequest{})

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

func TestTraceLookupStoreUnavailableIsBlocked(t *testing.T) {
	result := NewExecutor().TraceLookup(context.Background(), TraceLookupRequest{TraceID: "trace_missing"})

	if result.Verdict != selfcheck.VerdictBlocked || result.Severity != selfcheck.SeverityWarning || result.Found {
		t.Fatalf("unexpected trace lookup result: %#v", result)
	}
	if len(result.Findings) != 1 || result.Findings[0].ID != "orag.diagnostics.trace.store_unavailable" {
		t.Fatalf("missing stable store-unavailable finding: %#v", result.Findings)
	}
	if !contains(result.VerificationCommands, "oragctl trace --trace-id trace_missing") {
		t.Fatalf("missing trace verification command: %#v", result.VerificationCommands)
	}
	if containsTraceFixture(result) {
		t.Fatalf("store-unavailable lookup must not return fixture evidence: %#v", result)
	}
	if !result.ReadOnly {
		t.Fatalf("trace lookup must be read-only: %#v", result)
	}
}

func TestTraceLookupNonEmptyMissingTraceIsBlocked(t *testing.T) {
	result := NewExecutor(WithTraceGetter(fakeTraceGetter{})).TraceLookup(context.Background(), TraceLookupRequest{TraceID: "trace_missing"})

	if result.Verdict != selfcheck.VerdictBlocked || result.Severity != selfcheck.SeverityWarning || result.Found {
		t.Fatalf("unexpected trace lookup result: %#v", result)
	}
	if len(result.Findings) != 1 || result.Findings[0].ID != "orag.diagnostics.trace.not_found" {
		t.Fatalf("missing stable not-found finding: %#v", result.Findings)
	}
	if containsTraceFixture(result) {
		t.Fatalf("missing trace lookup must not return fixture evidence: %#v", result)
	}
}

func TestTraceLookupTraceSourceErrorIsBlocked(t *testing.T) {
	result := NewExecutor(WithTraceGetter(fakeTraceGetter{err: errors.New("database unavailable: secret DSN")})).
		TraceLookup(context.Background(), TraceLookupRequest{TraceID: "trace_1"})

	if result.Verdict != selfcheck.VerdictBlocked || result.Severity != selfcheck.SeverityWarning || result.Found {
		t.Fatalf("unexpected trace lookup result: %#v", result)
	}
	if len(result.Findings) != 1 || result.Findings[0].ID != "orag.diagnostics.trace.lookup_failed" {
		t.Fatalf("missing stable lookup-failed finding: %#v", result.Findings)
	}
	if strings.Contains(result.Findings[0].Title, "secret DSN") {
		t.Fatalf("lookup failure leaked internal error text: %#v", result.Findings[0])
	}
	if containsTraceFixture(result) {
		t.Fatalf("failed trace lookup must not return fixture evidence: %#v", result)
	}
}

func TestTraceLookupFoundTraceReturnsRealEvidence(t *testing.T) {
	record := postgres.TraceRecord{
		ID:         "trace_1",
		Profile:    rag.ProfileRealtime,
		LatencyMS:  42,
		ErrorCount: 1,
		NodeSpans: []postgres.TraceNodeSpan{{
			ID:        "span_1",
			NodeName:  "retrieve",
			Sequence:  1,
			LatencyMS: 40,
			Error:     "timeout",
		}},
	}
	result := NewExecutor(WithTraceGetter(fakeTraceGetter{record: record, found: true})).
		TraceLookup(context.Background(), TraceLookupRequest{TraceID: "trace_1"})

	if result.Verdict != selfcheck.VerdictPass || result.Severity != selfcheck.SeverityInfo || !result.Found {
		t.Fatalf("unexpected trace lookup result: %#v", result)
	}
	if len(result.Findings) != 1 || result.Findings[0].ID != "orag.diagnostics.trace.found" {
		t.Fatalf("missing stable found finding: %#v", result.Findings)
	}
	if len(result.Evidence) != 1 || !strings.Contains(result.Evidence[0].Message, "trace_1") || !strings.Contains(result.Evidence[0].Output, "retrieve") {
		t.Fatalf("missing real trace evidence: %#v", result.Evidence)
	}
	if containsTraceFixture(result) {
		t.Fatalf("found trace lookup must not return fixture evidence: %#v", result)
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

func containsTraceFixture(result TraceLookupResponse) bool {
	if strings.Contains(string(result.Verdict), "fixture") {
		return true
	}
	for _, finding := range result.Findings {
		if strings.Contains(finding.ID, "fixture") || strings.Contains(finding.Title, "fixture") {
			return true
		}
		for _, evidence := range finding.Evidence {
			if strings.Contains(evidence.Message, "connect a trace repository") || strings.Contains(evidence.Output, "connect a trace repository") {
				return true
			}
		}
	}
	for _, evidence := range result.Evidence {
		if strings.Contains(evidence.Message, "connect a trace repository") || strings.Contains(evidence.Output, "connect a trace repository") {
			return true
		}
	}
	return false
}

type fakeTraceGetter struct {
	record postgres.TraceRecord
	found  bool
	err    error
}

func (f fakeTraceGetter) GetTrace(context.Context, string) (postgres.TraceRecord, bool, error) {
	if f.err != nil {
		return postgres.TraceRecord{}, false, f.err
	}
	return f.record, f.found, nil
}
