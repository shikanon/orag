# Changelog

All notable changes to ORAG are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html) for published versions. Before `v1.0.0`, experimental and beta contracts may change according to [the compatibility policy](./docs/compatibility.md).

## [Unreleased]

### Added

- Added a phased PostgreSQL/Qdrant ingestion protocol with real-store failure, replacement, legacy payload, cleanup-warning, and concurrency integration coverage.
- Added regression coverage proving RAG graph spans reach trace persistence with contiguous sequence numbers and real UTC execution windows, without relying on store-time fallback normalization.

### Changed

- Qdrant points now record staged `searchable` state and `ingestion_job_id`; PostgreSQL `chunks.searchable` authorizes every dense candidate, including historical points without the new payload fields.
- Post-commit Qdrant cleanup failures now produce succeeded ingestion jobs with warnings instead of incorrectly reporting a committed document as failed.

### Fixed

- Failed or partially activated ingestions can no longer expose Qdrant vectors, and dense visibility lookup failures now fail closed.
- Same-source PostgreSQL activation is serialized with a tenant-, knowledge-base-, and source-scoped advisory transaction lock.
- Knowledge-base deletion now cleans semantic cache and vectors before PostgreSQL metadata, so transient Qdrant failures retain a durable DELETE retry path instead of leaving unreachable orphan points.
- Optimizer resume and execution acquisition now use PostgreSQL-backed compare-and-swap transitions, preventing duplicate run and candidate execution across concurrent API replicas and returning a documented `409` conflict to losing callers.

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

[Unreleased]: https://github.com/shikanon/orag/compare/v0.1.0-beta.1...HEAD
[0.1.0-beta.1]: https://github.com/shikanon/orag/releases/tag/v0.1.0-beta.1
