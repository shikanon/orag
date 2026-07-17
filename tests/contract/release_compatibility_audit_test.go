package contract_test

import (
	"os"
	"strings"
	"testing"
)

func TestReleaseCompatibilityAuditIsWired(t *testing.T) {
	checks := map[string][]string{
		"../../Makefile":                      {"compatibility-audit:", "COMPATIBILITY_BASE must name the previous published tag", "oragctl compatibility-audit"},
		"../../.github/workflows/release.yml": {"Audit published API and SDK compatibility", "git tag --merged", "make compatibility-audit"},
		"../../docs/compatibility.md":         {"Release Compatibility Audit", "compatibility-exceptions.json", "structural only"},
		"../../compatibility-exceptions.json": {"\"exceptions\""},
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

func TestReleaseRequiresPublicMultiArchitectureImages(t *testing.T) {
	body, err := os.ReadFile("../../.github/workflows/release.yml")
	if err != nil {
		t.Fatal(err)
	}
	for _, phrase := range []string{
		"public-images:",
		"needs: images",
		"https://ghcr.io/token?service=ghcr.io&scope=repository:${repository}:pull",
		"docker-content-digest",
		"application/vnd.oci.image.index.v1+json",
		".platform.architecture == \"amd64\"",
		".platform.architecture == \"arm64\"",
		"needs: [images, public-images]",
	} {
		if !strings.Contains(string(body), phrase) {
			t.Errorf("release workflow missing public image contract %q", phrase)
		}
	}
}
