# Official Text RAG Pack Release Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build, publish, and verify an immutable full `text-rag` Pack from the user-authorized CRUD-RAG source without committing data or credentials.

**Architecture:** A standalone Go command writes reproducible release artifacts to a caller-selected directory. It locks an upstream source commit, derives deterministic Quick and Benchmark objects, validates them with existing tutorial Manifest rules, then optionally uploads them through the TOS S3-compatible SDK and verifies anonymous HTTPS reads. The embedded catalog moves only after a published release has passed verification.

**Tech Stack:** Go 1.26, existing `internal/tutorial` Manifest parser, Volcengine TOS Go SDK, HTTPS.

## Global Constraints

- Never store access keys, object paths containing credentials, source data, or release output in Git.
- Publish only under a new semantic version; reject existing remote objects and never delete public release data.
- Build and verify use deterministic sorted objects and SHA-256 checksums.
- Anonymous HTTPS validation must succeed before catalog publication.

---

### Task 1: Release artifact builder and local verifier

**Files:**
- Create: `internal/packrelease/builder.go`, `internal/packrelease/builder_test.go`
- Create: `cmd/orag-pack-release/main.go`
- Modify: `.gitignore`

- [x] Write tests using a temporary source tree to prove generated manifests, archive inventory and `SHA256SUMS`; reject an existing immutable output.
- [x] Implement deterministic `Build` with a clean Git source lock, Quick/Benchmark runtime objects, full source archive, provenance and checksum inventory.
- [x] Implement `orag-pack-release` build flags for source, version and output.
- [x] Run `go test ./internal/packrelease ./cmd/orag-pack-release`.

### Task 2: TOS publisher and anonymous verifier

**Files:**
- Create: `internal/packrelease/tos.go`, `internal/packrelease/tos_test.go`
- Modify: `go.mod`, `go.sum`, `cmd/orag-pack-release/main.go`

- [x] Add a TOS client whose credentials come only from environment variables at process start.
- [x] Preflight every immutable versioned key, upload public cacheable objects and refuse pre-existing targets.
- [x] Implement unauthenticated download and SHA-256 verification for every listed release artifact.
- [x] Test public verification against `httptest.Server`; run `go test ./internal/packrelease`.

### Task 3: Catalog, release guide, and CI-safe command surface

**Files:**
- Modify: `internal/tutorial/catalog.json`, `internal/tutorial/catalog_test.go`, `internal/http/router_test.go`
- Create: `docs/tutorials/official-text-pack-release.md`, `docs-site/tutorials/official-text-pack-release.html`
- Modify: `README.md`, `ROADMAP.md`, `ROADMAP_EN.md`, `docs/tutorials/clone-and-pack-install.md`, `docs-site/index.html`, `Makefile`

- [x] Update `text-rag` to immutable `1.1.0` Quick/Benchmark paths under `https://lensrhyme.tos-cn-hongkong.volces.com/tutorial-packs`; preserve 1.0.0 for the existing Replay.
- [x] Add guarded Make targets for build, verify and publish.
- [x] Document source commit locking, full archive, immutable objects and anonymous verification.
- [x] Run catalog/API tests and build the hosted documentation.

### Task 4: Authorized release and deployment validation

**Files:** no repository file changes expected after Task 3.

- [x] Fetch the authorized CRUD-RAG source at its locked commit into a temporary directory and build `text-rag/1.1.0`.
- [x] Publish after local verification and run anonymous HTTPS SHA-256 validation against the TOS public endpoint.
- [x] Update the embedded catalog and validate the catalog/API test surface. A provider-backed live Clone remains an operations follow-up because it needs a running ORAG deployment with its private stores.
- [x] Build hosted docs, deploy the standalone static site to `http://101.47.11.44:5411/`, and verify the release guide at `/tutorials/text-rag-pack-release.html`.

### Plan self-review

- All design requirements map to Tasks 1–4: deterministic build (Task 1), TOS and anonymous verification (Task 2), catalog/documentation (Task 3), and real external release/deployment (Task 4).
- Credentials, source data, output directories and overwrite behavior have explicit constraints.
- The command names and public-prefix contract are consistent across all tasks.
