package kb

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

var ErrActivationCandidateMissing = errors.New("activation candidate is missing")

type KnowledgeBase struct {
	ID          string            `json:"id"`
	TenantID    string            `json:"tenant_id"`
	ProjectID   string            `json:"project_id,omitempty"`
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
	IngestionJobID  string            `json:"-"`
	Metadata        map[string]string `json:"metadata,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
}

type Chunk struct {
	ID              string            `json:"id"`
	TenantID        string            `json:"tenant_id"`
	KnowledgeBaseID string            `json:"knowledge_base_id"`
	DocumentID      string            `json:"document_id"`
	Content         string            `json:"content"`
	ContextualText  string            `json:"contextual_text,omitempty"`
	SourceURI       string            `json:"source_uri"`
	Page            int               `json:"page,omitempty"`
	Section         string            `json:"section,omitempty"`
	Offset          int               `json:"offset,omitempty"`
	Vector          []float64         `json:"-"`
	IngestionJobID  string            `json:"-"`
	Metadata        map[string]string `json:"metadata,omitempty"`
}

func (c Chunk) SearchText() string {
	contextual := strings.TrimSpace(c.ContextualText)
	content := strings.TrimSpace(c.Content)
	if contextual == "" {
		return content
	}
	if content == "" {
		return contextual
	}
	return contextual + "\n\n" + content
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

type GraphRelation struct {
	TenantID        string
	KnowledgeBaseID string
	DocumentID      string
	SourceChunkID   string
	TargetChunkID   string
	Subject         string
	Predicate       string
	Object          string
	Weight          float64
}

type GraphExpansionRequest struct {
	TenantID        string
	KnowledgeBaseID string
	Entities        []string
	Limit           int
}

type GraphStore interface {
	StoreGraphRelations(ctx context.Context, relations []GraphRelation) error
	ExpandGraph(ctx context.Context, req GraphExpansionRequest) ([]SearchResult, error)
}

type Retriever interface {
	Retrieve(ctx context.Context, req SearchRequest) ([]SearchResult, error)
}

type Indexer interface {
	Store(ctx context.Context, doc Document, chunks []Chunk) error
}

type SearchableChunkFilter interface {
	FilterSearchableChunkIDs(
		ctx context.Context,
		tenantID string,
		knowledgeBaseID string,
		chunkIDs []string,
	) (map[string]struct{}, error)
}

type DocumentSourceDeleter interface {
	DeleteDocumentSource(ctx context.Context, tenantID, kbID, sourceURI string) error
}

type MemoryStore struct {
	mu               sync.RWMutex
	kbs              map[string]KnowledgeBase
	documents        map[string]Document
	chunks           map[string]Chunk
	pendingDocuments map[string]Document
	pendingChunks    map[string]Chunk
	relations        []GraphRelation
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		kbs:              map[string]KnowledgeBase{},
		documents:        map[string]Document{},
		chunks:           map[string]Chunk{},
		pendingDocuments: map[string]Document{},
		pendingChunks:    map[string]Chunk{},
		relations:        nil,
	}
}

func (s *MemoryStore) PutKnowledgeBase(_ context.Context, kb KnowledgeBase) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.kbs[kb.ID] = kb
	return nil
}

func (s *MemoryStore) ListKnowledgeBases(_ context.Context, tenantID string) ([]KnowledgeBase, error) {
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

func (s *MemoryStore) GetKnowledgeBase(_ context.Context, tenantID, id string) (KnowledgeBase, bool, error) {
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
	for docID, doc := range s.pendingDocuments {
		if doc.TenantID == tenantID && doc.KnowledgeBaseID == id {
			delete(s.pendingDocuments, docID)
		}
	}
	for chunkID, chunk := range s.pendingChunks {
		if chunk.TenantID == tenantID && chunk.KnowledgeBaseID == id {
			delete(s.pendingChunks, chunkID)
		}
	}
	s.deleteGraphRelationsLocked(tenantID, id, "")
	return true, nil
}

func (s *MemoryStore) Store(ctx context.Context, doc Document, chunks []Chunk) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if isStagedStore(ctx) {
		s.stageDocumentLocked(doc, chunks)
		return nil
	}
	s.deleteDocumentSourceLocked(doc.TenantID, doc.KnowledgeBaseID, doc.SourceURI)
	s.deletePendingDocumentSourceLocked(doc.TenantID, doc.KnowledgeBaseID, doc.SourceURI)
	s.documents[doc.ID] = doc
	s.deleteGraphRelationsLocked(doc.TenantID, doc.KnowledgeBaseID, doc.ID)
	for _, chunk := range chunks {
		s.chunks[chunk.ID] = chunkWithDocumentVersion(chunk, doc)
	}
	return nil
}

func (s *MemoryStore) PrepareActivation(context.Context, Document, []Chunk) error {
	return nil
}

func (s *MemoryStore) CommitActivation(_ context.Context, doc Document, chunks []Chunk) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	stagedDoc, ok := s.pendingDocuments[doc.ID]
	if !ok {
		return nil
	}
	s.deleteDocumentSourceLocked(doc.TenantID, doc.KnowledgeBaseID, doc.SourceURI)
	s.documents[stagedDoc.ID] = stagedDoc
	for _, chunk := range chunks {
		if staged, ok := s.pendingChunks[chunk.ID]; ok {
			s.chunks[staged.ID] = staged
			delete(s.pendingChunks, chunk.ID)
		}
	}
	delete(s.pendingDocuments, doc.ID)
	return nil
}

func (s *MemoryStore) AbortActivation(_ context.Context, doc Document, _ []Chunk) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.pendingDocuments, doc.ID)
	for chunkID, chunk := range s.pendingChunks {
		if chunk.DocumentID == doc.ID {
			delete(s.pendingChunks, chunkID)
		}
	}
	return nil
}

func (s *MemoryStore) FinalizeActivation(context.Context, Document, []Chunk) error {
	return nil
}

func (s *MemoryStore) DeleteDocumentSource(_ context.Context, tenantID, kbID, sourceURI string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleteDocumentSourceLocked(tenantID, kbID, sourceURI)
	s.deletePendingDocumentSourceLocked(tenantID, kbID, sourceURI)
	return nil
}

func (s *MemoryStore) stageDocumentLocked(doc Document, chunks []Chunk) {
	s.deletePendingDocumentSourceLocked(doc.TenantID, doc.KnowledgeBaseID, doc.SourceURI)
	s.pendingDocuments[doc.ID] = doc
	for _, chunk := range chunks {
		s.pendingChunks[chunk.ID] = chunkWithDocumentVersion(chunk, doc)
	}
}

func (s *MemoryStore) deleteDocumentSourceLocked(tenantID, kbID, sourceURI string) {
	if sourceURI == "" {
		return
	}
	var deletedDocIDs []string
	for docID, doc := range s.documents {
		if doc.TenantID != tenantID || doc.KnowledgeBaseID != kbID || doc.SourceURI != sourceURI {
			continue
		}
		delete(s.documents, docID)
		deletedDocIDs = append(deletedDocIDs, docID)
		for chunkID, chunk := range s.chunks {
			if chunk.TenantID == tenantID && chunk.KnowledgeBaseID == kbID && chunk.DocumentID == docID {
				delete(s.chunks, chunkID)
			}
		}
	}
	for _, docID := range deletedDocIDs {
		s.deleteGraphRelationsLocked(tenantID, kbID, docID)
	}
}

func (s *MemoryStore) deletePendingDocumentSourceLocked(tenantID, kbID, sourceURI string) {
	if sourceURI == "" {
		return
	}
	for docID, doc := range s.pendingDocuments {
		if doc.TenantID != tenantID || doc.KnowledgeBaseID != kbID || doc.SourceURI != sourceURI {
			continue
		}
		delete(s.pendingDocuments, docID)
		for chunkID, chunk := range s.pendingChunks {
			if chunk.TenantID == tenantID && chunk.KnowledgeBaseID == kbID && chunk.DocumentID == docID {
				delete(s.pendingChunks, chunkID)
			}
		}
	}
}

func (s *MemoryStore) StoreGraphRelations(_ context.Context, relations []GraphRelation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, relation := range relations {
		if relation.Weight == 0 {
			relation.Weight = 1
		}
		s.relations = append(s.relations, relation)
	}
	return nil
}

func (s *MemoryStore) ExpandGraph(_ context.Context, req GraphExpansionRequest) ([]SearchResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entities := normalizedEntitySet(req.Entities)
	if len(entities) == 0 {
		return nil, nil
	}
	seen := map[string]SearchResult{}
	for _, relation := range s.relations {
		if relation.TenantID != req.TenantID || relation.KnowledgeBaseID != req.KnowledgeBaseID {
			continue
		}
		if !entities[NormalizeQuery(relation.Subject)] && !entities[NormalizeQuery(relation.Object)] {
			continue
		}
		for _, chunkID := range []string{relation.SourceChunkID, relation.TargetChunkID} {
			chunk, ok := s.chunks[chunkID]
			if !ok {
				continue
			}
			if _, exists := seen[chunk.ID]; exists {
				continue
			}
			seen[chunk.ID] = SearchResult{Chunk: chunk, Score: relation.Weight, From: "graph"}
		}
	}
	results := make([]SearchResult, 0, len(seen))
	for _, result := range seen {
		results = append(results, result)
	}
	return top(results, req.Limit), nil
}

func (s *MemoryStore) deleteGraphRelationsLocked(tenantID, kbID, documentID string) {
	filtered := s.relations[:0]
	for _, relation := range s.relations {
		match := relation.TenantID == tenantID && relation.KnowledgeBaseID == kbID
		if documentID != "" {
			match = match && relation.DocumentID == documentID
		}
		if !match {
			filtered = append(filtered, relation)
		}
	}
	s.relations = filtered
}

func normalizedEntitySet(values []string) map[string]bool {
	out := map[string]bool{}
	for _, value := range values {
		if normalized := NormalizeQuery(value); normalized != "" {
			out[normalized] = true
		}
	}
	return out
}

func (s *MemoryStore) Chunks(tenantID, kbID string) []Chunk {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Chunk, 0, len(s.chunks))
	for _, chunk := range s.chunks {
		if chunk.TenantID == tenantID && chunk.KnowledgeBaseID == kbID {
			if doc, ok := s.documents[chunk.DocumentID]; ok {
				chunk = chunkWithDocumentVersion(chunk, doc)
			}
			out = append(out, chunk)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func chunkWithDocumentVersion(chunk Chunk, doc Document) Chunk {
	if doc.ContentHash == "" {
		return chunk
	}
	if chunk.Metadata == nil {
		chunk.Metadata = map[string]string{}
	} else {
		copied := make(map[string]string, len(chunk.Metadata)+1)
		for key, value := range chunk.Metadata {
			copied[key] = value
		}
		chunk.Metadata = copied
	}
	if chunk.Metadata["doc_version"] == "" {
		chunk.Metadata["doc_version"] = doc.ContentHash
	}
	return chunk
}

func NormalizeQuery(q string) string {
	return strings.Join(strings.Fields(strings.ToLower(q)), " ")
}
