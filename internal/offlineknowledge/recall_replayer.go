package offlineknowledge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/shikanon/orag/internal/kb"
)

const defaultRecallReplayTopK = 10

var ErrSourceMetadataReaderRequired = errors.New("offline knowledge source metadata reader is required")

type SourceMetadataReader interface {
	ReadSourceMetadata(ctx context.Context, tenantID, kbID string, chunk kb.Chunk) (SourceChunk, bool, error)
}

type RetrieverRecallReplayer struct {
	Retriever      kb.Retriever
	SourceReader   SourceMetadataReader
	TopK           int
	DenseTopK      int
	SparseTopK     int
	IncludeTraceID bool
}

func NewRetrieverRecallReplayer(retriever kb.Retriever, sourceReader SourceMetadataReader, topK int) *RetrieverRecallReplayer {
	return &RetrieverRecallReplayer{
		Retriever:    retriever,
		SourceReader: sourceReader,
		TopK:         topK,
	}
}

func (r *RetrieverRecallReplayer) ReplayRecall(ctx context.Context, cluster QuestionCluster) (RecallReplayResult, error) {
	if r == nil || r.Retriever == nil {
		return RecallReplayResult{}, ErrRecallReplayerRequired
	}
	if r.SourceReader == nil {
		return RecallReplayResult{}, ErrSourceMetadataReaderRequired
	}
	topK := r.TopK
	if topK <= 0 {
		topK = defaultRecallReplayTopK
	}
	results, err := r.Retriever.Retrieve(ctx, kb.SearchRequest{
		TenantID:        cluster.TenantID,
		KnowledgeBaseID: cluster.KBID,
		Query:           cluster.CanonicalQuestion,
		TopK:            topK,
		DenseTopK:       r.DenseTopK,
		SparseTopK:      r.SparseTopK,
	})
	if err != nil {
		return RecallReplayResult{}, err
	}

	out := RecallReplayResult{
		TraceSummaries: traceSummariesForCluster(cluster),
		Metadata: map[string]any{
			"replay_mode": "baseline",
			"query":       cluster.CanonicalQuestion,
			"top_k":       topK,
		},
	}
	seenFingerprints := map[string]struct{}{}
	for _, result := range results {
		chunk := result.Chunk
		if chunk.TenantID != cluster.TenantID || chunk.KnowledgeBaseID != cluster.KBID {
			continue
		}
		source, found, err := r.SourceReader.ReadSourceMetadata(ctx, cluster.TenantID, cluster.KBID, chunk)
		if err != nil {
			return RecallReplayResult{}, err
		}
		if !found {
			continue
		}
		if source.TenantID != cluster.TenantID || source.KBID != cluster.KBID {
			continue
		}
		rank := result.Rank
		if rank <= 0 {
			rank = len(out.BaselineRecallResults) + 1
		}
		traceID := ""
		if r.IncludeTraceID && len(cluster.TraceIDs) > 0 {
			traceID = cluster.TraceIDs[0]
		}
		out.BaselineRecallResults = append(out.BaselineRecallResults, BaselineRecallItem{
			TraceID:          traceID,
			ChunkID:          source.ChunkID,
			DocID:            source.DocID,
			DocVersion:       source.DocVersion,
			ChunkContentHash: source.ChunkContentHash,
			Rank:             rank,
			Score:            result.Score,
			Matched:          true,
		})
		fingerprint := SourceFingerprint{
			DocID:            source.DocID,
			DocVersion:       source.DocVersion,
			ChunkID:          source.ChunkID,
			ChunkContentHash: source.ChunkContentHash,
		}
		key := fingerprint.DocID + "\x00" + fingerprint.DocVersion + "\x00" + fingerprint.ChunkID + "\x00" + fingerprint.ChunkContentHash
		if _, ok := seenFingerprints[key]; ok {
			continue
		}
		out.SourceFingerprints = append(out.SourceFingerprints, fingerprint)
		seenFingerprints[key] = struct{}{}
	}
	return out, nil
}

type ChunkSourceMetadataReader struct{}

func NewChunkSourceMetadataReader() ChunkSourceMetadataReader {
	return ChunkSourceMetadataReader{}
}

func (ChunkSourceMetadataReader) ReadSourceMetadata(_ context.Context, tenantID, kbID string, chunk kb.Chunk) (SourceChunk, bool, error) {
	if chunk.ID == "" || chunk.TenantID != tenantID || chunk.KnowledgeBaseID != kbID {
		return SourceChunk{}, false, nil
	}
	return SourceChunk{
		TenantID:         chunk.TenantID,
		KBID:             chunk.KnowledgeBaseID,
		DocID:            chunk.DocumentID,
		DocVersion:       sourceDocVersion(chunk),
		ChunkID:          chunk.ID,
		ChunkContentHash: sourceChunkContentHash(chunk),
		Text:             chunk.Content,
	}, true, nil
}

func traceSummariesForCluster(cluster QuestionCluster) []TraceSummary {
	out := make([]TraceSummary, 0, len(cluster.TraceIDs))
	for _, traceID := range cluster.TraceIDs {
		if traceID == "" {
			continue
		}
		out = append(out, TraceSummary{TraceID: traceID, Query: cluster.CanonicalQuestion})
	}
	return out
}

func sourceDocVersion(chunk kb.Chunk) string {
	for _, key := range []string{"doc_version", "document_version", "document_content_hash", "doc_content_hash", "content_hash"} {
		if value := strings.TrimSpace(chunk.Metadata[key]); value != "" {
			return value
		}
	}
	if chunk.SourceURI != "" {
		return "source:" + stableHash(chunk.SourceURI)
	}
	return "doc:" + stableHash(chunk.DocumentID)
}

func sourceChunkContentHash(chunk kb.Chunk) string {
	for _, key := range []string{"chunk_content_hash", "content_hash"} {
		if value := strings.TrimSpace(chunk.Metadata[key]); value != "" {
			return value
		}
	}
	return "sha256:" + stableHash(chunk.Content)
}

func stableHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
