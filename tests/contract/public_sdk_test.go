package contract_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestPublicSDKDoesNotExposeInternalTypes(t *testing.T) {
	command := exec.Command("go", "doc", "-all", "github.com/shikanon/orag")
	command.Dir = "../.."
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("inspect public SDK: %v\n%s", err, output)
	}
	if strings.Contains(string(output), "github.com/shikanon/orag/internal/") {
		t.Fatalf("public SDK documentation exposes an internal package:\n%s", output)
	}
}

func TestSDKDocumentationAndExampleAreIndexed(t *testing.T) {
	checks := map[string][]string{
		"../../README.md":               {"docs/sdk/README.md", "examples/go/sdk"},
		"../../README_EN.md":            {"docs/sdk/README.md", "examples/go/sdk"},
		"../../docs/README.md":          {"sdk/README.md"},
		"../../docs/sdk/README.md":      {"MockConfig", "NewFromEnv", "StreamQuery", "errors.Is", "make sdk-check"},
		"../../examples/go/sdk/main.go": {"github.com/shikanon/orag", "MockConfig", "RunEvaluation"},
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
