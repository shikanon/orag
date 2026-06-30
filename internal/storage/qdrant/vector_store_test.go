package qdrantstore

import (
	"testing"

	qdrant "github.com/qdrant/go-client/qdrant"
	"github.com/shikanon/orag/internal/kb"
)

func TestPayloadRoundTrip(t *testing.T) {
	chunk := kb.Chunk{
		ID:              "chk_1",
		TenantID:        "tenant_1",
		KnowledgeBaseID: "kb_1",
		DocumentID:      "doc_1",
		Content:         "hello",
		SourceURI:       "memory://doc",
		Page:            3,
		Section:         "intro",
		Offset:          42,
	}
	got := chunkFromPayload(chunkPayload(chunk))
	if got.ID != chunk.ID || got.TenantID != chunk.TenantID || got.Page != chunk.Page || got.Offset != chunk.Offset {
		t.Fatalf("roundtrip mismatch: %#v", got)
	}
}

func TestPointIDStable(t *testing.T) {
	a := pointID("chk_1").GetNum()
	b := pointID("chk_1").GetNum()
	c := pointID("chk_2").GetNum()
	if a != b {
		t.Fatalf("point id is not stable")
	}
	if a == c {
		t.Fatalf("different ids should not hash to the same value in this test")
	}
}

func TestFloat32Vector(t *testing.T) {
	got := float32Vector([]float64{1.25, -0.5})
	if len(got) != 2 || got[0] != float32(1.25) || got[1] != float32(-0.5) {
		t.Fatalf("unexpected vector: %#v", got)
	}
}

func TestMatchKeywordBuildsFieldCondition(t *testing.T) {
	cond := matchKeyword("tenant_id", "tenant_1")
	field := cond.GetField()
	if field.GetKey() != "tenant_id" {
		t.Fatalf("key = %q", field.GetKey())
	}
	if field.GetMatch().GetKeyword() != "tenant_1" {
		t.Fatalf("keyword = %q", field.GetMatch().GetKeyword())
	}
}

func TestDeleteKnowledgeBasePointsRequestUsesTenantAndKBFilter(t *testing.T) {
	req := deleteKnowledgeBasePointsRequest("orag_chunks", "tenant_1", "kb_1")
	if req.GetCollectionName() != "orag_chunks" {
		t.Fatalf("collection = %q", req.GetCollectionName())
	}
	if req.Wait == nil || !req.GetWait() {
		t.Fatalf("wait = %v, want true", req.Wait)
	}
	filter := req.GetPoints().GetFilter()
	if filter == nil {
		t.Fatal("delete request does not use a filter selector")
	}
	assertFilterMatches(t, filter, map[string]string{
		"tenant_id":         "tenant_1",
		"knowledge_base_id": "kb_1",
	})
}

func TestPayloadIntegerStringFallback(t *testing.T) {
	payload := map[string]*qdrant.Value{"offset": integerValue(12)}
	if got := payloadString(payload, "offset"); got != "12" {
		t.Fatalf("payloadString = %q", got)
	}
}

func assertFilterMatches(t *testing.T, filter *qdrant.Filter, want map[string]string) {
	t.Helper()
	got := map[string]string{}
	for _, cond := range filter.GetMust() {
		field := cond.GetField()
		got[field.GetKey()] = field.GetMatch().GetKeyword()
	}
	for key, value := range want {
		if got[key] != value {
			t.Fatalf("filter[%q] = %q, want %q; full filter = %#v", key, got[key], value, got)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("filter = %#v, want only %#v", got, want)
	}
}
