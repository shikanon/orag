package audit

import (
	"context"
	"log/slog"
	"sync"

	"github.com/shikanon/orag/internal/platform/clock"
)

type AuditService struct {
	repo    AuditRepository
	clock   clock.Clock
	async   bool
	buffer  chan AuditEvent
	wg      sync.WaitGroup
	mu      sync.RWMutex
	running bool
	logger  *slog.Logger
}

func NewAuditService(repo AuditRepository, timer clock.Clock, asyncBufferSize int) *AuditService {
	if timer == nil {
		timer = clock.RealClock{}
	}
	async := asyncBufferSize > 0
	svc := &AuditService{
		repo:   repo,
		clock:  timer,
		async:  async,
		logger: slog.Default(),
	}
	if async {
		svc.buffer = make(chan AuditEvent, asyncBufferSize)
	}
	return svc
}

func (s *AuditService) SetLogger(logger *slog.Logger) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logger = logger
}

func (s *AuditService) Start(ctx context.Context) {
	s.mu.Lock()
	if s.running || !s.async {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	s.wg.Add(1)
	go s.processLoop(ctx)
}

func (s *AuditService) processLoop(ctx context.Context) {
	defer s.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-s.buffer:
			if !ok {
				return
			}
			if err := s.repo.Record(ctx, event); err != nil {
				s.logger.Error("audit record failed",
					"event_id", event.ID,
					"action", event.Action,
					"error", err,
				)
			}
		}
	}
}

func (s *AuditService) Stop() {
	s.mu.Lock()
	if !s.running || !s.async {
		s.mu.Unlock()
		return
	}
	s.running = false
	close(s.buffer)
	s.mu.Unlock()

	s.wg.Wait()
}

func (s *AuditService) Record(ctx context.Context, event AuditEvent) error {
	if s.async {
		s.mu.RLock()
		running := s.running
		s.mu.RUnlock()

		if !running {
			return nil
		}

		select {
		case s.buffer <- event:
		default:
			s.logger.Warn("audit buffer full, dropping event",
				"action", event.Action,
				"resource_type", event.ResourceType,
			)
		}
		return nil
	}

	return s.repo.Record(ctx, event)
}

func (s *AuditService) List(ctx context.Context, filter AuditFilter) ([]AuditEvent, string, error) {
	return s.repo.List(ctx, filter)
}

func (s *AuditService) Get(ctx context.Context, eventID string) (AuditEvent, bool, error) {
	return s.repo.Get(ctx, eventID)
}
