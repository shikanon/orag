package offlineknowledge

import (
	"context"
	"sort"
	"strings"
	"time"
	"unicode"
)

type HistoryTraceFilter struct {
	TenantID string
	KBID     string
	Since    time.Time
	Until    time.Time
	Limit    int
}

type HistoryTrace struct {
	TenantID        string
	KBID            string
	TraceID         string
	Query           string
	Answer          string
	RetrievedChunks []string
	Latency         time.Duration
	HasError        bool
	Error           string
	CreatedAt       time.Time
}

type HistoryTraceRepository interface {
	ListHistoryTraces(ctx context.Context, filter HistoryTraceFilter) ([]HistoryTrace, error)
}

type NegativeFeedbackFilter struct {
	TenantID string
	KBID     string
	Since    time.Time
	Until    time.Time
	TraceIDs []string
	Limit    int
}

type NegativeFeedback struct {
	ID        string
	TenantID  string
	KBID      string
	TraceID   string
	Query     string
	Reason    string
	CreatedAt time.Time
}

type NegativeFeedbackSource interface {
	ListNegativeFeedback(ctx context.Context, filter NegativeFeedbackFilter) ([]NegativeFeedback, error)
}

type NegativeFeedbackRepository interface {
	NegativeFeedbackSource
	AddNegativeFeedback(ctx context.Context, item NegativeFeedback) error
}

type TraceHistoryExtractor struct {
	Traces           HistoryTraceRepository
	NegativeFeedback NegativeFeedbackSource
}

func NewTraceHistoryExtractor(traces HistoryTraceRepository, feedback NegativeFeedbackSource) *TraceHistoryExtractor {
	return &TraceHistoryExtractor{Traces: traces, NegativeFeedback: feedback}
}

func (e *TraceHistoryExtractor) ExtractHistory(ctx context.Context, request HistoryRequest) ([]HistorySignal, error) {
	if e == nil || e.Traces == nil {
		return nil, ErrHistorySourceRequired
	}
	traces, err := e.Traces.ListHistoryTraces(ctx, HistoryTraceFilter{
		TenantID: request.TenantID,
		KBID:     normalizeOptionalKBID(request.KBID),
		Since:    request.Window.Start,
		Until:    request.Window.End,
		Limit:    request.Limit,
	})
	if err != nil {
		return nil, err
	}
	feedback, err := e.listFeedback(ctx, request, traces)
	if err != nil {
		return nil, err
	}
	feedbackByTrace := make(map[string]NegativeFeedback, len(feedback))
	feedbackByQuestion := make(map[string]NegativeFeedback, len(feedback))
	for _, item := range feedback {
		if item.TraceID != "" {
			feedbackByTrace[item.TraceID] = item
		}
		if item.Query != "" {
			feedbackByQuestion[normalizeQuestionText(item.Query)] = item
		}
	}

	signals := make([]HistorySignal, 0, len(traces)+len(feedback))
	seenFeedback := make(map[string]struct{}, len(feedback))
	for _, trace := range traces {
		signal := HistorySignal{
			TenantID:        trace.TenantID,
			KBID:            trace.KBID,
			Query:           trace.Query,
			TraceID:         trace.TraceID,
			Answer:          trace.Answer,
			RetrievedChunks: append([]string(nil), trace.RetrievedChunks...),
			Latency:         trace.Latency,
			HasError:        trace.HasError,
			Error:           trace.Error,
			Metadata:        map[string]any{"created_at": trace.CreatedAt},
		}
		if item, ok := feedbackByTrace[trace.TraceID]; ok {
			applyNegativeFeedback(&signal, item)
			seenFeedback[feedbackKey(item)] = struct{}{}
		} else if item, ok := feedbackByQuestion[normalizeQuestionText(trace.Query)]; ok {
			applyNegativeFeedback(&signal, item)
			seenFeedback[feedbackKey(item)] = struct{}{}
		}
		signals = append(signals, signal)
	}
	for _, item := range feedback {
		if _, ok := seenFeedback[feedbackKey(item)]; ok {
			continue
		}
		signals = append(signals, HistorySignal{
			TenantID:                 item.TenantID,
			KBID:                     item.KBID,
			Query:                    item.Query,
			TraceID:                  item.TraceID,
			ExplicitNegativeFeedback: true,
			NegativeFeedbackReason:   item.Reason,
			Metadata:                 map[string]any{"feedback_created_at": item.CreatedAt},
		})
	}
	return signals, nil
}

func (e *TraceHistoryExtractor) listFeedback(ctx context.Context, request HistoryRequest, traces []HistoryTrace) ([]NegativeFeedback, error) {
	if e.NegativeFeedback == nil {
		return nil, nil
	}
	traceIDs := make([]string, 0, len(traces))
	for _, trace := range traces {
		if trace.TraceID != "" {
			traceIDs = append(traceIDs, trace.TraceID)
		}
	}
	return e.NegativeFeedback.ListNegativeFeedback(ctx, NegativeFeedbackFilter{
		TenantID: request.TenantID,
		KBID:     normalizeOptionalKBID(request.KBID),
		Since:    request.Window.Start,
		Until:    request.Window.End,
		TraceIDs: traceIDs,
		Limit:    request.Limit,
	})
}

func applyNegativeFeedback(signal *HistorySignal, item NegativeFeedback) {
	signal.ExplicitNegativeFeedback = true
	signal.NegativeFeedbackReason = item.Reason
	if signal.Query == "" {
		signal.Query = item.Query
	}
	if signal.TraceID == "" {
		signal.TraceID = item.TraceID
	}
	if signal.Metadata == nil {
		signal.Metadata = map[string]any{}
	}
	signal.Metadata["feedback_created_at"] = item.CreatedAt
}

func feedbackKey(item NegativeFeedback) string {
	return item.TraceID + "\x00" + normalizeQuestionText(item.Query)
}

type DeterministicQuestionClusterer struct {
	Now func() time.Time
}

func NewDeterministicQuestionClusterer(now func() time.Time) *DeterministicQuestionClusterer {
	return &DeterministicQuestionClusterer{Now: now}
}

func (c *DeterministicQuestionClusterer) ClusterQuestions(_ context.Context, request ClusterRequest) ([]QuestionCluster, error) {
	now := time.Now
	if c != nil && c.Now != nil {
		now = c.Now
	}
	grouped := make(map[string]*QuestionCluster)
	for _, signal := range request.Signals {
		normalized := normalizeQuestionText(signal.Query)
		if normalized == "" {
			continue
		}
		hash := shortHash(normalized)
		cluster := grouped[hash]
		if cluster == nil {
			cluster = &QuestionCluster{
				ID:                 stableID("cluster", request.Run.TenantID, request.Run.KBID, hash),
				TenantID:           request.Run.TenantID,
				RunID:              request.Run.ID,
				KBID:               request.Run.KBID,
				CanonicalQuestion:  strings.TrimSpace(signal.Query),
				NormalizedQuestion: normalized,
				QuestionHash:       hash,
				CreatedAt:          now(),
			}
			grouped[hash] = cluster
		}
		cluster.OccurrenceCount++
		cluster.SampleQuestions = appendUnique(cluster.SampleQuestions, strings.TrimSpace(signal.Query))
		cluster.TraceIDs = appendUnique(cluster.TraceIDs, signal.TraceID)
		cluster.LongTail = cluster.LongTail || signal.LongTail
	}
	out := make([]QuestionCluster, 0, len(grouped))
	for _, cluster := range grouped {
		cluster.LongTail = cluster.LongTail || cluster.OccurrenceCount == 1
		out = append(out, *cluster)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].LongTail != out[j].LongTail {
			return !out[i].LongTail
		}
		if out[i].OccurrenceCount != out[j].OccurrenceCount {
			return out[i].OccurrenceCount > out[j].OccurrenceCount
		}
		return out[i].QuestionHash < out[j].QuestionHash
	})
	return out, nil
}

func normalizeQuestionText(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	var b strings.Builder
	lastSpace := false
	for _, r := range value {
		switch {
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			b.WriteRune(r)
			lastSpace = false
		case unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r):
			if !lastSpace {
				b.WriteByte(' ')
				lastSpace = true
			}
		}
	}
	return strings.TrimSpace(b.String())
}

func appendUnique(values []string, value string) []string {
	if value == "" {
		return values
	}
	for _, current := range values {
		if current == value {
			return values
		}
	}
	return append(values, value)
}
