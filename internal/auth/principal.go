package auth

import "strings"

type PrincipalKind string

const (
	PrincipalUser   PrincipalKind = "user"
	PrincipalAPIKey PrincipalKind = "api_key"
)

type Role string

const (
	RoleTenantAdmin   Role = "tenant_admin"
	RoleProjectEditor Role = "project_editor"
	RoleProjectViewer Role = "project_viewer"
)

type Principal struct {
	Kind      PrincipalKind
	SubjectID string
	TenantID  string
	Role      Role
	ProjectID string
}

func (p Principal) Valid() bool {
	if strings.TrimSpace(p.SubjectID) == "" || strings.TrimSpace(p.TenantID) == "" {
		return false
	}
	switch p.Kind {
	case PrincipalUser, PrincipalAPIKey:
	default:
		return false
	}
	switch p.Role {
	case RoleTenantAdmin:
		return true
	case RoleProjectEditor, RoleProjectViewer:
		return strings.TrimSpace(p.ProjectID) != ""
	default:
		return false
	}
}
