package contract

import (
	"os"
	"strings"
	"testing"
)

func TestPublicExtensionsDocumentSupportLevelsAndContracts(t *testing.T) {
	raw, err := os.ReadFile("../../docs/extensions.md")
	if err != nil {
		t.Fatal(err)
	}
	content := string(raw)
	for _, required := range []string{"extensions/conformance", "certified", "community", "experimental", "Parser", "Storage"} {
		if !strings.Contains(content, required) {
			t.Errorf("extensions documentation is missing %q", required)
		}
	}
}
