package pipeline

import (
	"context"
	"errors"
	"testing"

	"github.com/shikanon/orag/internal/rag"
)

func TestCompilerCompileValidDefinition(t *testing.T) {
	compiler := NewCompiler(&rag.Service{}, BuiltinRegistry())
	runner, err := compiler.Compile(context.Background(), validDefinition())
	if err != nil {
		t.Fatalf("compile valid definition: %v", err)
	}
	if runner == nil {
		t.Fatal("expected compiled runner")
	}
}

func TestCompilerRejectsInvalidDefinition(t *testing.T) {
	compiler := NewCompiler(&rag.Service{}, BuiltinRegistry())
	definition := validDefinition()
	definition.Nodes[0].Type = "untrusted_factory"
	_, err := compiler.Compile(context.Background(), definition)
	var validation ValidationErrors
	if !errors.As(err, &validation) || !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("expected validation error, got %v", err)
	}
}
