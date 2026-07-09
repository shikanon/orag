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
	{Name: PrimaryMetricDeterministicAnswerMatch, Description: "Deterministic rule-based answer match against ground truth.", Aggregate: true},
	{Name: "answer_accuracy", Description: "Backward-compatible deterministic answer match alias.", Aggregate: true},
	{Name: "accuracy", Description: "Backward-compatible answer accuracy alias.", Aggregate: true},
	{Name: "hit_rate", Description: "Run-level answer hit rate."},
	{Name: PrimaryMetricPairwiseAccuracy, Description: "Real pairwise judge win-rate/accuracy metric; accepted for historical results and real pairwise judge output.", Aggregate: true},
	{Name: "citation_hit_rate", Description: "Whether an item returned at least one citation.", Aggregate: true},
	{Name: "context_recall", Description: "Share of relevant documents retrieved for an item.", Aggregate: true},
	{Name: "citation_precision", Description: "Share of citations pointing to relevant documents.", Aggregate: true},
	{Name: "latency_ms", Description: "Item query latency in milliseconds.", Aggregate: true},
	{Name: "latency_p95_ms", Description: "Run query latency p95 in milliseconds."},
	{Name: "cache_hit", Description: "Whether an item was served from cache.", Aggregate: true},
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
	{Name: "faithfulness", Description: "Judge score for evidence-supported answer claims.", Aggregate: true},
	{Name: "groundedness", Description: "Judge score for grounding in retrieved context.", Aggregate: true},
	{Name: "answer_relevance", Description: "Judge score for directly answering the user query.", Aggregate: true},
	{Name: "hallucination", Description: "Judge score for unsupported or fabricated claims.", Aggregate: true},
	{Name: "completeness", Description: "Judge score for covering required answer content.", Aggregate: true},
	{Name: "citation_support", Description: "Judge score for citation-backed answer claims.", Aggregate: true},
	{Name: "qag_score", Description: "Question-answer generation support score.", Aggregate: true},
	{Name: "qag_claim_coverage", Description: "Share of expected evidence covered by QAG claims.", Aggregate: true},
	{Name: "qag_question_count", Description: "Number of QAG claim-verification questions.", Aggregate: true},
	{Name: "qag_unverifiable_rate", Description: "Share of QAG claims that could not be verified from context.", Aggregate: true},
	{Name: "instruction_following", Description: "Judge score for following task instructions.", Aggregate: true},
	{Name: "safety", Description: "Judge score for safety policy compliance.", Aggregate: true},
	{Name: "cost_usd", Description: "Evaluation or harness cost in USD."},
	{Name: "prompt_tokens", Description: "Prompt token count."},
	{Name: "completion_tokens", Description: "Completion token count."},
	{Name: "total_tokens", Description: "Total token count."},
	{Name: "weighted_sample_count", Description: "Sum of dataset item weights included in the run."},
	{Name: "unweighted_sample_count", Description: "Number of dataset items included in the run."},
	{Name: "missing_split", Description: "Whether the requested split had no matching items."},
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
