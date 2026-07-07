package selfops

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPlanDryRunIncludesTOCTOUFieldsAndDoesNotWrite(t *testing.T) {
	runner := &recordingRunner{}
	executor := NewExecutor(Options{
		Snapshots: staticSnapshots(snapshotA),
		Runner:    runner,
		Now:       fixedClock(),
	})

	plan, err := executor.Plan(context.Background(), PlanRequest{
		Scope:   ScopeAgentArtifacts,
		DryRun:  true,
		TraceID: "trace_plan",
	})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if plan.SchemaVersion != SchemaVersion || plan.TraceID != "trace_plan" || plan.Verdict != VerdictPass {
		t.Fatalf("unexpected identity fields: %#v", plan)
	}
	if !plan.DryRun || plan.IdempotencyKey == "" || plan.LockKey != "selfops:agent-artifacts" {
		t.Fatalf("missing dry-run safety fields: %#v", plan)
	}
	if plan.Snapshot.ManifestHash != snapshotA.ManifestHash || len(plan.Preconditions) != 3 {
		t.Fatalf("unexpected snapshot/preconditions: %#v", plan)
	}
	if len(plan.Rollback) == 0 || len(plan.VerificationCommands) == 0 || !strings.Contains(plan.VerificationCommands[0], "agent-sync-check") {
		t.Fatalf("missing rollback or verification commands: %#v", plan)
	}
	if len(plan.Steps) != 1 {
		t.Fatalf("plan steps = %d, want 1: %#v", len(plan.Steps), plan.Steps)
	}
	wantWrites := []string{"agent/mcp/", "agent/skills/"}
	if !reflect.DeepEqual(plan.Steps[0].Writes, wantWrites) {
		t.Fatalf("step writes = %#v, want %#v", plan.Steps[0].Writes, wantWrites)
	}
	for _, hiddenInstallPath := range []string{".mcp", ".trae/skills", ".codex/skills", ".claude/skills"} {
		if containsString(plan.Steps[0].Writes, hiddenInstallPath) {
			t.Fatalf("step writes include hidden install path %q: %#v", hiddenInstallPath, plan.Steps[0].Writes)
		}
	}
	if runner.calls() != 0 {
		t.Fatalf("dry-run plan executed writes, calls=%d", runner.calls())
	}
}

func TestApplyBlocksWhenSourceAgentArtifactsDrift(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		original    string
		replacement string
	}{
		{
			name:        "mcp tool artifact",
			path:        "agent/mcp/tools/orag-self-check.json",
			original:    `{"name":"orag_check","version":1}`,
			replacement: `{"name":"orag_check","version":2}`,
		},
		{
			name:        "skill artifact",
			path:        "agent/skills/codex/orag-self-check/SKILL.md",
			original:    "# ORAG Self Check\n\nUse for read-only checks.\n",
			replacement: "# ORAG Self Check\n\nDrifted source artifact.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workDir := t.TempDir()
			artifacts := map[string]string{
				"agent/mcp/tools/orag-self-check.json":              `{"name":"orag_check","version":1}`,
				"agent/skills/codex/orag-self-check/SKILL.md":       "# ORAG Self Check\n\nUse for read-only checks.\n",
				"agent/skills/claude-code/orag-self-check/SKILL.md": "# ORAG Self Check\n\nUse for Claude Code checks.\n",
			}
			artifacts[tt.path] = tt.original
			for path, content := range artifacts {
				writeTestFile(t, workDir, path, content)
			}

			runner := &recordingRunner{}
			executor := NewExecutor(Options{
				WorkDir:   workDir,
				Snapshots: FileSnapshotProvider{WorkDir: workDir},
				Runner:    runner,
				Now:       fixedClock(),
			})
			plan, err := executor.Plan(context.Background(), PlanRequest{Scope: ScopeAgentArtifacts, DryRun: true})
			if err != nil {
				t.Fatalf("Plan() error = %v", err)
			}

			writeTestFile(t, workDir, tt.path, tt.replacement)
			result, err := executor.Apply(context.Background(), ApplyRequest{PlanID: plan.PlanID, Approved: true})
			if err != nil {
				t.Fatalf("Apply() error = %v", err)
			}
			if result.Verdict != VerdictBlocked || !strings.Contains(result.BlockedReason, "generated_artifacts_hash") {
				t.Fatalf("unexpected drift result: %#v", result)
			}
			if runner.calls() != 0 {
				t.Fatalf("runner calls = %d, want 0", runner.calls())
			}
		})
	}
}

func TestApplyBlocksWhenPreconditionsDrift(t *testing.T) {
	snapshots := sequenceSnapshots(snapshotA, Snapshot{
		ManifestHash:           "sha256:manifest-b",
		GitHead:                snapshotA.GitHead,
		ConfigHash:             snapshotA.ConfigHash,
		GeneratedArtifactsHash: snapshotA.GeneratedArtifactsHash,
	})
	executor := NewExecutor(Options{
		Snapshots: snapshots,
		Runner:    &recordingRunner{},
		Now:       fixedClock(),
	})
	plan, err := executor.Plan(context.Background(), PlanRequest{Scope: ScopeAgentArtifacts, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}

	result, err := executor.Apply(context.Background(), ApplyRequest{PlanID: plan.PlanID, Approved: true})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if result.Verdict != VerdictBlocked || !strings.Contains(result.BlockedReason, "manifest_hash") {
		t.Fatalf("unexpected drift result: %#v", result)
	}
}

func TestApplyRequiresApprovalAndKnownPlan(t *testing.T) {
	executor := NewExecutor(Options{Snapshots: staticSnapshots(snapshotA), Runner: &recordingRunner{}, Now: fixedClock()})
	plan, err := executor.Plan(context.Background(), PlanRequest{Scope: ScopeAgentArtifacts, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}

	unapproved, err := executor.Apply(context.Background(), ApplyRequest{PlanID: plan.PlanID, Approved: false})
	if err != nil {
		t.Fatalf("Apply() unapproved error = %v", err)
	}
	if unapproved.Verdict != VerdictBlocked || !strings.Contains(unapproved.BlockedReason, "approval") {
		t.Fatalf("unexpected unapproved result: %#v", unapproved)
	}
	missing, err := executor.Apply(context.Background(), ApplyRequest{PlanID: "plan_missing", Approved: true})
	if err != nil {
		t.Fatalf("Apply() missing error = %v", err)
	}
	if missing.Verdict != VerdictBlocked || !strings.Contains(missing.BlockedReason, "plan not found") {
		t.Fatalf("unexpected missing plan result: %#v", missing)
	}
}

func TestApplyUsesIdempotencyKeyForRepeatRequests(t *testing.T) {
	runner := &recordingRunner{output: "ok"}
	executor := NewExecutor(Options{Snapshots: staticSnapshots(snapshotA), Runner: runner, Now: fixedClock()})
	plan, err := executor.Plan(context.Background(), PlanRequest{Scope: ScopeAgentArtifacts, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}

	first, err := executor.Apply(context.Background(), ApplyRequest{PlanID: plan.PlanID, Approved: true, TraceID: "trace_first"})
	if err != nil {
		t.Fatalf("first Apply() error = %v", err)
	}
	second, err := executor.Apply(context.Background(), ApplyRequest{PlanID: plan.PlanID, Approved: true, TraceID: "trace_second"})
	if err != nil {
		t.Fatalf("second Apply() error = %v", err)
	}
	if first.Verdict != VerdictPass || second.Status != StatusSkipped || !second.IdempotentReplay {
		t.Fatalf("unexpected idempotency results first=%#v second=%#v", first, second)
	}
	if runner.calls() != 1 {
		t.Fatalf("runner calls = %d, want 1", runner.calls())
	}
}

func TestApplySingleFlightBlocksConcurrentApply(t *testing.T) {
	runner := newBlockingRunner()
	executor := NewExecutor(Options{Snapshots: staticSnapshots(snapshotA), Runner: runner, Now: fixedClock()})
	plan, err := executor.Plan(context.Background(), PlanRequest{Scope: ScopeAgentArtifacts, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = executor.Apply(context.Background(), ApplyRequest{PlanID: plan.PlanID, Approved: true})
	}()
	runner.waitStarted(t)

	blocked, err := executor.Apply(context.Background(), ApplyRequest{PlanID: plan.PlanID, Approved: true})
	if err != nil {
		t.Fatalf("concurrent Apply() error = %v", err)
	}
	if blocked.Verdict != VerdictBlocked || !strings.Contains(blocked.BlockedReason, "already running") {
		t.Fatalf("unexpected single-flight result: %#v", blocked)
	}
	runner.release()
	wg.Wait()
}

func TestApplyRedactsSensitiveOutput(t *testing.T) {
	runner := &recordingRunner{output: "generated with token_secret and password"}
	executor := NewExecutor(Options{Snapshots: staticSnapshots(snapshotA), Runner: runner, Now: fixedClock()})
	plan, err := executor.Plan(context.Background(), PlanRequest{Scope: ScopeAgentArtifacts, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}

	result, err := executor.Apply(context.Background(), ApplyRequest{PlanID: plan.PlanID, Approved: true})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if strings.Contains(result.Output, "token_secret") || strings.Contains(result.Output, "password") {
		t.Fatalf("sensitive output leaked: %#v", result)
	}
	if !strings.Contains(result.Output, "[redacted]") {
		t.Fatalf("output was not redacted: %#v", result.Output)
	}
}

var snapshotA = Snapshot{
	ManifestHash:           "sha256:manifest-a",
	GitHead:                "git-a",
	ConfigHash:             "sha256:config-a",
	GeneratedArtifactsHash: "sha256:artifacts-a",
}

type staticSnapshots Snapshot

func (s staticSnapshots) Snapshot(context.Context, string) (Snapshot, error) {
	return Snapshot(s), nil
}

type snapshotSequence []Snapshot

func (s *snapshotSequence) Snapshot(context.Context, string) (Snapshot, error) {
	if len(*s) == 0 {
		return Snapshot{}, nil
	}
	next := (*s)[0]
	if len(*s) > 1 {
		*s = (*s)[1:]
	}
	return next, nil
}

func sequenceSnapshots(first Snapshot, rest ...Snapshot) SnapshotProvider {
	seq := snapshotSequence(append([]Snapshot{first}, rest...))
	return &seq
}

type recordingRunner struct {
	mu     sync.Mutex
	count  int
	output string
}

func (r *recordingRunner) Run(context.Context, Action) ActionResult {
	r.mu.Lock()
	r.count++
	r.mu.Unlock()
	return ActionResult{Output: r.output}
}

func (r *recordingRunner) calls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.count
}

type blockingRunner struct {
	started chan struct{}
	done    chan struct{}
	once    sync.Once
}

func newBlockingRunner() *blockingRunner {
	return &blockingRunner{started: make(chan struct{}), done: make(chan struct{})}
}

func (r *blockingRunner) Run(context.Context, Action) ActionResult {
	r.once.Do(func() { close(r.started) })
	<-r.done
	return ActionResult{}
}

func (r *blockingRunner) waitStarted(t *testing.T) {
	t.Helper()
	select {
	case <-r.started:
	case <-time.After(time.Second):
		t.Fatal("runner did not start")
	}
}

func (r *blockingRunner) release() {
	close(r.done)
}

func fixedClock() func() time.Time {
	current := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	return func() time.Time {
		current = current.Add(time.Millisecond)
		return current
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func writeTestFile(t *testing.T, workDir, path, content string) {
	t.Helper()
	fullPath := filepath.Join(workDir, path)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(fullPath), err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", fullPath, err)
	}
}
