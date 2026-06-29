package kb

type KnowledgeBaseRepository interface {
	PutKnowledgeBase(kb KnowledgeBase) error
	ListKnowledgeBases(tenantID string) []KnowledgeBase
	GetKnowledgeBase(tenantID, id string) (KnowledgeBase, bool)
}

type ChunkSource interface {
	Chunks(tenantID, kbID string) []Chunk
}

type Store interface {
	KnowledgeBaseRepository
	ChunkSource
	Indexer
}
