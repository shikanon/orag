package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/shikanon/orag/pkg/memory"
)

func main() {
	if err := run(context.Background(), os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, out io.Writer) error {
	client := memory.New()

	doc, err := client.AddDocument(ctx, memory.Document{
		Title:     "ORAG Memory Mode",
		SourceURI: "memory://orag-demo",
		Content: `ORAG memory mode stores documents and chunks in process.

It is useful for examples because it does not require PostgreSQL, Qdrant, or Ark credentials.

Responses include trace metadata such as trace id, node count, slowest node, latency, and citations.`,
		Metadata: map[string]string{"example": "go-memory"},
	})
	if err != nil {
		return err
	}

	resp, err := client.Query(ctx, memory.QueryRequest{
		Query:   "How does ORAG memory mode expose trace metadata?",
		TopK:    2,
		TraceID: "trace_example_memory",
	})
	if err != nil {
		return err
	}
	trace, ok := client.Trace(ctx, resp.TraceID)
	if !ok {
		return fmt.Errorf("trace %q was not recorded", resp.TraceID)
	}

	fmt.Fprintf(out, "document_id=%s chunks=%d\n", doc.ID, len(doc.Chunks))
	fmt.Fprintf(out, "answer=%s\n", resp.Answer)
	fmt.Fprintf(out, "trace_id=%s cache_status=%s latency_ms=%d\n", resp.TraceID, resp.CacheStatus, resp.LatencyMS)
	fmt.Fprintf(out, "trace_summary=node_count:%d slowest_node:%s\n", resp.TraceSummary.NodeCount, resp.TraceSummary.SlowestNode)
	fmt.Fprintf(out, "trace_spans=%d citations=%d\n", len(trace.NodeSpans), len(resp.Citations))
	return nil
}
