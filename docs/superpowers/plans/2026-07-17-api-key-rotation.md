# API Key Rotation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Atomically replace and revoke a tenant API key while returning a one-time replacement secret and exposing safe rotation lineage.

**Architecture:** The auth service owns replacement generation and typed errors. Memory and PostgreSQL repositories expose one `RotateAPIKey` operation; PostgreSQL uses a row lock and transaction. HTTP, OpenAPI, Console and docs expose the endpoint without exposing secrets after the response.

**Tech Stack:** Go, PostgreSQL/pgx, Hertz, OpenAPI, React/TanStack Query, Vitest.

## Global Constraints

- Only existing `api_key.manage` tenant authorization may invoke rotation.
- The replacement preserves tenant, project, role, name and future expiry.
- Source revocation and replacement insert are atomic.
- The replacement secret is returned once and never listed or logged.
- Rotation is immediate; no grace period is implied.

---

### Task 1: Add failing domain rotation tests

**Files:** `internal/auth/api_key.go`, `internal/auth/api_key_test.go`, `internal/auth/api_key_memory.go`

- [x] Add tests for scope preservation, old-secret rejection, new-secret authentication, cross-tenant and revoked rejection.
- [x] Run the focused auth test before implementation; it fails because `Rotate` does not yet exist.
- [x] Add `APIKeyRotateInput`, `APIKeyService.Rotate`, lineage metadata and memory atomic rotation.
- [x] Re-run the focused auth tests; expected PASS.

### Task 2: Add PostgreSQL atomic lineage

**Files:** `migrations/000039_api_key_rotation.sql`, `internal/storage/postgres/api_key.go`, `internal/storage/postgres/api_key_test.go`

- [x] Add self-FK `rotated_from_key_id`, partial unique source index and reversible Down migration.
- [x] Implement a pgx transaction locking an active, unexpired source, inserting replacement and revoking source.
- [x] Add migration-backed integration coverage.
- [x] Run focused storage/auth checks and the complete integration suite; expected PASS.

### Task 3: Expose endpoint and Console action

**Files:** `internal/http/api_keys.go`, `internal/http/router.go`, `internal/http/router_test.go`, `api/openapi.yaml`, `console/src/api/client.ts`, `console/src/api/schema.d.ts`, `console/src/features/api-keys/api-key-list.tsx`, `console/src/features/api-keys/api-key-list.test.tsx`, `console/src/test/handlers.ts`

- [x] Add HTTP tests for `201`, source `401`, tenant `404` and secret-free list output.
- [x] Add `POST /v1/api-keys/{api_key_id}/rotate`, response contract and regenerate Console schema.
- [x] Add confirmation and one-time-secret Console dialog with immediate-cutover copy.
- [x] Run `make openapi-validate`, `npm --prefix console run typecheck`, and `npm --prefix console test -- --run api-key-list`; expected PASS.

### Task 4: Document and verify release

**Files:** `docs/api/auth-and-errors.md`, `docs/sdk/README.md`, `docs/operations/README.md`, `ROADMAP.md`, `ROADMAP_EN.md`, `tests/contract/api_key_rotation_test.go`

- [x] Document one-time storage, immediate source revocation and separate server-secret rotation boundary.
- [x] Add a contract test for route, Console action, no-grace boundary and lineage field.
- [ ] Run focused checks, `git diff --check`, `make agent-gate`, then commit, merge, sync main, deploy docs and verify HTTPS.
