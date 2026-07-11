# ORAG Console Project Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add tenant-scoped Projects and a runnable `console/` application shell that can create, list, select, and edit projects.

**Architecture:** A new `internal/project` service owns validation and authorization over a repository interface. PostgreSQL persists projects and fixed development/staging/production environments. The React shell consumes generated OpenAPI types and keys every route and query by project ID.

**Tech Stack:** Go, Hertz, PostgreSQL, OpenAPI 3, React, TypeScript, Vite, TanStack Router, TanStack Query, Zod, Vitest, MSW, Playwright.

## Global Constraints

- Project records are always tenant-scoped.
- Project environment bindings never return secret values.
- Existing API routes and authentication remain compatible.
- Use `GOTOOLCHAIN=go1.26.4` for direct Go commands outside Make targets.

---

### Task 1: Project domain and in-memory service contract

**Files:**
- Create: `internal/project/types.go`
- Create: `internal/project/service.go`
- Create: `internal/project/service_test.go`

**Interfaces:**
- Produces: `project.Project`, `project.Environment`, `project.Repository`, `project.Service.Create`, `project.Service.List`, `project.Service.Get`, and `project.Service.Update`.

- [ ] **Step 1: Write failing service tests**

```go
func TestServiceCreateInitializesThreeEnvironments(t *testing.T) {
    repo := newMemoryRepository()
    svc := project.NewService(repo, fixedClock)
    got, err := svc.Create(context.Background(), "tenant_a", project.CreateInput{Name: "Support"})
    require.NoError(t, err)
    require.Equal(t, "Support", got.Name)
    require.Equal(t, []project.EnvironmentKind{"development", "staging", "production"}, repo.environmentKinds(got.ID))
}

func TestServiceGetRejectsForeignTenant(t *testing.T) {
    repo := seededMemoryRepository("tenant_a")
    _, err := project.NewService(repo, fixedClock).Get(context.Background(), "tenant_b", "prj_1")
    require.ErrorIs(t, err, project.ErrNotFound)
}
```

- [ ] **Step 2: Run the focused test and verify RED**

Run: `GOTOOLCHAIN=go1.26.4 GOFLAGS='-tags=stdjson,gjson' go test ./internal/project -run 'TestService(Create|Get)' -v`

Expected: FAIL because `internal/project` does not exist.

- [ ] **Step 3: Implement the minimal domain**

```go
type EnvironmentKind string
const (
    EnvironmentDevelopment EnvironmentKind = "development"
    EnvironmentStaging EnvironmentKind = "staging"
    EnvironmentProduction EnvironmentKind = "production"
)

type Project struct {
    ID, TenantID, Name, Description string
    CreatedAt, UpdatedAt time.Time
}

type Repository interface {
    CreateWithEnvironments(context.Context, Project, []Environment) error
    List(context.Context, string) ([]Project, error)
    Get(context.Context, string, string) (Project, bool, error)
    Update(context.Context, Project) error
}
```

`Service.Create` trims and validates the name, generates `id.New("prj")`, and creates exactly three environment records in one repository call. `Get` maps a missing or foreign-tenant row to `ErrNotFound`.

- [ ] **Step 4: Run project tests and verify GREEN**

Run: `GOTOOLCHAIN=go1.26.4 GOFLAGS='-tags=stdjson,gjson' go test ./internal/project -v`

Expected: PASS.

- [ ] **Step 5: Commit the domain**

```bash
git add internal/project
git commit -m "feat: add project domain service"
```

### Task 2: PostgreSQL project persistence and migration

**Files:**
- Create: `migrations/000016_projects.sql`
- Create: `internal/storage/postgres/project.go`
- Create: `internal/storage/postgres/project_test.go`
- Modify: `internal/storage/postgres/repository.go`

**Interfaces:**
- Consumes: `project.Repository` from Task 1.
- Produces: compile-time assertion `var _ project.Repository = (*ProjectRepository)(nil)` and `postgres.NewProjectRepository`.

- [ ] **Step 1: Add failing migration and tenant-isolation tests**

```go
func TestProjectMigrationDefinesTenantScopedTables(t *testing.T) {
    sql := readMigration(t, "../../../migrations/000016_projects.sql")
    for _, fragment := range []string{"CREATE TABLE IF NOT EXISTS projects", "tenant_id TEXT NOT NULL REFERENCES tenants(id)", "CREATE TABLE IF NOT EXISTS project_environments", "UNIQUE (project_id, kind)"} {
        require.Contains(t, sql, fragment)
    }
}
```

- [ ] **Step 2: Verify RED**

Run: `GOTOOLCHAIN=go1.26.4 GOFLAGS='-tags=stdjson,gjson' go test ./internal/storage/postgres -run TestProject -v`

Expected: FAIL because the migration and repository are missing.

- [ ] **Step 3: Implement transaction-backed persistence**

The migration creates `projects` and `project_environments`, indexes `(tenant_id, updated_at DESC)`, and cascades environment deletion when a project is deleted. `CreateWithEnvironments` uses `pgx.BeginTx`, inserts the project, inserts all three environments, and commits only after every insert succeeds.

- [ ] **Step 4: Verify GREEN**

Run: `GOTOOLCHAIN=go1.26.4 GOFLAGS='-tags=stdjson,gjson' go test ./internal/storage/postgres -run TestProject -v`

Expected: PASS.

- [ ] **Step 5: Commit persistence**

```bash
git add migrations/000016_projects.sql internal/storage/postgres/project.go internal/storage/postgres/project_test.go internal/storage/postgres/repository.go
git commit -m "feat: persist project control plane"
```

### Task 3: Project HTTP and OpenAPI contract

**Files:**
- Create: `internal/http/projects.go`
- Modify: `internal/http/router.go`
- Modify: `internal/http/router_test.go`
- Modify: `internal/app/app.go`
- Modify: `internal/app/app_test.go`
- Modify: `api/openapi.yaml`
- Modify: `tests/contract/openapi_test.go`

**Interfaces:**
- Consumes: `*project.Service`.
- Produces: `GET/POST /v1/projects` and `GET/PATCH /v1/projects/{project_id}`.

- [ ] **Step 1: Add failing HTTP and OpenAPI tests**

```go
func TestProjectsAreTenantScoped(t *testing.T) {
    h := testServerWithProjects(t)
    created := postJSON(t, h, "/v1/projects", `{"name":"Support"}`, tokenFor("tenant_a"))
    require.Equal(t, 201, created.StatusCode)
    foreign := get(t, h, "/v1/projects/"+created.JSONString("id"), tokenFor("tenant_b"))
    require.Equal(t, 404, foreign.StatusCode)
}
```

- [ ] **Step 2: Verify RED**

Run: `GOTOOLCHAIN=go1.26.4 GOFLAGS='-tags=stdjson,gjson' go test ./internal/http ./tests/contract -run 'TestProjects|TestOpenAPI' -v`

Expected: FAIL because project routes and schemas are absent.

- [ ] **Step 3: Wire service, handlers, routes, and schemas**

Register the four routes under the authenticated `/v1` group. Return `invalid_request`, `project_not_found`, and `project_conflict` through the existing error envelope. Add `Project`, `CreateProjectRequest`, `UpdateProjectRequest`, and `ProjectListResponse` schemas to OpenAPI.

- [ ] **Step 4: Verify GREEN**

Run: `make openapi-validate && GOTOOLCHAIN=go1.26.4 GOFLAGS='-tags=stdjson,gjson' go test ./internal/project ./internal/http ./internal/app -v`

Expected: PASS.

- [ ] **Step 5: Commit API contract**

```bash
git add internal/http/projects.go internal/http/router.go internal/http/router_test.go internal/app/app.go internal/app/app_test.go api/openapi.yaml tests/contract/openapi_test.go
git commit -m "feat: expose project control plane api"
```

### Task 4: Runnable console shell and project switcher

**Files:**
- Create: `console/package.json`, `console/vite.config.ts`, `console/tsconfig.json`, `console/index.html`
- Create: `console/src/main.tsx`, `console/src/app/router.tsx`, `console/src/app/providers.tsx`
- Create: `console/src/api/client.ts`, `console/src/api/schema.d.ts`
- Create: `console/src/features/projects/project-list.tsx`, `console/src/features/projects/project-switcher.tsx`, `console/src/features/projects/project-form.tsx`
- Create: `console/src/test/setup.ts`, `console/src/test/handlers.ts`, `console/src/features/projects/project-switcher.test.tsx`
- Create: `console/e2e/projects.spec.ts`, `console/playwright.config.ts`
- Modify: `Makefile`

**Interfaces:**
- Consumes: project OpenAPI endpoints from Task 3.
- Produces: routes `/projects`, `/projects/new`, and `/projects/:projectId/overview`.

- [ ] **Step 1: Write failing project-switcher test**

```tsx
it('switches project route without leaking the previous project cache', async () => {
  renderApp('/projects/prj_a/overview')
  await userEvent.click(await screen.findByRole('button', { name: /Support/ }))
  await userEvent.click(screen.getByRole('option', { name: /Search/ }))
  expect(router.state.location.pathname).toBe('/projects/prj_b/overview')
  expect(queryClient.getQueryData(['projects', 'prj_a'])).toBeDefined()
})
```

- [ ] **Step 2: Verify RED after installing locked dependencies**

Run: `npm --prefix console install && npm --prefix console test -- --run project-switcher`

Expected: FAIL because the shell and component are missing.

- [ ] **Step 3: Implement shell, providers, and generated client script**

Add scripts `dev`, `build`, `typecheck`, `test`, `test:e2e`, and `api:generate`. Query keys must start with `['projects', projectId]` for project resources. The switcher navigates before rendering project content, and route loaders reject missing project IDs.

- [ ] **Step 4: Verify frontend and browser path**

Run: `npm --prefix console run typecheck && npm --prefix console test -- --run && npm --prefix console run build && npm --prefix console run test:e2e -- projects.spec.ts`

Expected: all commands exit 0.

- [ ] **Step 5: Commit the runnable foundation**

```bash
git add console Makefile
git commit -m "feat: add ORAG console project shell"
```
