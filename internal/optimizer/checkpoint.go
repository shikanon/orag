package optimizer

import "time"

type RunStatus string

const (
	RunStatusQueued        RunStatus = "queued"
	RunStatusRunning       RunStatus = "running"
	RunStatusCompleted     RunStatus = "completed"
	RunStatusCanceling     RunStatus = "canceling"
	RunStatusCanceled      RunStatus = "canceled"
	RunStatusBudgetStopped RunStatus = "budget_stopped"
	RunStatusFailed        RunStatus = "failed"
)

type CandidateStatus string

const (
	CandidateStatusQueued           CandidateStatus = "queued"
	CandidateStatusRunning          CandidateStatus = "running"
	CandidateStatusEvaluated        CandidateStatus = "evaluated"
	CandidateStatusJudged           CandidateStatus = "judged"
	CandidateStatusScored           CandidateStatus = "scored"
	CandidateStatusPromoted         CandidateStatus = "promoted"
	CandidateStatusHoldoutEvaluated CandidateStatus = "holdout_evaluated"
	CandidateStatusCleanupDone      CandidateStatus = "cleanup_done"
	CandidateStatusFailed           CandidateStatus = "failed"
)

type Budget struct {
	MaxJudgeCalls      int           `json:"max_judge_calls,omitempty"`
	MaxCostUSD         float64       `json:"max_cost_usd,omitempty"`
	MaxWallTimeSeconds int           `json:"max_wall_time_seconds,omitempty"`
	MaxWallTime        time.Duration `json:"-"`
}

type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
}

type Checkpoint struct {
	Stage                 string            `json:"stage,omitempty"`
	CompletedCandidateIDs []string          `json:"completed_candidate_ids,omitempty"`
	FailedCandidateIDs    []string          `json:"failed_candidate_ids,omitempty"`
	BestCandidateID       string            `json:"best_candidate_id,omitempty"`
	HoldoutCandidateID    string            `json:"holdout_candidate_id,omitempty"`
	JudgeCalls            int               `json:"judge_calls,omitempty"`
	TokenUsage            TokenUsage        `json:"token_usage,omitempty"`
	CostUSD               float64           `json:"cost_usd,omitempty"`
	TempNamespaces        []TempNamespace   `json:"temp_namespaces,omitempty"`
	LastCandidateID       string            `json:"last_candidate_id,omitempty"`
	CancelRequestedAt     *time.Time        `json:"cancel_requested_at,omitempty"`
	StatusReason          string            `json:"status_reason,omitempty"`
	Metadata              map[string]string `json:"metadata,omitempty"`
}

func (c Checkpoint) completedSet() map[string]struct{} {
	out := make(map[string]struct{}, len(c.CompletedCandidateIDs))
	for _, id := range c.CompletedCandidateIDs {
		out[id] = struct{}{}
	}
	return out
}

func (c *Checkpoint) markCompleted(candidateID string) {
	if candidateID == "" {
		return
	}
	for _, id := range c.CompletedCandidateIDs {
		if id == candidateID {
			return
		}
	}
	c.CompletedCandidateIDs = append(c.CompletedCandidateIDs, candidateID)
	c.LastCandidateID = candidateID
}

func (c *Checkpoint) markFailed(candidateID string) {
	if candidateID == "" {
		return
	}
	for _, id := range c.FailedCandidateIDs {
		if id == candidateID {
			return
		}
	}
	c.FailedCandidateIDs = append(c.FailedCandidateIDs, candidateID)
	c.LastCandidateID = candidateID
}

func (b Budget) wallTime() time.Duration {
	if b.MaxWallTime > 0 {
		return b.MaxWallTime
	}
	if b.MaxWallTimeSeconds > 0 {
		return time.Duration(b.MaxWallTimeSeconds) * time.Second
	}
	return 0
}
