package audit

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/shikanon/orag/internal/platform/clock"
)

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock(t time.Time) *fakeClock {
	return &fakeClock{now: t}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func (c *fakeClock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = t
}

var _ clock.Clock = (*fakeClock)(nil)

func newTestEvent(action, resourceType, resourceID string) AuditEvent {
	return AuditEvent{
		TenantID:     "test-tenant",
		ProjectID:    "test-project",
		ActorType:    ActorTypeUser,
		ActorID:      "user-123",
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Outcome:      OutcomeSuccess,
		TraceID:      "trace-abc",
		TaskID:       "task-xyz",
		Metadata: map[string]string{
			"version": "v1",
			"hash":    "sha256-abc",
		},
	}
}

func TestAuditEvent_AllRequiredFields(t *testing.T) {
	repo := NewMemoryAuditRepository()
	ctx := context.Background()

	event := newTestEvent(ActionKBCreated, ResourceTypeKnowledgeBase, "kb-1")
	err := repo.Record(ctx, event)
	if err != nil {
		t.Fatalf("Record failed: %v", err)
	}

	got, ok, err := repo.Get(ctx, "")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if ok {
		t.Fatal("expected not found with empty id")
	}

	events, _, err := repo.List(ctx, AuditFilter{TenantID: "test-tenant"})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	got = events[0]
	if got.ID == "" {
		t.Error("expected ID to be set")
	}
	if got.TenantID != "test-tenant" {
		t.Errorf("expected TenantID test-tenant, got %s", got.TenantID)
	}
	if got.ProjectID != "test-project" {
		t.Errorf("expected ProjectID test-project, got %s", got.ProjectID)
	}
	if got.ActorType != ActorTypeUser {
		t.Errorf("expected ActorType user, got %s", got.ActorType)
	}
	if got.ActorID != "user-123" {
		t.Errorf("expected ActorID user-123, got %s", got.ActorID)
	}
	if got.Action != ActionKBCreated {
		t.Errorf("expected Action %s, got %s", ActionKBCreated, got.Action)
	}
	if got.ResourceType != ResourceTypeKnowledgeBase {
		t.Errorf("expected ResourceType knowledge_base, got %s", got.ResourceType)
	}
	if got.ResourceID != "kb-1" {
		t.Errorf("expected ResourceID kb-1, got %s", got.ResourceID)
	}
	if got.Outcome != OutcomeSuccess {
		t.Errorf("expected Outcome success, got %s", got.Outcome)
	}
	if got.TraceID != "trace-abc" {
		t.Errorf("expected TraceID trace-abc, got %s", got.TraceID)
	}
	if got.TaskID != "task-xyz" {
		t.Errorf("expected TaskID task-xyz, got %s", got.TaskID)
	}
	if got.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
	if got.Metadata == nil {
		t.Error("expected Metadata to be initialized")
	}
}

func TestAsyncRecord_DoesNotBlock(t *testing.T) {
	slowRepo := &slowRepository{delay: 200 * time.Millisecond}
	svc := NewAuditService(slowRepo, clock.RealClock{}, 100)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	svc.Start(ctx)
	defer svc.Stop()

	event := newTestEvent(ActionKBCreated, ResourceTypeKnowledgeBase, "kb-1")

	start := time.Now()
	err := svc.Record(ctx, event)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Record failed: %v", err)
	}
	if elapsed > 100*time.Millisecond {
		t.Errorf("async record took too long: %v, expected < 100ms", elapsed)
	}

	time.Sleep(300 * time.Millisecond)

	if slowRepo.count() != 1 {
		t.Errorf("expected 1 event recorded, got %d", slowRepo.count())
	}
}

type slowRepository struct {
	mu    sync.Mutex
	delay time.Duration
	events []AuditEvent
}

func (r *slowRepository) Record(_ context.Context, event AuditEvent) error {
	time.Sleep(r.delay)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
	return nil
}

func (r *slowRepository) List(_ context.Context, _ AuditFilter) ([]AuditEvent, string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.events, "", nil
}

func (r *slowRepository) Get(_ context.Context, eventID string) (AuditEvent, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.events {
		if e.ID == eventID {
			return e, true, nil
		}
	}
	return AuditEvent{}, false, nil
}

func (r *slowRepository) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.events)
}

var _ AuditRepository = (*slowRepository)(nil)

func TestNoSensitiveInformationInMetadata(t *testing.T) {
	repo := NewMemoryAuditRepository()
	ctx := context.Background()

	event := newTestEvent(ActionKBCreated, ResourceTypeKnowledgeBase, "kb-1")
	event.Metadata = map[string]string{
		"version":       "v1",
		"hash":          "sha256-abc",
		"config_digest": "digest-123",
		"trace_id":      "trace-abc",
	}

	err := repo.Record(ctx, event)
	if err != nil {
		t.Fatalf("Record failed: %v", err)
	}

	events, _, _ := repo.List(ctx, AuditFilter{TenantID: "test-tenant"})
	got := events[0]

	sensitiveKeys := []string{"prompt", "completion", "content", "document_content", "api_key", "secret", "password", "provider_key"}
	for _, key := range sensitiveKeys {
		if _, ok := got.Metadata[key]; ok {
			t.Errorf("metadata should not contain sensitive key: %s", key)
		}
	}
}

func TestListByTenantAndResource(t *testing.T) {
	repo := NewMemoryAuditRepository()
	fc := newFakeClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	repo.SetClock(fc)
	ctx := context.Background()

	e1 := newTestEvent(ActionKBCreated, ResourceTypeKnowledgeBase, "kb-1")
	e1.TenantID = "tenant-a"
	e1.ProjectID = "proj-a"
	repo.Record(ctx, e1)
	fc.Add(1 * time.Second)

	e2 := newTestEvent(ActionDocumentImportCompleted, ResourceTypeDocument, "doc-1")
	e2.TenantID = "tenant-a"
	e2.ProjectID = "proj-a"
	repo.Record(ctx, e2)
	fc.Add(1 * time.Second)

	e3 := newTestEvent(ActionKBCreated, ResourceTypeKnowledgeBase, "kb-2")
	e3.TenantID = "tenant-b"
	e3.ProjectID = "proj-b"
	repo.Record(ctx, e3)

	events, _, err := repo.List(ctx, AuditFilter{TenantID: "tenant-a"})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("expected 2 events for tenant-a, got %d", len(events))
	}

	eventsKB, _, err := repo.List(ctx, AuditFilter{
		TenantID:     "tenant-a",
		ResourceType: ResourceTypeKnowledgeBase,
	})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(eventsKB) != 1 {
		t.Errorf("expected 1 kb event for tenant-a, got %d", len(eventsKB))
	}
	if eventsKB[0].ResourceID != "kb-1" {
		t.Errorf("expected resource kb-1, got %s", eventsKB[0].ResourceID)
	}
}

func TestCursorPagination(t *testing.T) {
	repo := NewMemoryAuditRepository()
	fc := newFakeClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	repo.SetClock(fc)
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		e := newTestEvent(ActionKBCreated, ResourceTypeKnowledgeBase, "kb-1")
		e.Metadata = map[string]string{"index": string(rune('0' + i))}
		repo.Record(ctx, e)
		fc.Add(1 * time.Second)
	}

	page1, cursor1, err := repo.List(ctx, AuditFilter{
		TenantID: "test-tenant",
		Limit:    3,
	})
	if err != nil {
		t.Fatalf("List page1 failed: %v", err)
	}
	if len(page1) != 3 {
		t.Fatalf("expected 3 events on page1, got %d", len(page1))
	}
	if cursor1 == "" {
		t.Fatal("expected cursor1 to be set")
	}

	page2, cursor2, err := repo.List(ctx, AuditFilter{
		TenantID: "test-tenant",
		Limit:    3,
		Cursor:   cursor1,
	})
	if err != nil {
		t.Fatalf("List page2 failed: %v", err)
	}
	if len(page2) != 3 {
		t.Fatalf("expected 3 events on page2, got %d", len(page2))
	}
	if cursor2 == "" {
		t.Fatal("expected cursor2 to be set")
	}

	page3, cursor3, err := repo.List(ctx, AuditFilter{
		TenantID: "test-tenant",
		Limit:    3,
		Cursor:   cursor2,
	})
	if err != nil {
		t.Fatalf("List page3 failed: %v", err)
	}
	if len(page3) != 3 {
		t.Fatalf("expected 3 events on page3, got %d", len(page3))
	}
	if cursor3 == "" {
		t.Fatal("expected cursor3 to be set")
	}

	page4, cursor4, err := repo.List(ctx, AuditFilter{
		TenantID: "test-tenant",
		Limit:    3,
		Cursor:   cursor3,
	})
	if err != nil {
		t.Fatalf("List page4 failed: %v", err)
	}
	if len(page4) != 1 {
		t.Fatalf("expected 1 event on page4, got %d", len(page4))
	}
	if cursor4 != "" {
		t.Error("expected cursor4 to be empty for last page")
	}

	seen := map[string]bool{}
	allPages := append(append(append(page1, page2...), page3...), page4...)
	for _, e := range allPages {
		if seen[e.ID] {
			t.Errorf("duplicate event: %s", e.ID)
		}
		seen[e.ID] = true
	}
	if len(seen) != 10 {
		t.Errorf("expected 10 unique events, got %d", len(seen))
	}
}

func TestListFilterByActionResourceTypeOutcome(t *testing.T) {
	repo := NewMemoryAuditRepository()
	fc := newFakeClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	repo.SetClock(fc)
	ctx := context.Background()

	e1 := newTestEvent(ActionKBCreated, ResourceTypeKnowledgeBase, "kb-1")
	e1.Outcome = OutcomeSuccess
	repo.Record(ctx, e1)
	fc.Add(1 * time.Second)

	e2 := newTestEvent(ActionDocumentImportRequested, ResourceTypeDocument, "doc-1")
	e2.Outcome = OutcomeFailure
	e2.ErrorCode = "file_too_large"
	repo.Record(ctx, e2)
	fc.Add(1 * time.Second)

	e3 := newTestEvent(ActionPipelineUpdated, ResourceTypePipeline, "pipe-1")
	e3.Outcome = OutcomeSuccess
	repo.Record(ctx, e3)
	fc.Add(1 * time.Second)

	e4 := newTestEvent(ActionReleasePromoted, ResourceTypeRelease, "rel-1")
	e4.Outcome = OutcomeCancelled
	repo.Record(ctx, e4)

	byAction, _, err := repo.List(ctx, AuditFilter{
		TenantID: "test-tenant",
		Action:   ActionKBCreated,
	})
	if err != nil {
		t.Fatalf("List by action failed: %v", err)
	}
	if len(byAction) != 1 {
		t.Errorf("expected 1 event for action kb.created, got %d", len(byAction))
	}

	byResourceType, _, err := repo.List(ctx, AuditFilter{
		TenantID:     "test-tenant",
		ResourceType: ResourceTypeDocument,
	})
	if err != nil {
		t.Fatalf("List by resource type failed: %v", err)
	}
	if len(byResourceType) != 1 {
		t.Errorf("expected 1 event for document type, got %d", len(byResourceType))
	}

	byOutcome, _, err := repo.List(ctx, AuditFilter{
		TenantID: "test-tenant",
		Outcome:  OutcomeFailure,
	})
	if err != nil {
		t.Fatalf("List by outcome failed: %v", err)
	}
	if len(byOutcome) != 1 {
		t.Errorf("expected 1 failure event, got %d", len(byOutcome))
	}
	if byOutcome[0].ErrorCode != "file_too_large" {
		t.Errorf("expected error code file_too_large, got %s", byOutcome[0].ErrorCode)
	}
}

func TestAuditService_SyncMode(t *testing.T) {
	repo := NewMemoryAuditRepository()
	svc := NewAuditService(repo, clock.RealClock{}, 0)
	ctx := context.Background()

	event := newTestEvent(ActionKBCreated, ResourceTypeKnowledgeBase, "kb-1")
	err := svc.Record(ctx, event)
	if err != nil {
		t.Fatalf("Record failed: %v", err)
	}

	events, _, err := svc.List(ctx, AuditFilter{TenantID: "test-tenant"})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
}

func TestAuditService_AsyncBufferFull(t *testing.T) {
	bufRepo := &bufferTrackingRepository{}
	svc := NewAuditService(bufRepo, clock.RealClock{}, 2)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	svc.Start(ctx)

	bufRepo.block()

	e1 := newTestEvent(ActionKBCreated, ResourceTypeKnowledgeBase, "kb-1")
	e2 := newTestEvent(ActionKBUpdated, ResourceTypeKnowledgeBase, "kb-1")
	e3 := newTestEvent(ActionDocumentImportRequested, ResourceTypeDocument, "doc-1")

	svc.Record(ctx, e1)
	svc.Record(ctx, e2)
	svc.Record(ctx, e3)

	bufRepo.unblock()
	svc.Stop()

	count := bufRepo.count()
	if count > 2 {
		t.Errorf("expected at most 2 events recorded (buffer full drop), got %d", count)
	}
}

type bufferTrackingRepository struct {
	mu     sync.Mutex
	events []AuditEvent
	blocked bool
	blockCh chan struct{}
}

func (r *bufferTrackingRepository) block() {
	r.mu.Lock()
	r.blocked = true
	r.blockCh = make(chan struct{})
	r.mu.Unlock()
}

func (r *bufferTrackingRepository) unblock() {
	r.mu.Lock()
	if r.blocked {
		r.blocked = false
		close(r.blockCh)
	}
	r.mu.Unlock()
}

func (r *bufferTrackingRepository) Record(_ context.Context, event AuditEvent) error {
	r.mu.Lock()
	blocked := r.blocked
	ch := r.blockCh
	r.mu.Unlock()

	if blocked {
		<-ch
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
	return nil
}

func (r *bufferTrackingRepository) List(_ context.Context, _ AuditFilter) ([]AuditEvent, string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.events, "", nil
}

func (r *bufferTrackingRepository) Get(_ context.Context, eventID string) (AuditEvent, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.events {
		if e.ID == eventID {
			return e, true, nil
		}
	}
	return AuditEvent{}, false, nil
}

func (r *bufferTrackingRepository) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.events)
}

var _ AuditRepository = (*bufferTrackingRepository)(nil)

func TestListOrdering_NewestFirst(t *testing.T) {
	repo := NewMemoryAuditRepository()
	fc := newFakeClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	repo.SetClock(fc)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		e := newTestEvent(ActionKBCreated, ResourceTypeKnowledgeBase, "kb-1")
		e.Metadata = map[string]string{"seq": string(rune('0' + i))}
		repo.Record(ctx, e)
		fc.Add(1 * time.Second)
	}

	events, _, err := repo.List(ctx, AuditFilter{TenantID: "test-tenant"})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(events) != 5 {
		t.Fatalf("expected 5 events, got %d", len(events))
	}

	if events[0].Metadata["seq"] != "4" {
		t.Errorf("expected newest first (seq=4), got seq=%s", events[0].Metadata["seq"])
	}
	if events[4].Metadata["seq"] != "0" {
		t.Errorf("expected oldest last (seq=0), got seq=%s", events[4].Metadata["seq"])
	}
}

func TestListByActorID(t *testing.T) {
	repo := NewMemoryAuditRepository()
	fc := newFakeClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	repo.SetClock(fc)
	ctx := context.Background()

	e1 := newTestEvent(ActionKBCreated, ResourceTypeKnowledgeBase, "kb-1")
	e1.ActorID = "user-alice"
	repo.Record(ctx, e1)
	fc.Add(1 * time.Second)

	e2 := newTestEvent(ActionKBUpdated, ResourceTypeKnowledgeBase, "kb-1")
	e2.ActorID = "user-bob"
	repo.Record(ctx, e2)
	fc.Add(1 * time.Second)

	e3 := newTestEvent(ActionPipelineUpdated, ResourceTypePipeline, "pipe-1")
	e3.ActorID = "user-alice"
	repo.Record(ctx, e3)

	byAlice, _, err := repo.List(ctx, AuditFilter{
		TenantID: "test-tenant",
		ActorID:  "user-alice",
	})
	if err != nil {
		t.Fatalf("List by actor failed: %v", err)
	}
	if len(byAlice) != 2 {
		t.Errorf("expected 2 events by alice, got %d", len(byAlice))
	}
}
