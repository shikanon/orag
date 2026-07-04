package eval

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/shikanon/orag/internal/dataset"
	"github.com/shikanon/orag/internal/platform/id"
	"github.com/shikanon/orag/internal/rag"
)

type Runner struct {
	RAG        *rag.Service
	Datasets   *dataset.Service
	Repository Repository
}

type Repository interface {
	StoreEvaluationRun(ctx context.Context, tenantID string, result RunResult) error
	StoreEvaluationResult(ctx context.Context, runID, datasetItemID, answer string, metrics map[string]float64) error
	GetEvaluationRun(ctx context.Context, tenantID, id string) (RunResult, bool, error)
}

type RunRequest struct {
	TenantID        string      `json:"-"`
	DatasetID       string      `json:"dataset_id"`
	KnowledgeBaseID string      `json:"knowledge_base_id"`
	Profile         rag.Profile `json:"profile"`
	TopK            int         `json:"top_k,omitempty"`
}

type RunResult struct {
	ID        string             `json:"id"`
	DatasetID string             `json:"dataset_id"`
	Profile   string             `json:"profile"`
	Total     int                `json:"total"`
	HitRate   float64            `json:"hit_rate"`
	Accuracy  float64            `json:"accuracy"`
	Metrics   map[string]float64 `json:"metrics,omitempty"`
	CreatedAt time.Time          `json:"created_at"`
}

const PrimaryMetricPairwiseAccuracy = "pairwise_accuracy"

func (r Runner) Run(ctx context.Context, req RunRequest) (RunResult, error) {
	items, err := r.Datasets.Items(ctx, req.DatasetID)
	if err != nil {
		return RunResult{}, err
	}
	runID := id.New("eval")
	var hits, cacheHits int
	latencies := make([]int64, 0, len(items))
	metricSums := map[string]float64{}
	type itemResult struct {
		itemID  string
		answer  string
		metrics map[string]float64
	}
	var itemResults []itemResult
	for _, item := range items {
		resp, err := r.RAG.Query(ctx, rag.QueryRequest{
			TenantID:        req.TenantID,
			KnowledgeBaseID: req.KnowledgeBaseID,
			Query:           item.Query,
			Profile:         req.Profile,
			TopK:            req.TopK,
		})
		if err != nil {
			return RunResult{}, err
		}
		latencies = append(latencies, resp.LatencyMS)
		if matches(resp.Answer, item.GroundTruth) || len(resp.Citations) > 0 {
			hits++
		}
		if resp.CacheStatus == "hit" {
			cacheHits++
		}
		itemMetrics := ScoreItemWithOptions(item, resp, ScoreOptions{TopK: req.TopK})
		for name, value := range itemMetrics {
			if shouldAggregateItemMetric(name) {
				metricSums[name] += value
			}
		}
		itemResults = append(itemResults, itemResult{itemID: item.ID, answer: resp.Answer, metrics: itemMetrics})
	}
	total := len(items)
	var score float64
	if total > 0 {
		score = float64(hits) / float64(total)
	}
	metrics := map[string]float64{
		"accuracy":          score,
		"hit_rate":          score,
		PrimaryMetricPairwiseAccuracy: score,
		"latency_p95_ms":    float64(p95(latencies)),
		"cache_hit_rate":    average(float64(cacheHits), total),
	}
	for name, sum := range metricSums {
		metrics[name] = average(sum, total)
	}
	result := RunResult{
		ID:        runID,
		DatasetID: req.DatasetID,
		Profile:   string(req.Profile),
		Total:     total,
		HitRate:   score,
		Accuracy:  score,
		Metrics:   metrics,
		CreatedAt: time.Now().UTC(),
	}
	if r.Repository != nil {
		if err := r.Repository.StoreEvaluationRun(ctx, req.TenantID, result); err != nil {
			return RunResult{}, err
		}
		for _, item := range itemResults {
			if err := r.Repository.StoreEvaluationResult(ctx, runID, item.itemID, item.answer, item.metrics); err != nil {
				return RunResult{}, err
			}
		}
	}
	return result, nil
}

func shouldAggregateItemMetric(name string) bool {
	switch name {
	case "accuracy", "context_recall", "citation_precision",
		"ndcg_at_k", "recall_at_k", "mrr", "map", "coverage", "retrieval_failure_rate",
		"redundancy_rate", "duplicate_count", "deduped_top_k_count",
		"alpha_ndcg", "aspect_coverage":
		return true
	default:
		return false
	}
}

func (r Runner) Get(ctx context.Context, tenantID, id string) (RunResult, bool, error) {
	if r.Repository == nil {
		return RunResult{}, false, nil
	}
	return r.Repository.GetEvaluationRun(ctx, tenantID, id)
}

type MemoryRepository struct {
	mu      sync.RWMutex
	runs    map[string]RunResult
	results map[string][]map[string]float64
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{runs: map[string]RunResult{}, results: map[string][]map[string]float64{}}
}

func (r *MemoryRepository) StoreEvaluationRun(_ context.Context, _ string, result RunResult) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runs[result.ID] = result
	return nil
}

func (r *MemoryRepository) StoreEvaluationResult(_ context.Context, runID, _ string, _ string, metrics map[string]float64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.results[runID] = append(r.results[runID], metrics)
	return nil
}

func (r *MemoryRepository) GetEvaluationRun(_ context.Context, _ string, id string) (RunResult, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result, ok := r.runs[id]
	return result, ok, nil
}

func matches(answer, groundTruth string) bool {
	answer = strings.ToLower(answer)
	for _, term := range strings.Fields(strings.ToLower(groundTruth)) {
		if len(term) > 3 && strings.Contains(answer, term) {
			return true
		}
	}
	return false
}
