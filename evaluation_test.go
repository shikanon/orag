package orag_test

import (
	"context"
	"testing"

	orag "github.com/shikanon/orag"
)

func TestSDKEvaluationWorkflow(t *testing.T) {
	ctx := context.Background()
	client, err := orag.New(ctx, orag.MockConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	ingested, err := client.IngestText(ctx, orag.IngestTextRequest{
		KnowledgeBaseID: "kb_default",
		Name:            "sdk-evaluation.txt",
		SourceURI:       "memory://sdk-evaluation",
		Text:            "ORAG is a Go-native RAG service with reproducible evaluation workflows.",
	})
	if err != nil {
		t.Fatal(err)
	}

	dataset, err := client.CreateDataset(ctx, orag.CreateDatasetRequest{Name: "SDK evaluation", Kind: "retrieval"})
	if err != nil {
		t.Fatalf("CreateDataset() error = %v", err)
	}
	item, err := client.AddDatasetItem(ctx, orag.AddDatasetItemRequest{
		DatasetID:      dataset.ID,
		Query:          "What kind of service is ORAG?",
		GroundTruth:    "ORAG is a Go-native RAG service.",
		RelevantDocIDs: []string{ingested.Document.ID},
		Split:          orag.DatasetSplitEval,
	})
	if err != nil || item.ID == "" {
		t.Fatalf("AddDatasetItem() item=%#v err=%v", item, err)
	}

	run, err := client.RunEvaluation(ctx, orag.RunEvaluationRequest{
		DatasetID:       dataset.ID,
		KnowledgeBaseID: "kb_default",
		Profile:         "realtime",
		Split:           orag.DatasetSplitEval,
	})
	if err != nil {
		t.Fatalf("RunEvaluation() error = %v", err)
	}
	if run.ID == "" || run.Total != 1 || len(run.Metrics) == 0 {
		t.Fatalf("RunEvaluation() = %#v", run)
	}

	stored, found, err := client.GetEvaluation(ctx, orag.GetEvaluationRequest{ID: run.ID})
	if err != nil || !found || stored.ID != run.ID {
		t.Fatalf("GetEvaluation() run=%#v found=%v err=%v", stored, found, err)
	}
}
