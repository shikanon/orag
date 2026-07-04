package rag

import (
	"context"
	"testing"
)

func TestHeuristicQueryRouterClassifiesDirectSingleAndMultiStep(t *testing.T) {
	router := HeuristicQueryRouter{DirectMaxRunes: 16, ComplexMinSignals: 2}

	tests := []struct {
		name  string
		query string
		want  QueryRoute
	}{
		{name: "direct greeting", query: "你好", want: QueryRouteDirect},
		{name: "single retrieval", query: "Qdrant 的 HNSW 索引是什么？", want: QueryRouteSingleRetrieval},
		{name: "multi step", query: "请比较 Qdrant 和 PostgreSQL FTS 的关系、优缺点，并总结如何组合使用", want: QueryRouteMultiStepRetrieval},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := router.Route(context.Background(), QueryRequest{Query: tt.query})
			if err != nil {
				t.Fatalf("Route() error = %v", err)
			}
			if got.Route != tt.want {
				t.Fatalf("route = %q, want %q; decision=%#v", got.Route, tt.want, got)
			}
			if got.Strategy != "heuristic" {
				t.Fatalf("strategy = %q, want heuristic", got.Strategy)
			}
			if got.Reason == "" {
				t.Fatalf("expected non-empty reason")
			}
		})
	}
}
