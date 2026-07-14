// Package api exposes the canonical ORAG OpenAPI document and the pinned
// interactive documentation assets used by the HTTP service.
package api

import _ "embed"

// OpenAPISpec is the canonical API description served at /openapi.yaml.
//
//go:embed openapi.yaml
var OpenAPISpec []byte

// SwaggerUIStyles is vendored from swagger-ui-dist 5.32.8.
//
//go:embed swagger-ui/swagger-ui.css
var SwaggerUIStyles []byte

// SwaggerUIBundle is vendored from swagger-ui-dist 5.32.8.
//
//go:embed swagger-ui/swagger-ui-bundle.js
var SwaggerUIBundle []byte
