package eval

import (
	"strings"

	"github.com/shikanon/orag/internal/dataset"
)

const (
	HoldoutGateReasonMissingSplit       = "missing_split"
	HoldoutGateReasonInsufficientSample = "insufficient_sample"
	HoldoutGateReasonMetricMissing      = "metric_missing"
	HoldoutGateReasonQualityBelowMin    = "quality_below_threshold"
)

type HoldoutGateConfig struct {
	Enabled                bool    `json:"enabled,omitempty"`
	MinSampleCount         int     `json:"min_sample_count,omitempty"`
	MinWeightedSampleCount float64 `json:"min_weighted_sample_count,omitempty"`
	QualityMetric          string  `json:"quality_metric,omitempty"`
	MinQuality             float64 `json:"min_quality,omitempty"`
}

type HoldoutGateResult struct {
	Enabled                bool                 `json:"enabled,omitempty"`
	Passed                 bool                 `json:"passed"`
	Reasons                []string             `json:"reasons,omitempty"`
	Split                  dataset.DatasetSplit `json:"split,omitempty"`
	QualityMetric          string               `json:"quality_metric,omitempty"`
	Quality                float64              `json:"quality,omitempty"`
	MinQuality             float64              `json:"min_quality,omitempty"`
	SampleCount            int                  `json:"sample_count,omitempty"`
	MinSampleCount         int                  `json:"min_sample_count,omitempty"`
	WeightedSampleCount    float64              `json:"weighted_sample_count,omitempty"`
	MinWeightedSampleCount float64              `json:"min_weighted_sample_count,omitempty"`
	MissingSplit           bool                 `json:"missing_split,omitempty"`
}

func EvaluateHoldoutGate(result RunResult, cfg HoldoutGateConfig) HoldoutGateResult {
	if !cfg.Enabled {
		return HoldoutGateResult{}
	}
	metricName := strings.TrimSpace(cfg.QualityMetric)
	if metricName == "" {
		metricName = PrimaryMetricDeterministicAnswerMatch
	}
	gate := HoldoutGateResult{
		Enabled:                true,
		Passed:                 true,
		Split:                  result.Split,
		QualityMetric:          metricName,
		MinQuality:             cfg.MinQuality,
		SampleCount:            result.UnweightedSampleCount,
		MinSampleCount:         cfg.MinSampleCount,
		WeightedSampleCount:    result.WeightedSampleCount,
		MinWeightedSampleCount: cfg.MinWeightedSampleCount,
		MissingSplit:           result.MissingSplit,
	}
	if result.MissingSplit || result.Total == 0 {
		gate.fail(HoldoutGateReasonMissingSplit)
	}
	if cfg.MinSampleCount > 0 && result.UnweightedSampleCount < cfg.MinSampleCount {
		gate.fail(HoldoutGateReasonInsufficientSample)
	}
	if cfg.MinWeightedSampleCount > 0 && result.WeightedSampleCount < cfg.MinWeightedSampleCount {
		gate.fail(HoldoutGateReasonInsufficientSample)
	}
	quality, ok := result.Metrics[metricName]
	if !ok {
		quality, ok = fallbackQualityMetric(result, metricName)
	}
	gate.Quality = quality
	if !ok {
		gate.fail(HoldoutGateReasonMetricMissing)
	} else if quality < cfg.MinQuality {
		gate.fail(HoldoutGateReasonQualityBelowMin)
	}
	return gate
}

func fallbackQualityMetric(result RunResult, metricName string) (float64, bool) {
	switch metricName {
	case PrimaryMetricDeterministicAnswerMatch:
		if value, ok := result.Metrics["answer_accuracy"]; ok {
			return value, true
		}
		if value, ok := result.Metrics["accuracy"]; ok {
			return value, true
		}
	case "answer_accuracy", "accuracy":
		if value, ok := result.Metrics[PrimaryMetricDeterministicAnswerMatch]; ok {
			return value, true
		}
	}
	return 0, false
}

func (r *HoldoutGateResult) fail(reason string) {
	r.Passed = false
	for _, existing := range r.Reasons {
		if existing == reason {
			return
		}
	}
	r.Reasons = append(r.Reasons, reason)
}
