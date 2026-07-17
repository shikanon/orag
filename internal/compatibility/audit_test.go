package compatibility

import (
	"strings"
	"testing"
)

func TestAuditRejectsPublishedSurfaceRemoval(t *testing.T) {
	base := []byte(`openapi: 3.0.3
info: {title: test, version: v1}
paths:
  /v1/items:
    get:
      responses:
        "200": {description: ok}
components:
  schemas:
    Item:
      type: object
      properties: {id: {type: string}, name: {type: string}}
`)
	current := []byte(`openapi: 3.0.3
info: {title: test, version: v2}
paths: {}
components:
  schemas:
    Item:
      type: object
      properties: {id: {type: string}}
`)
	baseSDK := map[string][]byte{"sdk.go": []byte("package orag\ntype Client struct{}\ntype Item struct { ID string; Name string }\nfunc (c *Client) Query() {}\nfunc New() {}\n")}
	currentSDK := map[string][]byte{"sdk.go": []byte("package orag\ntype Client struct{}\ntype Item struct { ID string }\n")}
	findings, err := Audit(base, current, baseSDK, currentSDK)
	if err != nil {
		t.Fatal(err)
	}
	joined := findingsText(findings)
	for _, want := range []string{"openapi.path_removed:/v1/items", "openapi.schema_property_removed:Item.name", "sdk.symbol_removed:field Item.Name", "sdk.symbol_removed:method Client.Query", "sdk.symbol_removed:func New"} {
		if !strings.Contains(joined, want) {
			t.Errorf("findings=%s, missing %s", joined, want)
		}
	}
}

func TestAuditAllowsAdditiveSurface(t *testing.T) {
	api := []byte(`openapi: 3.0.3
info: {title: test, version: v1}
paths:
  /v1/items:
    get:
      responses:
        "200": {description: ok}
components: {schemas: {Item: {type: object, properties: {id: {type: string}}}}}
`)
	base := map[string][]byte{"sdk.go": []byte("package orag\ntype Client struct{}\ntype Item struct { ID string }\nfunc (c *Client) Query() {}\n")}
	current := map[string][]byte{"sdk.go": []byte("package orag\ntype Client struct{}\ntype Item struct { ID string; Name string }\nfunc (c *Client) Query() {}\nfunc (c *Client) List() {}\n")}
	findings, err := Audit(api, api, base, current)
	if err != nil || len(findings) != 0 {
		t.Fatalf("findings=%v err=%v", findings, err)
	}
}

func findingsText(findings []Finding) string {
	values := make([]string, len(findings))
	for index := range findings {
		values[index] = findings[index].ID
	}
	return strings.Join(values, "\n")
}
