package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shikanon/orag/internal/rag"
	"github.com/shikanon/orag/internal/storage/postgres"
)

func TestRunTraceLookupFound(t *testing.T) {
	createdAt := time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
	getter := fakeTraceGetter{
		trace: postgres.TraceRecord{
			ID:         "trace_1",
			TenantID:   "tenant_1",
			Profile:    rag.Profile("realtime"),
			LatencyMS:  42,
			CreatedAt:  createdAt,
			HasError:   true,
			ErrorCount: 1,
			NodeSpans: []postgres.TraceNodeSpan{
				{ID: "span_1", NodeName: "generate", LatencyMS: 42, Error: "llm timeout", CreatedAt: createdAt},
			},
		},
		found: true,
	}
	var out bytes.Buffer

	if err := runTraceLookup(context.Background(), getter, "trace_1", &out); err != nil {
		t.Fatalf("runTraceLookup() error = %v", err)
	}
	var got traceLookupResult
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal output: %v\n%s", err, out.String())
	}
	if !got.Found || got.Trace == nil || got.Trace.ID != "trace_1" || !got.Trace.HasError {
		t.Fatalf("runTraceLookup() output = %#v", got)
	}
	if got.TraceID != "" {
		t.Fatalf("found output should not duplicate trace_id at top level: %#v", got)
	}
}

func TestBenchmarkReportCmdVerifiesControlledReport(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.json")
	report := `{
  "schema_version":"orag.performance-baseline.v1",
  "id":"text-rag/mock-baseline-v1",
  "generated_at":"2026-07-17T00:00:00Z",
  "provenance":{"workload_id":"text-rag/1.0.0/benchmark/replay-v1","pack_tier":"benchmark","deterministic_mock":true,"dataset_fingerprint":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","runtime_environment_sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","build_revision":"test-revision"},
  "load":{"warmup_requests":10,"measured_requests":20,"concurrency":1},
  "metrics":{"ingestion_documents":2,"ingestion_duration_ms":500,"ingestion_throughput_docs_per_sec":4,"query_p50_ms":12,"query_p95_ms":20,"cache_hit_rate":0.5,"evaluation_duration_ms":90,"model_calls":8,"cost_usd":0}
}`
	if err := os.WriteFile(path, []byte(report), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := benchmarkReportCmd([]string{"--file", path}, &out); err != nil {
		t.Fatalf("benchmarkReportCmd() error = %v", err)
	}
	if !strings.Contains(out.String(), "verified text-rag/mock-baseline-v1 ") {
		t.Fatalf("benchmarkReportCmd() output = %q", out.String())
	}
}

func TestRunTraceLookupNotFound(t *testing.T) {
	var out bytes.Buffer

	if err := runTraceLookup(context.Background(), fakeTraceGetter{}, "missing_trace", &out); err != nil {
		t.Fatalf("runTraceLookup() error = %v", err)
	}
	var got traceLookupResult
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal output: %v\n%s", err, out.String())
	}
	if got.Found || got.Trace != nil || got.TraceID != "missing_trace" {
		t.Fatalf("runTraceLookup() output = %#v", got)
	}
}

func TestRunTraceList(t *testing.T) {
	createdAt := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	hasError := true
	reader := &fakeTraceReader{
		traces: []postgres.TraceRecord{
			{
				ID:         "trace_2",
				TenantID:   "tenant_1",
				Profile:    rag.Profile("accurate"),
				LatencyMS:  1200,
				CreatedAt:  createdAt,
				HasError:   true,
				ErrorCount: 1,
			},
		},
	}
	opts := traceOptions{
		Filter: postgres.TraceListFilter{
			TenantID: "tenant_1",
			Profile:  rag.Profile("accurate"),
			Since:    createdAt.Add(-time.Hour),
			Until:    createdAt.Add(time.Hour),
			HasError: &hasError,
			SlowMS:   1000,
			Limit:    20,
		},
	}
	var out bytes.Buffer

	if err := runTraceCommand(context.Background(), reader, opts, &out); err != nil {
		t.Fatalf("runTraceCommand() error = %v", err)
	}
	var got traceListResult
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal output: %v\n%s", err, out.String())
	}
	if len(got.Traces) != 1 || got.Traces[0].ID != "trace_2" {
		t.Fatalf("runTraceCommand() output = %#v", got)
	}
	if reader.listCalls != 1 || reader.statsCalls != 0 || reader.getCalls != 0 {
		t.Fatalf("unexpected calls: list=%d stats=%d get=%d", reader.listCalls, reader.statsCalls, reader.getCalls)
	}
	if reader.lastFilter.TenantID != "tenant_1" || reader.lastFilter.Profile != rag.Profile("accurate") || reader.lastFilter.SlowMS != 1000 || reader.lastFilter.Limit != 20 {
		t.Fatalf("unexpected filter: %#v", reader.lastFilter)
	}
	if reader.lastFilter.HasError == nil || !*reader.lastFilter.HasError {
		t.Fatalf("unexpected has_error filter: %#v", reader.lastFilter.HasError)
	}
}

func TestRunTraceStats(t *testing.T) {
	reader := &fakeTraceReader{
		stats: []postgres.TraceNodeStat{
			{NodeName: "retrieve", Count: 3, AvgLatencyMS: 12.5, P95LatencyMS: 20, P99LatencyMS: 24, ErrorCount: 1},
		},
	}
	var out bytes.Buffer

	if err := runTraceCommand(context.Background(), reader, traceOptions{Stats: true}, &out); err != nil {
		t.Fatalf("runTraceCommand() error = %v", err)
	}
	var got traceStatsResult
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal output: %v\n%s", err, out.String())
	}
	if len(got.NodeStats) != 1 || got.NodeStats[0].NodeName != "retrieve" || got.NodeStats[0].ErrorCount != 1 {
		t.Fatalf("runTraceCommand() output = %#v", got)
	}
	if reader.statsCalls != 1 || reader.listCalls != 0 || reader.getCalls != 0 {
		t.Fatalf("unexpected calls: list=%d stats=%d get=%d", reader.listCalls, reader.statsCalls, reader.getCalls)
	}
}

func TestParseTraceOptionsRejectsInvalidTime(t *testing.T) {
	_, err := parseTraceOptions([]string{"--since", "not-a-time"})
	if err == nil {
		t.Fatal("parseTraceOptions() error = nil")
	}
	if want := "invalid since"; !bytes.Contains([]byte(err.Error()), []byte(want)) {
		t.Fatalf("parseTraceOptions() error = %q, want to contain %q", err.Error(), want)
	}
}

func TestParseTraceOptionsHasErrorFalse(t *testing.T) {
	opts, err := parseTraceOptions([]string{"--has-error=false"})
	if err != nil {
		t.Fatalf("parseTraceOptions() error = %v", err)
	}
	if opts.Filter.HasError == nil || *opts.Filter.HasError {
		t.Fatalf("parseTraceOptions() has_error = %#v, want false", opts.Filter.HasError)
	}
}

func TestGenerateAgentArtifactsCmdWritesMCPAndSkillOutputs(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer

	err := generateAgentArtifactsCmd([]string{
		"--openapi", filepath.Join("..", "..", "api", "openapi.yaml"),
		"--out", dir,
	}, &out)
	if err != nil {
		t.Fatalf("generateAgentArtifactsCmd() error = %v", err)
	}

	for _, path := range []string{
		"agent/mcp/openapi-facet.json",
		"agent/mcp/tools/ralph-loop.json",
		"agent/mcp/tools/orag-self-check.json",
		"agent/skills/codex/orag-self-check/SKILL.md",
		"agent/skills/claude-code/orag-self-diagnose/SKILL.md",
		"agent/skills/trae/orag-self-ops/SKILL.md",
	} {
		if !strings.Contains(out.String(), "generated ") || !strings.Contains(out.String(), path) {
			t.Fatalf("command output missing %s\n%s", path, out.String())
		}
		body, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(path)))
		if err != nil {
			t.Fatalf("read generated %s: %v", path, err)
		}
		if !strings.Contains(string(body), "orag.capabilities.v1") && !strings.Contains(string(body), "ralph_loop_run") {
			t.Fatalf("generated %s missing expected content\n%s", path, string(body))
		}
	}

	out.Reset()
	err = generateAgentArtifactsCmd([]string{
		"--openapi", filepath.Join("..", "..", "api", "openapi.yaml"),
		"--out", dir,
		"--check",
	}, &out)
	if err != nil {
		t.Fatalf("generateAgentArtifactsCmd(--check) error = %v", err)
	}
	if !strings.Contains(out.String(), "checked mcp agent/mcp/tools/ralph-loop.json") ||
		!strings.Contains(out.String(), "checked openapi-facet agent/mcp/openapi-facet.json") {
		t.Fatalf("check output missing MCP artifact\n%s", out.String())
	}
}

type fakeTraceGetter struct {
	trace postgres.TraceRecord
	found bool
	err   error
}

func (f fakeTraceGetter) GetTrace(context.Context, string) (postgres.TraceRecord, bool, error) {
	return f.trace, f.found, f.err
}

type fakeTraceReader struct {
	trace      postgres.TraceRecord
	found      bool
	traces     []postgres.TraceRecord
	stats      []postgres.TraceNodeStat
	lastFilter postgres.TraceListFilter
	getCalls   int
	listCalls  int
	statsCalls int
}

func (f *fakeTraceReader) GetTrace(context.Context, string) (postgres.TraceRecord, bool, error) {
	f.getCalls++
	return f.trace, f.found, nil
}

func (f *fakeTraceReader) ListTraces(_ context.Context, filter postgres.TraceListFilter) ([]postgres.TraceRecord, error) {
	f.listCalls++
	f.lastFilter = filter
	return f.traces, nil
}

func (f *fakeTraceReader) TraceNodeStats(_ context.Context, filter postgres.TraceListFilter) ([]postgres.TraceNodeStat, error) {
	f.statsCalls++
	f.lastFilter = filter
	return f.stats, nil
}
