package audit

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/shikanon/orag/internal/platform/clock"
	"github.com/shikanon/orag/internal/platform/id"
)

type MemoryAuditRepository struct {
	mu     sync.RWMutex
	events []AuditEvent
	byID   map[string]int
	clock  clock.Clock
}

func NewMemoryAuditRepository() *MemoryAuditRepository {
	return &MemoryAuditRepository{
		events: []AuditEvent{},
		byID:   map[string]int{},
		clock:  clock.RealClock{},
	}
}

func (r *MemoryAuditRepository) SetClock(c clock.Clock) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clock = c
}

func (r *MemoryAuditRepository) Record(_ context.Context, event AuditEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := r.clock.Now()
	if event.ID == "" {
		event.ID = id.New("audit")
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = now
	}
	if event.Metadata == nil {
		event.Metadata = map[string]string{}
	}

	eventCopy := event
	r.events = append(r.events, eventCopy)
	r.byID[event.ID] = len(r.events) - 1

	return nil
}

func (r *MemoryAuditRepository) Get(_ context.Context, eventID string) (AuditEvent, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	idx, ok := r.byID[eventID]
	if !ok {
		return AuditEvent{}, false, nil
	}
	return r.events[idx], true, nil
}

func (r *MemoryAuditRepository) List(_ context.Context, filter AuditFilter) ([]AuditEvent, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}

	type cursorKey struct {
		CreatedAt time.Time
		ID        string
	}

	parseCursor := func(cursor string) (cursorKey, error) {
		if cursor == "" {
			return cursorKey{}, nil
		}
		var unixNano int64
		var key cursorKey
		_, err := fmt.Sscanf(cursor, "%d_%s", &unixNano, &key.ID)
		if err != nil {
			return cursorKey{}, fmt.Errorf("invalid cursor: %w", err)
		}
		key.CreatedAt = time.Unix(0, unixNano)
		return key, nil
	}

	encodeCursor := func(event AuditEvent) string {
		return fmt.Sprintf("%d_%s", event.CreatedAt.UnixNano(), event.ID)
	}

	cursorVal, err := parseCursor(filter.Cursor)
	if err != nil {
		return nil, "", err
	}

	indices := make([]int, len(r.events))
	for i := range r.events {
		indices[i] = i
	}

	sort.Slice(indices, func(i, j int) bool {
		ei := r.events[indices[i]]
		ej := r.events[indices[j]]
		if !ei.CreatedAt.Equal(ej.CreatedAt) {
			return ei.CreatedAt.After(ej.CreatedAt)
		}
		return ei.ID > ej.ID
	})

	var result []AuditEvent
	startFound := filter.Cursor == ""

	for _, idx := range indices {
		event := r.events[idx]

		if !startFound {
			if event.CreatedAt.Equal(cursorVal.CreatedAt) && event.ID == cursorVal.ID {
				startFound = true
			}
			continue
		}

		if filter.TenantID != "" && event.TenantID != filter.TenantID {
			continue
		}
		if filter.ProjectID != "" && event.ProjectID != filter.ProjectID {
			continue
		}
		if filter.ResourceType != "" && event.ResourceType != filter.ResourceType {
			continue
		}
		if filter.ResourceID != "" && event.ResourceID != filter.ResourceID {
			continue
		}
		if filter.Action != "" && event.Action != filter.Action {
			continue
		}
		if filter.ActorID != "" && event.ActorID != filter.ActorID {
			continue
		}
		if filter.Outcome != "" && event.Outcome != filter.Outcome {
			continue
		}

		result = append(result, event)
		if len(result) >= limit {
			break
		}
	}

	var nextCursor string
	if len(result) == limit {
		nextCursor = encodeCursor(result[len(result)-1])
	}

	return result, nextCursor, nil
}
