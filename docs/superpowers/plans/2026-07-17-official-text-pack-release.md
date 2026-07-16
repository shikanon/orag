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

- [ ] Write tests using a temporary source tree to prove the builder produces byte-identical manifests and `SHA256SUMS`, rejects a non-commit source lock, and does not include files outside `data/`.
- [ ] Implement `Build(ctx, BuildOptions) (Release, error)` with `SourceCommit`, `SourceDir`, `OutputDir`, and `Version`; emit Quick and Benchmark release directories, source provenance, manifest and checksum inventory.
- [ ] Implement `orag-pack-release build --source-dir --source-commit --version --output-dir`; refuse output within the Git worktree unless it is ignored.
- [ ] Run `go test ./internal/packrelease ./cmd/orag-pack-release`; commit `feat: build deterministic tutorial pack releases`.

### Task 2: TOS publisher and anonymous verifier

**Files:**
- Create: `internal/packrelease/tos.go`, `internal/packrelease/tos_test.go`
- Modify: `go.mod`, `go.sum`, `cmd/orag-pack-release/main.go`

- [ ] Add a TOS client whose credentials come only from `OBJECT_STORAGE_ACCESS_KEY_ID` and `OBJECT_STORAGE_ACCESS_KEY_SECRET` at process start.
- [ ] Implement `Publish(ctx, PublisherOptions, Release) error`: preflight `HeadObject`, upload content types with immutable cache-control, and refuse any pre-existing target object.
- [ ] Implement `VerifyPublic(ctx, publicBaseURL, Release) error` with unauthenticated GETs of Manifest and every declared object, size/MIME/SHA checks, and no signed query parameters.
- [ ] Test existing-object rejection and public verification against `httptest.Server`; run `go test ./internal/packrelease`.
- [ ] Commit `feat: publish and verify tutorial packs on tos`.

### Task 3: Catalog, release guide, and CI-safe command surface

**Files:**
- Modify: `internal/tutorial/catalog.json`, `internal/tutorial/catalog_test.go`, `internal/http/router_test.go`
- Create: `docs/tutorials/official-text-pack-release.md`, `docs-site/tutorials/official-text-pack-release.html`
- Modify: `README.md`, `ROADMAP.md`, `ROADMAP_EN.md`, `docs/tutorials/clone-and-pack-install.md`, `docs-site/index.html`, `Makefile`

- [ ] Update `text-rag` to immutable `1.1.0` Quick/Benchmark paths under `https://lensrhyme.tos-cn-hongkong.volces.com/tutorial-packs` via documented `TUTORIAL_CATALOG_BASE_URL`; preserve the existing catalog JSON constraints.
- [ ] Add `make tutorial-pack-build`, `make tutorial-pack-verify`, and a guarded `make tutorial-pack-publish` that requires `ORAG_PACK_PUBLISH=1`.
- [ ] Document source commit locking, release output, environment-only credentials, immutable objects, anonymous verification, and catalog rollback.
- [ ] Run catalog/API tests, docs build, and Console contract generation; commit `docs: document official tutorial pack release`.

### Task 4: Authorized release and deployment validation

**Files:** no repository file changes expected after Task 3.

- [ ] Fetch the authorized CRUD-RAG source at its locked commit into a temporary directory and build `text-rag/1.1.0`.
- [ ] Publish only after local verify; then run unauthenticated HTTPS validation against the TOS public endpoint.
- [ ] Run a real clone against the public endpoint, publish/merge the catalog update, and validate the catalog endpoint.
- [ ] Build docs, deploy atomically to `101.47.11.44`, and verify the release guide over `https://www.tensorbytes.com/orag/`.

### Plan self-review

- All design requirements map to Tasks 1–4: deterministic build (Task 1), TOS and anonymous verification (Task 2), catalog/documentation (Task 3), and real external release/deployment (Task 4).
- Credentials, source data, output directories and overwrite behavior have explicit constraints.
- The command names and public-prefix contract are consistent across all tasks.
