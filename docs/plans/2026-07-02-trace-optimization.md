# Trace Optimization Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Improve trace reliability, queryability, observability, and developer experience for the RAG query path.

**Architecture:** Keep the current lightweight PostgreSQL trace store as the source of record, then extend it with reliable failure persistence, deterministic span ordering, indexed reads, list APIs, and developer-facing tooling. OpenTelemetry integration remains additive and must not replace existing trace IDs or PostgreSQL records.

**Tech Stack:** Go, Hertz HTTP server, PostgreSQL migrations, Qdrant-backed RAG pipeline, `oragctl`, existing contract/integration tests.

---

## Scope

Included:
- Persist traces for failed graph executions.
- Make trace writes idempotent.
- Add deterministic span sequence and timing metadata.
- Add PostgreSQL indexes for trace reads.
- Add trace list/search capabilities for diagnostics.
- Add slow-node statistics.
- Structure trace warnings.
- Add HTTP trace read endpoints.
- Enrich SSE `done` trace summary.
- Extend `oragctl trace`.
- Add in-memory trace store for local debugging and tests.
- Add OpenTelemetry bridge points while preserving current trace behavior.

Excluded:
- Raw query redaction or masking.
- Error message sanitization.
- External `X-Trace-ID` format validation.
- Security/compliance-specific retention policy.
- Cross-tenant authorization design beyond what is required by existing authenticated APIs.

## Task 1: Persist Failed Graph Traces

**Files:**
- Modify: [`internal/graph/rag_graph.go`](../../internal/graph/rag_graph.go)
- Modify: [`internal/graph/state.go`](../../internal/graph/state.go)
- Modify: [`internal/graph/nodes.go`](../../internal/graph/nodes.go)
- Test: [`internal/graph/rag_graph_test.go`](../../internal/graph/rag_graph_test.go)

**Step 1: Write failing graph test**

Add a test where one graph node returns an error after earlier nodes have recorded spans.

Expected behavior:
- `Invoke` still returns the original graph error.
- `TraceStore.StoreTrace` is called.
- Stored spans include executed nodes.
- At least one stored span contains the node error.

**Step 2: Run the failing test**

Run:

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./internal/graph -run TestGraphPersistsFailedTrace -v
```

Expected: FAIL because `RAGGraph.Invoke` currently returns before trace persistence.

**Step 3: Implement failure persistence**

Keep `TraceStore.StoreTrace` best-effort:
- On graph success, keep current behavior.
- On graph failure, persist the partial state if available.
- Return the original graph error even if trace persistence fails.
- If trace persistence fails on a failed query, log or attach a warning only when a response exists.

**Step 4: Run graph tests**

Run:

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./internal/graph -v
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/graph/rag_graph.go internal/graph/state.go internal/graph/nodes.go internal/graph/rag_graph_test.go
git commit -m "feat: persist failed rag traces"
```

## Task 2: Make Trace Writes Idempotent

**Files:**
- Modify: [`internal/storage/postgres/trace.go`](../../internal/storage/postgres/trace.go)
- Create: `migrations/000004_trace_span_idempotency.sql`
- Test: [`internal/storage/postgres/repository_test.go`](../../internal/storage/postgres/repository_test.go)

**Step 1: Write failing repository tests**

Add tests for duplicate `trace_id` writes:
- First `StoreTrace` inserts trace and spans.
- Second `StoreTrace` with the same `trace_id` does not duplicate spans.
- The final `GetTrace` result contains one span per recorded sequence.

**Step 2: Run the failing tests**

Run:

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./internal/storage/postgres -run TestStoreTraceIdempotent -v
```

Expected: FAIL because span inserts are not currently idempotent.

**Step 3: Add span identity**

Add a deterministic span key:
- Prefer `sequence` on `rag_node_spans`.
- Add unique index or constraint on `(trace_id, sequence)`.
- Keep `id` as the existing primary key for compatibility.

**Step 4: Update StoreTrace**

Change `StoreTrace` so:
- Trace row insert remains idempotent.
- Span rows use `ON CONFLICT (trace_id, sequence) DO NOTHING`.
- Repeated calls with the same spans leave the database unchanged.

**Step 5: Run storage tests**

Run:

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./internal/storage/postgres -v
```

Expected: PASS.

**Step 6: Commit**

```bash
git add internal/storage/postgres/trace.go internal/storage/postgres/repository_test.go migrations/000004_trace_span_idempotency.sql
git commit -m "feat: make trace storage idempotent"
```

## Task 3: Add Stable Span Ordering And Timing

**Files:**
- Modify: [`internal/graph/state.go`](../../internal/graph/state.go)
- Modify: [`internal/graph/nodes.go`](../../internal/graph/nodes.go)
- Modify: [`internal/storage/postgres/trace.go`](../../internal/storage/postgres/trace.go)
- Create: `migrations/000005_trace_span_timing.sql`
- Test: [`internal/graph/rag_graph_test.go`](../../internal/graph/rag_graph_test.go)
- Test: [`internal/storage/postgres/repository_test.go`](../../internal/storage/postgres/repository_test.go)

**Step 1: Write failing tests**

Add tests that assert:
- Spans have deterministic `Sequence` values in execution order.
- Spans have `StartedAt` and `EndedAt`.
- `GetTrace` orders spans by `sequence`, not insert timestamp.

**Step 2: Run the failing tests**

Run:

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./internal/graph ./internal/storage/postgres -run 'Trace|Span' -v
```

Expected: FAIL because `NodeSpan` has only `NodeName`, `LatencyMS`, and `Error`.

**Step 3: Extend NodeSpan**

Add:
- `Sequence int`
- `StartedAt time.Time`
- `EndedAt time.Time`

Ensure `withSpan` populates these fields consistently.

**Step 4: Extend PostgreSQL schema**

Add migration columns:
- `sequence INTEGER NOT NULL DEFAULT 0`
- `started_at TIMESTAMPTZ`
- `ended_at TIMESTAMPTZ`

Backfill existing rows in a deterministic order if needed.

**Step 5: Update GetTrace**

Read and return the new fields.

Sort spans by:

```sql
ORDER BY sequence, created_at, id
```

**Step 6: Run focused tests**

Run:

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./internal/graph ./internal/storage/postgres -v
```

Expected: PASS.

**Step 7: Commit**

```bash
git add internal/graph/state.go internal/graph/nodes.go internal/graph/rag_graph_test.go internal/storage/postgres/trace.go internal/storage/postgres/repository_test.go migrations/000005_trace_span_timing.sql
git commit -m "feat: add ordered trace span timing"
```

## Task 4: Add Trace Read Indexes

**Files:**
- Create: `migrations/000006_trace_read_indexes.sql`
- Test: [`tests/contract/openapi_test.go`](../../tests/contract/openapi_test.go) only if API docs change in later tasks.

**Step 1: Add migration**

Create indexes:

```sql
CREATE INDEX IF NOT EXISTS idx_rag_node_spans_trace_order
    ON rag_node_spans(trace_id, sequence, created_at, id);

CREATE INDEX IF NOT EXISTS idx_rag_traces_tenant_created
    ON rag_traces(tenant_id, created_at DESC, id);

CREATE INDEX IF NOT EXISTS idx_rag_traces_profile_created
    ON rag_traces(profile, created_at DESC, id);
```

**Step 2: Add down migration**

Drop the indexes in reverse order.

**Step 3: Run migration-related tests**

Run:

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./internal/storage/postgres ./tests/integration -v
```

Expected: PASS.

**Step 4: Commit**

```bash
git add migrations/000006_trace_read_indexes.sql
git commit -m "perf: add trace read indexes"
```

## Task 5: Add Trace List Repository API

**Files:**
- Modify: [`internal/storage/postgres/trace.go`](../../internal/storage/postgres/trace.go)
- Test: [`internal/storage/postgres/repository_test.go`](../../internal/storage/postgres/repository_test.go)

**Step 1: Define list query model**

Add:

```go
type TraceListFilter struct {
    TenantID string
    Profile rag.Profile
    Since time.Time
    Until time.Time
    HasError *bool
    SlowMS int64
    Limit int
}
```

Add:

```go
func (r *Repository) ListTraces(ctx context.Context, filter TraceListFilter) ([]TraceRecord, error)
```

**Step 2: Write failing tests**

Cover:
- Limit defaults and maximum.
- Filter by tenant.
- Filter by profile.
- Filter by time range.
- Filter by `SlowMS`.
- Filter by `HasError`.

**Step 3: Run failing tests**

Run:

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./internal/storage/postgres -run TestListTraces -v
```

Expected: FAIL because the API does not exist.

**Step 4: Implement SQL**

Implement list query using `rag_traces` and `EXISTS` over `rag_node_spans` for `HasError`.

Do not load node spans for list results unless needed by tests.

**Step 5: Run tests**

Run:

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./internal/storage/postgres -v
```

Expected: PASS.

**Step 6: Commit**

```bash
git add internal/storage/postgres/trace.go internal/storage/postgres/repository_test.go
git commit -m "feat: add trace list repository api"
```

## Task 6: Add Slow Node Statistics

**Files:**
- Modify: [`internal/storage/postgres/trace.go`](../../internal/storage/postgres/trace.go)
- Test: [`internal/storage/postgres/repository_test.go`](../../internal/storage/postgres/repository_test.go)

**Step 1: Define stats model**

Add:

```go
type TraceNodeStats struct {
    NodeName string `json:"node_name"`
    Count int64 `json:"count"`
    AvgLatencyMS float64 `json:"avg_latency_ms"`
    P95LatencyMS int64 `json:"p95_latency_ms"`
    P99LatencyMS int64 `json:"p99_latency_ms"`
    ErrorCount int64 `json:"error_count"`
}
```

Add:

```go
func (r *Repository) TraceNodeStats(ctx context.Context, tenantID string, since, until time.Time) ([]TraceNodeStats, error)
```

**Step 2: Write failing tests**

Test aggregation by node name with:
- Multiple traces.
- Mixed latencies.
- At least one error span.

**Step 3: Implement SQL aggregation**

Use PostgreSQL aggregate functions and percentile support.

Expected behavior:
- Results sorted by `p95_latency_ms DESC`.
- Empty range returns empty list.

**Step 4: Run tests**

Run:

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./internal/storage/postgres -run TestTraceNodeStats -v
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/storage/postgres/trace.go internal/storage/postgres/repository_test.go
git commit -m "feat: add trace node latency stats"
```

## Task 7: Structure Trace Warnings

**Files:**
- Modify: [`internal/rag/types.go`](../../internal/rag/types.go)
- Modify: [`internal/graph/rag_graph.go`](../../internal/graph/rag_graph.go)
- Modify: [`internal/http/sse.go`](../../internal/http/sse.go)
- Test: [`internal/graph/rag_graph_test.go`](../../internal/graph/rag_graph_test.go)
- Test: [`internal/http/router_test.go`](../../internal/http/router_test.go)

**Step 1: Add warning model**

Add:

```go
type Warning struct {
    Code string `json:"code"`
    Message string `json:"message"`
}
```

Keep existing `[]string` warnings only if needed for backward compatibility.

**Step 2: Write failing tests**

Cover:
- Trace store failure returns structured warning code `trace_store_failed`.
- SSE `done` event emits structured warnings.
- JSON query response remains compatible with existing tests.

**Step 3: Implement structured warnings**

Change trace store failure warning to structured form.

Avoid changing business error behavior.

**Step 4: Run tests**

Run:

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./internal/graph ./internal/http -v
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/rag/types.go internal/graph/rag_graph.go internal/graph/rag_graph_test.go internal/http/sse.go internal/http/router_test.go
git commit -m "feat: structure trace warnings"
```

## Task 8: Add HTTP Trace Read API

**Files:**
- Modify: [`api/openapi.yaml`](../../api/openapi.yaml)
- Modify: [`internal/http/router.go`](../../internal/http/router.go)
- Test: [`internal/http/router_test.go`](../../internal/http/router_test.go)
- Test: [`tests/contract/openapi_test.go`](../../tests/contract/openapi_test.go)
- Doc: [`docs/api.md`](../api.md)

**Step 1: Add OpenAPI paths**

Add:
- `GET /v1/traces/{trace_id}`
- `GET /v1/traces`

List query parameters:
- `profile`
- `since`
- `until`
- `has_error`
- `slow_ms`
- `limit`

**Step 2: Write failing HTTP tests**

Cover:
- Get trace by id returns trace record.
- Missing trace returns 404.
- List traces returns filtered records.
- Invalid query params return 400.

**Step 3: Add HTTP handlers**

Add handlers that delegate to repository interfaces.

Keep output JSON aligned with CLI trace output where practical.

**Step 4: Update docs**

Document examples in [`docs/api.md`](../api.md).

**Step 5: Run tests**

Run:

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./internal/http ./tests/contract -v
```

Expected: PASS.

**Step 6: Commit**

```bash
git add api/openapi.yaml docs/api.md internal/http/router.go internal/http/router_test.go tests/contract/openapi_test.go
git commit -m "feat: add trace http api"
```

## Task 9: Enrich SSE Trace Summary

**Files:**
- Modify: [`internal/rag/types.go`](../../internal/rag/types.go)
- Modify: [`internal/http/sse.go`](../../internal/http/sse.go)
- Test: [`internal/http/router_test.go`](../../internal/http/router_test.go)

**Step 1: Define response summary**

Add a compact trace summary to `rag.QueryResponse`:

```go
type TraceSummary struct {
    NodeCount int `json:"node_count"`
    SlowestNode string `json:"slowest_node,omitempty"`
    SlowestLatencyMS int64 `json:"slowest_latency_ms,omitempty"`
}
```

**Step 2: Write failing SSE test**

Assert `done` event contains:
- `trace_id`
- `latency_ms`
- `trace_summary.node_count`
- `trace_summary.slowest_node`

**Step 3: Populate summary**

Populate summary from graph spans before returning response.

**Step 4: Run HTTP tests**

Run:

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./internal/http -run TestStreamingQuery -v
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/rag/types.go internal/http/sse.go internal/http/router_test.go
git commit -m "feat: enrich sse trace summary"
```

## Task 10: Extend oragctl Trace

**Files:**
- Modify: [`cmd/oragctl/main.go`](../../cmd/oragctl/main.go)
- Test: [`cmd/oragctl/main_test.go`](../../cmd/oragctl/main_test.go)

**Step 1: Add flags**

Extend `oragctl trace` with:
- `--tenant-id`
- `--since`
- `--until`
- `--profile`
- `--has-error`
- `--slow-ms`
- `--limit`
- `--stats`

**Step 2: Write failing CLI tests**

Cover:
- Existing `--trace-id` behavior still works.
- List mode prints `{ "traces": [...] }`.
- Stats mode prints `{ "node_stats": [...] }`.
- Invalid time returns a helpful error.

**Step 3: Implement CLI mode selection**

Mode rules:
- `--trace-id` runs single lookup.
- `--stats` runs node stats.
- Otherwise run list traces.

**Step 4: Run CLI tests**

Run:

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./cmd/oragctl -v
```

Expected: PASS.

**Step 5: Commit**

```bash
git add cmd/oragctl/main.go cmd/oragctl/main_test.go
git commit -m "feat: extend trace cli queries"
```

## Task 11: Add Memory Trace Store

**Files:**
- Modify: [`internal/kb/types.go`](../../internal/kb/types.go)
- Modify: [`internal/app/app.go`](../../internal/app/app.go)
- Test: [`internal/kb/store_test.go`](../../internal/kb/store_test.go)
- Test: [`internal/app/app_test.go`](../../internal/app/app_test.go)

**Step 1: Define in-memory trace methods**

Add memory implementations equivalent to:
- `StoreTrace`
- `GetTrace`
- `ListTraces`
- `TraceNodeStats`

**Step 2: Write failing tests**

Cover:
- Store and get trace.
- Duplicate store is idempotent.
- List filters work.
- Node stats aggregate correctly.

**Step 3: Wire app backend**

When using the memory backend, ensure graph trace persistence has a working trace store.

**Step 4: Run focused tests**

Run:

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./internal/kb ./internal/app -v
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/kb/types.go internal/kb/store_test.go internal/app/app.go internal/app/app_test.go
git commit -m "feat: add memory trace store"
```

## Task 12: Add OpenTelemetry Bridge Points

**Files:**
- Modify: [`internal/observability/tracing.go`](../../internal/observability/tracing.go)
- Modify: [`internal/graph/nodes.go`](../../internal/graph/nodes.go)
- Test: [`internal/observability/tracing_test.go`](../../internal/observability/tracing_test.go)
- Test: [`internal/graph/rag_graph_test.go`](../../internal/graph/rag_graph_test.go)

**Step 1: Define minimal tracer abstraction**

Add an internal abstraction so production can keep no-op behavior unless an exporter is configured.

Do not require an OTel collector for tests.

**Step 2: Write failing tests**

Cover:
- `StartSpan` records span name through injected test tracer.
- Existing trace id remains unchanged.
- Graph node spans still populate PostgreSQL-oriented `NodeSpan`.

**Step 3: Implement bridge**

Wrap `StartSpan` so it can:
- Start an OTel span when configured.
- Fall back to no-op when not configured.
- Preserve the existing `X-Trace-ID` lifecycle.

**Step 4: Run observability tests**

Run:

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./internal/observability ./internal/graph -v
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/observability/tracing.go internal/observability/tracing_test.go internal/graph/nodes.go internal/graph/rag_graph_test.go
git commit -m "feat: add trace observability bridge"
```

## Final Verification

Run:

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./...
```

Expected: PASS.

Also run:

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./tests/contract -v
```

Expected: PASS.

## Suggested Execution Order

1. Task 1: Persist failed graph traces.
2. Task 2: Make trace writes idempotent.
3. Task 3: Add stable span ordering and timing.
4. Task 4: Add trace read indexes.
5. Task 5: Add trace list repository API.
6. Task 6: Add slow node statistics.
7. Task 7: Structure trace warnings.
8. Task 8: Add HTTP trace read API.
9. Task 9: Enrich SSE trace summary.
10. Task 10: Extend `oragctl trace`.
11. Task 11: Add memory trace store.
12. Task 12: Add OpenTelemetry bridge points.
