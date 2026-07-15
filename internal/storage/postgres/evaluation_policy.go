package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/shikanon/orag/internal/evaluationpolicy"
)

var _ evaluationpolicy.Repository = (*Repository)(nil)

func (r *Repository) Create(ctx context.Context, policy evaluationpolicy.Policy) error {
	gates, err := json.Marshal(policy.Gates)
	if err != nil {
		return err
	}
	_, err = r.evaluationQueryer().Exec(ctx, `
		INSERT INTO project_evaluation_policies(id, tenant_id, project_id, dataset_id, name, version, gates, created_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8)`,
		policy.ID, policy.TenantID, policy.ProjectID, policy.DatasetID, policy.Name, policy.Version, gates, policy.CreatedAt)
	return evaluationPolicyPersistenceError(err)
}

func (r *Repository) Get(ctx context.Context, tenantID, projectID, policyID string) (evaluationpolicy.Policy, error) {
	item, err := scanEvaluationPolicy(r.evaluationQueryer().QueryRow(ctx, `
		SELECT id, tenant_id, project_id, dataset_id, name, version, gates, created_at
		FROM project_evaluation_policies
		WHERE tenant_id=$1 AND project_id=$2 AND id=$3`, tenantID, projectID, policyID))
	if errors.Is(err, pgx.ErrNoRows) {
		return evaluationpolicy.Policy{}, evaluationpolicy.ErrPolicyNotFound
	}
	return item, err
}

func (r *Repository) List(ctx context.Context, tenantID, projectID string) ([]evaluationpolicy.Policy, error) {
	rows, err := r.evaluationQueryer().Query(ctx, `
		SELECT id, tenant_id, project_id, dataset_id, name, version, gates, created_at
		FROM project_evaluation_policies
		WHERE tenant_id=$1 AND project_id=$2
		ORDER BY created_at DESC, id DESC`, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]evaluationpolicy.Policy, 0)
	for rows.Next() {
		item, err := scanEvaluationPolicy(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) RecordEvidence(ctx context.Context, evidence evaluationpolicy.Evidence) error {
	frozen, err := json.Marshal(evidence.FrozenInput)
	if err != nil {
		return err
	}
	results, err := json.Marshal(evidence.GateResults)
	if err != nil {
		return err
	}
	_, err = r.evaluationQueryer().Exec(ctx, `
		INSERT INTO project_evaluation_evidence(
			id, tenant_id, project_id, policy_id, policy_version, evaluation_run_id,
			pipeline_version_id, content_hash, environment_kind, frozen_input, gate_results, passed, created_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,NULLIF($9,''),$10,$11,$12,$13)`,
		evidence.ID, evidence.TenantID, evidence.ProjectID, evidence.PolicyID, evidence.PolicyVersion,
		evidence.EvaluationRunID, evidence.PipelineVersionID, evidence.ContentHash, evidence.Environment, frozen, results,
		evidence.Passed, evidence.CreatedAt)
	return evaluationPolicyPersistenceError(err)
}

func scanEvaluationPolicy(row interface{ Scan(...any) error }) (evaluationpolicy.Policy, error) {
	var item evaluationpolicy.Policy
	var gates []byte
	if err := row.Scan(&item.ID, &item.TenantID, &item.ProjectID, &item.DatasetID, &item.Name, &item.Version, &gates, &item.CreatedAt); err != nil {
		return evaluationpolicy.Policy{}, err
	}
	if err := json.Unmarshal(gates, &item.Gates); err != nil {
		return evaluationpolicy.Policy{}, err
	}
	return item, nil
}

func evaluationPolicyPersistenceError(err error) error {
	if err == nil {
		return nil
	}
	if isUniqueViolation(err) {
		return errors.Join(evaluationpolicy.ErrInvalidPolicy, err)
	}
	return err
}
