package postgres

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	evalpkg "github.com/shikanon/orag/internal/eval"
	"github.com/shikanon/orag/internal/platform/id"
)

func (r *Repository) StoreEvaluationRun(ctx context.Context, tenantID string, result evalpkg.RunResult) error {
	metrics := map[string]any{
		"total":    result.Total,
		"hit_rate": result.HitRate,
		"accuracy": result.Accuracy,
	}
	for key, value := range result.Metrics {
		metrics[key] = value
	}
	body, err := json.Marshal(metrics)
	if err != nil {
		return err
	}
	_, err = r.Pool.Exec(ctx, `
		INSERT INTO evaluation_runs(id, tenant_id, dataset_id, profile, metrics, created_at)
		VALUES($1,$2,$3,$4,$5,$6)
		ON CONFLICT (id) DO NOTHING`,
		result.ID, tenantID, result.DatasetID, result.Profile, body, result.CreatedAt)
	return err
}

func (r *Repository) StoreEvaluationResult(ctx context.Context, runID, datasetItemID, answer string, metrics map[string]float64) error {
	body, err := json.Marshal(metrics)
	if err != nil {
		return err
	}
	_, err = r.Pool.Exec(ctx, `
		INSERT INTO evaluation_results(id, run_id, dataset_item_id, answer, metrics)
		VALUES($1,$2,$3,$4,$5)`,
		id.New("evalr"), runID, datasetItemID, answer, body)
	return err
}

func (r *Repository) GetEvaluationRun(ctx context.Context, tenantID, id string) (evalpkg.RunResult, bool, error) {
	row := r.Pool.QueryRow(ctx, `
		SELECT id, dataset_id, profile, metrics, created_at
		FROM evaluation_runs
		WHERE tenant_id=$1 AND id=$2`, tenantID, id)
	var result evalpkg.RunResult
	var metrics []byte
	if err := row.Scan(&result.ID, &result.DatasetID, &result.Profile, &metrics, &result.CreatedAt); err != nil {
		if err == pgx.ErrNoRows {
			return evalpkg.RunResult{}, false, nil
		}
		return evalpkg.RunResult{}, false, err
	}
	var decoded struct {
		Total    int     `json:"total"`
		HitRate  float64 `json:"hit_rate"`
		Accuracy float64 `json:"accuracy"`
	}
	_ = json.Unmarshal(metrics, &decoded)
	var metricMap map[string]float64
	_ = json.Unmarshal(metrics, &metricMap)
	result.Total = decoded.Total
	result.HitRate = decoded.HitRate
	result.Accuracy = decoded.Accuracy
	result.Metrics = metricMap
	return result, true, nil
}
