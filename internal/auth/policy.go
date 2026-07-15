package auth

import "errors"

var ErrForbidden = errors.New("forbidden")

type Action string

const (
	ActionAPIKeyManage  Action = "api_key.manage"
	ActionProjectCreate Action = "project.create"
	ActionProjectList   Action = "project.list"
	ActionProjectRead   Action = "project.read"
	ActionProjectUpdate Action = "project.update"
	ActionResourceRead  Action = "resource.read"
	ActionResourceWrite Action = "resource.write"
)

// Authorize denies unknown actions and malformed principals. resourceProjectID
// is required for project-scoped actions and must be empty for tenant-wide ones.
func Authorize(principal Principal, action Action, resourceTenantID, resourceProjectID string) error {
	if !principal.Valid() || resourceTenantID == "" || principal.TenantID != resourceTenantID {
		return ErrForbidden
	}

	tenantWide := action == ActionAPIKeyManage || action == ActionProjectCreate || action == ActionProjectList
	if tenantWide {
		if resourceProjectID != "" || principal.ProjectID != "" || principal.Role != RoleTenantAdmin {
			return ErrForbidden
		}
		return nil
	}

	switch action {
	case ActionProjectRead, ActionProjectUpdate, ActionResourceRead, ActionResourceWrite:
		if resourceProjectID == "" {
			return ErrForbidden
		}
	default:
		return ErrForbidden
	}

	if principal.ProjectID != "" && principal.ProjectID != resourceProjectID {
		return ErrForbidden
	}

	switch principal.Role {
	case RoleTenantAdmin:
		return nil
	case RoleProjectEditor:
		if action == ActionProjectRead || action == ActionResourceRead || action == ActionResourceWrite {
			return nil
		}
	case RoleProjectViewer:
		if action == ActionProjectRead || action == ActionResourceRead {
			return nil
		}
	}
	return ErrForbidden
}
