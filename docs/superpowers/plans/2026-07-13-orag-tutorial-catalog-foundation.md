# ORAG Tutorial Catalog Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver the first independently usable tutorial-space slice: a versioned, read-only catalog for the three approved end-to-end tutorials, authenticated catalog APIs, generated TypeScript contracts, and console library/detail pages.

**Architecture:** A focused `internal/tutorial` package owns immutable template types, embedded catalog authoring data, validation, latest-version selection, and lookup. `internal/app` constructs the catalog once at startup, `internal/http` exposes read-only endpoints, and the React console reads only the generated OpenAPI contract. Pack download, cloning, experiments, and OSS writes remain outside this phase and will consume the exact template and pack references produced here.

**Tech Stack:** Go 1.26.4, Hertz, Go `embed`, OpenAPI 3, React 18, TypeScript, TanStack Query, React Router, Vitest, Testing Library, MSW, Playwright.

## Global Constraints

- The only first-release tutorial modalities are `text`, `visual_document`, and `video`.
- The only first-release source benchmarks are CRUD-RAG, ViDoSeek, and Video-MME.
- Official tutorial data is anonymously downloadable from `https://orag.oss-cn-guangzhou.aliyuncs.com/tutorial-packs`; runtime catalog reads require no OSS credentials.
- Template versions are immutable; a new revision is a new semantic version string.
- System tutorial templates and Replay metadata are read-only.
- This phase must not add clone, dataset import, experiment run, model call, OSS upload, or user-private storage behavior.
- The API remains authenticated by the existing `/v1` middleware.
- Frontend types come from `api/openapi.yaml`; do not hand-maintain duplicate API DTOs.
- Existing unrelated `.superpowers/` files must remain untouched and uncommitted.

---

## File Structure

### New files

- `internal/tutorial/types.go`: public domain types and catalog errors.
- `internal/tutorial/catalog.go`: immutable catalog validation, latest-version selection, and lookup.
- `internal/tutorial/catalog.json`: build-time authoring source for the three approved tutorials.
- `internal/tutorial/catalog_test.go`: catalog validation and lookup tests.
- `internal/http/tutorials.go`: Hertz handlers and error mapping.
- `console/src/features/tutorials/tutorial-list.tsx`: global tutorial library page.
- `console/src/features/tutorials/tutorial-detail.tsx`: tutorial overview and pack/replay metadata page.
- `console/src/features/tutorials/tutorials.test.tsx`: list, detail, error, and retry coverage.
- `console/e2e/tutorials.spec.ts`: browser coverage for navigation from the global shell.

### Modified files

- `internal/config/config.go`: add `TutorialConfig.CatalogBaseURL` and environment loading.
- `internal/config/config_test.go`: pin the public default and override behavior.
- `internal/app/app.go`: construct and expose `*tutorial.Catalog`.
- `internal/app/app_test.go`: assert the application exposes the three built-in tutorials.
- `internal/http/router.go`: register the three read-only endpoints.
- `internal/http/router_test.go`: API authentication, list, current detail, version detail, and 404 tests.
- `api/openapi.yaml`: add paths and schemas for tutorial catalog resources.
- `console/src/api/schema.d.ts`: regenerate from OpenAPI.
- `console/src/api/client.ts`: add typed `tutorialApi` methods.
- `console/src/app/router.tsx`: add lazy `/tutorials` and `/tutorials/:templateId` routes plus global navigation.
- `console/src/test/handlers.ts`: add MSW tutorial fixtures and handlers.
- `console/src/styles.css`: add catalog, scenario-tag, pack, and detail styles using existing tokens.
- `.env.example`: document the non-secret catalog base URL.
- `configs/config.example.yaml`: document the tutorial catalog source.
- `README.md`: add the tutorial catalog API and console entry point.

---

### Task 1: Immutable Tutorial Domain And Embedded Catalog

**Files:**
- Create: `internal/tutorial/types.go`
- Create: `internal/tutorial/catalog.go`
- Create: `internal/tutorial/catalog.json`
- Test: `internal/tutorial/catalog_test.go`

**Interfaces:**
- Produces: `tutorial.Modality`, `tutorial.PackRef`, `tutorial.Template`, `tutorial.Catalog`, `tutorial.NewCatalog()`, `(*Catalog).List()`, and `(*Catalog).Get(id, version string)`.
- Consumes: no application services or storage; catalog construction is deterministic and side-effect free.

- [x] **Step 1: Write failing catalog tests**

Create table-driven tests that require exactly one latest template for each approved modality, reject duplicate `(id, version)` pairs, and distinguish missing template from missing version:

```go
func TestNewCatalogLoadsApprovedTemplates(t *testing.T) {
	catalog, err := NewCatalog()
	if err != nil { t.Fatal(err) }
	items := catalog.List()
	if len(items) != 3 { t.Fatalf("List() len = %d, want 3", len(items)) }
	want := []Modality{ModalityText, ModalityVideo, ModalityVisualDocument}
	got := make([]Modality, 0, len(items))
	for _, item := range items { got = append(got, item.Modality) }
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) { t.Fatalf("modalities = %v, want %v", got, want) }
}

func TestCatalogGetVersion(t *testing.T) {
	catalog, _ := NewCatalog()
	current, err := catalog.Get("text-rag", "")
	if err != nil { t.Fatal(err) }
	versioned, err := catalog.Get("text-rag", current.Version)
	if err != nil { t.Fatal(err) }
	if !reflect.DeepEqual(current, versioned) { t.Fatalf("versioned = %#v, current %#v", versioned, current) }
}
```

- [x] **Step 2: Run the focused test and verify RED**

Run:

```bash
GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./internal/tutorial -v
```

Expected: FAIL because `internal/tutorial` and its exported types do not exist.

- [x] **Step 3: Implement domain types and validation**

Define immutable value types and sentinel errors:

```go
type Modality string

const (
	ModalityText           Modality = "text"
	ModalityVisualDocument Modality = "visual_document"
	ModalityVideo          Modality = "video"
)

type PackRef struct {
	Tier                 string `json:"tier"`
	ManifestPath         string `json:"manifest_path"`
	EstimatedBytes       int64  `json:"estimated_bytes"`
	EstimatedMinutes     int    `json:"estimated_minutes"`
	RequiresLicenseCheck bool   `json:"requires_license_check"`
}

type Template struct {
	ID                       string     `json:"id"`
	Slug                     string     `json:"slug"`
	Title                    string     `json:"title"`
	Summary                  string     `json:"summary"`
	Version                  string     `json:"version"`
	Status                   string     `json:"status"`
	Modality                 Modality   `json:"modality"`
	Difficulty               string     `json:"difficulty"`
	EstimatedDurationMinutes int        `json:"estimated_duration_minutes"`
	SourceBenchmark          string     `json:"source_benchmark"`
	SourceURL                string     `json:"source_url"`
	ScenarioDimensions       []string   `json:"scenario_dimensions"`
	PipelineStages           []string   `json:"pipeline_stages"`
	RequiredCapabilities     []string   `json:"required_capabilities"`
	Packs                    []PackRef  `json:"packs"`
	ReplayAvailable          bool       `json:"replay_available"`
}

var (
	ErrTemplateNotFound = errors.New("tutorial template not found")
	ErrVersionNotFound  = errors.New("tutorial template version not found")
)
```

Use `//go:embed catalog.json`, decode with `DisallowUnknownFields`, validate non-empty IDs, semantic version strings, approved modalities, HTTPS source URLs, `quick` and `benchmark` pack tiers, relative manifest paths, and uniqueness. Copy slices before returning values so callers cannot mutate catalog state.

- [x] **Step 4: Author the three version `1.0.0` templates**

Create JSON entries with stable IDs `text-rag`, `visual-document-rag`, and `video-rag`. Their source benchmarks must be `CRUD-RAG`, `ViDoSeek`, and `Video-MME`; their scenario dimensions must include the approved negative or insufficient-evidence slice; pack manifests must resolve under:

```text
text-rag/1.0.0/{quick|benchmark}/manifest.json
visual-document-rag/1.0.0/{quick|benchmark}/manifest.json
video-rag/1.0.0/{quick|benchmark}/manifest.json
```

- [x] **Step 5: Run tests and verify GREEN**

Run the focused Go test from Step 2.

Expected: PASS with all catalog tests successful.

- [x] **Step 6: Commit the domain slice**

```bash
git add internal/tutorial
git commit -m "feat: add versioned tutorial catalog"
```

---

### Task 2: Configuration And Application Wiring

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `internal/app/app.go`
- Modify: `internal/app/app_test.go`
- Modify: `.env.example`
- Modify: `configs/config.example.yaml`

**Interfaces:**
- Consumes: `tutorial.NewCatalog() (*tutorial.Catalog, error)` from Task 1.
- Produces: `config.Config.Tutorial.CatalogBaseURL string` and `app.App.Tutorials *tutorial.Catalog`.

- [x] **Step 1: Write failing configuration and app tests**

Add assertions:

```go
if got := cfg.Tutorial.CatalogBaseURL; got != "https://orag.oss-cn-guangzhou.aliyuncs.com/tutorial-packs" {
	t.Fatalf("tutorial catalog base URL = %q", got)
}

t.Setenv("TUTORIAL_CATALOG_BASE_URL", "https://example.test/packs")
cfg = config.Load()
if cfg.Tutorial.CatalogBaseURL != "https://example.test/packs" { t.Fatal("override not applied") }
```

In the app test, construct the normal mock-provider application and assert `app.Tutorials.List()` contains three items.

- [x] **Step 2: Run focused tests and verify RED**

```bash
GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./internal/config ./internal/app -run 'Test.*Tutorial' -v
```

Expected: FAIL because `Config.Tutorial` and `App.Tutorials` do not exist.

- [x] **Step 3: Add exact configuration fields**

```go
type TutorialConfig struct {
	CatalogBaseURL string
}

type Config struct {
	Server   ServerConfig
	Storage  StorageConfig
	Auth     AuthConfig
	Database DatabaseConfig
	Qdrant   QdrantConfig
	Ark      ArkConfig
	Models   ModelProviderConfig
	RAG      RAGConfig
	Ingestion IngestionConfig
	ObjectStorage ObjectStorageConfig
	Observability ObservabilityConfig
	Maintenance MaintenanceConfig
	Tutorial TutorialConfig
}
```

Load `TUTORIAL_CATALOG_BASE_URL` with the exact public default. Do not add official OSS AccessKey fields to tutorial configuration.

- [x] **Step 4: Construct the catalog during app startup**

Call `tutorial.NewCatalog()` before returning `App`. Propagate validation errors so invalid embedded authoring data fails startup, then assign the result to `App.Tutorials`.

- [x] **Step 5: Document non-secret configuration**

Add the exact environment value to `.env.example` and:

```yaml
tutorial:
  catalog_base_url: https://orag.oss-cn-guangzhou.aliyuncs.com/tutorial-packs
```

to `configs/config.example.yaml`.

- [x] **Step 6: Run focused tests and verify GREEN**

Run the command from Step 2.

Expected: PASS.

- [x] **Step 7: Commit the application wiring**

```bash
git add internal/config internal/app .env.example configs/config.example.yaml
git commit -m "feat: wire tutorial catalog configuration"
```

---

### Task 3: Authenticated Tutorial Catalog API And OpenAPI Contract

**Files:**
- Create: `internal/http/tutorials.go`
- Modify: `internal/http/router.go`
- Modify: `internal/http/router_test.go`
- Modify: `api/openapi.yaml`
- Modify: `console/src/api/schema.d.ts`

**Interfaces:**
- Consumes: `App.Tutorials.List()` and `App.Tutorials.Get(templateID, version)`.
- Produces: `GET /v1/tutorials`, `GET /v1/tutorials/{template_id}`, and `GET /v1/tutorials/{template_id}/versions/{version}` with generated `TutorialTemplate` and `TutorialPackRef` schemas.

- [x] **Step 1: Write failing router tests**

Use the existing authenticated test helper and assert:

```go
resp := performJSON(h, "GET", "/v1/tutorials", "", token)
if resp.Code != http.StatusOK { t.Fatalf("status = %d body=%s", resp.Code, resp.Body) }
var body struct { Tutorials []tutorial.Template `json:"tutorials"` }
if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil { t.Fatal(err) }
if len(body.Tutorials) != 3 { t.Fatalf("tutorials = %d", len(body.Tutorials)) }
```

Add tests for current detail, explicit `1.0.0`, unknown template `tutorial_not_found`, unknown version `tutorial_version_not_found`, and unauthenticated `401`.

- [x] **Step 2: Run the focused router tests and verify RED**

```bash
GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./internal/http -run 'TestTutorial' -v
```

Expected: FAIL because the routes return 404.

- [x] **Step 3: Implement handlers and routes**

Register:

```go
v1.GET("/tutorials", s.listTutorials)
v1.GET("/tutorials/:template_id", s.getTutorial)
v1.GET("/tutorials/:template_id/versions/:version", s.getTutorialVersion)
```

Return `{ "tutorials": [...] }` for the list. Map `ErrTemplateNotFound` to `404 tutorial_not_found` and `ErrVersionNotFound` to `404 tutorial_version_not_found`; unexpected errors use `500 tutorial_catalog_failed` without leaking internal catalog data.

- [x] **Step 4: Add exact OpenAPI paths and schemas**

Define `TutorialModality`, `TutorialPackRef`, `TutorialTemplate`, and `TutorialListResponse`; document authentication, 200 responses, both 404 codes, and examples for all three templates. `manifest_url` in API responses must be the resolved URL produced from `Config.Tutorial.CatalogBaseURL` plus the relative manifest path; retain `manifest_path` only inside the embedded authoring file.

- [x] **Step 5: Resolve manifest URLs at the HTTP boundary**

Add a response mapper in `internal/http/tutorials.go` that joins the configured base URL and the catalog's relative manifest path with `net/url`, rejecting path traversal and non-HTTPS results. Domain values remain storage-agnostic.

- [x] **Step 6: Regenerate TypeScript contracts**

```bash
npm --prefix console run api:generate
```

Expected: `console/src/api/schema.d.ts` includes the new schemas and operations.

- [x] **Step 7: Validate API and run tests**

```bash
make openapi-validate
GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./internal/http -run 'TestTutorial' -v
```

Expected: both commands PASS.

- [x] **Step 8: Commit the API slice**

```bash
git add internal/http api/openapi.yaml console/src/api/schema.d.ts
git commit -m "feat: expose tutorial catalog api"
```

---

### Task 4: Console Tutorial Library And Detail Pages

**Files:**
- Create: `console/src/features/tutorials/tutorial-list.tsx`
- Create: `console/src/features/tutorials/tutorial-detail.tsx`
- Create: `console/src/features/tutorials/tutorials.test.tsx`
- Create: `console/e2e/tutorials.spec.ts`
- Modify: `console/src/api/client.ts`
- Modify: `console/src/app/router.tsx`
- Modify: `console/src/test/handlers.ts`
- Modify: `console/src/styles.css`

**Interfaces:**
- Consumes: generated `components['schemas']['TutorialTemplate']`, list response, and the three HTTP routes from Task 3.
- Produces: global `/tutorials` library, `/tutorials/:templateId` detail page, a global “教程实验室” navigation entry, loading/error/empty states, and visible Replay/Live availability.

- [x] **Step 1: Add failing MSW-backed component tests**

Cover:

```tsx
it('renders three end-to-end tutorials', async () => {
  renderApp(['/tutorials'])
  expect(await screen.findByRole('heading', { name: '教程与实验室' })).toBeInTheDocument()
  expect(screen.getByText('中文文本 RAG')).toBeInTheDocument()
  expect(screen.getByText('视觉文档 RAG')).toBeInTheDocument()
  expect(screen.getByText('视频 RAG')).toBeInTheDocument()
})

it('shows scenario dimensions and pack requirements', async () => {
  renderApp(['/tutorials/video-rag'])
  expect(await screen.findByRole('heading', { name: '视频 RAG' })).toBeInTheDocument()
  expect(screen.getByText('Replay 可用')).toBeInTheDocument()
  expect(screen.getByText('Quick Pack')).toBeInTheDocument()
  expect(screen.getByText('Benchmark Pack')).toBeInTheDocument()
})
```

Also cover API failure with a retry button and unknown detail with a return-to-library link.

- [x] **Step 2: Run component tests and verify RED**

```bash
npm --prefix console test -- --run src/features/tutorials/tutorials.test.tsx
```

Expected: FAIL because routes and components do not exist.

- [x] **Step 3: Add typed tutorial client methods**

```ts
export type TutorialTemplate = components['schemas']['TutorialTemplate']

export const tutorialApi = {
  list: () => request<{ tutorials: TutorialTemplate[] }>('/v1/tutorials'),
  get: (templateId: string) => request<TutorialTemplate>(`/v1/tutorials/${encodeURIComponent(templateId)}`),
  getVersion: (templateId: string, version: string) => request<TutorialTemplate>(
    `/v1/tutorials/${encodeURIComponent(templateId)}/versions/${encodeURIComponent(version)}`,
  ),
}
```

- [x] **Step 4: Implement lazy routes and global navigation**

Add route-level lazy imports for list/detail and a visible `/tutorials` `NavLink` labelled `教程实验室`. Keep project-specific RAG Studio, Evaluation, and Release entries disabled as they are today.

- [x] **Step 5: Implement list and detail states**

The list groups nothing by individual modules; it renders exactly three end-to-end tutorial cards. Cards show modality, benchmark source, scenario dimensions, P0-P8 stage count, Quick/Benchmark estimates, and Replay availability. The detail page shows the dataset-first experiment matrix, pack requirements, source link, and disabled “克隆教程” action labelled `即将开放` until the clone phase lands.

- [x] **Step 6: Add scoped styles**

Reuse `--ink`, `--muted`, `--line`, and `--accent`. Add `.tutorial-grid`, `.tutorial-card`, `.tutorial-tags`, `.tutorial-pack-grid`, and `.tutorial-detail` without changing existing project layouts or responsive breakpoints.

- [x] **Step 7: Make component tests GREEN**

Run the command from Step 2.

Expected: PASS.

- [x] **Step 8: Add and run Playwright coverage**

The E2E test must navigate from the global rail to `/tutorials`, open `video-rag`, assert the public benchmark source and both Pack tiers, and confirm the disabled clone action.

```bash
npm --prefix console run test:e2e -- tutorials.spec.ts
```

Expected: PASS.

- [x] **Step 9: Commit the console slice**

```bash
git add console/src console/e2e/tutorials.spec.ts
git commit -m "feat: add tutorial catalog console"
```

---

### Task 5: Documentation And Phase Verification

**Files:**
- Modify: `README.md`
- Modify: `docs/README.md`
- Modify: `docs/api/README.md`
- Modify: `docs/api/auth-and-errors.md`
- Modify: `docs/superpowers/plans/2026-07-13-orag-tutorial-catalog-foundation.md`

**Interfaces:**
- Consumes: the final API routes, environment variable, and console routes.
- Produces: user-facing entry points and a checked implementation record for the catalog phase.

- [x] **Step 1: Document the catalog without overstating later phases**

Add:

- `/tutorials` as the read-only console tutorial library.
- The three authenticated catalog endpoints.
- `TUTORIAL_CATALOG_BASE_URL` as a non-secret public download base.
- A clear statement that cloning, Pack installation, Live experiments, dataset generation, and video ingestion are later phases.
- `tutorial_not_found`, `tutorial_version_not_found`, and `tutorial_catalog_failed` error codes.

- [x] **Step 2: Run the complete phase verification**

```bash
GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./internal/tutorial ./internal/config ./internal/app ./internal/http
GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go vet ./internal/tutorial ./internal/config ./internal/app ./internal/http
make openapi-validate
npm --prefix console run typecheck
npm --prefix console test -- --run
npm --prefix console run build
npm --prefix console run test:e2e -- tutorials.spec.ts
git diff --check
```

Expected: every command exits 0; Vitest and Playwright report zero failed tests.

- [x] **Step 3: Review scope and generated files**

```bash
git status --short
git diff --stat origin/main...HEAD
git diff --name-only origin/main...HEAD
```

Expected: only the design/plan documents and files named by this plan are present; `.superpowers/` is not staged or committed.

- [x] **Step 4: Mark completed plan checkboxes and commit documentation**

```bash
git add README.md docs/README.md docs/api/README.md docs/api/auth-and-errors.md docs/superpowers/plans/2026-07-13-orag-tutorial-catalog-foundation.md
git commit -m "docs: document tutorial catalog foundation"
```

- [x] **Step 5: Run final clean-state proof**

```bash
git status --short --branch
git log -5 --oneline --decorate
```

Expected: the feature worktree is clean except for pre-existing ignored or explicitly excluded files, and the recent commits correspond to Tasks 1-5.
