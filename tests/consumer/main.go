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

	project, err := client.CreateProject(ctx, orag.CreateProjectRequest{Name: "consumer project"})
	if err != nil {
		return err
	}
	createdKey, err := client.CreateAPIKey(ctx, orag.CreateAPIKeyRequest{
		ProjectID: project.ID,
		Name:      "consumer automation",
		Role:      orag.RoleProjectEditor,
	})
	if err != nil {
		return err
	}
	if _, err := client.AuthenticateAPIKey(ctx, orag.AuthenticateAPIKeyRequest{Secret: createdKey.Secret}); err != nil {
		return err
	}
	if _, err := client.ListAPIKeys(ctx, orag.ListAPIKeysRequest{}); err != nil {
		return err
	}

	knowledgeBase, err := client.CreateKnowledgeBase(ctx, orag.CreateKnowledgeBaseRequest{ProjectID: project.ID, Name: "consumer proof"})
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

	dataset, err := client.CreateDataset(ctx, orag.CreateDatasetRequest{ProjectID: project.ID, Name: "consumer evaluation", Kind: "retrieval"})
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
	if err != nil {
		return err
	}
	return client.RevokeAPIKey(ctx, orag.RevokeAPIKeyRequest{ID: createdKey.APIKey.ID})
}
