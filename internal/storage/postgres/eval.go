package postgres

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"github.com/shikanon/orag/internal/dataset"
	evalpkg "github.com/shikanon/orag/internal/eval"
	"github.com/shikanon/orag/internal/platform/id"
)

func (r *Repository) StoreEvaluationRun(ctx context.Context, tenantID string, result evalpkg.RunResult) error {
	body, err := encodeEvaluationRunMetrics(result)
	if err != nil {
		return err
	}
	_, err = r.evaluationQueryer().Exec(ctx, `
		INSERT INTO evaluation_runs(id, tenant_id, project_id, dataset_id, profile, metrics, created_at)
		VALUES($1,$2,NULLIF($3,''),$4,$5,$6,$7)
		ON CONFLICT (id) DO NOTHING`,
		result.ID, tenantID, result.ProjectID, result.DatasetID, result.Profile, body, result.CreatedAt)
	return err
}

func encodeEvaluationRunMetrics(result evalpkg.RunResult) ([]byte, error) {
	if err := evalpkg.ValidateMetricMap(result.Metrics); err != nil {
		return nil, err
	}
	metrics := map[string]any{
		"total":                   result.Total,
		"hit_rate":                result.HitRate,
		"accuracy":                result.Accuracy,
		"knowledge_base_id":       result.KnowledgeBaseID,
		"top_k":                   result.TopK,
		"weighted_sample_count":   result.WeightedSampleCount,
		"unweighted_sample_count": result.UnweightedSampleCount,
		"missing_split":           result.MissingSplit,
	}
	if result.Split != "" {
		metrics["split"] = result.Split
	}
	if len(result.SplitSummary) > 0 {
		metrics["split_summary"] = result.SplitSummary
	}
	if result.HoldoutGate.Enabled {
		metrics["holdout_gate"] = result.HoldoutGate
	}
	if len(result.MetricSummaries) > 0 {
		metrics["metric_summaries"] = result.MetricSummaries
	}
	if result.DatasetSnapshot.DatasetID != "" {
		metrics["dataset_snapshot"] = result.DatasetSnapshot
	}
	if result.Manifest.SchemaVersion != "" {
		metrics["manifest"] = result.Manifest
	}
	if result.EvaluationFingerprint != "" {
		metrics["evaluation_fingerprint"] = result.EvaluationFingerprint
	}
	for key, value := range result.Metrics {
		metrics[key] = value
	}
	return json.Marshal(metrics)
}

func (r *Repository) StoreEvaluationResult(ctx context.Context, runID, datasetItemID, answer string, metrics map[string]float64) error {
	if err := evalpkg.ValidateMetricMap(metrics); err != nil {
		return err
	}
	body, err := json.Marshal(metrics)
	if err != nil {
		return err
	}
	_, err = r.evaluationQueryer().Exec(ctx, `
		INSERT INTO evaluation_results(id, run_id, dataset_item_id, answer, metrics)
		VALUES($1,$2,$3,$4,$5)`,
		id.New("evalr"), runID, datasetItemID, answer, body)
	return err
}

func (r *Repository) GetEvaluationRun(ctx context.Context, tenantID, id string) (evalpkg.RunResult, bool, error) {
	row := r.evaluationQueryer().QueryRow(ctx, `
		SELECT id, COALESCE(project_id,''), dataset_id, profile, metrics, created_at
		FROM evaluation_runs
		WHERE tenant_id=$1 AND id=$2`, tenantID, id)
	var result evalpkg.RunResult
	var metrics []byte
	if err := row.Scan(&result.ID, &result.ProjectID, &result.DatasetID, &result.Profile, &metrics, &result.CreatedAt); err != nil {
		if err == pgx.ErrNoRows {
			return evalpkg.RunResult{}, false, nil
		}
		return evalpkg.RunResult{}, false, err
	}
	decodeEvaluationRunMetrics(metrics, &result)
	return result, true, nil
}

func (r *Repository) GetEvaluationRunInProject(ctx context.Context, tenantID, projectID, id string) (evalpkg.RunResult, bool, error) {
	row := r.evaluationQueryer().QueryRow(ctx, `
		SELECT id, COALESCE(project_id,''), dataset_id, profile, metrics, created_at
		FROM evaluation_runs
		WHERE tenant_id=$1 AND project_id=$2 AND id=$3`, tenantID, projectID, id)
	var result evalpkg.RunResult
	var metrics []byte
	if err := row.Scan(&result.ID, &result.ProjectID, &result.DatasetID, &result.Profile, &metrics, &result.CreatedAt); err != nil {
		if err == pgx.ErrNoRows {
			return evalpkg.RunResult{}, false, nil
		}
		return evalpkg.RunResult{}, false, err
	}
	decodeEvaluationRunMetrics(metrics, &result)
	return result, true, nil
}

func (r *Repository) StoreJudgeRun(ctx context.Context, tenantID string, run evalpkg.JudgeRunRecord) error {
	rubric, err := json.Marshal(run.Rubric)
	if err != nil {
		return err
	}
	params, err := json.Marshal(run.JudgeParams)
	if err != nil {
		return err
	}
	ensemble, err := json.Marshal(run.Ensemble)
	if err != nil {
		return err
	}
	_, err = r.evaluationQueryer().Exec(ctx, `
		INSERT INTO judge_runs(
			id, tenant_id, evaluation_run_id, judge_provider, judge_model, prompt_version,
			rubric_hash, prompt_hash, judge_params_hash, mode, comparison_mode,
			rubric, judge_params, ensemble, created_at
		)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		ON CONFLICT (id) DO NOTHING`,
		run.ID, tenantID, run.EvaluationRunID, run.Provider, run.Model, run.PromptVersion,
		run.RubricHash, run.PromptHash, run.ConfigHash, run.Mode, run.ComparisonMode,
		rubric, params, ensemble, run.CreatedAt)
	return err
}

func (r *Repository) StoreJudgeResult(ctx context.Context, result evalpkg.JudgeResultRecord) error {
	if err := evalpkg.ValidateMetricMap(result.Scores); err != nil {
		return err
	}
	scores, err := json.Marshal(result.Scores)
	if err != nil {
		return err
	}
	findings, err := json.Marshal(result.Findings)
	if err != nil {
		return err
	}
	parsed, err := json.Marshal(result.ParsedJSON)
	if err != nil {
		return err
	}
	confidence, err := json.Marshal(result.Confidence)
	if err != nil {
		return err
	}
	tokenUsage, err := json.Marshal(result.TokenUsage)
	if err != nil {
		return err
	}
	_, err = r.evaluationQueryer().Exec(ctx, `
		INSERT INTO judge_results(
			id, judge_run_id, dataset_item_id, candidate_id, scores, pass, rationale,
			findings, raw_response, parsed_response, confidence, token_usage, cost_usd, created_at
		)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		result.ID, result.JudgeRunID, result.DatasetItemID, result.CandidateID, scores,
		result.Pass, result.Rationale, findings, result.RawResponse, parsed,
		confidence, tokenUsage, result.CostUSD, result.CreatedAt)
	return err
}

func (r *Repository) StorePairwiseJudgeResult(ctx context.Context, result evalpkg.PairwiseJudgeResultRecord) error {
	reasons, err := json.Marshal(result.Reasons)
	if err != nil {
		return err
	}
	parsed, err := json.Marshal(result.ParsedJSON)
	if err != nil {
		return err
	}
	tokenUsage, err := json.Marshal(result.TokenUsage)
	if err != nil {
		return err
	}
	_, err = r.evaluationQueryer().Exec(ctx, `
		INSERT INTO pairwise_judge_results(
			id, judge_run_id, dataset_item_id, candidate_a_id, candidate_b_id,
			winner, preference, reasons, raw_response, parsed_response, token_usage, cost_usd, created_at
		)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		result.ID, result.JudgeRunID, result.DatasetItemID, result.CandidateAID, result.CandidateBID,
		result.Winner, result.Preference, reasons, result.RawResponse, parsed,
		tokenUsage, result.CostUSD, result.CreatedAt)
	return err
}

func (r *Repository) StoreJudgeCalibrationRun(ctx context.Context, tenantID string, run evalpkg.JudgeCalibrationRunRecord) error {
	if err := evalpkg.ValidateMetricMap(run.Metrics); err != nil {
		return err
	}
	metrics, err := json.Marshal(run.Metrics)
	if err != nil {
		return err
	}
	_, err = r.evaluationQueryer().Exec(ctx, `
		INSERT INTO judge_calibration_runs(
			id, tenant_id, dataset_id, judge_config_hash, human_score_version,
			spearman, cohen_kappa, sample_count, metrics, created_at
		)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		run.ID, tenantID, run.DatasetID, run.JudgeConfigHash, run.HumanScoreVersion,
		run.Spearman, run.CohenKappa, run.SampleCount, metrics, run.CreatedAt)
	return err
}

func (r *Repository) GetEvaluationDetail(ctx context.Context, tenantID, id string, options evalpkg.EvaluationDetailOptions) (evalpkg.EvaluationDetail, bool, error) {
	run, ok, err := r.GetEvaluationRun(ctx, tenantID, id)
	if err != nil || !ok {
		return evalpkg.EvaluationDetail{}, ok, err
	}
	detail := evalpkg.EvaluationDetail{Run: run}
	if options.IncludeItems {
		items, err := r.evaluationItems(ctx, id)
		if err != nil {
			return evalpkg.EvaluationDetail{}, false, err
		}
		detail.Items = items
	}
	if options.IncludeJudge {
		runs, err := r.judgeRuns(ctx, tenantID, id)
		if err != nil {
			return evalpkg.EvaluationDetail{}, false, err
		}
		results, err := r.judgeResults(ctx, tenantID, id)
		if err != nil {
			return evalpkg.EvaluationDetail{}, false, err
		}
		calibrations, err := r.judgeCalibrationRuns(ctx, tenantID, run.DatasetID)
		if err != nil {
			return evalpkg.EvaluationDetail{}, false, err
		}
		detail.JudgeRuns = runs
		detail.JudgeResults = results
		detail.CalibrationRuns = calibrations
	}
	if options.IncludePairwise {
		results, err := r.pairwiseJudgeResults(ctx, tenantID, id)
		if err != nil {
			return evalpkg.EvaluationDetail{}, false, err
		}
		detail.PairwiseResults = results
	}
	return detail, true, nil
}

func (r *Repository) evaluationItems(ctx context.Context, runID string) ([]evalpkg.EvaluationItemDetail, error) {
	rows, err := r.evaluationQueryer().Query(ctx, `
		SELECT run_id, dataset_item_id, answer, metrics
		FROM evaluation_results
		WHERE run_id=$1
		ORDER BY id`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []evalpkg.EvaluationItemDetail
	for rows.Next() {
		var item evalpkg.EvaluationItemDetail
		var metrics []byte
		if err := rows.Scan(&item.RunID, &item.DatasetItemID, &item.Answer, &metrics); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(metrics, &item.Metrics)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *Repository) judgeRuns(ctx context.Context, tenantID, evaluationRunID string) ([]evalpkg.JudgeRunRecord, error) {
	rows, err := r.evaluationQueryer().Query(ctx, `
		SELECT id, evaluation_run_id, judge_provider, judge_model, prompt_version,
			rubric_hash, prompt_hash, judge_params_hash, mode, comparison_mode,
			rubric, judge_params, ensemble, created_at
		FROM judge_runs
		WHERE tenant_id=$1 AND evaluation_run_id=$2
		ORDER BY created_at, id`, tenantID, evaluationRunID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []evalpkg.JudgeRunRecord
	for rows.Next() {
		var run evalpkg.JudgeRunRecord
		var rubric, params, ensemble []byte
		if err := rows.Scan(&run.ID, &run.EvaluationRunID, &run.Provider, &run.Model, &run.PromptVersion,
			&run.RubricHash, &run.PromptHash, &run.ConfigHash, &run.Mode, &run.ComparisonMode,
			&rubric, &params, &ensemble, &run.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(rubric, &run.Rubric)
		_ = json.Unmarshal(params, &run.JudgeParams)
		_ = json.Unmarshal(ensemble, &run.Ensemble)
		out = append(out, run)
	}
	return out, rows.Err()
}

func (r *Repository) judgeResults(ctx context.Context, tenantID, evaluationRunID string) ([]evalpkg.JudgeResultRecord, error) {
	rows, err := r.evaluationQueryer().Query(ctx, `
		SELECT jr.id, jr.judge_run_id, jr.dataset_item_id, jr.candidate_id, jr.scores,
			jr.pass, jr.rationale, jr.findings, jr.raw_response, jr.parsed_response,
			jr.confidence, jr.token_usage, jr.cost_usd, jr.created_at
		FROM judge_results jr
		JOIN judge_runs jrun ON jrun.id=jr.judge_run_id
		WHERE jrun.tenant_id=$1 AND jrun.evaluation_run_id=$2
		ORDER BY jr.created_at, jr.id`, tenantID, evaluationRunID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []evalpkg.JudgeResultRecord
	for rows.Next() {
		var result evalpkg.JudgeResultRecord
		var scores, findings, parsed, confidence, tokenUsage []byte
		if err := rows.Scan(&result.ID, &result.JudgeRunID, &result.DatasetItemID, &result.CandidateID, &scores,
			&result.Pass, &result.Rationale, &findings, &result.RawResponse, &parsed,
			&confidence, &tokenUsage, &result.CostUSD, &result.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(scores, &result.Scores)
		_ = json.Unmarshal(findings, &result.Findings)
		_ = json.Unmarshal(parsed, &result.ParsedJSON)
		_ = json.Unmarshal(confidence, &result.Confidence)
		_ = json.Unmarshal(tokenUsage, &result.TokenUsage)
		out = append(out, result)
	}
	return out, rows.Err()
}

func (r *Repository) pairwiseJudgeResults(ctx context.Context, tenantID, evaluationRunID string) ([]evalpkg.PairwiseJudgeResultRecord, error) {
	rows, err := r.evaluationQueryer().Query(ctx, `
		SELECT pr.id, pr.judge_run_id, pr.dataset_item_id, pr.candidate_a_id, pr.candidate_b_id,
			pr.winner, pr.preference, pr.reasons, pr.raw_response, pr.parsed_response,
			pr.token_usage, pr.cost_usd, pr.created_at
		FROM pairwise_judge_results pr
		JOIN judge_runs jrun ON jrun.id=pr.judge_run_id
		WHERE jrun.tenant_id=$1 AND jrun.evaluation_run_id=$2
		ORDER BY pr.created_at, pr.id`, tenantID, evaluationRunID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []evalpkg.PairwiseJudgeResultRecord
	for rows.Next() {
		var result evalpkg.PairwiseJudgeResultRecord
		var reasons, parsed, tokenUsage []byte
		if err := rows.Scan(&result.ID, &result.JudgeRunID, &result.DatasetItemID, &result.CandidateAID, &result.CandidateBID,
			&result.Winner, &result.Preference, &reasons, &result.RawResponse, &parsed,
			&tokenUsage, &result.CostUSD, &result.CreatedAt); err != nil {
			return nil, err
		}
		result.Stable = result.Preference != "unstable"
		_ = json.Unmarshal(reasons, &result.Reasons)
		_ = json.Unmarshal(parsed, &result.ParsedJSON)
		_ = json.Unmarshal(tokenUsage, &result.TokenUsage)
		out = append(out, result)
	}
	return out, rows.Err()
}

func (r *Repository) judgeCalibrationRuns(ctx context.Context, tenantID, datasetID string) ([]evalpkg.JudgeCalibrationRunRecord, error) {
	rows, err := r.evaluationQueryer().Query(ctx, `
		SELECT id, dataset_id, judge_config_hash, human_score_version,
			spearman, cohen_kappa, sample_count, metrics, created_at
		FROM judge_calibration_runs
		WHERE tenant_id=$1 AND dataset_id=$2
		ORDER BY created_at DESC, id`, tenantID, datasetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []evalpkg.JudgeCalibrationRunRecord
	for rows.Next() {
		var run evalpkg.JudgeCalibrationRunRecord
		var metrics []byte
		if err := rows.Scan(&run.ID, &run.DatasetID, &run.JudgeConfigHash, &run.HumanScoreVersion,
			&run.Spearman, &run.CohenKappa, &run.SampleCount, &metrics, &run.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(metrics, &run.Metrics)
		out = append(out, run)
	}
	return out, rows.Err()
}

func decodeEvaluationRunMetrics(metrics []byte, result *evalpkg.RunResult) {
	var decoded struct {
		Total                 int                              `json:"total"`
		HitRate               float64                          `json:"hit_rate"`
		Accuracy              float64                          `json:"accuracy"`
		KnowledgeBaseID       string                           `json:"knowledge_base_id"`
		TopK                  int                              `json:"top_k"`
		WeightedSampleCount   float64                          `json:"weighted_sample_count"`
		UnweightedSampleCount int                              `json:"unweighted_sample_count"`
		Split                 string                           `json:"split"`
		SplitSummary          map[string]evalpkg.SplitSummary  `json:"split_summary"`
		MissingSplit          bool                             `json:"missing_split"`
		HoldoutGate           evalpkg.HoldoutGateResult        `json:"holdout_gate"`
		MetricSummaries       map[string]evalpkg.MetricSummary `json:"metric_summaries"`
		DatasetSnapshot       evalpkg.DatasetSnapshot          `json:"dataset_snapshot"`
		Manifest              evalpkg.EvaluationManifest       `json:"manifest"`
		EvaluationFingerprint string                           `json:"evaluation_fingerprint"`
	}
	_ = json.Unmarshal(metrics, &decoded)
	metricMap := numericEvaluationMetrics(metrics)
	result.Total = decoded.Total
	result.HitRate = decoded.HitRate
	result.Accuracy = decoded.Accuracy
	result.KnowledgeBaseID = decoded.KnowledgeBaseID
	result.TopK = decoded.TopK
	result.WeightedSampleCount = decoded.WeightedSampleCount
	result.UnweightedSampleCount = decoded.UnweightedSampleCount
	result.Split = dataset.DatasetSplit(decoded.Split)
	result.SplitSummary = decoded.SplitSummary
	result.MissingSplit = decoded.MissingSplit
	result.HoldoutGate = decoded.HoldoutGate
	result.MetricSummaries = decoded.MetricSummaries
	result.DatasetSnapshot = decoded.DatasetSnapshot
	result.Manifest = decoded.Manifest
	result.EvaluationFingerprint = decoded.EvaluationFingerprint
	if result.WeightedSampleCount == 0 && result.Total > 0 {
		result.WeightedSampleCount = float64(result.Total)
	}
	if result.UnweightedSampleCount == 0 {
		result.UnweightedSampleCount = result.Total
	}
	result.Metrics = metricMap
}

func numericEvaluationMetrics(metrics []byte) map[string]float64 {
	var raw map[string]any
	_ = json.Unmarshal(metrics, &raw)
	out := make(map[string]float64, len(raw))
	for key, value := range raw {
		if number, ok := value.(float64); ok {
			out[key] = number
		}
	}
	return out
}
