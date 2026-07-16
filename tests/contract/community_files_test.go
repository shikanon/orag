package contract_test

import (
	"bytes"
	"os"
	"strings"
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
		"../../GOVERNANCE.md": {"RFC process", "Committers", "Maintainers", "SECURITY.md"},
		"../../docs/decisions/README.md": {"accepted", "rejected", "superseded"},
		"../../.github/DISCUSSION_TEMPLATE/rfc.md": {"Problem", "Alternatives", "Compatibility", "Security", "Validation"},
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

func TestGitHubTemplatesContainRequiredIntake(t *testing.T) {
	t.Parallel()

	forms := []string{
		"../../.github/ISSUE_TEMPLATE/bug.yml",
		"../../.github/ISSUE_TEMPLATE/feature.yml",
		"../../.github/ISSUE_TEMPLATE/documentation.yml",
		"../../.github/ISSUE_TEMPLATE/rfc.yml",
	}
	for _, path := range forms {
		path := path
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			content := string(body)
			for _, field := range []string{"name:", "description:", "body:", "validations:", "required: true"} {
				if !strings.Contains(content, field) {
					t.Errorf("%s missing %q", path, field)
				}
			}
		})
	}

	config, err := os.ReadFile("../../.github/ISSUE_TEMPLATE/config.yml")
	if err != nil {
		t.Fatalf("read issue config: %v", err)
	}
	for _, phrase := range []string{"blank_issues_enabled: false", "security/advisories/new"} {
		if !bytes.Contains(config, []byte(phrase)) {
			t.Errorf("issue config missing %q", phrase)
		}
	}

	pullRequest, err := os.ReadFile("../../.github/pull_request_template.md")
	if err != nil {
		t.Fatalf("read pull request template: %v", err)
	}
	for _, heading := range []string{"Testing", "Documentation", "Security", "Compatibility", "Maturity"} {
		if !bytes.Contains(pullRequest, []byte(heading)) {
			t.Errorf("pull request template missing %q", heading)
		}
	}
}

func TestDependabotCoversCurrentDependencyEcosystems(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile("../../.github/dependabot.yml")
	if err != nil {
		t.Fatalf("read dependabot config: %v", err)
	}
	content := string(body)
	for _, phrase := range []string{
		`package-ecosystem: "gomod"`,
		`package-ecosystem: "npm"`,
		`package-ecosystem: "github-actions"`,
		`package-ecosystem: "docker"`,
		`directory: "/console"`,
		`directory: "/deployments"`,
		`interval: "weekly"`,
	} {
		if !strings.Contains(content, phrase) {
			t.Errorf("dependabot config missing %q", phrase)
		}
	}
}

func TestReadmesPublishCapabilityMaturityPolicy(t *testing.T) {
	t.Parallel()

	for _, path := range []string{"../../README.md", "../../README_EN.md"} {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, phrase := range []string{
			"docs/compatibility.md",
			"experimental",
			"beta",
			"stable",
			"v0.1.0-beta.1",
		} {
			if !bytes.Contains(body, []byte(phrase)) {
				t.Errorf("%s missing %q", path, phrase)
			}
		}
	}
}
