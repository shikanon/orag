# Reproducible Performance Baseline Runner Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Generate a strict, reproducible local mock performance-baseline report from an actual SDK workload.

**Architecture:** A root-package runner owns the public deterministic workload and invokes the existing embedded mock SDK. The CLI only validates flags and atomically serializes the report; the existing internal benchmark package remains the single validation contract.

**Tech Stack:** Go 1.26, public ORAG Go SDK, `internal/benchmark`, standard-library JSON/crypto/time, Make.

## Global Constraints

- The runner must always use `MockConfig`; never read provider credentials or ambient runtime configuration.
- The report must satisfy `benchmark.Validate` before being written.
- Workload input and runtime fingerprints are SHA-256 values over explicit canonical JSON values.
- Results are local mock baseline evidence, never a production or cross-hardware claim.

---

### Task 1: Add the deterministic SDK benchmark runner

**Files:**
- Create: `performance_baseline.go`
- Create: `performance_baseline_test.go`

**Interfaces:**
- Produces: `DefaultPerformanceBaselineOptions() PerformanceBaselineOptions` and `RunMockPerformanceBaseline(context.Context, PerformanceBaselineOptions) ([]byte, error)`. The returned bytes are one validated `orag.performance-baseline.v1` JSON value and do not expose an `internal/*` type.
- Consumes: `New`, `MockConfig`, `Client.IngestText`, `Client.Query`, `Client.CreateDataset`, `Client.AddDatasetItem`, and `Client.RunEvaluation`.

- [ ] **Step 1: Write failing runner tests**

```go
func TestRunMockPerformanceBaselineProducesValidObservedReport(t *testing.T) {
    raw, err := orag.RunMockPerformanceBaseline(context.Background(), orag.DefaultPerformanceBaselineOptions())
    if err != nil { t.Fatal(err) }
    report, err := benchmark.Parse(raw)
    if err := benchmark.Validate(report); err != nil { t.Fatal(err) }
    if report.Load.MeasuredRequests != 20 || report.Metrics.IngestionDocuments < 1 { t.Fatal(report) }
}
```

- [ ] **Step 2: Run the focused test and verify it fails**

Run: `GOTOOLCHAIN=go1.26.5 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test . -run TestRunMockPerformanceBaselineProducesValidObservedReport -v`

Expected: FAIL because the exported runner does not exist.

- [ ] **Step 3: Implement the fixed workload and runner**

```go
type PerformanceBaselineOptions struct {
    BuildRevision string
    WarmupRequests int
    MeasuredRequests int
    Concurrency int
}

func RunMockPerformanceBaseline(ctx context.Context, opts PerformanceBaselineOptions) ([]byte, error) {
    // Execute MockConfig ingestion/query/evaluation and return validated report JSON.
}
```

- [ ] **Step 4: Add negative and deterministic-fingerprint tests**

```go
func TestRunMockPerformanceBaselineRejectsInvalidOptions(t *testing.T) { /* blank revision and fewer than 20 measurements */ }
func TestMockPerformanceBaselineWorkloadFingerprintIsStable(t *testing.T) { /* exact 64-char SHA-256 */ }
```

- [ ] **Step 5: Run package tests**

Run: `GOTOOLCHAIN=go1.26.5 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test . -run 'Test(RunMockPerformanceBaseline|MockPerformanceBaseline)' -v`

Expected: PASS.

### Task 2: Add the safe CLI report writer

**Files:**
- Modify: `cmd/oragctl/main.go`
- Modify: `cmd/oragctl/main_test.go`

**Interfaces:**
- Consumes: `orag.RunMockPerformanceBaseline` and `benchmark.Parse`.
- Produces: `oragctl benchmark-run --output <file> --build-revision <revision>`.

- [ ] **Step 1: Write failing CLI tests**

```go
func TestBenchmarkRunCmdWritesVerifiedReport(t *testing.T) {
    output := filepath.Join(t.TempDir(), "report.json")
    if err := benchmarkRunCmd([]string{"--output", output, "--build-revision", "test"}, &bytes.Buffer{}); err != nil { t.Fatal(err) }
    _, err := benchmark.Parse(mustRead(t, output))
    if err != nil { t.Fatal(err) }
}
```

- [ ] **Step 2: Run CLI test and verify it fails**

Run: `GOTOOLCHAIN=go1.26.5 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./cmd/oragctl -run TestBenchmarkRunCmdWritesVerifiedReport -v`

Expected: FAIL because the command is absent.

- [ ] **Step 3: Implement flag parsing and atomic JSON output**

```go
case "benchmark-run":
    if err := benchmarkRunCmd(os.Args[2:], os.Stdout); err != nil { log.Fatalf("benchmark-run: %v", err) }
```

- [ ] **Step 4: Add invalid-argument and no-partial-file tests**

```go
func TestBenchmarkRunCmdRejectsInvalidInputWithoutOutput(t *testing.T) { /* invalid measured count leaves no file */ }
```

- [ ] **Step 5: Run CLI tests**

Run: `GOTOOLCHAIN=go1.26.5 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./cmd/oragctl -run TestBenchmarkRunCmd -v`

Expected: PASS.

### Task 3: Document and gate generation

**Files:**
- Modify: `Makefile`
- Modify: `docs/benchmarks/performance-baseline-contract.md`
- Modify: `docs-site/performance-baseline.html`
- Modify: `tests/contract/benchmark_runner_test.go`

**Interfaces:**
- Consumes: the CLI `benchmark-run` and `benchmark-report` commands.
- Produces: `make benchmark-report-run` and public instructions that distinguish local mock evidence from production claims.

- [ ] **Step 1: Write the failing contract test**

```go
func TestPerformanceBaselineRunnerIsDocumented(t *testing.T) {
    // assert Makefile, Markdown and hosted page mention benchmark-run and benchmark-report-verify
}
```

- [ ] **Step 2: Run it and verify it fails**

Run: `GOTOOLCHAIN=go1.26.5 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./tests/contract -run TestPerformanceBaselineRunnerIsDocumented -v`

Expected: FAIL before generation instructions are present.

- [ ] **Step 3: Add Make target and documentation**

```make
benchmark-report-run:
	@test -n "$(BENCHMARK_REPORT)" || (echo "BENCHMARK_REPORT must point to the output JSON file"; exit 2)
	CGO_ENABLED="$(CGO_ENABLED)" GOFLAGS="$(GOFLAGS)" go run ./cmd/oragctl benchmark-run --output "$(BENCHMARK_REPORT)" --build-revision "$$(git rev-parse HEAD)"
```

- [ ] **Step 4: Run focused contracts and documentation build**

Run: `GOTOOLCHAIN=go1.26.5 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./tests/contract -run TestPerformanceBaselineRunnerIsDocumented -v && ./scripts/build-docs-site.sh`

Expected: PASS and `_site/performance-baseline.html` exists.

### Task 4: Verify and publish

**Files:**
- Modify: `ROADMAP.md`
- Modify: `ROADMAP_EN.md`

- [ ] **Step 1: Mark the report generator accurately in both roadmaps**

Use language that states the local deterministic mock runner is complete while disclosed-hardware/provider public comparisons remain pending.

- [ ] **Step 2: Run the complete gate**

Run: `make agent-gate`

Expected: PASS.

- [ ] **Step 3: Commit, push, open a PR, wait for required checks, squash merge, sync `main`, deploy docs, and verify the hosted performance page returns HTTP 200**

Run: use the repository's protected-main workflow and `/Users/bytedance/.ssh/id_rsa` for the documented server deployment.

Expected: merged `main`, clean worktree except user-owned untracked files, and a live hosted page.
