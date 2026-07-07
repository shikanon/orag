package diagnostics

import (
	"context"
	"fmt"
	"strings"

	"github.com/shikanon/orag/internal/selfcheck"
)

const defaultRunbook = "docs/operations/troubleshooting.md"

type Executor struct {
	Evidence EvidenceProvider
}

func NewExecutor() *Executor {
	return &Executor{}
}

func NewExecutorWithEvidence(provider EvidenceProvider) *Executor {
	return &Executor{Evidence: provider}
}

type EvidenceProvider interface {
	GetTrace(ctx context.Context, traceID string) (TraceEvidence, bool, error)
	MetricsSnapshot(ctx context.Context, scope string) ([]selfcheck.Evidence, error)
	RecentLogs(ctx context.Context, traceID string, limit int) ([]selfcheck.Evidence, error)
}

type TraceEvidence struct {
	TraceID     string
	HasError    bool
	ErrorCount  int
	SlowestNode string
	LatencyMS   int64
	NodeCount   int
	Evidence    []selfcheck.Evidence
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

func (e *Executor) TraceLookup(req TraceLookupRequest) TraceLookupResponse {
	traceID := strings.TrimSpace(req.TraceID)
	response := TraceLookupResponse{
		SchemaVersion:        selfcheck.SchemaVersion,
		TraceID:              traceID,
		Verdict:              selfcheck.VerdictPass,
		Severity:             selfcheck.SeverityInfo,
		Found:                true,
		RecommendedActions:   []string{"使用 trace 证据继续调用 orag_diagnose；该查询不执行写操作。"},
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
	if e == nil || e.Evidence == nil {
		response.Verdict = selfcheck.VerdictBlocked
		response.Severity = selfcheck.SeverityWarning
		response.Found = false
		response.Findings = []Finding{{
			ID:       "orag.diagnostics.trace.evidence_unavailable",
			Title:    "trace evidence provider is not configured",
			Severity: selfcheck.SeverityWarning,
			Verdict:  selfcheck.VerdictBlocked,
			Evidence: []selfcheck.Evidence{{
				Type:    "trace",
				Message: "No trace evidence provider is configured for diagnostics.",
				Command: traceCommand(traceID),
			}},
		}}
		response.Evidence = response.Findings[0].Evidence
		return response
	}
	trace, found, err := e.Evidence.GetTrace(context.Background(), traceID)
	if err != nil {
		response.Verdict = selfcheck.VerdictFail
		response.Severity = selfcheck.SeverityCritical
		response.Found = false
		response.Findings = []Finding{{
			ID:       "orag.diagnostics.trace.lookup_failed",
			Title:    "trace lookup failed",
			Severity: selfcheck.SeverityCritical,
			Verdict:  selfcheck.VerdictFail,
			Evidence: []selfcheck.Evidence{{Type: "trace", Message: err.Error(), Command: traceCommand(traceID)}},
		}}
		response.Evidence = response.Findings[0].Evidence
		return response
	}
	if !found {
		response.Verdict = selfcheck.VerdictBlocked
		response.Severity = selfcheck.SeverityWarning
		response.Found = false
		response.Findings = []Finding{{
			ID:       "orag.diagnostics.trace.not_found",
			Title:    "trace evidence was not found",
			Severity: selfcheck.SeverityWarning,
			Verdict:  selfcheck.VerdictBlocked,
			Evidence: []selfcheck.Evidence{{Type: "trace", Message: "Trace ID was not found in the configured evidence provider.", Command: traceCommand(traceID)}},
		}}
		response.Evidence = response.Findings[0].Evidence
		return response
	}
	finding := Finding{
		ID:       "orag.diagnostics.trace.found",
		Title:    fmt.Sprintf("trace has %d node spans; slowest node: %s", trace.NodeCount, firstNonEmpty(trace.SlowestNode, "unknown")),
		Severity: selfcheck.SeverityInfo,
		Verdict:  selfcheck.VerdictPass,
		Evidence: trace.Evidence,
	}
	if trace.HasError {
		finding.ID = "orag.diagnostics.trace.has_error"
		finding.Title = fmt.Sprintf("trace contains %d node errors; slowest node: %s", trace.ErrorCount, firstNonEmpty(trace.SlowestNode, "unknown"))
		finding.Severity = selfcheck.SeverityCritical
		finding.Verdict = selfcheck.VerdictFail
		response.Verdict = selfcheck.VerdictFail
		response.Severity = selfcheck.SeverityCritical
	}
	response.Findings = []Finding{finding}
	response.Evidence = finding.Evidence
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
	if strings.TrimSpace(req.TraceID) != "" && e != nil && e.Evidence != nil {
		trace, found, err := e.Evidence.GetTrace(context.Background(), strings.TrimSpace(req.TraceID))
		if err != nil {
			result.Verdict = selfcheck.VerdictFail
			result.Severity = selfcheck.SeverityCritical
			result.Findings = []Finding{{
				ID:       "orag.diagnostics." + scope + ".trace_lookup_failed",
				Title:    "trace lookup failed during diagnosis",
				Severity: selfcheck.SeverityCritical,
				Verdict:  selfcheck.VerdictFail,
				Evidence: []selfcheck.Evidence{{Type: "trace", Message: err.Error(), Command: traceCommand(req.TraceID)}},
			}}
			result.RecommendedActions = []string{"不执行写操作；先确认 trace repository 和日志证据源可用。"}
			return result
		}
		if found && trace.HasError {
			result.Verdict = selfcheck.VerdictFail
			result.Severity = selfcheck.SeverityCritical
			result.Findings = []Finding{{
				ID:       "orag.diagnostics." + scope + ".trace_error",
				Title:    fmt.Sprintf("trace evidence contains %d node errors", trace.ErrorCount),
				Severity: selfcheck.SeverityCritical,
				Verdict:  selfcheck.VerdictFail,
				Evidence: trace.Evidence,
			}}
			result.RecommendedActions = []string{"不执行写操作；根据 trace 中的失败节点选择 runbook，再生成 dry-run 修复计划。"}
			return result
		}
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
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

func truncate(value string) string {
	const limit = 4096
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "...[truncated]"
}
