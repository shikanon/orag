package offlineknowledge

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"
)

const ShadowSourceOptimizationLibrary = "optimization_library"

var ErrShadowRepositoryRequired = errors.New("offline knowledge shadow repository is required")

type ShadowRetrieveRequest struct {
	TenantID                   string
	KBID                       string
	Query                      string
	TraceID                    string
	Limit                      int
	Inject                     bool
	ScopedItemID               string
	AllowLowConfidenceFallback bool
}

type ShadowMatch struct {
	ItemID     string            `json:"item_id"`
	ItemType   ItemType          `json:"item_type"`
	Source     string            `json:"source"`
	Score      float64           `json:"score"`
	Rank       int               `json:"rank"`
	AnswerItem *ShadowAnswerItem `json:"answer_item,omitempty"`
	Metadata   map[string]any    `json:"metadata,omitempty"`
}

type ShadowAnswerItem struct {
	SourceFingerprints []SourceFingerprint `json:"source_fingerprints"`
	Evidence           []Evidence          `json:"evidence"`
	GuidanceMetadata   map[string]any      `json:"guidance_metadata,omitempty"`
}

type ShadowEventDrop struct {
	Reason string
	Event  ShadowRetrievalEvent
	Err    error
}

type ShadowDropMetric interface {
	RecordShadowEventDrop(reason string)
}

type ShadowHitMetric interface {
	ObserveOptimizationShadowHit(injected bool, latencyMS int64)
}

type ShadowRetrieverOptions struct {
	Limit                int
	EventSamplingRate    float64
	EventSamplingRateSet bool
	Now                  func() time.Time
	RandFloat64          func() float64
	OnEventDropped       func(ShadowEventDrop)
	DropMetric           ShadowDropMetric
	HitMetric            ShadowHitMetric
}

type ShadowRetriever struct {
	repo              Repository
	limit             int
	eventSamplingRate float64
	now               func() time.Time
	randFloat64       func() float64
	onEventDropped    func(ShadowEventDrop)
	dropMetric        ShadowDropMetric
	hitMetric         ShadowHitMetric
}

var (
	ErrScopedShadowItemMissing  = errors.New("offline knowledge scoped shadow item missing")
	ErrScopedShadowItemDisabled = errors.New("offline knowledge scoped shadow item disabled")
	ErrScopedShadowItemStale    = errors.New("offline knowledge scoped shadow item stale")
	ErrScopedShadowItemMismatch = errors.New("offline knowledge scoped shadow item source mismatch")
)

func NewShadowRetriever(repo Repository, opts ShadowRetrieverOptions) *ShadowRetriever {
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	rate := 1.0
	if opts.EventSamplingRateSet {
		rate = opts.EventSamplingRate
	}
	if rate > 1 {
		rate = 1
	}
	return &ShadowRetriever{
		repo:              repo,
		limit:             opts.Limit,
		eventSamplingRate: rate,
		now:               now,
		randFloat64:       opts.RandFloat64,
		onEventDropped:    opts.OnEventDropped,
		dropMetric:        opts.DropMetric,
		hitMetric:         opts.HitMetric,
	}
}

func (r *ShadowRetriever) Retrieve(ctx context.Context, request ShadowRetrieveRequest) ([]ShadowMatch, error) {
	if r == nil || r.repo == nil {
		return nil, ErrShadowRepositoryRequired
	}
	started := r.now()
	items, err := r.listEligibleItems(ctx, request)
	if err != nil {
		return nil, err
	}
	matches := make([]ShadowMatch, 0, len(items))
	for _, item := range items {
		score := shadowScore(request.Query, item.CanonicalQuestion, request.AllowLowConfidenceFallback)
		if score <= 0 {
			continue
		}
		matches = append(matches, shadowMatchFromItem(item, score))
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].Score == matches[j].Score {
			return matches[i].ItemID < matches[j].ItemID
		}
		return matches[i].Score > matches[j].Score
	})
	limit := firstPositive(request.Limit, r.limit)
	if limit > 0 && len(matches) > limit {
		matches = matches[:limit]
	}
	for i := range matches {
		matches[i].Rank = i + 1
		r.RecordShadowEvent(ctx, ShadowRetrievalEvent{
			ID:        stableID("shadow", request.TenantID, normalizeOptionalKBID(request.KBID), request.TraceID, matches[i].ItemID, r.now().UTC().Format(time.RFC3339Nano)),
			TenantID:  request.TenantID,
			KBID:      normalizeOptionalKBID(request.KBID),
			ItemID:    matches[i].ItemID,
			TraceID:   request.TraceID,
			Query:     request.Query,
			Matched:   true,
			Injected:  request.Inject,
			Rank:      matches[i].Rank,
			Score:     matches[i].Score,
			CreatedAt: r.now(),
		})
	}
	if r.hitMetric != nil && len(matches) > 0 {
		latencyMS := r.now().Sub(started).Milliseconds()
		for range matches {
			r.hitMetric.ObserveOptimizationShadowHit(request.Inject, latencyMS)
		}
	}
	return matches, nil
}

func (r *ShadowRetriever) RecordShadowEvent(ctx context.Context, event ShadowRetrievalEvent) {
	if r == nil || r.repo == nil {
		return
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = r.now()
	}
	if event.ID == "" {
		event.ID = stableID("shadow", event.TenantID, event.KBID, event.TraceID, event.ItemID, event.CreatedAt.UTC().Format(time.RFC3339Nano))
	}
	if !r.shouldSampleEvent() {
		r.dropShadowEvent("sampled_out", event, nil)
		return
	}
	if err := r.repo.RecordShadowEvent(ctx, event); err != nil {
		r.dropShadowEvent("write_failed", event, err)
	}
}

func (r *ShadowRetriever) listEligibleItems(ctx context.Context, request ShadowRetrieveRequest) ([]OptimizationItem, error) {
	if strings.TrimSpace(request.ScopedItemID) != "" {
		item, found, err := r.repo.GetOptimizationItem(ctx, request.TenantID, request.ScopedItemID)
		if err != nil {
			return nil, err
		}
		if !found || normalizeOptionalKBID(item.KBID) != normalizeOptionalKBID(request.KBID) {
			return nil, ErrScopedShadowItemMissing
		}
		if item.Status == ItemStatusStale || item.Status == ItemStatusDeprecated {
			return nil, ErrScopedShadowItemStale
		}
		if !shadowStatusEligible(item.Status) {
			return nil, ErrScopedShadowItemDisabled
		}
		return []OptimizationItem{item}, nil
	}
	statuses := []ItemStatus{
		ItemStatusVerified,
		ItemStatusShadowEnabled,
		ItemStatusRegressionPassed,
		ItemStatusPublished,
	}
	var out []OptimizationItem
	for _, status := range statuses {
		items, err := r.repo.ListOptimizationItems(ctx, OptimizationItemFilter{
			TenantID: request.TenantID,
			KBID:     normalizeOptionalKBID(request.KBID),
			Status:   status,
		})
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
	}
	return out, nil
}

func shadowStatusEligible(status ItemStatus) bool {
	return status == ItemStatusVerified ||
		status == ItemStatusShadowEnabled ||
		status == ItemStatusRegressionPassed ||
		status == ItemStatusPublished
}

func (r *ShadowRetriever) shouldSampleEvent() bool {
	if r.eventSamplingRate >= 1 {
		return true
	}
	if r.eventSamplingRate <= 0 {
		return false
	}
	if r.randFloat64 == nil {
		return true
	}
	return r.randFloat64() < r.eventSamplingRate
}

func (r *ShadowRetriever) dropShadowEvent(reason string, event ShadowRetrievalEvent, err error) {
	if r.dropMetric != nil {
		r.dropMetric.RecordShadowEventDrop(reason)
	}
	if r.onEventDropped != nil {
		r.onEventDropped(ShadowEventDrop{Reason: reason, Event: event, Err: err})
	}
}

func shadowMatchFromItem(item OptimizationItem, score float64) ShadowMatch {
	match := ShadowMatch{
		ItemID:   item.ID,
		ItemType: item.ItemType,
		Source:   ShadowSourceOptimizationLibrary,
		Score:    score,
		Metadata: shadowGuidanceMetadata(item),
	}
	if item.ItemType == ItemTypeAnswer {
		match.AnswerItem = &ShadowAnswerItem{
			SourceFingerprints: append([]SourceFingerprint(nil), item.SourceFingerprints...),
			Evidence:           append([]Evidence(nil), item.Evidence...),
			GuidanceMetadata:   shadowGuidanceMetadata(item),
		}
	}
	return match
}

func shadowGuidanceMetadata(item OptimizationItem) map[string]any {
	return map[string]any{
		"canonical_question": item.CanonicalQuestion,
		"confidence":         item.Confidence,
		"failure_type":       string(item.FailureType),
		"item_type":          string(item.ItemType),
		"recall_quality":     string(item.RecallQuality),
	}
}

func shadowScore(query, canonicalQuestion string, allowLowConfidenceFallback bool) float64 {
	normalizedQuery := normalizeShadowText(query)
	normalizedQuestion := normalizeShadowText(canonicalQuestion)
	if normalizedQuery != "" && normalizedQuestion == normalizedQuery {
		return 1
	}
	if normalizedQuery != "" && normalizedQuestion != "" &&
		(strings.Contains(normalizedQuestion, normalizedQuery) || strings.Contains(normalizedQuery, normalizedQuestion)) {
		return 0.7
	}
	if allowLowConfidenceFallback {
		return 0.1
	}
	return 0
}

func normalizeShadowText(value string) string {
	replacer := strings.NewReplacer("?", "", "!", "", ".", "", ",", "", ":", "", ";", "")
	return strings.Join(strings.Fields(replacer.Replace(strings.ToLower(strings.TrimSpace(value)))), " ")
}
