package selfops_test

import (
	"context"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/shikanon/orag/internal/agentsync"
	"github.com/shikanon/orag/internal/capabilities"
	"github.com/shikanon/orag/internal/selfops"
)

func TestPlanAgentArtifactWritesMatchGeneratedManifestRoots(t *testing.T) {
	runner := &recordingRunner{}
	executor := selfops.NewExecutor(selfops.Options{
		Snapshots: staticSnapshots(snapshotA),
		Runner:    runner,
		Now:       fixedClock(),
	})

	plan, err := executor.Plan(context.Background(), selfops.PlanRequest{
		Scope:   selfops.ScopeAgentArtifacts,
		DryRun:  true,
		TraceID: "trace_plan",
	})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(plan.Steps) != 1 {
		t.Fatalf("steps = %#v, want one step", plan.Steps)
	}
	want := generatedManifestWriteRoots(t)
	if !reflect.DeepEqual(plan.Steps[0].Writes, want) {
		t.Fatalf("step writes = %#v, want generated manifest roots %#v", plan.Steps[0].Writes, want)
	}
	if !reflect.DeepEqual(plan.Steps[0].Commands, []string{"make agent-sync"}) {
		t.Fatalf("step commands = %#v, want make agent-sync unchanged", plan.Steps[0].Commands)
	}
	if runner.calls != 0 {
		t.Fatalf("dry-run plan executed writes, calls=%d", runner.calls)
	}
}

var snapshotA = selfops.Snapshot{
	ManifestHash:           "sha256:manifest-a",
	GitHead:                "git-a",
	ConfigHash:             "sha256:config-a",
	GeneratedArtifactsHash: "sha256:artifacts-a",
}

type staticSnapshots selfops.Snapshot

func (s staticSnapshots) Snapshot(context.Context, string) (selfops.Snapshot, error) {
	return selfops.Snapshot(s), nil
}

type recordingRunner struct {
	calls int
}

func (r *recordingRunner) Run(context.Context, selfops.Action) selfops.ActionResult {
	r.calls++
	return selfops.ActionResult{}
}

func fixedClock() func() time.Time {
	current := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	return func() time.Time {
		current = current.Add(time.Millisecond)
		return current
	}
}

func generatedManifestWriteRoots(t *testing.T) []string {
	t.Helper()
	files, err := agentsync.GenerateFromManifest(capabilities.MustBuiltinManifest())
	if err != nil {
		t.Fatalf("GenerateFromManifest() error = %v", err)
	}
	roots := map[string]bool{}
	for _, file := range files {
		path := filepath.ToSlash(file.Path)
		switch {
		case strings.HasPrefix(path, "agent/mcp/"):
			roots["agent/mcp/"] = true
		case strings.HasPrefix(path, "agent/skills/"):
			roots["agent/skills/"] = true
		default:
			t.Fatalf("generated file %q is outside expected agent artifact roots", path)
		}
	}
	got := make([]string, 0, len(roots))
	for root := range roots {
		got = append(got, root)
	}
	sort.Strings(got)
	return got
}
