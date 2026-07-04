package eval

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/shikanon/orag/internal/kb"
	"github.com/shikanon/orag/internal/platform/apperrors"
	"github.com/shikanon/orag/internal/rag"
)

type fakeJudgeChatModel struct {
	responses []JudgeChatResponse
	errors    []error
	prompts   []string
}

func (m *fakeJudgeChatModel) Chat(_ context.Context, prompt string) (JudgeChatResponse, error) {
	m.prompts = append(m.prompts, prompt)
	if len(m.errors) > 0 {
		err := m.errors[0]
		m.errors = m.errors[1:]
		if err != nil {
			return JudgeChatResponse{}, err
		}
	}
	if len(m.responses) == 0 {
		return JudgeChatResponse{}, nil
	}
	resp := m.responses[0]
	m.responses = m.responses[1:]
	return resp, nil
}

type deadlineJudgeChatModel struct {
	sawDeadline bool
}

func (m *deadlineJudgeChatModel) Chat(ctx context.Context, _ string) (JudgeChatResponse, error) {
	_, m.sawDeadline = ctx.Deadline()
	return JudgeChatResponse{Content: `{"winner":"A","preference":"A_better","reasons":[]}`}, nil
}

func TestJudgePromptRendersStrictJSONInstructions(t *testing.T) {
	input := judgeInputFixture()
	prompt, err := RenderPairwiseJudgePrompt(PairwiseJudgeInput{
		Query:       input.Query,
		GroundTruth: input.GroundTruth,
		AnswerA:     CandidateAnswer{ID: "a", Answer: "Paris", Citations: input.Citations, RetrievedChunks: input.RetrievedChunks},
		AnswerB:     CandidateAnswer{ID: "b", Answer: "Lyon"},
		Rubric:      input.Rubric,
	}, JudgeConfig{Metrics: []JudgeMetric{JudgeMetricFaithfulness}})
	if err != nil {
		t.Fatalf("RenderPairwiseJudgePrompt() error = %v", err)
	}
	for _, want := range []string{"strict JSON", "Answer A", "Answer B", "Paris", "Lyon", "retrieved_chunks", "faithfulness"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestLLMJudgeParsesStrictJSONAndNormalizesScores(t *testing.T) {
	model := &fakeJudgeChatModel{responses: []JudgeChatResponse{{
		Content: `{"scores":{"faithfulness":1.2,"hallucination":-0.1},"labels":{},"confidence":{"faithfulness":{"mean":1.5,"low":-1,"high":0.9,"n":2,"method":"repeat"}},"pass":true,"rationale":"ok","findings":[{"metric":"faithfulness","label":"supported"}]}`,
		Model:   "judge-model",
		TokenUsage: TokenUsage{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		},
	}}}
	judge := NewLLMJudge(model, JudgeConfig{Model: "fallback", PromptVersion: "pv1"})
	judge.Now = func() time.Time { return time.Date(2026, 7, 4, 1, 2, 3, 0, time.UTC) }

	out, err := judge.Judge(context.Background(), judgeInputFixture())
	if err != nil {
		t.Fatalf("Judge() error = %v", err)
	}
	if out.Scores["faithfulness"] != 1 || out.Scores["hallucination"] != 0 {
		t.Fatalf("scores = %#v, want clamped to [0,1]", out.Scores)
	}
	if out.Labels["faithfulness"] != "good" || out.Labels["hallucination"] != "bad" {
		t.Fatalf("labels = %#v, want coarse defaults", out.Labels)
	}
	if out.Confidence["faithfulness"].Mean != 1 || out.Confidence["faithfulness"].Low != 0 {
		t.Fatalf("confidence = %#v, want clamped interval", out.Confidence)
	}
	if out.RawResponse == "" || out.ParsedJSON["scores"] == nil {
		t.Fatalf("raw/parsed response not persisted: %#v", out)
	}
	if out.JudgeModel != "judge-model" || out.PromptVersion != "pv1" || out.RubricHash == "" || out.ConfigHash == "" {
		t.Fatalf("metadata missing: %#v", out)
	}
	if out.CreatedAt.IsZero() || out.TokenUsage.TotalTokens != 15 {
		t.Fatalf("created/token metadata missing: %#v", out)
	}
}

func TestLLMJudgeRejectsMalformedJSON(t *testing.T) {
	model := &fakeJudgeChatModel{responses: []JudgeChatResponse{{Content: `{"scores":{"faithfulness":0.9}`}}}
	judge := NewLLMJudge(model, JudgeConfig{})

	_, err := judge.Judge(context.Background(), judgeInputFixture())
	if !apperrors.IsCode(err, apperrors.CodeValidation) {
		t.Fatalf("Judge() error = %v, want validation", err)
	}
}

func TestLLMJudgePairwiseSwapDetectsStableAndUnstable(t *testing.T) {
	t.Run("stable", func(t *testing.T) {
		model := &fakeJudgeChatModel{responses: []JudgeChatResponse{
			{Content: `{"winner":"A","preference":"A_better","reasons":[{"message":"more grounded"}]}`},
			{Content: `{"winner":"B","preference":"B_better","reasons":[{"message":"more grounded"}]}`},
		}}
		judge := NewLLMJudge(model, JudgeConfig{PairwiseSwap: true})
		out, err := judge.Compare(context.Background(), pairwiseInputFixture())
		if err != nil {
			t.Fatalf("Compare() error = %v", err)
		}
		if out.Winner != "A" || !out.Stable {
			t.Fatalf("Compare() = %#v, want stable A", out)
		}
		if len(model.prompts) != 2 || !strings.Contains(model.prompts[1], `"answer_a":{"id":"b"`) {
			t.Fatalf("swapped prompt not rendered: %#v", model.prompts)
		}
	})

	t.Run("unstable", func(t *testing.T) {
		model := &fakeJudgeChatModel{responses: []JudgeChatResponse{
			{Content: `{"winner":"A","preference":"A_better","reasons":[]}`},
			{Content: `{"winner":"A","preference":"A_better","reasons":[]}`},
		}}
		judge := NewLLMJudge(model, JudgeConfig{PairwiseSwap: true})
		out, err := judge.Compare(context.Background(), pairwiseInputFixture())
		if err != nil {
			t.Fatalf("Compare() error = %v", err)
		}
		if out.Stable || out.Preference != "unstable" {
			t.Fatalf("Compare() = %#v, want unstable", out)
		}
	})
}

func TestLLMJudgePairwiseRepeatAggregatesMajorityAndStrength(t *testing.T) {
	model := &fakeJudgeChatModel{responses: []JudgeChatResponse{
		{Content: `{"winner":"A","preference":"A_much_better","reasons":[{"message":"repeat 1"}]}`, TokenUsage: TokenUsage{TotalTokens: 3}},
		{Content: `{"winner":"B","preference":"B_much_better","reasons":[{"message":"repeat 1 swapped"}]}`, TokenUsage: TokenUsage{TotalTokens: 4}},
		{Content: `{"winner":"A","preference":"A_better","reasons":[{"message":"repeat 2"}]}`, TokenUsage: TokenUsage{TotalTokens: 5}},
		{Content: `{"winner":"B","preference":"B_better","reasons":[{"message":"repeat 2 swapped"}]}`, TokenUsage: TokenUsage{TotalTokens: 6}},
		{Content: `{"winner":"B","preference":"B_better","reasons":[{"message":"repeat 3"}]}`, TokenUsage: TokenUsage{TotalTokens: 7}},
		{Content: `{"winner":"A","preference":"A_better","reasons":[{"message":"repeat 3 swapped"}]}`, TokenUsage: TokenUsage{TotalTokens: 8}},
	}}
	judge := NewLLMJudge(model, JudgeConfig{PairwiseSwap: true, Repeat: 3})

	out, err := judge.Compare(context.Background(), pairwiseInputFixture())
	if err != nil {
		t.Fatalf("Compare() error = %v", err)
	}

	if out.Winner != "A" || !out.Stable || out.Preference != "A_better" || out.VoteCount != 2 {
		t.Fatalf("Compare() = %#v, want stable A majority", out)
	}
	if out.PreferenceStrength != 1.0/3.0 {
		t.Fatalf("PreferenceStrength = %v, want signed average stable strength 1/3", out.PreferenceStrength)
	}
	if out.TokenUsage.TotalTokens != 33 {
		t.Fatalf("TokenUsage.TotalTokens = %d, want all pairwise calls summed", out.TokenUsage.TotalTokens)
	}
	if got := len(model.prompts); got != 6 {
		t.Fatalf("prompt count = %d, want 6 for repeat*swap", got)
	}
}

func TestLLMJudgePairwiseEnsembleDisagreementIsUnstable(t *testing.T) {
	primary := &fakeJudgeChatModel{responses: []JudgeChatResponse{{
		Content: `{"winner":"A","preference":"A_better","reasons":[{"message":"primary"}]}`,
	}}}
	ensemble := &fakeJudgeChatModel{responses: []JudgeChatResponse{{
		Content: `{"winner":"B","preference":"B_better","reasons":[{"message":"ensemble"}]}`,
	}}}
	judge := NewLLMJudge(primary, JudgeConfig{})
	judge.Ensemble = []JudgeChatModel{ensemble}

	out, err := judge.Compare(context.Background(), pairwiseInputFixture())
	if err != nil {
		t.Fatalf("Compare() error = %v", err)
	}

	if out.Stable || out.Preference != "unstable" {
		t.Fatalf("Compare() = %#v, want unstable ensemble split", out)
	}
	if out.VoteCount != 1 {
		t.Fatalf("VoteCount = %d, want no majority beyond one vote", out.VoteCount)
	}
}

func TestLLMJudgeRetriesRateLimitedPairwiseCall(t *testing.T) {
	model := &fakeJudgeChatModel{
		errors: []error{
			apperrors.New(apperrors.CodeRateLimited, "429"),
			nil,
			nil,
		},
		responses: []JudgeChatResponse{
			{Content: `{"winner":"A","preference":"A_better","reasons":[]}`},
			{Content: `{"winner":"B","preference":"B_better","reasons":[]}`},
		},
	}
	judge := NewLLMJudge(model, JudgeConfig{
		PairwiseSwap:   true,
		MaxRetries:     1,
		BackoffInitial: time.Nanosecond,
		BackoffMax:     time.Nanosecond,
	})

	out, err := judge.Compare(context.Background(), pairwiseInputFixture())
	if err != nil {
		t.Fatalf("Compare() error = %v", err)
	}

	if out.Winner != "A" || !out.Stable {
		t.Fatalf("Compare() = %#v, want retry then stable A", out)
	}
	if got := len(model.prompts); got != 3 {
		t.Fatalf("prompt count = %d, want initial 429 + retry + swap", got)
	}
}

func TestLLMJudgeStopsWhenPairwiseBudgetExceeded(t *testing.T) {
	model := &fakeJudgeChatModel{responses: []JudgeChatResponse{{
		Content: `{"winner":"A","preference":"A_better","reasons":[]}`,
	}}}
	judge := NewLLMJudge(model, JudgeConfig{PairwiseSwap: true, MaxJudgeCalls: 1})

	_, err := judge.Compare(context.Background(), pairwiseInputFixture())
	if !apperrors.IsCode(err, apperrors.CodeRateLimited) {
		t.Fatalf("Compare() error = %v, want budget rate_limited", err)
	}
	if got := len(model.prompts); got != 1 {
		t.Fatalf("prompt count = %d, want budget stop before swapped call", got)
	}
}

func TestLLMJudgePairwiseTimeoutAndCircuitBreaker(t *testing.T) {
	t.Run("timeout is applied to provider call", func(t *testing.T) {
		model := &deadlineJudgeChatModel{}
		judge := NewLLMJudge(model, JudgeConfig{Timeout: time.Second})

		_, err := judge.Compare(context.Background(), pairwiseInputFixture())
		if err != nil {
			t.Fatalf("Compare() error = %v", err)
		}
		if !model.sawDeadline {
			t.Fatal("provider call did not receive context deadline")
		}
	})

	t.Run("circuit breaker opens on repeated upstream failures", func(t *testing.T) {
		model := &fakeJudgeChatModel{errors: []error{
			apperrors.New(apperrors.CodeUpstreamUnavailable, "503"),
		}}
		judge := NewLLMJudge(model, JudgeConfig{
			MaxRetries:      2,
			CircuitFailures: 1,
			BackoffInitial:  time.Nanosecond,
		})

		_, err := judge.Compare(context.Background(), pairwiseInputFixture())
		if !apperrors.IsCode(err, apperrors.CodeUpstreamUnavailable) || !strings.Contains(err.Error(), "circuit breaker open") {
			t.Fatalf("Compare() error = %v, want circuit breaker upstream error", err)
		}
		if got := len(model.prompts); got != 1 {
			t.Fatalf("prompt count = %d, want circuit breaker after first failure", got)
		}
	})
}

func TestLLMJudgeQAGParsing(t *testing.T) {
	model := &fakeJudgeChatModel{responses: []JudgeChatResponse{{Content: `{"score":1.3,"claims":[{"claim":"Paris is the capital of France","question":"What is the capital of France?","answer":"Paris","verdict":"supported","evidence":"France capital is Paris"}]}`}}}
	judge := NewLLMJudge(model, JudgeConfig{})

	out, err := judge.ScoreQAG(context.Background(), judgeInputFixture())
	if err != nil {
		t.Fatalf("ScoreQAG() error = %v", err)
	}
	if out.Score != 1 || len(out.Claims) != 1 || out.Claims[0].Verdict != "supported" {
		t.Fatalf("ScoreQAG() = %#v", out)
	}
	if out.Metrics["qag_score"] != 1 || out.Metrics["qag_claim_coverage"] != 1 || out.Metrics["qag_question_count"] != 1 || out.Metrics["qag_unverifiable_rate"] != 0 {
		t.Fatalf("QAG metrics = %#v, want full support and coverage", out.Metrics)
	}
	if !strings.Contains(model.prompts[0], "context-only answers") || !strings.Contains(model.prompts[0], "expected_evidence") {
		t.Fatalf("QAG prompt missing verdict instruction:\n%s", model.prompts[0])
	}
}

func TestLLMJudgeQAGSummarizesContradictedAndUnverifiableClaims(t *testing.T) {
	model := &fakeJudgeChatModel{responses: []JudgeChatResponse{{Content: `{"score":1,"claims":[{"claim":"Paris is the capital of France","question":"What is the capital of France?","answer":"Paris","verdict":"SUPPORTED","evidence":"France capital is Paris"},{"claim":"Lyon is the capital","question":"Is Lyon the capital?","answer":"No, Paris is the capital","verdict":"contradicted","evidence":"France capital is Paris"},{"claim":"France has 70M people","question":"What is France population?","answer":"","verdict":"unverifiable","evidence":""}]}`}}}
	judge := NewLLMJudge(model, JudgeConfig{})

	out, err := judge.ScoreQAG(context.Background(), judgeInputFixture())
	if err != nil {
		t.Fatalf("ScoreQAG() error = %v", err)
	}

	if out.Score != 1.0/3.0 || out.Metrics["qag_score"] != 1.0/3.0 {
		t.Fatalf("qag score = %v metrics=%#v, want supported/total", out.Score, out.Metrics)
	}
	if out.Metrics["qag_question_count"] != 3 {
		t.Fatalf("qag_question_count = %v, want 3", out.Metrics["qag_question_count"])
	}
	if out.Metrics["qag_unverifiable_rate"] != 1.0/3.0 {
		t.Fatalf("qag_unverifiable_rate = %v, want 1/3", out.Metrics["qag_unverifiable_rate"])
	}
	if out.Claims[0].Verdict != "supported" {
		t.Fatalf("verdict normalization failed: %#v", out.Claims)
	}
}

func TestLLMJudgeQAGDetectsMissingExpectedEvidenceCoverage(t *testing.T) {
	model := &fakeJudgeChatModel{responses: []JudgeChatResponse{{Content: `{"score":1,"claims":[{"claim":"Paris is the capital of France","question":"What is the capital?","answer":"Paris","verdict":"supported","evidence":"France capital is Paris"}]}`}}}
	input := judgeInputFixture()
	input.ExpectedEvidence = []string{"France capital is Paris", "Eiffel Tower is in Paris"}
	judge := NewLLMJudge(model, JudgeConfig{})

	out, err := judge.ScoreQAG(context.Background(), input)
	if err != nil {
		t.Fatalf("ScoreQAG() error = %v", err)
	}

	if out.Metrics["qag_claim_coverage"] != 0.5 {
		t.Fatalf("qag_claim_coverage = %v, want 0.5 for one missing key claim", out.Metrics["qag_claim_coverage"])
	}
}

func TestJudgeHashesAreStable(t *testing.T) {
	cfg := JudgeConfig{Model: "m", PromptVersion: "p", Metrics: []JudgeMetric{JudgeMetricFaithfulness}}
	if HashJudgeConfig(cfg) == "" || HashJudgeConfig(cfg) != HashJudgeConfig(cfg) {
		t.Fatal("config hash should be stable and non-empty")
	}
	rubric := judgeInputFixture().Rubric
	if HashJudgeRubric(rubric) == "" || HashJudgeRubric(rubric) != HashJudgeRubric(rubric) {
		t.Fatal("rubric hash should be stable and non-empty")
	}
}

func judgeInputFixture() JudgeInput {
	return JudgeInput{
		Query:            "What is the capital of France?",
		GroundTruth:      "Paris",
		ExpectedEvidence: []string{"France capital is Paris"},
		RelevantDocIDs:   []string{"doc1"},
		Answer:           "Paris is the capital of France.",
		Citations:        []rag.Citation{{DocumentID: "doc1", ChunkID: "chunk1", Quote: "Paris is France's capital"}},
		RetrievedChunks: []kb.SearchResult{{
			Chunk: kb.Chunk{ID: "chunk1", DocumentID: "doc1", Content: "Paris is France's capital."},
			Score: 0.9,
			Rank:  1,
		}},
		Rubric: JudgeRubric{
			Name:    "grounding",
			Version: "v1",
			Criteria: []RubricCriterion{{
				Metric:      JudgeMetricFaithfulness,
				Description: "answer claims are supported by evidence",
				Weight:      1,
			}},
		},
	}
}

func pairwiseInputFixture() PairwiseJudgeInput {
	input := judgeInputFixture()
	return PairwiseJudgeInput{
		Query:       input.Query,
		GroundTruth: input.GroundTruth,
		AnswerA: CandidateAnswer{
			ID:              "a",
			Answer:          "Paris is the capital of France.",
			Citations:       input.Citations,
			RetrievedChunks: input.RetrievedChunks,
		},
		AnswerB: CandidateAnswer{
			ID:     "b",
			Answer: "Lyon is the capital of France.",
		},
		Rubric: input.Rubric,
	}
}
