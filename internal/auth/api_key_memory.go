package auth

import (
	"context"
	"sort"
	"sync"
	"time"
)

type MemoryAPIKeyRepository struct {
	mu    sync.RWMutex
	items map[string]APIKey
}

func NewMemoryAPIKeyRepository() *MemoryAPIKeyRepository {
	return &MemoryAPIKeyRepository{items: make(map[string]APIKey)}
}

func (r *MemoryAPIKeyRepository) CreateAPIKey(_ context.Context, item APIKey) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[item.ID] = item
	return nil
}

func (r *MemoryAPIKeyRepository) ListAPIKeys(_ context.Context, tenantID string) ([]APIKey, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]APIKey, 0)
	for _, item := range r.items {
		if item.TenantID == tenantID {
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return items, nil
}

func (r *MemoryAPIKeyRepository) GetAPIKeyByID(_ context.Context, id string) (APIKey, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.items[id]
	return item, ok, nil
}

func (r *MemoryAPIKeyRepository) RevokeAPIKey(_ context.Context, tenantID, id string, revokedAt time.Time) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	item, ok := r.items[id]
	if !ok || item.TenantID != tenantID {
		return false, nil
	}
	if item.RevokedAt == nil {
		item.RevokedAt = &revokedAt
		r.items[id] = item
	}
	return true, nil
}

func (r *MemoryAPIKeyRepository) TouchAPIKeyLastUsed(_ context.Context, id string, usedAt, notAfter time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	item, ok := r.items[id]
	if !ok {
		return ErrAPIKeyNotFound
	}
	if item.LastUsedAt == nil || !item.LastUsedAt.After(notAfter) {
		item.LastUsedAt = timePointerCopy(usedAt)
		r.items[id] = item
	}
	return nil
}

func timePointerCopy(value time.Time) *time.Time {
	copy := value
	return &copy
}
