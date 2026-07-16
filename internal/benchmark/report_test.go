package benchmark

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseAcceptsControlledReport(t *testing.T) {
	report := validReport()
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if !Comparable(report, got) || Fingerprint(got) == "" {
		t.Fatalf("parsed report is not comparable or fingerprinted: %#v", got)
	}
}

func TestValidateRejectsUncontrolledOrInconsistentReport(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Report)
	}{
		{"real provider", func(r *Report) { r.Provenance.DeterministicMock = false }},
		{"too few requests", func(r *Report) { r.Load.MeasuredRequests = 19 }},
		{"bad percentile", func(r *Report) { r.Metrics.QueryP95MS = r.Metrics.QueryP50MS - 1 }},
		{"wrong throughput", func(r *Report) { r.Metrics.IngestionThroughputDocsSec++ }},
		{"unbounded cache rate", func(r *Report) { r.Metrics.CacheHitRate = 1.1 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := validReport()
			tt.edit(&report)
			if err := Validate(report); err == nil {
				t.Fatal("Validate() error = nil")
			}
		})
	}
}

func TestComparableRequiresSameControlledInputs(t *testing.T) {
	left, right := validReport(), validReport()
	right.Provenance.BuildRevision = "another-revision"
	if Comparable(left, right) {
		t.Fatal("Comparable() = true for different build revision")
	}
}

func TestParseRejectsUnknownAndMultipleJSONValues(t *testing.T) {
	raw, err := json.Marshal(validReport())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(append(raw, []byte(" {}")...)); err == nil {
		t.Fatal("Parse() accepted multiple JSON values")
	}
	if _, err := Parse([]byte(strings.Replace(string(raw), "}", `,"unexpected":true}`, 1))); err == nil {
		t.Fatal("Parse() accepted unknown field")
	}
}

func validReport() Report {
	return Report{
		SchemaVersion: SchemaVersion,
		ID:            "text-rag/mock-baseline-v1",
		GeneratedAt:   "2026-07-17T00:00:00Z",
		Provenance: Provenance{
			WorkloadID:               "text-rag/1.0.0/benchmark/replay-v1",
			PackTier:                 "benchmark",
			DeterministicMock:        true,
			DatasetFingerprint:       strings.Repeat("a", 64),
			RuntimeEnvironmentSHA256: strings.Repeat("b", 64),
			BuildRevision:            "test-revision",
		},
		Load:    Load{WarmupRequests: 10, MeasuredRequests: 20, Concurrency: 1},
		Metrics: Metrics{IngestionDocuments: 2, IngestionDurationMS: 500, IngestionThroughputDocsSec: 4, QueryP50MS: 12, QueryP95MS: 20, CacheHitRate: 0.5, EvaluationDurationMS: 90, ModelCalls: 8, CostUSD: 0},
	}
}
