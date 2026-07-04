package eval

import "context"

type Judge interface {
	Judge(ctx context.Context, input JudgeInput) (JudgeOutput, error)
}

type PairwiseJudge interface {
	Compare(ctx context.Context, input PairwiseJudgeInput) (PairwiseJudgeOutput, error)
}

type QAGJudge interface {
	ScoreQAG(ctx context.Context, input JudgeInput) (QAGOutput, error)
}

type RuleBasedJudge struct {
	PromptVersion string
}

func (j RuleBasedJudge) Judge(_ context.Context, input JudgeInput) (JudgeOutput, error) {
	scores := map[string]float64{
		"answer_accuracy": boolScore(matches(input.Answer, input.GroundTruth)),
	}
	if err := ValidateMetricMap(scores); err != nil {
		return JudgeOutput{}, err
	}
	return JudgeOutput{
		Scores:        scores,
		Labels:        map[string]string{"answer_accuracy": coarseLabel(scores["answer_accuracy"])},
		Pass:          scores["answer_accuracy"] >= 0.5,
		Rationale:     "rule-based answer match",
		PromptVersion: j.PromptVersion,
	}, nil
}
