# Credential Rotation Drill Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a safe, repeatable API-key rotation drill and documented server-secret cutover boundary.

**Architecture:** A Bash runner reuses the isolated PostgreSQL/Qdrant Compose topology and an ephemeral local API process. It calls the public API, validates authorization before and after rotation, and emits a non-secret JSON evidence record. Documentation distinguishes this product exercise from operator-owned JWT/pepper rotations.

**Tech Stack:** Bash, Docker Compose, curl, jq, Go API binary, Markdown, static HTML, Go contract tests.

## Global Constraints

- The drill runs only against temporary local dependencies and explicit deterministic mocks.
- Evidence must not contain a bearer token, API-key secret, environment values or request/response bodies.
- API-key rotation is immediate cutover; JWT/pepper rotation requires a restart and maintenance procedure.

---

### Task 1: Implement the isolated API-key exercise

**Files:** `scripts/credential-rotation-drill.sh`, `Makefile`

- [x] Write the runner with command prerequisites, a unique Compose project,
  cleanup trap and an ignored temporary evidence directory.
- [x] Start PostgreSQL/Qdrant, migrate an explicit mock API, login, create a
  tenant-admin machine key and rotate it through `/v1/api-keys/{id}/rotate`.
- [x] Assert the source receives `401`, the replacement receives `200`, and
  serialize only safe IDs/booleans/build revision in
  `orag.credential-rotation-drill.v1` evidence.
- [x] Add `make credential-rotation-drill` and run it successfully.

### Task 2: Publish threat model and operator boundary

**Files:** `docs/security/threat-model.md`, `docs/operations/credential-rotation.md`, `docs/operations/README.md`, `docs-site/credential-rotation.html`, `docs-site/index.html`, `ROADMAP.md`, `ROADMAP_EN.md`

- [x] Document assets, boundaries, mitigations, detection and residual risks.
- [x] Document the distinct immediate API-key operation and controlled JWT/API
  pepper service-secret restart procedure without including secret values.
- [x] Link the hosted security page and update Roadmaps to record the local
  rotation exercise while retaining independent deployment evidence as open.

### Task 3: Prevent documentation drift and ship

**Files:** `tests/contract/credential_rotation_drill_test.go`

- [x] Add contract coverage for target, script, no-secret evidence terms,
  runbook, threat model, hosted page and Roadmap language.
- [x] Run focused checks, the live drill, full agent gate and documentation
  build; commit, merge, synchronize `main`, deploy docs and verify HTTPS.
