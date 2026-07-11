# ORAG Console RAG Orchestration Design

## 1. Summary

ORAG Console is a standalone web administration project for visually configuring, evaluating, and releasing ORAG query pipelines. The first release deliberately covers only three product areas:

1. RAG Studio for constrained visual DAG authoring and API debugging.
2. Evaluation Center for datasets, evaluation runs, comparisons, and hard quality gates.
3. Release Center for development, staging, and production promotion plus one-click rollback.

The console must support multiple projects per tenant. A project is the primary isolation boundary for pipelines, evaluation assets, environment bindings, releases, and API credentials.

This design does not add ingestion management, general knowledge-base administration, optimization management, trace search, or system monitoring pages. Those capabilities can be added later without changing the first-release information architecture.

## 2. Goals and Non-Goals

### Goals

- Provide a complete, independently runnable `console/` frontend project rather than a single-page prototype.
- Let both RAG engineers and application developers create query pipelines by dragging, connecting, and configuring built-in nodes.
- Keep common parameters visible and place advanced parameters in a progressively disclosed node inspector.
- Prevent arbitrary graphs through typed ports, graph invariants, and a backend-owned node registry.
- Turn API debugging requests into reusable evaluation cases.
- Require successful evaluation gates before creating or promoting an immutable pipeline version.
- Support development to staging to production promotion and atomic rollback to a previously validated version.
- Preserve existing CLI, MCP, and unscoped API behavior while adding a project-scoped control plane.

### Non-Goals

- User-authored code nodes, plugins, or arbitrary executables.
- Visual ingestion-pipeline authoring.
- Manual overrides for failed evaluation gates.
- Copying a mutable pipeline independently into every environment.
- Exposing model credentials or provider secrets to the browser.
- Replacing the existing query, dataset, evaluation, optimization, or trace implementation.

## 3. Users and Interaction Model

The console serves two primary audiences:

- RAG engineers need full node parameters, single-run traces, evaluation comparisons, and version diffs.
- Application developers need safe defaults, API debugging, code samples, and predictable release behavior.

The UI uses progressive disclosure. The default inspector shows only the parameters required to understand and operate a node. Advanced settings remain available in a collapsed section, with descriptions, defaults, valid ranges, and environment-binding indicators supplied by the backend node schema.

The main editing experience is canvas-first. Evaluation and release remain first-class navigation areas and contextual drawers, but they do not turn pipeline editing into a multi-step wizard.

## 4. Product Model

The control-plane hierarchy is:

```text
Tenant
└── Project
    ├── PipelineDraft
    ├── PipelineVersion
    ├── EvaluationPolicy
    ├── EvaluationRun
    ├── Environment
    ├── Release
    └── APICredential
```

### Project

A `Project` is the first-class isolation and navigation boundary. It owns display metadata and references project-scoped pipeline, evaluation, environment, release, and credential records. Every control-plane read and mutation checks tenant ownership and project membership on the server.

A project may bind one or more existing ORAG knowledge bases. Pipeline definitions reference logical binding names such as `primary_kb`; concrete knowledge-base IDs are resolved from the selected environment.

### PipelineDefinition

`PipelineDefinition` contains only portable graph behavior:

- node IDs and registered node types;
- node configuration without secrets or concrete environment resources;
- typed edges and branch conditions;
- graph metadata and schema version.

The definition is never executed directly by the browser. The server validates it against the current node registry and compiles it to an Eino graph.

### PipelineDraft

A draft is mutable and uses optimistic concurrency through a revision or ETag. Saving a stale revision returns a conflict with enough information to compare or reload. Debug execution freezes the submitted revision so later edits cannot change the active run.

### PipelineVersion

A version is immutable and records:

- the complete pipeline definition;
- the node-registry schema versions used to validate it;
- the evaluation policy version;
- successful evaluation evidence;
- a content hash and creation audit fields.

Only a fully successful evaluation can create a version.

### EvaluationPolicy

An evaluation policy references one or more project-scoped datasets and defines required metrics, thresholds, test coverage, and target-environment rules. All configured gates are hard gates. There is no force-promote operation.

### Environment

The first release provides three fixed environment kinds: `development`, `staging`, and `production`. An environment stores server-side resource bindings and an `active_version_id`. Secret material is referenced, not returned to the frontend.

### Release

A release is an append-only audit record for promotion or rollback. It captures the source and target versions, source and target environments, evidence, actor, timestamps, status, and failure details.

## 5. Frontend Information Architecture

The frontend lives in `console/` and uses project-scoped routes:

```text
/projects
├── /new
└── /:projectId
    ├── /overview
    ├── /studio/:pipelineId
    ├── /evaluations
    ├── /releases
    ├── /api-debugger
    └── /settings
```

The application shell contains:

- a tenant-aware project switcher;
- primary navigation for Project Overview, RAG Studio, Evaluation Center, and Release Center;
- developer navigation for API Debugger, credentials, and project settings;
- a global environment indicator;
- backend health and API compatibility state.

### Project Overview

Project Overview summarizes the active environment versions, recent draft activity, evaluation status, release history, and API integration entrypoints. It is a navigation and status page, not a general analytics dashboard.

### RAG Studio

RAG Studio uses a three-pane canvas layout:

- Left: searchable built-in node palette grouped by control, retrieval, context, and generation.
- Center: visual typed DAG with selection, pan, zoom, keyboard navigation, undo, redo, copy, paste, and version diff overlays.
- Right: schema-driven node inspector with common and advanced parameters, validation messages, binding information, and node documentation.

The bottom dock shows the latest debug run, retrieved chunks, trace latency, draft evaluation status, and the action required to create a candidate version.

The initial built-in node catalog maps to existing ORAG query behavior:

- query input;
- conditional query route;
- semantic cache lookup and write;
- query rewrite;
- multi-query and HyDE expansion;
- hybrid retrieval;
- graph retrieval expansion;
- rerank;
- context pack;
- prompt assembly and prefix cache;
- answer generation.

The catalog describes capability, not a promise that every current node can be freely reordered. Each node definition declares allowed predecessors, allowed successors, input and output port schemas, required cardinality, configuration schema, defaults, and whether it may appear more than once.

### API Debugger

The API Debugger is available as both a project route and a split panel inside RAG Studio. It provides:

- environment and pipeline-version selection;
- HTTP method and endpoint display;
- JSON request editing and validation;
- headers and safe project variables;
- request execution and cancellation;
- response body, citations, per-node inputs and outputs, warnings, logs, latency, and trace link;
- generated cURL, Go, and TypeScript examples;
- save-as-evaluation-case behavior.

Development may execute a specific frozen draft revision. Staging and production accept only the environment's immutable active version. Provider credentials and resolved resource bindings remain server-side.

### Evaluation Center

Evaluation Center includes:

- project-scoped dataset selection and case management;
- draft or version selection;
- evaluation-policy editing;
- asynchronous run progress;
- run summary and per-case results;
- baseline-versus-candidate metric comparison;
- trace and citation evidence drill-down;
- explicit hard-gate results;
- candidate-version creation after every gate passes.

### Release Center

Release Center displays development, staging, and production as an ordered lifecycle. Each environment card shows the active version, content hash, evaluation evidence, activation time, and available action.

Promotion is only possible from development to staging or staging to production. Rollback selects a version previously active and successfully validated in the target environment.

## 6. Frontend Architecture

The console is a React and TypeScript single-page application built with Vite. The architecture separates server state, local editing state, generated contracts, and reusable UI primitives.

Recommended foundation:

- TanStack Router for typed project-scoped routes and route-level lazy loading.
- TanStack Query for server state, caching, request cancellation, and mutation invalidation.
- React Flow for the visual node editor and accessible graph interactions.
- Zustand for ephemeral editor state and the undo/redo command history.
- Zod and React Hook Form for schema-derived node configuration forms.
- Monaco Editor for JSON request and response editing in the API Debugger.
- An OpenAPI-generated TypeScript client based on `api/openapi.yaml`.
- Vitest, Testing Library, Mock Service Worker, and Playwright for verification.

Suggested module boundaries:

```text
console/src/
├── app/                 # bootstrap, providers, route tree, error boundaries
├── api/                 # generated client, auth, errors, SSE helpers
├── components/          # reusable design-system components
├── features/
│   ├── projects/
│   ├── rag-studio/
│   ├── api-debugger/
│   ├── evaluations/
│   └── releases/
├── graph/               # node rendering, ports, commands, validation adapters
├── schemas/             # frontend refinements around API schemas
├── styles/              # tokens and global styles
└── test/                # fixtures, MSW handlers, render helpers
```

TanStack Query owns remote entities. The RAG Studio store owns only the current normalized graph, selection, viewport, transient validation, and command history. Saving converts the normalized editor state into a `PipelineDefinition`; reloading replaces local state from the authoritative server revision.

## 7. Backend Control-Plane Design

The existing API surface remains available. New project-scoped control-plane endpoints are added rather than changing current CLI and MCP contracts in place.

### Projects

```text
GET    /v1/projects
POST   /v1/projects
GET    /v1/projects/{project_id}
PATCH  /v1/projects/{project_id}
```

### Node Registry and Pipelines

```text
GET    /v1/pipeline-node-definitions
GET    /v1/pipeline-node-definitions/{type}
GET    /v1/projects/{project_id}/pipelines
POST   /v1/projects/{project_id}/pipelines
GET    /v1/pipelines/{pipeline_id}
PUT    /v1/pipelines/{pipeline_id}/draft
POST   /v1/pipelines/{pipeline_id}:validate
POST   /v1/pipelines/{pipeline_id}:run-debug
POST   /v1/pipelines/{pipeline_id}:create-version
```

The node registry returns stable node type IDs, display metadata, typed port definitions, JSON configuration schemas, defaults, graph constraints, and schema versions. The server repeats all validation during save, debug, version creation, and promotion.

### Evaluation

```text
GET    /v1/projects/{project_id}/evaluation-policies
POST   /v1/projects/{project_id}/evaluation-policies
POST   /v1/projects/{project_id}/evaluations
GET    /v1/evaluations/{run_id}
GET    /v1/evaluations/{run_id}/events
POST   /v1/debug-runs/{run_id}:save-case
```

The implementation should reuse the existing dataset and evaluation services. Project scoping is enforced by ownership references and service-layer authorization, not by trusting request IDs.

### Release

```text
GET    /v1/projects/{project_id}/environments
GET    /v1/projects/{project_id}/releases
POST   /v1/releases/{release_id}:promote
POST   /v1/environments/{environment_id}:rollback
```

### Debug Query

```text
POST   /v1/projects/{project_id}/query:debug
```

The request selects a development draft revision or an immutable version. The response extends the normal query response with ordered node events and safe diagnostic data. Sensitive configuration values, raw authorization headers, and secrets are never returned.

## 8. Graph Validation and Compilation

Validation occurs in layers:

1. JSON/schema validation checks node and edge payloads.
2. Registry validation checks node types and node-schema versions.
3. Port validation checks edge type compatibility and cardinality.
4. Structural validation checks exactly one query entry, at least one terminal answer, reachability, allowed cycles, and branch exhaustiveness.
5. Semantic validation checks required environment bindings and incompatible feature combinations.
6. Compilation builds an Eino graph from registered backend node factories.

Compilation never loads arbitrary frontend-supplied code or executable names. A node type can only resolve to a server-registered implementation.

## 9. Evaluation and Release State Machine

The lifecycle is:

```text
PipelineDraft
    -> frozen evaluation snapshot
    -> EvaluationRun
    -> all hard gates pass
    -> immutable PipelineVersion
    -> staging promotion
    -> staging evaluation gates pass
    -> production promotion
```

There is no manual gate override.

An evaluation run records the exact draft revision or version, pipeline content hash, policy version, dataset snapshot, environment bindings without secret values, metrics, per-case outcomes, trace IDs, and completion status.

Promotion is a backend transaction that revalidates:

- the source environment and active source version;
- successful required evaluation evidence;
- the pipeline content hash;
- target-environment bindings;
- the expected current target version.

The request includes `expected_active_version_id` for optimistic concurrency. A conflict returns the current active version and requires the user to refresh. The environment pointer changes only after every validation succeeds.

## 10. Rollback

Rollback is an atomic environment-pointer change to a version that was previously active and validated in that environment. It creates a new append-only release audit record containing the actor, reason, previous version, target version, evidence, and timestamps.

Rollback does not modify the selected version and does not copy definitions. Environment resource bindings remain unchanged. A failed rollback transaction leaves the current active version untouched.

## 11. Error Handling and Recovery

- Draft conflicts return a typed conflict response with current revision metadata.
- Invalid graphs return node- and edge-addressable validation errors for canvas highlighting.
- Evaluation failures retain the frozen input and completed case results; retry creates a new run linked to the failed run.
- SSE disconnection does not resubmit work. The frontend reconnects or polls using the existing run ID.
- Debug request cancellation is best-effort and returns the final server state if execution already completed.
- Release failure leaves the target environment on its previous version.
- API/schema incompatibility blocks editing and displays the expected and received contract versions.
- Route error boundaries isolate project navigation, editor failures, and API Debugger failures.
- All empty, loading, permission-denied, partial, stale, and unavailable states have explicit UI treatments.

## 12. Security and Audit Requirements

- Every control-plane operation is tenant- and project-authorized on the server.
- Project credentials and provider secrets are write-only references and never appear in frontend payloads.
- Debug output uses the current trace privacy and redaction rules.
- Pipeline definitions cannot reference executables, shell templates, remote URLs, or unregistered node implementations.
- Version creation, promotion, rollback, credential changes, and environment-binding changes are audited.
- Production rejects draft IDs even if a client constructs the request manually.
- Hard gates are enforced in release service transactions, not only in the UI.

## 13. Testing Strategy

### Frontend Unit Tests

- normalized graph transformations;
- undo and redo commands;
- port compatibility adapters;
- schema-to-form mapping;
- route and environment guards;
- release-state presentation.

### Component and Integration Tests

- node drag, connect, reconnect, delete, and keyboard flows;
- invalid-edge and unreachable-node presentation;
- node inspector common and advanced fields;
- API Debugger request validation, cancellation, response tabs, and save-as-case;
- evaluation progress, hard-gate failure, and candidate-version creation;
- promotion conflict, disabled promotion, successful promotion, and rollback dialogs.

### Backend Contract Tests

- OpenAPI contains every console endpoint and schema;
- generated frontend fixtures conform to the Go handlers;
- tenant and project isolation cannot be bypassed;
- node schema and compiler validation agree;
- hard gates cannot be bypassed with direct API requests;
- production cannot execute drafts;
- promotion and rollback remain atomic under conflicts and injected failures.

### End-to-End Tests

The release-blocking browser path is:

1. create and select a project;
2. create a pipeline from a template;
3. edit and validate the graph;
4. run a draft through API Debugger;
5. save the debug request as an evaluation case;
6. run the evaluation and observe a blocked gate;
7. edit the draft and pass every gate;
8. create an immutable version;
9. promote to staging and then production;
10. verify the production debug endpoint uses the active version;
11. roll back and verify the previous version becomes active.

## 14. Accessibility and Performance

- Every canvas action has a keyboard-accessible alternative through node search, selection, and command menus.
- Ports, edges, statuses, and validation do not rely on color alone.
- Focus is restored predictably after dialogs, drawers, and node deletion.
- Large editors virtualize or simplify off-screen node detail while preserving layout.
- Route modules and Monaco are lazy loaded.
- Query caches are project-keyed to prevent cross-project display leakage.
- Graph autosave is debounced, revision-aware, and cancellable.

## 15. Delivery Phases

### Phase 1: Foundation and Project Control Plane

- `console/` application shell, authentication integration, design tokens, routing, generated API client, project CRUD, project switcher, environment model, and error boundaries.
- Go project persistence, authorization, migrations, handlers, and OpenAPI schemas.

### Phase 2: RAG Studio and API Debugger

- node registry, pipeline drafts, visual editor, inspector, validation, debug compiler and runner, diagnostic events, code samples, and save-as-case.

### Phase 3: Evaluation and Hard Gates

- project-scoped evaluation policies, evaluation run UX, comparisons, evidence drill-down, hard-gate enforcement, and version creation.

### Phase 4: Release and Rollback

- immutable versions, environment bindings, promotion state machine, optimistic concurrency, release history, atomic rollback, and full audit events.

Each phase must add backend contract tests, frontend tests, OpenAPI regeneration checks, and an end-to-end slice before the next phase begins.

## 16. Acceptance Criteria

- A tenant can create and switch between multiple isolated projects.
- A project can create, save, reload, diff, and validate a constrained visual query DAG using only registered nodes.
- Invalid port types, unreachable nodes, invalid branches, and missing bindings cannot be versioned.
- The API Debugger can execute a frozen development draft and display response, citations, node output, logs, and trace linkage.
- A debug request can become an evaluation case.
- Evaluation freezes its inputs and exposes resumable progress.
- Any failed hard gate prevents version creation or promotion through both UI and direct API.
- A successful candidate becomes an immutable pipeline version with content and schema hashes.
- A version can move only from development to staging to production.
- Concurrent promotion detects stale target state instead of overwriting it.
- Production accepts only its active immutable version.
- Rollback atomically restores a previously validated version and writes an audit record.
- Existing ORAG query, CLI, MCP, dataset, evaluation, and trace behavior remains compatible.
