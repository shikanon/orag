# Contributing to ORAG

Thank you for helping improve ORAG. We welcome bug reports, documentation fixes, focused features, evaluation datasets, and reproducible benchmarks.

By participating, you agree to follow the [Code of Conduct](./CODE_OF_CONDUCT.md). Security vulnerabilities must follow [SECURITY.md](./SECURITY.md) and must not be reported in public issues.

## Before You Start

- Search existing Issues and Discussions before opening a new proposal.
- Use an Issue for a reproducible bug or a well-scoped change.
- Use an RFC Issue and a Discussion before changing cross-module contracts, compatibility policy, security boundaries, or public APIs.
- Keep pull requests focused. Separate unrelated refactors from behavior changes.
- Do not include model keys, credentials, `.env` files, production data, or private traces.

## Development Setup

Requirements:

- Go 1.26 or newer, using the toolchain version declared in `go.mod`;
- Node.js and npm for `console/` changes;
- Docker with Docker Compose for PostgreSQL, Qdrant, and integration tests.

Set up the repository:

```bash
git clone https://github.com/shikanon/orag.git
cd orag
go mod download
npm --prefix console ci
cp .env.example .env
```

Do not commit the local `.env` file. The default service configuration requires a real model provider key. Explicit deterministic mock mode is only for tests and documented demos.

## Running Checks

Run the smallest relevant test while developing, then run the complete affected gate before opening a Pull Request.

```bash
make test
make vet
make openapi-validate
make agent-gate
npm --prefix console test -- --run
npm --prefix console run build
```

Integration tests use isolated PostgreSQL and Qdrant ports:

```bash
make test-integration-up
make test-integration
make test-integration-down
```

If a check cannot run locally, explain why and identify the CI check that covers it.

## Change Requirements

### Tests first

Behavior changes and bug fixes require a failing automated test that demonstrates the intended behavior before the implementation. Prefer narrow tests near the owning package, then add contract or integration coverage where boundaries are involved.

### API and documentation synchronization

HTTP behavior is defined jointly by `internal/http/router.go` and `api/openapi.yaml`. API changes must update both, their contract tests, relevant pages under `docs/api/`, and generated Console types when applicable.

Public Go SDK changes must include external-package tests (`package orag_test`) and compile in the standalone consumer module. Exported signatures must not expose `internal/*` types.

### Generated artifacts

Files under `agent/mcp/` and `agent/skills/` are generated from the capability manifest. Change the source manifest or generator and run:

```bash
make agent-sync
make agent-sync-check
```

Do not hand-edit generated artifacts without changing their source of truth.

### Capability maturity

Every public capability is marked `experimental`, `beta`, or `stable`. A change that adds or alters public behavior must state whether maturity changes and update the capability manifest, OpenAPI extension, generated artifacts, and public documentation together. No capability is `stable` before `v1.0.0`.

## Commits and Pull Requests

Use concise imperative commit subjects, preferably with a conventional prefix such as `feat:`, `fix:`, `docs:`, `test:`, or `chore:`. Keep commits reviewable and avoid mixing generated output with unrelated edits.

A Pull Request must include:

- the problem and intended outcome;
- the implementation and important trade-offs;
- exact test commands and results;
- documentation, security, compatibility, and maturity impact;
- screenshots or recordings for visible Console or documentation changes;
- a linked Issue when the work changes behavior beyond a small fix.

Maintainers may ask for a smaller scope, an RFC, additional evaluation evidence, or migration guidance. A green CI run is required but does not replace review.

## First Contributions

Look for `good first issue` or `help wanted`. Documentation corrections, missing tests, clearer examples, and small provider-independent fixes are good starting points. Comment on the Issue before substantial work so maintainers can confirm scope and avoid duplication.

## Review and Release

Changes merge through pull requests into `main`. Force pushes and deletion of `main` are prohibited. Releases are created from version tags after the documented release gates pass; maintainers do not publish artifacts built from unmerged branches.
