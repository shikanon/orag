package observability

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

type Metrics struct {
	httpRequests atomic.Int64
	ragQueries   atomic.Int64
	cacheHits    atomic.Int64
	cacheMisses  atomic.Int64
	ragLatencyMS atomic.Int64

	mu             sync.Mutex
	httpByLabel    map[httpMetricLabels]int64
	httpErrors     map[httpMetricLabels]int64
	ragByLabel     map[ragMetricLabels]int64
	ragErrors      map[ragErrorLabels]int64
	ragLatencyByLB map[ragMetricLabels]*latencyHistogram
}

type httpMetricLabels struct {
	Method      string
	Route       string
	Status      string
	StatusClass string
}

type ragMetricLabels struct {
	Profile     string
	CacheStatus string
	Outcome     string
}

type ragErrorLabels struct {
	Profile   string
	ErrorCode string
}

type latencyHistogram struct {
	Count   int64
	Sum     int64
	Buckets map[int64]int64
}

func NewMetrics() *Metrics {
	return &Metrics{
		httpByLabel:    make(map[httpMetricLabels]int64),
		httpErrors:     make(map[httpMetricLabels]int64),
		ragByLabel:     make(map[ragMetricLabels]int64),
		ragErrors:      make(map[ragErrorLabels]int64),
		ragLatencyByLB: make(map[ragMetricLabels]*latencyHistogram),
	}
}

func (m *Metrics) IncHTTPRequests() {
	m.httpRequests.Add(1)
}

func (m *Metrics) ObserveHTTPRequest(method, route string, status int) {
	m.httpRequests.Add(1)
	labels := httpMetricLabels{
		Method:      normalizeMethod(method),
		Route:       normalizeRoute(route),
		Status:      normalizeStatus(status),
		StatusClass: statusClass(status),
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.httpByLabel[labels]++
	if status >= 400 {
		m.httpErrors[labels]++
	}
}

func (m *Metrics) IncRAGQuery(cacheStatus string, latencyMS int64) {
	m.ObserveRAGQuery("default", cacheStatus, "success", latencyMS)
}

func (m *Metrics) ObserveRAGQuery(profile, cacheStatus, outcome string, latencyMS int64) {
	cacheStatus = normalizeCacheStatus(cacheStatus)
	outcome = normalizeOutcome(outcome)
	labels := ragMetricLabels{
		Profile:     normalizeProfile(profile),
		CacheStatus: cacheStatus,
		Outcome:     outcome,
	}
	m.ragQueries.Add(1)
	if cacheStatus == "hit" {
		m.cacheHits.Add(1)
	} else if cacheStatus == "miss" {
		m.cacheMisses.Add(1)
	}
	m.ragLatencyMS.Add(latencyMS)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ragByLabel[labels]++
	hist := m.ragLatencyByLB[labels]
	if hist == nil {
		hist = &latencyHistogram{Buckets: make(map[int64]int64, len(ragLatencyBucketsMS))}
		m.ragLatencyByLB[labels] = hist
	}
	hist.Count++
	hist.Sum += latencyMS
	for _, upper := range ragLatencyBucketsMS {
		if latencyMS <= upper {
			hist.Buckets[upper]++
		}
	}
}

func (m *Metrics) IncRAGError(profile, errorCode string) {
	labels := ragErrorLabels{
		Profile:   normalizeProfile(profile),
		ErrorCode: normalizeErrorCode(errorCode),
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ragErrors[labels]++
}

func (m *Metrics) Render() string {
	var b strings.Builder
	b.WriteString("# HELP orag_up Service health\n# TYPE orag_up gauge\norag_up 1\n")
	b.WriteString("# HELP orag_http_requests_total Total HTTP requests\n# TYPE orag_http_requests_total counter\n")
	b.WriteString(fmt.Sprintf("orag_http_requests_total %d\n", m.httpRequests.Load()))
	m.renderHTTPCounters(&b)
	b.WriteString("# HELP orag_rag_queries_total Total RAG queries\n# TYPE orag_rag_queries_total counter\n")
	b.WriteString(fmt.Sprintf("orag_rag_queries_total %d\n", m.ragQueries.Load()))
	m.renderRAGCounters(&b)
	b.WriteString("# HELP orag_rag_cache_hits_total Total semantic cache hits\n# TYPE orag_rag_cache_hits_total counter\n")
	b.WriteString(fmt.Sprintf("orag_rag_cache_hits_total %d\n", m.cacheHits.Load()))
	b.WriteString("# HELP orag_rag_cache_misses_total Total semantic cache misses\n# TYPE orag_rag_cache_misses_total counter\n")
	b.WriteString(fmt.Sprintf("orag_rag_cache_misses_total %d\n", m.cacheMisses.Load()))
	b.WriteString(fmt.Sprintf("orag_rag_query_latency_ms_sum %d\n", m.ragLatencyMS.Load()))
	return b.String()
}

func (m *Metrics) renderHTTPCounters(b *strings.Builder) {
	m.mu.Lock()
	httpByLabel := copyHTTPMap(m.httpByLabel)
	httpErrors := copyHTTPMap(m.httpErrors)
	m.mu.Unlock()

	keys := sortedHTTPKeys(httpByLabel)
	for _, key := range keys {
		b.WriteString(fmt.Sprintf("orag_http_requests_total{method=%q,route=%q,status=%q,status_class=%q} %d\n",
			key.Method, key.Route, key.Status, key.StatusClass, httpByLabel[key]))
	}
	b.WriteString("# HELP orag_http_errors_total Total HTTP error responses\n# TYPE orag_http_errors_total counter\n")
	for _, key := range sortedHTTPKeys(httpErrors) {
		b.WriteString(fmt.Sprintf("orag_http_errors_total{method=%q,route=%q,status=%q,status_class=%q} %d\n",
			key.Method, key.Route, key.Status, key.StatusClass, httpErrors[key]))
	}
}

func (m *Metrics) renderRAGCounters(b *strings.Builder) {
	m.mu.Lock()
	ragByLabel := copyRAGMap(m.ragByLabel)
	ragErrors := copyRAGErrorMap(m.ragErrors)
	latencies := copyLatencyMap(m.ragLatencyByLB)
	m.mu.Unlock()

	for _, key := range sortedRAGKeys(ragByLabel) {
		b.WriteString(fmt.Sprintf("orag_rag_queries_total{profile=%q,cache_status=%q,outcome=%q} %d\n",
			key.Profile, key.CacheStatus, key.Outcome, ragByLabel[key]))
	}
	b.WriteString("# HELP orag_rag_errors_total Total failed RAG queries\n# TYPE orag_rag_errors_total counter\n")
	for _, key := range sortedRAGErrorKeys(ragErrors) {
		b.WriteString(fmt.Sprintf("orag_rag_errors_total{profile=%q,error_code=%q} %d\n",
			key.Profile, key.ErrorCode, ragErrors[key]))
	}
	b.WriteString("# HELP orag_rag_query_latency_ms RAG query latency in milliseconds\n# TYPE orag_rag_query_latency_ms histogram\n")
	for _, key := range sortedRAGKeysFromLatency(latencies) {
		hist := latencies[key]
		for _, upper := range ragLatencyBucketsMS {
			b.WriteString(fmt.Sprintf("orag_rag_query_latency_ms_bucket{profile=%q,cache_status=%q,outcome=%q,le=%q} %d\n",
				key.Profile, key.CacheStatus, key.Outcome, fmt.Sprintf("%d", upper), hist.Buckets[upper]))
		}
		b.WriteString(fmt.Sprintf("orag_rag_query_latency_ms_bucket{profile=%q,cache_status=%q,outcome=%q,le=%q} %d\n",
			key.Profile, key.CacheStatus, key.Outcome, "+Inf", hist.Count))
		b.WriteString(fmt.Sprintf("orag_rag_query_latency_ms_sum{profile=%q,cache_status=%q,outcome=%q} %d\n",
			key.Profile, key.CacheStatus, key.Outcome, hist.Sum))
		b.WriteString(fmt.Sprintf("orag_rag_query_latency_ms_count{profile=%q,cache_status=%q,outcome=%q} %d\n",
			key.Profile, key.CacheStatus, key.Outcome, hist.Count))
	}
}

var ragLatencyBucketsMS = []int64{50, 100, 250, 500, 1000, 2500, 5000, 10000}

func normalizeMethod(method string) string {
	method = strings.ToUpper(strings.TrimSpace(method))
	switch method {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "HEAD":
		return method
	default:
		return "OTHER"
	}
}

func normalizeRoute(route string) string {
	route = strings.TrimSpace(route)
	if route == "" {
		return "unknown"
	}
	if strings.Contains(route, "?") {
		route = strings.Split(route, "?")[0]
	}
	return route
}

func normalizeStatus(status int) string {
	if status < 100 || status > 599 {
		return "unknown"
	}
	return fmt.Sprintf("%d", status)
}

func statusClass(status int) string {
	if status < 100 || status > 599 {
		return "unknown"
	}
	return fmt.Sprintf("%dxx", status/100)
}

func normalizeProfile(profile string) string {
	profile = strings.TrimSpace(profile)
	switch profile {
	case "realtime", "high_precision":
		return profile
	case "":
		return "default"
	default:
		return "other"
	}
}

func normalizeCacheStatus(cacheStatus string) string {
	switch strings.TrimSpace(cacheStatus) {
	case "hit":
		return "hit"
	case "miss":
		return "miss"
	default:
		return "unknown"
	}
}

func normalizeOutcome(outcome string) string {
	if strings.TrimSpace(outcome) == "success" {
		return "success"
	}
	return "error"
}

func normalizeErrorCode(errorCode string) string {
	switch strings.TrimSpace(strings.ToLower(errorCode)) {
	case "query_failed":
		return "query_failed"
	case "":
		return "unknown"
	default:
		return "other"
	}
}

func copyHTTPMap(src map[httpMetricLabels]int64) map[httpMetricLabels]int64 {
	dst := make(map[httpMetricLabels]int64, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func copyRAGMap(src map[ragMetricLabels]int64) map[ragMetricLabels]int64 {
	dst := make(map[ragMetricLabels]int64, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func copyRAGErrorMap(src map[ragErrorLabels]int64) map[ragErrorLabels]int64 {
	dst := make(map[ragErrorLabels]int64, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func copyLatencyMap(src map[ragMetricLabels]*latencyHistogram) map[ragMetricLabels]*latencyHistogram {
	dst := make(map[ragMetricLabels]*latencyHistogram, len(src))
	for k, v := range src {
		buckets := make(map[int64]int64, len(v.Buckets))
		for bucket, count := range v.Buckets {
			buckets[bucket] = count
		}
		dst[k] = &latencyHistogram{Count: v.Count, Sum: v.Sum, Buckets: buckets}
	}
	return dst
}

func sortedHTTPKeys(values map[httpMetricLabels]int64) []httpMetricLabels {
	keys := make([]httpMetricLabels, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return strings.Join([]string{keys[i].Method, keys[i].Route, keys[i].Status}, "\xff") <
			strings.Join([]string{keys[j].Method, keys[j].Route, keys[j].Status}, "\xff")
	})
	return keys
}

func sortedRAGKeys(values map[ragMetricLabels]int64) []ragMetricLabels {
	keys := make([]ragMetricLabels, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sortRAGKeys(keys)
	return keys
}

func sortedRAGKeysFromLatency(values map[ragMetricLabels]*latencyHistogram) []ragMetricLabels {
	keys := make([]ragMetricLabels, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sortRAGKeys(keys)
	return keys
}

func sortRAGKeys(keys []ragMetricLabels) {
	sort.Slice(keys, func(i, j int) bool {
		return strings.Join([]string{keys[i].Profile, keys[i].CacheStatus, keys[i].Outcome}, "\xff") <
			strings.Join([]string{keys[j].Profile, keys[j].CacheStatus, keys[j].Outcome}, "\xff")
	})
}

func sortedRAGErrorKeys(values map[ragErrorLabels]int64) []ragErrorLabels {
	keys := make([]ragErrorLabels, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return strings.Join([]string{keys[i].Profile, keys[i].ErrorCode}, "\xff") <
			strings.Join([]string{keys[j].Profile, keys[j].ErrorCode}, "\xff")
	})
	return keys
}
