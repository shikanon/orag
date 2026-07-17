# Release Compatibility Audit Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Gate release tags on an auditable structural comparison of the public OpenAPI and Go SDK against the preceding tag.

**Architecture:** An internal compatibility package loads OpenAPI documents with kin-openapi and exported Go declarations with go/parser. `oragctl` resolves the baseline from git, prints stable findings, and applies a narrow JSON exception file. Release CI invokes it before artifact publication.

**Tech Stack:** Go stdlib parser/AST, kin-openapi, `git show`, GitHub Actions, JSON, Go tests.

## Global Constraints

- Audit only public HTTP/OpenAPI and root `github.com/shikanon/orag` SDK surfaces.
- Additions pass; published removals fail unless a precise migration-backed exception exists.
- Exceptions cannot use wildcards and must contain a nonempty migration reference.
- Release-only enforcement must not make ordinary experimental PR work depend on a prior tag.

---

### Task 1: Build structural comparison primitives

**Files:** `internal/compatibility/audit.go`, `internal/compatibility/audit_test.go`

- [x] Model stable findings and explicit exception records.
- [x] Compare OpenAPI paths/methods, statuses and public component schema fields.
- [x] Compare root-package exported functions/types, Client methods and exported struct fields with `go/parser`.
- [x] Prove removal failure, additive success and exact exception behavior.

### Task 2: Expose a release-safe CLI

**Files:** `cmd/oragctl/main.go`, `cmd/oragctl/compatibility.go`, `cmd/oragctl/main_test.go`, `compatibility-exceptions.json`

- [x] Add `oragctl compatibility-audit --base <tag> [--allow-file]`.
- [x] Read baseline OpenAPI and root Go files using `git show`, compare against the checkout and return stable text/exit status.
- [x] Add a bootstrap-safe exception file containing no wildcard entries.
- [x] Test argument validation, exception validation and a real published-baseline pass.

### Task 3: Enforce and document the release policy

**Files:** `.github/workflows/release.yml`, `Makefile`, `docs/compatibility.md`, `docs/operations/README.md`, `tests/contract/release_compatibility_audit_test.go`, `ROADMAP.md`, `ROADMAP_EN.md`

- [x] Add a developer command requiring `COMPATIBILITY_BASE`.
- [x] Discover and audit the preceding tag before release verification.
- [x] Document exceptions, bootstrap behavior and the structural boundary.
- [x] Add drift coverage and run focused/full gates before PR, merge and documentation deploy.
