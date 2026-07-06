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
	Namespace       string
	TenantID        string
	KnowledgeBaseID string
	Query           string
	Vector          []float64
	Threshold       float64
	Profile         Profile
	TopK            int
}

type SemanticCacheEntry struct {
	Namespace       string
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
	resp, ok := c.Get(namespacedCacheKey(req.Namespace, req.TenantID, req.KnowledgeBaseID, req.Profile, req.TopK, req.Query))
	return resp, ok, nil
}

func (c *InMemorySemanticCache) Store(_ context.Context, entry SemanticCacheEntry) error {
	profile := entry.Profile
	if profile == "" {
		profile = entry.Response.Profile
	}
	resp := entry.Response
	if resp.Profile == "" {
		resp.Profile = profile
	}
	c.Put(namespacedCacheKey(entry.Namespace, entry.TenantID, entry.KnowledgeBaseID, profile, entry.TopK, entry.Query), resp)
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
	return cacheKey(req.TenantID, req.KnowledgeBaseID, req.Profile, req.TopK, req.Query)
}

func NamespacedCacheKey(namespace string, req QueryRequest) string {
	return namespacedCacheKey(namespace, req.TenantID, req.KnowledgeBaseID, req.Profile, req.TopK, req.Query)
}

func cacheKey(tenantID, knowledgeBaseID string, profile Profile, topK int, query string) string {
	return namespacedCacheKey("", tenantID, knowledgeBaseID, profile, topK, query)
}

func namespacedCacheKey(namespace, tenantID, knowledgeBaseID string, profile Profile, topK int, query string) string {
	if profile == "" {
		profile = ProfileRealtime
	}
	parts := []string{
		semanticCacheKeyVersion,
	}
	if namespace = strings.TrimSpace(namespace); namespace != "" {
		parts = append(parts, "namespace", namespace)
	}
	parts = append(parts,
		tenantID,
		knowledgeBaseID,
		string(profile),
		strconv.Itoa(topK),
		normalizeCacheQuery(query),
	)
	return strings.Join(parts, "\x1f")
}

func key(query string) string {
	return normalizeCacheQuery(query)
}

func normalizeCacheQuery(query string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(query))), " ")
}
