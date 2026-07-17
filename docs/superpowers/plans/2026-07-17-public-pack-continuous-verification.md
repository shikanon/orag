# Public Pack Continuous Verification Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Continuously verify the anonymous public `text-rag/1.1.0` Pack without credentials or mutable writes.

**Architecture:** Add strict HTTP metadata checks to the existing Go public verifier, wrap it in a temporary-directory shell entry point, and run that entry point in a separate scheduled/manual GitHub Actions workflow. PR CI remains independent from the external object store.

**Tech Stack:** Go 1.26, Bash, GitHub Actions, HTTPS, SHA-256.

## Global Constraints

- Verify only anonymous HTTPS reads from `https://lensrhyme.tos-cn-hongkong.volces.com/tutorial-packs`.
- Never log or require object-storage credentials.
- Do not overwrite or delete published objects.
- Do not frame artifact availability as a performance or production-quality result.

---

### Task 1: Harden the public verifier

**Files:**
- Modify: `internal/packrelease/publisher.go`
- Modify: `internal/packrelease/builder_test.go`

- [x] Add a response validator that rejects non-HTTPS final URLs, non-200 responses, mismatched MIME types, and declared-length/streamed-byte mismatches before accepting the existing SHA-256 match.
- [x] Extend the `httptest` contract tests with correct metadata and negative MIME, length, and HTTPS-to-HTTP redirect cases.
- [x] Run `GOTOOLCHAIN=go1.26.5 CGO_ENABLED=0 GOFLAGS='-tags=stdjson,gjson' go test ./internal/packrelease -count=1` and expect PASS.

### Task 2: Add a credential-free operational entry point

**Files:**
- Create: `scripts/verify-public-tutorial-pack.sh`
- Modify: `Makefile`
- Modify: `docs/tutorials/text-rag-pack-release.md`

- [x] Create a strict Bash wrapper that makes a temporary `text-rag/1.1.0` root, downloads only `SHA256SUMS`, invokes `orag-pack-release -verify-public`, and cleans up on exit.
- [x] Add `tutorial-pack-public-verify` with an overridable base URL and document the exact no-key command and scope.
- [x] Run the make target against the current public Pack and expect every declared object to verify.

### Task 3: Schedule independent public verification

**Files:**
- Create: `.github/workflows/public-pack-verification.yml`
- Modify: `ROADMAP.md`
- Modify: `ROADMAP_EN.md`

- [x] Create a read-only workflow triggered by `workflow_dispatch` and daily cron; set a bounded timeout, Go 1.26.5, and run the make target.
- [x] Document that this is ongoing public artifact health evidence, not a second public benchmark or a production-performance claim.
- [x] Run `git diff --check`, the focused Go test, the public make target, and the docs build; commit the completed implementation.
