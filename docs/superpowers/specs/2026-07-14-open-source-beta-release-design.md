# ORAG Open-Source Beta Release Design

## 1. Summary

ORAG will ship `v0.1.0-beta.1` as a Go-native RAG service and embeddable control-plane SDK with an evaluation-first product position. The release program is split into four independently reviewable pull requests followed by one guarded release action:

1. community governance, repository policy, and capability maturity;
2. a public embeddable Go SDK and shared application boundary;
3. reproducible multi-architecture artifacts and a no-key full-stack walkthrough;
4. interactive and hosted documentation with release screenshots and a walkthrough GIF;
5. the `v0.1.0-beta.1` tag only after every release gate passes on `main`.

The program does not attach calendar dates to phases. Quality gates, reproducibility, and public usability determine when the Beta is released.

## 2. Goals

- Make the repository meet the baseline community and security expectations of a well-run public open-source project.
- Express capability maturity consistently as `experimental`, `beta`, or `stable` in the capability manifest, OpenAPI, generated artifacts, and public documentation.
- Publish a real root-module Go SDK that downstream modules can import without referencing `internal/*`.
- Keep HTTP and SDK behavior aligned by sharing the same application services and dependency assembly.
- Publish reproducible `linux/amd64` and `linux/arm64` API and Console images to GHCR.
- Start migration, API, Console, and demo initialization with one Compose command.
- Let a new user complete an ingestion, cited query, trace, and evaluation walkthrough without a real model API key.
- Replace the placeholder `/docs` page with an interactive OpenAPI UI.
- Publish versioned hosted documentation with real screenshots and a walkthrough GIF.
- Create `v0.1.0-beta.1` only from a verified `main` commit and make all artifacts traceable to that commit.

## 3. Non-Goals

- Declaring any capability `stable` before `v1.0.0`.
- Guaranteeing long-term compatibility for experimental APIs.
- Building a hosted SaaS control plane, Helm chart, Kubernetes operator, or cloud marketplace image.
- Adding arbitrary model providers merely to increase the integration count.
- Treating the deterministic mock as a production provider.
- Publishing a thin HTTP client and calling it the public Go SDK.
- Releasing from an unmerged feature branch or manually uploading untraceable binaries.

## 4. Current-State Constraints

The current repository has one general CI workflow, a single API Dockerfile, and a Compose stack for PostgreSQL, Qdrant, and the API. It does not yet contain community health files, issue forms, a pull-request template, Dependabot configuration, a release workflow, a Console image, or a hosted docs workflow. GitHub Discussions and Private Vulnerability Reporting are disabled, repository Topics are empty, and `main` is not protected.

`GET /docs` currently returns a minimal hard-coded HTML page. The module root has no importable Go package; application assembly lives in `internal/app`. A deterministic model implementation exists only as test infrastructure and is not packaged as a runnable demo service.

These constraints require explicit shared boundaries instead of duplicating service behavior in release scripts, docs, and the SDK.

## 5. Delivery and Pull-Request Boundaries

### PR 1: Governance and maturity

This pull request contains repository-controlled policy and metadata:

- `CONTRIBUTING.md`, `SECURITY.md`, `CODE_OF_CONDUCT.md`, and `CHANGELOG.md`;
- GitHub Bug, Feature, Documentation, and RFC issue forms;
- a pull-request template with test, documentation, security, compatibility, and maturity checklists;
- Dependabot updates for Go modules, npm, GitHub Actions, and Docker;
- the date-free phased roadmap update;
- capability and OpenAPI maturity fields plus drift tests;
- release, compatibility, deprecation, and maturity documentation.

After this PR is merged and required check names are known, repository settings are changed through the GitHub API: Discussions, Topics, Private Vulnerability Reporting, delete-branch-on-merge, and `main` protection. Branch protection blocks force pushes and deletion, requires up-to-date branches, required CI checks, and resolved conversations. It does not require an approving reviewer while there is only one maintainer.

### PR 2: Public Go SDK

This pull request introduces the root `github.com/shikanon/orag` package and the minimum internal refactor required to share application services. It must not change protocol behavior merely to simplify the SDK.

### PR 3: Release artifacts and mock walkthrough

This pull request introduces version metadata, API and Console container targets, the migration and demo jobs, the full Compose topology, smoke tests, and the tag-driven release workflow.

### PR 4: Interactive and hosted docs

This pull request replaces `/docs`, adds the hosted documentation site and deployment workflow, records real screenshots and the walkthrough GIF, and updates onboarding and SDK guides against the release candidate.

### Release action

The tag is not part of a pull request. After all four PRs are merged, the release checklist is run against the exact `main` SHA. An annotated `v0.1.0-beta.1` tag is pushed only when the gate passes. The workflow creates artifacts; maintainers do not hand-build or manually substitute them.

## 6. Capability Maturity Model

The canonical enum is:

- `experimental`: usable for exploration, but contracts or behavior may change without a migration path;
- `beta`: supported for evaluation and pilot use, with documented limitations and best-effort migration guidance;
- `stable`: covered by the stable compatibility policy. No capability receives this value before `v1.0.0`.

Every public capability manifest entry has a required maturity value. Every public OpenAPI operation has `x-orag-maturity`; schemas that represent standalone public concepts may also carry it. Generated MCP and Skill facets include the manifest value. README capability tables are generated from, or contract-tested against, the manifest rather than maintained as an unrelated list.

Validation rejects unknown or missing maturity values. Contract tests also reject any `stable` value while the repository version is below `v1.0.0`.

## 7. Public Go SDK Design

### Package boundary

The module root becomes package `orag`. Public signatures use root-package DTOs and interfaces only. No exported field, parameter, return type, error, or example may require an `internal/*` import.

The primary lifecycle is:

```go
client, err := orag.New(ctx, orag.Config{...})
if err != nil { ... }
defer client.Close()
```

An explicit `orag.Config` is the primary SDK configuration. `orag.NewFromEnv` is provided for service-style embedding but is not the only constructor. Configuration defaults are deterministic and are validated before external resources are opened.

### Beta surface

The first Beta exposes:

- client creation, readiness, and idempotent close;
- knowledge-base create, list, get, and delete;
- text and file ingestion plus ingestion status;
- synchronous and streaming query;
- dataset/evaluation submission and status;
- trace lookup;
- a deterministic in-memory/mock configuration for examples and tests;
- PostgreSQL + Qdrant configuration for real deployments.

Public operations use typed request and response DTOs. Streaming returns a typed event stream with cancellation and terminal error semantics rather than exposing the HTTP SSE representation.

### Shared application assembly

`internal/app` remains an internal composition root, but construction accepts explicit internal dependency options. The SDK maps public configuration and public provider interfaces into those options. `cmd/orag-api` uses the same constructor and then attaches the HTTP adapter. Business behavior remains in existing ingest, RAG, evaluation, project, and trace services.

Model injection is represented by narrow public interfaces required by ORAG operations. The deterministic mock implements those interfaces and is clearly named and documented as non-production. Production provider adapters remain internal until their public contracts are justified by downstream use.

### Errors

The SDK defines stable Beta error categories usable with `errors.Is` and typed details usable with `errors.As`. Categories include invalid argument, unauthorized, forbidden, not found, conflict, unavailable, deadline, and internal. Error details preserve operation, resource, trace ID, retryability, and cause without exposing internal concrete types or secrets.

### Consumer proof

Tests use `package orag_test`. A separate module under `tests/consumer` imports the repository through a module replacement during CI, compiles runnable examples, and scans exported API output to ensure no `internal/` types leak. The release gate runs the consumer against the candidate tag before the GitHub Release is marked complete.

## 8. Full-Stack Compose and Mock Walkthrough

### Service topology

The Compose project contains:

```text
postgres ─┐
          ├─> migrate ─> api ─> console
qdrant  ──┘             │
                        └─> demo
```

- `postgres` and `qdrant` own persistent health-checked data services.
- `migrate` is a one-shot job that completes successfully before the API starts.
- `api` starts only after storage is healthy and migration exits successfully.
- `console` serves the built frontend and proxies or targets the configured API URL.
- `demo` is an idempotent one-shot initializer that waits for API readiness, obtains a bootstrap token, creates demo resources, ingests deterministic content, runs a cited query, records a trace, creates evaluation data, runs an evaluation, and writes a machine-readable walkthrough summary.

The supported command is:

```bash
docker compose -f deployments/docker-compose.yml --profile demo up --build --wait
```

The base stack includes migration, API, and Console. The `demo` profile adds no-key mock configuration and seeded walkthrough data. Production-oriented use does not inherit mock providers, demo credentials, or demo data.

### Deterministic mock

The mock provider is opt-in through an explicit configuration flag and provider name. It implements chat, embeddings, rerank, and required multimodal behavior with deterministic results. It does not make network calls, accept real provider credentials, or silently replace a misconfigured production provider.

Startup fails when `mock` is selected without the explicit allow flag, or when a real provider is selected without its required key. Logs and `/readyz` identify mock mode so screenshots and traces cannot be mistaken for production evidence.

### Idempotency and failure behavior

Migration uses the existing ordered migration mechanism and exits non-zero on failure. Demo initialization uses stable external IDs or discovery-before-create, so repeated runs converge rather than duplicate resources. A failed demo job prints the last successful step, API error category, and trace ID. Compose health does not report success until the API and Console are ready and the demo job has completed when the profile is enabled.

## 9. Images and Release Workflow

The API and Console have independent image targets and GHCR names:

- `ghcr.io/shikanon/orag-api`;
- `ghcr.io/shikanon/orag-console`.

Images are built for `linux/amd64` and `linux/arm64`. Tags include the immutable Git tag and commit SHA; the prerelease does not move `latest`. OCI labels record source, revision, version, license, and creation time.

A tag matching `v*` triggers the release workflow. The workflow:

1. verifies the tag commit is reachable from `main`;
2. runs Go, OpenAPI, Console, consumer, integration, and Compose walkthrough gates;
3. builds and pushes both multi-architecture images by digest;
4. generates SBOM and provenance attestations;
5. signs images keylessly with GitHub OIDC;
6. creates a prerelease GitHub Release with image digests, checksums, migration notes, known limitations, and verification commands.

Workflow permissions are minimal: read repository contents, write packages and attestations, and write the release. Pull-request workflows never receive package or release write permissions.

The API binary, CLI, runtime version endpoint, image labels, and release metadata use one build-info package populated by linker flags. A mismatch fails the release.

## 10. Interactive and Hosted Documentation

`GET /openapi.yaml` serves the embedded release contract. `GET /docs` serves a pinned interactive OpenAPI UI configured to load that same endpoint. The UI supports bearer authentication, request examples, and clear SSE guidance. Assets are pinned or vendored so the page does not silently change with a floating CDN version.

The hosted site is built from repository Markdown and the same `api/openapi.yaml`, initially deployed to GitHub Pages. It includes:

- five-minute no-key quickstart;
- deployment and production configuration;
- interactive API reference;
- public Go SDK guide and runnable examples;
- capability maturity matrix;
- architecture and security model;
- troubleshooting and release verification;
- real Console screenshots and a concise walkthrough GIF.

Screenshots and the GIF are captured from the deterministic Compose demo at a fixed viewport and stored as documented source artifacts. A reproducible capture script records the tested commit and avoids credentials or user data. The docs build checks internal links, OpenAPI rendering, snippets, image references, and accessibility basics.

## 11. Community and Security Policy

`CONTRIBUTING.md` documents prerequisites, setup, test tiers, commit and PR expectations, generated files, documentation synchronization, maturity changes, and a first-contribution path. Issue forms collect reproduction details, expected behavior, versions, logs with a secret warning, scope, alternatives, and acceptance criteria.

`SECURITY.md` lists supported versions, uses GitHub Private Vulnerability Reporting as the primary intake, defines acknowledgement and triage targets, and forbids public security issues before coordinated disclosure. No personal email address is required in the repository.

The Contributor Covenant 2.1 is adopted with repository-owner enforcement contact through GitHub. The pull-request template requires explicit test evidence and flags security, compatibility, documentation, generated-artifact, and maturity effects.

Dependabot groups safe development updates by ecosystem but keeps security updates and major version changes separately reviewable. It covers Go modules, npm in `console`, GitHub Actions, and Docker base images.

## 12. Verification Matrix

### Governance and maturity

- GitHub community profile recognizes all policy files.
- YAML and issue-form validation passes.
- Manifest and OpenAPI reject missing or invalid maturity.
- Generated artifacts and README cannot drift from the maturity source of truth.
- Repository settings are read back through the GitHub API after mutation.

### Go SDK

- root external-package tests pass;
- standalone consumer module compiles and runs mock ingestion/query/evaluation;
- `go vet`, race tests, examples, and API export checks pass;
- cancellation, close, typed errors, and dependency failures are covered;
- HTTP and SDK golden tests produce equivalent core responses.

### Release and walkthrough

- local single-platform image builds pass on pull requests;
- release workflow validates both target platforms with Buildx;
- migration failure blocks API startup;
- Compose demo succeeds twice against the same volumes;
- no-key mode performs no external model call;
- production profile rejects mock leakage and weak demo defaults;
- image version, digest, SBOM, provenance, and signature are verifiable.

### Documentation

- `/openapi.yaml` matches the repository contract byte-for-byte for the build;
- `/docs` loads and can authorize and execute a safe request;
- hosted docs build and link checks pass;
- screenshots and GIF resolve and contain no secrets;
- quickstart commands are executed in CI or a release smoke workflow.

## 13. Release Gate and Rollback

The Beta release gate requires all four PRs merged, a clean `main`, all required checks green, the consumer module passing, the Compose mock walkthrough succeeding twice, documentation deployed, and both images built for both architectures.

If artifact publication fails before the GitHub Release exists, the tag is retained only while a rerun can deterministically publish the same commit and version. Artifacts are never replaced with builds from another SHA. If a published image is defective, maintainers document the issue and publish a new prerelease version rather than moving or rewriting `v0.1.0-beta.1`.

Repository setting changes are recorded in the first PR checklist and verified after application. If branch protection blocks the sole maintainer unexpectedly, only the minimal offending rule is relaxed, documented, and restored after correcting the workflow.

## 14. Accepted Decisions

- Use multiple focused PRs rather than one release mega-PR.
- Release only after all PRs merge and quality gates pass.
- Define the public SDK as an embeddable Go-native facade, not only an HTTP client.
- Keep stages date-free and allow overlap when dependencies permit.
- Preserve `v0.1.0-beta.1` as the first explicit public release target.
