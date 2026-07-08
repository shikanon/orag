package offlineknowledge

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shikanon/orag/internal/eval"
	"github.com/shikanon/orag/internal/kb"
)

func TestStoreSourceReaderReadsChunkTextAndHashes(t *testing.T) {
	ctx := context.Background()
	store := kb.NewMemoryStore()
	doc := kb.Document{
		ID:              "doc_real",
		TenantID:        "tenant_1",
		KnowledgeBaseID: "kb_1",
		SourceURI:       "file://source.md",
		ContentHash:     "doc-hash-v42",
		CreatedAt:       time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC),
	}
	chunkText := "ORAG validates answer conclusions using quoted evidence."
	if err := store.Store(ctx, doc, []kb.Chunk{{
		ID:              "chunk_real",
		TenantID:        "tenant_1",
		KnowledgeBaseID: "kb_1",
		DocumentID:      "doc_real",
		Content:         chunkText,
	}}); err != nil {
		t.Fatal(err)
	}
	reader := NewStoreSourceReader(store)

	got, found, err := reader.ReadSourceChunk(ctx, "tenant_1", "kb_1", "chunk_real")
	if err != nil || !found {
		t.Fatalf("ReadSourceChunk() found=%v err=%v", found, err)
	}
	if got.Text != chunkText || got.DocVersion != "doc-hash-v42" {
		t.Fatalf("ReadSourceChunk() = %#v, want real text and doc version", got)
	}
	wantHash := "sha256:" + stableHash(chunkText)
	if got.ChunkContentHash != wantHash {
		t.Fatalf("ChunkContentHash = %q, want %q", got.ChunkContentHash, wantHash)
	}
}

func TestValidatorRejectsQuoteNotContainedAndMissingSource(t *testing.T) {
	tests := []struct {
		name    string
		source  *fakeSourceReader
		mutate  func(*OptimizationItem)
		wantErr error
	}{
		{
			name:   "quote not contained",
			source: newFakeSourceReader(validSourceChunk()),
			mutate: func(item *OptimizationItem) {
				item.Evidence[0].Quote = "this exact quote is absent from the source"
			},
			wantErr: ErrQuoteNotContained,
		},
		{
			name:    "source missing",
			source:  newFakeSourceReader(),
			mutate:  func(*OptimizationItem) {},
			wantErr: ErrSourceNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := validValidatorItem()
			tt.mutate(&item)
			validator := NewValidator(tt.source, &fakeConclusionJudge{accepted: true}, ValidatorOptions{})

			err := validator.ValidateItem(context.Background(), "tenant_1", "kb_1", item)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ValidateItem() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidatorReturnsExplicitConclusionUnavailableOrDisabled(t *testing.T) {
	item := validValidatorItem()
	source := newFakeSourceReader(validSourceChunk())

	err := NewValidator(source, nil, ValidatorOptions{}).ValidateItem(context.Background(), "tenant_1", "kb_1", item)
	if !errors.Is(err, ErrConclusionUnavailable) {
		t.Fatalf("nil judge error = %v, want %v", err, ErrConclusionUnavailable)
	}

	err = NewValidator(source, DisabledConclusionJudge{}, ValidatorOptions{}).ValidateItem(context.Background(), "tenant_1", "kb_1", item)
	if !errors.Is(err, ErrConclusionDisabled) {
		t.Fatalf("disabled judge error = %v, want %v", err, ErrConclusionDisabled)
	}
}

func TestEvalConclusionJudgePassFail(t *testing.T) {
	item := validValidatorItem()
	evidence := item.Evidence

	passJudge := NewEvalConclusionJudge(&fakeQAGJudge{score: 0.92}, 0.8)
	accepted, err := passJudge.JudgeConclusion(context.Background(), item, evidence)
	if err != nil || !accepted {
		t.Fatalf("JudgeConclusion() accepted=%v err=%v, want pass", accepted, err)
	}
	if passJudge.Judge.(*fakeQAGJudge).got.Answer != item.FinalAnswer {
		t.Fatalf("JudgeConclusion() did not pass final answer to eval judge")
	}
	if len(passJudge.Judge.(*fakeQAGJudge).got.ExpectedEvidence) != len(evidence) {
		t.Fatalf("ExpectedEvidence = %#v, want evidence quotes", passJudge.Judge.(*fakeQAGJudge).got.ExpectedEvidence)
	}

	failJudge := NewEvalConclusionJudge(&fakeQAGJudge{score: 0.5}, 0.8)
	accepted, err = failJudge.JudgeConclusion(context.Background(), item, evidence)
	if err != nil || accepted {
		t.Fatalf("JudgeConclusion() accepted=%v err=%v, want fail", accepted, err)
	}
}

type fakeQAGJudge struct {
	score float64
	got   eval.JudgeInput
}

func (j *fakeQAGJudge) ScoreQAG(_ context.Context, input eval.JudgeInput) (eval.QAGOutput, error) {
	j.got = input
	return eval.QAGOutput{Score: j.score}, nil
}
