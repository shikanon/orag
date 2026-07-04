package eval

import (
	"math"
	"sort"
	"strings"

	"github.com/shikanon/orag/internal/platform/apperrors"
)

type CalibrationDimensionKind string

const (
	CalibrationDimensionEvidenceCheckable CalibrationDimensionKind = "evidence_checkable"
	CalibrationDimensionSubjective        CalibrationDimensionKind = "subjective"
)

type CalibrationThresholds struct {
	EvidenceCheckableSpearman float64 `json:"evidence_checkable_spearman"`
	EvidenceCheckableKappa    float64 `json:"evidence_checkable_kappa"`
	SubjectiveSpearman        float64 `json:"subjective_spearman"`
	SubjectiveKappa           float64 `json:"subjective_kappa"`
	QAGClaimCoverage          float64 `json:"qag_claim_coverage"`
}

type MetricCalibrationThresholds struct {
	Spearman         float64 `json:"spearman"`
	CohenKappa       float64 `json:"cohen_kappa"`
	QAGClaimCoverage float64 `json:"qag_claim_coverage,omitempty"`
}

type CalibrationWaiver struct {
	Metric         string `json:"metric"`
	AllowReport    bool   `json:"allow_report"`
	AllowPromotion bool   `json:"allow_promotion"`
	Reason         string `json:"reason,omitempty"`
	Reviewer       string `json:"reviewer,omitempty"`
}

type CalibrationExample struct {
	ID          string             `json:"id,omitempty"`
	HumanScores map[string]float64 `json:"human_scores,omitempty"`
	JudgeScores map[string]float64 `json:"judge_scores,omitempty"`
	HumanLabels map[string]string  `json:"human_labels,omitempty"`
	JudgeLabels map[string]string  `json:"judge_labels,omitempty"`
	QAGMetrics  map[string]float64 `json:"qag_metrics,omitempty"`
}

type CalibrationInput struct {
	Examples   []CalibrationExample                `json:"examples"`
	Metrics    []string                            `json:"metrics,omitempty"`
	Kinds      map[string]CalibrationDimensionKind `json:"kinds,omitempty"`
	Thresholds CalibrationThresholds               `json:"thresholds,omitempty"`
	Waivers    []CalibrationWaiver                 `json:"waivers,omitempty"`
}

type CalibrationReport struct {
	SampleCount      int                                `json:"sample_count"`
	Metrics          map[string]CalibrationMetricReport `json:"metrics"`
	ReportAllowed    bool                               `json:"report_allowed"`
	PromotionAllowed bool                               `json:"promotion_allowed"`
}

type CalibrationMetricReport struct {
	Metric                 string                      `json:"metric"`
	Kind                   CalibrationDimensionKind    `json:"kind"`
	SampleCount            int                         `json:"sample_count"`
	Spearman               float64                     `json:"spearman"`
	SpearmanSampleCount    int                         `json:"spearman_sample_count"`
	CohenKappa             float64                     `json:"cohen_kappa"`
	KappaSampleCount       int                         `json:"kappa_sample_count"`
	QAGClaimCoverage       float64                     `json:"qag_claim_coverage,omitempty"`
	QAGCoverageSampleCount int                         `json:"qag_coverage_sample_count,omitempty"`
	Thresholds             MetricCalibrationThresholds `json:"thresholds"`
	BelowThreshold         []string                    `json:"below_threshold,omitempty"`
	ReportAllowed          bool                        `json:"report_allowed"`
	PromotionAllowed       bool                        `json:"promotion_allowed"`
	Waiver                 *CalibrationWaiver          `json:"waiver,omitempty"`
}

func DefaultCalibrationThresholds() CalibrationThresholds {
	return CalibrationThresholds{
		EvidenceCheckableSpearman: 0.8,
		EvidenceCheckableKappa:    0.75,
		SubjectiveSpearman:        0.7,
		SubjectiveKappa:           0.6,
		QAGClaimCoverage:          0.8,
	}
}

func CalibrateJudgeGoldSet(input CalibrationInput) (CalibrationReport, error) {
	thresholds := normalizeCalibrationThresholds(input.Thresholds)
	metrics := calibrationMetrics(input)
	waivers := explicitWaivers(input.Waivers)
	report := CalibrationReport{
		SampleCount:      len(input.Examples),
		Metrics:          make(map[string]CalibrationMetricReport, len(metrics)),
		ReportAllowed:    true,
		PromotionAllowed: true,
	}
	for _, metric := range metrics {
		if !DefaultMetricRegistry.IsRegistered(metric) {
			return CalibrationReport{}, apperrors.New(apperrors.CodeValidation, "unknown calibration metric "+metric)
		}
		metricReport := calibrateMetric(metric, input.Examples, input.Kinds[metric], thresholds, waivers[metric])
		report.Metrics[metric] = metricReport
		if !metricReport.ReportAllowed {
			report.ReportAllowed = false
		}
		if !metricReport.PromotionAllowed {
			report.PromotionAllowed = false
		}
	}
	return report, nil
}

func calibrateMetric(metric string, examples []CalibrationExample, kind CalibrationDimensionKind, thresholds CalibrationThresholds, waiver *CalibrationWaiver) CalibrationMetricReport {
	if kind == "" {
		kind = defaultCalibrationKind(metric)
	}
	metricThresholds := thresholdsForKind(kind, thresholds)
	humanScores, judgeScores := pairedScores(metric, examples)
	humanLabels, judgeLabels := pairedLabels(metric, examples)
	coverageValues := qagCoverageValues(metric, examples)

	report := CalibrationMetricReport{
		Metric:                 metric,
		Kind:                   kind,
		SampleCount:            max(len(humanScores), max(len(humanLabels), len(coverageValues))),
		SpearmanSampleCount:    len(humanScores),
		KappaSampleCount:       len(humanLabels),
		QAGCoverageSampleCount: len(coverageValues),
		Thresholds:             metricThresholds,
		ReportAllowed:          true,
		PromotionAllowed:       true,
	}
	if len(humanScores) >= 2 {
		report.Spearman = spearman(humanScores, judgeScores)
		if report.Spearman < metricThresholds.Spearman {
			report.BelowThreshold = append(report.BelowThreshold, "spearman")
		}
	}
	if len(humanLabels) >= 2 {
		report.CohenKappa = cohenKappa(humanLabels, judgeLabels)
		if report.CohenKappa < metricThresholds.CohenKappa {
			report.BelowThreshold = append(report.BelowThreshold, "cohen_kappa")
		}
	}
	if len(coverageValues) > 0 {
		report.QAGClaimCoverage = mean(coverageValues)
		if report.QAGClaimCoverage < metricThresholds.QAGClaimCoverage {
			report.BelowThreshold = append(report.BelowThreshold, "qag_claim_coverage")
		}
	}
	if report.SampleCount == 0 {
		report.BelowThreshold = append(report.BelowThreshold, "insufficient_samples")
	}
	if len(report.BelowThreshold) > 0 {
		report.ReportAllowed = waiver != nil && (waiver.AllowReport || waiver.AllowPromotion)
		report.PromotionAllowed = waiver != nil && waiver.AllowPromotion
		report.Waiver = waiver
	}
	return report
}

func normalizeCalibrationThresholds(thresholds CalibrationThresholds) CalibrationThresholds {
	defaults := DefaultCalibrationThresholds()
	if thresholds.EvidenceCheckableSpearman <= 0 {
		thresholds.EvidenceCheckableSpearman = defaults.EvidenceCheckableSpearman
	}
	if thresholds.EvidenceCheckableKappa <= 0 {
		thresholds.EvidenceCheckableKappa = defaults.EvidenceCheckableKappa
	}
	if thresholds.SubjectiveSpearman <= 0 {
		thresholds.SubjectiveSpearman = defaults.SubjectiveSpearman
	}
	if thresholds.SubjectiveKappa <= 0 {
		thresholds.SubjectiveKappa = defaults.SubjectiveKappa
	}
	if thresholds.QAGClaimCoverage <= 0 {
		thresholds.QAGClaimCoverage = defaults.QAGClaimCoverage
	}
	return thresholds
}

func thresholdsForKind(kind CalibrationDimensionKind, thresholds CalibrationThresholds) MetricCalibrationThresholds {
	out := MetricCalibrationThresholds{QAGClaimCoverage: thresholds.QAGClaimCoverage}
	switch kind {
	case CalibrationDimensionSubjective:
		out.Spearman = thresholds.SubjectiveSpearman
		out.CohenKappa = thresholds.SubjectiveKappa
	default:
		out.Spearman = thresholds.EvidenceCheckableSpearman
		out.CohenKappa = thresholds.EvidenceCheckableKappa
	}
	return out
}

func defaultCalibrationKind(metric string) CalibrationDimensionKind {
	switch metric {
	case string(JudgeMetricAnswerRelevance), string(JudgeMetricCompleteness), string(JudgeMetricInstructionFollow), string(JudgeMetricSafety):
		return CalibrationDimensionSubjective
	default:
		return CalibrationDimensionEvidenceCheckable
	}
}

func calibrationMetrics(input CalibrationInput) []string {
	set := map[string]struct{}{}
	for _, metric := range input.Metrics {
		if strings.TrimSpace(metric) != "" {
			set[strings.TrimSpace(metric)] = struct{}{}
		}
	}
	if len(set) == 0 {
		for _, example := range input.Examples {
			addMetricKeys(set, example.HumanScores)
			addMetricKeys(set, example.JudgeScores)
			addLabelKeys(set, example.HumanLabels)
			addLabelKeys(set, example.JudgeLabels)
			if _, ok := example.QAGMetrics["qag_claim_coverage"]; ok {
				set["qag_score"] = struct{}{}
			}
		}
	}
	metrics := make([]string, 0, len(set))
	for metric := range set {
		metrics = append(metrics, metric)
	}
	sort.Strings(metrics)
	return metrics
}

func addMetricKeys(set map[string]struct{}, metrics map[string]float64) {
	for key := range metrics {
		set[key] = struct{}{}
	}
}

func addLabelKeys(set map[string]struct{}, labels map[string]string) {
	for key := range labels {
		set[key] = struct{}{}
	}
}

func explicitWaivers(waivers []CalibrationWaiver) map[string]*CalibrationWaiver {
	out := map[string]*CalibrationWaiver{}
	for i := range waivers {
		waiver := waivers[i]
		if strings.TrimSpace(waiver.Metric) == "" || strings.TrimSpace(waiver.Reason) == "" || strings.TrimSpace(waiver.Reviewer) == "" {
			continue
		}
		out[waiver.Metric] = &waiver
	}
	return out
}

func pairedScores(metric string, examples []CalibrationExample) ([]float64, []float64) {
	var human []float64
	var judge []float64
	for _, example := range examples {
		humanScore, humanOK := example.HumanScores[metric]
		judgeScore, judgeOK := example.JudgeScores[metric]
		if humanOK && judgeOK {
			human = append(human, humanScore)
			judge = append(judge, judgeScore)
		}
	}
	return human, judge
}

func pairedLabels(metric string, examples []CalibrationExample) ([]string, []string) {
	var human []string
	var judge []string
	for _, example := range examples {
		humanLabel, humanOK := example.HumanLabels[metric]
		judgeLabel, judgeOK := example.JudgeLabels[metric]
		if humanOK && judgeOK {
			human = append(human, normalizeCalibrationLabel(humanLabel))
			judge = append(judge, normalizeCalibrationLabel(judgeLabel))
		}
	}
	return human, judge
}

func normalizeCalibrationLabel(label string) string {
	return strings.ToLower(strings.TrimSpace(label))
}

func qagCoverageValues(metric string, examples []CalibrationExample) []float64 {
	if metric != "qag_score" && metric != "qag_claim_coverage" {
		return nil
	}
	var values []float64
	for _, example := range examples {
		if coverage, ok := example.QAGMetrics["qag_claim_coverage"]; ok {
			values = append(values, clamp01(coverage))
		}
	}
	return values
}

func spearman(left, right []float64) float64 {
	if len(left) != len(right) || len(left) < 2 {
		return 0
	}
	return pearson(ranks(left), ranks(right))
}

func ranks(values []float64) []float64 {
	type rankedValue struct {
		value float64
		index int
	}
	ranked := make([]rankedValue, len(values))
	for i, value := range values {
		ranked[i] = rankedValue{value: value, index: i}
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		return ranked[i].value < ranked[j].value
	})
	out := make([]float64, len(values))
	for i := 0; i < len(ranked); {
		j := i + 1
		for j < len(ranked) && ranked[j].value == ranked[i].value {
			j++
		}
		rank := (float64(i+1) + float64(j)) / 2
		for k := i; k < j; k++ {
			out[ranked[k].index] = rank
		}
		i = j
	}
	return out
}

func pearson(left, right []float64) float64 {
	meanLeft := mean(left)
	meanRight := mean(right)
	var numerator, leftSquares, rightSquares float64
	for i := range left {
		leftDelta := left[i] - meanLeft
		rightDelta := right[i] - meanRight
		numerator += leftDelta * rightDelta
		leftSquares += leftDelta * leftDelta
		rightSquares += rightDelta * rightDelta
	}
	denominator := math.Sqrt(leftSquares * rightSquares)
	if denominator == 0 {
		return 0
	}
	return numerator / denominator
}

func cohenKappa(human, judge []string) float64 {
	if len(human) != len(judge) || len(human) == 0 {
		return 0
	}
	humanCounts := map[string]int{}
	judgeCounts := map[string]int{}
	agreements := 0
	for i := range human {
		humanCounts[human[i]]++
		judgeCounts[judge[i]]++
		if human[i] == judge[i] {
			agreements++
		}
	}
	total := float64(len(human))
	observed := float64(agreements) / total
	var expected float64
	for label, humanCount := range humanCounts {
		expected += (float64(humanCount) / total) * (float64(judgeCounts[label]) / total)
	}
	if expected == 1 {
		if observed == 1 {
			return 1
		}
		return 0
	}
	return (observed - expected) / (1 - expected)
}

func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var sum float64
	for _, value := range values {
		sum += value
	}
	return sum / float64(len(values))
}
