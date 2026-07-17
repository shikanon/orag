# Grafana dashboard

ORAG ships an importable Grafana overview dashboard at [`../../deployments/grafana/dashboards/orag-overview.json`](../../deployments/grafana/dashboards/orag-overview.json). It reads the documented Prometheus series from `GET /metrics`; it does not add another API, store application data, or trigger remediation.

## Import

1. Configure a Prometheus datasource that scrapes the ORAG API's `/metrics` endpoint.
2. In Grafana, open **Dashboards** → **New** → **Import**, then upload `orag-overview.json`.
3. Select the Prometheus datasource for the `Datasource` variable and save the dashboard. The dashboard deliberately has no hard-coded datasource UID.

The dashboard contains these operator views:

| Panel | Prometheus series | Incident signal |
| --- | --- | --- |
| API availability | `orag_up` | The process can render its metrics endpoint. It does not prove dependencies are ready. |
| HTTP request and 5xx rate | `orag_http_requests_total`, `orag_http_errors_total` | A server-error increase at the HTTP boundary. |
| HTTP latency p95 | `orag_http_request_latency_ms_bucket` | End-to-end HTTP latency trend. |
| RAG outcomes and cache status | `orag_rag_queries_total` | Query success/error and semantic-cache behavior. |
| RAG latency p95 | `orag_rag_query_latency_ms_bucket` | RAG-path latency trend. |
| Dependency readiness checks | `orag_dependency_checks_total` | Results produced when `/readyz` is called. |
| Trace-store outcomes | `orag_trace_store_total` | Whether diagnostic trace evidence is being persisted. |

## Alert correlation and safe response

Importing this dashboard is complementary to the baseline rules in [`../../deployments/prometheus/alerts.yml`](../../deployments/prometheus/alerts.yml). Use an alert to select the relevant panel, then follow the matching read-only runbook in [`troubleshooting.md`](./troubleshooting.md). The default path is:

```text
alert -> dashboard and trace evidence -> self-check -> diagnose -> runbook -> dry-run plan -> explicit human approval
```

The dashboard does not authorize, enqueue, or execute a repair. Any self-ops apply action remains separately approved, audited, and reversible.

## Data and retention boundary

The built-in metrics are process-local counters and histograms. They reset on API restart; Prometheus retention is configured by the operator and is not managed by ORAG. Dashboard queries only use controlled labels such as route, status class, profile, cache status, dependency state, and outcome. They do not include trace IDs, tenants, user input, prompts, documents, model output, or raw errors.

OTLP trace and core metrics export are optional through `OTEL_EXPORTER_OTLP_ENDPOINT` and `OTEL_EXPORTER_OTLP_METRICS_ENDPOINT`. HTTP trace export accepts and returns W3C `traceparent`; `OTEL_TRACES_SAMPLER_ARG` sets the parent-based root sampling ratio (default `1`) and `OTEL_SERVICE_NAME` defaults to `orag`. Prometheus remains the complete local metrics surface and Grafana datasource for this release; metrics persistence and Collector retention/tail-sampling policy remain operator-owned.

## Controlled pilot profile

The optional [`docker-compose.observability.yml`](../../deployments/docker-compose.observability.yml)
overlay provides a reviewable starting point for a controlled pilot. It adds a
Prometheus named volume, a seven-day/10 GB default retention ceiling, existing
alert rules, and an internal-only OTLP Collector:

```bash
docker compose \
  -f deployments/docker-compose.yml \
  -f deployments/docker-compose.observability.yml \
  up -d
```

The overlay sets a 10% root head-sampling ratio and the Collector's reference
tail policies retain error spans, spans at or above 5 seconds, and a 10%
sample. Its default `nop` exporter deliberately drops received OTLP telemetry:
it proves transport/policy wiring without writing traces to logs or an
unreviewed backend. Before retaining traces, replace that exporter with an
authenticated, access-controlled backend and calibrate quotas, retention and
sampling from measured pilot traffic. These defaults are not production SLO or
capacity claims.
