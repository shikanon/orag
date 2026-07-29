package orag

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/shikanon/orag/internal/benchmark"
)

const mockPerformanceBaselineWorkloadID = "text-rag/mock-baseline-v1"

// PerformanceBaselineOptions controls a deterministic local mock baseline run.
// It never selects a real provider or external storage backend.
type PerformanceBaselineOptions struct {
	BuildRevision    string
	WarmupRequests   int
	MeasuredRequests int
	Concurrency      int
}

// DefaultPerformanceBaselineOptions returns the fixed load used by the local
// reproducible baseline. Set BuildRevision to the commit being measured.
func DefaultPerformanceBaselineOptions() PerformanceBaselineOptions {
	return PerformanceBaselineOptions{
		BuildRevision:    "development",
		WarmupRequests:   10,
		MeasuredRequests: 20,
		Concurrency:      1,
	}
}

type performanceBaselineDocument struct {
	Name    string `json:"name"`
	Source  string `json:"source"`
	Content string `json:"content"`
}

type performanceBaselineQuery struct {
	Query       string `json:"query"`
	GroundTruth string `json:"ground_truth"`
	Document    int    `json:"document"`
}

type performanceBaselineWorkload struct {
	SchemaVersion string                        `json:"schema_version"`
	ID            string                        `json:"id"`
	Documents     []performanceBaselineDocument `json:"documents"`
	Queries       []performanceBaselineQuery    `json:"queries"`
}

func mockPerformanceBaselineWorkload() performanceBaselineWorkload {
	return performanceBaselineWorkload{
		SchemaVersion: "orag.mock-baseline-workload.v1",
		ID:            mockPerformanceBaselineWorkloadID,
		Documents: []performanceBaselineDocument{
			{Name: "evaluation.txt", Source: "benchmark://evaluation", Content: "ORAG evaluates retrieval and generation with frozen datasets, repeatable metrics, and explicit quality gates."},
			{Name: "observability.txt", Source: "benchmark://observability", Content: "ORAG records traces for ingestion, retrieval, reranking, generation, and evaluation so operators can inspect evidence."},
			{Name: "deployment.txt", Source: "benchmark://deployment", Content: "ORAG offers deterministic mock walkthroughs for local validation and keeps production provider credentials explicit."},
		},
		Queries: []performanceBaselineQuery{
			{Query: "How does ORAG measure quality?", GroundTruth: "ORAG evaluates retrieval and generation with frozen datasets and repeatable metrics.", Document: 0},
			{Query: "What does an ORAG trace record?", GroundTruth: "ORAG traces ingestion, retrieval, reranking, generation, and evaluation.", Document: 1},
			{Query: "How can a user validate ORAG without credentials?", GroundTruth: "ORAG provides deterministic mock walkthroughs for local validation.", Document: 2},
		},
	}
}

// RunMockPerformanceBaseline executes the checked-in workload through the
// public embedded SDK and returns one validated performance-baseline JSON
// value. Timings are local observations, not portable performance claims.
func RunMockPerformanceBaseline(ctx context.Context, opts PerformanceBaselineOptions) ([]byte, error) {
	if err := validatePerformanceBaselineOptions(opts); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	workload := mockPerformanceBaselineWorkload()
	workloadRaw, err := json.Marshal(workload)
	if err != nil {
		return nil, fmt.Errorf("marshal baseline workload: %w", err)
	}
	client, err := New(ctx, MockConfig())
	if err != nil {
		return nil, fmt.Errorf("create mock benchmark client: %w", err)
	}
	defer client.Close()

	knowledgeBase, err := client.CreateKnowledgeBase(ctx, CreateKnowledgeBaseRequest{Name: "Performance baseline"})
	if err != nil {
		return nil, fmt.Errorf("create benchmark knowledge base: %w", err)
	}
	knowledgeBaseID := knowledgeBase.ID
	ingestStarted := time.Now()
	documentIDs := make([]string, len(workload.Documents))
	for index, document := range workload.Documents {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		result, err := client.IngestText(ctx, IngestTextRequest{KnowledgeBaseID: knowledgeBaseID, Name: document.Name, SourceURI: document.Source, Text: document.Content})
		if err != nil {
			return nil, fmt.Errorf("ingest benchmark document %q: %w", document.Name, err)
		}
		documentIDs[index] = result.Document.ID
	}
	ingestDuration := nonZeroMilliseconds(time.Since(ingestStarted))

	dataset, err := client.CreateDataset(ctx, CreateDatasetRequest{Name: "Performance baseline", Kind: "retrieval"})
	if err != nil {
		return nil, fmt.Errorf("create benchmark dataset: %w", err)
	}
	for _, query := range workload.Queries {
		if _, err := client.AddDatasetItem(ctx, AddDatasetItemRequest{DatasetID: dataset.ID, Query: query.Query, GroundTruth: query.GroundTruth, RelevantDocIDs: []string{documentIDs[query.Document]}, Split: DatasetSplitEval, Weight: 1}); err != nil {
			return nil, fmt.Errorf("add benchmark dataset item: %w", err)
		}
	}

	if _, _, err := runPerformanceBaselineQueries(ctx, client, knowledgeBaseID, workload.Queries, opts.WarmupRequests, opts.Concurrency, "warmup"); err != nil {
		return nil, err
	}
	durations, cacheHits, err := runPerformanceBaselineQueries(ctx, client, knowledgeBaseID, workload.Queries, opts.MeasuredRequests, opts.Concurrency, "measure")
	if err != nil {
		return nil, err
	}
	evaluationStarted := time.Now()
	if _, err := client.RunEvaluation(ctx, RunEvaluationRequest{DatasetID: dataset.ID, KnowledgeBaseID: knowledgeBaseID, Profile: "realtime", TopK: 8, Split: DatasetSplitEval}); err != nil {
		return nil, fmt.Errorf("run benchmark evaluation: %w", err)
	}
	evaluationDuration := nonZeroMilliseconds(time.Since(evaluationStarted))

	runtimeRaw, err := json.Marshal(struct {
		RunnerSchema string `json:"runner_schema"`
		GoVersion    string `json:"go_version"`
		GOOS         string `json:"goos"`
		GOARCH       string `json:"goarch"`
		ChatModel    string `json:"chat_model"`
		EmbedModel   string `json:"embedding_model"`
		RerankModel  string `json:"rerank_model"`
		DenseTopK    int    `json:"dense_top_k"`
		RerankTopN   int    `json:"rerank_top_n"`
		ContextTopN  int    `json:"context_top_n"`
	}{"orag.mock-baseline-runner.v1", runtime.Version(), runtime.GOOS, runtime.GOARCH, "orag-deterministic-mock", "orag-deterministic-embedding", "orag-deterministic-rerank", 8, 4, 4})
	if err != nil {
		return nil, fmt.Errorf("marshal benchmark runtime environment: %w", err)
	}
	report := benchmark.Report{
		SchemaVersion: benchmark.SchemaVersion,
		ID:            mockPerformanceBaselineWorkloadID,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		Provenance: benchmark.Provenance{
			WorkloadID:               workload.ID,
			PackTier:                 "benchmark",
			DeterministicMock:        true,
			DatasetFingerprint:       sha256Hex(workloadRaw),
			RuntimeEnvironmentSHA256: sha256Hex(runtimeRaw),
			BuildRevision:            opts.BuildRevision,
		},
		Load: benchmark.Load{WarmupRequests: opts.WarmupRequests, MeasuredRequests: opts.MeasuredRequests, Concurrency: opts.Concurrency},
		Metrics: benchmark.Metrics{
			IngestionDocuments:         len(workload.Documents),
			IngestionDurationMS:        ingestDuration,
			IngestionThroughputDocsSec: float64(len(workload.Documents)) * 1000 / float64(ingestDuration),
			QueryP50MS:                 percentileMilliseconds(durations, 0.50),
			QueryP95MS:                 percentileMilliseconds(durations, 0.95),
			CacheHitRate:               float64(cacheHits) / float64(len(durations)),
			EvaluationDurationMS:       evaluationDuration,
			ModelCalls:                 opts.WarmupRequests + opts.MeasuredRequests + len(workload.Queries),
			CostUSD:                    0,
		},
	}
	if err := benchmark.Validate(report); err != nil {
		return nil, fmt.Errorf("validate observed benchmark report: %w", err)
	}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal benchmark report: %w", err)
	}
	return append(raw, '\n'), nil
}

func validatePerformanceBaselineOptions(opts PerformanceBaselineOptions) error {
	if strings.TrimSpace(opts.BuildRevision) == "" {
		return fmt.Errorf("build revision is required")
	}
	if opts.WarmupRequests < 0 || opts.MeasuredRequests < 20 || opts.Concurrency < 1 {
		return fmt.Errorf("baseline load requires nonnegative warmup, at least 20 measured requests, and positive concurrency")
	}
	return nil
}

func runPerformanceBaselineQueries(ctx context.Context, client *Client, knowledgeBaseID string, queries []performanceBaselineQuery, count, concurrency int, phase string) ([]int64, int, error) {
	if count == 0 {
		return nil, 0, nil
	}
	type observation struct {
		duration int64
		cacheHit bool
		err      error
	}
	jobs := make(chan int)
	results := make(chan observation, count)
	var workers sync.WaitGroup
	for worker := 0; worker < concurrency; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range jobs {
				query := queries[index%len(queries)]
				started := time.Now()
				response, err := client.Query(ctx, QueryRequest{KnowledgeBaseID: knowledgeBaseID, Query: query.Query, Profile: "realtime", TopK: 8, TraceID: fmt.Sprintf("baseline_%s_%03d", phase, index)})
				results <- observation{duration: nonZeroMilliseconds(time.Since(started)), cacheHit: response.CacheStatus == "hit", err: err}
			}
		}()
	}
	for index := 0; index < count; index++ {
		select {
		case <-ctx.Done():
			close(jobs)
			workers.Wait()
			return nil, 0, ctx.Err()
		case jobs <- index:
		}
	}
	close(jobs)
	workers.Wait()
	durations := make([]int64, 0, count)
	cacheHits := 0
	for index := 0; index < count; index++ {
		result := <-results
		if result.err != nil {
			return nil, 0, fmt.Errorf("%s benchmark query: %w", phase, result.err)
		}
		durations = append(durations, result.duration)
		if result.cacheHit {
			cacheHits++
		}
	}
	return durations, cacheHits, nil
}

func percentileMilliseconds(values []int64, percentile float64) int64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]int64(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	index := int(math.Ceil(percentile*float64(len(sorted)))) - 1
	if index < 0 {
		index = 0
	}
	return sorted[index]
}

func nonZeroMilliseconds(duration time.Duration) int64 {
	if milliseconds := duration.Milliseconds(); milliseconds > 0 {
		return milliseconds
	}
	return 1
}

func sha256Hex(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
