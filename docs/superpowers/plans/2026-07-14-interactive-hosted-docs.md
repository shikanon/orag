# Interactive and Hosted Documentation Implementation Plan

**Goal:** Replace the placeholder API page with a self-contained interactive OpenAPI UI and publish a coherent GitHub Pages documentation experience with reproducible, real screenshots.

## 1. Lock the documentation contract

- Add HTTP tests for `/docs`, `/openapi.yaml`, and vendored UI assets.
- Add repository contract tests for the Pages workflow, site entry points, OpenAPI copy step, and required screenshots/GIF.
- Keep `api/openapi.yaml` as the only authored API specification.

## 2. Serve interactive API documentation

- Embed `api/openapi.yaml` and the pinned Swagger UI distribution in a small `api` package.
- Serve the specification from `/openapi.yaml` and the vendored assets from `/docs/assets/*`.
- Configure Swagger UI with deep links, persistent authorization, filtering, and Try it out.

## 3. Build the hosted documentation site

- Create a lightweight static site under `docs-site/` with quickstart, evaluation-first positioning, SDK guidance, maturity labels, and API reference.
- Add a GitHub Pages workflow that packages the site, the canonical OpenAPI file, and the vendored UI assets.
- Link the repository README and docs index to the hosted site and interactive local API reference.

## 4. Capture real documentation assets

- Start the real mock Compose stack and local hosted-docs preview.
- Capture fixed-viewport screenshots from the rendered site and live `/docs` page.
- Produce a short GIF from real browser frames and store the capture script with the assets.

## 5. Validate and publish

- Run Go/contract tests, frontend checks, Docker Compose validation, and a browser smoke test.
- Push a focused branch, open a PR, wait for CI, merge, and verify GitHub Pages.
