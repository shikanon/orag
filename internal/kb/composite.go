package kb

import "context"

type ActivatingIndexer interface {
	Activate(ctx context.Context, doc Document, chunks []Chunk) error
}

type CompositeIndexer struct {
	Indexers []Indexer
}

func (i CompositeIndexer) Store(ctx context.Context, doc Document, chunks []Chunk) error {
	var activators []ActivatingIndexer
	for _, indexer := range i.Indexers {
		if indexer == nil {
			continue
		}
		if err := indexer.Store(ctx, doc, chunks); err != nil {
			return err
		}
		if activator, ok := indexer.(ActivatingIndexer); ok {
			activators = append(activators, activator)
		}
	}
	for _, activator := range activators {
		if err := activator.Activate(ctx, doc, chunks); err != nil {
			return err
		}
	}
	return nil
}

func (i CompositeIndexer) DeleteDocumentSource(ctx context.Context, tenantID, kbID, sourceURI string) error {
	for _, indexer := range i.Indexers {
		deleter, ok := indexer.(DocumentSourceDeleter)
		if !ok || deleter == nil {
			continue
		}
		if err := deleter.DeleteDocumentSource(ctx, tenantID, kbID, sourceURI); err != nil {
			return err
		}
	}
	return nil
}
