# Console Real-Backend E2E Design

## Goal

Prove the Phase Four control-plane golden path in a browser against a real
PostgreSQL + Qdrant deployment: create a project and a frozen pipeline, run a
project evaluation, derive immutable environment evidence, activate and promote
the version, query production, inspect its trace lineage, and exercise a
validated rollback. The test must never require a real model key.

## Scope and non-goals

This change makes the existing Console paths usable with the server-derived
evaluation-evidence API and provides the missing explicit environment-binding
operation required by the existing release state machine. It does not relax a
gate, add a production mock mode, or claim that the evaluation runner executes
an arbitrary draft. The browser test uses the existing explicit deterministic
mock configuration only in an isolated E2E process.

The existing mocked Playwright component flows remain fast UI tests. The new
test is additive and is the release-control-plane integration gate.

## Chosen approach

Three options were considered:

1. Replace existing Playwright routes with a real API. This would leave no
   fast UI coverage and would not address the missing evidence controls.
2. Seed the release state through REST calls, then only inspect it in the
   browser. This would not prove the Console's user workflow.
3. Add a narrow evidence panel to Evaluation Center and drive the complete
   path through the Console while running real dependencies. This is the chosen
   approach because it directly verifies the roadmap's control-plane promise
   without exposing a client-controlled pass/fail field.

## Console behavior

`EvaluationCenter` gains an **immutable gate evidence** section. After an
evaluation finishes, the operator chooses one project-owned immutable pipeline
version, one previously created immutable policy, and the target environment.
Submitting calls:

```
POST /v1/projects/{projectID}/versions/{versionID}/evaluation-evidence
{
  "policy_id": "epol_…",
  "evaluation_run_id": "eval_…",
  "environment": "development|staging|production"
}
```

The UI never supplies metrics, a content hash, or `passed`. The API derives
those fields from its stored run and immutable policy. The page displays the
server response as a passed/failed evidence record and invalidates the release
version state. It reads policies and versions using existing authenticated
project APIs. Failures are visible as an operation error and retain the chosen
inputs so an operator can correct a missing policy, version, or run.

`ReleaseCenter` gains an **environment resource binding** section. An operator
chooses `development`, `staging`, or `production` and submits a non-empty opaque
binding reference. The server authorizes the project write, upserts the
reference in `project_environment_bindings`, and returns only the environment
state with `bound: true`; it never returns the reference or secret material.
This fills the current production-storage gap where new projects create
environment rows but no supported path can bind them, making every activation
and promotion fail safely with `release_binding_missing`.

The test will create a project through the Console, bind all three
environments through the Console, create a pipeline and freeze two immutable
versions through RAG Studio, create a dataset/sample, run an evaluation,
create a policy, record development/staging/production evidence for each
version as needed, activate and promote the newer version, issue a production
query through API Debugger, inspect a trace that contains
project/pipeline/version/release/evaluation lineage, then roll production back
to the earlier validated version.

The test creates the project-owned knowledge base through the existing API as
test fixture setup because the Console has no knowledge-base provisioning page.
This is explicit fixture preparation rather than an assertion about a Console
feature; all Phase Four control-plane operations are browser actions.

## E2E runtime and CI

`scripts/console-real-backend-e2e.sh` owns the local lifecycle. It starts the
existing test Compose PostgreSQL and Qdrant services, applies migrations,
starts `orag-api` with a dedicated port/database collections and the four mock
providers, waits for `/readyz`, runs Playwright with the real-backend project,
then always stops API and Compose services. It fails before running Playwright
if any production provider configuration is selected or the API readiness
check fails.

The Playwright configuration keeps unit-style E2E specs on the existing Vite
project and adds a `real-backend` project selected only by
`ORAG_REAL_BACKEND_E2E=1`. The test authenticates by using the normal login UI
with E2E-only administrator credentials, rather than injecting a forged token.
Vite's existing `/v1` proxy forwards browser requests to the dedicated API
port.

CI invokes the lifecycle script in a separate `console-real-backend-e2e` job,
installs a pinned Playwright Chromium browser, uploads Playwright reports and
API logs on failure, and leaves ordinary unit/build checks unchanged. The job
uses the same Compose file as PostgreSQL+Qdrant integration tests but an
independent database name, Qdrant collections, and ports so it can run in
parallel safely.

## Verification

Console unit tests assert policy/evidence request payloads, response display,
and error handling. The real browser spec asserts login, project creation,
immutable evidence, ordered development → staging → production promotion,
production query/trace lineage, and validated rollback. The lifecycle script
is exercised locally with real Compose dependencies. Existing Go, OpenAPI,
Console typecheck/build, PostgreSQL+Qdrant integration, and Playwright mocked
tests remain required.
