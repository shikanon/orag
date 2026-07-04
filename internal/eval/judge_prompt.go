package eval

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

const defaultJudgePromptVersion = "judge-v1"

func RenderJudgePrompt(input JudgeInput, cfg JudgeConfig) (string, error) {
	rubric := input.Rubric
	if len(rubric.Criteria) == 0 {
		rubric = cfg.Rubric
	}
	parts := map[string]any{
		"query":             input.Query,
		"ground_truth":      input.GroundTruth,
		"expected_evidence": input.ExpectedEvidence,
		"relevant_doc_ids":  input.RelevantDocIDs,
		"answer":            input.Answer,
		"citations":         input.Citations,
		"retrieved_chunks":  input.RetrievedChunks,
		"rubric":            rubric,
		"metrics":           cfg.Metrics,
	}
	payload, err := compactJSON(parts)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{
		"You are an evidence-checking LLM judge for a RAG system.",
		"Reason privately against the rubric, but do not reveal chain-of-thought.",
		"Return only one strict JSON object with keys: scores, labels, confidence, pass, rationale, findings.",
		"Scores must be numbers in [0,1]. Use only registered metric names.",
		"Judge input JSON:",
		payload,
	}, "\n"), nil
}

func RenderPairwiseJudgePrompt(input PairwiseJudgeInput, cfg JudgeConfig) (string, error) {
	rubric := input.Rubric
	if len(rubric.Criteria) == 0 {
		rubric = cfg.Rubric
	}
	parts := map[string]any{
		"query":             input.Query,
		"ground_truth":      input.GroundTruth,
		"expected_evidence": input.ExpectedEvidence,
		"relevant_doc_ids":  input.RelevantDocIDs,
		"answer_a":          input.AnswerA,
		"answer_b":          input.AnswerB,
		"rubric":            rubric,
		"metrics":           cfg.Metrics,
	}
	payload, err := compactJSON(parts)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{
		"You are a pairwise LLM judge for RAG answers.",
		"Compare Answer A and Answer B using only the query, ground truth, citations, retrieved chunks, and rubric.",
		"Reason privately against the rubric, but do not reveal chain-of-thought.",
		`Return only strict JSON: {"winner":"A|B|tie","preference":"A_much_better|A_better|tie|B_better|B_much_better","reasons":[...]}.`,
		"Pairwise input JSON:",
		payload,
	}, "\n"), nil
}

func RenderQAGPrompt(input JudgeInput, cfg JudgeConfig) (string, error) {
	parts := map[string]any{
		"query":             input.Query,
		"answer":            input.Answer,
		"expected_evidence": input.ExpectedEvidence,
		"retrieved_chunks":  input.RetrievedChunks,
		"citations":         input.Citations,
		"rubric":            firstRubric(input.Rubric, cfg.Rubric),
	}
	payload, err := compactJSON(parts)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{
		"You are a QAG verifier.",
		"Extract every key answer claim, generate one claim-verification question per claim, answer each question using only retrieved source context, then label each claim as supported, contradicted, or unverifiable.",
		"Do not use prior knowledge when writing context-only answers. Mark missing or unanswerable context as unverifiable.",
		"Reason privately, but return only strict JSON with keys: score and claims. Each claim must include claim, question, answer, verdict, and evidence.",
		"Score is advisory; the service recomputes qag_score and quality metrics from claim verdicts and expected evidence coverage.",
		"QAG input JSON:",
		payload,
	}, "\n"), nil
}

func compactJSON(v any) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "", fmt.Errorf("render judge payload: %w", err)
	}
	return strings.TrimSpace(buf.String()), nil
}

func firstRubric(primary, fallback JudgeRubric) JudgeRubric {
	if len(primary.Criteria) > 0 || primary.Name != "" || primary.Version != "" {
		return primary
	}
	return fallback
}
