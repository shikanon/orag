package eval

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/shikanon/orag/internal/platform/apperrors"
)

const maxJudgeResponseBytes = 1 << 20

type JudgeChatModel interface {
	Chat(ctx context.Context, prompt string) (JudgeChatResponse, error)
}

type JudgeChatResponse struct {
	Content    string     `json:"content"`
	TokenUsage TokenUsage `json:"token_usage,omitempty"`
	Model      string     `json:"model,omitempty"`
}

type LLMJudge struct {
	Model    JudgeChatModel
	Ensemble []JudgeChatModel
	Config   JudgeConfig
	Now      func() time.Time
}

func NewLLMJudge(model JudgeChatModel, cfg JudgeConfig) LLMJudge {
	if cfg.PromptVersion == "" {
		cfg.PromptVersion = defaultJudgePromptVersion
	}
	if cfg.Repeat <= 0 {
		cfg.Repeat = 1
	}
	return LLMJudge{Model: model, Config: cfg}
}

func (j LLMJudge) Judge(ctx context.Context, input JudgeInput) (JudgeOutput, error) {
	if j.Model == nil {
		return JudgeOutput{}, apperrors.New(apperrors.CodeValidation, "judge model is required")
	}
	prompt, err := RenderJudgePrompt(input, j.Config)
	if err != nil {
		return JudgeOutput{}, err
	}
	repeats := max(1, j.Config.Repeat)
	outputs := make([]JudgeOutput, 0, repeats)
	limiter := newJudgeCallLimiter(j.Config.MaxJudgeCalls)
	for i := 0; i < repeats; i++ {
		resp, err := j.callChat(ctx, j.Model, prompt, limiter)
		if err != nil {
			return JudgeOutput{}, err
		}
		out, err := parseJudgeOutput(resp.Content)
		if err != nil {
			return JudgeOutput{}, err
		}
		out.RawResponse = resp.Content
		out.TokenUsage = resp.TokenUsage
		out.JudgeModel = firstNonEmpty(resp.Model, j.Config.Model)
		out.PromptVersion = j.Config.PromptVersion
		out.RubricHash = HashJudgeRubric(firstRubric(input.Rubric, j.Config.Rubric))
		out.ConfigHash = HashJudgeConfig(j.Config)
		out.CreatedAt = j.now()
		outputs = append(outputs, out)
	}
	return aggregateJudgeOutputs(outputs), nil
}

func (j LLMJudge) Compare(ctx context.Context, input PairwiseJudgeInput) (PairwiseJudgeOutput, error) {
	if j.Model == nil {
		return PairwiseJudgeOutput{}, apperrors.New(apperrors.CodeValidation, "judge model is required")
	}
	models := j.pairwiseModels()
	repeats := max(1, j.Config.Repeat)
	outputs := make([]PairwiseJudgeOutput, 0, len(models)*repeats)
	limiter := newJudgeCallLimiter(j.Config.MaxJudgeCalls)
	for _, model := range models {
		for i := 0; i < repeats; i++ {
			out, err := j.comparePair(ctx, model, input, limiter)
			if err != nil {
				return PairwiseJudgeOutput{}, err
			}
			outputs = append(outputs, out)
		}
	}
	return aggregatePairwiseOutputs(outputs), nil
}

func (j LLMJudge) ScoreQAG(ctx context.Context, input JudgeInput) (QAGOutput, error) {
	if j.Model == nil {
		return QAGOutput{}, apperrors.New(apperrors.CodeValidation, "judge model is required")
	}
	prompt, err := RenderQAGPrompt(input, j.Config)
	if err != nil {
		return QAGOutput{}, err
	}
	limiter := newJudgeCallLimiter(j.Config.MaxJudgeCalls)
	resp, err := j.callChat(ctx, j.Model, prompt, limiter)
	if err != nil {
		return QAGOutput{}, err
	}
	out, err := parseQAGOutput(resp.Content, input.ExpectedEvidence)
	if err != nil {
		return QAGOutput{}, err
	}
	out.RawResponse = resp.Content
	out.TokenUsage = resp.TokenUsage
	out.JudgeModel = firstNonEmpty(resp.Model, j.Config.Model)
	out.PromptVersion = j.Config.PromptVersion
	out.RubricHash = HashJudgeRubric(firstRubric(input.Rubric, j.Config.Rubric))
	out.ConfigHash = HashJudgeConfig(j.Config)
	out.CreatedAt = j.now()
	return out, nil
}

func (j LLMJudge) comparePair(ctx context.Context, model JudgeChatModel, input PairwiseJudgeInput, limiter *judgeCallLimiter) (PairwiseJudgeOutput, error) {
	first, err := j.compareOnce(ctx, model, input, limiter)
	if err != nil {
		return PairwiseJudgeOutput{}, err
	}
	if !j.Config.PairwiseSwap {
		first.Stable = true
		first.VoteCount = 1
		return first, nil
	}
	swappedInput := input
	swappedInput.AnswerA = input.AnswerB
	swappedInput.AnswerB = input.AnswerA
	swapped, err := j.compareOnce(ctx, model, swappedInput, limiter)
	if err != nil {
		return PairwiseJudgeOutput{}, err
	}
	swapped = mapSwappedPairwise(swapped)
	first.Stable = pairwiseAgrees(first, swapped)
	if !first.Stable {
		first.Preference = "unstable"
		first.Reasons = append(first.Reasons, unstablePairwiseFinding("judge preference changed after A/B order swap"))
	}
	first.PreferenceStrength = averagePreferenceStrength([]PairwiseJudgeOutput{first, swapped})
	first.VoteCount = 1
	first.RawResponse = strings.Join([]string{first.RawResponse, swapped.RawResponse}, "\n--- swapped ---\n")
	first.TokenUsage = addTokenUsage(first.TokenUsage, swapped.TokenUsage)
	return first, nil
}

func (j LLMJudge) compareOnce(ctx context.Context, model JudgeChatModel, input PairwiseJudgeInput, limiter *judgeCallLimiter) (PairwiseJudgeOutput, error) {
	prompt, err := RenderPairwiseJudgePrompt(input, j.Config)
	if err != nil {
		return PairwiseJudgeOutput{}, err
	}
	resp, err := j.callChat(ctx, model, prompt, limiter)
	if err != nil {
		return PairwiseJudgeOutput{}, err
	}
	out, err := parsePairwiseJudgeOutput(resp.Content)
	if err != nil {
		return PairwiseJudgeOutput{}, err
	}
	out.RawResponse = resp.Content
	out.TokenUsage = resp.TokenUsage
	out.JudgeModel = firstNonEmpty(resp.Model, j.Config.Model)
	out.PromptVersion = j.Config.PromptVersion
	out.RubricHash = HashJudgeRubric(firstRubric(input.Rubric, j.Config.Rubric))
	out.ConfigHash = HashJudgeConfig(j.Config)
	out.CreatedAt = j.now()
	return out, nil
}

type judgeResponseJSON struct {
	Scores     map[string]float64            `json:"scores"`
	Labels     map[string]string             `json:"labels"`
	Confidence map[string]ConfidenceInterval `json:"confidence"`
	Pass       bool                          `json:"pass"`
	Rationale  string                        `json:"rationale"`
	Findings   []JudgeFinding                `json:"findings"`
}

func parseJudgeOutput(raw string) (JudgeOutput, error) {
	var parsed judgeResponseJSON
	obj, err := decodeStrictJSONObject(raw, &parsed)
	if err != nil {
		return JudgeOutput{}, err
	}
	scores := normalizeScores(parsed.Scores)
	if err := ValidateMetricMap(scores); err != nil {
		return JudgeOutput{}, err
	}
	confidence := normalizeConfidence(parsed.Confidence)
	labels := parsed.Labels
	if labels == nil {
		labels = map[string]string{}
	}
	for name, score := range scores {
		if labels[name] == "" {
			labels[name] = coarseLabel(score)
		}
	}
	return JudgeOutput{
		Scores:     scores,
		Labels:     labels,
		Confidence: confidence,
		Pass:       parsed.Pass,
		Rationale:  parsed.Rationale,
		Findings:   parsed.Findings,
		ParsedJSON: obj,
	}, nil
}

type pairwiseResponseJSON struct {
	Winner     string         `json:"winner"`
	Preference string         `json:"preference"`
	Reasons    []JudgeFinding `json:"reasons"`
}

func parsePairwiseJudgeOutput(raw string) (PairwiseJudgeOutput, error) {
	var parsed pairwiseResponseJSON
	obj, err := decodeStrictJSONObject(raw, &parsed)
	if err != nil {
		return PairwiseJudgeOutput{}, err
	}
	winner := strings.ToUpper(strings.TrimSpace(parsed.Winner))
	if winner != "A" && winner != "B" && strings.ToLower(winner) != "tie" {
		return PairwiseJudgeOutput{}, apperrors.New(apperrors.CodeValidation, "pairwise winner must be A, B, or tie")
	}
	if strings.ToLower(winner) == "tie" {
		winner = "tie"
	}
	return PairwiseJudgeOutput{
		Winner:             winner,
		Preference:         strings.TrimSpace(parsed.Preference),
		PreferenceStrength: math.Abs(preferenceStrength(parsed.Preference, winner)),
		Reasons:            parsed.Reasons,
		ParsedJSON:         obj,
	}, nil
}

type qagResponseJSON struct {
	Score  float64    `json:"score"`
	Claims []QAGClaim `json:"claims"`
}

const (
	qagVerdictSupported    = "supported"
	qagVerdictContradicted = "contradicted"
	qagVerdictUnverifiable = "unverifiable"
)

func parseQAGOutput(raw string, expectedEvidence []string) (QAGOutput, error) {
	var parsed qagResponseJSON
	obj, err := decodeStrictJSONObject(raw, &parsed)
	if err != nil {
		return QAGOutput{}, err
	}
	claims, err := normalizeQAGClaims(parsed.Claims)
	if err != nil {
		return QAGOutput{}, err
	}
	score, metrics := summarizeQAGClaims(claims, expectedEvidence)
	if len(claims) == 0 {
		score = clamp01(parsed.Score)
		metrics["qag_score"] = score
	}
	if err := ValidateMetricMap(metrics); err != nil {
		return QAGOutput{}, err
	}
	return QAGOutput{Score: score, Metrics: metrics, Claims: claims, ParsedJSON: obj}, nil
}

func normalizeQAGClaims(claims []QAGClaim) ([]QAGClaim, error) {
	normalized := make([]QAGClaim, 0, len(claims))
	for _, claim := range claims {
		claim.Claim = strings.TrimSpace(claim.Claim)
		claim.Question = strings.TrimSpace(claim.Question)
		claim.Answer = strings.TrimSpace(claim.Answer)
		claim.Evidence = strings.TrimSpace(claim.Evidence)
		claim.Verdict = strings.ToLower(strings.TrimSpace(claim.Verdict))
		switch claim.Verdict {
		case qagVerdictSupported, qagVerdictContradicted, qagVerdictUnverifiable:
			normalized = append(normalized, claim)
		default:
			return nil, apperrors.New(apperrors.CodeValidation, "QAG claim verdict must be supported, contradicted, or unverifiable")
		}
	}
	return normalized, nil
}

func summarizeQAGClaims(claims []QAGClaim, expectedEvidence []string) (float64, map[string]float64) {
	total := len(claims)
	supported := 0
	unverifiable := 0
	questionCount := 0
	for _, claim := range claims {
		if claim.Verdict == qagVerdictSupported {
			supported++
		}
		if claim.Verdict == qagVerdictUnverifiable {
			unverifiable++
		}
		if claim.Question != "" {
			questionCount++
		}
	}
	score := average(float64(supported), total)
	metrics := map[string]float64{
		"qag_score":             score,
		"qag_claim_coverage":    qagClaimCoverage(claims, expectedEvidence),
		"qag_question_count":    float64(questionCount),
		"qag_unverifiable_rate": average(float64(unverifiable), total),
	}
	return score, metrics
}

func qagClaimCoverage(claims []QAGClaim, expectedEvidence []string) float64 {
	expected := nonEmptyStringSlice(expectedEvidence)
	if len(expected) == 0 {
		if len(claims) > 0 {
			return 1
		}
		return 0
	}
	covered := 0
	for _, evidence := range expected {
		if qagEvidenceCovered(evidence, claims) {
			covered++
		}
	}
	return float64(covered) / float64(len(expected))
}

func qagEvidenceCovered(expected string, claims []QAGClaim) bool {
	needle := normalizeDuplicateText(expected)
	if needle == "" {
		return false
	}
	for _, claim := range claims {
		haystack := normalizeDuplicateText(strings.Join([]string{
			claim.Claim,
			claim.Question,
			claim.Answer,
			claim.Evidence,
		}, " "))
		if haystack == "" {
			continue
		}
		if strings.Contains(haystack, needle) || strings.Contains(needle, haystack) {
			return true
		}
		if tokenCoverage(needle, haystack) >= 0.6 {
			return true
		}
	}
	return false
}

func tokenCoverage(needle, haystack string) float64 {
	terms := strings.Fields(needle)
	if len(terms) == 0 {
		return 0
	}
	haystackTerms := stringSet(strings.Fields(haystack))
	matches := 0
	for _, term := range terms {
		if _, ok := haystackTerms[term]; ok {
			matches++
		}
	}
	return float64(matches) / float64(len(terms))
}

func nonEmptyStringSlice(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
	return out
}

func decodeStrictJSONObject(raw string, dest any) (map[string]any, error) {
	if len(raw) > maxJudgeResponseBytes {
		return nil, apperrors.New(apperrors.CodeValidation, "judge response is too large")
	}
	payload, err := extractJSONObject(raw)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader([]byte(payload)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dest); err != nil {
		return nil, apperrors.Wrap(apperrors.CodeValidation, "invalid judge JSON", err)
	}
	if dec.More() {
		return nil, apperrors.New(apperrors.CodeValidation, "invalid judge JSON")
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(payload), &obj); err != nil {
		return nil, apperrors.Wrap(apperrors.CodeValidation, "invalid judge JSON", err)
	}
	return obj, nil
}

func extractJSONObject(raw string) (string, error) {
	raw = strings.TrimSpace(strings.Trim(raw, "`"))
	if raw == "" {
		return "", apperrors.New(apperrors.CodeValidation, "empty judge response")
	}
	start := strings.IndexByte(raw, '{')
	if start < 0 {
		return "", apperrors.New(apperrors.CodeValidation, "judge response does not contain JSON object")
	}
	inString := false
	escaped := false
	depth := 0
	for i := start; i < len(raw); i++ {
		ch := raw[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				tail := strings.TrimSpace(raw[i+1:])
				if tail != "" && tail != "```" {
					return "", apperrors.New(apperrors.CodeValidation, "judge response contains trailing non-JSON content")
				}
				return raw[start : i+1], nil
			}
		}
	}
	return "", apperrors.New(apperrors.CodeValidation, "judge response contains incomplete JSON object")
}

func aggregateJudgeOutputs(outputs []JudgeOutput) JudgeOutput {
	if len(outputs) == 0 {
		return JudgeOutput{}
	}
	if len(outputs) == 1 {
		return outputs[0]
	}
	base := outputs[0]
	scoreValues := map[string][]float64{}
	labelVotes := map[string]map[string]int{}
	for _, output := range outputs {
		for name, value := range output.Scores {
			scoreValues[name] = append(scoreValues[name], value)
		}
		for name, label := range output.Labels {
			if labelVotes[name] == nil {
				labelVotes[name] = map[string]int{}
			}
			labelVotes[name][label]++
		}
		base.TokenUsage.PromptTokens += output.TokenUsage.PromptTokens
		base.TokenUsage.CompletionTokens += output.TokenUsage.CompletionTokens
		base.TokenUsage.TotalTokens += output.TokenUsage.TotalTokens
		base.CostUSD += output.CostUSD
	}
	base.Scores = map[string]float64{}
	for name, values := range scoreValues {
		base.Scores[name] = median(values)
	}
	base.Labels = map[string]string{}
	for name, votes := range labelVotes {
		base.Labels[name] = majorityLabel(votes)
	}
	return base
}

func aggregatePairwiseOutputs(outputs []PairwiseJudgeOutput) PairwiseJudgeOutput {
	if len(outputs) == 0 {
		return PairwiseJudgeOutput{}
	}
	if len(outputs) == 1 {
		return outputs[0]
	}
	base := outputs[0]
	base.TokenUsage = TokenUsage{}
	votes := map[string]int{}
	var stableOutputs []PairwiseJudgeOutput
	for _, output := range outputs {
		base.TokenUsage = addTokenUsage(base.TokenUsage, output.TokenUsage)
		base.CostUSD += output.CostUSD
		if output.RawResponse != "" && output.RawResponse != base.RawResponse {
			base.RawResponse = strings.Join(nonEmptyStrings(base.RawResponse, output.RawResponse), "\n--- repeat ---\n")
		}
		base.Reasons = append(base.Reasons, output.Reasons...)
		if !output.Stable {
			continue
		}
		votes[output.Winner]++
		stableOutputs = append(stableOutputs, output)
	}
	winner, count := majorityWinner(votes)
	base.VoteCount = count
	if count == 0 || count*2 <= len(outputs) {
		base.Winner = "tie"
		base.Preference = "unstable"
		base.PreferenceStrength = 0
		base.Stable = false
		base.Reasons = append(base.Reasons, unstablePairwiseFinding("pairwise ensemble/repeat did not reach majority"))
		return base
	}
	base.Winner = winner
	base.Stable = true
	base.PreferenceStrength = math.Abs(averagePreferenceStrength(stableOutputs))
	base.Preference = preferenceFromWinnerAndStrength(winner, base.PreferenceStrength)
	return base
}

func mapSwappedPairwise(out PairwiseJudgeOutput) PairwiseJudgeOutput {
	switch out.Winner {
	case "A":
		out.Winner = "B"
	case "B":
		out.Winner = "A"
	}
	switch out.Preference {
	case "A_much_better":
		out.Preference = "B_much_better"
	case "A_better":
		out.Preference = "B_better"
	case "B_better":
		out.Preference = "A_better"
	case "B_much_better":
		out.Preference = "A_much_better"
	}
	return out
}

func pairwiseAgrees(left, right PairwiseJudgeOutput) bool {
	if left.Winner == "tie" && right.Winner == "tie" {
		return true
	}
	return left.Winner == right.Winner && left.Preference != "" && right.Preference != ""
}

func preferenceStrength(preference, winner string) float64 {
	switch preference {
	case "A_much_better":
		return 1
	case "A_better":
		return 0.5
	case "B_better":
		return -0.5
	case "B_much_better":
		return -1
	}
	switch winner {
	case "A":
		return 0.5
	case "B":
		return -0.5
	default:
		return 0
	}
}

func averagePreferenceStrength(outputs []PairwiseJudgeOutput) float64 {
	if len(outputs) == 0 {
		return 0
	}
	var sum float64
	for _, output := range outputs {
		sum += preferenceStrength(output.Preference, output.Winner)
	}
	return sum / float64(len(outputs))
}

func preferenceFromWinnerAndStrength(winner string, strength float64) string {
	if winner == "tie" || strength < 0.25 {
		return "tie"
	}
	if winner == "A" {
		if strength >= 0.75 {
			return "A_much_better"
		}
		return "A_better"
	}
	if strength >= 0.75 {
		return "B_much_better"
	}
	return "B_better"
}

func majorityWinner(votes map[string]int) (string, int) {
	bestWinner := ""
	bestCount := 0
	for winner, count := range votes {
		if count > bestCount || count == bestCount && winner < bestWinner {
			bestWinner = winner
			bestCount = count
		}
	}
	return bestWinner, bestCount
}

func unstablePairwiseFinding(message string) JudgeFinding {
	return JudgeFinding{
		Metric:   "pairwise",
		Label:    "unstable",
		Severity: "warning",
		Message:  message,
	}
}

func addTokenUsage(left, right TokenUsage) TokenUsage {
	return TokenUsage{
		PromptTokens:     left.PromptTokens + right.PromptTokens,
		CompletionTokens: left.CompletionTokens + right.CompletionTokens,
		TotalTokens:      left.TotalTokens + right.TotalTokens,
	}
}

func nonEmptyStrings(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func normalizeScores(scores map[string]float64) map[string]float64 {
	out := make(map[string]float64, len(scores))
	for name, value := range scores {
		out[name] = clamp01(value)
	}
	return out
}

func normalizeConfidence(conf map[string]ConfidenceInterval) map[string]ConfidenceInterval {
	out := make(map[string]ConfidenceInterval, len(conf))
	for name, interval := range conf {
		interval.Mean = clamp01(interval.Mean)
		interval.Low = clamp01(interval.Low)
		interval.High = clamp01(interval.High)
		out[name] = interval
	}
	return out
}

func clamp01(value float64) float64 {
	if math.IsNaN(value) || value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func coarseLabel(score float64) string {
	switch {
	case score >= 0.8:
		return "good"
	case score >= 0.5:
		return "mixed"
	default:
		return "bad"
	}
}

func HashJudgeRubric(rubric JudgeRubric) string {
	return stableHash(rubric)
}

func HashJudgeConfig(cfg JudgeConfig) string {
	return stableHash(cfg)
}

func stableHash(v any) string {
	payload, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func median(values []float64) float64 {
	cp := append([]float64(nil), values...)
	sort.Float64s(cp)
	mid := len(cp) / 2
	if len(cp)%2 == 1 {
		return cp[mid]
	}
	return (cp[mid-1] + cp[mid]) / 2
}

func majorityLabel(votes map[string]int) string {
	bestLabel := ""
	bestCount := -1
	for label, count := range votes {
		if count > bestCount || count == bestCount && label < bestLabel {
			bestLabel = label
			bestCount = count
		}
	}
	return bestLabel
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (j LLMJudge) pairwiseModels() []JudgeChatModel {
	models := []JudgeChatModel{j.Model}
	models = append(models, j.Ensemble...)
	return models
}

func (j LLMJudge) callChat(ctx context.Context, model JudgeChatModel, prompt string, limiter *judgeCallLimiter) (JudgeChatResponse, error) {
	if model == nil {
		return JudgeChatResponse{}, apperrors.New(apperrors.CodeValidation, "judge model is required")
	}
	retries := j.Config.MaxRetries
	if retries < 0 {
		retries = 0
	}
	consecutiveFailures := 0
	var lastErr error
	for attempt := 0; attempt <= retries; attempt++ {
		if err := limiter.take(); err != nil {
			return JudgeChatResponse{}, err
		}
		callCtx := ctx
		cancel := func() {}
		if j.Config.Timeout > 0 {
			callCtx, cancel = context.WithTimeout(ctx, j.Config.Timeout)
		}
		resp, err := model.Chat(callCtx, prompt)
		cancel()
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if !isRetriableJudgeError(err) || attempt == retries {
			return JudgeChatResponse{}, err
		}
		consecutiveFailures++
		if j.Config.CircuitFailures > 0 && consecutiveFailures >= j.Config.CircuitFailures {
			return JudgeChatResponse{}, apperrors.Wrap(apperrors.CodeUpstreamUnavailable, "judge circuit breaker open", err)
		}
		if err := sleepWithContext(ctx, j.retryBackoff(attempt)); err != nil {
			return JudgeChatResponse{}, err
		}
	}
	return JudgeChatResponse{}, lastErr
}

func (j LLMJudge) retryBackoff(attempt int) time.Duration {
	backoff := j.Config.BackoffInitial
	if backoff <= 0 {
		backoff = 50 * time.Millisecond
	}
	for i := 0; i < attempt; i++ {
		backoff *= 2
	}
	if j.Config.BackoffMax > 0 && backoff > j.Config.BackoffMax {
		backoff = j.Config.BackoffMax
	}
	if j.Config.BackoffJitter > 0 {
		backoff += time.Duration(float64(backoff) * math.Min(j.Config.BackoffJitter, 1) / 2)
	}
	return backoff
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func isRetriableJudgeError(err error) bool {
	return apperrors.IsCode(err, apperrors.CodeRateLimited) ||
		apperrors.IsCode(err, apperrors.CodeUpstreamUnavailable)
}

type judgeCallLimiter struct {
	max  int
	used int
}

func newJudgeCallLimiter(maxCalls int) *judgeCallLimiter {
	return &judgeCallLimiter{max: maxCalls}
}

func (l *judgeCallLimiter) take() error {
	if l == nil || l.max <= 0 {
		return nil
	}
	if l.used >= l.max {
		return apperrors.New(apperrors.CodeRateLimited, "judge budget stopped: max_judge_calls reached")
	}
	l.used++
	return nil
}

func (j LLMJudge) now() time.Time {
	if j.Now != nil {
		return j.Now().UTC()
	}
	return time.Now().UTC()
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
