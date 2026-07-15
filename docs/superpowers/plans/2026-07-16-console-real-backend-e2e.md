# Console Real-Backend E2E Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the Console complete the server-enforced evaluation-to-production flow and prove it in Playwright against real PostgreSQL and Qdrant.

**Architecture:** The release service receives one explicit opaque environment-binding operation that persists a reference without returning it. Console Evaluation Center creates immutable policies and asks the server to derive evidence from a stored run; Release Center binds environments. A lifecycle shell script starts isolated dependencies, migrates the database, runs a deterministic-mock API, and then runs a real browser spec through Vite's proxy.

**Tech Stack:** Go 1.26, Hertz, PostgreSQL, Qdrant, OpenAPI 3, React 19, TypeScript, TanStack Query, Vitest, Playwright, Docker Compose, GitHub Actions.

## Global Constraints

- `binding_ref` is write-only: no HTTP response, Console state, log, or trace may expose it.
- The release service continues to reject missing bindings, missing evidence, invalid order, and stale expected active versions.
- Real-backend E2E sets all four model providers to `mock`, `ALLOW_DETERMINISTIC_MOCK=true`, and `REQUIRE_EXTERNAL_PROVIDERS=false`; it must not use model credentials.
- The E2E database, port, and Qdrant collections are distinct from `make test-integration` defaults.
- Existing route-mocked Playwright tests remain runnable without Docker or an API process.

---

### Task 1: Add a server-authorized write-only environment binding

**Files:**
- Modify: `internal/release/types.go`
- Modify: `internal/release/service.go`
- Modify: `internal/release/memory.go`
- Modify: `internal/storage/postgres/release.go`
- Modify: `internal/http/router.go`
- Modify: `internal/http/releases.go`
- Modify: `internal/release/service_test.go`
- Modify: `internal/storage/postgres/release_version_test.go`
- Modify: `internal/http/router_test.go`
- Modify: `api/openapi.yaml`
- Modify: `console/src/api/schema.d.ts`

**Interfaces:**
- Consumes: `release.Repository`, `release.Service`, existing `project_environment_bindings` table.
- Produces: `release.Service.Bind(ctx, projectID, environment, bindingRef) (Environment, error)` and `PUT /v1/projects/{project_id}/environments/{environment}/binding`.

- [ ] **Step 1: Write failing release service tests**

Add tests that verify an empty reference is invalid, `Bind` changes only the chosen environment to `Bound: true`, and later activation uses that state while another unbound environment remains rejected.

```go
func TestServiceBindRequiresReferenceAndMarksOnlyOneEnvironment(t *testing.T) {
	repo := newMemoryRepository()
	svc := NewService(repo)
	_, err := svc.Bind(context.Background(), "p1", Staging, " ")
	if !errors.Is(err, ErrBindingInvalid) { t.Fatalf("Bind() error = %v", err) }
	bound, err := svc.Bind(context.Background(), "p1", Staging, "deployment://staging")
	if err != nil || !bound.Bound || bound.Kind != Staging { t.Fatalf("Bind() = %#v, %v", bound, err) }
	development, _ := svc.Environment(context.Background(), "p1", Development)
	if development.Bound { t.Fatal("development binding unexpectedly changed") }
}
```

- [ ] **Step 2: Run the focused test and confirm the missing interface**

Run: `go test ./internal/release -run TestServiceBindRequiresReferenceAndMarksOnlyOneEnvironment -count=1`

Expected: compilation failure because `Bind` and `ErrBindingInvalid` do not exist.

- [ ] **Step 3: Implement the release and PostgreSQL contracts**

Extend the repository and service with the smallest explicit operation. Validate fixed kinds and a trimmed reference before storage; store the reference only in the repository.

```go
var ErrBindingInvalid = errors.New("invalid release environment binding")

type Repository interface {
	// existing methods ...
	Bind(context.Context, string, EnvironmentKind, string) error
}

func (s *Service) Bind(ctx context.Context, projectID string, kind EnvironmentKind, bindingRef string) (Environment, error) {
	if projectID == "" || (kind != Development && kind != Staging && kind != Production) || strings.TrimSpace(bindingRef) == "" {
		return Environment{}, ErrBindingInvalid
	}
	if err := s.repo.Bind(ctx, projectID, kind, strings.TrimSpace(bindingRef)); err != nil { return Environment{}, err }
	return s.repo.Environment(ctx, projectID, kind)
}
```

Implement PostgreSQL persistence with a parameterized upsert and no select of `binding_ref`:

```sql
INSERT INTO project_environment_bindings(project_id, environment_kind, binding_ref)
VALUES($1,$2,$3)
ON CONFLICT (project_id, environment_kind)
DO UPDATE SET binding_ref=EXCLUDED.binding_ref, created_at=now()
```

The memory repository stores only whether the binding exists, not the reference. Update all test fake repositories to implement `Bind`.

- [ ] **Step 4: Add the authenticated HTTP endpoint and contract**

Register and implement this route:

```go
v1.PUT("/projects/:project_id/environments/:environment/binding", s.bindReleaseEnvironment)

func (s *Server) bindReleaseEnvironment(ctx context.Context, c *app.RequestContext) {
	projectID, principal, ok := releaseProjectRequest(c)
	if !ok || !authorizeRequest(c, auth.ActionResourceWrite, principal.TenantID, projectID) { return }
	var req struct { BindingRef string `json:"binding_ref"` }
	if !bindJSON(c, &req) { return }
	item, err := s.App.Release.Bind(ctx, projectID, release.EnvironmentKind(c.Param("environment")), req.BindingRef)
	if err != nil { writeReleaseError(c, err); return }
	c.JSON(consts.StatusOK, item)
}
```

Map `ErrBindingInvalid` to `400 invalid_release_request`. Add OpenAPI request/response definitions where the request contains `binding_ref` but `Environment` remains `bound`-only. Regenerate the Console schema with `npm --prefix console run api:generate`.

- [ ] **Step 5: Run focused backend and contract verification**

Run:

```bash
go test ./internal/release ./internal/storage/postgres ./internal/http -run 'Bind|binding' -count=1
make openapi-validate
npm --prefix console run api:generate
git diff --check -- console/src/api/schema.d.ts
```

Expected: all tests pass and the generated schema contains the new binding
operation without whitespace errors.

- [ ] **Step 6: Commit the server binding slice**

```bash
git add internal/release internal/storage/postgres/release.go internal/http api/openapi.yaml console/src/api/schema.d.ts
git commit -m "feat(release): bind project environments explicitly"
```

### Task 2: Expose immutable policy/evidence and binding operations in Console

**Files:**
- Modify: `console/src/api/client.ts`
- Modify: `console/src/features/evaluation/evaluation-center.tsx`
- Modify: `console/src/features/evaluation/evaluation-center.test.tsx`
- Modify: `console/src/features/releases/release-center.tsx`
- Modify: `console/src/features/releases/release-center.test.tsx`
- Modify: `console/src/test/handlers.ts`

**Interfaces:**
- Consumes: generated `EvaluationPolicy`, `EvaluationEvidence`, `Environment`, and `PipelineVersion` schemas.
- Produces: `evaluationPolicyApi.list/create/recordEvidence`, `releaseApi.bindEnvironment`, and browser-visible gate/binding forms that never post metrics, hashes, or a passed flag.

- [ ] **Step 1: Write Console tests for payloads and safe rendering**

Add tests that create a policy and evidence through the page, inspect the exact request body, show the server-derived pass result, and never render `binding_ref` after a successful bind.

```ts
expect(await screen.findByText('Evidence passed · development')).toBeVisible()
expect(requestBody).toEqual({ policy_id: 'epol_a', evaluation_run_id: 'eval_a', environment: 'development' })
expect(screen.queryByText('deployment://production-secret')).not.toBeInTheDocument()
```

- [ ] **Step 2: Run focused tests and confirm they fail before implementation**

Run: `npm --prefix console run test -- --run src/features/evaluation/evaluation-center.test.tsx src/features/releases/release-center.test.tsx`

Expected: failure because the evidence and binding controls do not exist.

- [ ] **Step 3: Add strongly typed client operations**

Use schema aliases rather than handwritten response shapes:

```ts
export type EvaluationPolicy = components['schemas']['EvaluationPolicy']
export type EvaluationEvidence = components['schemas']['EvaluationEvidence']
export type CreateEvaluationPolicyInput = components['schemas']['CreateEvaluationPolicyRequest']

export const evaluationPolicyApi = {
  list: (projectId: string) => request<{ items: EvaluationPolicy[] }>(`/v1/projects/${encodeURIComponent(projectId)}/evaluation-policies`),
  create: (projectId: string, input: CreateEvaluationPolicyInput) => request<EvaluationPolicy>(`/v1/projects/${encodeURIComponent(projectId)}/evaluation-policies`, { method: 'POST', body: JSON.stringify(input) }),
  recordEvidence: (projectId: string, versionId: string, input: components['schemas']['RecordEvaluationEvidenceRequest']) => request<EvaluationEvidence>(`/v1/projects/${encodeURIComponent(projectId)}/versions/${encodeURIComponent(versionId)}/evaluation-evidence`, { method: 'POST', body: JSON.stringify(input) }),
}

export const releaseApi = {
  // existing methods ...
  bindEnvironment: (projectId: string, environment: EnvironmentKind, bindingRef: string) => request<Environment>(`/v1/projects/${encodeURIComponent(projectId)}/environments/${environment}/binding`, { method: 'PUT', body: JSON.stringify({ binding_ref: bindingRef }) }),
}
```

- [ ] **Step 4: Build the two narrow Console controls**

In `EvaluationCenter`, query project policies and versions only when `projectId` exists. Add a policy form with dataset selector, immutable name, metric `answer_accuracy`, comparator `gte`, and threshold `0`; it creates the policy after the data set exists. Add a derived-evidence form requiring a completed run, selected policy, selected frozen version, and selected target environment. On success, display `Evidence passed · {environment}` or `Evidence failed · {environment}` and invalidate release-version/environments queries.

In `ReleaseCenter`, add a separate binding form above lifecycle actions. It asks for the environment and a reference, calls `bindEnvironment`, clears the reference on success, and invalidates environments. Do not put the saved reference in React state after success, a query cache, or rendered history.

- [ ] **Step 5: Update MSW fixtures and make all Console tests pass**

Add defaults for policy list, versions, and environment bind responses in `console/src/test/handlers.ts`; route-specific tests override them. Run:

```bash
npm --prefix console run test -- --run src/features/evaluation/evaluation-center.test.tsx src/features/releases/release-center.test.tsx
npm --prefix console run typecheck
npm --prefix console run build
```

Expected: all tests, TypeScript, and production build pass.

- [ ] **Step 6: Commit the Console golden-path controls**

```bash
git add console/src/api/client.ts console/src/features/evaluation console/src/features/releases console/src/test/handlers.ts
git commit -m "feat(console): derive release evidence and bind environments"
```

### Task 3: Add an isolated real PostgreSQL + Qdrant browser lifecycle

**Files:**
- Modify: `console/vite.config.ts`
- Modify: `console/playwright.config.ts`
- Create: `console/e2e/real-backend-release.spec.ts`
- Create: `scripts/console-real-backend-e2e.sh`
- Modify: `Makefile`

**Interfaces:**
- Consumes: the normal login endpoint, Console controls from Task 2, test Compose services, `oragctl migrate`, and `/readyz`.
- Produces: `make console-real-e2e`, an API log at `.tmp/console-real-e2e/api.log`, and a real browser test selected by `ORAG_REAL_BACKEND_E2E=1`.

- [ ] **Step 1: Write the real browser test before the lifecycle runner**

The test must use normal Console login, not `authenticateConsole`. It may use
one `page.evaluate` fixture request to create the project-owned knowledge base
because no knowledge-base Console page exists; it reads the normal Console
session token from `sessionStorage` and calls the Vite-proxied API. Every Phase
Four control-plane transition is a page interaction.

```ts
test('releases and rolls back through real PostgreSQL and Qdrant', async ({ page }) => {
  await page.goto('/login')
  await page.getByLabel('用户名').fill('e2e-admin')
  await page.getByLabel('密码').fill('e2e-password')
  await page.getByRole('button', { name: '登录' }).click()
  await page.getByRole('link', { name: '新建项目' }).click()
  await page.getByLabel('项目名称').fill('Browser production control plane')
  await page.getByRole('button', { name: '创建项目' }).click()
  const projectID = /\/projects\/([^/]+)\/overview/.exec(page.url())?.[1]
  if (!projectID) throw new Error(`missing project id in ${page.url()}`)
  await page.evaluate(async (id) => {
    const raw = sessionStorage.getItem('orag.console.session.v1')
    const token = raw ? JSON.parse(raw).accessToken : ''
    const response = await fetch('/v1/knowledge-bases', { method: 'POST', headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' }, body: JSON.stringify({ name: 'E2E knowledge base', project_id: id }) })
    if (!response.ok) throw new Error(`knowledge base fixture failed: ${response.status}`)
  }, projectID)
  // Bind all environments, freeze versions, derive evidence, release, query,
  // inspect lineage, and roll back using labeled Console controls.
})
```

It must assert a production query trace contains non-empty `project_id`, `pipeline_id`, `pipeline_version_id`, `release_id`, `dataset_id`, `evaluation_run_id`, and `environment: production`; then assert the release card shows the older version after rollback.

- [ ] **Step 2: Run the spec and confirm it is skipped outside the real runner**

Guard it with `test.skip(process.env.ORAG_REAL_BACKEND_E2E !== '1', 'requires real backend lifecycle')`. Run: `npm --prefix console run test:e2e -- --list`

Expected: existing mocked specs are listed and the real-backend spec is marked skipped rather than attempting to connect to localhost API.

- [ ] **Step 3: Make the Vite proxy endpoint configurable**

Replace the literal proxy target with a safe local default:

```ts
const apiTarget = process.env.ORAG_CONSOLE_API_TARGET ?? 'http://127.0.0.1:8080'
export default defineConfig({ plugins: [react()], server: { proxy: { '/v1': apiTarget } } })
```

Keep Playwright's web server on `4173`; the runner sets `ORAG_CONSOLE_API_TARGET=http://127.0.0.1:18080`.

- [ ] **Step 4: Implement the cleanup-safe lifecycle script**

Create a POSIX shell script that uses a unique Compose project name, `set -eu`, and a trap:

```sh
cleanup() {
  test -n "${api_pid:-}" && kill "$api_pid" 2>/dev/null || true
  docker compose -p orag-console-e2e -f deployments/docker-compose.test.yml down -v || true
}
trap cleanup EXIT INT TERM
docker compose -p orag-console-e2e -f deployments/docker-compose.test.yml up -d --wait
docker compose -p orag-console-e2e -f deployments/docker-compose.test.yml exec -T postgres createdb -U orag orag_console_e2e
mkdir -p .tmp/console-real-e2e
env ALLOW_DETERMINISTIC_MOCK=true REQUIRE_EXTERNAL_PROVIDERS=false LLM_CHAT_PROVIDER=mock LLM_EMBEDDING_PROVIDER=mock LLM_RERANK_PROVIDER=mock LLM_MULTIMODAL_PROVIDER=mock \
  DATABASE_URL='postgres://orag:orag@127.0.0.1:55432/orag_console_e2e?sslmode=disable' \
  QDRANT_HOST=127.0.0.1 QDRANT_GRPC_PORT=6634 \
  CGO_ENABLED=0 GOFLAGS='-tags=stdjson,gjson' go run ./cmd/oragctl migrate
env PORT=18080 STORAGE_BACKEND=qdrant_postgres DATABASE_URL='postgres://orag:orag@127.0.0.1:55432/orag_console_e2e?sslmode=disable' \
  QDRANT_HOST=127.0.0.1 QDRANT_GRPC_PORT=6634 QDRANT_COLLECTION=orag_console_e2e_chunks QDRANT_SEMANTIC_CACHE_COLLECTION=orag_console_e2e_cache \
  ALLOW_DETERMINISTIC_MOCK=true REQUIRE_EXTERNAL_PROVIDERS=false LLM_CHAT_PROVIDER=mock LLM_EMBEDDING_PROVIDER=mock LLM_RERANK_PROVIDER=mock LLM_MULTIMODAL_PROVIDER=mock \
  ADMIN_DEFAULT_USERNAME=e2e-admin ADMIN_DEFAULT_PASSWORD=e2e-password JWT_SECRET=console-e2e-jwt \
  CGO_ENABLED=0 GOFLAGS='-tags=stdjson,gjson' go run ./cmd/orag-api >.tmp/console-real-e2e/api.log 2>&1 &
api_pid=$!
./scripts/wait-ready.sh http://127.0.0.1:18080/readyz
ORAG_REAL_BACKEND_E2E=1 ORAG_CONSOLE_API_TARGET=http://127.0.0.1:18080 npm --prefix console run test:e2e -- e2e/real-backend-release.spec.ts
```

Create the database first with `psql` in the Compose PostgreSQL container; do not reuse the integration test database. Add `console-real-e2e: ./scripts/console-real-backend-e2e.sh` to the Makefile and include it in `.PHONY`.

- [ ] **Step 5: Run the lifecycle locally and inspect real artifacts**

Run: `make console-real-e2e`

Expected: one Playwright browser test passes, all temporary containers/volumes are removed, and `.tmp/console-real-e2e/api.log` confirms only mock providers were selected.

- [ ] **Step 6: Commit the real browser E2E slice**

```bash
git add console/vite.config.ts console/playwright.config.ts console/e2e/real-backend-release.spec.ts scripts/console-real-backend-e2e.sh Makefile
git commit -m "test(console): cover real backend release lifecycle"
```

### Task 4: Make the real browser flow a CI gate and record roadmap evidence

**Files:**
- Modify: `.github/workflows/integration.yml`
- Modify: `ROADMAP.md`
- Modify: `ROADMAP_EN.md`

**Interfaces:**
- Consumes: `make console-real-e2e` and Playwright output directories.
- Produces: a `console real backend e2e` workflow check and an accurate Phase Four progress statement.

- [ ] **Step 1: Add a failing workflow-job assertion locally**

Use `actionlint` if installed; otherwise inspect rendered YAML with Ruby's standard YAML parser and confirm `console-real-backend-e2e` has checkout, Go, Node, browser install, the make target, and failure artifacts.

```bash
ruby -e 'require "yaml"; YAML.load_file(".github/workflows/integration.yml"); puts "workflow yaml ok"'
```

Expected before the edit: the named job is absent.

- [ ] **Step 2: Add the independently runnable CI job**

Use the repository's pinned actions and Node 22. Install dependencies and Chromium before running the lifecycle:

```yaml
  console-real-backend-e2e:
    name: console real backend e2e
    runs-on: ubuntu-latest
    timeout-minutes: 25
    steps:
      - uses: actions/checkout@9c091bb21b7c1c1d1991bb908d89e4e9dddfe3e0 # v7.0.0
      - uses: actions/setup-go@924ae3a1cded613372ab5595356fb5720e22ba16 # v6.5.0
        with: {go-version: "1.26.5"}
      - uses: actions/setup-node@820762786026740c76f36085b0efc47a31fe5020 # v7.0.0
        with: {node-version: "22", cache: npm, cache-dependency-path: console/package-lock.json}
      - run: go mod download
      - run: npm --prefix console ci
      - run: npm --prefix console exec playwright install --with-deps chromium
      - run: make console-real-e2e
      - if: failure()
        uses: actions/upload-artifact@043fb46d1a93c77aae656e7c1c64a875d1fc6a0a # v7.0.1
        with:
          name: console-real-backend-e2e-artifacts
          path: |
            console/playwright-report/
            console/test-results/
            .tmp/console-real-e2e/
```

- [ ] **Step 3: Update roadmap progress without claiming broader completion**

Replace only the Phase Four “still pending real browser E2E” clause with evidence that the CI-backed real PostgreSQL+Qdrant browser path now covers environment binding, immutable evidence, ordered promotion, production lineage, and rollback. Leave tutorial Replay/results comparison as pending.

- [ ] **Step 4: Run the full validation bundle**

Run:

```bash
go test ./...
go vet ./...
make openapi-validate
npm --prefix console run api:generate
npm --prefix console run typecheck
npm --prefix console run test -- --run
npm --prefix console run build
npm --prefix console run test:e2e
make console-real-e2e
docker compose -f deployments/docker-compose.test.yml down -v
```

Expected: all commands pass; only the explicit real-backend spec runs against Docker, while existing E2E specs retain their mocked routes.

- [ ] **Step 5: Commit documentation and CI**

```bash
git add .github/workflows/integration.yml ROADMAP.md ROADMAP_EN.md
git commit -m "ci: gate console release lifecycle against real storage"
```

### Task 5: Review, publish, and merge the validated slice

**Files:**
- Verify: all changed files from Tasks 1-4

**Interfaces:**
- Consumes: branch `codex/console-real-backend-e2e`, local validation output, GitHub Actions.
- Produces: a draft PR linked to the Phase Four work, followed by squash merge only after every required remote check passes.

- [ ] **Step 1: Inspect scope and secrets before staging**

Run:

```bash
git diff origin/main...HEAD --check
git diff --name-only origin/main...HEAD
git grep -nE 'e2e-password|console-e2e-jwt' -- ':!docs/superpowers/plans' ':!docs/superpowers/specs'
```

Expected: test-only literals occur only in the lifecycle script/test; no real credential, server IP, or private key appears.

- [ ] **Step 2: Push and open a draft PR**

```bash
git push -u origin codex/console-real-backend-e2e
gh pr create --draft --base main --head codex/console-real-backend-e2e \
  --title 'feat(console): gate release lifecycle against real storage' \
  --body 'Implements explicit environment bindings, server-derived evidence controls, and a real PostgreSQL+Qdrant Console browser gate.'
```

- [ ] **Step 3: Verify remote checks and merge**

Run `gh pr checks <number> --watch`; investigate any failure before proceeding. After all checks pass:

```bash
gh pr ready <number>
gh pr merge <number> --squash --delete-branch
git -C /Users/bytedance/Documents/orag pull --ff-only origin main
```

Expected: the merged commit is the local and remote `main` HEAD; remove the temporary worktree only after that proof.
