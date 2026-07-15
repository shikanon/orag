package auth

import (
	"errors"
	"testing"
)

func TestAuthorizeMatrix(t *testing.T) {
	admin := Principal{Kind: PrincipalUser, SubjectID: "user_1", TenantID: "tenant_1", Role: RoleTenantAdmin}
	constrainedAdmin := Principal{Kind: PrincipalAPIKey, SubjectID: "key_admin", TenantID: "tenant_1", Role: RoleTenantAdmin, ProjectID: "prj_1"}
	editor := Principal{Kind: PrincipalAPIKey, SubjectID: "key_editor", TenantID: "tenant_1", Role: RoleProjectEditor, ProjectID: "prj_1"}
	viewer := Principal{Kind: PrincipalAPIKey, SubjectID: "key_viewer", TenantID: "tenant_1", Role: RoleProjectViewer, ProjectID: "prj_1"}

	tests := []struct {
		name      string
		principal Principal
		action    Action
		tenant    string
		project   string
		allowed   bool
	}{
		{"admin manages keys", admin, ActionAPIKeyManage, "tenant_1", "", true},
		{"admin lists projects", admin, ActionProjectList, "tenant_1", "", true},
		{"constrained admin cannot manage keys", constrainedAdmin, ActionAPIKeyManage, "tenant_1", "", false},
		{"constrained admin reads own project", constrainedAdmin, ActionProjectRead, "tenant_1", "prj_1", true},
		{"constrained admin cannot cross project", constrainedAdmin, ActionResourceWrite, "tenant_1", "prj_2", false},
		{"editor reads project", editor, ActionProjectRead, "tenant_1", "prj_1", true},
		{"editor writes resource", editor, ActionResourceWrite, "tenant_1", "prj_1", true},
		{"editor cannot update project", editor, ActionProjectUpdate, "tenant_1", "prj_1", false},
		{"editor cannot enumerate projects", editor, ActionProjectList, "tenant_1", "", false},
		{"viewer reads resource", viewer, ActionResourceRead, "tenant_1", "prj_1", true},
		{"viewer cannot write resource", viewer, ActionResourceWrite, "tenant_1", "prj_1", false},
		{"viewer cannot cross project", viewer, ActionProjectRead, "tenant_1", "prj_2", false},
		{"admin reads compatibility resource", admin, ActionResourceRead, "tenant_1", "", true},
		{"editor cannot read compatibility resource", editor, ActionResourceRead, "tenant_1", "", false},
		{"tenant mismatch", admin, ActionProjectRead, "tenant_2", "prj_1", false},
		{"project action requires project", admin, ActionProjectRead, "tenant_1", "", false},
		{"unknown action denied", admin, Action("project.delete"), "tenant_1", "prj_1", false},
		{"malformed principal denied", Principal{}, ActionProjectList, "tenant_1", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Authorize(tt.principal, tt.action, tt.tenant, tt.project)
			if tt.allowed && err != nil {
				t.Fatalf("Authorize() error = %v", err)
			}
			if !tt.allowed && !errors.Is(err, ErrForbidden) {
				t.Fatalf("Authorize() error = %v, want ErrForbidden", err)
			}
		})
	}
}
