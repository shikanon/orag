package http

import (
	"context"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/shikanon/orag/internal/auth"
	"github.com/shikanon/orag/internal/dataset"
)

type saveDebugCaseRequest struct {
	DatasetID        string   `json:"dataset_id"`
	Query            string   `json:"query"`
	GroundTruth      string   `json:"ground_truth"`
	ExpectedEvidence []string `json:"expected_evidence,omitempty"`
}

func (s *Server) saveDebugCase(ctx context.Context, c *app.RequestContext) {
	projectID, principal, ok := releaseProjectRequest(c)
	if !ok || !authorizeRequest(c, auth.ActionResourceWrite, principal.TenantID, projectID) {
		return
	}
	var req saveDebugCaseRequest
	if !bindJSON(c, &req) {
		return
	}
	if strings.TrimSpace(req.DatasetID) == "" || strings.TrimSpace(req.Query) == "" || strings.TrimSpace(req.GroundTruth) == "" {
		writeError(c, consts.StatusBadRequest, "invalid_request", "dataset_id, query, and ground_truth are required")
		return
	}
	if _, ok := s.authorizedDataset(ctx, c, req.DatasetID, auth.ActionResourceWrite); !ok {
		return
	}
	item, err := s.App.Datasets.AddItem(ctx, principal.TenantID, req.DatasetID, dataset.Item{Query: strings.TrimSpace(req.Query), GroundTruth: strings.TrimSpace(req.GroundTruth), ExpectedEvidence: req.ExpectedEvidence})
	if err != nil {
		if err == dataset.ErrDatasetNotFound {
			writeDatasetNotFound(c)
			return
		}
		writeError(c, consts.StatusInternalServerError, "debug_case_create_failed", err.Error())
		return
	}
	c.JSON(consts.StatusCreated, map[string]any{"run_id": c.Param("run_id"), "item": item})
}
