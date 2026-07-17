package contract_test

import (
	"os"
	"strings"
	"testing"
)

func TestProductionConfigurationDocumentation(t *testing.T) {
	checks := map[string][]string{
		"../../.env.example": {
			"ORAG_ENV=development",
			"ORAG_ENV=production",
			"mock object storage",
		},
		"../../docs/operations/server-deployment.md": {
			"ORAG_ENV=production",
			"explicit startup guard",
			"It never prints secret values",
		},
		"../../docs/operations/README.md": {
			"ORAG_ENV=production",
			"不会打印 secret 值",
		},
		"../../internal/config/config.go": {
			"ORAG_ENV must be development or production",
			"production configuration forbids",
			"safeProductionPublicBaseURL",
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
