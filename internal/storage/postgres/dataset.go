package postgres

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"github.com/shikanon/orag/internal/dataset"
)

func (r *Repository) CreateDataset(ctx context.Context, ds dataset.Dataset) (dataset.Dataset, error) {
	_, err := r.Pool.Exec(ctx, `
		INSERT INTO datasets(id, tenant_id, name, kind, version, created_at)
		VALUES($1,$2,$3,$4,$5,$6)`,
		ds.ID, ds.TenantID, ds.Name, ds.Kind, ds.Version, ds.CreatedAt)
	return ds, err
}

func (r *Repository) GetDataset(ctx context.Context, tenantID, id string) (dataset.Dataset, bool, error) {
	row := r.Pool.QueryRow(ctx, `
		SELECT id, tenant_id, name, kind, version, created_at
		FROM datasets
		WHERE tenant_id=$1 AND id=$2`, tenantID, id)
	var ds dataset.Dataset
	if err := row.Scan(&ds.ID, &ds.TenantID, &ds.Name, &ds.Kind, &ds.Version, &ds.CreatedAt); err != nil {
		if err == pgx.ErrNoRows {
			return dataset.Dataset{}, false, nil
		}
		return dataset.Dataset{}, false, err
	}
	return ds, true, nil
}

func (r *Repository) AddDatasetItem(ctx context.Context, item dataset.Item) (dataset.Item, error) {
	body, err := json.Marshal(item.RelevantDocIDs)
	if err != nil {
		return dataset.Item{}, err
	}
	_, err = r.Pool.Exec(ctx, `
		INSERT INTO dataset_items(id, dataset_id, query, ground_truth, relevant_doc_ids)
		VALUES($1,$2,$3,$4,$5)`,
		item.ID, item.DatasetID, item.Query, item.GroundTruth, body)
	return item, err
}

func (r *Repository) DatasetItems(ctx context.Context, datasetID string) ([]dataset.Item, error) {
	rows, err := r.Pool.Query(ctx, `
		SELECT id, dataset_id, query, ground_truth, relevant_doc_ids
		FROM dataset_items
		WHERE dataset_id=$1
		ORDER BY id`, datasetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []dataset.Item
	for rows.Next() {
		var item dataset.Item
		var relevant []byte
		if err := rows.Scan(&item.ID, &item.DatasetID, &item.Query, &item.GroundTruth, &relevant); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(relevant, &item.RelevantDocIDs)
		out = append(out, item)
	}
	return out, rows.Err()
}
