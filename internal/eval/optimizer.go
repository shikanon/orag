package eval

import (
	"context"

	"github.com/shikanon/orag/internal/platform/id"
	"github.com/shikanon/orag/internal/rag"
)

type OptimizeRequest struct {
	TenantID        string        `json:"-"`
	DatasetID       string        `json:"dataset_id"`
	KnowledgeBaseID string        `json:"knowledge_base_id"`
	Profiles        []rag.Profile `json:"profiles,omitempty"`
	TopKs           []int         `json:"top_ks,omitempty"`
}

type CandidateResult struct {
	Profile rag.Profile `json:"profile"`
	TopK    int         `json:"top_k"`
	Score   float64     `json:"score"`
	RunID   string      `json:"run_id"`
}

type OptimizeResult struct {
	ID         string            `json:"id"`
	Status     string            `json:"status"`
	Best       CandidateResult   `json:"best"`
	Candidates []CandidateResult `json:"candidates"`
}

type Optimizer struct {
	Runner Runner
}

func (o Optimizer) Optimize(ctx context.Context, req OptimizeRequest) (OptimizeResult, error) {
	profiles := req.Profiles
	if len(profiles) == 0 {
		profiles = []rag.Profile{rag.ProfileRealtime, rag.ProfileHighPrecision}
	}
	topKs := req.TopKs
	if len(topKs) == 0 {
		topKs = []int{8}
	}
	out := OptimizeResult{ID: id.New("opt"), Status: "completed"}
	for _, profile := range profiles {
		for _, topK := range topKs {
			run, err := o.Runner.Run(ctx, RunRequest{
				TenantID:        req.TenantID,
				DatasetID:       req.DatasetID,
				KnowledgeBaseID: req.KnowledgeBaseID,
				Profile:         profile,
				TopK:            topK,
			})
			if err != nil {
				return OptimizeResult{}, err
			}
			candidate := CandidateResult{Profile: profile, TopK: topK, Score: run.Accuracy, RunID: run.ID}
			out.Candidates = append(out.Candidates, candidate)
			if candidate.Score >= out.Best.Score {
				out.Best = candidate
			}
		}
	}
	return out, nil
}
