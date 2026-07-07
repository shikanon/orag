package demo

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/shikanon/orag/pkg/memory"
)

// Dimension describes one usage dimension for a role-based ORAG scenario.
type Dimension struct {
	Name  string
	Value string
}

// Scenario is a runnable role-based ORAG demo.
type Scenario struct {
	ID               string
	Title            string
	Role             string
	BusinessGoal     string
	UserQuestion     string
	DemoDataPaths    []string
	SourceURI        string
	Profile          string
	TopK             int
	Dimensions       []Dimension
	ExpectedSignals  []string
	RecommendedSteps []string
}

// Run executes the scenario through the public in-memory ORAG facade.
func Run(ctx context.Context, out io.Writer, scenario Scenario) error {
	content, usedPath, err := readFirstFile(scenario.DemoDataPaths)
	if err != nil {
		return err
	}
	if strings.TrimSpace(scenario.ID) == "" {
		return fmt.Errorf("scenario id is required")
	}
	if strings.TrimSpace(scenario.UserQuestion) == "" {
		return fmt.Errorf("scenario user question is required")
	}

	client := memory.New(
		memory.WithTenantID("tenant_"+safeID(scenario.ID)),
		memory.WithKnowledgeBaseID("kb_"+safeID(scenario.ID)),
	)
	doc, err := client.AddDocument(ctx, memory.Document{
		Title:     scenario.Title + " Demo Data",
		SourceURI: scenario.SourceURI,
		Content:   content,
		Metadata: map[string]string{
			"scenario": scenario.ID,
			"role":     scenario.Role,
		},
	})
	if err != nil {
		return err
	}

	resp, err := client.Query(ctx, memory.QueryRequest{
		Query:   scenario.UserQuestion,
		TopK:    scenario.TopK,
		TraceID: "trace_" + safeID(scenario.ID),
		Profile: scenario.Profile,
	})
	if err != nil {
		return err
	}
	trace, ok := client.Trace(ctx, resp.TraceID)
	if !ok {
		return fmt.Errorf("trace %q was not recorded", resp.TraceID)
	}

	printScenario(out, scenario, usedPath, doc, resp, trace)
	return nil
}

func readFirstFile(paths []string) (string, string, error) {
	for _, path := range paths {
		body, err := os.ReadFile(path)
		if err == nil {
			return string(body), path, nil
		}
	}
	return "", "", fmt.Errorf("none of the demo data paths exist: %s", strings.Join(paths, ", "))
}

func printScenario(out io.Writer, scenario Scenario, dataPath string, doc memory.DocumentRecord, resp memory.QueryResponse, trace memory.TraceRecord) {
	fmt.Fprintf(out, "scenario=%s\n", scenario.ID)
	fmt.Fprintf(out, "title=%s\n", scenario.Title)
	fmt.Fprintf(out, "role=%s\n", scenario.Role)
	fmt.Fprintf(out, "business_goal=%s\n", scenario.BusinessGoal)
	fmt.Fprintf(out, "demo_data=%s\n", dataPath)
	fmt.Fprintf(out, "document_id=%s chunks=%d source=%s\n", doc.ID, len(doc.Chunks), doc.SourceURI)
	fmt.Fprintf(out, "question=%s\n", scenario.UserQuestion)
	fmt.Fprintf(out, "answer=%s\n", resp.Answer)
	fmt.Fprintf(out, "trace_id=%s profile=%s cache_status=%s latency_ms=%d\n", resp.TraceID, resp.Profile, resp.CacheStatus, resp.LatencyMS)
	fmt.Fprintf(out, "trace_summary=node_count:%d slowest_node:%s spans:%d errors:%d\n", resp.TraceSummary.NodeCount, resp.TraceSummary.SlowestNode, len(trace.NodeSpans), trace.ErrorCount)
	fmt.Fprintf(out, "citations=%d retrieved_chunks=%d\n", len(resp.Citations), len(resp.RetrievedChunks))
	if len(resp.Citations) > 0 {
		first := resp.Citations[0]
		fmt.Fprintf(out, "first_citation=%s %s %s\n", first.DocumentID, first.Section, first.SourceURI)
	}

	fmt.Fprintln(out, "usage_dimensions:")
	for _, dimension := range scenario.Dimensions {
		fmt.Fprintf(out, "- %s: %s\n", dimension.Name, dimension.Value)
	}

	fmt.Fprintln(out, "expected_signals:")
	for _, signal := range scenario.ExpectedSignals {
		fmt.Fprintf(out, "- %s\n", signal)
	}

	fmt.Fprintln(out, "recommended_next_steps:")
	for _, step := range scenario.RecommendedSteps {
		fmt.Fprintf(out, "- %s\n", step)
	}
}

func safeID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "-", "_")
	value = strings.ReplaceAll(value, " ", "_")
	return value
}
