package orag

import (
	"context"
	"strings"
	"time"

	"github.com/shikanon/orag/internal/rag"
	"github.com/shikanon/orag/internal/storage/postgres"
)

type GetTraceRequest struct {
	TenantID string
	ID       string
}

type ListTracesRequest struct {
	TenantID        string
	KnowledgeBaseID string
	Profile         string
	Since           time.Time
	Until           time.Time
	HasError        *bool
	SlowMS          int64
	Limit           int
}

type TraceRecord struct {
	ID              string
	TenantID        string
	KnowledgeBaseID string
	Query           string
	Profile         string
	Answer          string
	RetrievedChunks []string
	LatencyMS       int64
	CreatedAt       time.Time
	HasError        bool
	ErrorCount      int
	NodeSpans       []TraceNodeSpan
}

type TraceNodeSpan struct {
	ID        string
	NodeName  string
	Sequence  int
	LatencyMS int64
	Error     string
	StartedAt time.Time
	EndedAt   time.Time
	CreatedAt time.Time
}

func (c *Client) GetTrace(ctx context.Context, req GetTraceRequest) (TraceRecord, bool, error) {
	if err := c.requireOpen("get_trace"); err != nil {
		return TraceRecord{}, false, err
	}
	trace, found, err := c.app.Traces.GetTraceForTenant(ctx, c.tenant(req.TenantID), strings.TrimSpace(req.ID))
	if err != nil {
		return TraceRecord{}, false, wrapError("get_trace", req.ID, req.ID, err)
	}
	if !found {
		return TraceRecord{}, false, nil
	}
	return fromTraceRecord(trace), true, nil
}

func (c *Client) ListTraces(ctx context.Context, req ListTracesRequest) ([]TraceRecord, error) {
	if err := c.requireOpen("list_traces"); err != nil {
		return nil, err
	}
	items, err := c.app.Traces.ListTraces(ctx, postgres.TraceListFilter{
		TenantID: c.tenant(req.TenantID),
		KBID:     req.KnowledgeBaseID,
		Profile:  rag.Profile(req.Profile),
		Since:    req.Since,
		Until:    req.Until,
		HasError: req.HasError,
		SlowMS:   req.SlowMS,
		Limit:    req.Limit,
	})
	if err != nil {
		return nil, wrapError("list_traces", "", "", err)
	}
	result := make([]TraceRecord, len(items))
	for index := range items {
		result[index] = fromTraceRecord(items[index])
	}
	return result, nil
}

func fromTraceRecord(item postgres.TraceRecord) TraceRecord {
	spans := make([]TraceNodeSpan, len(item.NodeSpans))
	for index := range item.NodeSpans {
		value := item.NodeSpans[index]
		spans[index] = TraceNodeSpan{ID: value.ID, NodeName: value.NodeName, Sequence: value.Sequence, LatencyMS: value.LatencyMS, Error: value.Error, StartedAt: value.StartedAt, EndedAt: value.EndedAt, CreatedAt: value.CreatedAt}
	}
	return TraceRecord{ID: item.ID, TenantID: item.TenantID, KnowledgeBaseID: item.KBID, Query: item.Query, Profile: string(item.Profile), Answer: item.Answer, RetrievedChunks: append([]string(nil), item.RetrievedChunks...), LatencyMS: item.LatencyMS, CreatedAt: item.CreatedAt, HasError: item.HasError, ErrorCount: item.ErrorCount, NodeSpans: spans}
}
