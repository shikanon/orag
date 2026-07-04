package optimizer

import (
	"cmp"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/shikanon/orag/internal/eval"
)

type Direction string

const (
	Ascending  Direction = "asc"
	Descending Direction = "desc"
)

type ObjectiveSpec struct {
	Maximize            string           `json:"maximize,omitempty"`
	Constraints         []ConstraintSpec `json:"constraints,omitempty"`
	TieBreakers         []TieBreakerSpec `json:"tie_breakers,omitempty"`
	Budget              BudgetSpec       `json:"budget,omitempty"`
	BaselineID          string           `json:"baseline_id,omitempty"`
	BootstrapIterations int              `json:"bootstrap_iterations,omitempty"`
	SignificanceAlpha   float64          `json:"significance_alpha,omitempty"`
}

type BudgetSpec struct {
	LatencyP95LimitMS float64 `json:"latency_p95_limit_ms,omitempty"`
	CostLimitUSD      float64 `json:"cost_limit_usd,omitempty"`
}

type ConstraintSpec struct {
	Expression string `json:"expression"`
}

type TieBreakerSpec struct {
	Metric    string    `json:"metric"`
	Direction Direction `json:"direction,omitempty"`
}

type CandidateInput struct {
	ID        string
	Metrics   map[string]float64
	Pairwise  []PairwiseOutcome
	CreatedAt time.Time
}

type PairwiseOutcome struct {
	ItemID   string
	WinnerID string
	LoserID  string
	Tie      bool
}

type ObjectiveResult struct {
	Best       CandidateScore
	Candidates []CandidateScore
}

type CandidateScore struct {
	ID                  string
	Score               float64
	Metrics             map[string]float64
	Normalized          map[string]float64
	PairwiseWinRate     float64
	ConstraintFailed    bool
	ConstraintFailures  []string
	SignificantlyBetter bool
	CreatedAt           time.Time
}

type compiledConstraint struct {
	raw   string
	left  Expression
	op    string
	right Expression
}

func EvaluateObjective(spec ObjectiveSpec, candidates []CandidateInput) (ObjectiveResult, error) {
	maximize := strings.TrimSpace(spec.Maximize)
	if maximize == "" {
		maximize = "pairwise_accuracy"
	}
	for _, candidate := range candidates {
		if err := eval.ValidateMetricMap(candidate.Metrics); err != nil {
			return ObjectiveResult{}, err
		}
	}

	objectiveExpr, err := CompileExpression(maximize)
	if err != nil {
		return ObjectiveResult{}, err
	}
	constraints, err := compileConstraints(spec.Constraints)
	if err != nil {
		return ObjectiveResult{}, err
	}
	if err := validateTieBreakers(spec.TieBreakers); err != nil {
		return ObjectiveResult{}, err
	}

	scores := make([]CandidateScore, 0, len(candidates))
	for _, candidate := range candidates {
		vars := variablesForCandidate(spec, candidate)
		score, err := objectiveExpr.Evaluate(vars)
		if err != nil {
			return ObjectiveResult{}, err
		}
		candidateScore := CandidateScore{
			ID:              candidate.ID,
			Score:           score,
			Metrics:         cloneMetrics(candidate.Metrics),
			Normalized:      normalizedValues(spec, candidate.Metrics),
			PairwiseWinRate: vars["pairwise_win_rate"],
			CreatedAt:       candidate.CreatedAt,
		}
		for _, constraint := range constraints {
			passed, err := constraint.evaluate(vars)
			if err != nil {
				return ObjectiveResult{}, err
			}
			if !passed {
				candidateScore.ConstraintFailed = true
				candidateScore.ConstraintFailures = append(candidateScore.ConstraintFailures, constraint.raw)
			}
		}
		candidateScore.SignificantlyBetter = significantlyBetterThanBaseline(spec, candidate, vars["pairwise_win_rate"])
		scores = append(scores, candidateScore)
	}

	sort.SliceStable(scores, func(i, j int) bool {
		return compareCandidateScores(scores[i], scores[j], spec.TieBreakers)
	})

	result := ObjectiveResult{Candidates: scores}
	if len(scores) > 0 {
		result.Best = scores[0]
	}
	return result, nil
}

func compileConstraints(specs []ConstraintSpec) ([]compiledConstraint, error) {
	constraints := make([]compiledConstraint, 0, len(specs))
	for _, spec := range specs {
		raw := strings.TrimSpace(spec.Expression)
		op, idx := findComparisonOperator(raw)
		if op == "" {
			return nil, validationError("constraint %q must contain a comparison operator", raw)
		}
		left, err := CompileExpression(strings.TrimSpace(raw[:idx]))
		if err != nil {
			return nil, err
		}
		right, err := CompileExpression(strings.TrimSpace(raw[idx+len(op):]))
		if err != nil {
			return nil, err
		}
		constraints = append(constraints, compiledConstraint{raw: raw, left: left, op: op, right: right})
	}
	return constraints, nil
}

func findComparisonOperator(input string) (string, int) {
	for _, op := range []string{">=", "<=", "==", "!=", ">", "<"} {
		if idx := strings.Index(input, op); idx >= 0 {
			return op, idx
		}
	}
	return "", -1
}

func (c compiledConstraint) evaluate(vars map[string]float64) (bool, error) {
	left, err := c.left.Evaluate(vars)
	if err != nil {
		return false, err
	}
	right, err := c.right.Evaluate(vars)
	if err != nil {
		return false, err
	}
	switch c.op {
	case ">=":
		return left >= right, nil
	case "<=":
		return left <= right, nil
	case ">":
		return left > right, nil
	case "<":
		return left < right, nil
	case "==":
		return left == right, nil
	case "!=":
		return left != right, nil
	default:
		return false, validationError("unsupported constraint operator %q", c.op)
	}
}

func validateTieBreakers(tieBreakers []TieBreakerSpec) error {
	for _, tieBreaker := range tieBreakers {
		if tieBreaker.Metric != "created_at" && !eval.DefaultMetricRegistry.IsRegistered(tieBreaker.Metric) {
			return validationError("unknown tie-breaker metric %q", tieBreaker.Metric)
		}
		if tieBreaker.Direction != "" && tieBreaker.Direction != Ascending && tieBreaker.Direction != Descending {
			return validationError("unknown tie-breaker direction %q", tieBreaker.Direction)
		}
	}
	return nil
}

func variablesForCandidate(spec ObjectiveSpec, candidate CandidateInput) map[string]float64 {
	vars := cloneMetrics(candidate.Metrics)
	for name, value := range normalizedValues(spec, candidate.Metrics) {
		vars[name] = value
	}
	vars["pairwise_win_rate"] = pairwiseWinRate(candidate.ID, spec.BaselineID, candidate.Pairwise)
	return vars
}

func normalizedValues(spec ObjectiveSpec, metrics map[string]float64) map[string]float64 {
	normalized := map[string]float64{
		"normalized_latency": 0,
		"normalized_cost":    0,
	}
	if spec.Budget.LatencyP95LimitMS > 0 {
		normalized["normalized_latency"] = clamp01(metrics["latency_p95_ms"] / spec.Budget.LatencyP95LimitMS)
	}
	if spec.Budget.CostLimitUSD > 0 {
		normalized["normalized_cost"] = clamp01(metrics["cost_usd"] / spec.Budget.CostLimitUSD)
	}
	return normalized
}

func pairwiseWinRate(candidateID, baselineID string, outcomes []PairwiseOutcome) float64 {
	var total float64
	var wins float64
	for _, outcome := range outcomes {
		involvesCandidate := outcome.WinnerID == candidateID || outcome.LoserID == candidateID
		if !involvesCandidate {
			continue
		}
		if baselineID != "" && candidateID != baselineID {
			involvesBaseline := outcome.WinnerID == baselineID || outcome.LoserID == baselineID
			if !involvesBaseline {
				continue
			}
		}
		total++
		switch {
		case outcome.Tie:
			wins += 0.5
		case outcome.WinnerID == candidateID:
			wins++
		}
	}
	if total == 0 {
		return 0
	}
	return roundFloat(wins / total)
}

func significantlyBetterThanBaseline(spec ObjectiveSpec, candidate CandidateInput, winRate float64) bool {
	if spec.BaselineID == "" || candidate.ID == spec.BaselineID {
		return false
	}
	alpha := spec.SignificanceAlpha
	if alpha <= 0 {
		alpha = 0.05
	}
	wins, total := pairwiseWinsAgainstBaseline(candidate.ID, spec.BaselineID, candidate.Pairwise)
	if total == 0 {
		return false
	}
	if wins == total && winRate > 0.5 {
		return true
	}
	z := 1.96
	if alpha <= 0.01 {
		z = 2.58
	}
	p := float64(wins) / float64(total)
	margin := z * math.Sqrt((p*(1-p))/float64(total))
	return p-margin > 0.5
}

func pairwiseWinsAgainstBaseline(candidateID, baselineID string, outcomes []PairwiseOutcome) (int, int) {
	var wins, total int
	for _, outcome := range outcomes {
		involvesCandidate := outcome.WinnerID == candidateID || outcome.LoserID == candidateID
		involvesBaseline := outcome.WinnerID == baselineID || outcome.LoserID == baselineID
		if !involvesCandidate || !involvesBaseline {
			continue
		}
		total++
		if outcome.WinnerID == candidateID && !outcome.Tie {
			wins++
		}
	}
	return wins, total
}

func compareCandidateScores(left, right CandidateScore, tieBreakers []TieBreakerSpec) bool {
	if left.ConstraintFailed != right.ConstraintFailed {
		return !left.ConstraintFailed
	}
	if left.Score != right.Score {
		return left.Score > right.Score
	}
	for _, tieBreaker := range tieBreakers {
		leftValue := tieBreakerValue(left, tieBreaker.Metric)
		rightValue := tieBreakerValue(right, tieBreaker.Metric)
		if leftValue == rightValue {
			continue
		}
		if tieBreaker.Direction == Descending {
			return leftValue > rightValue
		}
		return leftValue < rightValue
	}
	return cmp.Less(left.ID, right.ID)
}

func tieBreakerValue(candidate CandidateScore, metric string) float64 {
	if metric == "created_at" {
		return float64(candidate.CreatedAt.UnixNano())
	}
	return candidate.Metrics[metric]
}

func cloneMetrics(metrics map[string]float64) map[string]float64 {
	cloned := make(map[string]float64, len(metrics))
	for key, value := range metrics {
		cloned[key] = value
	}
	return cloned
}

func clamp01(value float64) float64 {
	switch {
	case value < 0:
		return 0
	case value > 1:
		return 1
	default:
		return roundFloat(value)
	}
}
