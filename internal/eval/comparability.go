package eval

// Comparability reports whether two evaluations share the immutable evidence
// required for a safe automated comparison. It does not judge which run is
// better; callers use it before interpreting metric deltas.
type Comparability struct {
	Comparable     bool     `json:"comparable"`
	HardMismatches []string `json:"hard_mismatches,omitempty"`
	SoftMismatches []string `json:"soft_mismatches,omitempty"`
}

func CompareRuns(baseline, candidate RunResult) Comparability {
	comparison := Comparability{Comparable: true}
	if baseline.EvaluationFingerprint == "" || candidate.EvaluationFingerprint == "" {
		comparison.hard("evaluation_fingerprint")
		return comparison
	}
	left, right := baseline.Manifest, candidate.Manifest
	if left.Dataset.ContentHash != right.Dataset.ContentHash {
		comparison.hard("dataset.content_hash")
	}
	if left.Dataset.Split != right.Dataset.Split {
		comparison.hard("dataset.split")
	}
	if left.KnowledgeBaseID != right.KnowledgeBaseID {
		comparison.hard("knowledge_base_id")
	}
	if left.CodeCommit != right.CodeCommit {
		comparison.soft("code_commit")
	}
	if left.Profile != right.Profile {
		comparison.soft("profile")
	}
	if left.TopK != right.TopK {
		comparison.soft("top_k")
	}
	if left.JudgeConfigHash != right.JudgeConfigHash || left.QAGConfigHash != right.QAGConfigHash || left.PairwiseConfigHash != right.PairwiseConfigHash {
		comparison.hard("judge_config_hash")
	}
	return comparison
}

func (c *Comparability) hard(name string) {
	c.Comparable = false
	c.HardMismatches = append(c.HardMismatches, name)
}

func (c *Comparability) soft(name string) { c.SoftMismatches = append(c.SoftMismatches, name) }
