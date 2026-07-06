package selfops

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	SchemaVersion = "orag.selfops.result.v1"

	ScopeAgentArtifacts = "agent_artifacts"

	VerdictPass    = "pass"
	VerdictBlocked = "blocked"
	VerdictInvalid = "invalid"

	StatusPlanned   = "planned"
	StatusCompleted = "completed"
	StatusBlocked   = "blocked"
	StatusSkipped   = "skipped"
	StatusInvalid   = "invalid"
)

type PlanRequest struct {
	Scope   string
	DryRun  bool
	TraceID string
}

type ApplyRequest struct {
	PlanID   string
	Approved bool
	TraceID  string
}

type Plan struct {
	SchemaVersion        string         `json:"schema_version"`
	TraceID              string         `json:"trace_id"`
	PlanID               string         `json:"plan_id"`
	Scope                string         `json:"scope"`
	Action               string         `json:"action"`
	Verdict              string         `json:"verdict"`
	Status               string         `json:"status"`
	DryRun               bool           `json:"dry_run"`
	IdempotencyKey       string         `json:"idempotency_key"`
	LockKey              string         `json:"lock_key"`
	Snapshot             Snapshot       `json:"snapshot"`
	Preconditions        []Precondition `json:"preconditions"`
	Steps                []Step         `json:"steps"`
	Rollback             []string       `json:"rollback"`
	VerificationCommands []string       `json:"verification_commands"`
	CreatedAt            time.Time      `json:"created_at"`
}

type Snapshot struct {
	ManifestHash           string `json:"manifest_hash"`
	GitHead                string `json:"git_head"`
	ConfigHash             string `json:"config_hash"`
	GeneratedArtifactsHash string `json:"generated_artifacts_hash"`
}

type Precondition struct {
	ID       string `json:"id"`
	Actual   string `json:"actual"`
	Expected string `json:"expected"`
}

type Step struct {
	ID          string   `json:"id"`
	Description string   `json:"description"`
	Commands    []string `json:"commands"`
	Writes      []string `json:"writes"`
}

type ApplyResult struct {
	SchemaVersion        string   `json:"schema_version"`
	TraceID              string   `json:"trace_id"`
	PlanID               string   `json:"plan_id"`
	Verdict              string   `json:"verdict"`
	Status               string   `json:"status"`
	IdempotencyKey       string   `json:"idempotency_key"`
	LockKey              string   `json:"lock_key"`
	IdempotentReplay     bool     `json:"idempotent_replay,omitempty"`
	BlockedReason        string   `json:"blocked_reason,omitempty"`
	CompletedActions     []string `json:"completed_actions,omitempty"`
	VerificationCommands []string `json:"verification_commands,omitempty"`
	Output               string   `json:"output,omitempty"`
}

type SnapshotProvider interface {
	Snapshot(ctx context.Context, scope string) (Snapshot, error)
}

type ActionRunner interface {
	Run(ctx context.Context, action Action) ActionResult
}

type Action struct {
	PlanID   string
	Scope    string
	Name     string
	Commands []string
	Writes   []string
}

type ActionResult struct {
	Output string
	Err    error
}

type Options struct {
	WorkDir   string
	Snapshots SnapshotProvider
	Runner    ActionRunner
	Now       func() time.Time
}

type Executor struct {
	workDir   string
	snapshots SnapshotProvider
	runner    ActionRunner
	now       func() time.Time

	mu        sync.Mutex
	plans     map[string]Plan
	completed map[string]ApplyResult
	locks     map[string]bool
}

func NewExecutor(options Options) *Executor {
	now := options.Now
	if now == nil {
		now = time.Now
	}
	snapshots := options.Snapshots
	if snapshots == nil {
		snapshots = FileSnapshotProvider{WorkDir: options.WorkDir}
	}
	runner := options.Runner
	if runner == nil {
		runner = ShellActionRunner{WorkDir: options.WorkDir}
	}
	return &Executor{
		workDir:   options.WorkDir,
		snapshots: snapshots,
		runner:    runner,
		now:       now,
		plans:     map[string]Plan{},
		completed: map[string]ApplyResult{},
		locks:     map[string]bool{},
	}
}

func (e *Executor) Plan(ctx context.Context, req PlanRequest) (Plan, error) {
	if e == nil {
		e = NewExecutor(Options{})
	}
	if err := validatePlanRequest(req); err != nil {
		return Plan{}, err
	}
	snapshot, err := e.snapshots.Snapshot(ctx, req.Scope)
	if err != nil {
		return Plan{}, fmt.Errorf("capture snapshot: %w", err)
	}
	traceID := strings.TrimSpace(req.TraceID)
	if traceID == "" {
		traceID = newID("selfops")
	}
	action := actionForScope(req.Scope)
	planID := "plan_" + fingerprint(snapshot.ManifestHash + snapshot.GitHead + snapshot.GeneratedArtifactsHash)[:16]
	plan := Plan{
		SchemaVersion:        SchemaVersion,
		TraceID:              traceID,
		PlanID:               planID,
		Scope:                req.Scope,
		Action:               action.Name,
		Verdict:              VerdictPass,
		Status:               StatusPlanned,
		DryRun:               true,
		IdempotencyKey:       "selfops:" + action.Name + ":" + fingerprint(snapshot.ManifestHash+"|"+snapshot.GeneratedArtifactsHash),
		LockKey:              lockKey(req.Scope),
		Snapshot:             snapshot,
		Preconditions:        preconditions(snapshot),
		Steps:                []Step{{ID: "selfops.agent_artifacts.regenerate", Description: "Regenerate agent artifacts from the capability manifest.", Commands: action.Commands, Writes: action.Writes}},
		Rollback:             []string{"Discard generated artifacts and regenerate a fresh plan if any precondition drifts."},
		VerificationCommands: []string{"make agent-sync-check"},
		CreatedAt:            e.now(),
	}
	e.mu.Lock()
	e.plans[plan.PlanID] = plan
	e.mu.Unlock()
	return plan, nil
}

func (e *Executor) Apply(ctx context.Context, req ApplyRequest) (ApplyResult, error) {
	if e == nil {
		e = NewExecutor(Options{})
	}
	if strings.TrimSpace(req.PlanID) == "" {
		return ApplyResult{}, errors.New("plan_id is required")
	}
	traceID := strings.TrimSpace(req.TraceID)
	if traceID == "" {
		traceID = newID("selfops")
	}
	e.mu.Lock()
	plan, ok := e.plans[req.PlanID]
	e.mu.Unlock()
	if !ok {
		return blockedApply(traceID, req.PlanID, "", "", "plan not found; regenerate a dry-run maintenance plan"), nil
	}
	if !req.Approved {
		return blockedApply(traceID, plan.PlanID, plan.IdempotencyKey, plan.LockKey, "explicit approval is required for low-risk apply"), nil
	}
	if actionForScope(plan.Scope).Name == "" {
		return blockedApply(traceID, plan.PlanID, plan.IdempotencyKey, plan.LockKey, "scope is not authorized for low-risk apply"), nil
	}
	if replay, ok := e.completedResult(plan.IdempotencyKey); ok {
		replay.TraceID = traceID
		replay.IdempotentReplay = true
		replay.Status = StatusSkipped
		return replay, nil
	}
	if !e.tryLock(plan.LockKey) {
		return blockedApply(traceID, plan.PlanID, plan.IdempotencyKey, plan.LockKey, "another apply is already running for this lock key"), nil
	}
	defer e.unlock(plan.LockKey)

	current, err := e.snapshots.Snapshot(ctx, plan.Scope)
	if err != nil {
		return blockedApply(traceID, plan.PlanID, plan.IdempotencyKey, plan.LockKey, "snapshot recapture failed"), nil
	}
	if drift := driftReason(plan.Snapshot, current); drift != "" {
		return blockedApply(traceID, plan.PlanID, plan.IdempotencyKey, plan.LockKey, drift+"; regenerate the maintenance plan"), nil
	}
	action := actionForScope(plan.Scope)
	run := e.runner.Run(ctx, Action{PlanID: plan.PlanID, Scope: plan.Scope, Name: action.Name, Commands: action.Commands, Writes: action.Writes})
	if run.Err != nil {
		return blockedApply(traceID, plan.PlanID, plan.IdempotencyKey, plan.LockKey, "low-risk action failed: "+sanitize(run.Err.Error())), nil
	}
	result := ApplyResult{
		SchemaVersion:        SchemaVersion,
		TraceID:              traceID,
		PlanID:               plan.PlanID,
		Verdict:              VerdictPass,
		Status:               StatusCompleted,
		IdempotencyKey:       plan.IdempotencyKey,
		LockKey:              plan.LockKey,
		CompletedActions:     []string{action.Name},
		VerificationCommands: plan.VerificationCommands,
		Output:               sanitize(run.Output),
	}
	e.mu.Lock()
	e.completed[plan.IdempotencyKey] = result
	e.mu.Unlock()
	return result, nil
}

func validatePlanRequest(req PlanRequest) error {
	if strings.TrimSpace(req.Scope) == "" {
		return errors.New("scope is required")
	}
	if !req.DryRun {
		return errors.New("orag_maintenance_plan only supports dry_run=true")
	}
	if actionForScope(req.Scope).Name == "" {
		return fmt.Errorf("scope %q is not supported for maintenance planning", req.Scope)
	}
	return nil
}

func actionForScope(scope string) Action {
	switch scope {
	case ScopeAgentArtifacts:
		return Action{
			Scope:    scope,
			Name:     "agent-artifacts-regenerate",
			Commands: []string{"make agent-sync"},
			Writes:   []string{"agent/mcp/", "agent/skills/"},
		}
	default:
		return Action{}
	}
}

func preconditions(snapshot Snapshot) []Precondition {
	return []Precondition{
		{ID: "git_head", Actual: snapshot.GitHead, Expected: snapshot.GitHead},
		{ID: "manifest_hash", Actual: snapshot.ManifestHash, Expected: snapshot.ManifestHash},
		{ID: "generated_artifacts_hash", Actual: snapshot.GeneratedArtifactsHash, Expected: snapshot.GeneratedArtifactsHash},
	}
}

func driftReason(expected, actual Snapshot) string {
	switch {
	case actual.GitHead != expected.GitHead:
		return "git_head precondition drifted"
	case actual.ManifestHash != expected.ManifestHash:
		return "manifest_hash precondition drifted"
	case actual.GeneratedArtifactsHash != expected.GeneratedArtifactsHash:
		return "generated_artifacts_hash precondition drifted"
	default:
		return ""
	}
}

func blockedApply(traceID, planID, idempotencyKey, lockKey, reason string) ApplyResult {
	return ApplyResult{
		SchemaVersion:  SchemaVersion,
		TraceID:        traceID,
		PlanID:         planID,
		Verdict:        VerdictBlocked,
		Status:         StatusBlocked,
		IdempotencyKey: idempotencyKey,
		LockKey:        lockKey,
		BlockedReason:  sanitize(reason),
	}
}

func (e *Executor) completedResult(key string) (ApplyResult, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	result, ok := e.completed[key]
	return result, ok
}

func (e *Executor) tryLock(key string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.locks[key] {
		return false
	}
	e.locks[key] = true
	return true
}

func (e *Executor) unlock(key string) {
	e.mu.Lock()
	delete(e.locks, key)
	e.mu.Unlock()
}

func lockKey(scope string) string {
	return "selfops:" + strings.ReplaceAll(scope, "_", "-")
}

func fingerprint(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func sanitize(value string) string {
	replacers := []string{"token_secret", "api_token", "password", "secret"}
	out := value
	for _, secret := range replacers {
		out = strings.ReplaceAll(out, secret, "[redacted]")
	}
	if len(out) > 4096 {
		return out[:4096] + "...[truncated]"
	}
	return out
}

func newID(prefix string) string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err == nil {
		return prefix + "_" + hex.EncodeToString(buf[:])
	}
	return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
}

type FileSnapshotProvider struct {
	WorkDir string
}

func (p FileSnapshotProvider) Snapshot(ctx context.Context, _ string) (Snapshot, error) {
	workDir := p.WorkDir
	if workDir == "" {
		workDir = "."
	}
	return Snapshot{
		ManifestHash:           hashPaths(workDir, "internal/capabilities"),
		GitHead:                gitHead(ctx, workDir),
		ConfigHash:             hashPaths(workDir, "Makefile", "api/openapi.yaml"),
		GeneratedArtifactsHash: hashPaths(workDir, "agent/mcp", "agent/skills"),
	}, nil
}

func hashPaths(workDir string, paths ...string) string {
	h := sha256.New()
	for _, path := range paths {
		full := filepath.Join(workDir, path)
		_ = filepath.WalkDir(full, func(current string, entry os.DirEntry, err error) error {
			if err != nil || entry == nil || entry.IsDir() {
				return nil
			}
			data, readErr := os.ReadFile(current)
			if readErr != nil {
				return nil
			}
			_, _ = h.Write([]byte(current))
			_, _ = h.Write(data)
			return nil
		})
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

func gitHead(ctx context.Context, workDir string) string {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	cmd.Dir = workDir
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "unknown"
	}
	return strings.TrimSpace(out.String())
}

type ShellActionRunner struct {
	WorkDir string
}

func (r ShellActionRunner) Run(ctx context.Context, action Action) ActionResult {
	var combined strings.Builder
	for _, command := range action.Commands {
		parts := strings.Fields(command)
		if len(parts) == 0 {
			continue
		}
		cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
		if r.WorkDir != "" {
			cmd.Dir = r.WorkDir
		}
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		combined.WriteString(stdout.String())
		combined.WriteString(stderr.String())
		if err != nil {
			return ActionResult{Output: combined.String(), Err: err}
		}
	}
	return ActionResult{Output: combined.String()}
}
