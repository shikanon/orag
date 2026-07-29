// Package orag provides the public embeddable Go SDK for ORAG.
//
// The SDK and the ORAG HTTP service share the same application services. Use
// MockConfig for deterministic, dependency-free examples and tests, or provide
// explicit PostgreSQL, Qdrant, and model-provider configuration for a real
// deployment. Public signatures never expose packages below internal/.
package orag
