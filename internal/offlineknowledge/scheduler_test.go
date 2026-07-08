package offlineknowledge

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestSchedulerDisabledDoesNotStartOrTrigger(t *testing.T) {
	clock := newFakeSchedulerClock(time.Date(2026, 7, 8, 1, 0, 0, 0, time.UTC))
	runner := &fakeSchedulerRunner{}
	scheduler := NewScheduler(runner, SchedulerConfig{
		Enabled:      false,
		Schedule:     "* * * * *",
		LookbackDays: 7,
		Targets:      []SchedulerTarget{{TenantID: "tenant_1", KBID: "kb_1"}},
	}, SchedulerOptions{Clock: clock})

	if err := scheduler.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if scheduler.IsRunning() {
		t.Fatal("IsRunning() = true, want false for disabled scheduler")
	}
	if got := scheduler.Trigger(context.Background(), clock.Now()); got != nil {
		t.Fatalf("Trigger() = %#v, want nil for disabled scheduler", got)
	}
	if clock.afterCalls() != 0 {
		t.Fatalf("After() calls = %d, want 0", clock.afterCalls())
	}
	if runner.callCount() != 0 {
		t.Fatalf("RunOnce() calls = %d, want 0", runner.callCount())
	}
}

func TestSchedulerEnabledTriggerRunsOncePerTarget(t *testing.T) {
	now := time.Date(2026, 7, 8, 2, 0, 30, 0, time.UTC)
	runner := &fakeSchedulerRunner{}
	scheduler := NewScheduler(runner, SchedulerConfig{
		Enabled:            true,
		Schedule:           "0 2 * * *",
		LookbackDays:       3,
		MaxQuestionsPerRun: 50,
		MaxClustersPerRun:  20,
		Targets: []SchedulerTarget{
			{TenantID: "tenant_1", KBID: "kb_1"},
			{TenantID: "tenant_1", KBID: "kb_2"},
		},
		ConfigJSON: map[string]any{"version": "v1"},
	}, SchedulerOptions{})

	results := scheduler.Trigger(context.Background(), now)
	if len(results) != 2 {
		t.Fatalf("Trigger() results = %d, want 2", len(results))
	}
	if runner.callCount() != 2 {
		t.Fatalf("RunOnce() calls = %d, want 2", runner.callCount())
	}
	first := runner.requests()[0]
	if first.TenantID != "tenant_1" || first.KBID != "kb_1" {
		t.Fatalf("RunOnce() target = %s/%s, want tenant_1/kb_1", first.TenantID, first.KBID)
	}
	wantEnd := now.UTC().Truncate(time.Minute)
	if !first.WindowEnd.Equal(wantEnd) || !first.WindowStart.Equal(wantEnd.Add(-72*time.Hour)) {
		t.Fatalf("RunOnce() window = %s..%s, want %s..%s", first.WindowStart, first.WindowEnd, wantEnd.Add(-72*time.Hour), wantEnd)
	}
	if first.MaxQuestions != 50 || first.MaxClusters != 20 {
		t.Fatalf("RunOnce() limits = %d/%d, want 50/20", first.MaxQuestions, first.MaxClusters)
	}
	if first.ConfigHash == "" {
		t.Fatal("RunOnce() ConfigHash is empty")
	}
}

func TestSchedulerDeduplicatesInFlightWindowAndConfig(t *testing.T) {
	now := time.Date(2026, 7, 8, 2, 0, 0, 0, time.UTC)
	runner := &fakeSchedulerRunner{
		block:   make(chan struct{}),
		started: make(chan struct{}),
	}
	scheduler := NewScheduler(runner, SchedulerConfig{
		Enabled:      true,
		Schedule:     "0 2 * * *",
		LookbackDays: 1,
		Targets:      []SchedulerTarget{{TenantID: "tenant_1", KBID: "kb_1"}},
		ConfigJSON:   map[string]any{"version": "v1"},
	}, SchedulerOptions{})

	firstDone := make(chan []SchedulerRunResult, 1)
	go func() {
		firstDone <- scheduler.Trigger(context.Background(), now)
	}()
	<-runner.started

	second := scheduler.Trigger(context.Background(), now)
	if len(second) != 1 || !second[0].Deduplicated {
		t.Fatalf("second Trigger() = %#v, want in-flight deduplicated result", second)
	}
	if runner.callCount() != 1 {
		t.Fatalf("RunOnce() calls while in-flight = %d, want 1", runner.callCount())
	}
	close(runner.block)
	first := <-firstDone
	if len(first) != 1 || first[0].Err != nil {
		t.Fatalf("first Trigger() = %#v, want successful result", first)
	}
}

func TestSchedulerReportsRunFailure(t *testing.T) {
	runErr := errors.New("run failed")
	runner := &fakeSchedulerRunner{err: runErr}
	scheduler := NewScheduler(runner, SchedulerConfig{
		Enabled:      true,
		Schedule:     "* * * * *",
		LookbackDays: 1,
		Targets:      []SchedulerTarget{{TenantID: "tenant_1", KBID: "kb_1"}},
	}, SchedulerOptions{})

	results := scheduler.Trigger(context.Background(), time.Date(2026, 7, 8, 2, 0, 0, 0, time.UTC))
	if len(results) != 1 {
		t.Fatalf("Trigger() results = %d, want 1", len(results))
	}
	if !errors.Is(results[0].Err, runErr) {
		t.Fatalf("Trigger() error = %v, want %v", results[0].Err, runErr)
	}
}

func TestSchedulerGracefulStop(t *testing.T) {
	clock := newFakeSchedulerClock(time.Date(2026, 7, 8, 1, 59, 0, 0, time.UTC))
	scheduler := NewScheduler(&fakeSchedulerRunner{}, SchedulerConfig{
		Enabled:      true,
		Schedule:     "0 2 * * *",
		LookbackDays: 1,
		Targets:      []SchedulerTarget{{TenantID: "tenant_1", KBID: "kb_1"}},
	}, SchedulerOptions{Clock: clock})

	if err := scheduler.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	waitForCondition(t, func() bool { return clock.afterCalls() == 1 })
	if !scheduler.IsRunning() {
		t.Fatal("IsRunning() = false, want true after Start")
	}
	if err := scheduler.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if scheduler.IsRunning() {
		t.Fatal("IsRunning() = true, want false after Stop")
	}
}

type fakeSchedulerRunner struct {
	mu       sync.Mutex
	reqs     []RunRequest
	err      error
	block    chan struct{}
	started  chan struct{}
	startOne sync.Once
}

func (r *fakeSchedulerRunner) RunOnce(_ context.Context, request RunRequest) (RunResult, error) {
	r.mu.Lock()
	r.reqs = append(r.reqs, request)
	r.mu.Unlock()
	if r.started != nil {
		r.startOne.Do(func() { close(r.started) })
	}
	if r.block != nil {
		<-r.block
	}
	return RunResult{Deduplicated: false}, r.err
}

func (r *fakeSchedulerRunner) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.reqs)
}

func (r *fakeSchedulerRunner) requests() []RunRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]RunRequest(nil), r.reqs...)
}

type fakeSchedulerClock struct {
	mu    sync.Mutex
	now   time.Time
	after int
}

func newFakeSchedulerClock(now time.Time) *fakeSchedulerClock {
	return &fakeSchedulerClock{now: now}
}

func (c *fakeSchedulerClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeSchedulerClock) After(time.Duration) <-chan time.Time {
	c.mu.Lock()
	c.after++
	c.mu.Unlock()
	return make(chan time.Time)
}

func (c *fakeSchedulerClock) afterCalls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.after
}

func waitForCondition(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition was not met before deadline")
}
