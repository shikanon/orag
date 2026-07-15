package tutorial

import (
	"context"
	"errors"
	"sync"
)

// CloneRunner gives clone work a process lifetime independent of the HTTP
// request that scheduled it. Repository compare-and-swap claims make duplicate
// scheduling safe, including recovery after a process restart.
type CloneRunner struct {
	service *CloneService

	mu     sync.Mutex
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewCloneRunner(service *CloneService) *CloneRunner {
	return &CloneRunner{service: service}
}

func (r *CloneRunner) Start(parent context.Context) error {
	if r == nil || r.service == nil {
		return errors.New("tutorial clone runner is not configured")
	}
	if parent == nil {
		parent = context.Background()
	}
	r.mu.Lock()
	if r.ctx != nil {
		r.mu.Unlock()
		return errors.New("tutorial clone runner is already started")
	}
	r.ctx, r.cancel = context.WithCancel(parent)
	ctx := r.ctx
	r.mu.Unlock()

	pending, err := r.service.RecoverPending(ctx)
	if err != nil {
		_ = r.Stop()
		return err
	}
	for _, job := range pending {
		r.Schedule(Subject{TenantID: job.TenantID, ID: job.SubjectID}, job.ID)
	}
	return nil
}

// Schedule starts lifecycle-owned work. The durable Acquire compare-and-swap
// in CloneService makes no-op duplicate schedules safe.
func (r *CloneRunner) Schedule(subject Subject, jobID string) bool {
	if r == nil || r.service == nil {
		return false
	}
	r.mu.Lock()
	ctx := r.ctx
	if ctx == nil || ctx.Err() != nil {
		r.mu.Unlock()
		return false
	}
	r.wg.Add(1)
	r.mu.Unlock()
	go func() {
		defer r.wg.Done()
		_ = r.service.Run(ctx, subject, jobID)
	}()
	return true
}

func (r *CloneRunner) Stop() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	cancel := r.cancel
	r.ctx = nil
	r.cancel = nil
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	r.wg.Wait()
	return nil
}
