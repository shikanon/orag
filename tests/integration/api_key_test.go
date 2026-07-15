package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shikanon/orag/internal/auth"
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
		}
	}
	if !found {
		t.Fatal("created key missing from tenant list")
	}
	if err := app.APIKeys.Revoke(ctx, "tenant_other", created.APIKey.ID); !errors.Is(err, auth.ErrAPIKeyNotFound) {
		t.Fatalf("cross-tenant revoke error = %v", err)
	}
	if err := app.APIKeys.Revoke(ctx, testTenantID, created.APIKey.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := app.APIKeys.Authenticate(ctx, created.Secret); !errors.Is(err, auth.ErrAPIKeyRevoked) {
		t.Fatalf("revoked authentication error = %v", err)
	}
}
