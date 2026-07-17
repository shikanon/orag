# Public GHCR Release Verification Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Prevent a GitHub prerelease unless its two GHCR release tags are anonymously pullable, multi-architecture images matching the generated digests.

**Architecture:** A new read-only workflow job consumes the Buildx digest artifacts.  It obtains an anonymous GHCR bearer token, reads each tag's manifest index, and validates its HTTP result, digest, media type, and `amd64`/`arm64` platforms before the release job can run.

**Tech Stack:** GitHub Actions shell, curl, jq, existing Go contract tests.

## Global Constraints

- Never print registry credentials or an access token.
- Check the public tag rather than an authenticated Docker client cache.
- Preserve the existing image digest as the immutable release identity.

---

### Task 1: Pin the public image contract in tests

**Files:**
- Modify: `tests/contract/release_compatibility_audit_test.go`

- [ ] Add assertions that `public-images` needs `images`, requests an anonymous GHCR token, compares `Docker-Content-Digest`, verifies both architectures, and gates `github-release`.
- [ ] Run `go test ./tests/contract -run TestRelease -count=1` and confirm it fails before workflow wiring.

### Task 2: Implement the release gate

**Files:**
- Modify: `.github/workflows/release.yml`

- [ ] Add the `public-images` job after Buildx publishing. Download the API/Console digest artifacts and use anonymous token + manifest reads to validate the tag contract.
- [ ] Make `github-release` require both `images` and `public-images`.
- [ ] Run `go test ./tests/contract -run TestRelease -count=1` and `git diff --check`; both pass.

### Task 3: Document the public deployment invariant

**Files:**
- Modify: `docs/operations/server-deployment.md`
- Modify: `ROADMAP.md`
- Modify: `ROADMAP_EN.md`

- [ ] State that releases are blocked until anonymous GHCR multi-architecture verification succeeds; do not claim operator deployment or production evidence from this check.
- [ ] Run the targeted contract test and inspect the workflow YAML with `ruby -e 'require "yaml"; YAML.load_file(".github/workflows/release.yml")'`.
