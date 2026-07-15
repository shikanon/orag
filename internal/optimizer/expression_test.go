package optimizer

import (
	"math"
	"strings"
	"testing"
)

func TestExpressionEvaluatesAllowedMetrics(t *testing.T) {
	expr, err := CompileExpression("0.5*faithfulness + 0.25*(1-hallucination) + normalized_latency")
	if err != nil {
		t.Fatalf("CompileExpression() error = %v", err)
	}

	got, err := expr.Evaluate(map[string]float64{
		"faithfulness":       0.8,
		"hallucination":      0.2,
		"normalized_latency": 0.1,
	})
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if got != 0.7 {
		t.Fatalf("Evaluate() = %v, want 0.7", got)
	}
}

func TestExpressionRejectsUnsafeSyntax(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want string
	}{
		{name: "unknown variable", expr: "faithfulness + unknown_metric", want: "unknown variable"},
		{name: "function call", expr: "max(faithfulness, 0.8)", want: "function calls are not allowed"},
		{name: "property access", expr: "metrics.faithfulness", want: "invalid character"},
		{name: "index access", expr: "metrics[0]", want: "invalid character"},
		{name: "interpolation", expr: "${faithfulness}", want: "invalid character"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := CompileExpression(tt.expr)
			if err == nil {
				t.Fatalf("CompileExpression(%q) error = nil, want %q", tt.expr, tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("CompileExpression(%q) error = %v, want substring %q", tt.expr, err, tt.want)
			}
		})
	}
}

func TestExpressionRejectsDivisionByZero(t *testing.T) {
	expr, err := CompileExpression("faithfulness / (1 - hallucination)")
	if err != nil {
		t.Fatalf("CompileExpression() error = %v", err)
	}
	_, err = expr.Evaluate(map[string]float64{
		"faithfulness":  0.8,
		"hallucination": 1,
	})
	if err == nil || !strings.Contains(err.Error(), "division by zero") {
		t.Fatalf("Evaluate() error = %v, want division by zero", err)
	}
}

func TestExpressionPreservesLargeFiniteLiteral(t *testing.T) {
	literal := "18" + strings.Repeat("0", 306)
	expr, err := CompileExpression(literal)
	if err != nil {
		t.Fatalf("CompileExpression() error = %v", err)
	}
	value, err := expr.Evaluate(nil)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		t.Fatalf("Evaluate() = %v, want finite value", value)
	}
}

func TestExpressionRejectsNonFiniteResult(t *testing.T) {
	literal := "18" + strings.Repeat("0", 306)
	expr, err := CompileExpression(literal + " * " + literal)
	if err != nil {
		t.Fatalf("CompileExpression() error = %v", err)
	}
	if _, err := expr.Evaluate(nil); err == nil || !strings.Contains(err.Error(), "expression result must be finite") {
		t.Fatalf("Evaluate() error = %v, want finite-result validation error", err)
	}
}
