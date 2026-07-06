package qdrantstore

import (
	"context"
	"strings"
	"testing"

	qdrant "github.com/qdrant/go-client/qdrant"
	"google.golang.org/grpc"
)

func TestEnsureCollectionValidatesCompatibleExistingCollection(t *testing.T) {
	collections := &fakeCollectionsClient{
		exists: true,
		info:   testCollectionInfo(1024, qdrant.Distance_Cosine),
	}
	client := &Client{Collections: collections}

	if err := client.EnsureCollection(context.Background(), "chunks", 1024); err != nil {
		t.Fatalf("EnsureCollection() error = %v", err)
	}
	if collections.createReq != nil {
		t.Fatalf("compatible existing collection should not be created: %#v", collections.createReq)
	}
	if collections.getReq == nil || collections.getReq.GetCollectionName() != "chunks" {
		t.Fatalf("Get request = %#v", collections.getReq)
	}
}

func TestEnsureCollectionFailsOnMismatchedExistingSize(t *testing.T) {
	collections := &fakeCollectionsClient{
		exists: true,
		info:   testCollectionInfo(768, qdrant.Distance_Cosine),
	}
	client := &Client{Collections: collections}

	err := client.EnsureCollection(context.Background(), "chunks", 1024)
	if err == nil {
		t.Fatal("EnsureCollection() error = nil, want size mismatch")
	}
	assertErrorContains(t, err, "qdrant collection \"chunks\" vector config mismatch", "size=768", "expected single unnamed vector size=1024 distance=Cosine")
	if collections.createReq != nil {
		t.Fatalf("mismatched existing collection should not be created: %#v", collections.createReq)
	}
}

func TestEnsureCollectionFailsOnMismatchedExistingDistance(t *testing.T) {
	collections := &fakeCollectionsClient{
		exists: true,
		info:   testCollectionInfo(1024, qdrant.Distance_Dot),
	}
	client := &Client{Collections: collections}

	err := client.EnsureCollection(context.Background(), "chunks", 1024)
	if err == nil {
		t.Fatal("EnsureCollection() error = nil, want distance mismatch")
	}
	assertErrorContains(t, err, "qdrant collection \"chunks\" vector config mismatch", "distance=Dot", "expected single unnamed vector size=1024 distance=Cosine")
	if collections.createReq != nil {
		t.Fatalf("mismatched existing collection should not be created: %#v", collections.createReq)
	}
}

func TestEnsureCollectionCreatesMissingCollectionWithExpectedVectorConfig(t *testing.T) {
	collections := &fakeCollectionsClient{exists: false}
	client := &Client{Collections: collections}

	if err := client.EnsureCollection(context.Background(), "chunks", 1536); err != nil {
		t.Fatalf("EnsureCollection() error = %v", err)
	}
	if collections.getReq != nil {
		t.Fatalf("missing collection should not fetch metadata before create: %#v", collections.getReq)
	}
	if collections.createReq == nil {
		t.Fatal("Create was not called")
	}
	if got := collections.createReq.GetCollectionName(); got != "chunks" {
		t.Fatalf("collection name = %q", got)
	}
	params := collections.createReq.GetVectorsConfig().GetParams()
	if params.GetSize() != 1536 {
		t.Fatalf("vector size = %d", params.GetSize())
	}
	if params.GetDistance() != qdrant.Distance_Cosine {
		t.Fatalf("vector distance = %s", params.GetDistance())
	}
}

func testCollectionInfo(size uint64, distance qdrant.Distance) *qdrant.CollectionInfo {
	return &qdrant.CollectionInfo{Config: &qdrant.CollectionConfig{Params: &qdrant.CollectionParams{
		VectorsConfig: &qdrant.VectorsConfig{Config: &qdrant.VectorsConfig_Params{
			Params: &qdrant.VectorParams{
				Size:     size,
				Distance: distance,
			},
		}},
	}}}
}

func assertErrorContains(t *testing.T, err error, parts ...string) {
	t.Helper()
	msg := err.Error()
	for _, part := range parts {
		if !strings.Contains(msg, part) {
			t.Fatalf("error %q does not contain %q", msg, part)
		}
	}
}

type fakeCollectionsClient struct {
	exists    bool
	info      *qdrant.CollectionInfo
	existsErr error
	getErr    error
	createErr error
	existsReq *qdrant.CollectionExistsRequest
	getReq    *qdrant.GetCollectionInfoRequest
	createReq *qdrant.CreateCollection
}

func (c *fakeCollectionsClient) CollectionExists(_ context.Context, req *qdrant.CollectionExistsRequest, _ ...grpc.CallOption) (*qdrant.CollectionExistsResponse, error) {
	c.existsReq = req
	if c.existsErr != nil {
		return nil, c.existsErr
	}
	return &qdrant.CollectionExistsResponse{Result: &qdrant.CollectionExists{Exists: c.exists}}, nil
}

func (c *fakeCollectionsClient) Get(_ context.Context, req *qdrant.GetCollectionInfoRequest, _ ...grpc.CallOption) (*qdrant.GetCollectionInfoResponse, error) {
	c.getReq = req
	if c.getErr != nil {
		return nil, c.getErr
	}
	return &qdrant.GetCollectionInfoResponse{Result: c.info}, nil
}

func (c *fakeCollectionsClient) Create(_ context.Context, req *qdrant.CreateCollection, _ ...grpc.CallOption) (*qdrant.CollectionOperationResponse, error) {
	c.createReq = req
	if c.createErr != nil {
		return nil, c.createErr
	}
	return &qdrant.CollectionOperationResponse{Result: true}, nil
}
