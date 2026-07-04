package optimizer

import (
	"strings"
	"testing"
	"time"
)

func TestObjectiveScoresWithConstraintsAndBudgetNormalization(t *testing.T) {
	spec := ObjectiveSpec{
		Maximize: "0.7*faithfulness + 0.2*(1-hallucination) + 0.1*(1-normalized_latency)",
		Constraints: []ConstraintSpec{
			{Expression: "faithfulness >= 0.85"},
			{Expression: "hallucination <= 0.05"},
			{Expression: "latency_p95_ms <= 2500"},
		},
		Budget: BudgetSpec{LatencyP95LimitMS: 2500, CostLimitUSD: 2},
		TieBreakers: []TieBreakerSpec{
			{Metric: "latency_p95_ms", Direction: Ascending},
		},
	}
	input := []CandidateInput{
		{
			ID: "fast",
			Metrics: map[string]float64{
				"faithfulness":   0.90,
				"hallucination":  0.02,
				"latency_p95_ms": 1000,
				"cost_usd":       0.8,
			},
		},
		{
			ID: "slow",
			Metrics: map[string]float64{
				"faithfulness":   0.92,
				"hallucination":  0.02,
				"latency_p95_ms": 2200,
				"cost_usd":       1.2,
			},
		},
		{
			ID: "unsafe",
			Metrics: map[string]float64{
				"faithfulness":   0.90,
				"hallucination":  0.20,
				"latency_p95_ms": 800,
				"cost_usd":       0.4,
			},
		},
	}

	result, err := EvaluateObjective(spec, input)
	if err != nil {
		t.Fatalf("EvaluateObjective() error = %v", err)
	}
	if len(result.Candidates) != 3 {
		t.Fatalf("candidates = %d, want 3", len(result.Candidates))
	}
	if result.Best.ID != "fast" {
		t.Fatalf("best = %q, want fast", result.Best.ID)
	}
	if !result.Candidates[2].ConstraintFailed {
		t.Fatalf("unsafe candidate = %#v, want constraint failure", result.Candidates[2])
	}
	if result.Candidates[0].Normalized["normalized_latency"] != 0.4 {
		t.Fatalf("normalized latency = %v, want 0.4", result.Candidates[0].Normalized["normalized_latency"])
	}
}

func TestObjectiveRejectsUnknownMetricsBeforeScoring(t *testing.T) {
	_, err := EvaluateObjective(
		ObjectiveSpec{Maximize: "faithfulness + harness_custom"},
		[]CandidateInput{{ID: "candidate", Metrics: map[string]float64{
			"faithfulness":   0.9,
			"harness_custom": 0.1,
		}}},
	)
	if err == nil || !strings.Contains(err.Error(), "unknown evaluation metric") {
		t.Fatalf("EvaluateObjective() error = %v, want unknown metric validation", err)
	}
}

func TestObjectiveUsesTieBreakersForDeterministicOrdering(t *testing.T) {
	now := time.Unix(100, 0)
	spec := ObjectiveSpec{
		Maximize: "faithfulness",
		TieBreakers: []TieBreakerSpec{
			{Metric: "latency_p95_ms", Direction: Ascending},
			{Metric: "created_at", Direction: Ascending},
		},
	}
	result, err := EvaluateObjective(spec, []CandidateInput{
		{ID: "later", CreatedAt: now.Add(time.Second), Metrics: map[string]float64{"faithfulness": 0.9, "latency_p95_ms": 100}},
		{ID: "faster", CreatedAt: now, Metrics: map[string]float64{"faithfulness": 0.9, "latency_p95_ms": 80}},
		{ID: "earlier", CreatedAt: now.Add(-time.Second), Metrics: map[string]float64{"faithfulness": 0.9, "latency_p95_ms": 100}},
	})
	if err != nil {
		t.Fatalf("EvaluateObjective() error = %v", err)
	}
	got := []string{result.Candidates[0].ID, result.Candidates[1].ID, result.Candidates[2].ID}
	want := []string{"faster", "earlier", "later"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}

func TestObjectivePairwiseWinRateAndBootstrapPromotion(t *testing.T) {
	spec := ObjectiveSpec{
		Maximize:            "pairwise_win_rate",
		BaselineID:          "baseline",
		BootstrapIterations: 200,
		SignificanceAlpha:   0.05,
	}
	result, err := EvaluateObjective(spec, []CandidateInput{
		{
			ID: "baseline",
			Pairwise: []PairwiseOutcome{
				{ItemID: "q1", WinnerID: "challenger", LoserID: "baseline"},
				{ItemID: "q2", WinnerID: "challenger", LoserID: "baseline"},
				{ItemID: "q3", WinnerID: "challenger", LoserID: "baseline"},
				{ItemID: "q4", WinnerID: "challenger", LoserID: "baseline"},
			},
		},
		{
			ID: "challenger",
			Pairwise: []PairwiseOutcome{
				{ItemID: "q1", WinnerID: "challenger", LoserID: "baseline"},
				{ItemID: "q2", WinnerID: "challenger", LoserID: "baseline"},
				{ItemID: "q3", WinnerID: "challenger", LoserID: "baseline"},
				{ItemID: "q4", WinnerID: "challenger", LoserID: "baseline"},
			},
		},
	})
	if err != nil {
		t.Fatalf("EvaluateObjective() error = %v", err)
	}
	if result.Best.ID != "challenger" {
		t.Fatalf("best = %q, want challenger", result.Best.ID)
	}
	if result.Best.PairwiseWinRate != 1 {
		t.Fatalf("win rate = %v, want 1", result.Best.PairwiseWinRate)
	}
	if !result.Best.SignificantlyBetter {
		t.Fatalf("best = %#v, want significant promotion", result.Best)
	}
}
