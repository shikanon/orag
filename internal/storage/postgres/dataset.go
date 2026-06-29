package postgres

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"github.com/shikanon/orag/internal/dataset"
)

func (r *Repository) CreateDataset(ctx context.Context, ds dataset.Dataset) (dataset.Dataset, error) {
	_, err := r.datasetDB().Exec(ctx, `
			INSERT INTO datasets(id, tenant_id, name, kind, version, created_at)
			VALUES($1,$2,$3,$4,$5,$6)`,
		ds.ID, ds.TenantID, ds.Name, ds.Kind, ds.Version, ds.CreatedAt)
	return ds, err
}

func (r *Repository) GetDataset(ctx context.Context, tenantID, id string) (dataset.Dataset, bool, error) {
	row := r.datasetDB().QueryRow(ctx, `
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

func (r *Repository) AddDatasetItem(ctx context.Context, tenantID string, item dataset.Item) (dataset.Item, error) {
	body, err := json.Marshal(item.RelevantDocIDs)
	if err != nil {
		return dataset.Item{}, err
	}
	tag, err := r.datasetDB().Exec(ctx, `
			INSERT INTO dataset_items(id, dataset_id, query, ground_truth, relevant_doc_ids)
			SELECT $1, d.id, $4, $5, $6
			FROM datasets d
			WHERE d.tenant_id=$2 AND d.id=$3`,
		item.ID, tenantID, item.DatasetID, item.Query, item.GroundTruth, body)
	if err == nil && tag.RowsAffected() == 0 {
		return dataset.Item{}, dataset.NewNotFoundError()
	}
	return item, err
}

func (r *Repository) DatasetItems(ctx context.Context, tenantID, datasetID string) ([]dataset.Item, error) {
	if _, ok, err := r.GetDataset(ctx, tenantID, datasetID); err != nil {
		return nil, err
	} else if !ok {
		return nil, dataset.NewNotFoundError()
	}
	rows, err := r.datasetDB().Query(ctx, `
			SELECT di.id, di.dataset_id, di.query, di.ground_truth, di.relevant_doc_ids
			FROM dataset_items di
			JOIN datasets d ON d.id=di.dataset_id
			WHERE d.tenant_id=$1 AND di.dataset_id=$2
			ORDER BY di.id`, tenantID, datasetID)
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

func (r *Repository) datasetDB() datasetQueryer {
	if r.datasetRunner != nil {
		return r.datasetRunner
	}
	return pgxDatasetQueryer{pool: r.Pool}
}
