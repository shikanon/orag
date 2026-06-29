package observability

import (
	"fmt"
	"strings"
	"sync/atomic"
)

type Metrics struct {
	httpRequests atomic.Int64
	ragQueries   atomic.Int64
	cacheHits    atomic.Int64
	cacheMisses  atomic.Int64
	ragLatencyMS atomic.Int64
}

func NewMetrics() *Metrics {
	return &Metrics{}
}

func (m *Metrics) IncHTTPRequests() {
	m.httpRequests.Add(1)
}

func (m *Metrics) IncRAGQuery(cacheStatus string, latencyMS int64) {
	m.ragQueries.Add(1)
	if cacheStatus == "hit" {
		m.cacheHits.Add(1)
	} else {
		m.cacheMisses.Add(1)
	}
	m.ragLatencyMS.Add(latencyMS)
}

func (m *Metrics) Render() string {
	var b strings.Builder
	b.WriteString("# HELP orag_up Service health\n# TYPE orag_up gauge\norag_up 1\n")
	b.WriteString("# HELP orag_http_requests_total Total HTTP requests\n# TYPE orag_http_requests_total counter\n")
	b.WriteString(fmt.Sprintf("orag_http_requests_total %d\n", m.httpRequests.Load()))
	b.WriteString("# HELP orag_rag_queries_total Total RAG queries\n# TYPE orag_rag_queries_total counter\n")
	b.WriteString(fmt.Sprintf("orag_rag_queries_total %d\n", m.ragQueries.Load()))
	b.WriteString("# HELP orag_rag_cache_hits_total Total semantic cache hits\n# TYPE orag_rag_cache_hits_total counter\n")
	b.WriteString(fmt.Sprintf("orag_rag_cache_hits_total %d\n", m.cacheHits.Load()))
	b.WriteString("# HELP orag_rag_cache_misses_total Total semantic cache misses\n# TYPE orag_rag_cache_misses_total counter\n")
	b.WriteString(fmt.Sprintf("orag_rag_cache_misses_total %d\n", m.cacheMisses.Load()))
	b.WriteString("# HELP orag_rag_query_latency_ms_sum Sum of RAG query latency in milliseconds\n# TYPE orag_rag_query_latency_ms_sum counter\n")
	b.WriteString(fmt.Sprintf("orag_rag_query_latency_ms_sum %d\n", m.ragLatencyMS.Load()))
	return b.String()
}
