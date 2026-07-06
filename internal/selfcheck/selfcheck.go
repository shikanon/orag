package selfcheck

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	SchemaVersion       = "orag.selfops.result.v1"
	DefaultDeadline     = 60 * time.Second
	DefaultCheckTimeout = 15 * time.Second
	RuntimeGateWarning  = "Runtime probes do not replace static make agent-sync-check; CI static make agent-sync-check remains the authoritative release gate."
)

type Scope string

const (
	ScopeHealth    Scope = "health"
	ScopeContract  Scope = "contract"
	ScopeAgentSync Scope = "agent_sync"
	ScopeSmoke     Scope = "smoke"
	ScopeStorage   Scope = "storage"
	ScopeConfig    Scope = "config"
	ScopeRelease   Scope = "release"
	ScopeAll       Scope = "all"
)

type Mode string

const (
	ModeFocused Mode = "focused"
	ModeBroad   Mode = "broad"
)

type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

type Verdict string

const (
	VerdictPass    Verdict = "pass"
	VerdictFail    Verdict = "fail"
	VerdictBlocked Verdict = "blocked"
	VerdictInvalid Verdict = "invalid"
)

type Status string

const (
	StatusPass    Status = "pass"
	StatusFail    Status = "fail"
	StatusBlocked Status = "blocked"
)

type Request struct {
	Scope                  Scope
	Mode                   Mode
	TraceID                string
	OverallDeadlineSeconds int
	PerCheckTimeoutSeconds int
}

type Envelope struct {
	SchemaVersion          string        `json:"schema_version"`
	TraceID                string        `json:"trace_id"`
	Scope                  Scope         `json:"scope"`
	Mode                   Mode          `json:"mode"`
	Verdict                Verdict       `json:"verdict"`
	ExitCode               int           `json:"exit_code"`
	Partial                bool          `json:"partial"`
	RuntimeGateWarning     string        `json:"runtime_gate_warning,omitempty"`
	OverallDeadlineSeconds int           `json:"overall_deadline_seconds"`
	PerCheckTimeoutSeconds int           `json:"per_check_timeout_seconds"`
	StartedAt              time.Time     `json:"started_at"`
	CompletedAt            time.Time     `json:"completed_at"`
	Artifacts              []Artifact    `json:"artifacts"`
	Results                []CheckResult `json:"results"`
}

type CheckResult struct {
	ID          string     `json:"id"`
	Scope       Scope      `json:"scope"`
	Name        string     `json:"name"`
	Severity    Severity   `json:"severity"`
	Status      Status     `json:"status"`
	Verdict     Verdict    `json:"verdict"`
	Evidence    []Evidence `json:"evidence,omitempty"`
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt time.Time  `json:"completed_at"`
	DurationMS  int64      `json:"duration_ms"`
}

type Evidence struct {
	Type     string `json:"type"`
	Message  string `json:"message"`
	Command  string `json:"command,omitempty"`
	ExitCode int    `json:"exit_code,omitempty"`
	Output   string `json:"output,omitempty"`
}

type Artifact struct {
	Type string `json:"type"`
	URI  string `json:"uri"`
}

type Command struct {
	Name string
	Args []string
	Dir  string
}

func (c Command) String() string {
	return strings.TrimSpace(c.Name + " " + strings.Join(c.Args, " "))
}

type CommandResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Err      error
}

type CommandRunner interface {
	Run(ctx context.Context, command Command) CommandResult
}

type Options struct {
	Runner  CommandRunner
	WorkDir string
	Now     func() time.Time
}

type Executor struct {
	runner  CommandRunner
	workDir string
	now     func() time.Time
}

func NewExecutor(options Options) *Executor {
	runner := options.Runner
	if runner == nil {
		runner = ShellRunner{}
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	return &Executor{runner: runner, workDir: options.WorkDir, now: now}
}

func (e *Executor) Execute(ctx context.Context, req Request) (Envelope, error) {
	if e == nil {
		e = NewExecutor(Options{})
	}
	if err := req.validate(); err != nil {
		return Envelope{}, err
	}
	traceID := strings.TrimSpace(req.TraceID)
	if traceID == "" {
		traceID = newTraceID()
	}
	overall := durationFromSeconds(req.OverallDeadlineSeconds, DefaultDeadline)
	perCheck := durationFromSeconds(req.PerCheckTimeoutSeconds, DefaultCheckTimeout)
	defs := checksForScope(req.Scope, req.Mode)
	started := e.now()
	runCtx, cancel := context.WithTimeout(ctx, overall)
	defer cancel()

	results := make([]CheckResult, 0, len(defs))
	for _, def := range defs {
		select {
		case <-runCtx.Done():
			results = append(results, blockedResult(def, started, e.now(), "overall deadline reached before check started"))
			continue
		default:
		}
		results = append(results, e.runOne(runCtx, def, perCheck))
	}
	verdict := aggregateVerdict(results)
	completed := e.now()
	envelope := Envelope{
		SchemaVersion:          SchemaVersion,
		TraceID:                traceID,
		Scope:                  req.Scope,
		Mode:                   req.Mode,
		Verdict:                verdict,
		ExitCode:               ExitCode(verdict),
		Partial:                hasBlocked(results),
		OverallDeadlineSeconds: int(overall.Seconds()),
		PerCheckTimeoutSeconds: int(perCheck.Seconds()),
		StartedAt:              started,
		CompletedAt:            completed,
		Artifacts:              []Artifact{},
		Results:                results,
	}
	if req.Scope == ScopeAgentSync || req.Scope == ScopeRelease || req.Scope == ScopeAll {
		envelope.RuntimeGateWarning = RuntimeGateWarning
	}
	return envelope, nil
}

func (e *Executor) runOne(ctx context.Context, def checkDef, timeout time.Duration) CheckResult {
	started := e.now()
	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	result := CheckResult{
		ID:        def.id,
		Scope:     def.scope,
		Name:      def.name,
		Severity:  def.severity,
		Status:    StatusPass,
		Verdict:   VerdictPass,
		StartedAt: started,
	}
	if def.command.Name == "" {
		result.Evidence = []Evidence{{Type: "builtin", Message: def.message}}
		result.CompletedAt = e.now()
		result.DurationMS = result.CompletedAt.Sub(result.StartedAt).Milliseconds()
		return result
	}
	command := def.command
	if command.Dir == "" {
		command.Dir = e.workDir
	}
	commandResult := e.runner.Run(checkCtx, command)
	completed := e.now()
	result.CompletedAt = completed
	result.DurationMS = completed.Sub(result.StartedAt).Milliseconds()
	if errors.Is(checkCtx.Err(), context.DeadlineExceeded) {
		result.Status = StatusBlocked
		result.Verdict = VerdictBlocked
		result.Severity = SeverityWarning
		result.Evidence = []Evidence{{Type: "timeout", Message: "check exceeded per-check timeout", Command: command.String()}}
		return result
	}
	output := truncateOutput(strings.TrimSpace(commandResult.Stdout + "\n" + commandResult.Stderr))
	result.Evidence = []Evidence{{
		Type:     "command",
		Message:  "read-only command completed",
		Command:  command.String(),
		ExitCode: commandResult.ExitCode,
		Output:   output,
	}}
	if commandResult.ExitCode != 0 || commandResult.Err != nil {
		result.Status = StatusFail
		result.Verdict = VerdictFail
		result.Severity = SeverityCritical
		if commandResult.Err != nil {
			result.Evidence[0].Message = commandResult.Err.Error()
		}
	}
	return result
}

func (r Request) validate() error {
	switch r.Scope {
	case ScopeHealth, ScopeContract, ScopeAgentSync, ScopeSmoke, ScopeStorage, ScopeConfig, ScopeRelease, ScopeAll:
	default:
		return fmt.Errorf("invalid self-check scope %q", r.Scope)
	}
	switch r.Mode {
	case ModeFocused, ModeBroad:
	default:
		return fmt.Errorf("invalid self-check mode %q", r.Mode)
	}
	return nil
}

func ExitCode(verdict Verdict) int {
	switch verdict {
	case VerdictPass:
		return 0
	case VerdictFail:
		return 1
	case VerdictBlocked:
		return 2
	default:
		return 3
	}
}

type checkDef struct {
	id       string
	scope    Scope
	name     string
	severity Severity
	message  string
	command  Command
}

func checksForScope(scope Scope, mode Mode) []checkDef {
	byScope := map[Scope][]checkDef{
		ScopeHealth: {{
			id: "orag.selfcheck.health.runtime", scope: ScopeHealth, name: "Runtime health", severity: SeverityInfo,
			message: "Self-check executor is responsive and can return structured results.",
		}},
		ScopeConfig: {{
			id: "orag.selfcheck.config.runtime", scope: ScopeConfig, name: "Runtime configuration", severity: SeverityInfo,
			message: "Runtime timeout configuration is syntactically valid for MCP self-check execution.",
		}},
		ScopeStorage: {{
			id: "orag.selfcheck.storage.readiness", scope: ScopeStorage, name: "Storage readiness", severity: SeverityInfo,
			message: "Storage readiness is reported as a read-only placeholder until live dependency probes are configured.",
		}},
		ScopeAgentSync: {{
			id: "orag.selfcheck.agent_sync.artifacts", scope: ScopeAgentSync, name: "Agent artifact drift", severity: SeverityCritical,
			command: Command{Name: "make", Args: []string{"agent-sync-check"}},
		}},
		ScopeContract: {{
			id: "orag.selfcheck.contract.openapi", scope: ScopeContract, name: "OpenAPI contract", severity: SeverityCritical,
			command: Command{Name: "go", Args: []string{"test", "./tests/contract", "-run", "TestOpenAPI", "-v"}},
		}},
		ScopeSmoke: {{
			id: "orag.selfcheck.smoke.mcp_discovery", scope: ScopeSmoke, name: "MCP discovery smoke", severity: SeverityCritical,
			command: Command{Name: "go", Args: []string{"test", "./internal/mcp", "-run", "TestServerInitializeAndListTools", "-v"}},
		}},
	}
	switch scope {
	case ScopeRelease:
		return appendScopes(byScope, ScopeAgentSync, ScopeContract, ScopeSmoke, ScopeConfig)
	case ScopeAll:
		return appendScopes(byScope, ScopeHealth, ScopeConfig, ScopeAgentSync, ScopeContract, ScopeSmoke, ScopeStorage)
	default:
		return byScope[scope]
	}
}

func appendScopes(byScope map[Scope][]checkDef, scopes ...Scope) []checkDef {
	var defs []checkDef
	for _, scope := range scopes {
		defs = append(defs, byScope[scope]...)
	}
	return defs
}

func aggregateVerdict(results []CheckResult) Verdict {
	verdict := VerdictPass
	for _, result := range results {
		switch result.Verdict {
		case VerdictFail:
			return VerdictFail
		case VerdictBlocked:
			verdict = VerdictBlocked
		}
	}
	return verdict
}

func hasBlocked(results []CheckResult) bool {
	for _, result := range results {
		if result.Verdict == VerdictBlocked {
			return true
		}
	}
	return false
}

func blockedResult(def checkDef, started, completed time.Time, message string) CheckResult {
	return CheckResult{
		ID:          def.id,
		Scope:       def.scope,
		Name:        def.name,
		Severity:    SeverityWarning,
		Status:      StatusBlocked,
		Verdict:     VerdictBlocked,
		Evidence:    []Evidence{{Type: "deadline", Message: message, Command: def.command.String()}},
		StartedAt:   started,
		CompletedAt: completed,
		DurationMS:  completed.Sub(started).Milliseconds(),
	}
}

func durationFromSeconds(seconds int, fallback time.Duration) time.Duration {
	if seconds <= 0 {
		return fallback
	}
	return time.Duration(seconds) * time.Second
}

func truncateOutput(output string) string {
	const limit = 4096
	if len(output) <= limit {
		return output
	}
	return output[:limit] + "...[truncated]"
}

func newTraceID() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err == nil {
		return "selfcheck_" + hex.EncodeToString(buf[:])
	}
	return fmt.Sprintf("selfcheck_%d", time.Now().UnixNano())
}

type ShellRunner struct{}

func (ShellRunner) Run(ctx context.Context, command Command) CommandResult {
	cmd := exec.CommandContext(ctx, command.Name, command.Args...)
	if command.Dir != "" {
		cmd.Dir = command.Dir
	}
	cmd.Env = os.Environ()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result := CommandResult{ExitCode: 0, Stdout: stdout.String(), Stderr: stderr.String(), Err: err}
	if err == nil {
		return result
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		return result
	}
	result.ExitCode = 1
	return result
}
