package offlineknowledge

import (
	"context"

	"github.com/shikanon/orag/internal/kb"
)

type StoreSourceReader struct {
	Source kb.ChunkSource
}

func NewStoreSourceReader(source kb.ChunkSource) StoreSourceReader {
	return StoreSourceReader{Source: source}
}

func (r StoreSourceReader) ReadSourceChunk(ctx context.Context, tenantID, kbID, chunkID string) (SourceChunk, bool, error) {
	if r.Source == nil {
		return SourceChunk{}, false, ErrSourceReaderUnavailable
	}
	for _, chunk := range r.Source.Chunks(tenantID, kbID) {
		if chunk.ID != chunkID {
			continue
		}
		return NewChunkSourceMetadataReader().ReadSourceMetadata(ctx, tenantID, kbID, chunk)
	}
	return SourceChunk{}, false, nil
}
