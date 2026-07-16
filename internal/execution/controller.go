// Package execution applies finite, fail-fast execution budgets to expensive
// request classes. It intentionally has no queue: callers receive a retryable
// rejection instead of consuming unbounded memory while waiting.
package execution

import (
	"context"
	"time"
)

type Operation string

const (
	Ingestion  Operation = "ingestion"
	Query      Operation = "query"
	Evaluation Operation = "evaluation"
	Release    Operation = "release"
)

type Budget struct {
	Timeout     time.Duration
	Concurrency int
}

type Controller struct {
	budgets map[Operation]Budget
	slots   map[Operation]chan struct{}
}

func New(budgets map[Operation]Budget) *Controller {
	copyBudgets := make(map[Operation]Budget, len(budgets))
	slots := make(map[Operation]chan struct{}, len(budgets))
	for operation, budget := range budgets {
		copyBudgets[operation] = budget
		if budget.Concurrency > 0 {
			slots[operation] = make(chan struct{}, budget.Concurrency)
		}
	}
	return &Controller{budgets: copyBudgets, slots: slots}
}

// Start admits one operation without waiting. The returned context has the
// operation deadline; callers must defer release. A nil release means the
// operation was rejected because all permitted slots are occupied.
func (c *Controller) Start(ctx context.Context, operation Operation) (context.Context, func(), bool) {
	if c == nil {
		return ctx, func() {}, true
	}
	budget, ok := c.budgets[operation]
	if !ok || budget.Concurrency <= 0 || budget.Timeout <= 0 {
		return ctx, func() {}, true
	}
	slot := c.slots[operation]
	select {
	case slot <- struct{}{}:
	default:
		return ctx, nil, false
	}
	runCtx, cancel := context.WithTimeout(ctx, budget.Timeout)
	return runCtx, func() {
		cancel()
		<-slot
	}, true
}

func (c *Controller) Budget(operation Operation) (Budget, bool) {
	if c == nil {
		return Budget{}, false
	}
	budget, ok := c.budgets[operation]
	return budget, ok
}
