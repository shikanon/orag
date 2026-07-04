package postgres

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shikanon/orag/internal/kb"
)

type Repository struct {
	Pool            *pgxpool.Pool
	StageChunks     bool
	kbQueryer       knowledgeBaseQueryer
	kbTxBeginner    knowledgeBaseTxBeginner
	traceReader     traceQueryer
	traceTxBeginner traceTxBeginner
	datasetRunner   datasetQueryer
	evalQueryer     evalQueryer
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{Pool: pool, kbQueryer: pool, traceReader: pgxTraceQueryer{pool: pool}, datasetRunner: pgxDatasetQueryer{pool: pool}, evalQueryer: pgxEvalQueryer{pool: pool}}
}

type knowledgeBaseQueryer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type knowledgeBaseTx interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

type knowledgeBaseTxBeginner interface {
	BeginKnowledgeBaseTx(ctx context.Context) (knowledgeBaseTx, error)
}

type pgxKnowledgeBaseTxBeginner struct {
	pool *pgxpool.Pool
}

func (b pgxKnowledgeBaseTxBeginner) BeginKnowledgeBaseTx(ctx context.Context) (knowledgeBaseTx, error) {
	return b.pool.Begin(ctx)
}

type pgxTraceQueryer struct {
	pool *pgxpool.Pool
}

func (q pgxTraceQueryer) QueryRow(ctx context.Context, sql string, args ...any) traceRow {
	return q.pool.QueryRow(ctx, sql, args...)
}

func (q pgxTraceQueryer) Query(ctx context.Context, sql string, args ...any) (traceRows, error) {
	return q.pool.Query(ctx, sql, args...)
}

type traceTx interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

type traceTxBeginner interface {
	BeginTraceTx(ctx context.Context) (traceTx, error)
}

type pgxTraceTxBeginner struct {
	pool *pgxpool.Pool
}

func (b pgxTraceTxBeginner) BeginTraceTx(ctx context.Context) (traceTx, error) {
	return b.pool.Begin(ctx)
}

type datasetQueryer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type pgxDatasetQueryer struct {
	pool *pgxpool.Pool
}

func (q pgxDatasetQueryer) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return q.pool.Exec(ctx, sql, args...)
}

func (q pgxDatasetQueryer) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return q.pool.Query(ctx, sql, args...)
}

func (q pgxDatasetQueryer) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return q.pool.QueryRow(ctx, sql, args...)
}

type evalQueryer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type pgxEvalQueryer struct {
	pool *pgxpool.Pool
}

func (q pgxEvalQueryer) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return q.pool.Exec(ctx, sql, args...)
}

func (q pgxEvalQueryer) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return q.pool.Query(ctx, sql, args...)
}

func (q pgxEvalQueryer) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return q.pool.QueryRow(ctx, sql, args...)
}

func (r *Repository) evaluationQueryer() evalQueryer {
	if r.evalQueryer != nil {
		return r.evalQueryer
	}
	return r.Pool
}

func (r *Repository) PutKnowledgeBase(ctx context.Context, item kb.KnowledgeBase) error {
	meta := mustJSON(item.Metadata)
	_, err := r.knowledgeBaseQueryer().Exec(ctx, `
		INSERT INTO knowledge_bases(id, tenant_id, name, description, metadata, created_at, updated_at)
		VALUES($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (id) DO UPDATE SET
			name=EXCLUDED.name,
			description=EXCLUDED.description,
			metadata=EXCLUDED.metadata,
			updated_at=EXCLUDED.updated_at`,
		item.ID, item.TenantID, item.Name, item.Description, meta, item.CreatedAt, item.UpdatedAt)
	return err
}

func (r *Repository) ListKnowledgeBases(ctx context.Context, tenantID string) ([]kb.KnowledgeBase, error) {
	rows, err := r.knowledgeBaseQueryer().Query(ctx, `
		SELECT id, tenant_id, name, description, metadata, created_at, updated_at
		FROM knowledge_bases
		WHERE tenant_id=$1
		ORDER BY created_at`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []kb.KnowledgeBase
	for rows.Next() {
		item, err := scanKnowledgeBase(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Repository) GetKnowledgeBase(ctx context.Context, tenantID, id string) (kb.KnowledgeBase, bool, error) {
	row := r.knowledgeBaseQueryer().QueryRow(ctx, `
		SELECT id, tenant_id, name, description, metadata, created_at, updated_at
		FROM knowledge_bases
		WHERE tenant_id=$1 AND id=$2`, tenantID, id)
	item, err := scanKnowledgeBase(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return kb.KnowledgeBase{}, false, nil
		}
		return kb.KnowledgeBase{}, false, err
	}
	return item, true, nil
}

func (r *Repository) DeleteKnowledgeBase(ctx context.Context, tenantID, id string) (bool, error) {
	tx, err := r.knowledgeBaseTxBeginner().BeginKnowledgeBaseTx(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	var lockedID string
	if err = tx.QueryRow(ctx, `
		SELECT id
		FROM knowledge_bases
		WHERE tenant_id=$1 AND id=$2
		FOR UPDATE`, tenantID, id).Scan(&lockedID); err != nil {
		if err == pgx.ErrNoRows {
			return false, nil
		}
		return false, err
	}

	if _, err = tx.Exec(ctx, `
		DELETE FROM harness_runs
		WHERE tenant_id=$1
		  AND candidate_id IN (
			SELECT c.id
			FROM optimization_candidates c
			JOIN optimization_runs r ON r.id = c.optimization_run_id
			WHERE r.tenant_id=$1 AND r.knowledge_base_id=$2
		  )`, tenantID, id); err != nil {
		return false, err
	}
	if _, err = tx.Exec(ctx, `
		DELETE FROM optimization_candidates c
		USING optimization_runs r
		WHERE c.optimization_run_id = r.id
		  AND r.tenant_id=$1
		  AND r.knowledge_base_id=$2`, tenantID, id); err != nil {
		return false, err
	}
	if _, err = tx.Exec(ctx, `
		DELETE FROM optimization_runs
		WHERE tenant_id=$1 AND knowledge_base_id=$2`, tenantID, id); err != nil {
		return false, err
	}
	if _, err = tx.Exec(ctx, `
		DELETE FROM chunks
		WHERE tenant_id=$1 AND knowledge_base_id=$2`, tenantID, id); err != nil {
		return false, err
	}
	if _, err = tx.Exec(ctx, `
		DELETE FROM documents
		WHERE tenant_id=$1 AND knowledge_base_id=$2`, tenantID, id); err != nil {
		return false, err
	}
	if _, err = tx.Exec(ctx, `
		DELETE FROM ingestion_jobs
		WHERE tenant_id=$1 AND knowledge_base_id=$2`, tenantID, id); err != nil {
		return false, err
	}
	tag, err := tx.Exec(ctx, `
		DELETE FROM knowledge_bases
		WHERE tenant_id=$1 AND id=$2`, tenantID, id)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	return true, tx.Commit(ctx)
}

func (r *Repository) Store(ctx context.Context, doc kb.Document, chunks []kb.Chunk) error {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var existingID string
	err = tx.QueryRow(ctx, `
		SELECT id FROM documents
		WHERE tenant_id=$1 AND knowledge_base_id=$2 AND content_hash=$3`,
		doc.TenantID, doc.KnowledgeBaseID, doc.ContentHash).Scan(&existingID)
	if err != nil && err != pgx.ErrNoRows {
		return err
	}
	if existingID != "" {
		doc.ID = existingID
	}

	if err := deleteDocumentSource(ctx, tx, doc.TenantID, doc.KnowledgeBaseID, doc.SourceURI, doc.ContentHash); err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO documents(id, tenant_id, knowledge_base_id, source_uri, title, content_hash, metadata, created_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (tenant_id, knowledge_base_id, content_hash) DO UPDATE SET
			source_uri=EXCLUDED.source_uri,
			title=EXCLUDED.title,
			metadata=EXCLUDED.metadata`,
		doc.ID, doc.TenantID, doc.KnowledgeBaseID, doc.SourceURI, doc.Title, doc.ContentHash, mustJSON(doc.Metadata), doc.CreatedAt)
	if err != nil {
		return err
	}

	for _, chunk := range chunks {
		if existingID != "" {
			chunk.DocumentID = existingID
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO chunks(id, tenant_id, knowledge_base_id, document_id, content, contextual_text, source_uri, page, section, offset_start, metadata, searchable)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
			ON CONFLICT (id) DO UPDATE SET
				content=EXCLUDED.content,
				contextual_text=EXCLUDED.contextual_text,
				source_uri=EXCLUDED.source_uri,
				page=EXCLUDED.page,
				section=EXCLUDED.section,
				offset_start=EXCLUDED.offset_start,
				metadata=EXCLUDED.metadata`,
			chunk.ID, chunk.TenantID, chunk.KnowledgeBaseID, chunk.DocumentID, chunk.Content, chunk.ContextualText, chunk.SourceURI, chunk.Page, chunk.Section, chunk.Offset, mustJSON(chunk.Metadata), !r.StageChunks)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *Repository) DeleteDocumentSource(ctx context.Context, tenantID, kbID, sourceURI string) error {
	_, err := r.Pool.Exec(ctx, `
		DELETE FROM chunks
		WHERE tenant_id=$1 AND knowledge_base_id=$2 AND source_uri=$3`, tenantID, kbID, sourceURI)
	if err != nil {
		return err
	}
	_, err = r.Pool.Exec(ctx, `
		DELETE FROM documents
		WHERE tenant_id=$1 AND knowledge_base_id=$2 AND source_uri=$3`, tenantID, kbID, sourceURI)
	return err
}

func deleteDocumentSource(ctx context.Context, tx pgx.Tx, tenantID, kbID, sourceURI, keepContentHash string) error {
	if sourceURI == "" {
		return nil
	}
	_, err := tx.Exec(ctx, `
		DELETE FROM chunks
		WHERE tenant_id=$1 AND knowledge_base_id=$2 AND document_id IN (
			SELECT id FROM documents
			WHERE tenant_id=$1 AND knowledge_base_id=$2 AND source_uri=$3 AND content_hash<>$4
		)`, tenantID, kbID, sourceURI, keepContentHash)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		DELETE FROM documents
		WHERE tenant_id=$1 AND knowledge_base_id=$2 AND source_uri=$3 AND content_hash<>$4`, tenantID, kbID, sourceURI, keepContentHash)
	return err
}

func (r *Repository) Activate(ctx context.Context, doc kb.Document, chunks []kb.Chunk) error {
	if len(chunks) == 0 {
		return nil
	}
	_, err := r.Pool.Exec(ctx, `
		UPDATE chunks
		SET searchable=TRUE
		WHERE tenant_id=$1 AND knowledge_base_id=$2 AND document_id=$3`,
		doc.TenantID, doc.KnowledgeBaseID, doc.ID)
	return err
}

func (r *Repository) Chunks(tenantID, kbID string) []kb.Chunk {
	rows, err := r.Pool.Query(context.Background(), `
		SELECT id, tenant_id, knowledge_base_id, document_id, content, contextual_text, source_uri, page, section, offset_start, metadata
		FROM chunks
		WHERE tenant_id=$1 AND knowledge_base_id=$2 AND searchable
		ORDER BY id`, tenantID, kbID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []kb.Chunk
	for rows.Next() {
		chunk, err := scanChunk(rows)
		if err == nil {
			out = append(out, chunk)
		}
	}
	return out
}

func (r *Repository) StoreGraphRelations(ctx context.Context, relations []kb.GraphRelation) error {
	if len(relations) == 0 {
		return nil
	}
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	docIDs := map[string]bool{}
	for _, relation := range relations {
		if relation.DocumentID != "" {
			docIDs[relation.DocumentID] = true
		}
	}
	for docID := range docIDs {
		if _, err := tx.Exec(ctx, `DELETE FROM graph_relations WHERE document_id=$1`, docID); err != nil {
			return err
		}
	}
	for _, relation := range relations {
		if relation.Weight == 0 {
			relation.Weight = 1
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO graph_relations(
				tenant_id, knowledge_base_id, document_id, source_chunk_id, target_chunk_id,
				subject, predicate, object, weight
			)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
			relation.TenantID, relation.KnowledgeBaseID, relation.DocumentID, relation.SourceChunkID, relation.TargetChunkID,
			relation.Subject, relation.Predicate, relation.Object, relation.Weight)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *Repository) ExpandGraph(ctx context.Context, req kb.GraphExpansionRequest) ([]kb.SearchResult, error) {
	if len(req.Entities) == 0 {
		return nil, nil
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 8
	}
	entities := make([]string, 0, len(req.Entities))
	for _, entity := range req.Entities {
		if value := strings.ToLower(strings.TrimSpace(entity)); value != "" {
			entities = append(entities, value)
		}
	}
	if len(entities) == 0 {
		return nil, nil
	}
	rows, err := r.Pool.Query(ctx, `
		WITH matched AS (
			SELECT source_chunk_id AS chunk_id, weight
			FROM graph_relations
			WHERE tenant_id=$1 AND knowledge_base_id=$2
			  AND (lower(subject)=ANY($3::text[]) OR lower(object)=ANY($3::text[]))
			UNION ALL
			SELECT target_chunk_id AS chunk_id, weight
			FROM graph_relations
			WHERE tenant_id=$1 AND knowledge_base_id=$2
			  AND (lower(subject)=ANY($3::text[]) OR lower(object)=ANY($3::text[]))
		)
		SELECT c.id, c.tenant_id, c.knowledge_base_id, c.document_id, c.content, c.contextual_text,
		       c.source_uri, c.page, c.section, c.offset_start, c.metadata, max(m.weight) AS score
		FROM matched m
		JOIN chunks c ON c.id=m.chunk_id
		WHERE c.tenant_id=$1 AND c.knowledge_base_id=$2 AND c.searchable
		GROUP BY c.id
		ORDER BY score DESC, c.id
		LIMIT $4`, req.TenantID, req.KnowledgeBaseID, entities, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []kb.SearchResult
	for rows.Next() {
		chunk, score, err := scanScoredChunk(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, kb.SearchResult{Chunk: chunk, Score: score, Rank: len(out) + 1, From: "graph"})
	}
	return out, rows.Err()
}

func (r *Repository) BootstrapDefaults(ctx context.Context, tenantID, kbID string) error {
	now := time.Now().UTC()
	if _, err := r.knowledgeBaseQueryer().Exec(ctx, `
		INSERT INTO tenants(id, name, created_at)
		VALUES($1, $2, $3)
		ON CONFLICT (id) DO NOTHING`, tenantID, tenantID, now); err != nil {
		return err
	}
	return r.PutKnowledgeBase(ctx, kb.KnowledgeBase{
		ID:          kbID,
		TenantID:    tenantID,
		Name:        "Default Knowledge Base",
		Description: "默认知识库",
		Metadata:    map[string]string{"created_by": "bootstrap"},
		CreatedAt:   now,
		UpdatedAt:   now,
	})
}

func (r *Repository) knowledgeBaseQueryer() knowledgeBaseQueryer {
	if r.kbQueryer != nil {
		return r.kbQueryer
	}
	return r.Pool
}

func (r *Repository) knowledgeBaseTxBeginner() knowledgeBaseTxBeginner {
	if r.kbTxBeginner != nil {
		return r.kbTxBeginner
	}
	return pgxKnowledgeBaseTxBeginner{pool: r.Pool}
}

type kbScanner interface {
	Scan(dest ...any) error
}

func scanKnowledgeBase(row kbScanner) (kb.KnowledgeBase, error) {
	var item kb.KnowledgeBase
	var meta []byte
	err := row.Scan(&item.ID, &item.TenantID, &item.Name, &item.Description, &meta, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return item, err
	}
	item.Metadata = stringMap(meta)
	return item, nil
}

func scanChunk(row kbScanner) (kb.Chunk, error) {
	var chunk kb.Chunk
	var meta []byte
	err := row.Scan(&chunk.ID, &chunk.TenantID, &chunk.KnowledgeBaseID, &chunk.DocumentID, &chunk.Content, &chunk.ContextualText, &chunk.SourceURI, &chunk.Page, &chunk.Section, &chunk.Offset, &meta)
	if err != nil {
		return chunk, err
	}
	chunk.Metadata = stringMap(meta)
	return chunk, nil
}

func mustJSON(v any) []byte {
	if v == nil {
		return []byte("{}")
	}
	body, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return body
}

func stringMap(body []byte) map[string]string {
	if len(body) == 0 {
		return nil
	}
	var out map[string]string
	if err := json.Unmarshal(body, &out); err != nil {
		return nil
	}
	return out
}
