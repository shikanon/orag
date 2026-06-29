package http

import (
	"encoding/json"
	"strings"

	"github.com/shikanon/orag/internal/rag"
)

func querySSE(resp rag.QueryResponse) string {
	var b strings.Builder
	writeSSEEvent(&b, "trace", map[string]string{"trace_id": resp.TraceID})
	for _, chunk := range answerChunks(resp.Answer, 96) {
		writeSSEEvent(&b, "chunk", map[string]string{"text": chunk})
	}
	writeSSEEvent(&b, "citations", resp.Citations)
	writeSSEEvent(&b, "done", map[string]any{
		"trace_id":     resp.TraceID,
		"cache_status": resp.CacheStatus,
		"profile":      resp.Profile,
		"latency_ms":   resp.LatencyMS,
		"warnings":     resp.Warnings,
	})
	return b.String()
}

func errorSSE(code, message, traceID string) string {
	var b strings.Builder
	writeSSEEvent(&b, "error", map[string]string{
		"code":     code,
		"message":  message,
		"trace_id": traceID,
	})
	return b.String()
}

func writeSSEEvent(b *strings.Builder, event string, data any) {
	body, err := json.Marshal(data)
	if err != nil {
		body = []byte(`null`)
	}
	b.WriteString("event: ")
	b.WriteString(event)
	b.WriteString("\n")
	b.WriteString("data: ")
	b.Write(body)
	b.WriteString("\n\n")
}

func answerChunks(answer string, size int) []string {
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return []string{""}
	}
	if size <= 0 {
		size = 96
	}
	runes := []rune(answer)
	if len(runes) <= size {
		return []string{answer}
	}
	chunks := make([]string, 0, len(runes)/size+1)
	for len(runes) > 0 {
		n := size
		if len(runes) < n {
			n = len(runes)
		}
		chunks = append(chunks, string(runes[:n]))
		runes = runes[n:]
	}
	return chunks
}
