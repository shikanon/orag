package offlineknowledge

import (
	"context"

	"github.com/shikanon/orag/internal/eval"
	"github.com/shikanon/orag/internal/kb"
	"github.com/shikanon/orag/internal/rag"
)

type DisabledConclusionJudge struct{}

func (DisabledConclusionJudge) JudgeConclusion(context.Context, OptimizationItem, []Evidence) (bool, error) {
	return false, ErrConclusionDisabled
}

type EvalConclusionJudge struct {
	Judge    eval.QAGJudge
	MinScore float64
}

func NewEvalConclusionJudge(judge eval.QAGJudge, minScore float64) EvalConclusionJudge {
	if minScore == 0 {
		minScore = 0.8
	}
	return EvalConclusionJudge{Judge: judge, MinScore: minScore}
}

func (j EvalConclusionJudge) JudgeConclusion(ctx context.Context, item OptimizationItem, evidence []Evidence) (bool, error) {
	if j.Judge == nil {
		return false, ErrConclusionUnavailable
	}
	out, err := j.Judge.ScoreQAG(ctx, eval.JudgeInput{
		TenantID:         item.TenantID,
		Query:            item.CanonicalQuestion,
		Answer:           item.FinalAnswer,
		ExpectedEvidence: evidenceQuotes(evidence),
		Citations:        evidenceCitations(evidence),
		RetrievedChunks:  evidenceRetrievedChunks(item, evidence),
		CandidateID:      item.ID,
	})
	if err != nil {
		return false, err
	}
	minScore := j.MinScore
	if minScore == 0 {
		minScore = 0.8
	}
	return out.Score >= minScore, nil
}

func evidenceQuotes(evidence []Evidence) []string {
	out := make([]string, 0, len(evidence))
	for _, item := range evidence {
		if item.Quote != "" {
			out = append(out, item.Quote)
		}
	}
	return out
}

func evidenceCitations(evidence []Evidence) []rag.Citation {
	out := make([]rag.Citation, 0, len(evidence))
	for _, item := range evidence {
		out = append(out, rag.Citation{
			ChunkID:    item.ChunkID,
			DocumentID: item.DocID,
			Quote:      item.Quote,
		})
	}
	return out
}

func evidenceRetrievedChunks(item OptimizationItem, evidence []Evidence) []kb.SearchResult {
	out := make([]kb.SearchResult, 0, len(evidence))
	for i, ev := range evidence {
		out = append(out, kb.SearchResult{
			Chunk: kb.Chunk{
				ID:              ev.ChunkID,
				TenantID:        item.TenantID,
				KnowledgeBaseID: item.KBID,
				DocumentID:      ev.DocID,
				Content:         ev.Quote,
			},
			Rank: i + 1,
		})
	}
	return out
}
