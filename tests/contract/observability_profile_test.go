package contract_test

import (
	"os"
	"strings"
	"testing"
)

func TestPilotObservabilityProfile(t *testing.T) {
	checks := map[string][]string{
		"../../deployments/docker-compose.observability.yml": {
			"prometheus_data:/prometheus", "--storage.tsdb.retention.time=${ORAG_PROMETHEUS_RETENTION:-7d}",
			"ORAG_PILOT_TRACE_HEAD_SAMPLING_RATIO:-0.1", "otel-collector",
		},
		"../../deployments/prometheus/prometheus.yml": {
			"targets: [orag-api:8080]", "metrics_path: /metrics", "/etc/prometheus/alerts.yml",
		},
		"../../deployments/otel-collector/pilot.yml": {
			"tail_sampling", "retain-errors", "retain-slow", "sampling_percentage: 10", "exporters: [nop]",
		},
	}
	for path, phrases := range checks {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("read %s: %v", path, err)
			continue
		}
		for _, phrase := range phrases {
			if !strings.Contains(string(body), phrase) {
				t.Errorf("%s missing %q", path, phrase)
			}
		}
	}
}
