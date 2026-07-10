package rag

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/shikanon/orag/internal/kb"
)

type ShadowRetriever interface {
	RetrieveShadow(ctx context.Context, req ShadowRetrieveRequest) ([]ShadowMatch, error)
}

type ShadowSourceReader interface {
	ReadShadowSourceChunk(ctx context.Context, tenantID, kbID, chunkID string) (ShadowSourceChunk, bool, error)
}

type ShadowRetrieveRequest struct {
	TenantID     string
	KBID         string
	Query        string
	TraceID      string
	Limit        int
	Inject       bool
	ScopedItemID string
}

type ShadowMatch struct {
	ItemID     string
	ItemType   string
	Source     string
	Score      float64
	Rank       int
	AnswerItem *ShadowAnswerItem
	Metadata   map[string]any
}

type ShadowAnswerItem struct {
	SourceFingerprints []ShadowSourceFingerprint
	Evidence           []ShadowEvidence
	GuidanceMetadata   map[string]any
}

type ShadowSourceFingerprint struct {
	DocID            string
	DocVersion       string
	ChunkID          string
	ChunkContentHash string
}

type ShadowEvidence struct {
	ChunkID  string
	DocID    string
	Quote    string
	Supports string
}

type ShadowSourceChunk struct {
	TenantID         string
	KBID             string
	DocID            string
	DocVersion       string
	ChunkID          string
	ChunkContentHash string
	Text             string
}

type ShadowOptions struct {
	Enabled bool
	Inject  bool
	Limit   int
}

var (
	ErrScopedShadowRetrievalFailed  = errors.New("scoped offline knowledge shadow retrieval failed")
	ErrScopedShadowSourceMissing    = errors.New("scoped offline knowledge shadow source missing")
	ErrScopedShadowSourceMismatch   = errors.New("scoped offline knowledge shadow source mismatch")
	ErrScopedShadowNoInjectableItem = errors.New("scoped offline knowledge shadow item has no injectable source")
)

func (s *Service) ApplyShadowRetrieval(ctx context.Context, req QueryRequest, results []kb.SearchResult) []kb.SearchResult {
	out, _, _ := s.ApplyShadowRetrievalWithDiagnostics(ctx, req, results)
	return out
}

func (s *Service) ApplyShadowRetrievalWithDiagnostics(ctx context.Context, req QueryRequest, results []kb.SearchResult) ([]kb.SearchResult, []string, error) {
	if s == nil || !s.Shadow.Enabled || s.ShadowRetriever == nil {
		return results, nil, nil
	}
	matches, err := s.ShadowRetriever.RetrieveShadow(ctx, ShadowRetrieveRequest{
		TenantID:     req.TenantID,
		KBID:         req.KnowledgeBaseID,
		Query:        req.Query,
		TraceID:      req.TraceID,
		Limit:        s.Shadow.Limit,
		Inject:       s.Shadow.Inject,
		ScopedItemID: req.ScopedShadowItemID,
	})
	if err != nil || !s.Shadow.Inject || len(matches) == 0 || s.ShadowSourceReader == nil {
		if req.ScopedShadowItemID != "" && err != nil {
			return results, []string{"offline_knowledge_shadow_scoped_retrieval_failed"}, fmt.Errorf("%w: %v", ErrScopedShadowRetrievalFailed, err)
		}
		return results, nil, nil
	}
	injected, warnings, err := s.shadowInjectedResults(ctx, req, matches, results)
	if err != nil {
		return results, warnings, err
	}
	if len(injected) == 0 {
		if req.ScopedShadowItemID != "" {
			return results, append(warnings, "offline_knowledge_shadow_scoped_no_injectable_source"), ErrScopedShadowNoInjectableItem
		}
		return results, warnings, nil
	}
	out := make([]kb.SearchResult, 0, len(injected)+len(results))
	out = append(out, injected...)
	out = append(out, results...)
	return out, warnings, nil
}

func (s *Service) shadowInjectedResults(ctx context.Context, req QueryRequest, matches []ShadowMatch, existing []kb.SearchResult) ([]kb.SearchResult, []string, error) {
	seen := make(map[string]struct{}, len(existing))
	for _, result := range existing {
		if result.Chunk.ID != "" {
			seen[result.Chunk.ID] = struct{}{}
		}
	}
	var out []kb.SearchResult
	var warnings []string
	scoped := req.ScopedShadowItemID != ""
	for _, match := range matches {
		if match.AnswerItem == nil {
			continue
		}
		for _, chunkID := range shadowChunkIDs(match.AnswerItem) {
			if _, ok := seen[chunkID]; ok {
				continue
			}
			source, found, err := s.ShadowSourceReader.ReadShadowSourceChunk(ctx, req.TenantID, req.KnowledgeBaseID, chunkID)
			if err != nil || !found || strings.TrimSpace(source.Text) == "" {
				if scoped {
					warnings = append(warnings, "offline_knowledge_shadow_scoped_source_missing")
					if err != nil {
						return out, warnings, fmt.Errorf("%w: chunk_id=%s: %v", ErrScopedShadowSourceMissing, chunkID, err)
					}
					return out, warnings, fmt.Errorf("%w: chunk_id=%s", ErrScopedShadowSourceMissing, chunkID)
				}
				continue
			}
			if scoped && !shadowSourceMatchesFingerprint(source, match.AnswerItem.SourceFingerprints) {
				warnings = append(warnings, "offline_knowledge_shadow_scoped_source_mismatch")
				return out, warnings, fmt.Errorf("%w: chunk_id=%s", ErrScopedShadowSourceMismatch, chunkID)
			}
			seen[chunkID] = struct{}{}
			out = append(out, shadowSourceResult(source, match))
		}
	}
	return out, warnings, nil
}

func shadowChunkIDs(answer *ShadowAnswerItem) []string {
	seen := map[string]struct{}{}
	var ids []string
	for _, evidence := range answer.Evidence {
		id := strings.TrimSpace(evidence.ChunkID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	for _, fp := range answer.SourceFingerprints {
		id := strings.TrimSpace(fp.ChunkID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}

func shadowSourceResult(source ShadowSourceChunk, match ShadowMatch) kb.SearchResult {
	metadata := map[string]string{
		"shadow_item_id": match.ItemID,
		"shadow_source":  match.Source,
		"doc_version":    source.DocVersion,
	}
	if source.ChunkContentHash != "" {
		metadata["chunk_content_hash"] = source.ChunkContentHash
	}
	if guidance := safeShadowGuidance(match); guidance != "" {
		metadata["shadow_guidance_metadata"] = guidance
	}
	return kb.SearchResult{
		Chunk: kb.Chunk{
			ID:              source.ChunkID,
			TenantID:        source.TenantID,
			KnowledgeBaseID: source.KBID,
			DocumentID:      source.DocID,
			Content:         shadowContextText(source.Text, match),
			Metadata:        metadata,
		},
		Score: match.Score,
		Rank:  match.Rank,
		From:  "offline_knowledge_shadow",
	}
}

func shadowSourceMatchesFingerprint(source ShadowSourceChunk, fingerprints []ShadowSourceFingerprint) bool {
	for _, fp := range fingerprints {
		if strings.TrimSpace(fp.ChunkID) != source.ChunkID {
			continue
		}
		if fp.DocID != "" && source.DocID != "" && fp.DocID != source.DocID {
			return false
		}
		if fp.DocVersion != "" && source.DocVersion != "" && fp.DocVersion != source.DocVersion {
			return false
		}
		if fp.ChunkContentHash != "" && source.ChunkContentHash != "" && fp.ChunkContentHash != source.ChunkContentHash {
			return false
		}
		return true
	}
	return len(fingerprints) == 0
}

func shadowContextText(text string, match ShadowMatch) string {
	guidance := safeShadowGuidance(match)
	if guidance == "" {
		return text
	}
	return fmt.Sprintf("Offline knowledge guidance metadata: %s\n%s", guidance, text)
}

func safeShadowGuidance(match ShadowMatch) string {
	guidance := match.Metadata
	if match.AnswerItem != nil && len(match.AnswerItem.GuidanceMetadata) > 0 {
		guidance = match.AnswerItem.GuidanceMetadata
	}
	if len(guidance) == 0 {
		return ""
	}
	clean := make(map[string]any, len(guidance)+3)
	for key, value := range guidance {
		if strings.EqualFold(key, "final_answer") {
			continue
		}
		clean[key] = value
	}
	if match.ItemID != "" {
		clean["item_id"] = match.ItemID
	}
	if match.ItemType != "" {
		clean["item_type"] = match.ItemType
	}
	if match.Source != "" {
		clean["source"] = match.Source
	}
	if len(clean) == 0 {
		return ""
	}
	payload, err := json.Marshal(clean)
	if err != nil {
		return ""
	}
	return string(payload)
}
