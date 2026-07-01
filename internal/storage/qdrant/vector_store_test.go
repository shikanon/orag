package qdrantstore

import (
	"context"
	"testing"

	qdrant "github.com/qdrant/go-client/qdrant"
	"github.com/shikanon/orag/internal/kb"
	"google.golang.org/grpc"
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

func TestDeleteKnowledgeBasePointsRequestFiltersTenantAndKnowledgeBase(t *testing.T) {
	req := deleteKnowledgeBasePointsRequest("vectors", "tenant_1", "kb_1")
	if req.GetCollectionName() != "vectors" {
		t.Fatalf("collection = %q", req.GetCollectionName())
	}
	if !req.GetWait() {
		t.Fatal("wait = false, want true")
	}
	filter := req.GetPoints().GetFilter()
	if filter == nil {
		t.Fatal("points selector is not a filter")
	}
	assertFilterKeyword(t, filter, "tenant_id", "tenant_1")
	assertFilterKeyword(t, filter, "knowledge_base_id", "kb_1")
}

func TestPurgeKnowledgeBaseDeletesVectorAndSemanticCacheCollections(t *testing.T) {
	points := &recordingPointsClient{}
	client := &Client{Points: points}

	if err := PurgeKnowledgeBase(context.Background(), client, "vectors", "cache", "tenant_1", "kb_1"); err != nil {
		t.Fatalf("PurgeKnowledgeBase() error = %v", err)
	}
	if len(points.deletes) != 2 {
		t.Fatalf("delete requests = %d, want 2", len(points.deletes))
	}
	for i, collection := range []string{"vectors", "cache"} {
		req := points.deletes[i]
		if req.GetCollectionName() != collection {
			t.Fatalf("delete[%d] collection = %q, want %q", i, req.GetCollectionName(), collection)
		}
		filter := req.GetPoints().GetFilter()
		assertFilterKeyword(t, filter, "tenant_id", "tenant_1")
		assertFilterKeyword(t, filter, "knowledge_base_id", "kb_1")
	}
}

func TestPayloadIntegerStringFallback(t *testing.T) {
	payload := map[string]*qdrant.Value{"offset": integerValue(12)}
	if got := payloadString(payload, "offset"); got != "12" {
		t.Fatalf("payloadString = %q", got)
	}
}

type recordingPointsClient struct {
	qdrant.PointsClient
	deletes []*qdrant.DeletePoints
}

func (c *recordingPointsClient) Delete(_ context.Context, req *qdrant.DeletePoints, _ ...grpc.CallOption) (*qdrant.PointsOperationResponse, error) {
	c.deletes = append(c.deletes, req)
	return &qdrant.PointsOperationResponse{}, nil
}

func assertFilterKeyword(t *testing.T, filter *qdrant.Filter, key, value string) {
	t.Helper()
	for _, cond := range filter.GetMust() {
		field := cond.GetField()
		if field.GetKey() == key && field.GetMatch().GetKeyword() == value {
			return
		}
	}
	t.Fatalf("filter missing %s=%q: %#v", key, value, filter.GetMust())
}
