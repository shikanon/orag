package kb

type KnowledgeBaseRepository interface {
	PutKnowledgeBase(kb KnowledgeBase) error
	ListKnowledgeBases(tenantID string) ([]KnowledgeBase, error)
	GetKnowledgeBase(tenantID, id string) (KnowledgeBase, bool, error)
}

type ChunkSource interface {
	Chunks(tenantID, kbID string) []Chunk
}

type Store interface {
	KnowledgeBaseRepository
	ChunkSource
	Indexer
}
