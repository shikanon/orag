package orag

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/shikanon/orag/internal/auth"
	"github.com/shikanon/orag/internal/project"
)

// Role is the authorization role assigned to an API key.
type Role string

const (
	RoleTenantAdmin   Role = "tenant_admin"
	RoleProjectEditor Role = "project_editor"
	RoleProjectViewer Role = "project_viewer"
)

// PrincipalKind identifies how an authenticated principal was established.
type PrincipalKind string

const (
	PrincipalUser   PrincipalKind = "user"
	PrincipalAPIKey PrincipalKind = "api_key"
)

// Principal is the public identity produced by API key authentication.
type Principal struct {
	Kind      PrincipalKind
	SubjectID string
	TenantID  string
	Role      Role
	ProjectID string
}

// Project is a tenant-owned control-plane boundary.
type Project struct {
	ID          string
	TenantID    string
	Name        string
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type CreateProjectRequest struct {
	TenantID    string
	Name        string
	Description string
}

type ListProjectsRequest struct{ TenantID string }

type GetProjectRequest struct {
	TenantID string
	ID       string
}

type UpdateProjectRequest struct {
	TenantID    string
	ID          string
	Name        string
	Description string
}

// APIKey is API key metadata. It never contains the key hash or secret.
type APIKey struct {
	ID               string
	TenantID         string
	ProjectID        string
	Name             string
	Prefix           string
	Role             Role
	CreatedBy        string
	CreatedAt        time.Time
	ExpiresAt        *time.Time
	RevokedAt        *time.Time
	LastUsedAt       *time.Time
	RotatedFromKeyID string
}

type CreateAPIKeyRequest struct {
	TenantID  string
	ProjectID string
	Name      string
	Role      Role
	CreatedBy string
	ExpiresAt *time.Time
}

// CreateAPIKeyResult contains a newly created key. Secret is returned exactly
// once and is never included by ListAPIKeys.
type CreateAPIKeyResult struct {
	APIKey APIKey
	Secret string
}

type ListAPIKeysRequest struct{ TenantID string }

type RevokeAPIKeyRequest struct {
	TenantID string
	ID       string
}

// RotateAPIKeyRequest identifies an active key to replace immediately.
type RotateAPIKeyRequest struct {
	TenantID  string
	ID        string
	RotatedBy string
}

type AuthenticateAPIKeyRequest struct{ Secret string }

func (c *Client) CreateProject(ctx context.Context, req CreateProjectRequest) (Project, error) {
	if err := c.requireOpen("create_project"); err != nil {
		return Project{}, err
	}
	item, err := c.app.Projects.Create(ctx, c.tenant(req.TenantID), project.CreateInput{Name: req.Name, Description: req.Description})
	if err != nil {
		return Project{}, controlPlaneError("create_project", "project", err)
	}
	return fromProject(item), nil
}

func (c *Client) ListProjects(ctx context.Context, req ListProjectsRequest) ([]Project, error) {
	if err := c.requireOpen("list_projects"); err != nil {
		return nil, err
	}
	items, err := c.app.Projects.List(ctx, c.tenant(req.TenantID))
	if err != nil {
		return nil, controlPlaneError("list_projects", "project", err)
	}
	result := make([]Project, len(items))
	for index := range items {
		result[index] = fromProject(items[index])
	}
	return result, nil
}

func (c *Client) GetProject(ctx context.Context, req GetProjectRequest) (Project, error) {
	if err := c.requireOpen("get_project"); err != nil {
		return Project{}, err
	}
	if strings.TrimSpace(req.ID) == "" {
		return Project{}, newError(CodeInvalidArgument, "get_project", "project", "", false, errors.New("id is required"))
	}
	item, err := c.app.Projects.Get(ctx, c.tenant(req.TenantID), strings.TrimSpace(req.ID))
	if err != nil {
		return Project{}, controlPlaneError("get_project", req.ID, err)
	}
	return fromProject(item), nil
}

func (c *Client) UpdateProject(ctx context.Context, req UpdateProjectRequest) (Project, error) {
	if err := c.requireOpen("update_project"); err != nil {
		return Project{}, err
	}
	if strings.TrimSpace(req.ID) == "" {
		return Project{}, newError(CodeInvalidArgument, "update_project", "project", "", false, errors.New("id is required"))
	}
	item, err := c.app.Projects.Update(ctx, c.tenant(req.TenantID), strings.TrimSpace(req.ID), project.UpdateInput{Name: req.Name, Description: req.Description})
	if err != nil {
		return Project{}, controlPlaneError("update_project", req.ID, err)
	}
	return fromProject(item), nil
}

func (c *Client) CreateAPIKey(ctx context.Context, req CreateAPIKeyRequest) (CreateAPIKeyResult, error) {
	if err := c.requireOpen("create_api_key"); err != nil {
		return CreateAPIKeyResult{}, err
	}
	tenantID := c.tenant(req.TenantID)
	projectID := strings.TrimSpace(req.ProjectID)
	if projectID != "" {
		if _, err := c.app.Projects.Get(ctx, tenantID, projectID); err != nil {
			return CreateAPIKeyResult{}, controlPlaneError("create_api_key", projectID, err)
		}
	}
	createdBy := strings.TrimSpace(req.CreatedBy)
	if createdBy == "" {
		createdBy = "sdk"
	}
	created, err := c.app.APIKeys.Create(ctx, auth.APIKeyCreateInput{
		TenantID: tenantID, ProjectID: projectID, Name: req.Name,
		Role: auth.Role(req.Role), CreatedBy: createdBy, ExpiresAt: req.ExpiresAt,
	})
	if err != nil {
		return CreateAPIKeyResult{}, controlPlaneError("create_api_key", "api_key", err)
	}
	return CreateAPIKeyResult{APIKey: fromAPIKey(created.APIKey), Secret: created.Secret}, nil
}

func (c *Client) ListAPIKeys(ctx context.Context, req ListAPIKeysRequest) ([]APIKey, error) {
	if err := c.requireOpen("list_api_keys"); err != nil {
		return nil, err
	}
	items, err := c.app.APIKeys.List(ctx, c.tenant(req.TenantID))
	if err != nil {
		return nil, controlPlaneError("list_api_keys", "api_key", err)
	}
	result := make([]APIKey, len(items))
	for index := range items {
		result[index] = fromAPIKey(items[index])
	}
	return result, nil
}

func (c *Client) RevokeAPIKey(ctx context.Context, req RevokeAPIKeyRequest) error {
	if err := c.requireOpen("revoke_api_key"); err != nil {
		return err
	}
	if strings.TrimSpace(req.ID) == "" {
		return newError(CodeInvalidArgument, "revoke_api_key", "api_key", "", false, errors.New("id is required"))
	}
	if err := c.app.APIKeys.Revoke(ctx, c.tenant(req.TenantID), strings.TrimSpace(req.ID)); err != nil {
		return controlPlaneError("revoke_api_key", req.ID, err)
	}
	return nil
}

// RotateAPIKey creates a replacement key and atomically revokes the source.
// The returned secret is available only in this result.
func (c *Client) RotateAPIKey(ctx context.Context, req RotateAPIKeyRequest) (CreateAPIKeyResult, error) {
	if err := c.requireOpen("rotate_api_key"); err != nil {
		return CreateAPIKeyResult{}, err
	}
	if strings.TrimSpace(req.ID) == "" {
		return CreateAPIKeyResult{}, newError(CodeInvalidArgument, "rotate_api_key", "api_key", "", false, errors.New("id is required"))
	}
	rotatedBy := strings.TrimSpace(req.RotatedBy)
	if rotatedBy == "" {
		rotatedBy = "sdk"
	}
	created, err := c.app.APIKeys.Rotate(ctx, auth.APIKeyRotateInput{TenantID: c.tenant(req.TenantID), KeyID: strings.TrimSpace(req.ID), RotatedBy: rotatedBy})
	if err != nil {
		return CreateAPIKeyResult{}, controlPlaneError("rotate_api_key", req.ID, err)
	}
	return CreateAPIKeyResult{APIKey: fromAPIKey(created.APIKey), Secret: created.Secret}, nil
}

// AuthenticateAPIKey verifies a secret against the embedded control-plane
// store. It is useful for applications embedding ORAG without the HTTP layer.
func (c *Client) AuthenticateAPIKey(ctx context.Context, req AuthenticateAPIKeyRequest) (Principal, error) {
	if err := c.requireOpen("authenticate_api_key"); err != nil {
		return Principal{}, err
	}
	principal, err := c.app.APIKeys.Authenticate(ctx, strings.TrimSpace(req.Secret))
	if err != nil {
		if errors.Is(err, auth.ErrAPIKeyInvalid) || errors.Is(err, auth.ErrAPIKeyExpired) || errors.Is(err, auth.ErrAPIKeyRevoked) {
			return Principal{}, newError(CodeUnauthorized, "authenticate_api_key", "api_key", "", false, err)
		}
		return Principal{}, controlPlaneError("authenticate_api_key", "api_key", err)
	}
	return Principal{Kind: PrincipalKind(principal.Kind), SubjectID: principal.SubjectID, TenantID: principal.TenantID, Role: Role(principal.Role), ProjectID: principal.ProjectID}, nil
}

func fromProject(item project.Project) Project {
	return Project{ID: item.ID, TenantID: item.TenantID, Name: item.Name, Description: item.Description, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt}
}

func fromAPIKey(item auth.APIKey) APIKey {
	return APIKey{ID: item.ID, TenantID: item.TenantID, ProjectID: item.ProjectID, Name: item.Name, Prefix: item.Prefix, Role: Role(item.Role), CreatedBy: item.CreatedBy, CreatedAt: item.CreatedAt, ExpiresAt: item.ExpiresAt, RevokedAt: item.RevokedAt, LastUsedAt: item.LastUsedAt, RotatedFromKeyID: item.RotatedFromKeyID}
}

func controlPlaneError(operation, resource string, err error) error {
	switch {
	case errors.Is(err, project.ErrTenantRequired), errors.Is(err, project.ErrNameRequired), errors.Is(err, auth.ErrAPIKeyInvalid):
		return newError(CodeInvalidArgument, operation, resource, "", false, err)
	case errors.Is(err, project.ErrNotFound), errors.Is(err, auth.ErrAPIKeyNotFound):
		return newError(CodeNotFound, operation, resource, "", false, err)
	case errors.Is(err, project.ErrConflict):
		return newError(CodeConflict, operation, resource, "", false, err)
	default:
		return wrapError(operation, resource, "", err)
	}
}
