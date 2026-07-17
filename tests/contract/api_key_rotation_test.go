package contract

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func TestAPIKeyRotationContract(t *testing.T) {
	doc, err := openapi3.NewLoader().LoadFromFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatal(err)
	}
	path := doc.Paths.Find("/v1/api-keys/{api_key_id}/rotate")
	if path == nil || path.Post == nil || path.Post.OperationID != "rotateAPIKey" || path.Post.Responses.Value("201") == nil {
		t.Fatalf("rotation endpoint contract missing or incomplete: %#v", path)
	}
	apiKey := doc.Components.Schemas["APIKey"].Value
	if apiKey.Properties["rotated_from_key_id"] == nil {
		t.Fatal("APIKey schema must expose safe rotation lineage")
	}

	for _, file := range []string{"../../console/src/features/api-keys/api-key-list.tsx", "../../docs/api/auth-and-errors.md"} {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(content), "立即") {
			t.Fatalf("%s must document immediate rotation cutover", file)
		}
	}
}
