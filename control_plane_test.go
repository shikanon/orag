package orag_test

import (
	"context"
	"errors"
	"testing"

	orag "github.com/shikanon/orag"
)

func TestSDKProjectAndAPIKeyLifecycle(t *testing.T) {
	ctx := context.Background()
	client, err := orag.New(ctx, orag.MockConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	created, err := client.CreateProject(ctx, orag.CreateProjectRequest{Name: "SDK project", Description: "before"})
	if err != nil || created.ID == "" {
		t.Fatalf("CreateProject() project=%#v err=%v", created, err)
	}
	updated, err := client.UpdateProject(ctx, orag.UpdateProjectRequest{ID: created.ID, Name: "SDK project updated", Description: "after"})
	if err != nil || updated.Name != "SDK project updated" || updated.Description != "after" {
		t.Fatalf("UpdateProject() project=%#v err=%v", updated, err)
	}
	got, err := client.GetProject(ctx, orag.GetProjectRequest{ID: created.ID})
	if err != nil || got.ID != created.ID {
		t.Fatalf("GetProject() project=%#v err=%v", got, err)
	}
	projects, err := client.ListProjects(ctx, orag.ListProjectsRequest{})
	if err != nil || !containsProject(projects, created.ID) {
		t.Fatalf("ListProjects() projects=%#v err=%v", projects, err)
	}

	createdKey, err := client.CreateAPIKey(ctx, orag.CreateAPIKeyRequest{
		ProjectID: created.ID,
		Name:      "automation",
		Role:      orag.RoleProjectEditor,
	})
	if err != nil || createdKey.APIKey.ID == "" || createdKey.Secret == "" {
		t.Fatalf("CreateAPIKey() id=%q secret_present=%v err=%v", createdKey.APIKey.ID, createdKey.Secret != "", err)
	}
	if createdKey.APIKey.ProjectID != created.ID || createdKey.APIKey.CreatedBy != "sdk" {
		t.Fatalf("CreateAPIKey() metadata=%#v", createdKey.APIKey)
	}

	principal, err := client.AuthenticateAPIKey(ctx, orag.AuthenticateAPIKeyRequest{Secret: createdKey.Secret})
	if err != nil || principal.Kind != orag.PrincipalAPIKey || principal.SubjectID != createdKey.APIKey.ID || principal.ProjectID != created.ID {
		t.Fatalf("AuthenticateAPIKey() principal=%#v err=%v", principal, err)
	}
	keys, err := client.ListAPIKeys(ctx, orag.ListAPIKeysRequest{})
	if err != nil || len(keys) != 1 || keys[0].ID != createdKey.APIKey.ID {
		t.Fatalf("ListAPIKeys() keys=%#v err=%v", keys, err)
	}
	rotatedKey, err := client.RotateAPIKey(ctx, orag.RotateAPIKeyRequest{ID: createdKey.APIKey.ID})
	if err != nil || rotatedKey.Secret == "" || rotatedKey.APIKey.RotatedFromKeyID != createdKey.APIKey.ID {
		t.Fatalf("RotateAPIKey() result=%#v err=%v", rotatedKey, err)
	}
	if _, err := client.AuthenticateAPIKey(ctx, orag.AuthenticateAPIKeyRequest{Secret: createdKey.Secret}); !errors.Is(err, orag.ErrUnauthorized) {
		t.Fatalf("AuthenticateAPIKey() rotated source error=%v, want ErrUnauthorized", err)
	}
	if principal, err := client.AuthenticateAPIKey(ctx, orag.AuthenticateAPIKeyRequest{Secret: rotatedKey.Secret}); err != nil || principal.SubjectID != rotatedKey.APIKey.ID {
		t.Fatalf("AuthenticateAPIKey() rotated replacement principal=%#v err=%v", principal, err)
	}
	if err := client.RevokeAPIKey(ctx, orag.RevokeAPIKeyRequest{ID: rotatedKey.APIKey.ID}); err != nil {
		t.Fatalf("RevokeAPIKey() error=%v", err)
	}
	if _, err := client.AuthenticateAPIKey(ctx, orag.AuthenticateAPIKeyRequest{Secret: rotatedKey.Secret}); !errors.Is(err, orag.ErrUnauthorized) {
		t.Fatalf("AuthenticateAPIKey() after revoke error=%v, want ErrUnauthorized", err)
	}
}

func TestSDKProjectOwnedResources(t *testing.T) {
	ctx := context.Background()
	client, err := orag.New(ctx, orag.MockConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	project, err := client.CreateProject(ctx, orag.CreateProjectRequest{Name: "resource owner"})
	if err != nil {
		t.Fatal(err)
	}
	knowledgeBase, err := client.CreateKnowledgeBase(ctx, orag.CreateKnowledgeBaseRequest{ProjectID: project.ID, Name: "owned knowledge"})
	if err != nil || knowledgeBase.ProjectID != project.ID {
		t.Fatalf("CreateKnowledgeBase() knowledge_base=%#v err=%v", knowledgeBase, err)
	}
	dataset, err := client.CreateDataset(ctx, orag.CreateDatasetRequest{ProjectID: project.ID, Name: "owned dataset", Kind: "retrieval"})
	if err != nil || dataset.ProjectID != project.ID {
		t.Fatalf("CreateDataset() dataset=%#v err=%v", dataset, err)
	}
	if _, err := client.AddDatasetItem(ctx, orag.AddDatasetItemRequest{DatasetID: dataset.ID, Query: "What is ORAG?", GroundTruth: "Insufficient context."}); err != nil {
		t.Fatal(err)
	}
	evaluation, err := client.RunEvaluation(ctx, orag.RunEvaluationRequest{ProjectID: project.ID, DatasetID: dataset.ID, KnowledgeBaseID: knowledgeBase.ID, Profile: "realtime"})
	if err != nil || evaluation.ProjectID != project.ID {
		t.Fatalf("RunEvaluation() evaluation=%#v err=%v", evaluation, err)
	}
	storedEvaluation, found, err := client.GetEvaluation(ctx, orag.GetEvaluationRequest{ProjectID: project.ID, ID: evaluation.ID})
	if err != nil || !found || storedEvaluation.ProjectID != project.ID {
		t.Fatalf("GetEvaluation() evaluation=%#v found=%v err=%v", storedEvaluation, found, err)
	}
	if _, found, err := client.GetEvaluation(ctx, orag.GetEvaluationRequest{ProjectID: "prj_other", ID: evaluation.ID}); err != nil || found {
		t.Fatalf("GetEvaluation() cross-project found=%v err=%v", found, err)
	}
	if _, err := client.CreateKnowledgeBase(ctx, orag.CreateKnowledgeBaseRequest{ProjectID: "prj_missing", Name: "invalid"}); !errors.Is(err, orag.ErrNotFound) {
		t.Fatalf("CreateKnowledgeBase() missing project error=%v, want ErrNotFound", err)
	}
	if _, err := client.CreateAPIKey(ctx, orag.CreateAPIKeyRequest{ProjectID: "prj_missing", Name: "invalid", Role: orag.RoleProjectViewer}); !errors.Is(err, orag.ErrNotFound) {
		t.Fatalf("CreateAPIKey() missing project error=%v, want ErrNotFound", err)
	}
}

func containsProject(items []orag.Project, id string) bool {
	for _, item := range items {
		if item.ID == id {
			return true
		}
	}
	return false
}
