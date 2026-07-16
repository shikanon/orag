package tutorial

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"slices"
	"strings"
	"time"
)

//go:embed replays/text-rag-1.0.0-benchmark-replay-v1.json
var embeddedTextRAGReplay []byte

func (c *Catalog) loadOfficialReplays() error {
	replay, err := parseReplay(embeddedTextRAGReplay)
	if err != nil {
		return fmt.Errorf("decode official tutorial replay: %w", err)
	}
	template, err := c.Get(replay.TemplateID, replay.TemplateVersion)
	if err != nil {
		return fmt.Errorf("official replay template: %w", err)
	}
	if !template.ReplayAvailable {
		return fmt.Errorf("official replay %q is not enabled by catalog", replay.ID)
	}
	if _, exists := c.replays[replay.TemplateID]; exists {
		return fmt.Errorf("duplicate official replay for %q", replay.TemplateID)
	}
	c.replays[replay.TemplateID] = replay
	return nil
}

func parseReplay(raw []byte) (ReplaySnapshot, error) {
	// Replays are public aggregates. Reject recognisable confidential/storage
	// field names before parsing, even if a future struct accidentally grows.
	lower := strings.ToLower(string(raw))
	for _, forbidden := range []string{"access_key", "secret", "api_key", "object_key", "manifest_url", "private_store", "credential"} {
		if strings.Contains(lower, forbidden) {
			return ReplaySnapshot{}, fmt.Errorf("replay contains forbidden field %q", forbidden)
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var replay ReplaySnapshot
	if err := decoder.Decode(&replay); err != nil {
		return ReplaySnapshot{}, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return ReplaySnapshot{}, fmt.Errorf("replay contains multiple JSON values")
	}
	if err := validateReplay(replay); err != nil {
		return ReplaySnapshot{}, err
	}
	replay.Fingerprint = replayFingerprint(replay)
	return replay, nil
}

func validateReplay(replay ReplaySnapshot) error {
	if replay.ID != "text-rag/1.0.0/benchmark/replay-v1" || replay.TemplateID != "text-rag" || replay.TemplateVersion != "1.0.0" || replay.PackTier != "benchmark" {
		return fmt.Errorf("replay identity is invalid")
	}
	if !sha256Pattern.MatchString(replay.PackManifestSHA256) || !sha256Pattern.MatchString(replay.RuntimeEnvironmentSHA256) {
		return fmt.Errorf("replay SHA-256 is invalid")
	}
	if replay.BuildRevision == "" || replay.EvaluatorVersion != "tutorial_eval_v5" || strings.TrimSpace(replay.Summary) == "" {
		return fmt.Errorf("replay provenance is incomplete")
	}
	if _, err := time.Parse(time.RFC3339, replay.GeneratedAt); err != nil {
		return fmt.Errorf("replay generated_at is invalid")
	}
	if err := validateReplayVariant(replay.Baseline, "baseline", TutorialBaselineContextPackTopN); err != nil {
		return fmt.Errorf("baseline: %w", err)
	}
	if err := validateReplayVariant(replay.Candidate, TutorialP8ContextPackCandidateID, TutorialP8ContextPackTopN); err != nil {
		return fmt.Errorf("candidate: %w", err)
	}
	if replay.Baseline.Profile != replay.Candidate.Profile || replay.Baseline.TopK != replay.Candidate.TopK || replay.Baseline.ContextPackMaxTokens != replay.Candidate.ContextPackMaxTokens {
		return fmt.Errorf("replay P0 and P8 are not comparable")
	}
	return nil
}

func validateReplayVariant(variant ReplayVariant, expectedID string, expectedTopN int) error {
	if variant.Variant != expectedID || variant.Profile != "high_precision" || variant.TopK != 8 || variant.ContextPackTopN != expectedTopN || variant.ContextPackMaxTokens != TutorialContextPackMaxTokens {
		return fmt.Errorf("run definition violates the controlled benchmark contract")
	}
	if err := validateReplayMetrics(variant.Metrics); err != nil {
		return fmt.Errorf("metrics: %w", err)
	}
	if err := validateReplayMetrics(variant.IndexMetrics); err != nil {
		return fmt.Errorf("index metrics: %w", err)
	}
	return nil
}

func validateReplayMetrics(metrics []ReplayMetric) error {
	if len(metrics) == 0 {
		return fmt.Errorf("must not be empty")
	}
	names := make(map[string]bool, len(metrics))
	for _, metric := range metrics {
		if strings.TrimSpace(metric.Name) == "" || names[metric.Name] || math.IsNaN(metric.Value) || math.IsInf(metric.Value, 0) || metric.Value < 0 {
			return fmt.Errorf("contains an invalid metric")
		}
		names[metric.Name] = true
	}
	return nil
}

func replayFingerprint(replay ReplaySnapshot) string {
	replay.Fingerprint = ""
	raw, err := json.Marshal(replay)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func cloneReplay(replay ReplaySnapshot) ReplaySnapshot {
	cloned := replay
	cloned.Baseline.Metrics = slices.Clone(replay.Baseline.Metrics)
	cloned.Baseline.IndexMetrics = slices.Clone(replay.Baseline.IndexMetrics)
	cloned.Candidate.Metrics = slices.Clone(replay.Candidate.Metrics)
	cloned.Candidate.IndexMetrics = slices.Clone(replay.Candidate.IndexMetrics)
	return cloned
}
