package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/shikanon/orag/internal/ingest/chunker"
	"github.com/shikanon/orag/internal/ingest/parser"
	"github.com/shikanon/orag/internal/kb"
	"github.com/shikanon/orag/internal/platform/id"
)

type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float64, error)
}

type Contextualizer interface {
	Contextualize(ctx context.Context, req ContextualizationRequest) ([]string, []string, error)
}

type ContextualizationRequest struct {
	DocumentName string
	DocumentText string
	Chunks       []chunker.Chunk
}

type RAPTORBuilder interface {
	Build(ctx context.Context, req RAPTORRequest) ([]kb.Chunk, []string, error)
}

type GraphBuilder interface {
	Build(ctx context.Context, req GraphBuildRequest) ([]kb.GraphRelation, []string, error)
}

var ErrKnowledgeBaseNotFound = errors.New("knowledge base not found")

type Service struct {
	Parser           parser.Parser
	Splitter         chunker.Recursive
	Embedder         Embedder
	Contextualizer   Contextualizer
	RAPTORBuilder    RAPTORBuilder
	GraphBuilder     GraphBuilder
	KnowledgeBases   kb.KnowledgeBaseRepository
	Indexer          kb.Indexer
	Jobs             JobStore
	Uploads          UploadStore
	MaxDocumentBytes int64
	sourceLocks      sync.Map
}

// NewVariantService creates an app-lifetime ingestion service for a fixed,
// server-owned parser variant. It shares immutable runtime dependencies with
// the primary service but intentionally starts its own source-lock map instead
// of copying a live sync.Map.
func NewVariantService(base *Service, documentParser parser.Parser, splitter chunker.Recursive) *Service {
	if base == nil {
		return nil
	}
	return &Service{
		Parser:           documentParser,
		Splitter:         splitter,
		Embedder:         base.Embedder,
		Contextualizer:   base.Contextualizer,
		RAPTORBuilder:    base.RAPTORBuilder,
		GraphBuilder:     base.GraphBuilder,
		KnowledgeBases:   base.KnowledgeBases,
		Indexer:          base.Indexer,
		Jobs:             base.Jobs,
		Uploads:          base.Uploads,
		MaxDocumentBytes: base.MaxDocumentBytes,
	}
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
		if _, ok, err := s.KnowledgeBases.GetKnowledgeBase(ctx, req.TenantID, req.KnowledgeBaseID); err != nil {
			return Result{}, err
		} else if !ok {
			return Result{}, fmt.Errorf("%w: %s/%s", ErrKnowledgeBaseNotFound, req.TenantID, req.KnowledgeBaseID)
		}
	}
	lockKey := req.TenantID + "\x00" + req.KnowledgeBaseID + "\x00" + req.SourceURI
	lockValue, _ := s.sourceLocks.LoadOrStore(lockKey, &sync.Mutex{})
	sourceLock := lockValue.(*sync.Mutex)
	sourceLock.Lock()
	defer sourceLock.Unlock()

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
	hash := contentHash(req.Content)
	doc := kb.Document{
		ID:              documentID(req.TenantID, req.KnowledgeBaseID, hash),
		TenantID:        req.TenantID,
		KnowledgeBaseID: req.KnowledgeBaseID,
		SourceURI:       req.SourceURI,
		Title:           req.Name,
		ContentHash:     hash,
		IngestionJobID:  job.ID,
		Metadata:        parsed.Metadata,
		CreatedAt:       now,
	}
	contextualTexts, contextualWarnings, err := s.contextualize(ctx, req.Name, parsed.Markdown, split)
	if err != nil {
		return fail(err)
	}
	chunks := make([]kb.Chunk, len(split))
	for i := range split {
		chunks[i] = kb.Chunk{
			ID:              chunkID(doc.ID, i),
			TenantID:        req.TenantID,
			KnowledgeBaseID: req.KnowledgeBaseID,
			DocumentID:      doc.ID,
			Content:         split[i].Content,
			ContextualText:  contextualTextAt(contextualTexts, i),
			SourceURI:       req.SourceURI,
			Section:         split[i].Section,
			Offset:          split[i].Offset,
			Metadata:        map[string]string{"document_title": req.Name},
			IngestionJobID:  job.ID,
		}
	}
	raptorSummaries, raptorWarnings, err := s.buildRAPTOR(ctx, doc, chunks)
	if err != nil {
		return fail(err)
	}
	chunks = append(chunks, raptorSummaries...)
	for i := range chunks {
		chunks[i].IngestionJobID = job.ID
	}
	texts := make([]string, len(chunks))
	for i := range chunks {
		texts[i] = chunks[i].SearchText()
	}
	vectors, err := s.Embedder.Embed(ctx, texts)
	if err != nil {
		return fail(err)
	}
	if len(vectors) != len(chunks) {
		return fail(fmt.Errorf("embedding count %d does not match chunks %d", len(vectors), len(chunks)))
	}
	for i := range chunks {
		chunks[i].Vector = vectors[i]
	}
	var indexWarnings []string
	if err := s.Indexer.Store(ctx, doc, chunks); err != nil {
		var cleanupWarning *kb.PostCommitCleanupWarning
		if !errors.As(err, &cleanupWarning) {
			return fail(err)
		}
		indexWarnings = append(indexWarnings, cleanupWarning.Error())
	}
	graphWarnings, err := s.storeGraphRelations(ctx, doc, chunks)
	if err != nil {
		graphWarnings = append(graphWarnings, fmt.Sprintf("graph indexing failed: %v", err))
	}
	job.Status = JobStatusSucceeded
	job.DocumentID = doc.ID
	job.ChunkCount = len(chunks)
	warnings := append(append(append(contextualWarnings, raptorWarnings...), indexWarnings...), graphWarnings...)
	if len(warnings) > 0 && job.Error == "" {
		job.Error = strings.Join(warnings, "; ")
	}
	job.UpdatedAt = time.Now().UTC()
	if s.Jobs != nil {
		if err := s.Jobs.UpdateJob(ctx, job); err != nil {
			return Result{}, err
		}
	}
	return Result{Document: doc, Chunks: chunks, Job: job}, nil
}

func (s *Service) storeGraphRelations(ctx context.Context, doc kb.Document, chunks []kb.Chunk) ([]string, error) {
	if s.GraphBuilder == nil {
		return nil, nil
	}
	store, ok := s.Indexer.(kb.GraphStore)
	if !ok {
		return []string{"graph indexing skipped: indexer does not support graph storage"}, nil
	}
	relations, warnings, err := s.GraphBuilder.Build(ctx, GraphBuildRequest{Document: doc, Chunks: chunks})
	if err != nil {
		return warnings, err
	}
	if len(relations) == 0 {
		return warnings, nil
	}
	if err := store.StoreGraphRelations(ctx, relations); err != nil {
		return append(warnings, fmt.Sprintf("graph indexing failed: %v", err)), nil
	}
	return warnings, nil
}

func (s *Service) buildRAPTOR(ctx context.Context, doc kb.Document, chunks []kb.Chunk) ([]kb.Chunk, []string, error) {
	if s.RAPTORBuilder == nil || len(chunks) == 0 {
		return nil, nil, nil
	}
	summaries, warnings, err := s.RAPTORBuilder.Build(ctx, RAPTORRequest{Document: doc, Chunks: chunks})
	if err != nil {
		return nil, warnings, err
	}
	for i := range summaries {
		if summaries[i].TenantID == "" {
			summaries[i].TenantID = doc.TenantID
		}
		if summaries[i].KnowledgeBaseID == "" {
			summaries[i].KnowledgeBaseID = doc.KnowledgeBaseID
		}
		if summaries[i].DocumentID == "" {
			summaries[i].DocumentID = doc.ID
		}
		if summaries[i].SourceURI == "" {
			summaries[i].SourceURI = doc.SourceURI
		}
		if summaries[i].Metadata == nil {
			summaries[i].Metadata = map[string]string{}
		}
		summaries[i].Metadata["document_title"] = doc.Title
		if summaries[i].Metadata["kind"] == "" {
			summaries[i].Metadata["kind"] = "raptor_summary"
		}
	}
	return summaries, warnings, nil
}

func (s *Service) contextualize(ctx context.Context, name string, documentText string, chunks []chunker.Chunk) ([]string, []string, error) {
	if s.Contextualizer == nil || len(chunks) == 0 {
		return nil, nil, nil
	}
	contexts, warnings, err := s.Contextualizer.Contextualize(ctx, ContextualizationRequest{
		DocumentName: name,
		DocumentText: documentText,
		Chunks:       chunks,
	})
	if err != nil {
		return nil, warnings, err
	}
	if len(contexts) != len(chunks) {
		return nil, warnings, fmt.Errorf("contextualization count %d does not match chunks %d", len(contexts), len(chunks))
	}
	return contexts, warnings, nil
}

func contextualTextAt(values []string, i int) string {
	if i < 0 || i >= len(values) {
		return ""
	}
	return values[i]
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
