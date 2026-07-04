package optimizer

import (
	"context"
	"time"
)

type RateLimiter interface {
	Wait(ctx context.Context) error
}

type NoopRateLimiter struct{}

func (NoopRateLimiter) Wait(context.Context) error { return nil }

type IntervalRateLimiter struct {
	Interval time.Duration
	last     chan time.Time
}

func NewIntervalRateLimiter(interval time.Duration) *IntervalRateLimiter {
	limiter := &IntervalRateLimiter{Interval: interval, last: make(chan time.Time, 1)}
	limiter.last <- time.Time{}
	return limiter
}

func (l *IntervalRateLimiter) Wait(ctx context.Context) error {
	if l == nil || l.Interval <= 0 {
		return nil
	}
	previous := <-l.last
	defer func() { l.last <- time.Now() }()
	if previous.IsZero() {
		return nil
	}
	delay := time.Until(previous.Add(l.Interval))
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
