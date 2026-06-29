package postgres

import (
	"context"

	raggraph "github.com/shikanon/orag/internal/graph"
	"github.com/shikanon/orag/internal/platform/id"
	"github.com/shikanon/orag/internal/rag"
)

func (r *Repository) StoreTrace(ctx context.Context, tenantID, traceID, query string, profile rag.Profile, latencyMS int64, spans []raggraph.NodeSpan) error {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		INSERT INTO rag_traces(id, tenant_id, query, profile, latency_ms)
		VALUES($1,$2,$3,$4,$5)
		ON CONFLICT (id) DO NOTHING`,
		traceID, tenantID, query, string(profile), latencyMS); err != nil {
		return err
	}
	for _, span := range spans {
		if _, err := tx.Exec(ctx, `
			INSERT INTO rag_node_spans(id, trace_id, node_name, latency_ms, error)
			VALUES($1,$2,$3,$4,$5)`,
			id.New("span"), traceID, span.NodeName, span.LatencyMS, span.Error); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
