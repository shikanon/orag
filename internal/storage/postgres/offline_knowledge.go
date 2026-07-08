package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shikanon/orag/internal/offlineknowledge"
)

var ErrOfflineKnowledgeRunConflict = offlineknowledge.ErrRunConflict

func (r *Repository) AddNegativeFeedback(ctx context.Context, item offlineknowledge.NegativeFeedback) error {
	item.CreatedAt = defaultTime(item.CreatedAt)
	if item.ID == "" {
		item.ID = stableNegativeFeedbackID(item)
	}
	_, err := r.evaluationQueryer().Exec(ctx, `
		INSERT INTO offline_negative_feedback(id, tenant_id, kb_id, trace_id, query, reason, created_at)
		VALUES($1,$2,$3,$4,$5,$6,$7)`,
		item.ID, item.TenantID, item.KBID, item.TraceID, item.Query, item.Reason, item.CreatedAt)
	return err
}

func (r *Repository) ListNegativeFeedback(ctx context.Context, filter offlineknowledge.NegativeFeedbackFilter) ([]offlineknowledge.NegativeFeedback, error) {
	sqlText := `
		SELECT id, tenant_id, kb_id, trace_id, query, reason, created_at
		FROM offline_negative_feedback
		WHERE tenant_id=$1`
	args := []any{filter.TenantID}
	if filter.KBID != "" {
		args = append(args, filter.KBID)
		sqlText += fmt.Sprintf(" AND kb_id=$%d", len(args))
	}
	if !filter.Since.IsZero() {
		args = append(args, filter.Since)
		sqlText += fmt.Sprintf(" AND created_at >= $%d", len(args))
	}
	if !filter.Until.IsZero() {
		args = append(args, filter.Until)
		sqlText += fmt.Sprintf(" AND created_at < $%d", len(args))
	}
	if len(filter.TraceIDs) > 0 {
		args = append(args, filter.TraceIDs)
		sqlText += fmt.Sprintf(" AND (trace_id = ANY($%d) OR trace_id = '')", len(args))
	}
	sqlText += " ORDER BY created_at DESC, id"
	if filter.Limit > 0 {
		args = append(args, filter.Limit)
		sqlText += fmt.Sprintf(" LIMIT $%d", len(args))
	}
	rows, err := r.evaluationQueryer().Query(ctx, sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []offlineknowledge.NegativeFeedback
	for rows.Next() {
		item, err := scanNegativeFeedback(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *Repository) CreateRun(ctx context.Context, run offlineknowledge.OfflineKnowledgeRun) error {
	run.StartedAt = defaultTime(run.StartedAt)
	configJSON, err := json.Marshal(run.ConfigJSON)
	if err != nil {
		return err
	}
	_, err = r.evaluationQueryer().Exec(ctx, `
		INSERT INTO offline_knowledge_runs(
			id, tenant_id, kb_id, status, window_start, window_end, config_hash,
			config_json, error, started_at, finished_at
		)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		run.ID, run.TenantID, run.KBID, run.Status, run.WindowStart, run.WindowEnd,
		run.ConfigHash, configJSON, run.Error, run.StartedAt, nullableTime(run.FinishedAt))
	if isUniqueViolation(err) {
		return fmt.Errorf("%w: %w", offlineknowledge.ErrRunConflict, err)
	}
	return err
}

func (r *Repository) GetRun(ctx context.Context, tenantID, runID string) (offlineknowledge.OfflineKnowledgeRun, bool, error) {
	row := r.evaluationQueryer().QueryRow(ctx, `
		SELECT id, tenant_id, kb_id, status, window_start, window_end, config_hash, config_json,
			error, started_at, finished_at
		FROM offline_knowledge_runs
		WHERE tenant_id=$1 AND id=$2`, tenantID, runID)
	run, err := scanOfflineKnowledgeRun(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return offlineknowledge.OfflineKnowledgeRun{}, false, nil
		}
		return offlineknowledge.OfflineKnowledgeRun{}, false, err
	}
	return run, true, nil
}

func (r *Repository) ListRuns(ctx context.Context, filter offlineknowledge.RunFilter) ([]offlineknowledge.OfflineKnowledgeRun, error) {
	sqlText := `
		SELECT id, tenant_id, kb_id, status, window_start, window_end, config_hash, config_json,
			error, started_at, finished_at
		FROM offline_knowledge_runs
		WHERE tenant_id=$1`
	args := []any{filter.TenantID}
	if filter.KBID != "" {
		args = append(args, filter.KBID)
		sqlText += fmt.Sprintf(" AND kb_id=$%d", len(args))
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		sqlText += fmt.Sprintf(" AND status=$%d", len(args))
	}
	sqlText += " ORDER BY started_at DESC, id"
	if filter.Limit > 0 {
		args = append(args, filter.Limit)
		sqlText += fmt.Sprintf(" LIMIT $%d", len(args))
	}
	rows, err := r.evaluationQueryer().Query(ctx, sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []offlineknowledge.OfflineKnowledgeRun
	for rows.Next() {
		run, err := scanOfflineKnowledgeRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	return out, rows.Err()
}

func (r *Repository) UpdateRun(ctx context.Context, run offlineknowledge.OfflineKnowledgeRun) (bool, error) {
	configJSON, err := json.Marshal(run.ConfigJSON)
	if err != nil {
		return false, err
	}
	tag, err := r.evaluationQueryer().Exec(ctx, `
		UPDATE offline_knowledge_runs
		SET kb_id=$3, status=$4, window_start=$5, window_end=$6, config_hash=$7,
			config_json=$8, error=$9, started_at=$10, finished_at=$11
		WHERE tenant_id=$1 AND id=$2`,
		run.TenantID, run.ID, run.KBID, run.Status, run.WindowStart, run.WindowEnd,
		run.ConfigHash, configJSON, run.Error, defaultTime(run.StartedAt), nullableTime(run.FinishedAt))
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (r *Repository) UpsertQuestionCluster(ctx context.Context, cluster offlineknowledge.QuestionCluster) error {
	cluster.CreatedAt = defaultTime(cluster.CreatedAt)
	embedding, err := json.Marshal(cluster.EmbeddingJSON)
	if err != nil {
		return err
	}
	samples, err := json.Marshal(cluster.SampleQuestions)
	if err != nil {
		return err
	}
	traces, err := json.Marshal(cluster.TraceIDs)
	if err != nil {
		return err
	}
	_, err = r.evaluationQueryer().Exec(ctx, `
		INSERT INTO offline_question_clusters(
			id, tenant_id, run_id, kb_id, canonical_question, normalized_question,
			question_hash, embedding_ref, embedding_json, occurrence_count,
			sample_questions_json, trace_ids_json, long_tail, created_at
		)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		ON CONFLICT (tenant_id, kb_id, question_hash) DO UPDATE SET
			run_id=EXCLUDED.run_id,
			canonical_question=EXCLUDED.canonical_question,
			normalized_question=EXCLUDED.normalized_question,
			embedding_ref=EXCLUDED.embedding_ref,
			embedding_json=EXCLUDED.embedding_json,
			occurrence_count=EXCLUDED.occurrence_count,
			sample_questions_json=EXCLUDED.sample_questions_json,
			trace_ids_json=EXCLUDED.trace_ids_json,
			long_tail=EXCLUDED.long_tail`,
		cluster.ID, cluster.TenantID, cluster.RunID, cluster.KBID, cluster.CanonicalQuestion,
		cluster.NormalizedQuestion, cluster.QuestionHash, cluster.EmbeddingRef, embedding,
		cluster.OccurrenceCount, samples, traces, cluster.LongTail, cluster.CreatedAt)
	return err
}

func (r *Repository) GetQuestionCluster(ctx context.Context, tenantID, clusterID string) (offlineknowledge.QuestionCluster, bool, error) {
	row := r.evaluationQueryer().QueryRow(ctx, questionClusterSelectSQL()+`
		WHERE tenant_id=$1 AND id=$2`, tenantID, clusterID)
	cluster, err := scanQuestionCluster(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return offlineknowledge.QuestionCluster{}, false, nil
		}
		return offlineknowledge.QuestionCluster{}, false, err
	}
	return cluster, true, nil
}

func (r *Repository) ListQuestionClusters(ctx context.Context, filter offlineknowledge.QuestionClusterFilter) ([]offlineknowledge.QuestionCluster, error) {
	sqlText := questionClusterSelectSQL() + " WHERE tenant_id=$1"
	args := []any{filter.TenantID}
	if filter.KBID != "" {
		args = append(args, filter.KBID)
		sqlText += fmt.Sprintf(" AND kb_id=$%d", len(args))
	}
	if filter.RunID != "" {
		args = append(args, filter.RunID)
		sqlText += fmt.Sprintf(" AND run_id=$%d", len(args))
	}
	if filter.QuestionHash != "" {
		args = append(args, filter.QuestionHash)
		sqlText += fmt.Sprintf(" AND question_hash=$%d", len(args))
	}
	sqlText += " ORDER BY created_at DESC, id"
	if filter.Limit > 0 {
		args = append(args, filter.Limit)
		sqlText += fmt.Sprintf(" LIMIT $%d", len(args))
	}
	rows, err := r.evaluationQueryer().Query(ctx, sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []offlineknowledge.QuestionCluster
	for rows.Next() {
		cluster, err := scanQuestionCluster(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, cluster)
	}
	return out, rows.Err()
}

func (r *Repository) CreateOptimizationItem(ctx context.Context, item offlineknowledge.OptimizationItem) error {
	item.CreatedAt = defaultTime(item.CreatedAt)
	item.UpdatedAt = defaultTime(item.UpdatedAt)
	sourceFingerprints, err := json.Marshal(item.SourceFingerprints)
	if err != nil {
		return err
	}
	evidence, err := json.Marshal(item.Evidence)
	if err != nil {
		return err
	}
	deepSearchSteps, err := json.Marshal(item.DeepSearchSteps)
	if err != nil {
		return err
	}
	evalReport := evalReportJSON(item.EvalReportJSON)
	sourceChunkIDs, sourceDocIDs := sourceIDs(item.SourceFingerprints)
	chunksJSON, err := json.Marshal(sourceChunkIDs)
	if err != nil {
		return err
	}
	docsJSON, err := json.Marshal(sourceDocIDs)
	if err != nil {
		return err
	}
	_, err = r.evaluationQueryer().Exec(ctx, `
		INSERT INTO optimization_items(
			id, tenant_id, run_id, kb_id, question_cluster_id, item_type, status,
			canonical_question, final_answer, recall_quality, failure_type, confidence,
			source_chunk_ids_json, source_doc_ids_json, source_fingerprints_json,
			evidence_json, deep_search_steps_json, analyzer_report_json,
			validation_report_json, eval_report_json, created_at, updated_at, published_at
		)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23)`,
		item.ID, item.TenantID, item.RunID, item.KBID, item.QuestionClusterID, item.ItemType,
		item.Status, item.CanonicalQuestion, item.FinalAnswer, item.RecallQuality, item.FailureType,
		item.Confidence, chunksJSON, docsJSON, sourceFingerprints, evidence, deepSearchSteps,
		[]byte(`{}`), []byte(`{}`), evalReport, item.CreatedAt, item.UpdatedAt, nullableTime(item.PublishedAt))
	return err
}

func (r *Repository) GetOptimizationItem(ctx context.Context, tenantID, itemID string) (offlineknowledge.OptimizationItem, bool, error) {
	row := r.evaluationQueryer().QueryRow(ctx, optimizationItemSelectSQL()+`
		WHERE tenant_id=$1 AND id=$2`, tenantID, itemID)
	item, err := scanOptimizationItem(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return offlineknowledge.OptimizationItem{}, false, nil
		}
		return offlineknowledge.OptimizationItem{}, false, err
	}
	return item, true, nil
}

func (r *Repository) ListOptimizationItems(ctx context.Context, filter offlineknowledge.OptimizationItemFilter) ([]offlineknowledge.OptimizationItem, error) {
	sqlText := optimizationItemSelectSQL() + " WHERE tenant_id=$1"
	args := []any{filter.TenantID}
	if filter.KBID != "" {
		args = append(args, filter.KBID)
		sqlText += fmt.Sprintf(" AND kb_id=$%d", len(args))
	}
	if filter.RunID != "" {
		args = append(args, filter.RunID)
		sqlText += fmt.Sprintf(" AND run_id=$%d", len(args))
	}
	if filter.QuestionClusterID != "" {
		args = append(args, filter.QuestionClusterID)
		sqlText += fmt.Sprintf(" AND question_cluster_id=$%d", len(args))
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		sqlText += fmt.Sprintf(" AND status=$%d", len(args))
	}
	if filter.ItemType != "" {
		args = append(args, filter.ItemType)
		sqlText += fmt.Sprintf(" AND item_type=$%d", len(args))
	}
	sqlText += " ORDER BY updated_at DESC, id"
	if filter.Limit > 0 {
		args = append(args, filter.Limit)
		sqlText += fmt.Sprintf(" LIMIT $%d", len(args))
	}
	rows, err := r.evaluationQueryer().Query(ctx, sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []offlineknowledge.OptimizationItem
	for rows.Next() {
		item, err := scanOptimizationItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *Repository) UpdateOptimizationItem(ctx context.Context, item offlineknowledge.OptimizationItem) (bool, error) {
	sourceFingerprints, evidence, deepSearchSteps, evalReport, chunksJSON, docsJSON, err := encodeOfflineKnowledgeItemJSON(item)
	if err != nil {
		return false, err
	}
	tag, err := r.evaluationQueryer().Exec(ctx, `
		UPDATE optimization_items
		SET run_id=$3, kb_id=$4, question_cluster_id=$5, item_type=$6, status=$7,
			canonical_question=$8, final_answer=$9, recall_quality=$10, failure_type=$11,
			confidence=$12, source_chunk_ids_json=$13, source_doc_ids_json=$14,
			source_fingerprints_json=$15, evidence_json=$16, deep_search_steps_json=$17,
			eval_report_json=$18, updated_at=$19, published_at=$20
		WHERE tenant_id=$1 AND id=$2`,
		item.TenantID, item.ID, item.RunID, item.KBID, item.QuestionClusterID, item.ItemType,
		item.Status, item.CanonicalQuestion, item.FinalAnswer, item.RecallQuality, item.FailureType,
		item.Confidence, chunksJSON, docsJSON, sourceFingerprints, evidence, deepSearchSteps,
		evalReport, defaultTime(item.UpdatedAt), nullableTime(item.PublishedAt))
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (r *Repository) UpdateOptimizationItemStatus(ctx context.Context, tenantID, itemID string, status offlineknowledge.ItemStatus, updatedAt time.Time) (bool, error) {
	tag, err := r.evaluationQueryer().Exec(ctx, `
		UPDATE optimization_items
		SET status=$3, updated_at=$4
		WHERE tenant_id=$1 AND id=$2`, tenantID, itemID, status, defaultTime(updatedAt))
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (r *Repository) AppendItemEvent(ctx context.Context, event offlineknowledge.OptimizationItemEvent) error {
	event.CreatedAt = defaultTime(event.CreatedAt)
	payload, err := json.Marshal(event.Payload)
	if err != nil {
		return err
	}
	_, err = r.evaluationQueryer().Exec(ctx, `
		INSERT INTO optimization_item_events(id, tenant_id, item_id, event_type, operator, payload_json, created_at)
		VALUES($1,$2,$3,$4,$5,$6,$7)`,
		event.ID, event.TenantID, event.ItemID, event.EventType, event.Operator, payload, event.CreatedAt)
	return err
}

func (r *Repository) ListItemEvents(ctx context.Context, filter offlineknowledge.OptimizationItemEventFilter) ([]offlineknowledge.OptimizationItemEvent, error) {
	sqlText := `
		SELECT id, tenant_id, item_id, event_type, operator, payload_json, created_at
		FROM optimization_item_events
		WHERE tenant_id=$1`
	args := []any{filter.TenantID}
	if filter.ItemID != "" {
		args = append(args, filter.ItemID)
		sqlText += fmt.Sprintf(" AND item_id=$%d", len(args))
	}
	if filter.EventType != "" {
		args = append(args, filter.EventType)
		sqlText += fmt.Sprintf(" AND event_type=$%d", len(args))
	}
	if !filter.Since.IsZero() {
		args = append(args, filter.Since)
		sqlText += fmt.Sprintf(" AND created_at >= $%d", len(args))
	}
	if !filter.Until.IsZero() {
		args = append(args, filter.Until)
		sqlText += fmt.Sprintf(" AND created_at < $%d", len(args))
	}
	sqlText += " ORDER BY created_at DESC, id"
	if filter.Limit > 0 {
		args = append(args, filter.Limit)
		sqlText += fmt.Sprintf(" LIMIT $%d", len(args))
	}
	rows, err := r.evaluationQueryer().Query(ctx, sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []offlineknowledge.OptimizationItemEvent
	for rows.Next() {
		event, err := scanOptimizationItemEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, event)
	}
	return out, rows.Err()
}

func (r *Repository) RecordShadowEvent(ctx context.Context, event offlineknowledge.ShadowRetrievalEvent) error {
	event.CreatedAt = defaultTime(event.CreatedAt)
	_, err := r.evaluationQueryer().Exec(ctx, `
		INSERT INTO shadow_retrieval_events(
			id, tenant_id, kb_id, optimization_item_id, trace_id, query, matched,
			injected, rank, score, recall_lift, answer_lift, hallucination_risk, created_at
		)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		event.ID, event.TenantID, event.KBID, event.ItemID, event.TraceID, event.Query,
		event.Matched, event.Injected, event.Rank, event.Score, event.RecallLift,
		event.AnswerLift, event.HallucinationRisk, event.CreatedAt)
	return err
}

func (r *Repository) ListShadowEvents(ctx context.Context, filter offlineknowledge.ShadowRetrievalEventFilter) ([]offlineknowledge.ShadowRetrievalEvent, error) {
	sqlText := `
		SELECT id, tenant_id, kb_id, optimization_item_id, trace_id, query, matched,
			injected, rank, score, recall_lift, answer_lift, hallucination_risk, created_at
		FROM shadow_retrieval_events
		WHERE tenant_id=$1`
	args := []any{filter.TenantID}
	if filter.KBID != "" {
		args = append(args, filter.KBID)
		sqlText += fmt.Sprintf(" AND kb_id=$%d", len(args))
	}
	if filter.TraceID != "" {
		args = append(args, filter.TraceID)
		sqlText += fmt.Sprintf(" AND trace_id=$%d", len(args))
	}
	if filter.ItemID != "" {
		args = append(args, filter.ItemID)
		sqlText += fmt.Sprintf(" AND optimization_item_id=$%d", len(args))
	}
	if !filter.Since.IsZero() {
		args = append(args, filter.Since)
		sqlText += fmt.Sprintf(" AND created_at >= $%d", len(args))
	}
	if !filter.Until.IsZero() {
		args = append(args, filter.Until)
		sqlText += fmt.Sprintf(" AND created_at < $%d", len(args))
	}
	sqlText += " ORDER BY created_at DESC, id"
	if filter.Limit > 0 {
		args = append(args, filter.Limit)
		sqlText += fmt.Sprintf(" LIMIT $%d", len(args))
	}
	rows, err := r.evaluationQueryer().Query(ctx, sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []offlineknowledge.ShadowRetrievalEvent
	for rows.Next() {
		event, err := scanShadowRetrievalEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, event)
	}
	return out, rows.Err()
}

func (r *Repository) RecordCodexToolAudit(ctx context.Context, event offlineknowledge.CodexToolAuditEvent) error {
	event.StartedAt = defaultTime(event.StartedAt)
	event.FinishedAt = defaultTime(event.FinishedAt)
	_, err := r.evaluationQueryer().Exec(ctx, `
		INSERT INTO offline_codex_tool_audit(
			id, tenant_id, kb_id, session_id, tool, rows, steps, allowed, error, started_at, finished_at
		)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		event.ID, event.TenantID, event.KBID, event.SessionID, event.Tool, event.Rows,
		event.Steps, event.Allowed, event.Error, event.StartedAt, event.FinishedAt)
	return err
}

func (r *Repository) ListCodexToolAuditEvents(ctx context.Context, filter offlineknowledge.CodexToolAuditFilter) ([]offlineknowledge.CodexToolAuditEvent, error) {
	sqlText := `
		SELECT id, tenant_id, kb_id, session_id, tool, rows, steps, allowed, error, started_at, finished_at
		FROM offline_codex_tool_audit
		WHERE tenant_id=$1`
	args := []any{filter.TenantID}
	if filter.KBID != "" {
		args = append(args, filter.KBID)
		sqlText += fmt.Sprintf(" AND kb_id=$%d", len(args))
	}
	if filter.SessionID != "" {
		args = append(args, filter.SessionID)
		sqlText += fmt.Sprintf(" AND session_id=$%d", len(args))
	}
	if filter.Tool != "" {
		args = append(args, filter.Tool)
		sqlText += fmt.Sprintf(" AND tool=$%d", len(args))
	}
	if !filter.Since.IsZero() {
		args = append(args, filter.Since)
		sqlText += fmt.Sprintf(" AND started_at >= $%d", len(args))
	}
	if !filter.Until.IsZero() {
		args = append(args, filter.Until)
		sqlText += fmt.Sprintf(" AND started_at < $%d", len(args))
	}
	sqlText += " ORDER BY started_at DESC, id"
	if filter.Limit > 0 {
		args = append(args, filter.Limit)
		sqlText += fmt.Sprintf(" LIMIT $%d", len(args))
	}
	rows, err := r.evaluationQueryer().Query(ctx, sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []offlineknowledge.CodexToolAuditEvent
	for rows.Next() {
		event, err := scanCodexToolAuditEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, event)
	}
	return out, rows.Err()
}

func questionClusterSelectSQL() string {
	return `
		SELECT id, tenant_id, run_id, kb_id, canonical_question, normalized_question,
			question_hash, embedding_ref, embedding_json, occurrence_count,
			sample_questions_json, trace_ids_json, long_tail, created_at
		FROM offline_question_clusters`
}

func optimizationItemSelectSQL() string {
	return `
		SELECT id, tenant_id, run_id, kb_id, question_cluster_id, item_type, status,
			canonical_question, final_answer, recall_quality, failure_type, confidence,
			source_fingerprints_json, evidence_json, deep_search_steps_json,
			eval_report_json, created_at, updated_at, published_at
		FROM optimization_items`
}

type offlineKnowledgeScanner interface {
	Scan(dest ...any) error
}

func scanNegativeFeedback(row offlineKnowledgeScanner) (offlineknowledge.NegativeFeedback, error) {
	var item offlineknowledge.NegativeFeedback
	err := row.Scan(&item.ID, &item.TenantID, &item.KBID, &item.TraceID, &item.Query, &item.Reason, &item.CreatedAt)
	if err != nil {
		return offlineknowledge.NegativeFeedback{}, err
	}
	return item, nil
}

func scanOfflineKnowledgeRun(row offlineKnowledgeScanner) (offlineknowledge.OfflineKnowledgeRun, error) {
	var run offlineknowledge.OfflineKnowledgeRun
	var configJSON []byte
	var finishedAt sql.NullTime
	err := row.Scan(&run.ID, &run.TenantID, &run.KBID, &run.Status, &run.WindowStart, &run.WindowEnd,
		&run.ConfigHash, &configJSON, &run.Error, &run.StartedAt, &finishedAt)
	if err != nil {
		return offlineknowledge.OfflineKnowledgeRun{}, err
	}
	if len(configJSON) > 0 {
		if err := json.Unmarshal(configJSON, &run.ConfigJSON); err != nil {
			return offlineknowledge.OfflineKnowledgeRun{}, err
		}
	}
	if finishedAt.Valid {
		run.FinishedAt = finishedAt.Time
	}
	return run, nil
}

func scanQuestionCluster(row offlineKnowledgeScanner) (offlineknowledge.QuestionCluster, error) {
	var cluster offlineknowledge.QuestionCluster
	var embedding, samples, traces []byte
	err := row.Scan(&cluster.ID, &cluster.TenantID, &cluster.RunID, &cluster.KBID,
		&cluster.CanonicalQuestion, &cluster.NormalizedQuestion, &cluster.QuestionHash,
		&cluster.EmbeddingRef, &embedding, &cluster.OccurrenceCount, &samples,
		&traces, &cluster.LongTail, &cluster.CreatedAt)
	if err != nil {
		return offlineknowledge.QuestionCluster{}, err
	}
	if err := json.Unmarshal(embedding, &cluster.EmbeddingJSON); err != nil {
		return offlineknowledge.QuestionCluster{}, err
	}
	if err := json.Unmarshal(samples, &cluster.SampleQuestions); err != nil {
		return offlineknowledge.QuestionCluster{}, err
	}
	if err := json.Unmarshal(traces, &cluster.TraceIDs); err != nil {
		return offlineknowledge.QuestionCluster{}, err
	}
	return cluster, nil
}

func scanOptimizationItem(row offlineKnowledgeScanner) (offlineknowledge.OptimizationItem, error) {
	var item offlineknowledge.OptimizationItem
	var sourceFingerprints, evidence, deepSearchSteps, evalReport []byte
	var publishedAt sql.NullTime
	err := row.Scan(&item.ID, &item.TenantID, &item.RunID, &item.KBID, &item.QuestionClusterID,
		&item.ItemType, &item.Status, &item.CanonicalQuestion, &item.FinalAnswer,
		&item.RecallQuality, &item.FailureType, &item.Confidence, &sourceFingerprints,
		&evidence, &deepSearchSteps, &evalReport, &item.CreatedAt, &item.UpdatedAt, &publishedAt)
	if err != nil {
		return offlineknowledge.OptimizationItem{}, err
	}
	if err := json.Unmarshal(sourceFingerprints, &item.SourceFingerprints); err != nil {
		return offlineknowledge.OptimizationItem{}, err
	}
	if err := json.Unmarshal(evidence, &item.Evidence); err != nil {
		return offlineknowledge.OptimizationItem{}, err
	}
	if err := json.Unmarshal(deepSearchSteps, &item.DeepSearchSteps); err != nil {
		return offlineknowledge.OptimizationItem{}, err
	}
	item.EvalReportJSON = append([]byte(nil), evalReport...)
	if publishedAt.Valid {
		item.PublishedAt = publishedAt.Time
	}
	return item, nil
}

func scanOptimizationItemEvent(row offlineKnowledgeScanner) (offlineknowledge.OptimizationItemEvent, error) {
	var event offlineknowledge.OptimizationItemEvent
	var payload []byte
	err := row.Scan(&event.ID, &event.TenantID, &event.ItemID, &event.EventType,
		&event.Operator, &payload, &event.CreatedAt)
	if err != nil {
		return offlineknowledge.OptimizationItemEvent{}, err
	}
	if err := json.Unmarshal(payload, &event.Payload); err != nil {
		return offlineknowledge.OptimizationItemEvent{}, err
	}
	return event, nil
}

func scanShadowRetrievalEvent(row offlineKnowledgeScanner) (offlineknowledge.ShadowRetrievalEvent, error) {
	var event offlineknowledge.ShadowRetrievalEvent
	err := row.Scan(&event.ID, &event.TenantID, &event.KBID, &event.ItemID, &event.TraceID,
		&event.Query, &event.Matched, &event.Injected, &event.Rank, &event.Score,
		&event.RecallLift, &event.AnswerLift, &event.HallucinationRisk, &event.CreatedAt)
	if err != nil {
		return offlineknowledge.ShadowRetrievalEvent{}, err
	}
	return event, nil
}

func scanCodexToolAuditEvent(row offlineKnowledgeScanner) (offlineknowledge.CodexToolAuditEvent, error) {
	var event offlineknowledge.CodexToolAuditEvent
	err := row.Scan(&event.ID, &event.TenantID, &event.KBID, &event.SessionID, &event.Tool,
		&event.Rows, &event.Steps, &event.Allowed, &event.Error, &event.StartedAt, &event.FinishedAt)
	if err != nil {
		return offlineknowledge.CodexToolAuditEvent{}, err
	}
	return event, nil
}

func encodeOfflineKnowledgeItemJSON(item offlineknowledge.OptimizationItem) (sourceFingerprints, evidence, deepSearchSteps, evalReport, chunksJSON, docsJSON []byte, err error) {
	if sourceFingerprints, err = json.Marshal(item.SourceFingerprints); err != nil {
		return
	}
	if evidence, err = json.Marshal(item.Evidence); err != nil {
		return
	}
	if deepSearchSteps, err = json.Marshal(item.DeepSearchSteps); err != nil {
		return
	}
	evalReport = evalReportJSON(item.EvalReportJSON)
	sourceChunkIDs, sourceDocIDs := sourceIDs(item.SourceFingerprints)
	if chunksJSON, err = json.Marshal(sourceChunkIDs); err != nil {
		return
	}
	docsJSON, err = json.Marshal(sourceDocIDs)
	return
}

func evalReportJSON(value []byte) []byte {
	if len(value) == 0 {
		return []byte(`{}`)
	}
	return append([]byte(nil), value...)
}

func sourceIDs(fingerprints []offlineknowledge.SourceFingerprint) ([]string, []string) {
	chunkSeen := make(map[string]struct{}, len(fingerprints))
	docSeen := make(map[string]struct{}, len(fingerprints))
	chunkIDs := make([]string, 0, len(fingerprints))
	docIDs := make([]string, 0, len(fingerprints))
	for _, fingerprint := range fingerprints {
		if fingerprint.ChunkID != "" {
			if _, ok := chunkSeen[fingerprint.ChunkID]; !ok {
				chunkSeen[fingerprint.ChunkID] = struct{}{}
				chunkIDs = append(chunkIDs, fingerprint.ChunkID)
			}
		}
		if fingerprint.DocID != "" {
			if _, ok := docSeen[fingerprint.DocID]; !ok {
				docSeen[fingerprint.DocID] = struct{}{}
				docIDs = append(docIDs, fingerprint.DocID)
			}
		}
	}
	return chunkIDs, docIDs
}

func defaultTime(value time.Time) time.Time {
	if value.IsZero() {
		return time.Now().UTC()
	}
	return value
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func stableNegativeFeedbackID(item offlineknowledge.NegativeFeedback) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%s\x00%s",
		item.TenantID,
		item.KBID,
		item.TraceID,
		item.Query,
		item.Reason,
		item.CreatedAt.Format(time.RFC3339Nano),
	)))
	return "negfb_" + hex.EncodeToString(sum[:])[:16]
}
