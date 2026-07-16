package contract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHostedDocumentationAssets(t *testing.T) {
	required := map[string][]string{
		"../../docs-site/index.html": {
			"Build RAG systems",
			"you can measure.",
			"make demo",
			"api.html",
			"grafana.html",
			"screenshots/api-reference.png",
			"screenshots/walkthrough.gif",
		},
		"../../docs-site/api.html": {
			"SwaggerUIBundle",
			"./openapi.yaml",
			"Try it out",
		},
		"../../docs-site/grafana.html": {
			"ORAG Grafana dashboard",
			"orag-overview.json",
			"Low-cardinality by design.",
		},
		"../../.github/workflows/docs.yml": {
			"actions/configure-pages@",
			"actions/upload-pages-artifact@",
			"actions/deploy-pages@",
			"api/openapi.yaml",
		},
		"../../scripts/capture-docs-assets.sh": {
			"api-reference.png",
			"hosted-docs-home.png",
			"walkthrough.gif",
		},
		"../../scripts/build-docs-site.sh": {
			"docs-site",
			"api/openapi.yaml",
			"api/swagger-ui/swagger-ui-bundle.js",
		},
	}

	for path, fragments := range required {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("read %s: %v", path, err)
			continue
		}
		for _, fragment := range fragments {
			if !strings.Contains(string(content), fragment) {
				t.Errorf("%s missing %q", path, fragment)
			}
		}
	}

	for _, name := range []string{"api-reference.png", "hosted-docs-home.png", "walkthrough.gif"} {
		path := filepath.Join("../../docs-site/screenshots", name)
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("stat %s: %v", path, err)
			continue
		}
		if info.Size() < 1_000 {
			t.Errorf("%s is too small to be a real rendered asset: %d bytes", path, info.Size())
		}
	}
}
