// Command sdk demonstrates the public embedded ORAG Go SDK without external
// storage or a real model API key.
package main

import (
	"context"
	"fmt"
	"log"

	orag "github.com/shikanon/orag"
)

func main() {
	ctx := context.Background()
	client, err := orag.New(ctx, orag.MockConfig())
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	result, err := client.IngestText(ctx, orag.IngestTextRequest{
		KnowledgeBaseID: "kb_default",
		Name:            "orag-sdk.txt",
		SourceURI:       "memory://orag-sdk",
		Text:            "ORAG is a Go-native RAG service and evaluation control plane.",
	})
	if err != nil {
		log.Fatal(err)
	}
	response, err := client.Query(ctx, orag.QueryRequest{
		KnowledgeBaseID: "kb_default",
		Query:           "What is ORAG?",
		TraceID:         "trace_sdk_example",
	})
	if err != nil {
		log.Fatal(err)
	}

	dataset, err := client.CreateDataset(ctx, orag.CreateDatasetRequest{Name: "SDK demo", Kind: "retrieval"})
	if err != nil {
		log.Fatal(err)
	}
	if _, err := client.AddDatasetItem(ctx, orag.AddDatasetItemRequest{
		DatasetID:      dataset.ID,
		Query:          "What is ORAG?",
		GroundTruth:    "ORAG is a Go-native RAG service.",
		RelevantDocIDs: []string{result.Document.ID},
		Split:          orag.DatasetSplitEval,
	}); err != nil {
		log.Fatal(err)
	}
	run, err := client.RunEvaluation(ctx, orag.RunEvaluationRequest{
		DatasetID:       dataset.ID,
		KnowledgeBaseID: "kb_default",
		Profile:         "realtime",
		Split:           orag.DatasetSplitEval,
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("answer=%q citations=%d trace_id=%s evaluation_id=%s metrics=%v\n", response.Answer, len(response.Citations), response.TraceID, run.ID, run.Metrics)
}
