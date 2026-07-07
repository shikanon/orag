package operations

import (
	"context"
	"testing"
	"time"

	"github.com/shikanon/orag/internal/diagnostics"
	"github.com/shikanon/orag/internal/selfcheck"
	"github.com/shikanon/orag/internal/selfops"
)

func TestControllerHandlesStorageAlertWithoutApply(t *testing.T) {
	checks := fakeChecks{}
	diagnostics := fakeDiagnostics{verdict: selfcheck.VerdictFail}
	ops := &fakeOps{}
	controller := Controller{Checks: checks, Diagnostics: diagnostics, Ops: ops}

	result := controller.HandleAlert(context.Background(), Alert{Name: "ORAGDependencyCheckFailing", TraceID: "trace_alert"})
	if result.State != StatePlanGenerated {
		t.Fatalf("state = %q, want plan_generated result=%#v", result.State, result)
	}
	if result.Scope != string(selfcheck.ScopeStorage) {
		t.Fatalf("scope = %q, want storage", result.Scope)
	}
	if result.Plan == nil || result.Plan.Scope != selfops.ScopeAgentArtifacts || !result.Plan.DryRun {
		t.Fatalf("expected dry-run plan only: %#v", result.Plan)
	}
	if ops.applies != 0 {
		t.Fatalf("controller must not apply actions, applies=%d", ops.applies)
	}
	if !result.ReadOnly {
		t.Fatalf("controller result must be read-only: %#v", result)
	}
}

type fakeChecks struct{}

func (fakeChecks) Execute(_ context.Context, req selfcheck.Request) (selfcheck.Envelope, error) {
	return selfcheck.Envelope{
		SchemaVersion: selfcheck.SchemaVersion,
		TraceID:       req.TraceID,
		Scope:         req.Scope,
		Mode:          req.Mode,
		Verdict:       selfcheck.VerdictPass,
		ExitCode:      0,
		StartedAt:     time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC),
		CompletedAt:   time.Date(2026, 7, 6, 0, 0, 1, 0, time.UTC),
	}, nil
}

type fakeDiagnostics struct {
	verdict selfcheck.Verdict
}

func (f fakeDiagnostics) Diagnose(req diagnostics.DiagnoseRequest) diagnostics.DiagnoseResult {
	return diagnostics.DiagnoseResult{
		SchemaVersion: selfcheck.SchemaVersion,
		TraceID:       req.TraceID,
		Scope:         req.Scope,
		Verdict:       f.verdict,
		Severity:      selfcheck.SeverityCritical,
		ReadOnly:      true,
	}
}

func (f fakeDiagnostics) RunbookSuggest(req diagnostics.RunbookSuggestRequest) diagnostics.RunbookSuggestResponse {
	return diagnostics.RunbookSuggestResponse{
		SchemaVersion: selfcheck.SchemaVersion,
		Scope:         req.Scope,
		Verdict:       selfcheck.VerdictPass,
		Severity:      selfcheck.SeverityCritical,
		Runbook:       "docs/operations/troubleshooting.md",
		ReadOnly:      true,
	}
}

type fakeOps struct {
	applies int
}

func (f *fakeOps) Plan(_ context.Context, req selfops.PlanRequest) (selfops.Plan, error) {
	return selfops.Plan{
		SchemaVersion: selfops.SchemaVersion,
		TraceID:       req.TraceID,
		PlanID:        "plan_test",
		Scope:         req.Scope,
		Action:        "agent-artifacts-regenerate",
		Verdict:       selfops.VerdictPass,
		Status:        selfops.StatusPlanned,
		DryRun:        true,
		CreatedAt:     time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC),
	}, nil
}
