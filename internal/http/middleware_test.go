package http

import (
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/shikanon/orag/internal/auth"
)

func TestTenantIDComesOnlyFromValidPrincipal(t *testing.T) {
	c := &app.RequestContext{}
	if got := tenantID(c); got != "" {
		t.Fatalf("tenantID() without principal = %q, want empty", got)
	}

	c.Set("tenant_id", "tenant_spoofed")
	if got := tenantID(c); got != "" {
		t.Fatalf("tenantID() with legacy value only = %q, want empty", got)
	}

	principal := auth.Principal{Kind: auth.PrincipalUser, SubjectID: "user_1", TenantID: "tenant_1", Role: auth.RoleTenantAdmin}
	c.Set("principal", principal)
	if got := tenantID(c); got != "tenant_1" {
		t.Fatalf("tenantID() = %q, want tenant_1", got)
	}
}
