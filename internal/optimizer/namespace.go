package optimizer

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type CleanupStatus string

const (
	CleanupNotRequired CleanupStatus = "not_required"
	CleanupPending     CleanupStatus = "pending"
	CleanupDone        CleanupStatus = "done"
	CleanupFailed      CleanupStatus = "failed"
)

type TempNamespace struct {
	Name        string        `json:"name"`
	OwnerID     string        `json:"owner_id"`
	Kind        string        `json:"kind"`
	ExpiresAt   time.Time     `json:"expires_at"`
	Status      CleanupStatus `json:"status"`
	Error       string        `json:"error,omitempty"`
	CreatedAt   time.Time     `json:"created_at"`
	CleanedAt   time.Time     `json:"cleaned_at,omitempty"`
	CleanupRuns int           `json:"cleanup_runs,omitempty"`
}

type NamespaceCleaner interface {
	DeleteTempNamespace(ctx context.Context, namespace TempNamespace) error
}

type TempNamespaceManager struct {
	mu         sync.Mutex
	namespaces map[string]TempNamespace
	cleaner    NamespaceCleaner
	now        func() time.Time
}

func NewTempNamespaceManager(cleaner NamespaceCleaner) *TempNamespaceManager {
	return &TempNamespaceManager{
		namespaces: map[string]TempNamespace{},
		cleaner:    cleaner,
		now:        time.Now,
	}
}

func (m *TempNamespaceManager) Register(ownerID, kind, name string, ttl time.Duration) TempNamespace {
	if m == nil {
		return TempNamespace{}
	}
	if ttl <= 0 {
		ttl = time.Hour
	}
	now := m.clock().UTC()
	namespace := TempNamespace{
		Name:      name,
		OwnerID:   ownerID,
		Kind:      kind,
		ExpiresAt: now.Add(ttl),
		Status:    CleanupPending,
		CreatedAt: now,
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.namespaces[name] = namespace
	return namespace
}

func (m *TempNamespaceManager) ListByOwner(ownerID string) []TempNamespace {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []TempNamespace
	for _, namespace := range m.namespaces {
		if namespace.OwnerID == ownerID {
			out = append(out, namespace)
		}
	}
	return out
}

func (m *TempNamespaceManager) CleanupOwner(ctx context.Context, ownerID string) ([]TempNamespace, error) {
	if m == nil {
		return nil, nil
	}
	m.mu.Lock()
	var targets []TempNamespace
	for _, namespace := range m.namespaces {
		if namespace.OwnerID == ownerID && namespace.Status != CleanupDone {
			targets = append(targets, namespace)
		}
	}
	m.mu.Unlock()
	return m.cleanup(ctx, targets)
}

func (m *TempNamespaceManager) GC(ctx context.Context) ([]TempNamespace, error) {
	if m == nil {
		return nil, nil
	}
	now := m.clock().UTC()
	m.mu.Lock()
	var targets []TempNamespace
	for _, namespace := range m.namespaces {
		if namespace.Status != CleanupDone && !namespace.ExpiresAt.After(now) {
			targets = append(targets, namespace)
		}
	}
	m.mu.Unlock()
	return m.cleanup(ctx, targets)
}

func (m *TempNamespaceManager) cleanup(ctx context.Context, targets []TempNamespace) ([]TempNamespace, error) {
	cleaned := make([]TempNamespace, 0, len(targets))
	var firstErr error
	for _, namespace := range targets {
		namespace.CleanupRuns++
		if m.cleaner != nil {
			if err := m.cleaner.DeleteTempNamespace(ctx, namespace); err != nil {
				namespace.Status = CleanupFailed
				namespace.Error = err.Error()
				if firstErr == nil {
					firstErr = err
				}
				m.save(namespace)
				cleaned = append(cleaned, namespace)
				continue
			}
		}
		namespace.Status = CleanupDone
		namespace.Error = ""
		namespace.CleanedAt = m.clock().UTC()
		m.save(namespace)
		cleaned = append(cleaned, namespace)
	}
	return cleaned, firstErr
}

func (m *TempNamespaceManager) save(namespace TempNamespace) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.namespaces[namespace.Name] = namespace
}

func (m *TempNamespaceManager) clock() time.Time {
	if m.now != nil {
		return m.now()
	}
	return time.Now()
}

func defaultNamespaceName(candidateID, kind string) string {
	return fmt.Sprintf("orag_tmp_%s_%s", candidateID, kind)
}
