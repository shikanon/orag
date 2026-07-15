package auth

import "testing"

func TestPrincipalValid(t *testing.T) {
	tests := []struct {
		name      string
		principal Principal
		want      bool
	}{
		{"tenant admin", Principal{Kind: PrincipalUser, SubjectID: "user_1", TenantID: "tenant_1", Role: RoleTenantAdmin}, true},
		{"project editor", Principal{Kind: PrincipalAPIKey, SubjectID: "key_1", TenantID: "tenant_1", Role: RoleProjectEditor, ProjectID: "prj_1"}, true},
		{"editor requires project", Principal{Kind: PrincipalAPIKey, SubjectID: "key_1", TenantID: "tenant_1", Role: RoleProjectEditor}, false},
		{"unknown kind", Principal{Kind: "service", SubjectID: "key_1", TenantID: "tenant_1", Role: RoleTenantAdmin}, false},
		{"unknown role", Principal{Kind: PrincipalUser, SubjectID: "user_1", TenantID: "tenant_1", Role: "owner"}, false},
		{"missing subject", Principal{Kind: PrincipalUser, TenantID: "tenant_1", Role: RoleTenantAdmin}, false},
		{"missing tenant", Principal{Kind: PrincipalUser, SubjectID: "user_1", Role: RoleTenantAdmin}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.principal.Valid(); got != tt.want {
				t.Fatalf("Valid() = %v, want %v", got, tt.want)
			}
		})
	}
}
