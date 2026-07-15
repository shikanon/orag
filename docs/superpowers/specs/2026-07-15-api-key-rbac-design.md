# Machine API Keys, Minimal RBAC, and Project Authorization

Status: accepted for implementation  
Issue: [#226](https://github.com/shikanon/orag/issues/226)  
Roadmap: Stage 3, Security and tenant boundaries

## Context

ORAG currently treats every valid Bearer token as a tenant-wide administrator. The signed login token contains a tenant and user ID, while the unused initial `api_keys` table contains only a name and hash. Projects are tenant-owned, but knowledge bases, datasets, evaluations, traces, optimizer runs, and offline-knowledge records do not yet have a project boundary.

This design introduces machine credentials and explicit authorization without changing the meaning of existing beta user tokens in the first migration. It then moves resource roots behind project ownership in independently reversible slices.

## Goals

- Support long-lived machine authentication without storing recoverable secrets.
- Make every authenticated request carry a typed principal.
- Provide a small, auditable role model with deny-by-default policy evaluation.
- Prevent project-scoped credentials from enumerating or accessing other projects.
- Move RAG resource roots to project ownership without an unsafe flag-day migration.
- Keep HTTP, OpenAPI, Console, and public Go SDK contracts aligned.

## Non-goals

- User invitations, SSO, OAuth, custom roles, or per-field permissions.
- Treating API keys as human login sessions.
- Replacing tenant predicates with project predicates. Both remain mandatory.
- Exposing API key secrets after creation.

## Principal and roles

Authentication produces one request-scoped `Principal`:

```go
type Principal struct {
    Kind      PrincipalKind // user or api_key
    SubjectID string
    TenantID  string
    Role      Role
    ProjectID string // empty means tenant-wide
}
```

The initial roles are intentionally fixed:

| Role | Tenant scope | Project scope | Read project resources | Mutate project resources | Manage projects and keys |
| --- | --- | --- | --- | --- | --- |
| `tenant_admin` | yes | optional | yes | yes | yes |
| `project_editor` | no | required | yes | yes | no |
| `project_viewer` | no | required | yes | no | no |

Existing admin login tokens resolve to a `user` principal with `tenant_admin`. API keys may use any role, but editor/viewer keys require a project. A tenant-admin key may optionally be project constrained; the project constraint always narrows its effective access.

Policy evaluation accepts an action and optional resource project. It first validates tenant equality, then project scope, then the role/action matrix. Unknown actions and malformed principals are denied. Handlers must not infer authorization from role strings directly.

## API key format and storage

The secret format is:

```text
orag_sk_<public-id>_<base64url-secret>
```

- `public-id` is a non-secret lookup identifier included in list responses.
- The secret contains 32 bytes from `crypto/rand`.
- PostgreSQL stores `HMAC-SHA-256(API_KEY_PEPPER, full-key)` and never stores the full key. The separately managed server pepper adds protection if the database alone is disclosed.
- Verification parses the public ID, fetches one tenant-independent candidate by ID, hashes the presented key, and uses constant-time comparison.
- The full key is returned once by create and never returned by list/get.
- HTTP logs, errors, traces, metrics labels, Console analytics, and audit payloads must not include it.

The replacement `api_keys` shape contains: ID, tenant ID, project ID, name, prefix, key hash, role, creator principal, created time, expiry, revoked time, and last-used time. Key hash and public ID are unique. Check constraints enforce the role/project combinations.

Last-used writes are best effort and throttled to at most once per key per hour. Authentication remains available if that audit update fails.

## HTTP contract

Tenant administrators manage keys:

- `POST /v1/api-keys` creates a key and returns the secret once.
- `GET /v1/api-keys` lists metadata only.
- `DELETE /v1/api-keys/{api_key_id}` revokes a key idempotently.

All three use the existing Bearer scheme. Key creation rejects expiry in the past, unsupported roles, editor/viewer keys without a project, and projects outside the principal's tenant. Revocation of the current key is allowed and affects the next request.

Authentication errors remain `401`; authenticated but disallowed requests use stable `403 forbidden`. A foreign or inaccessible resource is returned as `404` where revealing its existence would cross a tenant/project boundary.

Project endpoint behavior:

- Tenant-wide administrators may create and list projects.
- Project-constrained principals cannot create or enumerate all projects.
- A constrained principal may get its own project.
- Editors and viewers may not update project metadata in the initial policy; project administration stays tenant-admin-only.

## Resource ownership migration

Project ownership rolls out in three steps:

1. Add nullable `project_id` to root tables and allow tenant administrators to use legacy tenant-owned rows.
2. Require project ID on new writes, backfill legacy rows into one documented default project per tenant, and propagate project predicates through repositories.
3. Make root project IDs non-null after compatibility telemetry and integration tests prove the backfill.

Root ownership is added to knowledge bases and datasets first. Evaluations inherit from datasets; ingestion, documents, chunks, traces, optimization, and offline-knowledge records inherit from their root. Queries verify that the knowledge base belongs to the principal's project before execution. Child tables do not accept caller-provided project IDs when the parent already determines ownership.

Every storage query retains `tenant_id` and adds the root/project predicate. Authorization in handlers is defense in depth, not a substitute for scoped repositories.

## Runtime wiring

The authentication middleware resolves a user token first only when its strict two-part format is present; `orag_sk_` tokens are resolved only by the API key service. A malformed value never falls back across credential types. Successful resolution stores a typed principal in the request context and no handler defaults to `tenant_default` when the principal is absent.

Memory and PostgreSQL backends implement the same API key repository interface. The public Go SDK models key metadata and the one-time create result without exposing internal packages. Console state holds a newly created secret only in the result dialog and clears it when the dialog closes.

## Threat model and controls

| Threat | Control |
| --- | --- |
| Database disclosure | Only high-entropy key hashes are stored |
| Key enumeration | Random public IDs, generic authentication failures, rate-limit-ready lookup path |
| Cross-tenant access | Tenant predicate remains on every repository operation |
| Cross-project confused deputy | Typed principal plus explicit resource-project policy check |
| Secret leakage | One-time response, response redaction tests, no secret fields in metadata models |
| Revoked/expired reuse | Checked before every request; fail closed |
| Authorization drift | Table-driven policy tests and HTTP negative contract tests |
| Unsafe backfill | Nullable phase, deterministic default project, reversible migration |

## Observability and audit

Authentication metrics use credential kind and outcome only. Logs may include API key ID, principal subject, tenant, role, project, route, and decision, but never the presented credential or hash. Create and revoke operations emit structured audit records once an append-only audit facility is introduced; until then, structured application logs carry the non-secret metadata.

## Compatibility and rollout

Existing login and user tokens remain tenant-admin compatible through the beta series. API key endpoints and role values are beta contracts. Project ownership changes include migration notes and a deprecation window for writes without project ID. Rollback disables API-key authentication first, then reverts nullable ownership migrations without discarding tenant-owned data.

## Verification

- Unit tests for parsing, randomness boundaries, hash verification, expiry, revocation, and policy matrix.
- HTTP tests for admin lifecycle, viewer/editor behavior, and cross-project/tenant denial.
- PostgreSQL migration and repository tests with real PostgreSQL integration coverage.
- Race tests for concurrent authentication/revocation and last-used throttling.
- OpenAPI validation, SDK external-consumer tests, Console tests, and secret scanning.
