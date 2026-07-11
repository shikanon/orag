# ORAG Console RAG Studio Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a backend-owned node registry, mutable pipeline drafts, constrained React Flow editing, server compilation, and project-scoped API debugging.

**Architecture:** Node schemas and graph invariants live in Go and are exposed through OpenAPI. The frontend normalizes the graph for editing but the backend repeats validation before save and execution. Debug runs freeze a draft revision and execute only registered node factories.

**Tech Stack:** Go, Eino, Hertz, PostgreSQL, OpenAPI, React Flow, Zustand, Zod, React Hook Form, Monaco, Vitest, Playwright.

## Global Constraints

- No arbitrary code nodes, executables, or remote implementation URLs.
- Development may execute a frozen draft; non-development environments reject drafts.
- Debug responses redact secrets and link to existing traces.
- Use `GOTOOLCHAIN=go1.26.4` for direct Go commands outside Make targets.

---

### Task 1: Node registry and graph validator

**Files:**
- Create: `internal/pipeline/types.go`, `internal/pipeline/registry.go`, `internal/pipeline/validate.go`
- Create: `internal/pipeline/registry_test.go`, `internal/pipeline/validate_test.go`

**Interfaces:**
- Produces: `pipeline.NodeDefinition`, `pipeline.Definition`, `pipeline.Registry`, and `pipeline.Validate(Definition, Registry) []ValidationError`.

- [ ] Write failing tests for unknown types, incompatible ports, duplicate singletons, unreachable nodes, missing answer terminals, and non-exhaustive branches.
- [ ] Run `GOTOOLCHAIN=go1.26.4 GOFLAGS='-tags=stdjson,gjson' go test ./internal/pipeline -v`; expected: FAIL because the package is missing.
- [ ] Implement typed `PortDefinition`, JSON configuration schema metadata, graph constraints, stable validation error codes, and built-in definitions matching the current `internal/graph` stages.
- [ ] Run the focused test again; expected: PASS.
- [ ] Commit with `git commit -m "feat: add pipeline node registry and validation"`.

### Task 2: Pipeline draft persistence and optimistic concurrency

**Files:**
- Create: `migrations/000017_pipeline_drafts.sql`
- Create: `internal/pipeline/service.go`, `internal/pipeline/service_test.go`
- Create: `internal/storage/postgres/pipeline.go`, `internal/storage/postgres/pipeline_test.go`

**Interfaces:**
- Consumes: `pipeline.Validate`.
- Produces: `Service.CreatePipeline`, `Service.GetDraft`, and `Service.SaveDraft(ctx, tenantID, projectID, pipelineID string, expectedRevision int64, definition Definition)`.

- [ ] Add a test where two saves use revision 3 and the second returns `pipeline.ErrRevisionConflict` carrying current revision 4.
- [ ] Run `go test ./internal/pipeline ./internal/storage/postgres -run 'Test.*Draft' -v`; expected: FAIL.
- [ ] Add `pipelines` and `pipeline_drafts` tables; persist JSONB definition plus revision and schema version; update with `WHERE revision = expected_revision`.
- [ ] Re-run the focused tests; expected: PASS.
- [ ] Commit with `git commit -m "feat: persist revisioned pipeline drafts"`.

### Task 3: Compiler, debug runner, and diagnostic events

**Files:**
- Create: `internal/pipeline/compiler.go`, `internal/pipeline/compiler_test.go`
- Create: `internal/pipeline/debug.go`, `internal/pipeline/debug_test.go`
- Modify: `internal/graph/nodes.go`, `internal/graph/rag_graph.go`

**Interfaces:**
- Produces: `Compiler.Compile(context.Context, Definition) (compose.Runnable[graph.State, graph.State], error)` and `DebugRunner.Run(context.Context, DebugRequest) (DebugResponse, error)`.

- [ ] Add tests proving unknown node types never invoke a factory, registered linear and routed graphs compile, a draft revision is frozen before execution, and diagnostics contain ordered safe node events.
- [ ] Run `go test ./internal/pipeline ./internal/graph -run 'Test(Compile|Debug)' -v`; expected: FAIL.
- [ ] Implement a factory map keyed by registered node type; reuse existing `NodeSet` methods through adapters; emit sequence, node ID, latency, safe input summary, safe output summary, and error.
- [ ] Re-run tests; expected: PASS and no secrets in golden diagnostic JSON.
- [ ] Commit with `git commit -m "feat: compile and debug registered pipelines"`.

### Task 4: Pipeline and debugger HTTP contracts

**Files:**
- Create: `internal/http/pipelines.go`, `internal/http/pipeline_debug.go`
- Modify: `internal/http/router.go`, `internal/http/router_test.go`, `internal/app/app.go`
- Modify: `api/openapi.yaml`, `tests/contract/openapi_test.go`

**Interfaces:**
- Produces: node-definition, pipeline CRUD, validation, and `POST /v1/projects/{project_id}/query:debug` endpoints.

- [ ] Add handler tests for tenant isolation, revision conflict, invalid graph errors, development draft debug, production draft rejection, and trace linkage.
- [ ] Run `make openapi-validate`; expected: FAIL because paths are missing.
- [ ] Add handlers and OpenAPI schemas with stable error codes and node-addressable validation details.
- [ ] Run `make openapi-validate && go test ./internal/http ./internal/app -run 'Test(Pipeline|ProjectDebug)' -v`; expected: PASS.
- [ ] Commit with `git commit -m "feat: expose pipeline and debug APIs"`.

### Task 5: RAG Studio editor and API Debugger UI

**Files:**
- Create: `console/src/features/rag-studio/{rag-studio-page,node-palette,pipeline-canvas,node-inspector,validation-overlay}.tsx`
- Create: `console/src/graph/{types,commands,store,ports,serialize}.ts`
- Create: `console/src/features/api-debugger/{api-debugger,request-editor,response-tabs,code-samples}.tsx`
- Create tests beside every component and `console/e2e/rag-studio.spec.ts`
- Modify: `console/src/app/router.tsx`

**Interfaces:**
- Consumes: generated node registry, draft, validation, and debug endpoints.
- Produces: `/projects/:projectId/studio/:pipelineId` and `/projects/:projectId/api-debugger`.

- [ ] Add failing tests for typed connections, undo/redo, common-versus-advanced fields, stale revision UI, request validation, response tabs, and save-as-case.
- [ ] Run `npm --prefix console test -- --run rag-studio`; expected: FAIL.
- [ ] Implement normalized nodes/edges, command-based mutation history, schema-driven inspector, lazy Monaco, request cancellation, and trace links.
- [ ] Run `npm --prefix console run typecheck && npm --prefix console test -- --run && npm --prefix console run test:e2e -- rag-studio.spec.ts`; expected: PASS.
- [ ] Commit with `git commit -m "feat: add RAG Studio and API debugger"`.
