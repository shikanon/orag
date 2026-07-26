package orag

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/shikanon/orag/internal/ingest"
	"github.com/shikanon/orag/internal/kb"
)

type IngestTextRequest struct {
	TenantID        string
	KnowledgeBaseID string
	Name            string
	SourceURI       string
	Text            string
}

type IngestFileRequest struct {
	TenantID        string
	KnowledgeBaseID string
	Name            string
	SourceURI       string
	Reader          io.Reader
}

type IngestResult struct {
	Document Document
	Job      IngestionJob
	Chunks   []Chunk
}

type Document struct {
	ID              string
	TenantID        string
	KnowledgeBaseID string
	SourceURI       string
	Title           string
	ContentHash     string
	Metadata        map[string]string
	CreatedAt       time.Time
}

type Chunk struct {
	ID              string
	TenantID        string
	KnowledgeBaseID string
	DocumentID      string
	Content         string
	ContextualText  string
	SourceURI       string
	Page            int
	Section         string
	Offset          int
	Metadata        map[string]string
}

type IngestionStatus string

const (
	IngestionRunning   IngestionStatus = "running"
	IngestionSucceeded IngestionStatus = "succeeded"
	IngestionFailed    IngestionStatus = "failed"
)

type IngestionJob struct {
	ID              string
	TenantID        string
	KnowledgeBaseID string
	Status          IngestionStatus
	SourceURI       string
	DocumentID      string
	ChunkCount      int
	Error           string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type GetIngestionJobRequest struct {
	TenantID string
	ID       string
}

func (c *Client) IngestText(ctx context.Context, req IngestTextRequest) (IngestResult, error) {
	return c.ingest(ctx, req.TenantID, req.KnowledgeBaseID, req.Name, req.SourceURI, []byte(req.Text))
}

func (c *Client) IngestFile(ctx context.Context, req IngestFileRequest) (IngestResult, error) {
	if req.Reader == nil {
		return IngestResult{}, newError(CodeInvalidArgument, "ingest_file", req.KnowledgeBaseID, "", false, errors.New("reader is required"))
	}
	if err := c.requireOpen("ingest_file"); err != nil {
		return IngestResult{}, err
	}
	limit := c.app.Config.Ingestion.MaxDocumentBytes
	reader := req.Reader
	if limit > 0 {
		reader = io.LimitReader(req.Reader, limit+1)
	}
	body, err := io.ReadAll(reader)
	if err != nil {
		return IngestResult{}, wrapError("ingest_file", req.KnowledgeBaseID, "", err)
	}
	if limit > 0 && int64(len(body)) > limit {
		return IngestResult{}, newError(CodeInvalidArgument, "ingest_file", req.KnowledgeBaseID, "", false, fmt.Errorf("document exceeds max size %d bytes", limit))
	}
	return c.ingest(ctx, req.TenantID, req.KnowledgeBaseID, req.Name, req.SourceURI, body)
}

func (c *Client) ingest(ctx context.Context, tenantID, knowledgeBaseID, name, sourceURI string, body []byte) (IngestResult, error) {
	if err := c.requireOpen("ingest"); err != nil {
		return IngestResult{}, err
	}
	if strings.TrimSpace(knowledgeBaseID) == "" || strings.TrimSpace(name) == "" || len(body) == 0 {
		return IngestResult{}, newError(CodeInvalidArgument, "ingest", knowledgeBaseID, "", false, errors.New("knowledge_base_id, name, and content are required"))
	}
	result, err := c.app.Ingest.Ingest(ctx, ingest.Request{
		TenantID:        c.tenant(tenantID),
		KnowledgeBaseID: knowledgeBaseID,
		SourceURI:       strings.TrimSpace(sourceURI),
		Name:            strings.TrimSpace(name),
		Content:         append([]byte(nil), body...),
	})
	if err != nil {
		if errors.Is(err, ingest.ErrKnowledgeBaseNotFound) {
			return IngestResult{}, newError(CodeNotFound, "ingest", knowledgeBaseID, "", false, err)
		}
		return IngestResult{}, wrapError("ingest", knowledgeBaseID, "", err)
	}
	return fromIngestResult(result), nil
}

func (c *Client) GetIngestionJob(ctx context.Context, req GetIngestionJobRequest) (IngestionJob, bool, error) {
	if err := c.requireOpen("get_ingestion_job"); err != nil {
		return IngestionJob{}, false, err
	}
	job, found, err := c.app.Ingest.Jobs.GetJob(ctx, c.tenant(req.TenantID), strings.TrimSpace(req.ID))
	if err != nil {
		return IngestionJob{}, false, wrapError("get_ingestion_job", req.ID, "", err)
	}
	if !found {
		return IngestionJob{}, false, nil
	}
	return fromIngestionJob(job), true, nil
}

func fromIngestResult(result ingest.Result) IngestResult {
	chunks := make([]Chunk, len(result.Chunks))
	for index := range result.Chunks {
		chunks[index] = fromChunk(result.Chunks[index])
	}
	return IngestResult{Document: fromDocument(result.Document), Job: fromIngestionJob(result.Job), Chunks: chunks}
}

func fromDocument(item kb.Document) Document {
	return Document{ID: item.ID, TenantID: item.TenantID, KnowledgeBaseID: item.KnowledgeBaseID, SourceURI: item.SourceURI, Title: item.Title, ContentHash: item.ContentHash, Metadata: cloneStrings(item.Metadata), CreatedAt: item.CreatedAt}
}

func fromChunk(item kb.Chunk) Chunk {
	return Chunk{ID: item.ID, TenantID: item.TenantID, KnowledgeBaseID: item.KnowledgeBaseID, DocumentID: item.DocumentID, Content: item.Content, ContextualText: item.ContextualText, SourceURI: item.SourceURI, Page: item.Page, Section: item.Section, Offset: item.Offset, Metadata: cloneStrings(item.Metadata)}
}

func fromIngestionJob(item ingest.Job) IngestionJob {
	return IngestionJob{ID: item.ID, TenantID: item.TenantID, KnowledgeBaseID: item.KnowledgeBaseID, Status: IngestionStatus(item.Status), SourceURI: item.SourceURI, DocumentID: item.DocumentID, ChunkCount: item.ChunkCount, Error: item.Error, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt}
}
