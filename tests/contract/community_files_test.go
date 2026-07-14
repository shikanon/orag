package contract_test

import (
	"bytes"
	"os"
	"testing"
)

func TestCommunityFilesContainRequiredPolicy(t *testing.T) {
	t.Parallel()

	checks := map[string][]string{
		"../../CONTRIBUTING.md": {
			"make test",
			"Pull Request",
			"generated",
		},
		"../../SECURITY.md": {
			"Supported Versions",
			"Private Vulnerability Reporting",
			"security advisory",
		},
		"../../CODE_OF_CONDUCT.md": {
			"Contributor Covenant",
			"Enforcement Responsibilities",
		},
		"../../CHANGELOG.md": {
			"Keep a Changelog",
			"[Unreleased]",
		},
		"../../docs/compatibility.md": {
			"experimental",
			"beta",
			"stable",
			"v1.0.0",
		},
	}

	for path, phrases := range checks {
		path := path
		phrases := phrases
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			for _, phrase := range phrases {
				if !bytes.Contains(body, []byte(phrase)) {
					t.Errorf("%s missing %q", path, phrase)
				}
			}
		})
	}
}
