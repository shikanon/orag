# Pilot Observability Profile Design

**Roadmap:** Phase 3 — metrics persistence and Collector retention/tail-sampling preparation

## Problem

ORAG exports Prometheus and optional OTLP telemetry, but the default Compose
stack intentionally has no persistent metrics store or Collector policy. That
is safe for a local walkthrough but leaves an operator without a versioned,
reviewable starting point for a controlled pilot.

## Decision

Add an optional Compose overlay rather than changing the default stack. The
overlay starts Prometheus and an OpenTelemetry Collector, stores Prometheus
data in a named volume, and uses explicit pilot defaults: seven-day retention,
bounded disk use, a 10% root head-sampling ratio in ORAG, and Collector tail
sampling that always retains error spans and retains a small latency sample.

The Collector only receives OTLP from the internal Compose network. Prometheus
scrapes `orag-api:8080/metrics` and loads the existing alert rules. The
Collector's debug exporter is deliberately omitted so prompt/document/query
data cannot appear in logs; the profile is a metrics/traces transport baseline,
not a telemetry backend or a production capacity claim.

## Verification and limits

`docker compose config` will validate the overlay with the normal stack. A
contract test pins the profile's persistence, scrape, retention, sampling and
privacy boundaries. Operators must set retention, quotas and sampling from
their actual workload before production. This profile does not claim that
pilot-capacity calibration, external trace retention, or production SLOs are
complete.
