# Public Go SDK Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Provide an embeddable root-module Go SDK for ORAG ingestion, query, evaluation, and trace workflows without exposing `internal/*` types.

**Architecture:** Package `github.com/shikanon/orag` is a public facade over the existing application composition root and domain services. Explicit public configuration maps to a complete validated internal configuration; `NewFromEnv` remains available for service-style embedding. Public DTOs and typed errors isolate callers from internal packages, while the HTTP binary continues to use the same application services.

**Tech Stack:** Go 1.26, existing ORAG application services, PostgreSQL/pgx, Qdrant, deterministic mock provider, Go examples and external consumer module.

## Global Constraints

- Public signatures must not expose any `internal/*` type.
- `New` with explicit configuration must not depend on ambient environment variables.
- Deterministic mock mode must be explicit and must never impersonate a production provider.
- `NewFromEnv` keeps current service configuration behavior.
- `Close` is nil-safe and idempotent.
- SDK errors support `errors.Is` and `errors.As`, preserve causes, and do not expose secrets.
- The first Beta surface covers knowledge bases, text/file ingestion, synchronous and typed streaming query, datasets/evaluations, readiness, and trace lookup.

---

### Task 1: Explicit public configuration and client lifecycle

**Files:**
- Create: `orag.go`
- Create: `config.go`
- Create: `errors.go`
- Create: `orag_test.go`
- Modify: `internal/app/app.go`
- Modify: `cmd/orag-api/main.go`

**Interfaces:**
- Produces: `func New(context.Context, Config) (*Client, error)`, `func NewFromEnv(context.Context) (*Client, error)`, `func MockConfig() Config`, `func (c *Client) Close() error`, and `func (c *Client) Readiness(context.Context) (Readiness, error)`.

- [ ] **Step 1: Write the failing external-package lifecycle test**

```go
package orag_test

func TestMockClientLifecycleDoesNotReadEnvironment(t *testing.T) {
    t.Setenv("LLM_CHAT_PROVIDER", "volcengine")
    t.Setenv("ARK_API_KEY", "")
    client, err := orag.New(context.Background(), orag.MockConfig())
    if err != nil { t.Fatal(err) }
    if err := client.Close(); err != nil { t.Fatal(err) }
    if err := client.Close(); err != nil { t.Fatal(err) }
}
```

- [ ] **Step 2: Run and confirm failure**

Run: `go test . -run TestMockClientLifecycleDoesNotReadEnvironment -v`

Expected: FAIL because the root package and constructors do not exist.

- [ ] **Step 3: Add public configuration and constructors**

Define public `Config`, `StorageConfig`, `ModelConfig`, `RAGConfig`, and `IngestionConfig`. `MockConfig` returns memory storage, all providers set to `mock`, `AllowDeterministicMock: true`, four embedding dimensions, basic parsing, and conservative RAG defaults. The private mapper builds a complete `internal/config.Config` literal and calls `Validate` before opening dependencies.

- [ ] **Step 4: Make application close idempotent**

Add `sync.Once` and a stored close error inside `internal/app.App`; keep all existing closers and reverse ordering. The SDK guards its app pointer with its own `sync.Once` so nil clients are safe.

- [ ] **Step 5: Run lifecycle and service tests**

Run: `go test . ./internal/app ./cmd/orag-api -v`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add orag.go config.go errors.go orag_test.go internal/app/app.go cmd/orag-api/main.go
git commit -m "feat: add public sdk lifecycle"
```

### Task 2: Knowledge-base, ingestion, query, and trace facade

**Files:**
- Create: `types.go`
- Modify: `orag.go`
- Create: `workflow_test.go`

**Interfaces:**
- Produces: `CreateKnowledgeBase`, `ListKnowledgeBases`, `GetKnowledgeBase`, `DeleteKnowledgeBase`, `IngestText`, `IngestFile`, `GetIngestionJob`, `Query`, `GetTrace`, and `ListTraces`.

- [ ] **Step 1: Write the failing end-to-end SDK test**

Create a mock client, create a knowledge base, ingest deterministic text, query it with a fixed trace ID, fetch the trace, list the knowledge base, delete it, and assert a second delete returns `ErrNotFound`.

```go
resp, err := client.Query(ctx, orag.QueryRequest{
    KnowledgeBaseID: kb.ID,
    Query: "What is ORAG?",
    TraceID: "trace_sdk_workflow",
})
if err != nil { t.Fatal(err) }
if resp.TraceID != "trace_sdk_workflow" || len(resp.Citations) == 0 { t.Fatalf("%#v", resp) }
```

- [ ] **Step 2: Run and confirm failure**

Run: `go test . -run TestSDKKnowledgeWorkflow -v`

Expected: FAIL because the facade methods do not exist.

- [ ] **Step 3: Implement public DTO mapping and operations**

Use the client's default tenant when a request tenant is empty. Copy maps and slices at the boundary. Generate knowledge-base IDs through the existing internal ID helper. `IngestFile` accepts `io.Reader`, a name, source URI, and size limit; it delegates to the same ingest service as text ingestion.

- [ ] **Step 4: Normalize domain failures**

Map validation, missing knowledge base, missing dataset, cancellation, deadline, conflict, and unavailable errors through the public error wrapper. Preserve the original cause for `errors.Is` and `errors.As`.

- [ ] **Step 5: Run focused and owning-package tests**

Run: `go test . ./internal/ingest ./internal/rag ./internal/app -v`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add orag.go types.go errors.go workflow_test.go
git commit -m "feat: expose core rag workflow in go sdk"
```

### Task 3: Dataset, evaluation, and typed query stream

**Files:**
- Create: `evaluation.go`
- Create: `stream.go`
- Modify: `types.go`
- Create: `evaluation_test.go`
- Create: `stream_test.go`

**Interfaces:**
- Produces: `CreateDataset`, `AddDatasetItem`, `RunEvaluation`, `GetEvaluation`, and `StreamQuery` returning `<-chan QueryEvent`.

- [ ] **Step 1: Write failing evaluation and stream tests**

The evaluation test creates a dataset and item from the ingested document, runs the deterministic evaluation, fetches it by ID, and asserts metric output. The stream test cancels a context and asserts exactly one terminal event and a closed channel.

```go
events := client.StreamQuery(ctx, request)
for event := range events {
    if event.Type == orag.QueryEventDone { done++ }
    if event.Type == orag.QueryEventError { streamErr = event.Err }
}
```

- [ ] **Step 2: Run and confirm failure**

Run: `go test . -run 'TestSDK(Evaluation|Stream)' -v`

Expected: FAIL because evaluation and streaming methods do not exist.

- [ ] **Step 3: Implement evaluation DTOs and mappings**

Expose only rule-based Beta fields required by the current runner: dataset identity, query, ground truth, relevant document IDs, split, weight, evaluation ID, totals, hit rate, accuracy, latency, metrics, and timestamps.

- [ ] **Step 4: Implement typed stream behavior**

`StreamQuery` starts one goroutine, calls the same `Query` method as synchronous callers, sends a response event followed by a done event, and closes the channel. Context cancellation sends one error event wrapping `context.Canceled` or `context.DeadlineExceeded`. This accurately reflects the current core, which computes the full answer before the HTTP SSE adapter emits events.

- [ ] **Step 5: Run tests**

Run: `go test . ./internal/dataset ./internal/eval ./internal/rag -v`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add evaluation.go stream.go types.go evaluation_test.go stream_test.go
git commit -m "feat: add evaluation and streaming sdk APIs"
```

### Task 4: Standalone downstream consumer proof

**Files:**
- Create: `tests/consumer/go.mod`
- Create: `tests/consumer/main.go`
- Create: `tests/consumer/main_test.go`
- Create: `tests/contract/public_sdk_test.go`
- Modify: `Makefile`
- Modify: `.github/workflows/ci.yml`

**Interfaces:**
- Consumes: public package APIs from Tasks 1-3.
- Produces: `make sdk-check`, which compiles and runs a separate module and rejects exported `internal/` types.

- [ ] **Step 1: Add the failing contract and consumer**

Use `go list -json github.com/shikanon/orag` and `go doc` output to reject public signatures containing `/internal/`. The consumer imports only `github.com/shikanon/orag`, runs mock create/ingest/query/evaluate/trace, and closes the client.

- [ ] **Step 2: Run and confirm the consumer fails before wiring**

Run: `go test ./tests/contract -run TestPublicSDK -v && (cd tests/consumer && go test ./...)`

Expected: FAIL until the consumer module replacement and complete surface are present.

- [ ] **Step 3: Add `sdk-check` and CI**

The Make target runs root external-package tests, the consumer module with `GOWORK=off`, `go vet`, and the public API leak test. CI invokes it before Docker build.

- [ ] **Step 4: Run the gate**

Run: `make sdk-check`

Expected: PASS without importing any internal package.

- [ ] **Step 5: Commit**

```bash
git add tests/consumer tests/contract/public_sdk_test.go Makefile .github/workflows/ci.yml
git commit -m "test: verify public sdk from downstream module"
```

### Task 5: Public documentation and examples

**Files:**
- Create: `docs/sdk/README.md`
- Create: `examples/go/sdk/main.go`
- Modify: `README.md`
- Modify: `README_EN.md`
- Modify: `docs/README.md`
- Modify: `docs/compatibility.md`
- Modify: `CHANGELOG.md`

**Interfaces:**
- Consumes: public SDK and `make sdk-check`.
- Produces: pkg.go.dev-compatible package comments, runnable no-key example, production configuration guidance, and Beta limitations.

- [ ] **Step 1: Add failing example/index assertions**

Extend contract tests to require `examples/go/sdk`, `docs/sdk/README.md`, and links from both READMEs and the docs index.

- [ ] **Step 2: Run and confirm failure**

Run: `go test ./tests/contract -run 'Test(SDK|ExamplesReadmeIndex)' -v`

Expected: FAIL because docs and example are absent.

- [ ] **Step 3: Add documentation and runnable example**

Document explicit mock versus PostgreSQL/Qdrant configuration, lifecycle, tenant behavior, typed errors, stream semantics, concurrency, limitations, and migration expectations. Do not claim feature stability.

- [ ] **Step 4: Run full validation**

Run: `make test vet openapi-validate agent-gate sdk-check && npm --prefix console test -- --run && npm --prefix console run build && git diff --check`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add docs/sdk examples/go/sdk README.md README_EN.md docs/README.md docs/compatibility.md CHANGELOG.md tests/contract
git commit -m "docs: publish public go sdk guide"
```

### Task 6: Publish and merge the SDK PR

**Files:**
- No planned source changes after validation.

- [ ] **Step 1: Rebase and push**

Run: `git fetch origin && git rebase origin/main && git push -u origin codex/public-go-sdk`

- [ ] **Step 2: Create a ready-for-review PR**

Include the external consumer proof, public surface summary, stream semantics, error categories, and exact validation evidence.

- [ ] **Step 3: Wait for checks and merge**

Run `gh pr checks --watch`, then squash merge and delete the branch only after all checks pass.

- [ ] **Step 4: Verify main**

Read back the merge commit, switch to `main`, fast-forward from origin, and confirm `main...origin/main` is `0 0` before starting release engineering.
