package contract_test

import (
	"os"
	"strings"
	"testing"
)

func TestCredentialRotationDrillIsDocumented(t *testing.T) {
	checks := map[string][]string{
		"../../Makefile": {
			"credential-rotation-drill:",
			"./scripts/credential-rotation-drill.sh",
		},
		"../../scripts/credential-rotation-drill.sh": {
			"orag.credential-rotation-drill.v1",
			"/rotate",
			"source_status\" == \"401",
			"replacement_status\" == \"200",
			"immediate_cutover:true",
		},
		"../../docs/operations/credential-rotation.md": {
			"make credential-rotation-drill",
			"JWT_SECRET",
			"API_KEY_PEPPER",
			"不应把 secret",
		},
		"../../docs/security/threat-model.md": {
			"威胁模型",
			"credential-rotation-drill",
			"残余风险",
		},
		"../../docs-site/credential-rotation.html": {
			"make credential-rotation-drill",
			"401",
			"200",
			"not production proof",
		},
		"../../docs-site/index.html": {
			"credential-rotation.html",
		},
		"../../ROADMAP.md": {
			"隔离 API Key immediate-cutover 演练",
		},
		"../../ROADMAP_EN.md": {
			"isolated API-key immediate-cutover drill",
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
