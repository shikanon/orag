# Public Performance Baseline Evidence Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Publish and verify a safe, versioned deterministic-mock performance baseline evidence directory.

**Architecture:** Bash capture and verification scripts wrap the existing public SDK benchmark command. A static evidence directory stores the report, safe environment disclosure, strict manifest and checksums; the documentation site links directly to those immutable artifacts.

**Tech Stack:** Bash, Go `oragctl`, `jq`, SHA-256, static HTML, Go contract tests.

## Global Constraints

- The report must use `orag.performance-baseline.v1` and deterministic mock only.
- No artifact may disclose hostname, username, IP address, credential, `.env` value or provider configuration.
- A public report is local regression evidence only, never a production or cross-hardware claim.
- The first evidence directory must be tied to build revision `75eda8f80787d205e16e4ff7f65096bcd8926888`.

---

### Task 1: Add failing evidence contract test

**Files:**
- Create: `tests/contract/performance_baseline_evidence_test.go`
- Modify: `Makefile`

**Interfaces:**
- Produces contract requirements for `performance-baseline-evidence-verify`,
  static evidence files and hosted documentation links.

- [ ] Write a contract test which requires the evidence manifest, report,
  environment disclosure, checksum list, capture/verify scripts and the docs
  page wording.
- [ ] Run `go test ./tests/contract -run TestPublicPerformanceBaselineEvidence -v` and confirm it fails before artifacts exist.
- [ ] Add the Make target shape used by the future verifier.

### Task 2: Implement capture and verification scripts

**Files:**
- Create: `scripts/capture-performance-baseline-evidence.sh`
- Create: `scripts/verify-performance-baseline-evidence.sh`

**Interfaces:**
- Consumes: `--output DIR --build-revision REV` at capture time and `--dir DIR` at verification time.
- Produces: `report.json`, `environment.json`, `manifest.json`, `SHA256SUMS`.

- [ ] Make capture require `go`, `jq`, `shasum`, and a clean explicit output directory; call `oragctl benchmark-run` with the provided revision.
- [ ] Generate environment disclosure using an allowlisted JSON schema and calculate checksums after all three JSON artifacts exist.
- [ ] Make verifier use `shasum -a 256 -c`, enforce exact manifest/environment keys with `jq`, compare the manifest revision to report provenance, and invoke `oragctl benchmark-report`.
- [ ] Run capture into `.tmp/performance-baseline-evidence` and verify it; expected output starts with `verified public performance baseline evidence`.

### Task 3: Capture the first public artifact and expose it in docs

**Files:**
- Create: `docs-site/benchmarks/2026-07-17-darwin-arm64-main-75eda8f/{report.json,environment.json,manifest.json,SHA256SUMS}`
- Modify: `docs/benchmarks/performance-baseline-contract.md`
- Modify: `docs-site/performance-baseline.html`
- Modify: `docs-site/index.html`

**Interfaces:**
- Produces static HTTPS artifacts linked as the first disclosed local baseline.

- [ ] Capture from the exact main revision into the named directory.
- [ ] Verify the committed directory with the Make target.
- [ ] Document the immutable result, command, constraints and direct artifact links without describing metrics as portable performance.

### Task 4: Complete repository verification and release

**Files:**
- Modify: `ROADMAP.md`, `ROADMAP_EN.md`

**Interfaces:**
- Produces an accurate roadmap note that a disclosed public local baseline is available while external/production criteria remain open.

- [ ] Run the focused contract test, evidence verifier, `./scripts/build-docs-site.sh`, `git diff --check`, and `make agent-gate`.
- [ ] Commit, push, create a PR, wait for all required GitHub checks, squash merge, sync `main`, deploy docs and verify the public checksum/report URLs.
