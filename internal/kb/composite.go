package kb

import (
	"context"
	"errors"
)

var ErrNonTransactionalCompositeIndexer = errors.New("composite indexer requires activation participants")

type ActivationParticipant interface {
	Indexer
	PrepareActivation(ctx context.Context, doc Document, chunks []Chunk) error
	CommitActivation(ctx context.Context, doc Document, chunks []Chunk) error
	AbortActivation(ctx context.Context, doc Document, chunks []Chunk) error
	FinalizeActivation(ctx context.Context, doc Document, chunks []Chunk) error
}

type PostCommitCleanupWarning struct {
	Err error
}

func (w *PostCommitCleanupWarning) Error() string {
	return "post-commit index cleanup failed: " + w.Err.Error()
}

func (w *PostCommitCleanupWarning) Unwrap() error {
	return w.Err
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
	indexers := make([]Indexer, 0, len(i.Indexers))
	for _, indexer := range i.Indexers {
		if indexer != nil {
			indexers = append(indexers, indexer)
		}
	}
	if len(indexers) == 0 {
		return nil
	}
	if len(indexers) == 1 {
		if _, ok := indexers[0].(ActivationParticipant); !ok {
			return indexers[0].Store(ctx, doc, chunks)
		}
	}

	participants := make([]ActivationParticipant, 0, len(indexers))
	for _, indexer := range indexers {
		participant, ok := indexer.(ActivationParticipant)
		if !ok {
			return ErrNonTransactionalCompositeIndexer
		}
		participants = append(participants, participant)
	}

	stored := make([]ActivationParticipant, 0, len(participants))
	for _, participant := range participants {
		if err := participant.Store(withStagedStore(ctx), doc, chunks); err != nil {
			return abortActivation(ctx, stored, doc, chunks, err)
		}
		stored = append(stored, participant)
	}
	for _, participant := range participants {
		if err := participant.PrepareActivation(ctx, doc, chunks); err != nil {
			return abortActivation(ctx, stored, doc, chunks, err)
		}
	}
	for _, participant := range participants {
		if err := participant.CommitActivation(ctx, doc, chunks); err != nil {
			return abortActivation(ctx, stored, doc, chunks, err)
		}
	}

	var cleanupErr error
	for _, participant := range participants {
		cleanupErr = errors.Join(cleanupErr, participant.FinalizeActivation(ctx, doc, chunks))
	}
	if cleanupErr != nil {
		return &PostCommitCleanupWarning{Err: cleanupErr}
	}
	return nil
}

func abortActivation(ctx context.Context, participants []ActivationParticipant, doc Document, chunks []Chunk, cause error) error {
	err := cause
	for idx := len(participants) - 1; idx >= 0; idx-- {
		err = errors.Join(err, participants[idx].AbortActivation(ctx, doc, chunks))
	}
	return err
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
