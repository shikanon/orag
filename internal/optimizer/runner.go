package optimizer

import (
	"context"
	"time"

	"github.com/shikanon/orag/internal/eval"
	"github.com/shikanon/orag/internal/rag"
)

type CandidateRunner interface {
	RunCandidate(ctx context.Context, req CandidateRunRequest) (CandidateRunResult, error)
}

type EvaluationRunner interface {
	Run(ctx context.Context, req eval.RunRequest) (eval.RunResult, error)
}

type CandidateRunRequest struct {
	TenantID        string
	DatasetID       string
	KnowledgeBaseID string
	Candidate       CandidateConfig
	Profile         rag.Profile
	TopK            int
	NamespaceTTL    time.Duration
	Phase           string
	Split           string
}

type CandidateRunResult struct {
	CandidateID    string
	EvaluationRun  eval.RunResult
	Metrics        map[string]float64
	TempNamespaces []TempNamespace
	CleanupStatus  CleanupStatus
}
