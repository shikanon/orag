package http

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/shikanon/orag/internal/observability"
	"go.opentelemetry.io/otel/trace"
)

func TestTraceMiddlewareW3CTraceparent(t *testing.T) {
	collector := httptest.NewServer(nil)
	defer collector.Close()
	closeOTLP, err := observability.ConfigureOTLP(context.Background(), collector.URL, 1, "orag-test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = closeOTLP() }()

	h, closeServer := newTestHertz(t)
	defer closeServer()
	const incoming = "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01"
	resp := performJSONWithHeaders(h, "GET", "/healthz", "", "",
		ut.Header{Key: observability.TraceIDHeader, Value: "trace_application"},
		ut.Header{Key: "traceparent", Value: incoming},
	)
	if resp.Code != 200 {
		t.Fatalf("status = %d, want 200", resp.Code)
	}
	if resp.TraceIDHeader != "trace_application" {
		t.Fatalf("X-Trace-ID = %q, want trace_application", resp.TraceIDHeader)
	}
	spanContext := trace.SpanContextFromContext(observability.ExtractTraceContext(context.Background(), resp.Traceparent))
	if !spanContext.IsValid() {
		t.Fatalf("response traceparent = %q, want valid W3C context", resp.Traceparent)
	}
	if got, want := spanContext.TraceID().String(), "0123456789abcdef0123456789abcdef"; got != want {
		t.Fatalf("response trace ID = %q, want %q", got, want)
	}
	if got, want := spanContext.SpanID().String(), "0123456789abcdef"; got == want {
		t.Fatalf("response span ID = %q, want a new child span", got)
	}

	malformed := performJSONWithHeaders(h, "GET", "/healthz", "", "",
		ut.Header{Key: observability.TraceIDHeader, Value: "trace_malformed"},
		ut.Header{Key: "traceparent", Value: "invalid"},
	)
	if malformed.Code != 200 || malformed.TraceIDHeader != "trace_malformed" {
		t.Fatalf("malformed traceparent response = %#v", malformed)
	}
}
