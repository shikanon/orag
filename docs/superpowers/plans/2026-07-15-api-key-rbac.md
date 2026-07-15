# API Key and Project Authorization Implementation Plan

Design: [Machine API Keys, Minimal RBAC, and Project Authorization](../specs/2026-07-15-api-key-rbac-design.md)  
Tracking: [#226](https://github.com/shikanon/orag/issues/226)

Each slice is independently reviewable and keeps `main` deployable. A slice is complete only after local gates, protected GitHub checks, merge, main synchronization, and branch/worktree cleanup.

## Slice 1: principal and policy foundation

- Add typed principal, role, action, and deny-by-default policy types under `internal/auth`.
- Extend signed user claims with an explicit role while accepting older role-less tokens as `tenant_admin`.
- Replace implicit `tenant_default` request fallback with principal-derived tenant access.
- Add table-driven role/action/project/tenant tests and middleware compatibility tests.
- Document stable `401` versus `403` behavior.

## Slice 2: API key persistence and service

- Add migration `000017_api_key_rbac.sql` to evolve the existing table with project, prefix, role, creator, expiry, revocation, and usage metadata plus constraints and indexes.
- Implement memory and PostgreSQL repositories.
- Implement cryptographic generation, one-time secret result, constant-time verification, expiry/revocation checks, list, and idempotent revoke.
- Add unit, migration-contract, repository, and real PostgreSQL integration tests.

## Slice 3: authentication and lifecycle endpoints

- Resolve `orag_sk_` Bearer credentials through the API key service and user credentials through signed-token parsing.
- Add admin-only create/list/revoke handlers and explicit authorization middleware/helpers.
- Enforce project endpoint permissions and non-enumeration behavior.
- Add HTTP negative tests for malformed, expired, revoked, cross-tenant, and cross-project credentials.
- Update `api/openapi.yaml`, maturity metadata, examples, and generated/embedded contract checks.

## Slice 4: project-owned RAG roots

- Add nullable project ownership and indexes to knowledge bases and datasets.
- Create one deterministic legacy/default project per tenant and document backfill/rollback.
- Require project on new HTTP writes while retaining an explicit compatibility path for existing SDK callers during beta.
- Scope knowledge-base, ingestion, query, dataset, evaluation, trace, optimizer, and offline-knowledge access through the owning root.
- Add real PostgreSQL + Qdrant cross-project integration tests.

## Slice 5: public clients and operator experience

- Add public Go SDK API key lifecycle models and methods without exposing `internal/*`.
- Extend the independent consumer module and runnable examples.
- Add Console key list/create/revoke UI with one-time reveal, copy warning, and immediate state clearing.
- Add operator docs for creation, rotation, expiry, revocation, bootstrap admin retirement, and incident response.
- Update walkthrough and screenshots without embedding a real key.

## Slice 6: hardening and migration completion

- Add throttled best-effort last-used updates and non-secret authentication/authorization telemetry.
- Add credential redaction regression tests and threat-model documentation.
- Make project ownership non-null only after backfill and compatibility verification.
- Run full Go race, OpenAPI, SDK consumer, Console, PostgreSQL + Qdrant integration, container, and security gates.
- Update Roadmap progress and close #226 only when all acceptance criteria are verified.

