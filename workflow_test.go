package orag_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	orag "github.com/shikanon/orag"
)

func TestSDKKnowledgeWorkflow(t *testing.T) {
	ctx := context.Background()
	client, err := orag.New(ctx, orag.MockConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	knowledgeBase, err := client.CreateKnowledgeBase(ctx, orag.CreateKnowledgeBaseRequest{
		Name:        "SDK Knowledge",
		Description: "Public SDK workflow",
		Metadata:    map[string]string{"owner": "sdk-test"},
	})
	if err != nil {
		t.Fatalf("CreateKnowledgeBase() error = %v", err)
	}
	if knowledgeBase.ID == "" || knowledgeBase.Name != "SDK Knowledge" {
		t.Fatalf("CreateKnowledgeBase() = %#v", knowledgeBase)
	}

	ingested, err := client.IngestText(ctx, orag.IngestTextRequest{
		KnowledgeBaseID: knowledgeBase.ID,
		Name:            "orag-sdk.txt",
		SourceURI:       "memory://orag-sdk",
		Text:            "ORAG is a Go-native retrieval augmented generation service with evaluation-first workflows.",
	})
	if err != nil {
		t.Fatalf("IngestText() error = %v", err)
	}
	if ingested.Job.ID == "" || ingested.Job.Status != orag.IngestionSucceeded || ingested.Job.ChunkCount == 0 {
		t.Fatalf("IngestText() = %#v", ingested)
	}

	fileResult, err := client.IngestFile(ctx, orag.IngestFileRequest{
		KnowledgeBaseID: knowledgeBase.ID,
		Name:            "evaluation.txt",
		SourceURI:       "memory://evaluation",
		Reader:          bytes.NewBufferString("ORAG evaluates retrieval quality with reproducible datasets."),
	})
	if err != nil {
		t.Fatalf("IngestFile() error = %v", err)
	}
	job, found, err := client.GetIngestionJob(ctx, orag.GetIngestionJobRequest{ID: fileResult.Job.ID})
	if err != nil || !found || job.Status != orag.IngestionSucceeded {
		t.Fatalf("GetIngestionJob() job=%#v found=%v err=%v", job, found, err)
	}

	response, err := client.Query(ctx, orag.QueryRequest{
		KnowledgeBaseID: knowledgeBase.ID,
		Query:           "How does ORAG evaluate retrieval?",
		TraceID:         "trace_sdk_workflow",
	})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if response.TraceID != "trace_sdk_workflow" || len(response.Citations) == 0 {
		t.Fatalf("Query() = %#v", response)
	}

	trace, found, err := client.GetTrace(ctx, orag.GetTraceRequest{ID: response.TraceID})
	if err != nil || !found || trace.ID != response.TraceID || len(trace.NodeSpans) == 0 {
		t.Fatalf("GetTrace() trace=%#v found=%v err=%v", trace, found, err)
	}

	items, err := client.ListKnowledgeBases(ctx, orag.ListKnowledgeBasesRequest{})
	if err != nil {
		t.Fatalf("ListKnowledgeBases() error = %v", err)
	}
	if !containsKnowledgeBase(items, knowledgeBase.ID) {
		t.Fatalf("ListKnowledgeBases() missing %s: %#v", knowledgeBase.ID, items)
	}

	if err := client.DeleteKnowledgeBase(ctx, orag.DeleteKnowledgeBaseRequest{ID: knowledgeBase.ID}); err != nil {
		t.Fatalf("DeleteKnowledgeBase() error = %v", err)
	}
	if err := client.DeleteKnowledgeBase(ctx, orag.DeleteKnowledgeBaseRequest{ID: knowledgeBase.ID}); !errors.Is(err, orag.ErrNotFound) {
		t.Fatalf("second DeleteKnowledgeBase() error = %v, want ErrNotFound", err)
	}
}

func containsKnowledgeBase(items []orag.KnowledgeBase, id string) bool {
	for _, item := range items {
		if item.ID == id {
			return true
		}
	}
	return false
}
