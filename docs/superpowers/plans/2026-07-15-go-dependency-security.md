# Go Dependency Security Remediation Plan

> **Issue:** [#211](https://github.com/shikanon/orag/issues/211)

**Goal:** Upgrade every Go package family named by the 27 open Dependabot alerts and prove ORAG, the public SDK consumer, PostgreSQL, Qdrant, OpenAPI, and gRPC paths remain compatible.

### Task 1: Upgrade affected modules

- [x] Upgrade root direct dependencies to at least: kin-openapi v0.131.0, pgx/v5 v5.9.2, grpc v1.79.3.
- [x] Upgrade transitive security floors to at least: x/crypto v0.52.0, x/net v0.55.0, phonenumbers v1.2.2.
- [x] Run root go mod tidy and review all transitive changes.
- [x] Run GOWORK=off go mod tidy in tests/consumer and ensure kin-openapi is at least v0.131.0.

### Task 2: Validate compatibility

- [x] Run targeted PostgreSQL, HTTP/OpenAPI, SDK, and gRPC/Qdrant package tests.
- [x] Run the standalone consumer test and vet gates.
- [x] Run make agent-gate.
- [x] Run make test-integration against real PostgreSQL and Qdrant.
- [x] Run applicable race tests for database and public SDK paths.

### Task 3: Document, publish, and verify alerts

- [x] Record dependency remediation in CHANGELOG.md and the bilingual Stage 3 Roadmap while keeping the phase incomplete.
- [x] Push codex/go-dependency-security and open a ready PR with Closes #211.
- [x] Wait for required checks, squash merge, sync main, and remove only this worktree/branch.
- [ ] Poll Dependabot after GitHub re-evaluates main and prove the open-alert count reaches zero; otherwise continue remediation.
