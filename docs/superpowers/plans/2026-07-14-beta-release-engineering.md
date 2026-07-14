# Beta Release Engineering Implementation Plan

**Goal:** Deliver reproducible API/Console containers, a complete local Compose stack, an idempotent no-key walkthrough, and a guarded multi-architecture GHCR release workflow for `v0.1.0-beta.1`.

**Architecture:** The API image contains `orag-api`, `oragctl`, and `orag-demo`; Compose runs migration as a one-shot job before API startup, serves the Console through Nginx with same-origin API proxying, and optionally runs the demo initializer. Tag-driven GitHub Actions build API and Console for `linux/amd64` and `linux/arm64` without moving `latest`.

## Task 1: Release metadata

- Add a public build-info package with development defaults and linker-injected version, commit, and build time.
- Add `GET /version` and contract tests.
- Make binaries expose the same release identity.

## Task 2: Container targets

- Expand the API Dockerfile into reusable build and runtime targets containing API, migration CLI, and demo binaries.
- Add a Console multi-stage Dockerfile and pinned Nginx configuration with same-origin API proxying and health checks.
- Add OCI label build arguments and non-root runtime users where practical.

## Task 3: Full Compose topology

- Add health-checked PostgreSQL and Qdrant services.
- Add one-shot `migrate`, then health-gated `api`, then `console`.
- Add a `demo` profile whose completion is required by the documented walkthrough command.
- Keep local demo mock selection explicit and document that the stack is not a production credential template.

## Task 4: Idempotent no-key walkthrough

- Add an `orag-demo` command that waits for readiness, logs in, discovers or creates a stable demo knowledge base, ingests content, queries with citations, looks up the trace, creates a dataset, and runs evaluation.
- Write a machine-readable JSON summary to a mounted volume and include the last completed step on failure.
- Add unit tests around the client workflow and repeatability behavior.

## Task 5: Release workflow and gates

- Add pull-request Compose smoke validation and tag-triggered release workflow.
- Verify the tag commit is contained in `origin/main`.
- Run Go, SDK, OpenAPI, Console, image, and Compose walkthrough gates.
- Publish multi-architecture API and Console images to GHCR with version/SHA tags, SBOM/provenance attestations, and a prerelease GitHub Release.
- Never publish or move `latest` for this prerelease.

## Verification

- `make test vet openapi-validate sdk-check`
- `npm --prefix console test -- --run && npm --prefix console run build`
- `docker compose -f deployments/docker-compose.yml config`
- `docker compose -f deployments/docker-compose.yml --profile demo up --build --wait`
- Repeat demo against the same volumes and compare successful summaries.
- `docker buildx build --platform linux/amd64,linux/arm64 --target api --load` is represented by per-platform CI or a registry-backed multi-platform build.
