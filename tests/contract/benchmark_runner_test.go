package contract_test

import (
	"os"
	"strings"
	"testing"
)

func TestPerformanceBaselineRunnerIsDocumented(t *testing.T) {
	checks := map[string][]string{
		"../../Makefile": {"benchmark-report-run:", "benchmark-run --output", "benchmark-report-verify:"},
		"../../docs/benchmarks/performance-baseline-contract.md": {"benchmark-report-run", "MockConfig", "跨硬件"},
		"../../docs-site/performance-baseline.html":              {"benchmark-report-run", "public Go SDK", "cross-hardware"},
		"../../docs/sdk/README.md":                               {"RunMockPerformanceBaseline", "benchmark-report-run"},
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
