# Compatibility and Capability Maturity

ORAG uses Semantic Versioning for published distributions and a separate maturity label for each public capability. A distribution version and a capability maturity answer different questions: `v0.1.0-beta.1` describes the release as a whole, while a feature within that release can still be experimental.

## Maturity Levels

### `experimental`

The capability is available for exploration and evaluation. Its API, configuration, storage representation, generated artifacts, or behavior may change without a migration path. Production use requires independent validation and an explicit fallback.

### `beta`

The capability is supported for evaluation and production pilots. Known limitations are documented, regressions are treated as release issues, and maintainers provide best-effort migration guidance. Feedback may still require breaking changes before stability.

### `stable`

The capability is covered by the stable compatibility policy. Breaking changes require deprecation notice, a documented migration path, and an appropriate major version transition.

ORAG will not mark any capability `stable` before `v1.0.0`.

## Sources of Truth

- The capability manifest owns maturity for agent-facing capabilities.
- `api/openapi.yaml` exposes `x-orag-maturity` for HTTP operations.
- Generated MCP and Skill artifacts carry the manifest maturity.
- README tables summarize these sources and are checked for drift.

Availability and maturity are separate. For example, `status: planned` means an operation is not yet registered at runtime, while `maturity: experimental` describes the compatibility expectation for its published design and generated artifacts.

## Pre-1.0 Compatibility

Before `v1.0.0`:

- experimental behavior may change in any release;
- beta behavior should include migration guidance when a practical path exists;
- security fixes may require immediate behavior changes;
- stored-data migrations are forward-only unless release notes explicitly provide rollback steps;
- public Go SDK changes are recorded in the changelog and checked with an external consumer module.

The root-module public Go SDK is `beta` beginning with `v0.1.0-beta.1` and is most recently published as `v0.1.0-beta.3`. Its documented core workflow receives best-effort migration guidance, while capabilities explicitly listed as SDK limitations retain their HTTP/control-plane maturity.

## Release Compatibility Audit

Every release tag compares its public OpenAPI and root Go SDK with the
preceding published tag before artifacts are produced. The structural audit
rejects removal of a published HTTP path, operation, response status, component
schema/property, exported SDK symbol, `Client` method, or exported SDK struct
field. Additive changes pass.

Run the same check locally against a fetched release tag:

```bash
COMPATIBILITY_BASE=v0.1.0-beta.3 make compatibility-audit
```

The first release has no predecessor and records a bootstrap result. A Beta
breaking change must be listed exactly in `compatibility-exceptions.json`; each
entry requires the stable finding ID, the target release, and a migration
reference. Wildcards and undocumented exceptions are rejected. This audit is
structural only: behavioral, performance, provider, and stored-data migration
compatibility still require their own release evidence.

Deprecations identify the replacement, first deprecated version, and earliest removal version. Removal must not occur in the same prerelease that first announces the deprecation unless required for security.

## Reporting Compatibility Problems

Open a Bug Issue with the previous and current version, affected capability, reproduction, expected behavior, and migration impact. Use the private process in `SECURITY.md` if the compatibility failure exposes credentials, tenant data, or an authorization boundary.
