package main

import (
	"context"
	"fmt"

	orag "github.com/shikanon/orag"
)

func main() {
	if err := walkthrough(context.Background()); err != nil {
		panic(err)
	}
	fmt.Println("ORAG SDK walkthrough completed")
}

func walkthrough(ctx context.Context) error {
	client, err := orag.New(ctx, orag.MockConfig())
	if err != nil {
		return err
	}
	defer client.Close()

	knowledgeBase, err := client.CreateKnowledgeBase(ctx, orag.CreateKnowledgeBaseRequest{Name: "consumer proof"})
	if err != nil {
		return err
	}
	ingested, err := client.IngestText(ctx, orag.IngestTextRequest{
		KnowledgeBaseID: knowledgeBase.ID,
		Name:            "orag.txt",
		SourceURI:       "memory://orag",
		Text:            "ORAG is a Go-native RAG service and evaluation control plane.",
	})
	if err != nil {
		return err
	}
	response, err := client.Query(ctx, orag.QueryRequest{
		KnowledgeBaseID: knowledgeBase.ID,
		Query:           "What is ORAG?",
		TraceID:         "trace_sdk_consumer",
	})
	if err != nil {
		return err
	}
	if _, found, err := client.GetTrace(ctx, orag.GetTraceRequest{ID: response.TraceID}); err != nil || !found {
		if err != nil {
			return err
		}
		return fmt.Errorf("trace %q was not stored", response.TraceID)
	}

	dataset, err := client.CreateDataset(ctx, orag.CreateDatasetRequest{Name: "consumer evaluation", Kind: "retrieval"})
	if err != nil {
		return err
	}
	if _, err := client.AddDatasetItem(ctx, orag.AddDatasetItemRequest{
		DatasetID:      dataset.ID,
		Query:          "What is ORAG?",
		GroundTruth:    "ORAG is a Go-native RAG service.",
		RelevantDocIDs: []string{ingested.Document.ID},
		Split:          orag.DatasetSplitEval,
	}); err != nil {
		return err
	}
	_, err = client.RunEvaluation(ctx, orag.RunEvaluationRequest{
		DatasetID:       dataset.ID,
		KnowledgeBaseID: knowledgeBase.ID,
		Profile:         "realtime",
		Split:           orag.DatasetSplitEval,
	})
	return err
}
