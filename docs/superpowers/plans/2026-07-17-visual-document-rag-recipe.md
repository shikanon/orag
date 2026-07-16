# Visual-document RAG Recipe Pack Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `visual-document-rag/1.0.0` cloneable and runnable from a pinned, directly-downloaded ViDoSeek Recipe without publishing source data through ORAG.

**Architecture:** Add a strict Recipe Manifest and fixed-source downloader beside the existing public object Pack reader. Extend durable clone state with private source conversion stages, then add a visual-only runtime declaration and P0 execution path that cannot fall back to the text runtime. Public Replay remains an aggregate-only immutable record.

**Tech Stack:** Go, Hertz, PostgreSQL, Qdrant, archive/zip, SHA-256, existing tutorial Clone/Run services, React Console, Playwright.

## Global Constraints

- Public catalog artifacts may contain only Recipe JSON, provenance, and checksums; never source PDFs, annotations, derived pages, temporary URLs, or private coordinates.
- The accepted source is `Qiuchen-Wang/ViDoSeek@e91a92ba5f38690696c7e66be5c5474b54c6e791` using HTTPS, a fixed allowlist, size limits, and SHA-256.
- Visual runtime contracts are modality-specific; no visual request may execute through the text runtime or silently degrade to text-only retrieval.
- Clone retries reuse only verified private source and deterministic derived outputs; failures expose a stage and safe code only.
- Deterministic mock visual providers are permitted only in explicit test/demo configuration and never produce an official Replay.

---

## File structure

| File | Responsibility |
| --- | --- |
| `internal/tutorial/recipe.go` | Recipe types, strict JSON validation, fixed ViDoSeek allowlist and visual runtime declaration. |
| `internal/tutorial/recipe_fetch.go` | Bounded HTTPS fetch, checksum verification, safe ZIP extraction and deterministic page conversion. |
| `internal/tutorial/recipe_test.go` | Manifest, source drift, limit and archive-safety tests. |
| `internal/tutorial/clone.go` | Recipe clone stages and safe public status projection. |
| `internal/tutorial/recipe_clone.go` | Private-source installer and retry-safe clone orchestration. |
| `internal/tutorial/visual_runtime.go` | Visual project resources and P0/candidate validation. |
| `internal/tutorial/visual_runtime_test.go` | Visual lineage, isolation and mock-provider tests. |
| `internal/tutorial/catalog.json` | Recipe paths, truthful size estimates and visual replay availability. |
| `internal/http/tutorials.go` | Recipe-aware worker setup and redacted errors. |
| `console/src/features/tutorials/*` | Recipe download disclosure and visual run status. |
| `tests/fixtures/tutorial-recipes/*` | Fixed local source and Recipe fixtures for deterministic tests/E2E. |
| `docs/tutorials/visual-document-rag.md` | User workflow, licence, provenance, recovery and limitations. |

### Task 1: Add strict Recipe Manifest validation

**Files:**
- Create: `internal/tutorial/recipe.go`
- Create: `internal/tutorial/recipe_test.go`
- Test: `internal/tutorial/recipe_test.go`

**Interfaces:**
- Produces `ParseRecipe(raw []byte, template Template, pack PackRef) (RecipeManifest, error)`.
- Produces `RecipeManifest`, `RecipeSourceObject`, and `VisualRuntimeManifest`.

- [ ] **Step 1: Write failing tests for pinned source, unknown fields, drift and invalid modality.**

```go
func TestParseRecipeRejectsSourceDrift(t *testing.T) {
    _, err := ParseRecipe([]byte(`{"template_id":"visual-document-rag","version":"1.0.0","tier":"quick","source":{"dataset":"Qiuchen-Wang/ViDoSeek","revision":"main"}}`), visualTemplate(), visualQuickPack())
    if !errors.Is(err, ErrRecipeInvalid) { t.Fatalf("err=%v", err) }
}
```

- [ ] **Step 2: Run the focused test.**

Run: `go test ./internal/tutorial -run TestParseRecipeRejectsSourceDrift -count=1`

Expected: FAIL because `ParseRecipe` is undefined.

- [ ] **Step 3: Implement strict recipe types and parser.**

```go
type RecipeManifest struct { TemplateID, Version, Tier string; Source RecipeSource; Runtime VisualRuntimeManifest }
type RecipeSource struct { Dataset, Revision, License string; Objects []RecipeSourceObject }
type RecipeSourceObject struct { Path, SHA256 string; Bytes int64 }
func ParseRecipe(raw []byte, template Template, pack PackRef) (RecipeManifest, error) { /* decoder.DisallowUnknownFields; validate pinned source, HTTPS allowlist, SHA and limits */ }
```

- [ ] **Step 4: Run all Recipe tests.**

Run: `go test ./internal/tutorial -run 'TestParseRecipe' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit.**

```bash
git add internal/tutorial/recipe.go internal/tutorial/recipe_test.go
git commit -m "feat: validate visual tutorial recipes"
```

### Task 2: Implement bounded direct-source acquisition and safe conversion

**Files:**
- Create: `internal/tutorial/recipe_fetch.go`
- Modify: `internal/tutorial/recipe_test.go`
- Test: `internal/tutorial/recipe_test.go`

**Interfaces:**
- Consumes `RecipeManifest`.
- Produces `RecipeInstaller.Install(context.Context, RecipeManifest, string) (VisualSourceResult, error)`.

- [ ] **Step 1: Add failing local-HTTP tests for checksum mismatch, redirected host, Zip Slip, symlink, and deterministic page IDs.**

```go
func TestRecipeInstallerRejectsZipSlip(t *testing.T) {
    _, err := installer.Install(context.Background(), recipeWithArchive(zipWithEntry("../escape.pdf")), privateDir)
    if !errors.Is(err, ErrRecipeArchiveUnsafe) { t.Fatalf("err=%v", err) }
}
```

- [ ] **Step 2: Run the focused test.**

Run: `go test ./internal/tutorial -run TestRecipeInstallerRejectsZipSlip -count=1`

Expected: FAIL because `RecipeInstaller` is undefined.

- [ ] **Step 3: Add the installer with bounded streaming and extraction.**

```go
type RecipeInstaller struct { Client *http.Client; MaxBytes, MaxExtractedBytes int64; MaxEntries int }
func (i RecipeInstaller) Install(ctx context.Context, recipe RecipeManifest, root string) (VisualSourceResult, error) { /* fixed URLs, io.LimitReader, SHA-256, temporary directory, safe ZIP paths, atomic rename */ }
```

- [ ] **Step 4: Run installer tests.**

Run: `go test ./internal/tutorial -run 'TestRecipeInstaller' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit.**

```bash
git add internal/tutorial/recipe_fetch.go internal/tutorial/recipe_test.go
git commit -m "feat: install verified visual recipe sources"
```

### Task 3: Make clone orchestration recipe-aware and retry-safe

**Files:**
- Modify: `internal/tutorial/clone.go`
- Create: `internal/tutorial/recipe_clone.go`
- Modify: `internal/tutorial/clone_test.go`
- Modify: `internal/tutorial/clone_memory.go`

**Interfaces:**
- Produces new clone stages `download_recipe_source`, `verify_recipe_source`, and `convert_visual_pages`.
- Consumes `RecipeInstaller` and existing `CloneRepository` compare-and-swap operations.

- [ ] **Step 1: Write failing clone tests for idempotent source installation and resume from conversion.**

```go
func TestRecipeCloneRetryReusesVerifiedSource(t *testing.T) {
    // Fail conversion once, retry, and assert downloader.Calls == 1.
}
```

- [ ] **Step 2: Run the focused test.**

Run: `go test ./internal/tutorial -run TestRecipeCloneRetryReusesVerifiedSource -count=1`

Expected: FAIL until recipe stages are wired.

- [ ] **Step 3: Add recipe stage transitions without changing text Pack transitions.**

```go
func (s *CloneService) runRecipe(ctx context.Context, job CloneJob, template Template, pack PackRef) error { /* acquire, install verified source, persist safe stage, initialize visual resources */ }
```

- [ ] **Step 4: Run clone regression tests.**

Run: `go test ./internal/tutorial -run 'Test(RecipeClone|Clone)' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit.**

```bash
git add internal/tutorial/clone.go internal/tutorial/recipe_clone.go internal/tutorial/clone_test.go internal/tutorial/clone_memory.go
git commit -m "feat: add resumable visual recipe clone"
```

### Task 4: Add visual runtime isolation and P0 execution

**Files:**
- Create: `internal/tutorial/visual_runtime.go`
- Create: `internal/tutorial/visual_runtime_test.go`
- Modify: `internal/tutorial/runtime_resources.go`
- Modify: `internal/tutorial/run.go`

**Interfaces:**
- Produces `VisualRuntimeInitializer.Ensure(context.Context, CloneJob, RecipeManifest) (VisualRuntimeResources, error)`.
- Produces `VisualRunService.StartBaseline(context.Context, Subject, string, string) (ExperimentRun, error)`.

- [ ] **Step 1: Add failing tests for a visual P0 path and text-runtime rejection.**

```go
func TestVisualRunRejectsTextRuntimeFallback(t *testing.T) {
    _, err := service.StartBaseline(ctx, subject, projectID, "key")
    if !errors.Is(err, ErrVisualRuntimeUnavailable) { t.Fatalf("err=%v", err) }
}
```

- [ ] **Step 2: Run the focused test.**

Run: `go test ./internal/tutorial -run TestVisualRunRejectsTextRuntimeFallback -count=1`

Expected: FAIL because the visual run service is missing.

- [ ] **Step 3: Implement separate visual roots, capability checks, and fixed P0.**

```go
type VisualRuntimeResources struct { PageAssetRoot, DatasetID, EnvironmentSHA256 string }
func (s *VisualRunService) StartBaseline(ctx context.Context, subject Subject, projectID, key string) (ExperimentRun, error) { /* validate visual recipe and provider capability; create a durable P0 run */ }
```

- [ ] **Step 4: Run visual runtime tests and existing text run tests.**

Run: `go test ./internal/tutorial -run 'Test(Visual|LiveRun)' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit.**

```bash
git add internal/tutorial/visual_runtime.go internal/tutorial/visual_runtime_test.go internal/tutorial/runtime_resources.go internal/tutorial/run.go
git commit -m "feat: add isolated visual tutorial runtime"
```

### Task 5: Publish truthful catalog, API and Console states

**Files:**
- Modify: `internal/tutorial/catalog.json`
- Modify: `internal/http/tutorials.go`
- Modify: `api/openapi.yaml`
- Modify: `console/src/features/tutorials/tutorial-detail.tsx`
- Modify: `console/src/api/schema.d.ts`
- Test: `internal/http/router_test.go`

**Interfaces:**
- Consumes recipe clone status and `VisualRuntimeResources`.
- Produces a redacted recipe-aware tutorial response and Console recovery state.

- [ ] **Step 1: Add failing HTTP tests proving Recipe responses never reveal source/private coordinates.**

```go
if strings.Contains(body, "huggingface.co") || strings.Contains(body, "private/") { t.Fatal("source coordinates leaked") }
```

- [ ] **Step 2: Run the focused HTTP test.**

Run: `go test ./internal/http -run TestTutorialRecipeResponseIsRedacted -count=1`

Expected: FAIL until the response projection is updated.

- [ ] **Step 3: Update catalog/status copy, OpenAPI and Console recovery UI.**

```tsx
<p className="tutorial-dialog-note">确认许可后，服务会从固定上游版本下载到此项目私有存储并验证校验和。</p>
```

- [ ] **Step 4: Regenerate API schema and run web/API checks.**

Run: `make openapi-validate && npm --prefix console run typecheck && npm --prefix console test -- --run`

Expected: PASS.

- [ ] **Step 5: Commit.**

```bash
git add internal/tutorial/catalog.json internal/http/tutorials.go api/openapi.yaml console/src/features/tutorials/tutorial-detail.tsx console/src/api/schema.d.ts internal/http/router_test.go
git commit -m "feat: expose visual recipe clone status"
```

### Task 6: Add fixed-source integration fixtures and browser E2E

**Files:**
- Create: `tests/fixtures/tutorial-recipes/visual-document-rag/1.0.0/quick/*`
- Create: `console/e2e/real-backend-visual-tutorial-clone.spec.ts`
- Modify: `scripts/console-real-backend-tutorial-clone-e2e.sh`
- Modify: `Makefile`

**Interfaces:**
- Consumes local Recipe fixture served by the E2E HTTP server.
- Proves private clone, visual P0, redaction and recovery in PostgreSQL + Qdrant.

- [ ] **Step 1: Add a failing Playwright test for visual Quick clone and visual P0.**

```ts
await expect(page.getByText('视觉 Recipe 已验证并写入项目私有存储。')).toBeVisible()
await page.getByRole('link', { name: '打开视觉 P0 Live Run' }).click()
```

- [ ] **Step 2: Run the isolated E2E command.**

Run: `make console-real-visual-tutorial-clone-e2e`

Expected: FAIL until the local Recipe source fixture is wired.

- [ ] **Step 3: Serve a local fixed-source fixture and add the dedicated Make target.**

```make
console-real-visual-tutorial-clone-e2e:
	./scripts/console-real-backend-visual-tutorial-clone-e2e.sh
```

- [ ] **Step 4: Run the full dedicated E2E.**

Run: `make console-real-visual-tutorial-clone-e2e`

Expected: PASS with PostgreSQL, Qdrant, API, Console and no external source request.

- [ ] **Step 5: Commit.**

```bash
git add tests/fixtures/tutorial-recipes console/e2e/real-backend-visual-tutorial-clone.spec.ts scripts/console-real-backend-visual-tutorial-clone-e2e.sh Makefile
git commit -m "test: cover real visual tutorial recipe clone"
```

### Task 7: Create verified Replay, docs and release checks

**Files:**
- Modify: `internal/tutorial/replay.go`
- Modify: `internal/tutorial/replay_test.go`
- Create: `docs/tutorials/visual-document-rag.md`
- Modify: `docs-site/tutorials/index.html`
- Modify: `ROADMAP.md`
- Modify: `ROADMAP_EN.md`

**Interfaces:**
- Consumes verified Recipe and visual environment fingerprints.
- Produces aggregate-only `ReplaySnapshot` and documented source-recipe verification command.

- [ ] **Step 1: Add failing Replay tests for absence of source/private data and fingerprint binding.**

```go
if strings.Contains(encoded, "vidoseek_pdf_document.zip") || strings.Contains(encoded, "private/") { t.Fatal("replay leaked source data") }
```

- [ ] **Step 2: Run the focused test.**

Run: `go test ./internal/tutorial -run TestVisualReplay -count=1`

Expected: FAIL until the visual replay builder exists.

- [ ] **Step 3: Implement aggregate-only Replay and tutorial documentation.**

```go
func BuildVisualReplay(recipeSHA, environmentSHA, evaluator, build string, baseline, candidate ReplayVariant) (ReplaySnapshot, error) { /* validate aggregate-only metrics and derive fingerprint */ }
```

- [ ] **Step 4: Run release-level verification.**

Run: `go test ./... && make docs-build && make console-real-visual-tutorial-clone-e2e`

Expected: PASS.

- [ ] **Step 5: Commit.**

```bash
git add internal/tutorial/replay.go internal/tutorial/replay_test.go docs/tutorials/visual-document-rag.md docs-site/tutorials/index.html ROADMAP.md ROADMAP_EN.md
git commit -m "docs: publish visual tutorial recipe replay"
```

## Plan self-review

- Spec coverage: Tasks 1–2 implement pinned Recipe validation and direct, bounded source acquisition; Task 3 supplies durable private clone/retry; Task 4 isolates visual runtime; Task 5 protects public API/UI; Task 6 proves the real stack; Task 7 publishes only aggregate Replay evidence and documentation.
- Placeholder scan: the plan has no deferred-work markers or unspecified validation steps.
- Type consistency: `RecipeManifest` is consumed by the installer and clone orchestration; `VisualRuntimeResources` is produced by the visual initializer and consumed by the visual run service; public APIs receive only redacted projections.
