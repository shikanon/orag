package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/shikanon/orag/internal/ingest/chunker"
	"github.com/shikanon/orag/internal/ingest/parser"
	"github.com/shikanon/orag/internal/kb"
	"github.com/shikanon/orag/internal/platform/id"
)

type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float64, error)
}

var ErrKnowledgeBaseNotFound = errors.New("knowledge base not found")

type Service struct {
	Parser           parser.Parser
	Splitter         chunker.Recursive
	Embedder         Embedder
	KnowledgeBases   kb.KnowledgeBaseRepository
	Indexer          kb.Indexer
	Jobs             JobStore
	MaxDocumentBytes int64
}

type Request struct {
	TenantID        string
	KnowledgeBaseID string
	SourceURI       string
	Name            string
	Content         []byte
}

type Result struct {
	Document kb.Document
	Chunks   []kb.Chunk
	Job      Job
}

func (s *Service) Ingest(ctx context.Context, req Request) (Result, error) {
	if s.KnowledgeBases != nil {
		if _, ok := s.KnowledgeBases.GetKnowledgeBase(req.TenantID, req.KnowledgeBaseID); !ok {
			return Result{}, fmt.Errorf("%w: %s/%s", ErrKnowledgeBaseNotFound, req.TenantID, req.KnowledgeBaseID)
		}
	}

	now := time.Now().UTC()
	job := Job{
		ID:              id.New("job"),
		TenantID:        req.TenantID,
		KnowledgeBaseID: req.KnowledgeBaseID,
		Status:          JobStatusRunning,
		SourceURI:       req.SourceURI,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if s.Jobs != nil {
		created, err := s.Jobs.CreateJob(ctx, job)
		if err != nil {
			return Result{}, err
		}
		job = created
	}
	fail := func(err error) (Result, error) {
		job.Status = JobStatusFailed
		job.Error = err.Error()
		job.UpdatedAt = time.Now().UTC()
		if s.Jobs != nil {
			_ = s.Jobs.UpdateJob(ctx, job)
		}
		return Result{Job: job}, err
	}
	if s.MaxDocumentBytes > 0 && int64(len(req.Content)) > s.MaxDocumentBytes {
		return fail(fmt.Errorf("document exceeds max size %d bytes", s.MaxDocumentBytes))
	}
	parsed, err := s.Parser.Parse(ctx, req.Name, req.Content)
	if err != nil {
		return fail(err)
	}
	split := s.Splitter.Split(parsed.Markdown)
	texts := make([]string, len(split))
	for i := range split {
		texts[i] = split[i].Content
	}
	vectors, err := s.Embedder.Embed(ctx, texts)
	if err != nil {
		return fail(err)
	}
	if len(vectors) != len(split) {
		return fail(fmt.Errorf("embedding count %d does not match chunks %d", len(vectors), len(split)))
	}
	hash := contentHash(req.Content)
	doc := kb.Document{
		ID:              documentID(req.TenantID, req.KnowledgeBaseID, hash),
		TenantID:        req.TenantID,
		KnowledgeBaseID: req.KnowledgeBaseID,
		SourceURI:       req.SourceURI,
		Title:           req.Name,
		ContentHash:     hash,
		Metadata:        parsed.Metadata,
		CreatedAt:       now,
	}
	chunks := make([]kb.Chunk, len(split))
	for i := range split {
		chunks[i] = kb.Chunk{
			ID:              chunkID(doc.ID, i),
			TenantID:        req.TenantID,
			KnowledgeBaseID: req.KnowledgeBaseID,
			DocumentID:      doc.ID,
			Content:         split[i].Content,
			SourceURI:       req.SourceURI,
			Section:         split[i].Section,
			Offset:          split[i].Offset,
			Vector:          vectors[i],
			Metadata:        map[string]string{"document_title": req.Name},
		}
	}
	if err := s.Indexer.Store(ctx, doc, chunks); err != nil {
		return fail(err)
	}
	job.Status = JobStatusSucceeded
	job.DocumentID = doc.ID
	job.ChunkCount = len(chunks)
	job.UpdatedAt = time.Now().UTC()
	if s.Jobs != nil {
		if err := s.Jobs.UpdateJob(ctx, job); err != nil {
			return Result{}, err
		}
	}
	return Result{Document: doc, Chunks: chunks, Job: job}, nil
}

func contentHash(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func documentID(tenantID, kbID, hash string) string {
	sum := sha256.Sum256([]byte(tenantID + "/" + kbID + "/" + hash))
	return "doc_" + hex.EncodeToString(sum[:])[:24]
}

func chunkID(docID string, index int) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s/%d", docID, index)))
	return "chk_" + hex.EncodeToString(sum[:])[:24]
}
