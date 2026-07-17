# Credential Rotation Drill Design

**Roadmap:** Phase 3 — secret injection, rotation, redaction and tenant boundaries

## Goal

Provide a repeatable, no-real-key credential rotation exercise with safe
evidence, and make the distinct server-secret cutover procedure explicit for
operators.

## Scope

The repository will add `make credential-rotation-drill`. It starts isolated
PostgreSQL and Qdrant dependencies, runs the API in explicit deterministic
mock mode, creates a tenant-admin API key, rotates it through the public API,
then proves that the source credential receives `401` while the replacement
receives `200`. The runner stores only metadata IDs, booleans, timestamps and
the build revision in `.tmp/credential-rotation-drill`; no full secret,
token, environment file or request body is copied into its evidence JSON.

This is intentionally an immediate-cutover exercise. It does not claim
zero-downtime service-secret rotation, provider-key validation, or a
production deployment drill.

## Server Secret Boundary

`JWT_SECRET` and `API_KEY_PEPPER` are not rotatable through the HTTP API.
Changing `JWT_SECRET` invalidates every active user token after restart.
Changing `API_KEY_PEPPER` invalidates every stored machine API key after
restart because stored HMACs were created with the old pepper. The operator
runbook therefore requires a maintenance window, a server-side secret update,
readiness validation, fresh login, recreation and distribution of machine
keys, and evidence that old credentials are rejected. It never asks operators
to record a secret value or hash in the repository.

## Threat Model

The public threat-model document records the protected assets, trust
boundaries, credible attack paths, preventive controls, detection signals and
operator recovery for: browser/user tokens, machine API keys, server signing
and pepper secrets, provider/object-store credentials, tenant scope and logs.
It labels residual risks honestly: a local drill cannot prove provider
revocation, host compromise resistance, external secret-manager policy, or a
production RTO.

## Verification

- A contract test confirms the Make target, runner, runbook, threat model and
  hosted security page remain linked and describe immediate invalidation.
- The runner exits only after old/new API-key authorization checks pass and
  writes a schema-versioned secret-free evidence file.
- `make credential-rotation-drill`, focused contract checks, `make agent-gate`
  and the hosted-doc build validate the change.
