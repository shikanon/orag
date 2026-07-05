package kb

import "context"

type ActivatingIndexer interface {
	Activate(ctx context.Context, doc Document, chunks []Chunk) error
}

type stagedStoreContextKey struct{}

func withStagedStore(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, stagedStoreContextKey{}, true)
}

func isStagedStore(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	staged, _ := ctx.Value(stagedStoreContextKey{}).(bool)
	return staged
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
		storeCtx := ctx
		activator, activating := indexer.(ActivatingIndexer)
		if activating {
			storeCtx = withStagedStore(ctx)
		}
		if err := indexer.Store(storeCtx, doc, chunks); err != nil {
			return err
		}
		if activating {
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

func (i CompositeIndexer) StoreGraphRelations(ctx context.Context, relations []GraphRelation) error {
	for _, indexer := range i.Indexers {
		store, ok := indexer.(GraphStore)
		if !ok || store == nil {
			continue
		}
		if err := store.StoreGraphRelations(ctx, relations); err != nil {
			return err
		}
	}
	return nil
}

func (i CompositeIndexer) ExpandGraph(ctx context.Context, req GraphExpansionRequest) ([]SearchResult, error) {
	for _, indexer := range i.Indexers {
		store, ok := indexer.(GraphStore)
		if !ok || store == nil {
			continue
		}
		return store.ExpandGraph(ctx, req)
	}
	return nil, nil
}
