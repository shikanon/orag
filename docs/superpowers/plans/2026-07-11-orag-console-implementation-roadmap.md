# ORAG Console Implementation Roadmap

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver the approved ORAG Console design as four independently testable increments.

**Architecture:** Build the project control plane before the visual editor, then reuse existing evaluation services behind project-scoped hard gates, and finish with immutable promotion and rollback. Each increment leaves the repository in a deployable state and owns its migrations, OpenAPI contract, backend tests, frontend tests, and browser acceptance path.

**Tech Stack:** Go, Hertz, Eino, PostgreSQL, OpenAPI 3, React, TypeScript, Vite, TanStack Router/Query, React Flow, Zustand, Zod, React Hook Form, Monaco, Vitest, Testing Library, MSW, Playwright.

## Global Constraints

- Project is the tenant-scoped isolation boundary for pipelines, datasets, environments, releases, and credentials.
- The browser never receives model credentials or provider secrets.
- Only server-registered built-in node types may be executed.
- Development may execute a frozen draft revision; staging and production execute immutable active versions only.
- Every evaluation gate is mandatory and cannot be overridden.
- Promotion order is development to staging to production.
- Rollback is an atomic pointer change to a version previously validated in the target environment.
- Existing `/v1/query`, CLI, MCP, dataset, evaluation, and trace behavior remains compatible.
- Use `GOTOOLCHAIN=go1.26.4` for direct Go commands outside Make targets.

---

## Plan Set

Execute these plans in order:

1. [`2026-07-11-orag-console-project-foundation.md`](./2026-07-11-orag-console-project-foundation.md) creates the Project control plane and runnable frontend shell.
2. [`2026-07-11-orag-console-rag-studio.md`](./2026-07-11-orag-console-rag-studio.md) adds the node registry, constrained DAG editor, compiler, and API Debugger.
3. [`2026-07-11-orag-console-evaluation-gates.md`](./2026-07-11-orag-console-evaluation-gates.md) adds project-scoped evaluation policies, frozen runs, hard gates, and version creation.
4. [`2026-07-11-orag-console-release-lifecycle.md`](./2026-07-11-orag-console-release-lifecycle.md) adds environment promotion, optimistic concurrency, immutable release history, and rollback.

## Cross-Plan Completion Gate

- [ ] Run `make openapi-validate`; expected: `PASS`.
- [ ] Run `make test`; expected: all Go packages pass.
- [ ] Run `make vet`; expected: exit 0.
- [ ] Run `npm --prefix console run typecheck`; expected: exit 0.
- [ ] Run `npm --prefix console test -- --run`; expected: all Vitest suites pass.
- [ ] Run `npm --prefix console run build`; expected: Vite production build exits 0.
- [ ] Run `npm --prefix console run test:e2e`; expected: create project through rollback scenario passes.
