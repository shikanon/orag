# Role Scenario Demos Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add runnable role-based ORAG demos for customer support, engineering, platform, product, and agent developer users.

**Architecture:** Each role scenario owns a `run.sh` orchestrator and `demo-data.md` input material. The scripts reuse maintained curl, MCP, Skill, and Go examples instead of duplicating raw API calls. Contract tests verify every role folder has a runnable entry, data file, sample input, expected output, README references, and existing asset references.

**Tech Stack:** POSIX shell, Markdown, Go contract tests, existing ORAG examples under `examples/curl`, `examples/mcp`, `examples/skills`, and `examples/go`.

---

### Task 1: Role Demo Entrypoints

**Files:**
- Create: `examples/scenarios/customer-support/run.sh`
- Create: `examples/scenarios/engineering-runbook/run.sh`
- Create: `examples/scenarios/platform-team/run.sh`
- Create: `examples/scenarios/product-team/run.sh`
- Create: `examples/scenarios/agent-developer/run.sh`

- [ ] **Step 1: Add executable shell wrappers**

Each wrapper must:

```sh
#!/usr/bin/env sh
set -eu
SCENARIO_DIR="$(CDPATH= cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(CDPATH= cd "$SCENARIO_DIR/../../.." && pwd)"
cd "$REPO_ROOT"
```

- [ ] **Step 2: Reuse maintained commands**

Service-mode role demos must run existing `examples/curl/*.sh` scripts. Agent demos must run MCP and agent sync commands. The scripts must not duplicate HTTP endpoints.

- [ ] **Step 3: Make scripts executable**

Run:

```sh
chmod +x examples/scenarios/customer-support/run.sh examples/scenarios/engineering-runbook/run.sh examples/scenarios/platform-team/run.sh examples/scenarios/product-team/run.sh examples/scenarios/agent-developer/run.sh
```

Expected: all five files are executable.

### Task 2: Role Demo Data

**Files:**
- Create: `examples/scenarios/customer-support/demo-data.md`
- Create: `examples/scenarios/engineering-runbook/demo-data.md`
- Create: `examples/scenarios/platform-team/demo-data.md`
- Create: `examples/scenarios/product-team/demo-data.md`
- Create: `examples/scenarios/agent-developer/demo-data.md`

- [ ] **Step 1: Add role-specific material**

Each data file must include concrete source material that explains the role context, the user question, and the expected ORAG capability.

- [ ] **Step 2: Link data from README**

Each role README must mention `demo-data.md` in the scenario files section and explain that the current service scripts reuse maintained sample payloads.

### Task 3: Contract Tests

**Files:**
- Modify: `tests/contract/examples_test.go`

- [ ] **Step 1: Extend scenario assertions**

The test must require each role demo to include:

```go
"demo-data.md",
"run.sh",
"## Demo Implementation",
```

- [ ] **Step 2: Assert executable mode**

Add a helper that checks `run.sh` has at least one executable bit set.

- [ ] **Step 3: Run focused tests**

Run:

```sh
GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./tests/contract -run TestExamples -v
```

Expected: PASS.

### Task 4: Validation and PR

**Files:**
- Modify: `examples/README.md`

- [ ] **Step 1: Document role demo commands**

The top-level examples README must point each role scenario at `run.sh`.

- [ ] **Step 2: Run example validation**

Run:

```sh
GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./examples/... -v
GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./tests/contract -v
```

Expected: PASS.

- [ ] **Step 3: Commit and open PR**

Commit only role scenario files, examples README, contract tests, and this plan. Push `feat/role-scenario-demos` and open a PR against `main`.
