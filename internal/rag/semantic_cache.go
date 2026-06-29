package rag

import (
	"context"
	"strings"
	"sync"
	"time"
)

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
}

type SemanticCacheEntry struct {
	TenantID        string
	KnowledgeBaseID string
	Query           string
	Vector          []float64
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
	resp, ok := c.Get(cacheKey(req.TenantID, req.KnowledgeBaseID, req.Query))
	return resp, ok, nil
}

func (c *InMemorySemanticCache) Store(_ context.Context, entry SemanticCacheEntry) error {
	c.Put(cacheKey(entry.TenantID, entry.KnowledgeBaseID, entry.Query), entry.Response)
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

func cacheKey(tenantID, knowledgeBaseID, query string) string {
	return tenantID + "/" + knowledgeBaseID + "/" + query
}

func key(query string) string {
	return strings.Join(strings.Fields(strings.ToLower(query)), " ")
}
