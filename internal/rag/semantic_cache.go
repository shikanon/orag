package rag

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"
)

const semanticCacheKeyVersion = "v2"

type SemanticCacheStore interface {
	Lookup(ctx context.Context, req SemanticCacheLookupRequest) (QueryResponse, bool, error)
	Store(ctx context.Context, entry SemanticCacheEntry) error
}

type SemanticCacheLookupRequest struct {
	TenantID        string
	KnowledgeBaseID string
	Query           string
	Vector          []float64
	Threshold       float64
	Profile         Profile
	TopK            int
}

type SemanticCacheEntry struct {
	TenantID        string
	KnowledgeBaseID string
	Query           string
	Vector          []float64
	Profile         Profile
	TopK            int
	Response        QueryResponse
	CreatedAt       time.Time
}

type CacheEntry struct {
	Query     string
	Response  QueryResponse
	CreatedAt time.Time
}

type InMemorySemanticCache struct {
	mu      sync.RWMutex
	entries map[string]CacheEntry
	max     int
}

func NewSemanticCache(max int) *InMemorySemanticCache {
	if max <= 0 {
		max = 10000
	}
	return &InMemorySemanticCache{entries: map[string]CacheEntry{}, max: max}
}

func (c *InMemorySemanticCache) Lookup(_ context.Context, req SemanticCacheLookupRequest) (QueryResponse, bool, error) {
	resp, ok := c.Get(CacheKey(QueryRequest{
		TenantID:        req.TenantID,
		KnowledgeBaseID: req.KnowledgeBaseID,
		Query:           req.Query,
		Profile:         req.Profile,
		TopK:            req.TopK,
	}))
	return resp, ok, nil
}

func (c *InMemorySemanticCache) Store(_ context.Context, entry SemanticCacheEntry) error {
	c.Put(CacheKey(QueryRequest{
		TenantID:        entry.TenantID,
		KnowledgeBaseID: entry.KnowledgeBaseID,
		Query:           entry.Query,
		Profile:         entry.Profile,
		TopK:            entry.TopK,
	}), entry.Response)
	return nil
}

func (c *InMemorySemanticCache) Get(query string) (QueryResponse, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.entries[key(query)]
	return entry.Response, ok
}

func (c *InMemorySemanticCache) Put(query string, resp QueryResponse) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= c.max {
		for k := range c.entries {
			delete(c.entries, k)
			break
		}
	}
	c.entries[key(query)] = CacheEntry{Query: query, Response: resp, CreatedAt: time.Now().UTC()}
}

func CacheKey(req QueryRequest) string {
	profile := req.Profile
	if profile == "" {
		profile = ProfileRealtime
	}
	return strings.Join([]string{
		semanticCacheKeyVersion,
		req.TenantID,
		req.KnowledgeBaseID,
		string(profile),
		strconv.Itoa(req.TopK),
		normalizeCacheQuery(req.Query),
	}, "\x1f")
}

func key(query string) string {
	return normalizeCacheQuery(query)
}

func normalizeCacheQuery(query string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(query))), " ")
}
