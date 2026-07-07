# Role Scenario Demos Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add runnable role-based ORAG demos for customer support, engineering, platform, product, and agent developer users.

**Architecture:** Each role scenario owns a `main.go` program and `demo-data.md` input material. The Go demos use the public `pkg/memory` facade through a shared `examples/scenarios/internal/demo` helper, so they run without PostgreSQL, Qdrant, Ark, or a live API service. Contract tests verify every role folder has a Go entrypoint, data file, sample input, expected output, README usage dimensions, and referenced assets.

**Tech Stack:** Go, `pkg/memory`, Markdown, Go contract tests, existing ORAG examples under `examples/mcp`, `examples/skills`, and `examples/go`.

---

### Task 1: Role Demo Entrypoints

**Files:**
- Create: `examples/scenarios/internal/demo/demo.go`
- Create: `examples/scenarios/customer-support/main.go`
- Create: `examples/scenarios/engineering-runbook/main.go`
- Create: `examples/scenarios/platform-team/main.go`
- Create: `examples/scenarios/product-team/main.go`
- Create: `examples/scenarios/agent-developer/main.go`

- [x] **Step 1: Add shared Go scenario runner**

The shared helper must load role data, add it to `pkg/memory`, run a query, read trace metadata, and print usage dimensions.

```go
type Scenario struct {
	ID string
	Title string
	Role string
	BusinessGoal string
	UserQuestion string
	DemoDataPaths []string
	Dimensions []Dimension
}
```

- [x] **Step 2: Add role-specific main packages**

Each role package must call the shared runner with concrete role data and must not import root `internal/*` packages.

- [x] **Step 3: Remove shell wrappers**

Delete the old `run.sh` files so the primary demo path is Go:

```sh
go run ./examples/scenarios/customer-support
go run ./examples/scenarios/engineering-runbook
go run ./examples/scenarios/platform-team
go run ./examples/scenarios/product-team
go run ./examples/scenarios/agent-developer
```

Expected: each command prints `scenario=`, `document_id=doc_`, `answer=Found`, `trace_id=`, `usage_dimensions`, `expected_signals`, and `recommended_next_steps`.

### Task 2: Role Demo Data

**Files:**
- Create: `examples/scenarios/customer-support/demo-data.md`
- Create: `examples/scenarios/engineering-runbook/demo-data.md`
- Create: `examples/scenarios/platform-team/demo-data.md`
- Create: `examples/scenarios/product-team/demo-data.md`
- Create: `examples/scenarios/agent-developer/demo-data.md`

- [x] **Step 1: Add role-specific material**

Each data file must include concrete source material that explains the role context, the user question, and the expected ORAG capability.

- [x] **Step 2: Link data from README**

Each role README must mention `demo-data.md`, `main.go`, and `pkg/memory`, and must include `## Usage Dimensions`.

### Task 3: Contract Tests

**Files:**
- Modify: `tests/contract/examples_test.go`

- [x] **Step 1: Extend scenario assertions**

The test must require each role demo to include:

```go
"demo-data.md",
"main.go",
"## Demo Implementation",
"## Usage Dimensions",
```

- [x] **Step 2: Assert Go package boundaries**

Add a helper that rejects imports of `github.com/shikanon/orag/internal/` from role demo `main.go` files.

- [x] **Step 3: Run focused tests**

Run:

```sh
GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./tests/contract -run TestExamples -v
```

Expected: PASS.

### Task 4: Validation and PR

**Files:**
- Modify: `examples/README.md`

- [x] **Step 1: Document role demo commands**

The top-level examples README must point each role scenario at `go run ./examples/scenarios/<role>`.

- [ ] **Step 2: Run example validation**

Run:

```sh
GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./examples/... -v
GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./tests/contract -v
```

Expected: PASS.

- [ ] **Step 3: Commit and open PR**

Commit only role scenario files, examples README, contract tests, and this plan. Push `feat/role-scenario-demos` and open a PR against `main`.
