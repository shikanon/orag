package orag

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/shikanon/orag/internal/benchmark"
)

func TestRunMockPerformanceBaselineProducesValidObservedReport(t *testing.T) {
	opts := DefaultPerformanceBaselineOptions()
	opts.BuildRevision = "test-revision"
	raw, err := RunMockPerformanceBaseline(context.Background(), opts)
	if err != nil {
		t.Fatalf("RunMockPerformanceBaseline() error = %v", err)
	}
	report, err := benchmark.Parse(raw)
	if err != nil {
		t.Fatalf("benchmark.Parse() error = %v", err)
	}
	if report.Load.MeasuredRequests != 20 || report.Load.WarmupRequests != 10 || report.Load.Concurrency != 1 {
		t.Fatalf("report load = %#v", report.Load)
	}
	if report.Metrics.IngestionDocuments != len(mockPerformanceBaselineWorkload().Documents) || report.Metrics.QueryP95MS < report.Metrics.QueryP50MS {
		t.Fatalf("report metrics = %#v", report.Metrics)
	}
	if report.Metrics.ModelCalls != 33 || report.Metrics.CostUSD != 0 {
		t.Fatalf("model accounting = %#v", report.Metrics)
	}
}

func TestRunMockPerformanceBaselineRejectsInvalidOptionsAndCancellation(t *testing.T) {
	invalid := DefaultPerformanceBaselineOptions()
	invalid.BuildRevision = ""
	if _, err := RunMockPerformanceBaseline(context.Background(), invalid); err == nil {
		t.Fatal("RunMockPerformanceBaseline() accepted blank build revision")
	}
	invalid = DefaultPerformanceBaselineOptions()
	invalid.MeasuredRequests = 19
	if _, err := RunMockPerformanceBaseline(context.Background(), invalid); err == nil {
		t.Fatal("RunMockPerformanceBaseline() accepted fewer than 20 measured requests")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := RunMockPerformanceBaseline(ctx, DefaultPerformanceBaselineOptions()); err == nil {
		t.Fatal("RunMockPerformanceBaseline() accepted canceled context")
	}
}

func TestMockPerformanceBaselineWorkloadAndRuntimeFingerprintsAreStable(t *testing.T) {
	workloadRaw, err := json.Marshal(mockPerformanceBaselineWorkload())
	if err != nil {
		t.Fatal(err)
	}
	fingerprint := sha256Hex(workloadRaw)
	if len(fingerprint) != 64 || strings.Trim(fingerprint, "0123456789abcdef") != "" {
		t.Fatalf("workload fingerprint = %q", fingerprint)
	}
	if percentileMilliseconds([]int64{9, 1, 5, 3}, .5) != 3 || percentileMilliseconds([]int64{9, 1, 5, 3}, .95) != 9 {
		t.Fatal("percentile calculation is unstable")
	}
}
