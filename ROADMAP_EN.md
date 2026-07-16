# ORAG Open Source Roadmap

English | [简体中文](./ROADMAP.md)

Last updated: 2026-07-16

## Positioning

ORAG is a Go-native RAG service and control plane for Go and backend platform teams. It is evaluation-first: ingestion, hybrid retrieval, generation, observability, offline evaluation, optimization, and controlled promotion share one reproducible path.

ORAG does not aim to win by supporting the largest number of providers or UI pages. It focuses on three outcomes:

1. Go teams can adopt RAG through a deployable service or a real public Go SDK.
2. Retrieval, generation, and configuration changes can be evaluated, traced, and reproduced.
3. Only evaluated versions can reach staging and production, with an auditable rollback path.

## Roadmap principles

- **Reliability before breadth:** data consistency, tenant isolation, idempotency, and recovery take priority over new providers, pages, or retrieval strategies.
- **Evaluation uses the production path:** evaluation, optimization, and release gates reuse real query execution.
- **One core for API and SDK:** the HTTP service and public Go SDK share the application layer rather than duplicating business rules.
- **Verifiable onboarding:** users without a real model key can complete an explicit mock walkthrough; production configuration must never enable mock behavior implicitly.
- **Honest maturity labels:** experimental features are not marketed as stable, and promotion requires public exit criteria.
- **Community participation is a product feature:** open decisions, reproducible issues, reviewable changes, and clear contribution paths matter as much as code.

## Capability maturity

User-facing capabilities will use the same status in the README, documentation, OpenAPI extensions, and capability manifest:

| Status | Meaning | Compatibility commitment |
| --- | --- | --- |
| `experimental` | The problem and interface are still being validated | May change or be removed in a minor release, with release-note disclosure |
| `beta` | The primary journey is complete and ready for real trials or controlled production pilots | Avoid breaking changes without a migration path; document all breaking changes |
| `stable` | Stable contracts, upgrade paths, operations documentation, and production adoption evidence exist | Follow SemVer; breaking changes require a major release |
| `planned` | Publicly scheduled but not yet a usable contract | Must not be presented by docs or UI as available |

Current baseline:

| Capability area | Current status | Promotion requirement |
| --- | --- | --- |
| HTTP API, knowledge bases, ingestion, JSON/SSE query | `beta` | Close consistency gaps and validate versioned contracts in production pilots |
| PostgreSQL + Qdrant hybrid retrieval, RRF, rerank, semantic cache | `beta` | Complete load, recovery, and compatibility validation |
| Datasets, evaluations, LLM-as-Judge, optimizer | `beta` | Publish reproducible benchmarks and enforce budget/concurrency limits |
| Application traces, Prometheus metrics, readiness and health | `beta` | Complete metrics persistence/sampling policy and cross-service topology |
| Contextual Retrieval, RAPTOR, Query Router, Graph Retrieval | `experimental` | Publish ablations, cost, fallback behavior, and recommended use cases |
| Offline Knowledge and MCP self-check/diagnose/ops | `experimental` | Remove fixture dependencies and validate approval and audit boundaries |
| ORAG Console | `experimental` | Complete orchestration, API debugging, evaluation gates, promotion, and rollback |
| Tutorial Lab | `experimental` | Support clone, Quick Run, Replay, and result comparison |
| Public Go SDK | `beta` | Shipped in `v0.1.0-beta.2`; continue validation through the external consumer gate and compatibility policy |
| GHCR images, full-stack Compose, hosted docs | `beta` | Shipped in `v0.1.0-beta.2`; continue validating signatures, the walkthrough, and documentation contracts |

ORAG will not label any capability `stable` before `v1.0.0`.

## Delivery phases

Phases advance according to quality gates and available capacity, without target dates. Phases may overlap, and the project can move forward as soon as the relevant exit criteria are met.

| Phase | Outcome |
| --- | --- |
| Phase 1: Trusted open-source baseline | Community governance, security intake, maturity labels, and a protected default branch |
| Phase 2: Release `v0.1.0-beta.1` | A downloadable, runnable, embeddable Beta with a no-key walkthrough |
| Phase 3: Production pilot baseline | Consistency, security, observability, and CI/CD hardening for reference deployments |
| Phase 4: Evaluation-first control plane | Orchestration, evaluation gates, promotion/rollback, and tutorial experimentation |
| Phase 5: Ecosystem and `v1.0` readiness | Stable extension points, governance, compatibility policy, and adoption evidence |

## Phase 1: Trusted open-source baseline

### Community and governance

- Add `CONTRIBUTING.md` covering setup, test matrices, commits, pull requests, documentation synchronization, and a first-contribution path.
- Add `SECURITY.md` with supported versions, private reporting, response targets, disclosure policy, and GitHub Private Vulnerability Reporting.
- Adopt Contributor Covenant 2.1 in `CODE_OF_CONDUCT.md` with clear enforcement ownership.
- Add Bug, Feature, Documentation, and RFC issue forms plus a pull-request template covering tests, docs, security, compatibility, and maturity changes.
- Establish `good first issue`, `help wanted`, `area/*`, `maturity/*`, and `priority/*` labels with public triage rules.
- Enable GitHub Discussions with Announcements, Q&A, Ideas, and Show and tell categories.
- Configure repository topics: `rag`, `retrieval-augmented-generation`, `golang`, `llm-evaluation`, `qdrant`, `postgresql`, `openapi`, `mcp`, `eino`, and `hertz`.
- Protect `main`: prevent force pushes and deletion, require up-to-date branches, required checks, and resolved review conversations. Do not require an external approving reviewer until a second maintainer exists.
- Enable weekly Dependabot updates for Go modules, npm, GitHub Actions, and Docker; handle security updates immediately.

### Maturity and release discipline

- Add a shared `x-orag-maturity` OpenAPI extension accepting only `experimental`, `beta`, or `stable`.
- Reuse the same maturity enum in the capability manifest and add contract tests that prevent drift across README, OpenAPI, and generated artifacts.
- Define SemVer, deprecation, migration, and release-note policy; experimental changes still appear in the changelog.
- Add `CHANGELOG.md` and a public roadmap update process. Refresh the capability matrix at every minor release and review this file when priorities or project status change.

### Phase exit criteria

- GitHub community profile reaches at least 90%.
- `main` protection and required checks are active, and Dependabot can create verified pull requests.
- Every current consistency or concurrency issue is fixed, or has an owner, priority, target release, and verified mitigation.
- README, OpenAPI, and capability manifest maturity labels agree and are checked in CI.

## Phase 2: Release `v0.1.0-beta.1`

### Reproducible artifacts

- Add a tag-driven release workflow. Only `v*` tags create GitHub Releases and GHCR images; normal `main` pushes run CI only.
- Publish `linux/amd64` and `linux/arm64` images for at least `orag-api` and `orag-console`.
- Generate SBOMs, provenance, checksums, and keyless signatures. Release notes include image digests, migrations, changes, and known limitations.
- Make `orag-api --version`, `oragctl version`, and the runtime version endpoint return the same version, commit, and build time.

### One-command full-stack experience

- The full Compose stack includes PostgreSQL, Qdrant, a one-shot migration service, API, Console, and demo/walkthrough.
- `docker compose --profile demo up --wait` uses explicit deterministic mock configuration, requires no real model key, and seeds a queryable knowledge base and evaluation dataset.
- The production profile must not inherit mock providers or weak credentials. Demo data, volumes, credentials, and port policies are explicitly isolated.
- The walkthrough covers login, ingestion, cited query, trace, evaluation, and one parameter comparison, with results visible in the Console.
- Validate the journey on clean macOS, Linux amd64, and Linux arm64 environments.

### Interactive and hosted documentation

- Replace `/docs` with an interactive UI backed by the repository OpenAPI contract, including authentication, request examples, and SSE guidance.
- Publish a hosted documentation site, initially through GitHub Pages. README, hosted docs, and `/docs` must generate from or validate against the same OpenAPI and examples.
- Add an architecture overview, real Console screenshots, a five-minute walkthrough GIF, deployment guidance, SDK documentation, and a maturity page.
- Gate documentation builds, internal links, code snippets, and OpenAPI coverage in CI.

### A real public Go SDK

- Provide a public `github.com/shikanon/orag` facade at the module root; public signatures must not expose `internal/*` types.
- Share application assembly, ingestion, query, evaluation, and trace services between the SDK and HTTP service. The HTTP layer remains a protocol adapter.
- The first Beta covers client lifecycle, knowledge-base management, text/file ingestion, synchronous and streaming query, evaluation submission/status, and trace lookup.
- Support explicit memory/mock and PostgreSQL + Qdrant configurations. Test models must be clearly identified and must not impersonate production providers.
- Prove external usability with external test packages and a standalone consumer module. Publish pkg.go.dev documentation, runnable examples, and compatibility guidance.
- Provide stable error categories compatible with `errors.Is`/`errors.As`, preserving trace ID, retryability, and the underlying cause.

### Phase exit criteria

- The `v0.1.0-beta.1` tag, GitHub Release, dual-architecture GHCR images, SBOMs, and signatures are publicly verifiable.
- Median clone-to-first-cited-answer time is below 10 minutes, with at least 10 non-maintainer testers and a 90% completion rate.
- A standalone Go module imports the public SDK; examples, race tests, API documentation, and upgrade checks pass.
- Console, interactive `/docs`, hosted docs, and mock walkthrough use one versioned contract.

## Phase 3: Production pilot baseline

Current progress: [#175](https://github.com/shikanon/orag/issues/175) is implemented according to the [cross-store staged visibility design](./docs/superpowers/specs/2026-07-15-qdrant-staged-visibility-design.md). PostgreSQL now authorizes sparse and dense visibility, failed candidates cannot enter retrieval, and the protocol is covered by real PostgreSQL + Qdrant failure, replacement, legacy, cleanup-warning, and concurrency tests. [#177](https://github.com/shikanon/orag/issues/177) is also implemented according to the [retryable knowledge-base deletion design](./docs/superpowers/specs/2026-07-15-kb-delete-retry-design.md): failed external cleanup retains metadata as a durable retry handle, and real-store tests prove that repeating DELETE completes cleanup. [#176](https://github.com/shikanon/orag/issues/176) implements the [optimizer single-flight state-transition design](./docs/superpowers/specs/2026-07-15-optimizer-singleflight-design.md): cross-instance CAS gives concurrent resume/run/candidate claims exactly one winner, returns a diagnosable `409` to losers, and is covered by real PostgreSQL concurrency tests. For observability, [#166](https://github.com/shikanon/orag/issues/166) was closed after verifying the existing real TraceGetter/MCP HTTP wiring and found/not-found/unavailable tests; [#163](https://github.com/shikanon/orag/issues/163) now has focused regression proof that graph spans carry contiguous sequence values and real UTC execution windows before persistence. For security, [#211](https://github.com/shikanon/orag/issues/211) upgrades all six Go dependency families named by 27 Dependabot alerts and passes the public SDK, OpenAPI, race, and real PostgreSQL + Qdrant integration gates; [#213](https://github.com/shikanon/orag/issues/213) adds SHA-pinned CodeQL, `govulncheck`, npm audit, reachable-history secret scanning, dual-image Trivy, and isolated Scorecard workflows while moving the Go runtime to 1.26.5, which fixes GO-2026-5856; [#215](https://github.com/shikanon/orag/issues/215) bounds Fake Ark vector allocation with focused regression tests after the first CodeQL run identified an uncontrolled size; [#217](https://github.com/shikanon/orag/issues/217) pins every workflow action and base image to an immutable SHA or digest and reduces the general CI token to read-only access. This completes only part of the consistency, security, and observability work below; Phase 3 is not complete.

For continuous security exploration, [#219](https://github.com/shikanon/orag/issues/219) adds native Go fuzz targets, pull-request smoke gates, and longer scheduled exploration for document/Office parsing and optimizer expressions.

For dependency boundaries, [#221](https://github.com/shikanon/orag/issues/221) upgrades Eino to 0.9.12 with its Jinja filesystem-access fix, adds `file`/`fileset` regressions, and records why GO-2026-5932 is unreachable and has no fixed upstream x/crypto release.

### Consistency and execution safety

- Use staged/active visibility or an equivalent transaction protocol so failed documents and vectors never become searchable early.
- Make knowledge-base deletion, upload recovery, and optimizer resume idempotent, concurrency-safe, compensatable, and retryable.
- Add migration completeness checks, Qdrant collection compatibility checks, backup/restore exercises, and disaster-recovery documentation.
- Define and test timeout, retry, cancellation, and backpressure behavior for ingestion, query, evaluation, and release.

### Security and tenant boundaries

- Add machine API keys, minimal RBAC, and project-scoped authorization; default administrator credentials are bootstrap-only.
- Threat-model and test secret injection/rotation, log redaction, prompt/document recording, and cross-tenant queries.
- Add CodeQL, `govulncheck`, npm audit, secret scanning, container scanning, and OpenSSF Scorecard to CI.

### Observability and quality gates

- Optional OpenTelemetry trace/metrics exporters and importable Prometheus/Grafana resources with baseline alerts are available; complete metrics persistence, sampling, and cross-service topology next.
- Gate Go unit/vet/race, OpenAPI, Console typecheck/unit/build/E2E, PostgreSQL + Qdrant integration, and dual-architecture image smoke tests.
- Publish performance baselines for ingestion throughput, query p50/p95, cache hit rate, evaluation duration, model calls, and cost accounting.

### Phase exit criteria

- A production pilot runs for 30 days without an unmitigated P0; known P1 issues have owners and target releases.
- At least two independent reference deployments complete upgrade, backup/restore, and rollback exercises.
- Security, integration, Console, and release checks are required on `main`.

## Phase 4: Evaluation-first control plane

Current progress: Tutorial Lab supports durable, idempotent, resumable template cloning and Pack installation plus `text-rag` Quick/Benchmark P0–P8 single-variable runs. The controlled Benchmark Pack fixes `high_precision`/Top-K 8 and persists Manifest SHA-256, runtime-environment SHA-256, build revision, frozen evaluation inputs, and evaluator-v5 identity; comparisons require direct P0 lineage and identical reproduction evidence. The official `text-rag` Replay is now an embedded, offline, read-only snapshot with versioned Pack/environment/build evidence and P0/P8 audit facts; it is not a claim about a user's Live Run. `text-rag/1.1.0` has been built from a locked CRUD-RAG revision and publicly released over anonymous HTTPS: Quick, Benchmark, the complete `data/` archive, `SOURCE.json`, and `SHA256SUMS` were verified by downloading every artifact. Visual-document/video Replay and Live Runs remain outstanding.

Latest progress: the Pack-declared P2 `p2_recursive_400_80` is now a direct child of P0. Tutorial P0/P1 chunking is pinned at 800/120; P2 changes only recursive chunking to 400/80 while retaining the Basic parser and an independent Knowledge Base. Each run persists measured chunk count and average text units, and comparisons keep these `index_metrics` separate from ordinary evaluation metrics. The controlled `1.0.2` fixture and real PostgreSQL/Qdrant/browser E2E prove P0/P1/P2 isolation and a measurable index shape; the official public `1.0.2` Pack still needs a separate anonymous-HTTPS, MIME, length, and SHA-256 publication pipeline.

### Project-to-release golden path

- Complete the project-scoped RAG Studio, constrained DAG, API Debugger, and immutable pipeline versions.
- Complete project-scoped datasets, frozen evaluation runs, hard metric gates, and candidate comparison.
- Complete ordered development-to-staging-to-production promotion, non-bypassable gates, optimistic concurrency, append-only audit, and atomic rollback.
- Resolve production queries to an explicit active version. Traces record pipeline, model, retrieval parameters, dataset, and release lineage.

### Tutorial experimentation loop

- Clone and Pack installation, `text-rag` Quick P0–P8 candidates, the controlled Benchmark Run, the offline read-only official text-rag Replay, and the public `text-rag/1.1.0` Pack are complete with frozen inputs, durable reproduction evidence, actual evaluation metrics for Live Runs, measured index facts, and anonymous artifact verification. Visual-document/video Replay and execution remain pending.
- Keep text, visual-document, and video tutorials focused on engineering and evaluation; model training is out of scope.
- Document per-strategy ablations, cost, latency, failure fallback, and recommended scenarios.

### Phase exit criteria

- The create-project-to-release-and-rollback browser E2E passes against real PostgreSQL and Qdrant.
- At least two public benchmarks are fully reproducible from tagged images, configuration, and datasets.
- At least five external teams use ORAG continuously, three external pull requests merge, and two production cases are publicly referenceable.

## Phase 5: Ecosystem and `v1.0` readiness

### Stable extension points

- Define minimal stable interfaces and conformance tests for parsers, chunkers, embeddings, retrievers, rerankers, model providers, and storage adapters.
- Expand integrations only in response to real users. Separate certified, community, and experimental integrations in the support matrix.
- Publish SDK/API compatibility policy, deprecation windows, upgrade tooling, and long-term support scope.

### Community governance and awareness

- Establish RFCs, maintainer/committer roles, decision records, and a security response rotation.
- Maintain a predictable, quality-gated release process and public changelog, and review the roadmap whenever project status changes.
- Grow through reproducible benchmarks, architecture articles, tutorials, public cases, conference talks, and community demos rather than feature lists or star campaigns.
- Publish Helm charts and cloud reference architectures only after sustained Kubernetes demand; Docker/Compose remains the primary path until then.

### `v1.0` exit criteria

- At least 10 confirmed production deployments with upgrade and recovery evidence, including two public cases.
- At least 20 external contributors and three maintainers capable of independent review and release.
- Core API and Go SDK pass compatibility audits, with two consecutive minor releases containing no breaking change without a migration path.
- Security response, dependency updates, releases, backup/restore, capacity, and incident procedures have exercise records.

## Project metrics

Adoption and trust are primary. Stars are a lagging awareness signal.

| Dimension | Metrics |
| --- | --- |
| Activation | Time to first success, walkthrough completion, documentation-to-runtime drop-off |
| Reliability | P0/P1 count, ingestion recovery, release failure rate, rollback time, SLO attainment |
| Adoption | Active external deployments, 30/90-day retention, production upgrades, public cases |
| Community | External contributors, first response time, pull-request lead time, independent maintainers |
| Quality | Benchmark reproducibility, contract compatibility, coverage, vulnerability remediation time |
| Awareness | Documentation MAU, organic search/citations, technical content adoption, stars and forks |

## Explicit non-goals

- ORAG is not a model training platform.
- ORAG will not copy a broad general-purpose AI application platform before reliability, release, and evaluation loops are complete.
- Provider count is not a primary success metric. Providers without conformance tests and maintainers do not enter the certified list.
- ORAG does not promise long-term stable interfaces before `v1.0`.
- Automated remediation and agent operations never bypass approval, audit, or rollback boundaries.

## Participating in the roadmap

- Use GitHub Issues for bugs and well-scoped requests.
- Use an RFC Issue and Discussions before implementing cross-module interfaces, compatibility changes, or governance changes.
- Split each phase into independently testable and reviewable implementation plans. This roadmap does not replace engineering design.
- Maintainers update phase status and priorities as production feedback, community demand, project status, and maintenance capacity change.
- Change this roadmap through pull requests that explain the reason, affected phases, and metric impact.
