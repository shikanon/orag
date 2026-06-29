package kb

import "context"

type CompositeIndexer struct {
	Indexers []Indexer
}

func (i CompositeIndexer) Store(ctx context.Context, doc Document, chunks []Chunk) error {
	for _, indexer := range i.Indexers {
		if indexer == nil {
			continue
		}
		if err := indexer.Store(ctx, doc, chunks); err != nil {
			return err
		}
	}
	return nil
}
