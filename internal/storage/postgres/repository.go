package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shikanon/orag/internal/kb"
)

type Repository struct {
	Pool        *pgxpool.Pool
	traceReader traceQueryer
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{Pool: pool, traceReader: pgxTraceQueryer{pool: pool}}
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

func (r *Repository) PutKnowledgeBase(item kb.KnowledgeBase) {
	ctx := context.Background()
	meta := mustJSON(item.Metadata)
	_, _ = r.Pool.Exec(ctx, `
		INSERT INTO knowledge_bases(id, tenant_id, name, description, metadata, created_at, updated_at)
		VALUES($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (id) DO UPDATE SET
			name=EXCLUDED.name,
			description=EXCLUDED.description,
			metadata=EXCLUDED.metadata,
			updated_at=EXCLUDED.updated_at`,
		item.ID, item.TenantID, item.Name, item.Description, meta, item.CreatedAt, item.UpdatedAt)
}

func (r *Repository) ListKnowledgeBases(tenantID string) []kb.KnowledgeBase {
	rows, err := r.Pool.Query(context.Background(), `
		SELECT id, tenant_id, name, description, metadata, created_at, updated_at
		FROM knowledge_bases
		WHERE tenant_id=$1
		ORDER BY created_at`, tenantID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []kb.KnowledgeBase
	for rows.Next() {
		item, err := scanKnowledgeBase(rows)
		if err == nil {
			out = append(out, item)
		}
	}
	return out
}

func (r *Repository) GetKnowledgeBase(tenantID, id string) (kb.KnowledgeBase, bool) {
	row := r.Pool.QueryRow(context.Background(), `
		SELECT id, tenant_id, name, description, metadata, created_at, updated_at
		FROM knowledge_bases
		WHERE tenant_id=$1 AND id=$2`, tenantID, id)
	item, err := scanKnowledgeBase(row)
	return item, err == nil
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
		chunks = chunksWithDocumentID(chunks, existingID)
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
		_, err = tx.Exec(ctx, `
			INSERT INTO chunks(id, tenant_id, knowledge_base_id, document_id, content, source_uri, page, section, offset_start, metadata)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
			ON CONFLICT (id) DO UPDATE SET
				content=EXCLUDED.content,
				source_uri=EXCLUDED.source_uri,
				page=EXCLUDED.page,
				section=EXCLUDED.section,
				offset_start=EXCLUDED.offset_start,
				metadata=EXCLUDED.metadata`,
			chunk.ID, chunk.TenantID, chunk.KnowledgeBaseID, chunk.DocumentID, chunk.Content, chunk.SourceURI, chunk.Page, chunk.Section, chunk.Offset, mustJSON(chunk.Metadata))
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func chunksWithDocumentID(chunks []kb.Chunk, documentID string) []kb.Chunk {
	out := make([]kb.Chunk, len(chunks))
	copy(out, chunks)
	for i := range out {
		if out[i].DocumentID == documentID {
			continue
		}
		out[i].DocumentID = documentID
		out[i].ID = chunkID(documentID, i)
	}
	return out
}

func chunkID(docID string, index int) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s/%d", docID, index)))
	return "chk_" + hex.EncodeToString(sum[:])[:24]
}

func (r *Repository) Chunks(tenantID, kbID string) []kb.Chunk {
	rows, err := r.Pool.Query(context.Background(), `
		SELECT id, tenant_id, knowledge_base_id, document_id, content, source_uri, page, section, offset_start, metadata
		FROM chunks
		WHERE tenant_id=$1 AND knowledge_base_id=$2
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

func (r *Repository) BootstrapDefaults(ctx context.Context, tenantID, kbID string) error {
	now := time.Now().UTC()
	if _, err := r.Pool.Exec(ctx, `
		INSERT INTO tenants(id, name, created_at)
		VALUES($1, $2, $3)
		ON CONFLICT (id) DO NOTHING`, tenantID, tenantID, now); err != nil {
		return err
	}
	r.PutKnowledgeBase(kb.KnowledgeBase{
		ID:          kbID,
		TenantID:    tenantID,
		Name:        "Default Knowledge Base",
		Description: "默认知识库",
		Metadata:    map[string]string{"created_by": "bootstrap"},
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	return nil
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
	err := row.Scan(&chunk.ID, &chunk.TenantID, &chunk.KnowledgeBaseID, &chunk.DocumentID, &chunk.Content, &chunk.SourceURI, &chunk.Page, &chunk.Section, &chunk.Offset, &meta)
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
