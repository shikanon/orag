package eval

import (
	"math"
	"math/rand"
	"sort"
)

const defaultBootstrapSamples = 2000

// StatisticalConfidenceInterval describes uncertainty for an aggregated metric.
type StatisticalConfidenceInterval struct {
	Low             float64 `json:"low"`
	High            float64 `json:"high"`
	ConfidenceLevel float64 `json:"confidence_level"`
	Method          string  `json:"method"`
}

// MetricSummary keeps a numeric compatibility value while exposing whether the
// number is based on sufficient, appropriately annotated evidence.
type MetricSummary struct {
	Value                float64                        `json:"value"`
	EligibleSampleCount  int                            `json:"eligible_sample_count"`
	TotalSampleCount     int                            `json:"total_sample_count"`
	AnnotationCoverage   float64                        `json:"annotation_coverage"`
	WeightedSampleCount  float64                        `json:"weighted_sample_count"`
	EffectiveSampleCount float64                        `json:"effective_sample_count"`
	ConfidenceInterval   *StatisticalConfidenceInterval `json:"confidence_interval,omitempty"`
}

type metricObservation struct {
	value  float64
	weight float64
}

func summarizeMetric(observations []metricObservation, total int, seed int64) MetricSummary {
	summary := MetricSummary{TotalSampleCount: total, EligibleSampleCount: len(observations)}
	if total > 0 {
		summary.AnnotationCoverage = float64(len(observations)) / float64(total)
	}
	if len(observations) == 0 {
		return summary
	}
	var weightedSum, totalWeight, squaredWeight float64
	values := make([]float64, 0, len(observations))
	for _, observation := range observations {
		weight := observation.weight
		if weight <= 0 {
			weight = 1
		}
		weightedSum += observation.value * weight
		totalWeight += weight
		squaredWeight += weight * weight
		values = append(values, observation.value)
	}
	summary.Value = weightedAverage(weightedSum, totalWeight)
	summary.WeightedSampleCount = totalWeight
	if squaredWeight > 0 {
		summary.EffectiveSampleCount = totalWeight * totalWeight / squaredWeight
	}
	if len(values) < 2 {
		return summary
	}
	if isBinary(values) {
		summary.ConfidenceInterval = wilsonInterval(sum(values), float64(len(values)))
		return summary
	}
	summary.ConfidenceInterval = bootstrapWeightedMean(observations, seed)
	return summary
}

func isBinary(values []float64) bool {
	for _, value := range values {
		if value != 0 && value != 1 {
			return false
		}
	}
	return true
}

func sum(values []float64) float64 {
	var total float64
	for _, value := range values {
		total += value
	}
	return total
}

func wilsonInterval(successes, n float64) *StatisticalConfidenceInterval {
	if n <= 0 {
		return nil
	}
	const z = 1.959963984540054
	denominator := 1 + z*z/n
	center := (successes/n + z*z/(2*n)) / denominator
	margin := z * math.Sqrt((successes/n*(1-successes/n)+z*z/(4*n))/n) / denominator
	return &StatisticalConfidenceInterval{Low: math.Max(0, center-margin), High: math.Min(1, center+margin), ConfidenceLevel: 0.95, Method: "wilson"}
}

func bootstrapWeightedMean(observations []metricObservation, seed int64) *StatisticalConfidenceInterval {
	if len(observations) < 2 {
		return nil
	}
	rng := rand.New(rand.NewSource(seed))
	means := make([]float64, defaultBootstrapSamples)
	for iteration := range means {
		var weightedSum, totalWeight float64
		for range observations {
			observation := observations[rng.Intn(len(observations))]
			weight := observation.weight
			if weight <= 0 {
				weight = 1
			}
			weightedSum += observation.value * weight
			totalWeight += weight
		}
		means[iteration] = weightedAverage(weightedSum, totalWeight)
	}
	sort.Float64s(means)
	return &StatisticalConfidenceInterval{
		Low:             means[int(math.Floor(float64(len(means)-1)*0.025))],
		High:            means[int(math.Ceil(float64(len(means)-1)*0.975))],
		ConfidenceLevel: 0.95,
		Method:          "bootstrap",
	}
}
