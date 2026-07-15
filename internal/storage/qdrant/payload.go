package qdrantstore

import (
	"hash/fnv"
	"strconv"

	qdrant "github.com/qdrant/go-client/qdrant"
	"github.com/shikanon/orag/internal/kb"
)

func pointID(id string) *qdrant.PointId {
	h := fnv.New64a()
	_, _ = h.Write([]byte(id))
	return &qdrant.PointId{PointIdOptions: &qdrant.PointId_Num{Num: h.Sum64()}}
}

func float32Vector(in []float64) []float32 {
	out := make([]float32, len(in))
	for i, v := range in {
		out[i] = float32(v)
	}
	return out
}

func chunkPayload(chunk kb.Chunk) map[string]*qdrant.Value {
	return map[string]*qdrant.Value{
		"chunk_id":          stringValue(chunk.ID),
		"tenant_id":         stringValue(chunk.TenantID),
		"knowledge_base_id": stringValue(chunk.KnowledgeBaseID),
		"document_id":       stringValue(chunk.DocumentID),
		"content":           stringValue(chunk.Content),
		"contextual_text":   stringValue(chunk.ContextualText),
		"source_uri":        stringValue(chunk.SourceURI),
		"page":              integerValue(int64(chunk.Page)),
		"section":           stringValue(chunk.Section),
		"offset":            integerValue(int64(chunk.Offset)),
		"ingestion_job_id":  stringValue(chunk.IngestionJobID),
		"searchable":        boolValue(false),
	}
}

func chunkFromPayload(payload map[string]*qdrant.Value) kb.Chunk {
	page, _ := strconv.Atoi(payloadString(payload, "page"))
	offset, _ := strconv.Atoi(payloadString(payload, "offset"))
	return kb.Chunk{
		ID:              payloadString(payload, "chunk_id"),
		TenantID:        payloadString(payload, "tenant_id"),
		KnowledgeBaseID: payloadString(payload, "knowledge_base_id"),
		DocumentID:      payloadString(payload, "document_id"),
		Content:         payloadString(payload, "content"),
		ContextualText:  payloadString(payload, "contextual_text"),
		SourceURI:       payloadString(payload, "source_uri"),
		Page:            page,
		Section:         payloadString(payload, "section"),
		Offset:          offset,
	}
}

func stringValue(v string) *qdrant.Value {
	return &qdrant.Value{Kind: &qdrant.Value_StringValue{StringValue: v}}
}

func integerValue(v int64) *qdrant.Value {
	return &qdrant.Value{Kind: &qdrant.Value_IntegerValue{IntegerValue: v}}
}

func boolValue(v bool) *qdrant.Value {
	return &qdrant.Value{Kind: &qdrant.Value_BoolValue{BoolValue: v}}
}

func payloadString(payload map[string]*qdrant.Value, key string) string {
	item := payload[key]
	if item == nil {
		return ""
	}
	if v := item.GetStringValue(); v != "" {
		return v
	}
	if n := item.GetIntegerValue(); n != 0 {
		return strconv.FormatInt(n, 10)
	}
	return ""
}
