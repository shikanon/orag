// Package benchmark defines the portable, verifiable performance-baseline report.
package benchmark

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"
	"time"
)

const SchemaVersion = "orag.performance-baseline.v1"

type Report struct {
	SchemaVersion string     `json:"schema_version"`
	ID            string     `json:"id"`
	GeneratedAt   string     `json:"generated_at"`
	Provenance    Provenance `json:"provenance"`
	Load          Load       `json:"load"`
	Metrics       Metrics    `json:"metrics"`
}

type Provenance struct {
	WorkloadID               string `json:"workload_id"`
	PackTier                 string `json:"pack_tier"`
	DeterministicMock        bool   `json:"deterministic_mock"`
	DatasetFingerprint       string `json:"dataset_fingerprint"`
	RuntimeEnvironmentSHA256 string `json:"runtime_environment_sha256"`
	BuildRevision            string `json:"build_revision"`
}

type Load struct {
	WarmupRequests   int `json:"warmup_requests"`
	MeasuredRequests int `json:"measured_requests"`
	Concurrency      int `json:"concurrency"`
}

type Metrics struct {
	IngestionDocuments         int     `json:"ingestion_documents"`
	IngestionDurationMS        int64   `json:"ingestion_duration_ms"`
	IngestionThroughputDocsSec float64 `json:"ingestion_throughput_docs_per_sec"`
	QueryP50MS                 int64   `json:"query_p50_ms"`
	QueryP95MS                 int64   `json:"query_p95_ms"`
	CacheHitRate               float64 `json:"cache_hit_rate"`
	EvaluationDurationMS       int64   `json:"evaluation_duration_ms"`
	ModelCalls                 int     `json:"model_calls"`
	CostUSD                    float64 `json:"cost_usd"`
}

// Parse validates one JSON value and returns a report that can be compared only
// when all provenance dimensions match.
func Parse(raw []byte) (Report, error) {
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	var report Report
	if err := decoder.Decode(&report); err != nil {
		return Report{}, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return Report{}, fmt.Errorf("report contains multiple JSON values")
	}
	if err := Validate(report); err != nil {
		return Report{}, err
	}
	return report, nil
}

func Validate(report Report) error {
	if report.SchemaVersion != SchemaVersion || strings.TrimSpace(report.ID) == "" {
		return fmt.Errorf("report identity is invalid")
	}
	if _, err := time.Parse(time.RFC3339, report.GeneratedAt); err != nil {
		return fmt.Errorf("generated_at is invalid")
	}
	p := report.Provenance
	if strings.TrimSpace(p.WorkloadID) == "" || p.PackTier != "benchmark" || !p.DeterministicMock || !isSHA256(p.DatasetFingerprint) || !isSHA256(p.RuntimeEnvironmentSHA256) || strings.TrimSpace(p.BuildRevision) == "" {
		return fmt.Errorf("provenance is not a controlled benchmark run")
	}
	if report.Load.WarmupRequests < 0 || report.Load.MeasuredRequests < 20 || report.Load.Concurrency < 1 {
		return fmt.Errorf("load is invalid")
	}
	m := report.Metrics
	if m.IngestionDocuments < 1 || m.IngestionDurationMS < 1 || m.QueryP50MS < 0 || m.QueryP95MS < m.QueryP50MS || m.EvaluationDurationMS < 0 || m.ModelCalls < 0 {
		return fmt.Errorf("metric counts or durations are invalid")
	}
	for _, value := range []float64{m.IngestionThroughputDocsSec, m.CacheHitRate, m.CostUSD} {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			return fmt.Errorf("metric value is invalid")
		}
	}
	if m.CacheHitRate > 1 {
		return fmt.Errorf("cache_hit_rate must be between 0 and 1")
	}
	expectedThroughput := float64(m.IngestionDocuments) * 1000 / float64(m.IngestionDurationMS)
	if math.Abs(expectedThroughput-m.IngestionThroughputDocsSec) > 0.000001 {
		return fmt.Errorf("ingestion throughput does not match document count and duration")
	}
	return nil
}

func Comparable(left, right Report) bool {
	return left.SchemaVersion == right.SchemaVersion &&
		left.Provenance.WorkloadID == right.Provenance.WorkloadID &&
		left.Provenance.PackTier == right.Provenance.PackTier &&
		left.Provenance.DeterministicMock == right.Provenance.DeterministicMock &&
		left.Provenance.DatasetFingerprint == right.Provenance.DatasetFingerprint &&
		left.Provenance.RuntimeEnvironmentSHA256 == right.Provenance.RuntimeEnvironmentSHA256 &&
		left.Provenance.BuildRevision == right.Provenance.BuildRevision &&
		left.Load.WarmupRequests == right.Load.WarmupRequests &&
		left.Load.MeasuredRequests == right.Load.MeasuredRequests &&
		left.Load.Concurrency == right.Load.Concurrency
}

func Fingerprint(report Report) string {
	raw, err := json.Marshal(report)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func isSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
