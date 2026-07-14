package contract_test

import (
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
