package contract_test

import (
	"os"
	"strings"
	"testing"
)

func TestPublicPerformanceBaselineEvidence(t *testing.T) {
	const root = "../../docs-site/benchmarks/2026-07-17-darwin-arm64-main-75eda8f"
	checks := map[string][]string{
		"../../Makefile": {
			"performance-baseline-evidence-verify:",
			"verify-performance-baseline-evidence.sh",
		},
		"../../scripts/capture-performance-baseline-evidence.sh": {
			"benchmark-run",
			"environment.json",
			"SHA256SUMS",
		},
		"../../scripts/verify-performance-baseline-evidence.sh": {
			"benchmark-report",
			"manifest.json",
			"deterministic_mock",
		},
		root + "/report.json": {
			`"schema_version": "orag.performance-baseline.v1"`,
			`"deterministic_mock": true`,
			`"build_revision": "75eda8f80787d205e16e4ff7f65096bcd8926888"`,
		},
		root + "/environment.json": {
			`"schema_version": "orag.performance-baseline-environment.v1"`,
			`"machine":`,
			`"runner_command":`,
		},
		root + "/manifest.json": {
			`"schema_version": "orag.performance-baseline-evidence.v1"`,
			`"id": "2026-07-17-darwin-arm64-main-75eda8f"`,
		},
		root + "/SHA256SUMS": {"report.json", "environment.json", "manifest.json"},
		"../../docs/benchmarks/performance-baseline-contract.md": {
			"performance-baseline-evidence-verify",
			"公开基线证据",
			"不得写成生产吞吐或跨硬件结论",
		},
		"../../docs-site/performance-baseline.html": {
			"2026-07-17-darwin-arm64-main-75eda8f",
			"local regression evidence",
			"not a production",
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
