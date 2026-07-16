# Changelog

All notable changes to ORAG are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html) for published versions. Before `v1.0.0`, experimental and beta contracts may change according to [the compatibility policy](./docs/compatibility.md).

## [Unreleased]

### Added

- Added `oragctl backup-verify`, a read-only verifier for versioned PostgreSQL/Qdrant backup manifests, required artifacts, SHA-256 integrity, migration provenance, and credential-leakage boundaries before a restore drill.
- Added independent fail-fast execution budgets for synchronous ingestion, query, evaluation, and release writes. Each class now has configured concurrency and a propagated deadline; saturation returns retryable `429` rather than an unbounded in-memory queue, and expired work returns `504`.
- Added the strict `orag.performance-baseline.v1` contract and `oragctl benchmark-report` verifier, which bind a deterministic Benchmark Pack run to its environment/build/load fingerprints and reject incomparable or internally inconsistent throughput, latency, cache, evaluation, model-call, and cost reports.
- Added an optional OTLP/HTTP metrics exporter for core HTTP, RAG, dependency-readiness, and trace-store telemetry, with low-cardinality attribute and shutdown-flush contract tests.
- Added a versioned, importable Grafana overview dashboard for ORAG's documented Prometheus metrics, a parser-backed dashboard contract gate, and an operator guide that preserves the low-cardinality, read-only observability boundary.
- Added the public, immutable `text-rag/1.1.0` CRUD-RAG release: deterministic Quick and Benchmark Packs, the complete upstream `data/` archive, source provenance, `SHA256SUMS`, anonymous HTTPS verification, a guarded TOS publisher, and a hosted release guide.
- Added a reproducible experimental `text-rag` Benchmark Pack Run: verified Manifest/environment/build evidence, fixed high-precision inputs, P0–P8 contracts, read-only API/Console audit fields, and a controlled PostgreSQL/Qdrant/browser walkthrough.
- Added the experimental Pack-declared `p8_context_pack` tutorial candidate: direct P0-only lineage, evaluator-v5 isolation, compatible P0-index reuse, durable Context Pack audit fields, read-only API/Console output, and a controlled PostgreSQL/Qdrant/browser P0-to-P8 walkthrough.
- Added the experimental Pack-declared `p7_graph_retrieval` tutorial candidate: direct P0-only lineage, evaluator-v4 isolation from ambient graph/RAPTOR defaults, a separate graph-enabled candidate index, durable graph audit fields, read-only API/Console output, and a controlled PostgreSQL/Qdrant/browser P0-to-P7 walkthrough.
- Added the experimental Pack-declared `p6_rerank_retrieval` tutorial candidate: direct P0-only lineage, evaluator-v3 rerank isolation, compatible P0-index reuse, durable rerank audit fields, read-only API/Console output, and a controlled PostgreSQL/Qdrant/browser P0-to-P6 walkthrough.
- Added the experimental Pack-declared `p5_multi_query_retrieval` tutorial candidate: direct P0-only lineage, evaluator-v2 isolation from production rewrite/HyDE/cache/pipeline defaults, P0-index reuse, fixed three-query expansion audit fields, read-only API/Console output, and a controlled PostgreSQL/Qdrant/browser P0-to-P5 walkthrough.
- Added the experimental Pack-declared `p4_sparse_retrieval` tutorial candidate: direct P0-only lineage, pure sparse evaluator isolation, compatible P0-index reuse, durable retrieval/index-reuse audit fields, read-only API/Console output, and a controlled PostgreSQL/Qdrant/browser P0-to-P4 walkthrough.
- Added the experimental Pack-declared `p3_contextual_retrieval` tutorial candidate: direct P0-only lineage, isolated strict-fail server-owned contextualization, durable actual context facts, read-only API/Console audit output, and a controlled PostgreSQL/Qdrant/browser P0-to-P3 walkthrough.
- Added the experimental Pack-declared `p2_recursive_400_80` tutorial candidate: fixed P0/P1 800/120 splitters, a direct P0-only P2 parent, independent 400/80 indexes, durable measured chunk facts, generic candidate comparisons, Console P2 controls, and a real PostgreSQL/Qdrant/browser fixture that proves differing index shapes.
- Added the experimental Pack-declared `p1_structured_json` tutorial candidate: immutable P0/P1 comparison fingerprints, a P0 parent requirement, independent candidate indexes, durable parser/lineage audit fields, a comparison API exposing only persisted standard-evaluation metric deltas, Console P0/P1 controls, and a controlled JSON Pack fixture.
- Added an experimental Text Quick Pack baseline Live Run: verified Manifest snapshots, private Pack reads, project-scoped knowledge-base/dataset roots, durable/recoverable/cancellable runs, standard evaluation references, Console progress, and a no-key PostgreSQL/Qdrant browser E2E.
- Added the experimental tutorial clone control plane: idempotent, durable and restart-recoverable Pack jobs; strict anonymous public-manifest/object validation; server-only local or Aliyun private output; project-scoped authorization; Console Pack selection and redacted progress; and a real PostgreSQL/Qdrant browser E2E with a controlled public fixture.
- Added project ownership and project-scoped authorization for evaluation and optimization run roots, including cross-project input rejection, migration backfill, OpenAPI fields, and public Go SDK scoping.
- Added Console admin login, versioned tab-scoped Bearer sessions, authenticated API requests, automatic 401 session invalidation, and explicit logout.
- Added Console API Key management with project-aware roles, metadata-only listing, explicit revocation confirmation, and a one-time secret reveal that is cleared when closed.
- Added public Go SDK project and API key lifecycle methods, including one-time secret creation, metadata-only listing, revocation, embedded authentication, and project ownership on knowledge bases and datasets.
- Added a phased PostgreSQL/Qdrant ingestion protocol with real-store failure, replacement, legacy payload, cleanup-warning, and concurrency integration coverage.
- Added regression coverage proving RAG graph spans reach trace persistence with contiguous sequence numbers and real UTC execution windows, without relying on store-time fallback normalization.
- Added SHA-pinned CodeQL, `govulncheck`, production npm audit, reachable-history secret scanning, API/Console container scanning, and published OpenSSF Scorecard workflows.

## [0.1.0-beta.2] - 2026-07-15

### Changed

- Published the current authenticated project/API-key SDK surface as a reproducible Beta.2 release with dual-architecture GHCR images and the full release verification gate.
- The standalone consumer now resolves `github.com/shikanon/orag v0.1.0-beta.2` directly, proving the current public SDK can be consumed without a repository-local module replacement.

### Changed

- Qdrant points now record staged `searchable` state and `ingestion_job_id`; PostgreSQL `chunks.searchable` authorizes every dense candidate, including historical points without the new payload fields.
- Post-commit Qdrant cleanup failures now produce succeeded ingestion jobs with warnings instead of incorrectly reporting a committed document as failed.

### Fixed

- Optimizer expression evaluation now preserves very large finite values during rounding and rejects true NaN/Inf results instead of allowing non-finite scores into candidate ranking.
- Failed or partially activated ingestions can no longer expose Qdrant vectors, and dense visibility lookup failures now fail closed.
- Same-source PostgreSQL activation is serialized with a tenant-, knowledge-base-, and source-scoped advisory transaction lock.
- Knowledge-base deletion now cleans semantic cache and vectors before PostgreSQL metadata, so transient Qdrant failures retain a durable DELETE retry path instead of leaving unreachable orphan points.
- Optimizer resume and execution acquisition now use PostgreSQL-backed compare-and-swap transitions, preventing duplicate run and candidate execution across concurrent API replicas and returning a documented `409` conflict to losing callers.

### Security

- Upgraded Eino from 0.6.0 to 0.9.12 and added a dependency-contract regression proving Jinja `file` and `fileset` filters cannot read local files; documented why GO-2026-5932 is an unreachable module-level OpenPGP advisory with no fixed x/crypto release.
- Added native Go fuzz targets for untrusted document/Office archive parsing and optimizer expression compilation, with short pull-request gates, longer scheduled exploration, and retained crash artifacts.
- Pinned every CI action to an immutable commit and every API/Console base image to a verified multi-architecture manifest digest; the general CI token now defaults to read-only contents access while release-specific jobs retain only their required write scopes.
- Upgraded the six Go dependency families behind 27 Dependabot alerts, including `kin-openapi`, `pgx/v5`, gRPC, `x/crypto`, `x/net`, and `phonenumbers`, while preserving standalone SDK, OpenAPI, PostgreSQL, and Qdrant compatibility.
- Upgraded the project, CI, examples, and API image builder to Go 1.26.5 after `govulncheck` identified the reachable standard-library vulnerability GO-2026-5856 in Go 1.26.4.
- Upgraded `jsonparser` to 1.1.2 and the Console runtime to the official NGINX 1.30.3 Alpine image with build-time Alpine security updates after the container gate found CVE-2026-32285 and fixed HIGH/CRITICAL packages in the legacy runtime image.
- Bounded Fake Ark embedding dimensions and reject oversized requests before allocation, closing the high-severity CodeQL uncontrolled-allocation path.

## [0.1.0-beta.1] - 2026-07-14

### Added

- Open-source roadmap for an evaluation-first, Go-native RAG service and control plane.
- Community governance with contribution, security, conduct, Issue/PR templates, Discussions, Topics, protected `main`, and Dependabot.
- Public embedded Go SDK for knowledge bases, ingestion, query events, traces, datasets, and deterministic evaluation.
- Standalone downstream-module compatibility gate and no-key SDK walkthrough.
- Multi-architecture API and Console container definitions, full Compose migration/API/Console/demo topology, and an idempotent no-key HTTP walkthrough.
- Tag-driven GHCR prerelease workflow with immutable version/SHA tags, SBOM/provenance attestations, and keyless image signatures.
- Interactive runtime `/docs`, canonical `/openapi.yaml`, and a GitHub Pages documentation site with real browser screenshots and a walkthrough GIF.

### Changed

- Roadmap delivery is expressed as date-free phases governed by exit criteria.
- Every OpenAPI operation declares `experimental`, `beta`, or `stable` capability maturity.

### Security

- Added a private vulnerability reporting and coordinated disclosure policy.

### Known limitations

- This is a pre-1.0 Beta distribution. Most product capabilities remain `experimental`; use the per-operation OpenAPI maturity label as the compatibility source of truth.
- Deterministic mock providers are for walkthroughs and tests only. Production deployments require explicit provider and storage configuration.
- The public Go SDK is intentionally pre-1.0 and follows the migration rules in `docs/compatibility.md`.

[Unreleased]: https://github.com/shikanon/orag/compare/v0.1.0-beta.2...HEAD
[0.1.0-beta.2]: https://github.com/shikanon/orag/releases/tag/v0.1.0-beta.2
[0.1.0-beta.1]: https://github.com/shikanon/orag/releases/tag/v0.1.0-beta.1
