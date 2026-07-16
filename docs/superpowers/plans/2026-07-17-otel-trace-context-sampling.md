# OTLP Trace Context and Sampling Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make optional OTLP tracing interoperable across services and control its export volume without changing ORAG application-trace persistence.

**Architecture:** Add validated observability configuration, construct the SDK provider with a parent-based ratio sampler and service resource, and make HTTP middleware extract remote W3C context before opening the request span. The existing `X-Trace-ID` context value and trace store remain independent.

**Tech Stack:** Go 1.26, OpenTelemetry Go SDK, Hertz, Go unit tests.

## Global Constraints

- `OTEL_TRACES_SAMPLER_ARG` is a decimal ratio in `[0,1]`, default `1`.
- OTLP attributes must not contain tenant, prompt, document, query, model output, or raw error text.
- Sampling affects only optional OTLP exports; PostgreSQL application traces remain unchanged.
- Keep global tracer and propagator restoration safe and idempotent.

---

### Task 1: Add validated trace-export configuration

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

**Interfaces:**
- Produces: `ObservabilityConfig.OTLPTraceSampleRatio float64` and `ObservabilityConfig.OTLPServiceName string`.

- [x] **Step 1: Write failing configuration tests**

```go
t.Setenv("OTEL_TRACES_SAMPLER_ARG", "0.25")
t.Setenv("OTEL_SERVICE_NAME", "orag-pilot")
cfg, err := Load()
if err != nil { t.Fatal(err) }
if cfg.Observability.OTLPTraceSampleRatio != 0.25 { t.Fatal("ratio not loaded") }
if cfg.Observability.OTLPServiceName != "orag-pilot" { t.Fatal("service name not loaded") }
```

Also assert malformed, negative and greater-than-one ratios return an error.

- [x] **Step 2: Run the focused test and verify failure**

Run: `GOTOOLCHAIN=go1.26.5 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./internal/config -run TestLoadOTLPTraceSampling -v`

- [x] **Step 3: Implement parsing and validation**

Use `strconv.ParseFloat(strings.TrimSpace(...), 64)`, reject non-finite and
out-of-range values, use `1` when the variable is blank, and read
`OTEL_SERVICE_NAME` with `orag` as the default.

- [x] **Step 4: Run the focused test and verify it passes**

Run: `GOTOOLCHAIN=go1.26.5 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./internal/config -run TestLoadOTLPTraceSampling -v`

### Task 2: Configure parent-based sampled OTLP providers

**Files:**
- Modify: `internal/observability/otlp.go`
- Modify: `internal/observability/tracing_test.go`
- Modify: `internal/app/app.go`

**Interfaces:**
- Changes: `ConfigureOTLP(ctx, endpoint, sampleRatio, serviceName)`.
- Produces: globally installed `propagation.TraceContext{}` and a provider using `trace.ParentBased(trace.TraceIDRatioBased(sampleRatio))`.

- [x] **Step 1: Write failing exporter tests**

Create a local HTTP collector and assert ratio-zero root spans do not send an
export request, ratio-one root spans do, and a sampled remote parent is exported
even at ratio zero. Also assert shutdown restores the previous propagator.

- [x] **Step 2: Run the observability tests and verify failure**

Run: `GOTOOLCHAIN=go1.26.5 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./internal/observability -run 'TestConfigureOTLP.*(Sampling|Propagator)' -v`

- [x] **Step 3: Implement provider construction**

Build a resource with `service.name`, install `propagation.TraceContext{}`, and
pass `trace.WithSampler(trace.ParentBased(trace.TraceIDRatioBased(sampleRatio)))`
to `trace.NewTracerProvider`. Restore provider, propagator, and ORAG tracer in
the idempotent shutdown closure. Pass the new config fields from `app.New`.

- [x] **Step 4: Run observability tests and verify pass**

Run: `GOTOOLCHAIN=go1.26.5 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./internal/observability -v`

### Task 3: Link incoming HTTP requests to W3C context

**Files:**
- Modify: `internal/observability/tracing.go`
- Modify: `internal/http/middleware.go`
- Modify: `internal/http/router_test.go`

**Interfaces:**
- Produces: `ExtractTraceContext(ctx, traceparent string) context.Context` and `TraceparentFromContext(ctx) string`.

- [x] **Step 1: Write failing HTTP tests**

Send a request with a sampled `traceparent` and assert its response has a valid
new `traceparent` sharing the submitted trace ID, while `X-Trace-ID` retains
its independently supplied value. Send malformed input and assert the request
still succeeds with its `X-Trace-ID`.

- [x] **Step 2: Run the focused HTTP test and verify failure**

Run: `GOTOOLCHAIN=go1.26.5 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./internal/http -run TestTraceMiddlewareW3CTraceparent -v`

- [x] **Step 3: Implement safe extraction and response injection**

Use OpenTelemetry `propagation.TraceContext` with a one-value carrier. Start
and end an `http.server` ORAG span around `c.Next`, and inject its span context
to the response only when valid. Preserve all current `X-Trace-ID` behavior.

- [x] **Step 4: Run focused HTTP test and full package tests**

Run: `GOTOOLCHAIN=go1.26.5 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./internal/http ./internal/observability ./internal/config -v`

### Task 4: Document the topology and validate repository gates

**Files:**
- Modify: `docs/operations/README.md`
- Modify: `docs/operations/grafana.md`
- Modify: `docs/architecture/README.md`
- Modify: `ROADMAP.md`
- Modify: `ROADMAP_EN.md`

- [x] **Step 1: Document variables and boundaries**

State the default ratio, remote-parent behavior, W3C propagation, service-name
override, and that Collector tail sampling owns error/latency retention.

- [x] **Step 2: Update roadmap progress without claiming metric persistence**

Mark trace sampling and cross-service propagation complete while retaining
metrics persistence as an operator-owned/open item.

- [x] **Step 3: Run all relevant gates**

Run: `make agent-gate`

Expected: all command, contract, SDK, unit and vet checks pass.

- [x] **Step 4: Build hosted documentation**

Run: `./scripts/build-docs-site.sh`

Expected: `Built hosted documentation in /Users/bytedance/Documents/orag/_site`.
