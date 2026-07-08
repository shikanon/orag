package offlineknowledge

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	ErrSchedulerServiceRequired = errors.New("offline knowledge scheduler service is required")
	ErrSchedulerTargetRequired  = errors.New("offline knowledge scheduler requires at least one tenant/kb target")
)

type RunOnceRunner interface {
	RunOnce(ctx context.Context, request RunRequest) (RunResult, error)
}

type SchedulerTarget struct {
	TenantID string
	KBID     string
}

type SchedulerConfig struct {
	Enabled            bool
	Schedule           string
	LookbackDays       int
	Targets            []SchedulerTarget
	ConfigJSON         map[string]any
	MaxQuestionsPerRun int
	MaxClustersPerRun  int
}

type SchedulerOptions struct {
	Clock  SchedulerClock
	Logger *slog.Logger
}

type SchedulerClock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

type SchedulerRunResult struct {
	Target       SchedulerTarget
	Request      RunRequest
	Result       RunResult
	Deduplicated bool
	Err          error
}

type Scheduler struct {
	runner RunOnceRunner
	cfg    SchedulerConfig
	clock  SchedulerClock
	logger *slog.Logger

	mu       sync.Mutex
	started  bool
	cancel   context.CancelFunc
	done     chan struct{}
	inFlight map[string]struct{}
}

func NewScheduler(runner RunOnceRunner, cfg SchedulerConfig, opts SchedulerOptions) *Scheduler {
	clock := opts.Clock
	if clock == nil {
		clock = systemSchedulerClock{}
	}
	return &Scheduler{
		runner:   runner,
		cfg:      normalizeSchedulerConfig(cfg),
		clock:    clock,
		logger:   opts.Logger,
		inFlight: make(map[string]struct{}),
	}
}

func (s *Scheduler) Enabled() bool {
	return s != nil && s.cfg.Enabled
}

func (s *Scheduler) IsRunning() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.started
}

func (s *Scheduler) Start(ctx context.Context) error {
	if s == nil || !s.cfg.Enabled {
		return nil
	}
	if s.runner == nil {
		return ErrSchedulerServiceRequired
	}
	if len(s.cfg.Targets) == 0 {
		return ErrSchedulerTargetRequired
	}
	parsed, err := parseCronSchedule(s.cfg.Schedule)
	if err != nil {
		return err
	}

	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return nil
	}
	runCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.done = make(chan struct{})
	s.started = true
	s.mu.Unlock()

	go s.loop(runCtx, parsed, s.done)
	return nil
}

func (s *Scheduler) Stop() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	if !s.started {
		s.mu.Unlock()
		return nil
	}
	cancel := s.cancel
	done := s.done
	s.started = false
	s.cancel = nil
	s.done = nil
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
	return nil
}

func (s *Scheduler) Trigger(ctx context.Context, scheduledAt time.Time) []SchedulerRunResult {
	if s == nil || !s.cfg.Enabled {
		return nil
	}
	windowEnd := scheduledAt.UTC().Truncate(time.Minute)
	windowStart := windowEnd.Add(-time.Duration(s.cfg.LookbackDays) * 24 * time.Hour)
	results := make([]SchedulerRunResult, 0, len(s.cfg.Targets))
	for _, target := range s.cfg.Targets {
		req := RunRequest{
			TenantID:     target.TenantID,
			KBID:         target.KBID,
			WindowStart:  windowStart,
			WindowEnd:    windowEnd,
			ConfigJSON:   copyMap(s.cfg.ConfigJSON),
			MaxQuestions: s.cfg.MaxQuestionsPerRun,
			MaxClusters:  s.cfg.MaxClustersPerRun,
		}
		configHash, err := resolveConfigHash("", req.ConfigJSON)
		if err != nil {
			results = append(results, SchedulerRunResult{Target: target, Request: req, Err: err})
			continue
		}
		req.ConfigHash = configHash
		key := schedulerRunKey(req)
		if !s.acquireRun(key) {
			results = append(results, SchedulerRunResult{Target: target, Request: req, Deduplicated: true})
			continue
		}
		result, err := s.runner.RunOnce(ctx, req)
		s.releaseRun(key)
		results = append(results, SchedulerRunResult{
			Target:       target,
			Request:      req,
			Result:       result,
			Deduplicated: result.Deduplicated,
			Err:          err,
		})
	}
	return results
}

func (s *Scheduler) loop(ctx context.Context, schedule cronSchedule, done chan struct{}) {
	defer close(done)
	for {
		next := schedule.Next(s.clock.Now())
		wait := next.Sub(s.clock.Now())
		if wait < 0 {
			wait = 0
		}
		select {
		case <-ctx.Done():
			return
		case firedAt := <-s.clock.After(wait):
			scheduledAt := next
			if !firedAt.IsZero() {
				scheduledAt = firedAt
			}
			for _, result := range s.Trigger(ctx, scheduledAt) {
				if result.Err != nil && s.logger != nil {
					s.logger.Warn("offline knowledge scheduled run failed",
						"tenant_id", result.Target.TenantID,
						"kb_id", result.Target.KBID,
						"error", result.Err)
				}
			}
		}
	}
}

func (s *Scheduler) acquireRun(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.inFlight[key]; ok {
		return false
	}
	s.inFlight[key] = struct{}{}
	return true
}

func (s *Scheduler) releaseRun(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.inFlight, key)
}

func normalizeSchedulerConfig(cfg SchedulerConfig) SchedulerConfig {
	cfg.Schedule = strings.TrimSpace(cfg.Schedule)
	if cfg.LookbackDays <= 0 {
		cfg.LookbackDays = 1
	}
	targets := make([]SchedulerTarget, 0, len(cfg.Targets))
	seen := map[string]struct{}{}
	for _, target := range cfg.Targets {
		target.TenantID = strings.TrimSpace(target.TenantID)
		target.KBID = normalizeKBID(target.KBID)
		if target.TenantID == "" {
			continue
		}
		key := target.TenantID + "\x00" + target.KBID
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		targets = append(targets, target)
	}
	cfg.Targets = targets
	cfg.ConfigJSON = normalizeSchedulerConfigJSON(cfg)
	return cfg
}

func normalizeSchedulerConfigJSON(cfg SchedulerConfig) map[string]any {
	out := copyMap(cfg.ConfigJSON)
	if out == nil {
		out = map[string]any{}
	}
	out["schedule"] = cfg.Schedule
	out["lookback_days"] = cfg.LookbackDays
	out["max_questions_per_run"] = cfg.MaxQuestionsPerRun
	out["max_clusters_per_run"] = cfg.MaxClustersPerRun
	return out
}

func schedulerRunKey(req RunRequest) string {
	return strings.Join([]string{
		req.TenantID,
		normalizeKBID(req.KBID),
		req.WindowStart.UTC().Format(time.RFC3339Nano),
		req.WindowEnd.UTC().Format(time.RFC3339Nano),
		req.ConfigHash,
	}, "\x00")
}

type systemSchedulerClock struct{}

func (systemSchedulerClock) Now() time.Time {
	return time.Now()
}

func (systemSchedulerClock) After(d time.Duration) <-chan time.Time {
	return time.After(d)
}

type cronSchedule struct {
	minute     cronField
	hour       cronField
	dayOfMonth cronField
	month      cronField
	dayOfWeek  cronField
}

func parseCronSchedule(expr string) (cronSchedule, error) {
	parts := strings.Fields(expr)
	if len(parts) != 5 {
		return cronSchedule{}, fmt.Errorf("offline knowledge scheduler schedule must be a 5-field cron expression")
	}
	minute, err := parseCronField(parts[0], 0, 59)
	if err != nil {
		return cronSchedule{}, fmt.Errorf("invalid cron minute: %w", err)
	}
	hour, err := parseCronField(parts[1], 0, 23)
	if err != nil {
		return cronSchedule{}, fmt.Errorf("invalid cron hour: %w", err)
	}
	dayOfMonth, err := parseCronField(parts[2], 1, 31)
	if err != nil {
		return cronSchedule{}, fmt.Errorf("invalid cron day-of-month: %w", err)
	}
	month, err := parseCronField(parts[3], 1, 12)
	if err != nil {
		return cronSchedule{}, fmt.Errorf("invalid cron month: %w", err)
	}
	dayOfWeek, err := parseCronField(parts[4], 0, 7)
	if err != nil {
		return cronSchedule{}, fmt.Errorf("invalid cron day-of-week: %w", err)
	}
	return cronSchedule{minute: minute, hour: hour, dayOfMonth: dayOfMonth, month: month, dayOfWeek: dayOfWeek}, nil
}

func (s cronSchedule) Next(after time.Time) time.Time {
	next := after.UTC().Truncate(time.Minute).Add(time.Minute)
	for i := 0; i < 366*24*60*5; i++ {
		if s.matches(next) {
			return next
		}
		next = next.Add(time.Minute)
	}
	return after.UTC().Add(24 * time.Hour)
}

func (s cronSchedule) matches(t time.Time) bool {
	weekday := int(t.Weekday())
	return s.minute.matches(t.Minute()) &&
		s.hour.matches(t.Hour()) &&
		s.dayOfMonth.matches(t.Day()) &&
		s.month.matches(int(t.Month())) &&
		(s.dayOfWeek.matches(weekday) || (weekday == 0 && s.dayOfWeek.matches(7)))
}

type cronField struct {
	values map[int]struct{}
}

func parseCronField(expr string, minValue, maxValue int) (cronField, error) {
	values := map[int]struct{}{}
	for _, part := range strings.Split(expr, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			return cronField{}, errors.New("empty cron field part")
		}
		step := 1
		base := part
		if strings.Contains(part, "/") {
			pieces := strings.Split(part, "/")
			if len(pieces) != 2 {
				return cronField{}, fmt.Errorf("invalid step expression %q", part)
			}
			base = pieces[0]
			parsedStep, err := strconv.Atoi(pieces[1])
			if err != nil || parsedStep <= 0 {
				return cronField{}, fmt.Errorf("invalid step %q", pieces[1])
			}
			step = parsedStep
		}
		start, end, err := cronRange(base, minValue, maxValue)
		if err != nil {
			return cronField{}, err
		}
		for value := start; value <= end; value += step {
			if value < minValue || value > maxValue {
				return cronField{}, fmt.Errorf("value %d outside [%d,%d]", value, minValue, maxValue)
			}
			values[value] = struct{}{}
		}
	}
	return cronField{values: values}, nil
}

func cronRange(expr string, minValue, maxValue int) (int, int, error) {
	if expr == "*" {
		return minValue, maxValue, nil
	}
	if strings.Contains(expr, "-") {
		pieces := strings.Split(expr, "-")
		if len(pieces) != 2 {
			return 0, 0, fmt.Errorf("invalid range %q", expr)
		}
		start, err := strconv.Atoi(pieces[0])
		if err != nil {
			return 0, 0, err
		}
		end, err := strconv.Atoi(pieces[1])
		if err != nil {
			return 0, 0, err
		}
		if start > end {
			return 0, 0, fmt.Errorf("range %q starts after end", expr)
		}
		return start, end, nil
	}
	value, err := strconv.Atoi(expr)
	if err != nil {
		return 0, 0, err
	}
	return value, value, nil
}

func (f cronField) matches(value int) bool {
	_, ok := f.values[value]
	return ok
}
