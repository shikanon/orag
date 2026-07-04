package rag

import (
	"context"
	"strings"
	"unicode/utf8"
)

type QueryRouter interface {
	Route(ctx context.Context, req QueryRequest) (RouteDecision, error)
}

type HeuristicQueryRouter struct {
	DirectMaxRunes    int
	ComplexMinSignals int
}

func (r HeuristicQueryRouter) Route(_ context.Context, req QueryRequest) (RouteDecision, error) {
	query := strings.TrimSpace(req.Query)
	directMaxRunes := defaultPositive(r.DirectMaxRunes, 16)
	complexMinSignals := defaultPositive(r.ComplexMinSignals, 2)
	signals := routeSignals(query)
	if isDirectQuery(query, directMaxRunes, signals) {
		return RouteDecision{
			Route:    QueryRouteDirect,
			Reason:   "short conversational query without retrieval signals",
			Strategy: "heuristic",
			Signals:  signals,
		}, nil
	}
	if len(signals) >= complexMinSignals {
		return RouteDecision{
			Route:    QueryRouteMultiStepRetrieval,
			Reason:   "query contains multiple synthesis or comparison signals",
			Strategy: "heuristic",
			Signals:  signals,
		}, nil
	}
	return RouteDecision{
		Route:    QueryRouteSingleRetrieval,
		Reason:   "query needs one retrieval pass",
		Strategy: "heuristic",
		Signals:  signals,
	}, nil
}

func DefaultHeuristicQueryRouter() HeuristicQueryRouter {
	return HeuristicQueryRouter{DirectMaxRunes: 16, ComplexMinSignals: 2}
}

func routeSignals(query string) []string {
	lower := strings.ToLower(strings.TrimSpace(query))
	if lower == "" {
		return nil
	}
	candidates := []struct {
		name  string
		terms []string
	}{
		{name: "compare", terms: []string{"比较", "对比", "区别", "优缺点", "vs", "versus"}},
		{name: "summarize", terms: []string{"总结", "归纳", "综合", "梳理"}},
		{name: "causal", terms: []string{"为什么", "原因", "影响", "导致"}},
		{name: "relation", terms: []string{"关系", "关联", "之间", "依赖"}},
		{name: "procedure", terms: []string{"如何", "步骤", "方案", "规划"}},
		{name: "multiple", terms: []string{"多个", "分别", "同时", "以及", "并且"}},
	}
	var signals []string
	for _, candidate := range candidates {
		for _, term := range candidate.terms {
			if strings.Contains(lower, term) {
				signals = append(signals, candidate.name)
				break
			}
		}
	}
	if utf8.RuneCountInString(lower) >= 80 {
		signals = append(signals, "long_query")
	}
	return signals
}

func isDirectQuery(query string, directMaxRunes int, signals []string) bool {
	if query == "" || utf8.RuneCountInString(query) > directMaxRunes || len(signals) > 0 {
		return false
	}
	lower := strings.ToLower(query)
	for _, term := range []string{"你好", "您好", "谢谢", "你是谁", "hello", "hi", "thanks"} {
		if strings.Contains(lower, term) {
			return true
		}
	}
	return false
}

func defaultPositive(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}
