package tutorial

import (
	"context"
	"errors"
	"sync"
)

type ExperimentRunRunner struct {
	service *LiveRunService
	mu      sync.Mutex
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

func NewExperimentRunRunner(service *LiveRunService) *ExperimentRunRunner {
	return &ExperimentRunRunner{service: service}
}

func (r *ExperimentRunRunner) Start(parent context.Context) error {
	if r == nil || r.service == nil {
		return errors.New("tutorial experiment run runner is not configured")
	}
	if parent == nil {
		parent = context.Background()
	}
	r.mu.Lock()
	if r.ctx != nil {
		r.mu.Unlock()
		return errors.New("tutorial experiment run runner is already started")
	}
	r.ctx, r.cancel = context.WithCancel(parent)
	ctx := r.ctx
	r.mu.Unlock()
	pending, err := r.service.RecoverPending(ctx)
	if err != nil {
		_ = r.Stop()
		return err
	}
	for _, run := range pending {
		r.Schedule(run.TenantID, run.ID)
	}
	return nil
}

func (r *ExperimentRunRunner) Schedule(tenantID, runID string) bool {
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
		_ = r.service.Execute(ctx, tenantID, runID)
	}()
	return true
}

func (r *ExperimentRunRunner) Stop() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	cancel := r.cancel
	r.ctx, r.cancel = nil, nil
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	r.wg.Wait()
	return nil
}
