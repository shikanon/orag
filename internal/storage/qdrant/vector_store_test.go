package qdrantstore

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	qdrant "github.com/qdrant/go-client/qdrant"
	"github.com/shikanon/orag/internal/kb"
)

type fixedSearchableChunkFilter struct {
	active map[string]struct{}
	err    error
	calls  [][]string
}

func (f *fixedSearchableChunkFilter) FilterSearchableChunkIDs(_ context.Context, _, _ string, ids []string) (map[string]struct{}, error) {
	f.calls = append(f.calls, append([]string(nil), ids...))
	if f.err != nil {
		return nil, f.err
	}
	return f.active, nil
}

func TestPayloadRoundTrip(t *testing.T) {
	chunk := kb.Chunk{
		ID:              "chk_1",
		TenantID:        "tenant_1",
		KnowledgeBaseID: "kb_1",
		DocumentID:      "doc_1",
		Content:         "hello",
		ContextualText:  "This chunk introduces the greeting example.",
		SourceURI:       "memory://doc",
		Page:            3,
		Section:         "intro",
		Offset:          42,
	}
	got := chunkFromPayload(chunkPayload(chunk))
	if got.ID != chunk.ID || got.TenantID != chunk.TenantID || got.Page != chunk.Page || got.Offset != chunk.Offset {
		t.Fatalf("roundtrip mismatch: %#v", got)
	}
	if got.ContextualText != chunk.ContextualText {
		t.Fatalf("contextual text = %q, want %q", got.ContextualText, chunk.ContextualText)
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

func TestVectorStoreStoresStagedPayload(t *testing.T) {
	points := &recordingPointsClient{}
	store := VectorStore{Client: &Client{Points: points}, Collection: "chunks"}
	chunk := kb.Chunk{
		ID: "chk_1", TenantID: "tenant_1", KnowledgeBaseID: "kb_1", DocumentID: "doc_1",
		IngestionJobID: "job_1", Content: "hello", Vector: []float64{0.1, 0.2},
	}

	if err := store.Store(context.Background(), kb.Document{ID: "doc_1"}, []kb.Chunk{chunk}); err != nil {
		t.Fatal(err)
	}
	if points.upsertReq == nil || len(points.upsertReq.GetPoints()) != 1 {
		t.Fatalf("upsert request = %#v", points.upsertReq)
	}
	payload := points.upsertReq.GetPoints()[0].GetPayload()
	if payload["searchable"].GetBoolValue() {
		t.Fatal("staged point is searchable")
	}
	if got := payload["ingestion_job_id"].GetStringValue(); got != "job_1" {
		t.Fatalf("ingestion_job_id = %q", got)
	}
}

func TestPrepareActivationMarksDocumentSearchable(t *testing.T) {
	points := &recordingPointsClient{}
	store := VectorStore{Client: &Client{Points: points}, Collection: "chunks"}
	doc := kb.Document{ID: "doc_new", TenantID: "tenant_1", KnowledgeBaseID: "kb_1"}

	if err := store.PrepareActivation(context.Background(), doc, nil); err != nil {
		t.Fatal(err)
	}
	assertDocumentPayloadMutation(t, points.setPayloadReq, true, doc)
}

func TestAbortActivationMarksDocumentUnsearchable(t *testing.T) {
	points := &recordingPointsClient{}
	store := VectorStore{Client: &Client{Points: points}, Collection: "chunks"}
	doc := kb.Document{ID: "doc_new", TenantID: "tenant_1", KnowledgeBaseID: "kb_1"}

	if err := store.AbortActivation(context.Background(), doc, nil); err != nil {
		t.Fatal(err)
	}
	assertDocumentPayloadMutation(t, points.setPayloadReq, false, doc)
}

func TestCommitActivationDoesNotMutateQdrant(t *testing.T) {
	points := &recordingPointsClient{}
	store := VectorStore{Client: &Client{Points: points}, Collection: "chunks"}

	if err := store.CommitActivation(context.Background(), kb.Document{ID: "doc_new"}, nil); err != nil {
		t.Fatal(err)
	}
	if points.upsertReq != nil || points.setPayloadReq != nil || points.deleteReq != nil {
		t.Fatalf("commit mutated Qdrant: upsert=%v payload=%v delete=%v", points.upsertReq, points.setPayloadReq, points.deleteReq)
	}
}

func TestFinalizeActivationDeletesPreviousSourcePoints(t *testing.T) {
	points := &recordingPointsClient{}
	store := VectorStore{Client: &Client{Points: points}, Collection: "chunks"}

	err := store.FinalizeActivation(context.Background(), kb.Document{
		ID:              "doc_new",
		TenantID:        "tenant_1",
		KnowledgeBaseID: "kb_1",
		SourceURI:       "memory://doc.md",
	}, []kb.Chunk{{ID: "chk_new"}})
	if err != nil {
		t.Fatal(err)
	}
	if points.deleteReq == nil {
		t.Fatal("FinalizeActivation did not call Qdrant delete")
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
	if len(filter.GetMustNot()) != 1 {
		t.Fatalf("must_not conditions = %d", len(filter.GetMustNot()))
	}
	field := filter.GetMustNot()[0].GetField()
	if field.GetKey() != "document_id" || field.GetMatch().GetKeyword() != "doc_new" {
		t.Fatalf("current document exclusion = %#v", field)
	}
}

func TestRetrieveRequiresVisibilityFilter(t *testing.T) {
	points := &recordingPointsClient{}
	store := VectorStore{Client: &Client{Points: points}, Collection: "chunks"}

	results, err := store.Retrieve(context.Background(), kb.SearchRequest{TenantID: "tenant_1", KnowledgeBaseID: "kb_1", TopK: 1})
	if !errors.Is(err, ErrVisibilityFilterRequired) {
		t.Fatalf("Retrieve() error = %v, want ErrVisibilityFilterRequired", err)
	}
	if results != nil || len(points.searchReqs) != 0 {
		t.Fatalf("Retrieve() results/searches = %#v/%d, want nil/0", results, len(points.searchReqs))
	}
}

func TestRetrieveFiltersInactiveCandidates(t *testing.T) {
	states := []struct {
		id         string
		searchable *bool
	}{
		{id: "chk_true", searchable: boolPointer(true)},
		{id: "chk_false", searchable: boolPointer(false)},
		{id: "chk_missing", searchable: nil},
	}
	points := &recordingPointsClient{searchFn: func(*qdrant.SearchPoints) (*qdrant.SearchResponse, error) {
		result := make([]*qdrant.ScoredPoint, 0, len(states))
		for idx, state := range states {
			result = append(result, testScoredPoint(state.id, float32(1)-float32(idx)/10, state.searchable))
		}
		return &qdrant.SearchResponse{Result: result}, nil
	}}
	visibility := &fixedSearchableChunkFilter{active: map[string]struct{}{"chk_false": {}}}
	store := VectorStore{Client: &Client{Points: points}, Collection: "chunks", Visibility: visibility}

	results, err := store.Retrieve(context.Background(), kb.SearchRequest{TenantID: "tenant_1", KnowledgeBaseID: "kb_1", TopK: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Chunk.ID != "chk_false" || results[0].Rank != 1 {
		t.Fatalf("authorized results = %#v", results)
	}
	if len(visibility.calls) != 1 || !reflect.DeepEqual(visibility.calls[0], []string{"chk_true", "chk_false", "chk_missing"}) {
		t.Fatalf("visibility calls = %#v", visibility.calls)
	}
}

func TestRetrieveFailsClosed(t *testing.T) {
	want := errors.New("postgres visibility unavailable")
	points := &recordingPointsClient{searchFn: func(*qdrant.SearchPoints) (*qdrant.SearchResponse, error) {
		return &qdrant.SearchResponse{Result: []*qdrant.ScoredPoint{testScoredPoint("chk_1", 1, nil)}}, nil
	}}
	visibility := &fixedSearchableChunkFilter{err: want}
	store := VectorStore{Client: &Client{Points: points}, Collection: "chunks", Visibility: visibility}

	results, err := store.Retrieve(context.Background(), kb.SearchRequest{TenantID: "tenant_1", KnowledgeBaseID: "kb_1", TopK: 1})
	if !errors.Is(err, want) || results != nil {
		t.Fatalf("Retrieve() = %#v, %v; want nil preserving %v", results, err, want)
	}
}

func TestRetrievePagesForActiveResults(t *testing.T) {
	points := &recordingPointsClient{searchFn: func(req *qdrant.SearchPoints) (*qdrant.SearchResponse, error) {
		if req.GetOffset() == 0 {
			result := make([]*qdrant.ScoredPoint, 32)
			for idx := range result {
				result[idx] = testScoredPoint(fmt.Sprintf("inactive_%d", idx), 1, nil)
			}
			return &qdrant.SearchResponse{Result: result}, nil
		}
		return &qdrant.SearchResponse{Result: []*qdrant.ScoredPoint{testScoredPoint("chk_active", .5, nil)}}, nil
	}}
	visibility := &fixedSearchableChunkFilter{active: map[string]struct{}{"chk_active": {}}}
	store := VectorStore{Client: &Client{Points: points}, Collection: "chunks", Visibility: visibility}

	results, err := store.Retrieve(context.Background(), kb.SearchRequest{TenantID: "tenant_1", KnowledgeBaseID: "kb_1", TopK: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Chunk.ID != "chk_active" {
		t.Fatalf("results = %#v", results)
	}
	if len(points.searchReqs) != 2 || points.searchReqs[1].GetOffset() != 32 {
		t.Fatalf("search offsets = %#v", searchOffsets(points.searchReqs))
	}
}

func TestRetrieveStopsAtScanCap(t *testing.T) {
	points := &recordingPointsClient{searchFn: func(req *qdrant.SearchPoints) (*qdrant.SearchResponse, error) {
		result := make([]*qdrant.ScoredPoint, req.GetLimit())
		for idx := range result {
			id := fmt.Sprintf("inactive_%d", int(req.GetOffset())+idx)
			result[idx] = testScoredPoint(id, 1, nil)
		}
		return &qdrant.SearchResponse{Result: result}, nil
	}}
	visibility := &fixedSearchableChunkFilter{active: map[string]struct{}{}}
	store := VectorStore{Client: &Client{Points: points}, Collection: "chunks", Visibility: visibility}

	results, err := store.Retrieve(context.Background(), kb.SearchRequest{TenantID: "tenant_1", KnowledgeBaseID: "kb_1", TopK: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 || len(points.searchReqs) != 8 {
		t.Fatalf("results/searches = %d/%d, want 0/8", len(results), len(points.searchReqs))
	}
	if got := points.searchReqs[len(points.searchReqs)-1].GetOffset(); got != 224 {
		t.Fatalf("last offset = %d, want 224", got)
	}
}

func testScoredPoint(id string, score float32, searchable *bool) *qdrant.ScoredPoint {
	payload := map[string]*qdrant.Value{"chunk_id": stringValue(id)}
	if searchable != nil {
		payload["searchable"] = boolValue(*searchable)
	}
	return &qdrant.ScoredPoint{Payload: payload, Score: score}
}

func boolPointer(value bool) *bool { return &value }

func searchOffsets(reqs []*qdrant.SearchPoints) []uint64 {
	offsets := make([]uint64, len(reqs))
	for idx, req := range reqs {
		offsets[idx] = req.GetOffset()
	}
	return offsets
}

func assertDocumentPayloadMutation(t *testing.T, req *qdrant.SetPayloadPoints, searchable bool, doc kb.Document) {
	t.Helper()
	if req == nil {
		t.Fatal("SetPayload was not called")
	}
	if got := req.GetPayload()["searchable"].GetBoolValue(); got != searchable {
		t.Fatalf("searchable = %v, want %v", got, searchable)
	}
	filter := req.GetPointsSelector().GetFilter()
	if got := filterKeyword(t, filter, "tenant_id"); got != doc.TenantID {
		t.Fatalf("tenant filter = %q", got)
	}
	if got := filterKeyword(t, filter, "knowledge_base_id"); got != doc.KnowledgeBaseID {
		t.Fatalf("knowledge base filter = %q", got)
	}
	if got := filterKeyword(t, filter, "document_id"); got != doc.ID {
		t.Fatalf("document filter = %q", got)
	}
}

func TestPayloadIntegerStringFallback(t *testing.T) {
	payload := map[string]*qdrant.Value{"offset": integerValue(12)}
	if got := payloadString(payload, "offset"); got != "12" {
		t.Fatalf("payloadString = %q", got)
	}
}
