package operations

import (
	"context"
	"strings"

	"github.com/shikanon/orag/internal/diagnostics"
	"github.com/shikanon/orag/internal/selfcheck"
	"github.com/shikanon/orag/internal/selfops"
)

type Alert struct {
	Name     string            `json:"name"`
	Severity string            `json:"severity"`
	Labels   map[string]string `json:"labels,omitempty"`
	TraceID  string            `json:"trace_id,omitempty"`
	Symptom  string            `json:"symptom,omitempty"`
}

type State string

const (
	StateReceived        State = "received"
	StateChecked         State = "checked"
	StateDiagnosed       State = "diagnosed"
	StateRunbookSelected State = "runbook_selected"
	StatePlanGenerated   State = "plan_generated"
	StateBlocked         State = "blocked"
)

type CheckExecutor interface {
	Execute(ctx context.Context, req selfcheck.Request) (selfcheck.Envelope, error)
}

type DiagnosticExecutor interface {
	Diagnose(req diagnostics.DiagnoseRequest) diagnostics.DiagnoseResult
	RunbookSuggest(req diagnostics.RunbookSuggestRequest) diagnostics.RunbookSuggestResponse
}

type PlanExecutor interface {
	Plan(ctx context.Context, req selfops.PlanRequest) (selfops.Plan, error)
}

type Controller struct {
	Checks      CheckExecutor
	Diagnostics DiagnosticExecutor
	Ops         PlanExecutor
}

type Result struct {
	State         State                              `json:"state"`
	Scope         string                             `json:"scope"`
	Check         selfcheck.Envelope                 `json:"check,omitempty"`
	Diagnosis     diagnostics.DiagnoseResult         `json:"diagnosis,omitempty"`
	Runbook       diagnostics.RunbookSuggestResponse `json:"runbook,omitempty"`
	Plan          *selfops.Plan                      `json:"plan,omitempty"`
	BlockedReason string                             `json:"blocked_reason,omitempty"`
	ReadOnly      bool                               `json:"read_only"`
}

func (c Controller) HandleAlert(ctx context.Context, alert Alert) Result {
	scope := scopeForAlert(alert)
	result := Result{State: StateReceived, Scope: scope, ReadOnly: true}
	if c.Checks == nil || c.Diagnostics == nil {
		result.State = StateBlocked
		result.BlockedReason = "self-check and diagnostics executors are required"
		return result
	}
	check, err := c.Checks.Execute(ctx, selfcheck.Request{
		Scope:   selfcheck.Scope(scope),
		Mode:    selfcheck.ModeFocused,
		TraceID: strings.TrimSpace(alert.TraceID),
	})
	if err != nil {
		result.State = StateBlocked
		result.BlockedReason = err.Error()
		return result
	}
	result.Check = check
	result.State = StateChecked

	diagnosis := c.Diagnostics.Diagnose(diagnostics.DiagnoseRequest{
		Scope:   scope,
		Symptom: firstNonEmpty(alert.Symptom, alert.Name),
		TraceID: alert.TraceID,
	})
	result.Diagnosis = diagnosis
	result.State = StateDiagnosed

	runbook := c.Diagnostics.RunbookSuggest(diagnostics.RunbookSuggestRequest{
		Scope:   scope,
		Verdict: string(diagnosis.Verdict),
	})
	result.Runbook = runbook
	result.State = StateRunbookSelected

	if c.Ops == nil || diagnosis.Verdict == selfcheck.VerdictPass {
		return result
	}
	plan, err := c.Ops.Plan(ctx, selfops.PlanRequest{
		Scope:   selfops.ScopeAgentArtifacts,
		DryRun:  true,
		TraceID: strings.TrimSpace(alert.TraceID),
	})
	if err != nil {
		result.State = StateBlocked
		result.BlockedReason = err.Error()
		return result
	}
	result.Plan = &plan
	result.State = StatePlanGenerated
	return result
}

func scopeForAlert(alert Alert) string {
	name := strings.ToLower(alert.Name)
	for _, value := range alert.Labels {
		name += " " + strings.ToLower(value)
	}
	switch {
	case strings.Contains(name, "storage"), strings.Contains(name, "dependency"), strings.Contains(name, "postgres"), strings.Contains(name, "qdrant"):
		return string(selfcheck.ScopeStorage)
	case strings.Contains(name, "contract"), strings.Contains(name, "openapi"):
		return string(selfcheck.ScopeContract)
	case strings.Contains(name, "agent"), strings.Contains(name, "mcp"):
		return string(selfcheck.ScopeAgentSync)
	case strings.Contains(name, "release"):
		return string(selfcheck.ScopeRelease)
	default:
		return string(selfcheck.ScopeHealth)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
