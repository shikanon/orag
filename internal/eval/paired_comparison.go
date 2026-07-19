package eval

import (
	"math"
	"math/rand"
	"sort"

	"github.com/shikanon/orag/internal/dataset"
)

// PairedMetricComparison compares the same dataset items across a baseline and
// candidate run. It is intentionally only emitted after CompareRuns confirms
// that the immutable inputs are comparable.
type PairedMetricComparison struct {
	Metric             string                         `json:"metric"`
	Baseline           float64                        `json:"baseline"`
	Candidate          float64                        `json:"candidate"`
	AbsoluteDelta      float64                        `json:"absolute_delta"`
	RelativeDelta      *float64                       `json:"relative_delta,omitempty"`
	PairedSampleCount  int                            `json:"paired_sample_count"`
	ConfidenceInterval *StatisticalConfidenceInterval `json:"confidence_interval,omitempty"`
	Decision           string                         `json:"decision"`
}

type pairedObservation struct {
	baseline  float64
	candidate float64
	weight    float64
}

// ComparePairedMetrics calculates candidate-minus-baseline deltas from the
// persisted per-item result rows. Missing annotations remain excluded by the
// same eligibility rule used for individual run summaries.
func ComparePairedMetrics(baseline, candidate RunResult, baselineItems, candidateItems []EvaluationItemDetail) []PairedMetricComparison {
	baselineByID := make(map[string]EvaluationItemDetail, len(baselineItems))
	for _, item := range baselineItems {
		baselineByID[item.DatasetItemID] = item
	}
	snapshotItems := baseline.DatasetSnapshot.Items
	if len(snapshotItems) == 0 {
		snapshotItems = baseline.Manifest.Dataset.Items
	}
	itemsByID := make(map[string]datasetItemForComparison, len(snapshotItems))
	for _, item := range snapshotItems {
		itemsByID[item.ID] = datasetItemForComparison{item: item}
	}
	observations := map[string][]pairedObservation{}
	for _, candidateItem := range candidateItems {
		baselineItem, ok := baselineByID[candidateItem.DatasetItemID]
		if !ok {
			continue
		}
		frozen, ok := itemsByID[candidateItem.DatasetItemID]
		if !ok {
			continue
		}
		for metric, candidateValue := range candidateItem.Metrics {
			baselineValue, ok := baselineItem.Metrics[metric]
			if !ok || !shouldAggregateItemMetric(metric) || !metricEligible(metric, frozen.item) {
				continue
			}
			observations[metric] = append(observations[metric], pairedObservation{baseline: baselineValue, candidate: candidateValue, weight: itemWeight(frozen.item)})
		}
	}
	comparisons := make([]PairedMetricComparison, 0, len(observations))
	for metric, pairs := range observations {
		if len(pairs) == 0 {
			continue
		}
		comparisons = append(comparisons, summarizePairedMetric(metric, pairs, int64(stableMetricSeed(candidate.ID, "paired:"+metric))))
	}
	sort.Slice(comparisons, func(i, j int) bool { return comparisons[i].Metric < comparisons[j].Metric })
	return comparisons
}

// The alias keeps the comparison loop readable without exporting dataset.Item
// solely for an internal helper.
type datasetItemForComparison struct{ item dataset.Item }

func summarizePairedMetric(metric string, pairs []pairedObservation, seed int64) PairedMetricComparison {
	var baselineSum, candidateSum, totalWeight float64
	for _, pair := range pairs {
		weight := pair.weight
		if weight <= 0 {
			weight = 1
		}
		baselineSum += pair.baseline * weight
		candidateSum += pair.candidate * weight
		totalWeight += weight
	}
	baseline := weightedAverage(baselineSum, totalWeight)
	candidate := weightedAverage(candidateSum, totalWeight)
	comparison := PairedMetricComparison{Metric: metric, Baseline: baseline, Candidate: candidate, AbsoluteDelta: candidate - baseline, PairedSampleCount: len(pairs), Decision: "insufficient_sample"}
	if baseline != 0 {
		relative := comparison.AbsoluteDelta / math.Abs(baseline)
		comparison.RelativeDelta = &relative
	}
	if len(pairs) < 2 {
		return comparison
	}
	comparison.ConfidenceInterval = bootstrapPairedDelta(pairs, seed)
	if comparison.ConfidenceInterval.Low >= 0 {
		comparison.Decision = "improved"
	} else if comparison.ConfidenceInterval.High < 0 {
		comparison.Decision = "regressed"
	} else {
		comparison.Decision = "inconclusive"
	}
	return comparison
}

func bootstrapPairedDelta(pairs []pairedObservation, seed int64) *StatisticalConfidenceInterval {
	rng := rand.New(rand.NewSource(seed))
	deltas := make([]float64, defaultBootstrapSamples)
	for iteration := range deltas {
		var baselineSum, candidateSum, totalWeight float64
		for range pairs {
			pair := pairs[rng.Intn(len(pairs))]
			weight := pair.weight
			if weight <= 0 {
				weight = 1
			}
			baselineSum += pair.baseline * weight
			candidateSum += pair.candidate * weight
			totalWeight += weight
		}
		deltas[iteration] = weightedAverage(candidateSum, totalWeight) - weightedAverage(baselineSum, totalWeight)
	}
	sort.Float64s(deltas)
	return &StatisticalConfidenceInterval{Low: deltas[int(math.Floor(float64(len(deltas)-1)*0.025))], High: deltas[int(math.Ceil(float64(len(deltas)-1)*0.975))], ConfidenceLevel: 0.95, Method: "bootstrap"}
}
