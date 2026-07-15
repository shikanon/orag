package optimizer

import (
	"math"
	"strings"
	"testing"
)

func FuzzCompileExpression(f *testing.F) {
	for _, seed := range []string{
		"faithfulness",
		"0.5*faithfulness + 0.25*(1-hallucination)",
		"normalized_latency / (1 - hallucination)",
		"max(faithfulness, 0.8)",
		"${metrics.faithfulness}",
		"((((((((faithfulness))))))))",
		"18" + strings.Repeat("0", 306),
	} {
		f.Add(seed)
	}

	values := make(map[string]float64)
	for name := range defaultAllowedVariables() {
		values[name] = 0.5
	}

	f.Fuzz(func(t *testing.T, input string) {
		expression, err := CompileExpression(input)
		if err != nil {
			return
		}
		value, err := expression.Evaluate(values)
		if err != nil {
			return
		}
		if math.IsNaN(value) || math.IsInf(value, 0) {
			t.Fatalf("successful evaluation produced non-finite value %v for %q", value, input)
		}
	})
}
