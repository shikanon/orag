package diagnostics

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/shikanon/orag/internal/selfcheck"
	"github.com/shikanon/orag/internal/storage/postgres"
)

const defaultRunbook = "docs/operations/troubleshooting.md"

type TraceGetter interface {
	GetTrace(ctx context.Context, traceID string) (postgres.TraceRecord, bool, error)
}

type Option func(*Executor)

type Executor struct {
	traces TraceGetter
}

func NewExecutor(options ...Option) *Executor {
	executor := &Executor{}
	for _, option := range options {
		if option != nil {
			option(executor)
		}
	}
	return executor
}

func WithTraceGetter(getter TraceGetter) Option {
	return func(e *Executor) {
		e.traces = getter
	}
}

type TraceLookupRequest struct {
	TraceID string `json:"trace_id"`
}

type TraceLookupResponse struct {
	SchemaVersion        string               `json:"schema_version"`
	TraceID              string               `json:"trace_id"`
	Verdict              selfcheck.Verdict    `json:"verdict"`
	Severity             selfcheck.Severity   `json:"severity"`
	Found                bool                 `json:"found"`
	Findings             []Finding            `json:"findings"`
	Evidence             []selfcheck.Evidence `json:"evidence,omitempty"`
	RecommendedActions   []string             `json:"recommended_actions"`
	VerificationCommands []string             `json:"verification_commands"`
	Artifacts            []selfcheck.Artifact `json:"artifacts"`
	ReadOnly             bool                 `json:"read_only"`
}

type DiagnoseRequest struct {
	Scope                 string `json:"scope"`
	Symptom               string `json:"symptom"`
	TraceID               string `json:"trace_id,omitempty"`
	FailedCommand         string `json:"failed_command,omitempty"`
	FailedCommandExitCode int    `json:"failed_command_exit_code,omitempty"`
	FailedCommandOutput   string `json:"failed_command_output,omitempty"`
	AllowCommands         bool   `json:"allow_commands,omitempty"`
}

type DiagnoseResult struct {
	SchemaVersion        string               `json:"schema_version"`
	TraceID              string               `json:"trace_id,omitempty"`
	Scope                string               `json:"scope"`
	Symptom              string               `json:"symptom,omitempty"`
	Verdict              selfcheck.Verdict    `json:"verdict"`
	Severity             selfcheck.Severity   `json:"severity"`
	Findings             []Finding            `json:"findings"`
	RecommendedActions   []string             `json:"recommended_actions"`
	VerificationCommands []string             `json:"verification_commands"`
	Artifacts            []selfcheck.Artifact `json:"artifacts"`
	ReadOnly             bool                 `json:"read_only"`
}

type RunbookSuggestRequest struct {
	Scope   string `json:"scope"`
	Verdict string `json:"verdict,omitempty"`
}

type RunbookSuggestResponse struct {
	SchemaVersion        string               `json:"schema_version"`
	Scope                string               `json:"scope"`
	Verdict              selfcheck.Verdict    `json:"verdict"`
	Severity             selfcheck.Severity   `json:"severity"`
	Runbook              string               `json:"runbook"`
	RecommendedActions   []string             `json:"recommended_actions"`
	VerificationCommands []string             `json:"verification_commands"`
	Artifacts            []selfcheck.Artifact `json:"artifacts"`
	ReadOnly             bool                 `json:"read_only"`
}

type Finding struct {
	ID       string               `json:"id"`
	Title    string               `json:"title"`
	Severity selfcheck.Severity   `json:"severity"`
	Verdict  selfcheck.Verdict    `json:"verdict"`
	Evidence []selfcheck.Evidence `json:"evidence,omitempty"`
}

func (e *Executor) TraceLookup(ctx context.Context, req TraceLookupRequest) TraceLookupResponse {
	if ctx == nil {
		ctx = context.Background()
	}
	traceID := strings.TrimSpace(req.TraceID)
	response := TraceLookupResponse{
		SchemaVersion:        selfcheck.SchemaVersion,
		TraceID:              traceID,
		Verdict:              selfcheck.VerdictBlocked,
		Severity:             selfcheck.SeverityWarning,
		Found:                false,
		RecommendedActions:   []string{"配置 trace repository 或 trace API 后重试；该查询不执行写操作。"},
		VerificationCommands: []string{traceCommand(traceID)},
		Artifacts:            []selfcheck.Artifact{{Type: "runbook", URI: defaultRunbook}},
		ReadOnly:             true,
	}
	if traceID == "" {
		response.Verdict = selfcheck.VerdictBlocked
		response.Severity = selfcheck.SeverityWarning
		response.Found = false
		response.Findings = []Finding{{
			ID:       "orag.diagnostics.trace.missing",
			Title:    "trace_id is required for trace lookup",
			Severity: selfcheck.SeverityWarning,
			Verdict:  selfcheck.VerdictBlocked,
		}}
		response.VerificationCommands = []string{"oragctl trace --trace-id <trace_id>"}
		return response
	}
	if e == nil || e.traces == nil {
		response.Findings = []Finding{traceBlockedFinding(
			"orag.diagnostics.trace.store_unavailable",
			"trace store is unavailable for trace lookup",
		)}
		return response
	}

	record, found, err := e.traces.GetTrace(ctx, traceID)
	if err != nil {
		response.Findings = []Finding{traceBlockedFinding(
			"orag.diagnostics.trace.lookup_failed",
			"trace lookup failed; retry after checking trace store availability",
		)}
		return response
	}
	if !found {
		response.Findings = []Finding{traceBlockedFinding(
			"orag.diagnostics.trace.not_found",
			"trace was not found in the configured trace store",
		)}
		return response
	}

	response.TraceID = firstNonEmpty(strings.TrimSpace(record.ID), traceID)
	response.Verdict = selfcheck.VerdictPass
	response.Severity = selfcheck.SeverityInfo
	response.Found = true
	response.RecommendedActions = []string{"使用 trace 证据继续调用 orag_diagnose；该查询不执行写操作。"}
	response.VerificationCommands = []string{traceCommand(response.TraceID)}
	response.Findings = []Finding{{
		ID:       "orag.diagnostics.trace.found",
		Title:    "trace evidence was found in the configured trace store",
		Severity: selfcheck.SeverityInfo,
		Verdict:  selfcheck.VerdictPass,
		Evidence: traceEvidence(record, response.TraceID),
	}}
	response.Evidence = response.Findings[0].Evidence
	return response
}

func (e *Executor) Diagnose(req DiagnoseRequest) DiagnoseResult {
	scope := normalizeScope(req.Scope)
	result := DiagnoseResult{
		SchemaVersion:        selfcheck.SchemaVersion,
		TraceID:              strings.TrimSpace(req.TraceID),
		Scope:                scope,
		Symptom:              strings.TrimSpace(req.Symptom),
		Verdict:              selfcheck.VerdictPass,
		Severity:             selfcheck.SeverityInfo,
		Artifacts:            []selfcheck.Artifact{{Type: "runbook", URI: runbookForScope(scope)}},
		ReadOnly:             true,
		VerificationCommands: verificationCommandsForScope(scope, strings.TrimSpace(req.TraceID)),
	}
	if !knownScope(scope) {
		result.Verdict = selfcheck.VerdictInvalid
		result.Severity = selfcheck.SeverityWarning
		result.Scope = strings.TrimSpace(req.Scope)
		result.Findings = []Finding{{
			ID:       "orag.diagnostics.scope.unknown",
			Title:    fmt.Sprintf("unknown diagnostic scope %q", strings.TrimSpace(req.Scope)),
			Severity: selfcheck.SeverityWarning,
			Verdict:  selfcheck.VerdictInvalid,
		}}
		result.RecommendedActions = []string{"选择 health、contract、agent_sync、mcp、smoke、storage、config、release、all 或 trace 之一后重试。"}
		return result
	}

	if strings.TrimSpace(req.FailedCommand) != "" || req.FailedCommandExitCode != 0 {
		result.Verdict = selfcheck.VerdictFail
		result.Severity = selfcheck.SeverityCritical
		result.Findings = []Finding{{
			ID:       "orag.diagnostics." + scope + ".failed_command",
			Title:    "failed command evidence indicates a reproducible check failure",
			Severity: selfcheck.SeverityCritical,
			Verdict:  selfcheck.VerdictFail,
			Evidence: []selfcheck.Evidence{{
				Type:     "command",
				Message:  "caller-provided command evidence; diagnostics did not execute it",
				Command:  strings.TrimSpace(req.FailedCommand),
				ExitCode: req.FailedCommandExitCode,
				Output:   truncate(strings.TrimSpace(req.FailedCommandOutput)),
			}},
		}}
		result.RecommendedActions = []string{"不执行写操作；先重新运行验证命令确认失败，再用 orag-self-ops 生成 dry-run 修复计划。"}
		return result
	}

	result.Findings = []Finding{{
		ID:       "orag.diagnostics." + scope + ".symptom",
		Title:    findingTitle(scope, result.Symptom),
		Severity: selfcheck.SeverityInfo,
		Verdict:  selfcheck.VerdictPass,
	}}
	result.RecommendedActions = []string{"不执行写操作；收集 trace、日志或失败命令证据后再推进修复。"}
	if req.AllowCommands {
		result.RecommendedActions = append(result.RecommendedActions, "即使 allow_commands=true，诊断工具也只返回建议命令，不会执行命令。")
	}
	return result
}

func (e *Executor) RunbookSuggest(req RunbookSuggestRequest) RunbookSuggestResponse {
	scope := normalizeScope(req.Scope)
	if !knownScope(scope) {
		return RunbookSuggestResponse{
			SchemaVersion:        selfcheck.SchemaVersion,
			Scope:                strings.TrimSpace(req.Scope),
			Verdict:              selfcheck.VerdictInvalid,
			Severity:             selfcheck.SeverityWarning,
			Runbook:              defaultRunbook,
			RecommendedActions:   []string{"选择已知诊断 scope 后重试。"},
			VerificationCommands: []string{"make agent-sync-check", "go test ./tests/contract -run TestOpenAPI -v"},
			Artifacts:            []selfcheck.Artifact{{Type: "runbook", URI: defaultRunbook}},
			ReadOnly:             true,
		}
	}
	runbook := runbookForScope(scope)
	return RunbookSuggestResponse{
		SchemaVersion:        selfcheck.SchemaVersion,
		Scope:                scope,
		Verdict:              selfcheck.VerdictPass,
		Severity:             severityForVerdict(req.Verdict),
		Runbook:              runbook,
		RecommendedActions:   []string{"阅读 runbook 并执行验证命令；该工具仅返回建议，不执行命令或写操作。"},
		VerificationCommands: verificationCommandsForScope(scope, ""),
		Artifacts:            []selfcheck.Artifact{{Type: "runbook", URI: runbook}},
		ReadOnly:             true,
	}
}

func normalizeScope(scope string) string {
	scope = strings.TrimSpace(strings.ToLower(scope))
	if scope == "" {
		return "trace"
	}
	if scope == "agent" || scope == "agentsync" {
		return "agent_sync"
	}
	return scope
}

func knownScope(scope string) bool {
	switch scope {
	case "health", "contract", "agent_sync", "mcp", "smoke", "storage", "config", "release", "all", "trace":
		return true
	default:
		return false
	}
}

func runbookForScope(scope string) string {
	switch scope {
	case "agent_sync", "mcp", "contract", "smoke", "release":
		return "docs/api/agent-integrations.md"
	default:
		return defaultRunbook
	}
}

func verificationCommandsForScope(scope, traceID string) []string {
	commands := []string{"oragctl trace --trace-id <trace_id>"}
	if traceID != "" {
		commands[0] = traceCommand(traceID)
	}
	switch scope {
	case "agent_sync", "mcp":
		return append([]string{"make agent-sync-check"}, commands...)
	case "contract":
		return append([]string{"go test ./tests/contract -run TestOpenAPI -v"}, commands...)
	case "smoke":
		return append([]string{"go test ./internal/mcp -run TestServerInitializeAndListTools -v"}, commands...)
	case "release", "all":
		return append([]string{"make agent-sync-check", "go test ./tests/contract -run TestOpenAPI -v", "go test ./internal/mcp -run TestServerInitializeAndListTools -v"}, commands...)
	case "storage":
		return append(commands, "curl -fsS http://localhost:8080/readyz")
	case "health", "config":
		return append(commands, "curl -fsS http://localhost:8080/healthz")
	default:
		return commands
	}
}

func severityForVerdict(verdict string) selfcheck.Severity {
	switch selfcheck.Verdict(strings.TrimSpace(verdict)) {
	case selfcheck.VerdictFail:
		return selfcheck.SeverityCritical
	case selfcheck.VerdictBlocked, selfcheck.VerdictInvalid:
		return selfcheck.SeverityWarning
	default:
		return selfcheck.SeverityInfo
	}
}

func findingTitle(scope, symptom string) string {
	if symptom != "" {
		return "diagnostic summary for " + symptom
	}
	return "diagnostic summary for " + scope
}

func traceCommand(traceID string) string {
	if strings.TrimSpace(traceID) == "" {
		return "oragctl trace --trace-id <trace_id>"
	}
	return "oragctl trace --trace-id " + strings.TrimSpace(traceID)
}

func traceBlockedFinding(id, title string) Finding {
	return Finding{
		ID:       id,
		Title:    title,
		Severity: selfcheck.SeverityWarning,
		Verdict:  selfcheck.VerdictBlocked,
	}
}

func traceEvidence(record postgres.TraceRecord, traceID string) []selfcheck.Evidence {
	output, _ := json.Marshal(traceSummary(record, traceID))
	return []selfcheck.Evidence{{
		Type: "trace",
		Message: fmt.Sprintf(
			"Trace evidence found for %s (profile=%s latency_ms=%d errors=%d spans=%d).",
			traceID,
			record.Profile,
			record.LatencyMS,
			record.ErrorCount,
			len(record.NodeSpans),
		),
		Command: traceCommand(traceID),
		Output:  truncate(string(output)),
	}}
}

func traceSummary(record postgres.TraceRecord, traceID string) map[string]any {
	const maxSpans = 20
	spans := record.NodeSpans
	truncated := false
	if len(spans) > maxSpans {
		spans = spans[:maxSpans]
		truncated = true
	}
	return map[string]any{
		"trace_id":    traceID,
		"profile":     record.Profile,
		"latency_ms":  record.LatencyMS,
		"error_count": record.ErrorCount,
		"span_count":  len(record.NodeSpans),
		"truncated":   truncated,
		"node_spans":  spans,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func truncate(value string) string {
	const limit = 4096
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "...[truncated]"
}
