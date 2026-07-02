package kb

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"
)

type KnowledgeBase struct {
	ID          string            `json:"id"`
	TenantID    string            `json:"tenant_id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

type Document struct {
	ID              string            `json:"id"`
	TenantID        string            `json:"tenant_id"`
	KnowledgeBaseID string            `json:"knowledge_base_id"`
	SourceURI       string            `json:"source_uri"`
	Title           string            `json:"title"`
	ContentHash     string            `json:"content_hash"`
	Metadata        map[string]string `json:"metadata,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
}

type Chunk struct {
	ID              string            `json:"id"`
	TenantID        string            `json:"tenant_id"`
	KnowledgeBaseID string            `json:"knowledge_base_id"`
	DocumentID      string            `json:"document_id"`
	Content         string            `json:"content"`
	SourceURI       string            `json:"source_uri"`
	Page            int               `json:"page,omitempty"`
	Section         string            `json:"section,omitempty"`
	Offset          int               `json:"offset,omitempty"`
	Vector          []float64         `json:"-"`
	Metadata        map[string]string `json:"metadata,omitempty"`
}

type SearchRequest struct {
	TenantID        string
	KnowledgeBaseID string
	Query           string
	Vector          []float64
	TopK            int
	DenseTopK       int
	SparseTopK      int
}

type SearchResult struct {
	Chunk Chunk   `json:"chunk"`
	Score float64 `json:"score"`
	Rank  int     `json:"rank"`
	From  string  `json:"from"`
}

type Retriever interface {
	Retrieve(ctx context.Context, req SearchRequest) ([]SearchResult, error)
}

type Indexer interface {
	Store(ctx context.Context, doc Document, chunks []Chunk) error
}

type MemoryStore struct {
	mu        sync.RWMutex
	kbs       map[string]KnowledgeBase
	documents map[string]Document
	chunks    map[string]Chunk
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		kbs:       map[string]KnowledgeBase{},
		documents: map[string]Document{},
		chunks:    map[string]Chunk{},
	}
}

func (s *MemoryStore) PutKnowledgeBase(kb KnowledgeBase) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.kbs[kb.ID] = kb
	return nil
}

func (s *MemoryStore) ListKnowledgeBases(tenantID string) ([]KnowledgeBase, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]KnowledgeBase, 0, len(s.kbs))
	for _, item := range s.kbs {
		if item.TenantID == tenantID {
			out = append(out, item)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (s *MemoryStore) GetKnowledgeBase(tenantID, id string) (KnowledgeBase, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.kbs[id]
	return item, ok && item.TenantID == tenantID, nil
}

func (s *MemoryStore) DeleteKnowledgeBase(_ context.Context, tenantID, id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.kbs[id]
	if !ok || item.TenantID != tenantID {
		return false, nil
	}
	delete(s.kbs, id)
	for docID, doc := range s.documents {
		if doc.TenantID == tenantID && doc.KnowledgeBaseID == id {
			delete(s.documents, docID)
		}
	}
	for chunkID, chunk := range s.chunks {
		if chunk.TenantID == tenantID && chunk.KnowledgeBaseID == id {
			delete(s.chunks, chunkID)
		}
	}
	return true, nil
}

func (s *MemoryStore) Store(_ context.Context, doc Document, chunks []Chunk) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.documents[doc.ID] = doc
	for _, chunk := range chunks {
		s.chunks[chunk.ID] = chunk
	}
	return nil
}

func (s *MemoryStore) Chunks(tenantID, kbID string) []Chunk {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Chunk, 0, len(s.chunks))
	for _, chunk := range s.chunks {
		if chunk.TenantID == tenantID && chunk.KnowledgeBaseID == kbID {
			out = append(out, chunk)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func NormalizeQuery(q string) string {
	return strings.Join(strings.Fields(strings.ToLower(q)), " ")
}
