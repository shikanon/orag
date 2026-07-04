package eval

import (
	"math"
	"testing"
)

func TestCalibrateJudgeGoldSetPassesDimensionThresholds(t *testing.T) {
	report, err := CalibrateJudgeGoldSet(CalibrationInput{
		Examples: []CalibrationExample{
			calibrationExample(0.1, 0.2, "bad", "bad", 0.9),
			calibrationExample(0.5, 0.4, "mixed", "mixed", 0.8),
			calibrationExample(0.9, 0.8, "good", "good", 1.0),
		},
		Metrics: []string{"faithfulness", "qag_score"},
	})
	if err != nil {
		t.Fatalf("CalibrateJudgeGoldSet() error = %v", err)
	}

	faithfulness := report.Metrics["faithfulness"]
	if !report.ReportAllowed || !report.PromotionAllowed || !faithfulness.PromotionAllowed {
		t.Fatalf("report gates = %#v faithfulness=%#v, want allowed", report, faithfulness)
	}
	if !nearlyEqual(faithfulness.Spearman, 1) || !nearlyEqual(faithfulness.CohenKappa, 1) {
		t.Fatalf("faithfulness calibration = %#v, want perfect agreement", faithfulness)
	}
	qag := report.Metrics["qag_score"]
	if !nearlyEqual(qag.QAGClaimCoverage, 0.9) || qag.QAGCoverageSampleCount != 3 {
		t.Fatalf("qag coverage = %#v, want average coverage 0.9 from 3 samples", qag)
	}
}

func TestCalibrateJudgeGoldSetBlocksBelowThresholdWithoutWaiver(t *testing.T) {
	report, err := CalibrateJudgeGoldSet(CalibrationInput{
		Examples: []CalibrationExample{
			calibrationExample(0.1, 0.9, "bad", "good", 1),
			calibrationExample(0.5, 0.4, "mixed", "mixed", 1),
			calibrationExample(0.9, 0.1, "good", "bad", 1),
		},
		Metrics: []string{"faithfulness"},
	})
	if err != nil {
		t.Fatalf("CalibrateJudgeGoldSet() error = %v", err)
	}

	metric := report.Metrics["faithfulness"]
	if report.ReportAllowed || report.PromotionAllowed || metric.ReportAllowed || metric.PromotionAllowed {
		t.Fatalf("below-threshold report = %#v metric=%#v, want both gates blocked", report, metric)
	}
	if len(metric.BelowThreshold) == 0 {
		t.Fatalf("BelowThreshold is empty for %#v", metric)
	}
}

func TestCalibrateJudgeGoldSetAllowsReportButNotPromotionWithWaiver(t *testing.T) {
	report, err := CalibrateJudgeGoldSet(CalibrationInput{
		Examples: []CalibrationExample{
			{
				HumanScores: map[string]float64{"answer_relevance": 0.1},
				JudgeScores: map[string]float64{"answer_relevance": 0.9},
				HumanLabels: map[string]string{"answer_relevance": "bad"},
				JudgeLabels: map[string]string{"answer_relevance": "good"},
			},
			{
				HumanScores: map[string]float64{"answer_relevance": 0.9},
				JudgeScores: map[string]float64{"answer_relevance": 0.1},
				HumanLabels: map[string]string{"answer_relevance": "good"},
				JudgeLabels: map[string]string{"answer_relevance": "bad"},
			},
		},
		Metrics: []string{"answer_relevance"},
		Waivers: []CalibrationWaiver{{
			Metric:      "answer_relevance",
			AllowReport: true,
			Reason:      "Human review accepts this subjective metric for reporting only.",
			Reviewer:    "eval-owner",
		}},
	})
	if err != nil {
		t.Fatalf("CalibrateJudgeGoldSet() error = %v", err)
	}

	metric := report.Metrics["answer_relevance"]
	if !report.ReportAllowed || metric.PromotionAllowed || report.PromotionAllowed {
		t.Fatalf("waived report = %#v metric=%#v, want report allowed and promotion blocked", report, metric)
	}
	if metric.Kind != CalibrationDimensionSubjective || metric.Thresholds.CohenKappa != 0.6 {
		t.Fatalf("subjective thresholds = %#v, want kappa threshold 0.6", metric)
	}
}

func TestCalibrateJudgeGoldSetAllowsPromotionOnlyWithExplicitPromotionWaiver(t *testing.T) {
	report, err := CalibrateJudgeGoldSet(CalibrationInput{
		Examples: []CalibrationExample{
			calibrationExample(0.1, 0.9, "bad", "good", 0.6),
			calibrationExample(0.9, 0.1, "good", "bad", 0.6),
		},
		Metrics: []string{"faithfulness", "qag_score"},
		Waivers: []CalibrationWaiver{
			{
				Metric:         "faithfulness",
				AllowPromotion: true,
				Reason:         "Manual audit found judge labels acceptable for this release.",
				Reviewer:       "eval-owner",
			},
			{
				Metric:         "qag_score",
				AllowPromotion: true,
				Reason:         "Manual QAG claim coverage spot-check passed.",
				Reviewer:       "eval-owner",
			},
		},
	})
	if err != nil {
		t.Fatalf("CalibrateJudgeGoldSet() error = %v", err)
	}

	if !report.ReportAllowed || !report.PromotionAllowed {
		t.Fatalf("promotion waiver report = %#v, want both gates allowed", report)
	}
	if report.Metrics["faithfulness"].Waiver == nil || report.Metrics["qag_score"].Waiver == nil {
		t.Fatalf("waivers were not recorded in report: %#v", report.Metrics)
	}
}

func TestCalibrateJudgeGoldSetSpearmanHandlesTiedRanks(t *testing.T) {
	report, err := CalibrateJudgeGoldSet(CalibrationInput{
		Examples: []CalibrationExample{
			{HumanScores: map[string]float64{"completeness": 1}, JudgeScores: map[string]float64{"completeness": 1}},
			{HumanScores: map[string]float64{"completeness": 1}, JudgeScores: map[string]float64{"completeness": 1}},
			{HumanScores: map[string]float64{"completeness": 2}, JudgeScores: map[string]float64{"completeness": 2}},
			{HumanScores: map[string]float64{"completeness": 3}, JudgeScores: map[string]float64{"completeness": 3}},
		},
		Metrics: []string{"completeness"},
	})
	if err != nil {
		t.Fatalf("CalibrateJudgeGoldSet() error = %v", err)
	}

	metric := report.Metrics["completeness"]
	if !nearlyEqual(metric.Spearman, 1) {
		t.Fatalf("spearman with tied ranks = %v, want 1", metric.Spearman)
	}
	if metric.SpearmanSampleCount != 4 {
		t.Fatalf("spearman samples = %d, want 4", metric.SpearmanSampleCount)
	}
}

func TestCalibrateJudgeGoldSetRejectsUnknownMetric(t *testing.T) {
	_, err := CalibrateJudgeGoldSet(CalibrationInput{
		Metrics: []string{"not_registered"},
		Examples: []CalibrationExample{{
			HumanScores: map[string]float64{"not_registered": 1},
			JudgeScores: map[string]float64{"not_registered": 1},
		}},
	})
	if err == nil {
		t.Fatal("CalibrateJudgeGoldSet() error = nil, want unknown metric validation error")
	}
}

func calibrationExample(humanScore, judgeScore float64, humanLabel, judgeLabel string, qagCoverage float64) CalibrationExample {
	return CalibrationExample{
		HumanScores: map[string]float64{
			"faithfulness": humanScore,
			"qag_score":    humanScore,
		},
		JudgeScores: map[string]float64{
			"faithfulness": judgeScore,
			"qag_score":    judgeScore,
		},
		HumanLabels: map[string]string{
			"faithfulness": humanLabel,
			"qag_score":    humanLabel,
		},
		JudgeLabels: map[string]string{
			"faithfulness": judgeLabel,
			"qag_score":    judgeLabel,
		},
		QAGMetrics: map[string]float64{"qag_claim_coverage": qagCoverage},
	}
}

func nearlyEqual(left, right float64) bool {
	return math.Abs(left-right) < 1e-9
}
