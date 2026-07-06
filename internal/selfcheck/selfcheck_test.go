package selfcheck

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestExecutorAgentSyncPassIncludesStableIDAndGateWarning(t *testing.T) {
	executor := NewExecutor(Options{
		Runner: fakeRunner{results: map[string]CommandResult{
			"make agent-sync-check": {ExitCode: 0, Stdout: "artifacts match"},
		}},
		Now: fixedClock(),
	})

	result, err := executor.Execute(context.Background(), Request{
		Scope:   ScopeAgentSync,
		Mode:    ModeFocused,
		TraceID: "trace_req",
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.SchemaVersion != SchemaVersion || result.TraceID != "trace_req" {
		t.Fatalf("unexpected envelope identity: %#v", result)
	}
	if result.Verdict != VerdictPass || result.ExitCode != 0 || result.Partial {
		t.Fatalf("unexpected verdict fields: %#v", result)
	}
	if result.RuntimeGateWarning == "" || !strings.Contains(result.RuntimeGateWarning, "authoritative release gate") {
		t.Fatalf("missing runtime gate warning: %q", result.RuntimeGateWarning)
	}
	if len(result.Results) != 1 || result.Results[0].ID != "orag.selfcheck.agent_sync.artifacts" {
		t.Fatalf("unexpected check IDs: %#v", result.Results)
	}
}

func TestExecutorMapsCommandFailureToFailExitCode(t *testing.T) {
	executor := NewExecutor(Options{
		Runner: fakeRunner{results: map[string]CommandResult{
			"go test ./tests/contract -run TestOpenAPI -v": {ExitCode: 1, Stderr: "openapi drift", Err: errFailed},
		}},
		Now: fixedClock(),
	})

	result, err := executor.Execute(context.Background(), Request{Scope: ScopeContract, Mode: ModeFocused})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Verdict != VerdictFail || result.ExitCode != 1 {
		t.Fatalf("verdict=%q exit=%d, want fail/1", result.Verdict, result.ExitCode)
	}
	check := result.Results[0]
	if check.Status != StatusFail || check.Severity != SeverityCritical || check.Verdict != VerdictFail {
		t.Fatalf("unexpected failed check: %#v", check)
	}
}

func TestExecutorPerCheckTimeoutReturnsPartialBlocked(t *testing.T) {
	executor := NewExecutor(Options{
		Runner: blockingRunner{},
		Now:    fixedClock(),
	})

	result, err := executor.Execute(context.Background(), Request{
		Scope:                  ScopeRelease,
		Mode:                   ModeFocused,
		OverallDeadlineSeconds: 2,
		PerCheckTimeoutSeconds: 1,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Verdict != VerdictBlocked || result.ExitCode != 2 || !result.Partial {
		t.Fatalf("unexpected timeout verdict: %#v", result)
	}
	if len(result.Results) == 0 || result.Results[0].Status != StatusBlocked {
		t.Fatalf("first check was not blocked: %#v", result.Results)
	}
	if result.Results[0].ID != "orag.selfcheck.agent_sync.artifacts" {
		t.Fatalf("stable check ID changed: %#v", result.Results[0])
	}
}

func TestExecutorRejectsInvalidRequest(t *testing.T) {
	executor := NewExecutor(Options{})
	_, err := executor.Execute(context.Background(), Request{Scope: "bad", Mode: ModeFocused})
	if err == nil || !strings.Contains(err.Error(), "invalid self-check scope") {
		t.Fatalf("Execute() error = %v, want invalid scope", err)
	}
	if ExitCode(VerdictInvalid) != 3 {
		t.Fatalf("invalid exit code = %d, want 3", ExitCode(VerdictInvalid))
	}
}

type fakeRunner struct {
	results map[string]CommandResult
}

func (f fakeRunner) Run(_ context.Context, command Command) CommandResult {
	if result, ok := f.results[command.String()]; ok {
		return result
	}
	return CommandResult{ExitCode: 0}
}

type blockingRunner struct{}

func (blockingRunner) Run(ctx context.Context, _ Command) CommandResult {
	<-ctx.Done()
	return CommandResult{ExitCode: 1, Err: ctx.Err()}
}

var errFailed = simpleError("command failed")

type simpleError string

func (e simpleError) Error() string { return string(e) }

func fixedClock() func() time.Time {
	current := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	return func() time.Time {
		current = current.Add(time.Millisecond)
		return current
	}
}
