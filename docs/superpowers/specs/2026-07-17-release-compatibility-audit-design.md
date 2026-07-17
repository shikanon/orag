# Release Compatibility Audit Design

**Roadmap:** Phase 5 — SDK/API compatibility audits and predictable releases

## Goal

Make the existing Beta compatibility policy executable at release time by
comparing the release candidate's public HTTP and root Go SDK contracts with
the previous published tag.

## Scope

`oragctl compatibility-audit --base <tag>` will load the base OpenAPI document
through `git show` and compare it to `api/openapi.yaml`. It rejects removal of
an existing operation, response status, required request/response schema field
or public schema. It also parses root-package Go source from the tag and the
working tree, rejecting removed exported package symbols, `Client` methods and
exported struct fields.

The audit accepts `--allow-file <path>` only for explicit, reviewed Beta
exceptions. Each exact finding has a migration explanation and first/target
release. An allowlist does not make an undocumented or broad wildcard change
pass. The command emits stable, sorted findings and exits nonzero when any
unallowed breaking change remains.

## Release Wiring

The tag workflow discovers the immediately previous SemVer tag reachable from
the release commit. If no previous tag exists it records a bootstrap audit; if
one exists it runs the audit before artifacts are published. Normal main/PR
CI continues to use fast local contract tests and does not compare tags.

## Boundaries

The audit is structural, not behavioral: it cannot prove semantics,
performance or database migration compatibility. It intentionally protects
already published paths and exported names while still allowing additive Beta
work. Experimental contracts remain subject to the documented policy, but an
exception must be explicit rather than silent.

## Verification

- Unit tests create temporary baseline/current OpenAPI and Go source fixtures
  to prove removals fail, additions pass and precise allowlist entries work.
- CLI tests prove tag source resolution and stable output.
- A contract test pins the release workflow and compatibility documentation to
  the audit command.
