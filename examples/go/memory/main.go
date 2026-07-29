package main

import (
	"context"
	"fmt"
	"io"
	"os"

	orag "github.com/shikanon/orag"
)

func main() {
	if err := run(context.Background(), os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, out io.Writer) error {
	client, err := orag.New(ctx, orag.MockConfig())
	if err != nil {
		return err
	}
	defer client.Close()

	result, err := client.IngestText(ctx, orag.IngestTextRequest{
		KnowledgeBaseID: "kb_default",
		Name:            "ORAG Memory Mode",
		SourceURI:       "memory://orag-demo",
		Text: `ORAG memory mode stores documents and chunks in process.

It is useful for examples because it does not require PostgreSQL, Qdrant, or Ark credentials.

Responses include trace metadata such as trace id, node count, slowest node, latency, and citations.`,
	})
	if err != nil {
		return err
	}

	resp, err := client.Query(ctx, orag.QueryRequest{
		KnowledgeBaseID: "kb_default",
		Query:           "How does ORAG memory mode expose trace metadata?",
		TopK:            2,
		TraceID:         "trace_example_memory",
	})
	if err != nil {
		return err
	}
	trace, ok, err := client.GetTrace(ctx, orag.GetTraceRequest{ID: resp.TraceID})
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("trace %q was not recorded", resp.TraceID)
	}

	fmt.Fprintf(out, "document_id=%s chunks=%d\n", result.Document.ID, len(result.Chunks))
	fmt.Fprintf(out, "answer=%s\n", resp.Answer)
	fmt.Fprintf(out, "trace_id=%s cache_status=%s latency_ms=%d\n", resp.TraceID, resp.CacheStatus, resp.LatencyMS)
	nodeCount := len(trace.NodeSpans)
	slowestNode := ""
	var slowestLatency int64
	for _, span := range trace.NodeSpans {
		if span.LatencyMS >= slowestLatency {
			slowestNode = span.NodeName
			slowestLatency = span.LatencyMS
		}
	}
	fmt.Fprintf(out, "trace_summary=node_count:%d slowest_node:%s\n", nodeCount, slowestNode)
	fmt.Fprintf(out, "trace_spans=%d citations=%d\n", len(trace.NodeSpans), len(resp.Citations))
	return nil
}
