# Grafana Observability Package Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Provide a versioned, importable Grafana dashboard for existing low-cardinality Prometheus metrics and make its operational contract verifiable in CI.

**Architecture:** Grafana reads the existing `/metrics` Prometheus series. A repository JSON dashboard contains panels only for documented metrics, and a Go contract test rejects missing required panels or unsupported metric references.

**Tech Stack:** Grafana dashboard JSON, PromQL, Go contract tests, existing Prometheus alert rules, hosted Markdown documentation.

## Global Constraints

- Keep Prometheus labels low cardinality; never add query, prompt, document, tenant, user, or trace identifiers.
- Dashboard import must use a selected Prometheus datasource; no datasource UID is hard-coded.
- PromQL must only use metrics emitted by `internal/observability.Metrics`.
- The dashboard is observational only and cannot trigger a write action.

---

### Task 1: Define the importable dashboard contract

**Files:**

- Create: `deployments/grafana/dashboards/orag-overview.json`
- Test: `tests/contract/grafana_dashboard_test.go`

**Interfaces:**

- Consumes: `orag_up`, `orag_http_requests_total`, `orag_http_request_latency_ms_bucket`, `orag_rag_queries_total`, `orag_rag_query_latency_ms_bucket`, `orag_dependency_checks_total`, and `orag_trace_store_total`.
- Produces: A Grafana schema-v41 dashboard with datasource variables and panels for availability, request/error rate, HTTP latency, RAG outcomes/cache, RAG p95, dependencies, and trace-store failures.

- [ ] Write a parser-backed contract test that checks dashboard schema, datasource variable, panel titles, and documented metric references.
- [ ] Add `orag-overview.json` with `${datasource}` and `$__rate_interval`; use `histogram_quantile` only over documented histogram buckets.
- [ ] Run `GOTOOLCHAIN=go1.26.5 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./tests/contract -run TestGrafanaDashboard -v`; expect PASS.
- [ ] Commit the dashboard and its contract test.

### Task 2: Publish the operator workflow

**Files:**

- Create: `docs/operations/grafana.md`
- Modify: `docs/operations/README.md`, `docs/operations.md`, `docs/architecture/README.md`
- Modify: `ROADMAP.md`, `ROADMAP_EN.md`, `CHANGELOG.md`

**Interfaces:**

- Consumes: `deployments/grafana/dashboards/orag-overview.json`, `deployments/prometheus/alerts.yml`, and `/metrics`.
- Produces: Import instructions, datasource configuration, retention boundary, dashboard-to-alert mapping, and truthful Roadmap progress.

- [ ] Explain datasource setup, dashboard import, alert correlation, process-local retention, sensitive-label boundary, and human-authorized remediation.
- [ ] Link the guide from operations docs and state that OTel metrics export, persisted metrics, sampling, and cross-service topology remain pending.
- [ ] Run `./scripts/build-docs-site.sh` and `GOTOOLCHAIN=go1.26.5 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./tests/contract -run 'Test(GrafanaDashboard|DocumentationSite)' -v`; expect PASS.
- [ ] Commit documentation and Roadmap evidence.
