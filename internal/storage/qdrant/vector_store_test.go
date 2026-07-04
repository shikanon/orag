package qdrantstore

import (
	"context"
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

func TestKnowledgeBaseFilterIncludesTenantAndKnowledgeBase(t *testing.T) {
	filter := knowledgeBaseFilter("tenant_1", "kb_1")
	if len(filter.GetMust()) != 2 {
		t.Fatalf("filter must conditions = %d", len(filter.GetMust()))
	}
	got := map[string]string{}
	for _, cond := range filter.GetMust() {
		field := cond.GetField()
		got[field.GetKey()] = field.GetMatch().GetKeyword()
	}
	if got["tenant_id"] != "tenant_1" || got["knowledge_base_id"] != "kb_1" {
		t.Fatalf("unexpected filter: %#v", got)
	}
}

func TestDocumentSourceFilterIncludesTenantKnowledgeBaseAndSource(t *testing.T) {
	filter := documentSourceFilter("tenant_1", "kb_1", "memory://doc.md")
	if len(filter.GetMust()) != 3 {
		t.Fatalf("filter must conditions = %d", len(filter.GetMust()))
	}
	got := map[string]string{}
	for _, cond := range filter.GetMust() {
		field := cond.GetField()
		got[field.GetKey()] = field.GetMatch().GetKeyword()
	}
	if got["tenant_id"] != "tenant_1" || got["knowledge_base_id"] != "kb_1" || got["source_uri"] != "memory://doc.md" {
		t.Fatalf("unexpected filter: %#v", got)
	}
}

func TestDeleteDocumentSourceUsesTenantScopedSourceFilter(t *testing.T) {
	points := &recordingPointsClient{}
	store := VectorStore{Client: &Client{Points: points}, Collection: "chunks"}

	if err := store.DeleteDocumentSource(context.Background(), "tenant_1", "kb_1", "memory://doc.md"); err != nil {
		t.Fatal(err)
	}
	if points.deleteReq == nil {
		t.Fatal("DeleteDocumentSource did not call Qdrant delete")
	}
	if got := points.deleteReq.GetCollectionName(); got != "chunks" {
		t.Fatalf("collection = %q", got)
	}
	filter := points.deleteReq.GetPoints().GetFilter()
	if got := filterKeyword(t, filter, "tenant_id"); got != "tenant_1" {
		t.Fatalf("tenant filter = %q", got)
	}
	if got := filterKeyword(t, filter, "knowledge_base_id"); got != "kb_1" {
		t.Fatalf("knowledge base filter = %q", got)
	}
	if got := filterKeyword(t, filter, "source_uri"); got != "memory://doc.md" {
		t.Fatalf("source URI filter = %q", got)
	}
}

func TestPayloadIntegerStringFallback(t *testing.T) {
	payload := map[string]*qdrant.Value{"offset": integerValue(12)}
	if got := payloadString(payload, "offset"); got != "12" {
		t.Fatalf("payloadString = %q", got)
	}
}
