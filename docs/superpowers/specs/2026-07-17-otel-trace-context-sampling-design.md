# OTLP Trace Context and Sampling Design

**Status:** Implemented and verified on 2026-07-17

**Roadmap:** Stage 3 — production-pilot baseline / observability and quality gates

## Decision

ORAG will accept a W3C `traceparent` request header, make its valid remote span
context the parent of ORAG OpenTelemetry spans, and use a parent-based,
ratio-based head sampler for newly created traces. `X-Trace-ID` remains the
application trace key used by HTTP responses, structured logs and the
PostgreSQL trace store. The two identifiers intentionally have different
formats and purposes.

Sampling changes only optional OTLP export. ORAG continues to persist its
application RAG trace according to the existing trace-store policy, including
when an OTLP trace is not sampled. A malformed `traceparent` is ignored and
must neither reject a request nor affect its `X-Trace-ID` behavior.

## Configuration

`OTEL_TRACES_SAMPLER_ARG` is parsed as a decimal ratio in `[0,1]`. It defaults
to `1`, preserving current export behavior. A configured OTLP endpoint uses
`ParentBased(TraceIDRatioBased(ratio))`: valid sampled remote parents remain
sampled, valid unsampled remote parents remain unsampled, and only new root
traces use the configured ratio. Invalid values fail configuration loading
rather than silently changing telemetry volume.

`OTEL_SERVICE_NAME`, when non-empty, overrides the exported service name;
otherwise it is `orag`.

## Request and export flow

1. HTTP middleware reads `X-Trace-ID` exactly as today and adds it to the
   request context.
2. It extracts a valid W3C `traceparent` into the same context, then starts an
   `http.server` span. Graph node spans are children of that span.
3. The response keeps `X-Trace-ID` and adds the resulting W3C `traceparent`
   when an OTLP span context exists. No user content, tenant, prompt, document
   or model output is added to OTLP attributes.
4. The OTLP provider uses the configured sampler and batch exporter. Shutdown
   restores the previous global provider and propagator as existing tests
   require.

## Non-goals

- No metrics sampling or application-metric persistence change.
- No automatic external Collector deployment or vendor APM integration.
- No tail sampling in-process. Operators who need error/latency-aware retention
  configure it at their Collector; this preserves a bounded application path.

## Acceptance criteria

- A valid sampled remote `traceparent` becomes the parent of emitted ORAG
  spans; an invalid header is harmless.
- Ratio `0` suppresses new root exports while remote sampled parents are still
  exported; ratio `1` exports new roots.
- `X-Trace-ID` and PostgreSQL trace persistence keep their existing behavior.
- Invalid sampling configuration is rejected at load time.
- OTLP resource uses the configured service name and documentation describes
  the topology and Collector tail-sampling boundary.
