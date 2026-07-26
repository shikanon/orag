package orag

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/shikanon/orag/internal/kb"
	"github.com/shikanon/orag/internal/platform/id"
)

// KnowledgeBase is the public SDK representation of an ORAG knowledge base.
type KnowledgeBase struct {
	ID          string
	TenantID    string
	ProjectID   string
	Name        string
	Description string
	Metadata    map[string]string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type CreateKnowledgeBaseRequest struct {
	TenantID    string
	ProjectID   string
	Name        string
	Description string
	Metadata    map[string]string
}

type ListKnowledgeBasesRequest struct{ TenantID string }

type GetKnowledgeBaseRequest struct {
	TenantID string
	ID       string
}

type DeleteKnowledgeBaseRequest struct {
	TenantID string
	ID       string
}

func (c *Client) CreateKnowledgeBase(ctx context.Context, req CreateKnowledgeBaseRequest) (KnowledgeBase, error) {
	if err := c.requireOpen("create_knowledge_base"); err != nil {
		return KnowledgeBase{}, err
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return KnowledgeBase{}, newError(CodeInvalidArgument, "create_knowledge_base", "knowledge_base", "", false, errors.New("name is required"))
	}
	tenantID := c.tenant(req.TenantID)
	projectID := strings.TrimSpace(req.ProjectID)
	if projectID != "" {
		if _, err := c.app.Projects.Get(ctx, tenantID, projectID); err != nil {
			return KnowledgeBase{}, controlPlaneError("create_knowledge_base", projectID, err)
		}
	}
	now := time.Now().UTC()
	item := kb.KnowledgeBase{
		ID:          id.New("kb"),
		TenantID:    tenantID,
		ProjectID:   projectID,
		Name:        name,
		Description: strings.TrimSpace(req.Description),
		Metadata:    cloneStrings(req.Metadata),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := c.app.KBStore.PutKnowledgeBase(ctx, item); err != nil {
		return KnowledgeBase{}, wrapError("create_knowledge_base", item.ID, "", err)
	}
	return fromKnowledgeBase(item), nil
}

func (c *Client) ListKnowledgeBases(ctx context.Context, req ListKnowledgeBasesRequest) ([]KnowledgeBase, error) {
	if err := c.requireOpen("list_knowledge_bases"); err != nil {
		return nil, err
	}
	items, err := c.app.KBStore.ListKnowledgeBases(ctx, c.tenant(req.TenantID))
	if err != nil {
		return nil, wrapError("list_knowledge_bases", "", "", err)
	}
	result := make([]KnowledgeBase, len(items))
	for index := range items {
		result[index] = fromKnowledgeBase(items[index])
	}
	return result, nil
}

func (c *Client) GetKnowledgeBase(ctx context.Context, req GetKnowledgeBaseRequest) (KnowledgeBase, bool, error) {
	if err := c.requireOpen("get_knowledge_base"); err != nil {
		return KnowledgeBase{}, false, err
	}
	item, found, err := c.app.KBStore.GetKnowledgeBase(ctx, c.tenant(req.TenantID), strings.TrimSpace(req.ID))
	if err != nil {
		return KnowledgeBase{}, false, wrapError("get_knowledge_base", req.ID, "", err)
	}
	if !found {
		return KnowledgeBase{}, false, nil
	}
	return fromKnowledgeBase(item), true, nil
}

func (c *Client) DeleteKnowledgeBase(ctx context.Context, req DeleteKnowledgeBaseRequest) error {
	if err := c.requireOpen("delete_knowledge_base"); err != nil {
		return err
	}
	deleted, err := c.app.KBStore.DeleteKnowledgeBase(ctx, c.tenant(req.TenantID), strings.TrimSpace(req.ID))
	if err != nil {
		return wrapError("delete_knowledge_base", req.ID, "", err)
	}
	if !deleted {
		return newError(CodeNotFound, "delete_knowledge_base", req.ID, "", false, errors.New("knowledge base not found"))
	}
	return nil
}

func fromKnowledgeBase(item kb.KnowledgeBase) KnowledgeBase {
	return KnowledgeBase{ID: item.ID, TenantID: item.TenantID, ProjectID: item.ProjectID, Name: item.Name, Description: item.Description, Metadata: cloneStrings(item.Metadata), CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt}
}
