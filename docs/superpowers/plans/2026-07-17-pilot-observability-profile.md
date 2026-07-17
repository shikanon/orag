# Pilot Observability Profile Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship a safe, optional, persistent Prometheus and OTLP Collector reference profile for controlled ORAG pilots.

**Architecture:** A Compose overlay joins the existing API network without changing default services. Prometheus persists only metrics, while the Collector applies explicit tail policies and forwards no debug output.

**Tech Stack:** Docker Compose, Prometheus, OpenTelemetry Collector Contrib, existing Go contract tests and Markdown operations docs.

## Global Constraints

- Default `deployments/docker-compose.yml` remains unchanged in behavior.
- No query, prompt, document, tenant, trace ID, or model output is emitted as a metric label or Collector debug log.
- Reference retention and sampling values are explicitly non-production defaults.

---

### Task 1: Add versioned profile configuration

**Files:**
- Create: `deployments/docker-compose.observability.yml`
- Create: `deployments/prometheus/prometheus.yml`
- Create: `deployments/otel-collector/pilot.yml`

- [ ] Define Prometheus scrape, alert loading, seven-day retention and a named data volume.
- [ ] Define an OTLP-only Collector with error/latency tail policy and no debug exporter.

### Task 2: Pin behavior with a contract test

**Files:**
- Create: `tests/contract/observability_profile_test.go`

- [ ] Assert overlay, scrape target, persistent volume, retention, tail policy and privacy boundary strings.
- [ ] Run `go test ./tests/contract -run TestPilotObservabilityProfile -count=1`.

### Task 3: Document activation and scope

**Files:**
- Modify: `docs/operations/grafana.md`
- Modify: `docs/operations/README.md`
- Modify: `ROADMAP.md`
- Modify: `ROADMAP_EN.md`

- [ ] Document `docker compose -f deployments/docker-compose.yml -f deployments/docker-compose.observability.yml up -d` and required calibration before production.
- [ ] Validate via `docker compose ... config`, contract test, docs-site build and `git diff --check`.
