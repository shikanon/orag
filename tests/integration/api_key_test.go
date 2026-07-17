package integration

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/shikanon/orag/internal/auth"
	oraghttp "github.com/shikanon/orag/internal/http"
	"github.com/shikanon/orag/internal/kb"
	"github.com/shikanon/orag/internal/project"
)

func TestPostgresAPIKeyLifecycleAndTenantProjectBoundary(t *testing.T) {
	app := newIntegrationApp(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	ownedProject, err := app.Projects.Create(ctx, testTenantID, project.CreateInput{Name: "API key integration"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	knowledgeBase := kb.KnowledgeBase{
		ID: "kb_project_ownership_integration", TenantID: testTenantID, ProjectID: ownedProject.ID,
		Name: "Project-owned integration knowledge base", CreatedAt: now, UpdatedAt: now,
	}
	if err := app.KBStore.PutKnowledgeBase(ctx, knowledgeBase); err != nil {
		t.Fatal(err)
	}
	storedKnowledgeBase, ok, err := app.KBStore.GetKnowledgeBase(ctx, testTenantID, knowledgeBase.ID)
	if err != nil || !ok {
		t.Fatalf("get project-owned knowledge base found=%v err=%v", ok, err)
	}
	if storedKnowledgeBase.ProjectID != ownedProject.ID {
		t.Fatalf("stored knowledge base project=%q, want %q", storedKnowledgeBase.ProjectID, ownedProject.ID)
	}
	storedDataset, err := app.Datasets.CreateInProject(ctx, testTenantID, ownedProject.ID, "Project-owned integration dataset", "golden")
	if err != nil {
		t.Fatal(err)
	}
	storedDataset, ok, err = app.Datasets.Get(ctx, testTenantID, storedDataset.ID)
	if err != nil || !ok {
		t.Fatalf("get project-owned dataset found=%v err=%v", ok, err)
	}
	if storedDataset.ProjectID != ownedProject.ID {
		t.Fatalf("stored dataset project=%q, want %q", storedDataset.ProjectID, ownedProject.ID)
	}
	created, err := app.APIKeys.Create(ctx, auth.APIKeyCreateInput{
		TenantID: testTenantID, ProjectID: ownedProject.ID, Name: "integration robot",
		Role: auth.RoleProjectEditor, CreatedBy: "integration_test",
	})
	if err != nil {
		t.Fatal(err)
	}

	principal, err := app.APIKeys.Authenticate(ctx, created.Secret)
	if err != nil {
		t.Fatal(err)
	}
	if principal.TenantID != testTenantID || principal.ProjectID != ownedProject.ID || principal.Role != auth.RoleProjectEditor {
		t.Fatalf("principal = %#v", principal)
	}
	items, err := app.APIKeys.List(ctx, testTenantID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range items {
		if item.ID == created.APIKey.ID {
			found = true
			if item.KeyHash == "" {
				t.Fatal("repository must retain the hash for authentication")
			}
			if item.LastUsedAt == nil {
				t.Fatal("successful authentication must record last_used_at")
			}
		}
	}
	if !found {
		t.Fatal("created key missing from tenant list")
	}
	h := oraghttp.NewServer(app).Hertz().Engine
	if status, body := performIntegrationJSON(h, "GET", "/v1/projects/"+ownedProject.ID, "", created.Secret); status != 200 {
		t.Fatalf("HTTP API key project read status=%d body=%s", status, body)
	}
	if status, body := performIntegrationJSON(h, "GET", "/v1/projects", "", created.Secret); status != 403 {
		t.Fatalf("HTTP API key tenant enumeration status=%d body=%s", status, body)
	}
	if status, body := performIntegrationJSON(h, "GET", "/v1/knowledge-bases/"+knowledgeBase.ID, "", created.Secret); status != 200 {
		t.Fatalf("HTTP project knowledge-base read status=%d body=%s", status, body)
	}
	if status, body := performIntegrationJSON(h, "GET", "/v1/knowledge-bases", "", created.Secret); status != 200 || !strings.Contains(body, knowledgeBase.ID) || strings.Contains(body, "kb_default") {
		t.Fatalf("HTTP project knowledge-base list status=%d body=%s", status, body)
	}
	if status, body := performIntegrationJSON(h, "GET", "/v1/knowledge-bases/kb_default", "", created.Secret); status != 404 {
		t.Fatalf("HTTP cross-project knowledge-base read status=%d body=%s", status, body)
	}
	if status, body := performIntegrationJSON(h, "POST", "/v1/knowledge-bases", `{"name":"Created by project key"}`, created.Secret); status != 201 || !strings.Contains(body, `"project_id":"`+ownedProject.ID+`"`) {
		t.Fatalf("HTTP project knowledge-base create status=%d body=%s", status, body)
	}
	if status, body := performIntegrationJSON(h, "POST", "/v1/datasets/"+storedDataset.ID+"/items", `{"query":"hello","ground_truth":"Insufficient context."}`, created.Secret); status != 201 {
		t.Fatalf("HTTP project dataset item status=%d body=%s", status, body)
	}
	if status, body := performIntegrationJSON(h, "POST", "/v1/query", `{"knowledge_base_id":"`+knowledgeBase.ID+`","query":"hello"}`, created.Secret); status != 409 || !strings.Contains(body, `"code":"production_version_unavailable"`) {
		t.Fatalf("HTTP project query without production version status=%d body=%s", status, body)
	}
	if status, body := performIntegrationJSON(h, "GET", "/v1/traces", "", created.Secret); status != 403 {
		t.Fatalf("HTTP unmigrated trace route status=%d body=%s", status, body)
	}
	rotated, err := app.APIKeys.Rotate(ctx, auth.APIKeyRotateInput{TenantID: testTenantID, KeyID: created.APIKey.ID, RotatedBy: "integration_rotate"})
	if err != nil || rotated.Secret == "" || rotated.APIKey.RotatedFromKeyID != created.APIKey.ID {
		t.Fatalf("rotate result=%#v err=%v", rotated, err)
	}
	if _, err := app.APIKeys.Authenticate(ctx, created.Secret); !errors.Is(err, auth.ErrAPIKeyRevoked) {
		t.Fatalf("rotated source authentication error=%v", err)
	}
	if principal, err := app.APIKeys.Authenticate(ctx, rotated.Secret); err != nil || principal.SubjectID != rotated.APIKey.ID {
		t.Fatalf("rotated replacement principal=%#v err=%v", principal, err)
	}
	if err := app.APIKeys.Revoke(ctx, "tenant_other", rotated.APIKey.ID); !errors.Is(err, auth.ErrAPIKeyNotFound) {
		t.Fatalf("cross-tenant revoke error = %v", err)
	}
	if err := app.APIKeys.Revoke(ctx, testTenantID, rotated.APIKey.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := app.APIKeys.Authenticate(ctx, rotated.Secret); !errors.Is(err, auth.ErrAPIKeyRevoked) {
		t.Fatalf("revoked authentication error = %v", err)
	}
	if status, body := performIntegrationJSON(h, "GET", "/v1/projects/"+ownedProject.ID, "", rotated.Secret); status != 401 {
		t.Fatalf("revoked HTTP API key status=%d body=%s", status, body)
	}
}
