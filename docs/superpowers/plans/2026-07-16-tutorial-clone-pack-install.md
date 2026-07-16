# Tutorial Clone and Pack Installation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox syntax for tracking.

**Goal:** Let an authenticated tenant clone an immutable tutorial template into a normal project and install its selected Quick or Benchmark Pack through a server-owned, durable, resumable workflow.

**Architecture:** The global tutorial catalog remains read-only. A clone request creates a tenant-owned project plus a persisted tutorial experiment and deduplicated clone job; a process-local worker advances that job from durable checkpoints. Only this worker resolves catalog-relative manifests, reads public pack data anonymously, validates checksums, and writes to the configured private output store. The Console only selects a catalog pack and polls job state.

**Tech Stack:** Go 1.26, Hertz, PostgreSQL/Goose migrations, current memory backend, net/http and httptest, React, TypeScript, TanStack Query, MSW, Playwright, and canonical OpenAPI.

## Global Constraints

- Preserve the immutable global tutorial catalog. Do not accept a manifest URL, object-storage credential, completed-stage flag, or tenant ID from a browser.
- Scope clone jobs and experiments by tenant and target project. A project API key may read only its own job and experiment; only a tenant admin can start a clone because it creates a project.
- The public tutorial origin is anonymous, HTTPS, and allowlisted to TUTORIAL_CATALOG_BASE_URL. It is never usable as a private-output writer.
- Every manifest object has a canonical relative path, byte count, MIME type, lowercase SHA-256, and explicit redistributable license decision. Reject origin escapes, redirects outside the origin, traversal, oversized responses, unknown JSON fields, and checksum mismatch.
- The private output writer uses deterministic project/job prefixes. Do not return bucket names, object keys, signed URLs, Access Keys, secrets, or manifest-private detail through HTTP, Console, logs, traces, or metrics.
- A duplicate tuple of tenant, subject, template, template version, and idempotency key returns the original project/job. Retry resumes from the first incomplete checksum-valid stage; templates are never changed.
- This slice completes design phase 2 only: a selected Pack becomes verified and copied, and the experiment reaches pack_installed. Knowledge-base/dataset creation, P0-P8 indexing, Live Run, visual/video execution, Replay rendering, and result comparison remain later slices and must be marked unavailable.
- No template is operationally cloneable until an unauthenticated GET of its Quick manifest and every referenced object succeeds from the release environment. On 2026-07-16 the configured OSS URL returns 403 AccessDenied. Preserve that honest, resumable failure until the bucket ACL and objects are published.

---

## File Structure

- internal/tutorial/manifest.go: strict manifest data model, parser, catalog-origin resolution, and deterministic validation.
- internal/tutorial/public_reader.go: bounded anonymous HTTPS fetch and streamed checksum verification.
- internal/tutorial/clone.go: clone/experiment types, state machine, repository interface, idempotent service, and worker steps.
- internal/tutorial/clone_memory.go: concurrency-safe memory repository and private-store fake.
- internal/tutorial manifest/clone tests and testdata: negative and recovery coverage.
- internal/storage/postgres/tutorial_clone.go and migration 000026_tutorial_clone_jobs.sql: durable jobs, experiments, stage events, and compare-and-swap transitions.
- internal/config/config.go and .env.example: tutorial reader limits and private output config.
- internal/app/app.go: clone service and lifecycle-managed worker wiring.
- internal/auth/policy.go: explicit clone creation/read policy actions.
- internal/http/tutorial_clones.go, router, and tests: request validation, auth, and error mapping.
- api/openapi.yaml: experimental clone/job/experiment schema and routes; console schema generated from it.
- Console tutorials feature, MSW handlers, Playwright spec, real backend fixture script, and CI: pack chooser and evidence.
- Tutorial docs, README, changelog, and Roadmaps: deploy preflight, recovery, and truthful scope.

## Task 1: Strict public Pack manifests

**Files:**

- Create: internal/tutorial/manifest.go
- Create: internal/tutorial/manifest_test.go
- Create: internal/tutorial/testdata/pack-manifest-valid.json
- Create: internal/tutorial/testdata/pack-manifest-invalid-sha.json
- Modify: internal/tutorial/types.go
- Modify: internal/tutorial/catalog.go

**Interfaces:**

- Consumes existing Template, PackRef, and catalog-relative ManifestPath.
- Produces ParseManifest(raw []byte, template Template, pack PackRef) (Manifest, error).

- [ ] **Step 1: Write failing parser coverage**

~~~go
func TestParseManifestBindsTemplateAndRejectsHostileObjects(t *testing.T) {
    template, _ := NewCatalog().Get("text-rag", "1.0.0")
    pack := template.Packs[0]
    got, err := ParseManifest(loadFixture(t, "pack-manifest-valid.json"), template, pack)
    if err != nil || got.TemplateID != template.ID || len(got.Objects) != 2 {
        t.Fatalf("manifest=%#v err=%v", got, err)
    }
    if _, err := ParseManifest([]byte("{\"objects\":[{\"path\":\"../secret\"}]}"), template, pack); err == nil {
        t.Fatal("expected invalid object path")
    }
}
~~~

- [ ] **Step 2: Run the focused test**

Run: go test ./internal/tutorial -run TestParseManifestBindsTemplateAndRejectsHostileObjects -count=1

Expected: FAIL because ParseManifest and Manifest do not exist.

- [ ] **Step 3: Implement strict types and validation**

~~~go
type Manifest struct {
    TemplateID string
    Version string
    Tier string
    License License
    Objects []PackObject
}
type PackObject struct {
    Path string
    SHA256 string
    Bytes int64
    ContentType string
}
func ParseManifest(raw []byte, template Template, pack PackRef) (Manifest, error)
~~~

Use json.Decoder.DisallowUnknownFields. Require a redistributable license decision, lowercase 64-character checksum, positive bounded size, supported MIME, and clean relative object path. Verify manifest template ID, version, and tier equal the selected catalog values. Copy returned slices.

- [ ] **Step 4: Verify package behavior**

Run: go test ./internal/tutorial -count=1

Expected: PASS for valid manifest, unknown fields, duplicate objects, traversal, checksum syntax, MIME, and template/pack mismatch.

- [ ] **Step 5: Commit**

~~~bash
git add internal/tutorial/manifest.go internal/tutorial/manifest_test.go internal/tutorial/testdata internal/tutorial/types.go internal/tutorial/catalog.go
git commit -m "feat(tutorial): validate immutable pack manifests"
~~~

## Task 2: Durable clone and experiment state

**Files:**

- Create: migrations/000026_tutorial_clone_jobs.sql
- Create: internal/tutorial/clone.go
- Create: internal/tutorial/clone_memory.go
- Create: internal/tutorial/clone_test.go
- Create: internal/storage/postgres/tutorial_clone.go
- Modify: internal/storage/postgres/repository.go
- Modify: internal/app/app.go
- Test: tests/integration/tutorial_clone_postgres_test.go

**Interfaces:**

- Consumes project.Service and Manifest.
- Produces CloneService.Start, GetJob, Retry, GetExperiment, and a durable Repository.

- [ ] **Step 1: Write failing idempotence/retry tests**

~~~go
func TestCloneStartIsIdempotentAndRetryResumesCheckpoint(t *testing.T) {
    svc, repo := newCloneService(t)
    input := CloneRequest{
        TemplateID: "text-rag", Version: "1.0.0", Tier: "quick",
        ProjectName: "Text lab", IdempotencyKey: "req_1",
    }
    first, replayed, err := svc.Start(ctx, Subject{TenantID: "tenant_a", ID: "user_a"}, input)
    if err != nil || replayed { t.Fatalf("first=%#v replayed=%v err=%v", first, replayed, err) }
    again, replayed, err := svc.Start(ctx, sameSubject, input)
    if err != nil || !replayed || again.ID != first.ID || repo.projectCreates != 1 {
        t.Fatal("duplicate created a second project")
    }
    repo.failAt(StageVerifyPack, errors.New("checksum mismatch"))
    runOne(t, svc, first.ID)
    retried, err := svc.Retry(ctx, sameSubject, first.ID)
    if err != nil || retried.Stage != StageFetchPack || retried.Attempt != 2 {
        t.Fatalf("retry=%#v err=%v", retried, err)
    }
}
~~~

- [ ] **Step 2: Run focused test**

Run: go test ./internal/tutorial -run 'TestClone(StartIsIdempotentAndRetryResumesCheckpoint|JobTenantIsolation)' -count=1

Expected: FAIL because the clone domain does not exist.

- [ ] **Step 3: Add migration invariants**

~~~sql
CREATE TABLE tutorial_experiments (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES tenants(id),
  project_id TEXT NOT NULL UNIQUE REFERENCES projects(id) ON DELETE CASCADE,
  template_id TEXT NOT NULL,
  template_version TEXT NOT NULL,
  pack_tier TEXT NOT NULL CHECK (pack_tier IN ('quick','benchmark')),
  pack_status TEXT NOT NULL CHECK (pack_status IN ('pending','installing','pack_installed','failed')),
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);
CREATE TABLE tutorial_clone_jobs (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES tenants(id),
  subject_id TEXT NOT NULL,
  project_id TEXT NOT NULL UNIQUE REFERENCES projects(id) ON DELETE CASCADE,
  template_id TEXT NOT NULL,
  template_version TEXT NOT NULL,
  pack_tier TEXT NOT NULL,
  idempotency_key TEXT NOT NULL,
  stage TEXT NOT NULL,
  status TEXT NOT NULL,
  attempt INTEGER NOT NULL DEFAULT 1,
  last_error_code TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  UNIQUE (tenant_id, subject_id, template_id, template_version, idempotency_key)
);
CREATE TABLE tutorial_clone_stage_events (
  id BIGSERIAL PRIMARY KEY,
  job_id TEXT NOT NULL REFERENCES tutorial_clone_jobs(id) ON DELETE CASCADE,
  stage TEXT NOT NULL,
  outcome TEXT NOT NULL,
  detail_code TEXT NOT NULL DEFAULT '',
  occurred_at TIMESTAMPTZ NOT NULL
);
~~~

Add indexes for tenant updated time and project. The down migration removes events, jobs, then experiments. Conditional UPDATE based on expected status and stage gives one worker ownership of a stage.

- [ ] **Step 4: Implement domain and repository boundaries**

~~~go
type Repository interface {
    FindByIdempotency(context.Context, Subject, TemplateRef, string) (CloneJob, bool, error)
    Create(context.Context, CloneJob, Experiment) error
    GetJob(context.Context, string, string) (CloneJob, bool, error)
    Transition(context.Context, CloneTransition) (CloneJob, bool, error)
    Events(context.Context, string, string) ([]StageEvent, error)
}
type CloneJob struct {
    ID, TenantID, SubjectID, ProjectID, TemplateID, TemplateVersion, Tier string
    Stage Stage
    Status Status
    Attempt int
    LastErrorCode string
    CreatedAt, UpdatedAt time.Time
}
~~~

Do not persist raw URLs, object keys, credentials, or external error bodies. Memory state must be locked and return copies. Expose the PostgreSQL implementation through knowledgeBackend alongside existing project and release repositories.

- [ ] **Step 5: Validate unit and PostgreSQL integration**

Run: go test ./internal/tutorial ./internal/storage/postgres ./internal/app -count=1 && go test ./tests/integration -run TutorialClone -count=1

Expected: PASS for idempotency, tenant isolation, CAS ownership, append-only events, and cascade cleanup.

- [ ] **Step 6: Commit**

~~~bash
git add migrations/000026_tutorial_clone_jobs.sql internal/tutorial/clone.go internal/tutorial/clone_memory.go internal/tutorial/clone_test.go internal/storage/postgres/tutorial_clone.go internal/storage/postgres/repository.go internal/app/app.go tests/integration/tutorial_clone_postgres_test.go
git commit -m "feat(tutorial): persist resumable clone jobs"
~~~

## Task 3: Bounded public read and private installation

**Files:**

- Create: internal/tutorial/public_reader.go
- Create: internal/tutorial/private_store.go
- Create: internal/tutorial/public_reader_test.go
- Modify: internal/tutorial/clone.go
- Modify: internal/config/config.go
- Modify: internal/config/config_test.go
- Modify: .env.example
- Modify go.mod and go.sum only if a private Aliyun writer cannot use existing dependencies

**Interfaces:**

- Consumes catalog-derived manifest paths and Manifest.Objects.
- Produces PublicPackReader.FetchManifest, PublicPackReader.FetchObject, PrivateStore.PutVerified, and CloneService.RunOne.

- [ ] **Step 1: Write failing transport tests**

~~~go
func TestPublicPackReaderRejectsRedirectAndChecksumMismatch(t *testing.T) {
    reader := NewPublicPackReader(server.URL+"/tutorial-packs", 1<<20, http.DefaultClient)
    if _, err := reader.FetchObject(ctx, "https://evil.invalid/object", PackObject{}); err == nil {
        t.Fatal("expected origin rejection")
    }
    _, err := reader.FetchObject(ctx, server.URL+"/tutorial-packs/a.txt", PackObject{
        Path: "a.txt", Bytes: 2, SHA256: strings.Repeat("0", 64), ContentType: "text/plain",
    })
    if !errors.Is(err, ErrObjectChecksum) { t.Fatalf("err=%v", err) }
}
~~~

- [ ] **Step 2: Run focused test**

Run: go test ./internal/tutorial -run 'TestPublicPackReader|TestCloneWorker' -count=1

Expected: FAIL because public reader and private store do not exist.

- [ ] **Step 3: Implement isolated adapters and worker**

~~~go
type PublicPackReader interface {
    FetchManifest(context.Context, string) ([]byte, error)
    FetchObject(context.Context, string, PackObject) (VerifiedObject, error)
}
type PrivateStore interface {
    PutVerified(context.Context, PrivateObject) error
}
func (s *CloneService) RunOne(ctx context.Context, jobID string) error
~~~

Resolve URLs only from CatalogBaseURL plus canonical catalog paths. Disable redirect following or revalidate each redirect scheme, host, and path. Set deadline and byte cap, stream through io.LimitReader, and verify MIME, byte count, and SHA-256 before committing. On failure delete temporary output. Support test local-directory storage and a private Aliyun OSS bucket under tutorial-experiments/tenant/project/sha256; the public reader stays anonymous HTTPS. A missing private configuration creates the redacted, retryable storage_not_configured error.

Advance created, validate_manifest, download_pack, verify_pack, write_private_store, pack_installed. Events contain stage, outcome, counters, checksum, and stable failure code only. This slice does not create a KB, Dataset, Pipeline, Trace, or fake Run.

- [ ] **Step 4: Add config and redaction coverage**

~~~go
type TutorialConfig struct {
    CatalogBaseURL string
    MaxManifestBytes int64
    MaxObjectBytes int64
    HTTPTimeout time.Duration
    PrivateOutputPrefix string
}
~~~

Require HTTPS in non-test configuration, positive limits, distinct public/private buckets, and server-only OBJECT_STORAGE_ACCESS_KEY values. Extend RedactedEnv tests so secret, key, object path, bucket content, and signed URL do not appear in diagnostics.

- [ ] **Step 5: Verify backend behavior**

Run: go test ./internal/tutorial ./internal/config ./internal/app -count=1

Expected: PASS for success, 403/404/redirect, oversized object, checksum mismatch, private-write error, checkpoint retry, and leakage prevention.

- [ ] **Step 6: Commit**

~~~bash
git add internal/tutorial/public_reader.go internal/tutorial/private_store.go internal/tutorial/public_reader_test.go internal/tutorial/clone.go internal/config/config.go internal/config/config_test.go .env.example go.mod go.sum
git commit -m "feat(tutorial): install verified packs privately"
~~~

## Task 4: Secure OpenAPI clone control plane

**Files:**

- Create: internal/http/tutorial_clones.go
- Modify: internal/http/router.go
- Modify: internal/http/router_test.go
- Modify: internal/auth/policy.go
- Modify: internal/auth/policy_test.go
- Modify: api/openapi.yaml
- Modify: Makefile only if the existing OpenAPI target needs an extra tutorial-contract assertion

**Interfaces:**

- Consumes CloneService operations and requestPrincipal/authorizeRequest.
- Produces POST tutorial clone, GET clone job, POST clone retry, and GET project tutorial experiment endpoints.

- [ ] **Step 1: Write failing HTTP contract tests**

~~~go
func TestTutorialCloneCreatesOneTenantProjectAndNeverLeaksStorage(t *testing.T) {
    created := performJSON(h, "POST", "/v1/tutorials/text-rag/clones",
        "{\"version\":\"1.0.0\",\"pack_tier\":\"quick\",\"project\":{\"name\":\"Text lab\"},\"idempotency_key\":\"req_1\"}", adminToken)
    if created.Code != http.StatusAccepted { t.Fatalf("status=%d body=%s", created.Code, created.Body) }
    assertJSONNotContains(t, created.Body, "bucket", "access_key", "object_key", "manifest_url")
    assertErrorResponse(t, performJSON(h, "POST", "/v1/tutorials/text-rag/clones", sameBody, projectEditorToken), 403, "forbidden", "")
}
~~~

- [ ] **Step 2: Run focused router test**

Run: go test ./internal/http -run 'TestTutorialClone|TestTutorialCloneJob' -count=1

Expected: FAIL because clone routes and handlers do not exist.

- [ ] **Step 3: Implement requests, responses, auth, and mappings**

~~~yaml
post /v1/tutorials/{template_id}/clones:
  request: version, pack_tier, project name/description, idempotency_key
  responses: 202, 400, 401, 403, 404, 409
get /v1/tutorial-clone-jobs/{job_id}:
  responses: 200, 401, 404
post /v1/tutorial-clone-jobs/{job_id}:retry:
  responses: 202, 401, 403, 404, 409
~~~

Mark every added operation and schema experimental. Return job_id, project_id, status, stage, attempt, redacted failure_code, events, and a relative poll URL. Add ActionTutorialCloneCreate and ActionTutorialCloneRead. Validate project ownership on reads, map template/version absence to 404, invalid tier/name/key to 400, and nonretryable active state to 409. POST schedules after durable commit and never waits for transfer completion.

- [ ] **Step 4: Generate and validate contract**

Run: make openapi-validate && npm --prefix console run api:generate && git diff --check

Expected: PASS and generated console schema contains clone/job/experiment types.

- [ ] **Step 5: Run authorization and HTTP suites**

Run: go test ./internal/auth ./internal/http ./internal/app -count=1

Expected: PASS for malformed input, duplicate key, cross-tenant job/project, restricted API keys, retry, and redaction.

- [ ] **Step 6: Commit**

~~~bash
git add internal/http/tutorial_clones.go internal/http/router.go internal/http/router_test.go internal/auth/policy.go internal/auth/policy_test.go api/openapi.yaml Makefile console/src/api/schema.ts
git commit -m "feat(api): add tutorial clone control plane"
~~~

## Task 5: Console Pack selection and truthful setup progress

**Files:**

- Modify: console/src/api/client.ts
- Modify: console/src/features/tutorials/tutorial-detail.tsx
- Create: console/src/features/tutorials/tutorial-clone-progress.tsx
- Modify: console/src/features/tutorials/tutorials.test.tsx
- Modify: console/src/test/handlers.ts
- Create: console/e2e/tutorial-clone.spec.ts
- Modify: console/src/tutorials.css

**Interfaces:**

- Consumes generated clone/job/experiment schemas.
- Produces a pack-selection dialog and project setup route at /projects/:projectId/tutorial/setup.

- [ ] **Step 1: Write failing React coverage**

~~~tsx
it("starts the chosen Quick Pack clone and shows server progress", async () => {
  renderApp("/tutorials/text-rag")
  await userEvent.click(await screen.findByRole("button", { name: "克隆教程" }))
  await userEvent.click(screen.getByRole("radio", { name: "Quick Pack" }))
  await userEvent.click(screen.getByRole("checkbox", { name: "我已确认数据许可" }))
  await userEvent.click(screen.getByRole("button", { name: "创建实验项目" }))
  expect(await screen.findByText("正在校验数据包")).toBeVisible()
  expect(screen.queryByText(/manifest_url|access key/i)).not.toBeInTheDocument()
})
~~~

- [ ] **Step 2: Run focused Console test**

Run: npm --prefix console run test -- --run tutorials

Expected: FAIL because clone is disabled.

- [ ] **Step 3: Implement client and accessible UI**

~~~ts
export const tutorialApi = {
  startClone: (templateId: string, input: StartTutorialCloneRequest) =>
    request<TutorialCloneAccepted>("/v1/tutorials/" + encodeURIComponent(templateId) + "/clones", { method: "POST", body: JSON.stringify(input) }),
  getCloneJob: (jobId: string) =>
    request<TutorialCloneJob>("/v1/tutorial-clone-jobs/" + encodeURIComponent(jobId)),
  retryClone: (jobId: string) =>
    request<TutorialCloneJob>("/v1/tutorial-clone-jobs/" + encodeURIComponent(jobId) + ":retry", { method: "POST" }),
}
~~~

Enable cloning only after tier selection and its license confirmation. Create one browser idempotency key per confirmation, keep it while in flight, route after 202, and poll only nonterminal jobs. Render server stage and redacted failure code; expose retry only when retryable. On success render Pack 已安装，Live Run 即将开放 and do not invent Replay metrics, Live Runs, credentials, or object links.

- [ ] **Step 4: Add MSW and browser coverage**

~~~ts
test("clone detail flow chooses quick pack and reaches setup", async ({ page }) => {
  await loginAsAdmin(page)
  await page.goto("/tutorials/text-rag")
  await page.getByRole("button", { name: "克隆教程" }).click()
  await page.getByRole("radio", { name: "Quick Pack" }).check()
  await page.getByRole("checkbox", { name: "我已确认数据许可" }).check()
  await page.getByRole("button", { name: "创建实验项目" }).click()
  await expect(page).toHaveURL(/\/projects\/prj_clone\/tutorial\/setup$/)
  await expect(page.getByText("Pack 已安装，Live Run 即将开放")).toBeVisible()
})
~~~

- [ ] **Step 5: Run Console verification**

Run: npm --prefix console run api:generate && npm --prefix console run typecheck && npm --prefix console run test -- --run && npm --prefix console run build && npm --prefix console run test:e2e

Expected: PASS while preserving the three catalog cards.

- [ ] **Step 6: Commit**

~~~bash
git add console/src/api/client.ts console/src/api/schema.ts console/src/features/tutorials console/src/test/handlers.ts console/e2e/tutorial-clone.spec.ts console/src/tutorials.css
git commit -m "feat(console): clone tutorial packs into projects"
~~~

## Task 6: Real backend evidence and honest release documentation

**Files:**

- Create: scripts/console-real-backend-tutorial-clone-e2e.sh
- Modify: .github/workflows/integration.yml
- Create: docs/tutorials/clone-and-pack-install.md
- Modify: docs/README.md
- Modify: ROADMAP.md
- Modify: ROADMAP_EN.md
- Modify: CHANGELOG.md

**Interfaces:**

- Consumes Tasks 1-5, PostgreSQL/Qdrant Compose infrastructure, and a controlled anonymous fixture origin.
- Produces a real browser lifecycle CI gate plus external OSS preflight instructions.

- [ ] **Step 1: Write deterministic real-backend test harness**

~~~bash
ORAG_REAL_BACKEND_E2E=1 \
ORAG_TUTORIAL_CATALOG_BASE_URL="http://127.0.0.1:fixture-port/tutorial-packs" \
ORAG_TUTORIAL_PRIVATE_OUTPUT_DIR="temporary-private-output" \
npm --prefix console run test:e2e -- tutorial-clone.spec.ts
~~~

The shell script starts temporary PostgreSQL, Qdrant, API, Console, and a read-only fixture server. It waits for readyz and cleans all temporary data. The fixture is only CI test infrastructure and cannot change production defaults.

- [ ] **Step 2: Implement CI and operations doc**

Add a console real backend tutorial clone E2E job with pinned Chromium and failure artifact uploads. Document:

~~~bash
curl --fail --location --max-time 30 \
  "$TUTORIAL_CATALOG_BASE_URL/text-rag/1.0.0/quick/manifest.json" \
  -o /tmp/orag-text-rag-manifest.json
sha256sum /tmp/orag-text-rag-manifest.json
~~~

The document must state that public-read OSS ACL and published objects are a production prerequisite. A 403 AccessDenied response remains a setup error; it is not a reason to add browser credentials or use the private writer as an input source.

- [ ] **Step 3: Execute complete validation**

~~~bash
go test ./...
go vet ./...
make openapi-validate
npm --prefix console run api:generate
npm --prefix console run typecheck
npm --prefix console run test -- --run
npm --prefix console run build
npm --prefix console run test:e2e
make console-real-e2e
make console-real-tutorial-clone-e2e
git diff --check
~~~

Expected: all local checks pass. Record the external public-OSS result separately: successful anonymous reads prove the deployment prerequisite; 403 keeps production availability honestly blocked while the local fixture proves application behavior.

- [ ] **Step 4: Update Roadmap without overclaiming**

Mark only catalog/manifest, secure clone, Pack installation, and real browser lifecycle complete. Keep text Live Run/P0-P8, visual execution, video execution, Replay display, result comparison, dataset builder, external pack availability, and external-adoption exit criteria pending.

- [ ] **Step 5: Commit**

~~~bash
git add scripts/console-real-backend-tutorial-clone-e2e.sh .github/workflows/integration.yml docs/tutorials/clone-and-pack-install.md docs/README.md ROADMAP.md ROADMAP_EN.md CHANGELOG.md
git commit -m "test(tutorial): verify clone installation lifecycle"
~~~

## Plan Self-Review

- Coverage: Tasks 1-6 cover phase-2 catalog binding, anonymous-reader/private-writer separation, durable idempotence/retry, tenant/project authorization, OpenAPI/Console state, and real browser evidence.
- Scope: phase-3 and later Run, Replay, result comparison, visual/video, and dataset-builder work are deliberately deferred and visibly unavailable.
- Type consistency: Manifest feeds CloneService; CloneJob is persisted then transported by Hertz/OpenAPI then consumed by the Console poller. Public reader and private writer never cross browser boundaries.

