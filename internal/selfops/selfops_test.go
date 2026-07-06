package selfops

import (
	"context"
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
	if runner.calls() != 0 {
		t.Fatalf("dry-run plan executed writes, calls=%d", runner.calls())
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
