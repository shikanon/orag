package eval

import (
	"fmt"
	"sort"

	"github.com/shikanon/orag/internal/platform/apperrors"
)

type MetricDefinition struct {
	Name        string
	Description string
	Aggregate   bool
}

type MetricRegistry struct {
	metrics map[string]MetricDefinition
}

var DefaultMetricRegistry = NewMetricRegistry([]MetricDefinition{
	{Name: "answer_accuracy", Description: "Rule-based answer match against ground truth.", Aggregate: true},
	{Name: "accuracy", Description: "Backward-compatible answer accuracy alias.", Aggregate: true},
	{Name: "hit_rate", Description: "Run-level answer hit rate."},
	{Name: PrimaryMetricPairwiseAccuracy, Description: "Primary optimizer quality metric."},
	{Name: "citation_hit_rate", Description: "Whether an item returned at least one citation.", Aggregate: true},
	{Name: "context_recall", Description: "Share of relevant documents retrieved for an item.", Aggregate: true},
	{Name: "citation_precision", Description: "Share of citations pointing to relevant documents.", Aggregate: true},
	{Name: "latency_ms", Description: "Item query latency in milliseconds."},
	{Name: "latency_p95_ms", Description: "Run query latency p95 in milliseconds."},
	{Name: "cache_hit", Description: "Whether an item was served from cache."},
	{Name: "cache_hit_rate", Description: "Run cache hit rate."},
	{Name: "ndcg_at_k", Description: "Retrieval ranking quality at k.", Aggregate: true},
	{Name: "recall_at_k", Description: "Retrieval recall at k.", Aggregate: true},
	{Name: "mrr", Description: "Mean reciprocal rank.", Aggregate: true},
	{Name: "map", Description: "Mean average precision.", Aggregate: true},
	{Name: "coverage", Description: "Whether any relevant document was retrieved.", Aggregate: true},
	{Name: "retrieval_failure_rate", Description: "Whether retrieval missed all relevant documents.", Aggregate: true},
	{Name: "redundancy_rate", Description: "Share of duplicate retrieved chunks.", Aggregate: true},
	{Name: "duplicate_count", Description: "Duplicate retrieved chunk count.", Aggregate: true},
	{Name: "deduped_top_k_count", Description: "Retrieved chunk count after duplicate removal.", Aggregate: true},
	{Name: "alpha_ndcg", Description: "Diversity-aware alpha nDCG.", Aggregate: true},
	{Name: "aspect_coverage", Description: "Share of annotated aspects covered by retrieved evidence.", Aggregate: true},
	{Name: "faithfulness", Description: "Judge score for evidence-supported answer claims."},
	{Name: "groundedness", Description: "Judge score for grounding in retrieved context."},
	{Name: "answer_relevance", Description: "Judge score for directly answering the user query."},
	{Name: "hallucination", Description: "Judge score for unsupported or fabricated claims."},
	{Name: "completeness", Description: "Judge score for covering required answer content."},
	{Name: "citation_support", Description: "Judge score for citation-backed answer claims."},
	{Name: "qag_score", Description: "Question-answer generation support score.", Aggregate: true},
	{Name: "qag_claim_coverage", Description: "Share of expected evidence covered by QAG claims.", Aggregate: true},
	{Name: "qag_question_count", Description: "Number of QAG claim-verification questions.", Aggregate: true},
	{Name: "qag_unverifiable_rate", Description: "Share of QAG claims that could not be verified from context.", Aggregate: true},
	{Name: "instruction_following", Description: "Judge score for following task instructions."},
	{Name: "safety", Description: "Judge score for safety policy compliance."},
	{Name: "cost_usd", Description: "Evaluation or harness cost in USD."},
	{Name: "prompt_tokens", Description: "Prompt token count."},
	{Name: "completion_tokens", Description: "Completion token count."},
	{Name: "total_tokens", Description: "Total token count."},
})

func NewMetricRegistry(defs []MetricDefinition) MetricRegistry {
	registry := MetricRegistry{metrics: make(map[string]MetricDefinition, len(defs))}
	for _, def := range defs {
		if def.Name == "" {
			continue
		}
		registry.metrics[def.Name] = def
	}
	return registry
}

func (r MetricRegistry) IsRegistered(name string) bool {
	_, ok := r.metrics[name]
	return ok
}

func (r MetricRegistry) ShouldAggregate(name string) bool {
	def, ok := r.metrics[name]
	return ok && def.Aggregate
}

func (r MetricRegistry) Names() []string {
	names := make([]string, 0, len(r.metrics))
	for name := range r.metrics {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func ValidateMetricMap(metrics map[string]float64) error {
	return DefaultMetricRegistry.Validate(metrics)
}

func (r MetricRegistry) Validate(metrics map[string]float64) error {
	for name := range metrics {
		if !r.IsRegistered(name) {
			return apperrors.New(apperrors.CodeValidation, fmt.Sprintf("unknown evaluation metric %q", name))
		}
	}
	return nil
}
