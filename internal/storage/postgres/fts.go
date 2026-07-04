package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shikanon/orag/internal/kb"
)

type ftsQueryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

type FTSRetriever struct {
	Pool    *pgxpool.Pool
	queryer ftsQueryer
}

func NewFTSRetriever(repo *Repository) FTSRetriever {
	return FTSRetriever{Pool: repo.Pool}
}

func (r FTSRetriever) Retrieve(ctx context.Context, req kb.SearchRequest) ([]kb.SearchResult, error) {
	limit := req.TopK
	if req.SparseTopK > 0 {
		limit = req.SparseTopK
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.ftsQueryer().Query(ctx, `
		WITH q AS (SELECT plainto_tsquery('simple', $3) AS query)
		SELECT id, tenant_id, knowledge_base_id, document_id, content, contextual_text, source_uri, page, section, offset_start, metadata,
		       ts_rank_cd(search_text_tsvector, (SELECT query FROM q)) AS score
		FROM chunks
		WHERE tenant_id=$1
		  AND knowledge_base_id=$2
		  AND searchable
		  AND search_text_tsvector @@ (SELECT query FROM q)
		ORDER BY score DESC, id
		LIMIT $4`, req.TenantID, req.KnowledgeBaseID, req.Query, limit)
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
		out = append(out, kb.SearchResult{Chunk: chunk, Score: score, Rank: len(out) + 1, From: "postgres_fts"})
	}
	return out, nil
}

func (r FTSRetriever) ftsQueryer() ftsQueryer {
	if r.queryer != nil {
		return r.queryer
	}
	return r.Pool
}

func scanScoredChunk(row kbScanner) (kb.Chunk, float64, error) {
	var chunk kb.Chunk
	var meta []byte
	var score float64
	err := row.Scan(&chunk.ID, &chunk.TenantID, &chunk.KnowledgeBaseID, &chunk.DocumentID, &chunk.Content, &chunk.ContextualText, &chunk.SourceURI, &chunk.Page, &chunk.Section, &chunk.Offset, &meta, &score)
	if err != nil {
		return chunk, 0, err
	}
	chunk.Metadata = stringMap(meta)
	return chunk, score, nil
}
