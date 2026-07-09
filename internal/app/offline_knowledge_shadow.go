package app

import (
	"context"

	"github.com/shikanon/orag/internal/config"
	"github.com/shikanon/orag/internal/offlineknowledge"
	"github.com/shikanon/orag/internal/rag"
)

func configureRAGShadow(svc *rag.Service, organizer config.OfflineKnowledgeOrganizerConfig, opts offlineknowledge.ServiceOptions) {
	if svc == nil {
		return
	}
	svc.Shadow = rag.ShadowOptions{
		Enabled: organizer.ShadowRetrievalEnabled,
		Inject:  organizer.ShadowInjectEnabled,
		Limit:   organizer.MaxClustersPerRun,
	}
	if opts.ShadowRetriever != nil {
		svc.ShadowRetriever = offlineKnowledgeShadowRetrieverAdapter{retriever: opts.ShadowRetriever}
	}
	if opts.SourceReader != nil {
		svc.ShadowSourceReader = offlineKnowledgeShadowSourceReaderAdapter{reader: opts.SourceReader}
	}
}

type offlineKnowledgeShadowRetrieverAdapter struct {
	retriever *offlineknowledge.ShadowRetriever
}

func (a offlineKnowledgeShadowRetrieverAdapter) RetrieveShadow(ctx context.Context, req rag.ShadowRetrieveRequest) ([]rag.ShadowMatch, error) {
	matches, err := a.retriever.Retrieve(ctx, offlineknowledge.ShadowRetrieveRequest{
		TenantID:     req.TenantID,
		KBID:         req.KBID,
		Query:        req.Query,
		TraceID:      req.TraceID,
		Limit:        req.Limit,
		Inject:       req.Inject,
		ScopedItemID: req.ScopedItemID,
	})
	if err != nil {
		return nil, err
	}
	out := make([]rag.ShadowMatch, 0, len(matches))
	for _, match := range matches {
		out = append(out, rag.ShadowMatch{
			ItemID:     match.ItemID,
			ItemType:   string(match.ItemType),
			Source:     match.Source,
			Score:      match.Score,
			Rank:       match.Rank,
			AnswerItem: adaptShadowAnswerItem(match.AnswerItem),
			Metadata:   match.Metadata,
		})
	}
	return out, nil
}

type offlineKnowledgeShadowSourceReaderAdapter struct {
	reader offlineknowledge.SourceReader
}

func (a offlineKnowledgeShadowSourceReaderAdapter) ReadShadowSourceChunk(ctx context.Context, tenantID, kbID, chunkID string) (rag.ShadowSourceChunk, bool, error) {
	source, found, err := a.reader.ReadSourceChunk(ctx, tenantID, kbID, chunkID)
	if err != nil || !found {
		return rag.ShadowSourceChunk{}, found, err
	}
	return rag.ShadowSourceChunk{
		TenantID:         source.TenantID,
		KBID:             source.KBID,
		DocID:            source.DocID,
		DocVersion:       source.DocVersion,
		ChunkID:          source.ChunkID,
		ChunkContentHash: source.ChunkContentHash,
		Text:             source.Text,
	}, true, nil
}

func adaptShadowAnswerItem(answer *offlineknowledge.ShadowAnswerItem) *rag.ShadowAnswerItem {
	if answer == nil {
		return nil
	}
	out := &rag.ShadowAnswerItem{
		SourceFingerprints: make([]rag.ShadowSourceFingerprint, 0, len(answer.SourceFingerprints)),
		Evidence:           make([]rag.ShadowEvidence, 0, len(answer.Evidence)),
		GuidanceMetadata:   answer.GuidanceMetadata,
	}
	for _, fp := range answer.SourceFingerprints {
		out.SourceFingerprints = append(out.SourceFingerprints, rag.ShadowSourceFingerprint{
			DocID:            fp.DocID,
			DocVersion:       fp.DocVersion,
			ChunkID:          fp.ChunkID,
			ChunkContentHash: fp.ChunkContentHash,
		})
	}
	for _, evidence := range answer.Evidence {
		out.Evidence = append(out.Evidence, rag.ShadowEvidence{
			ChunkID:  evidence.ChunkID,
			DocID:    evidence.DocID,
			Quote:    evidence.Quote,
			Supports: evidence.Supports,
		})
	}
	return out
}
