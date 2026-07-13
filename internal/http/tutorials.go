package http

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/shikanon/orag/internal/tutorial"
)

type tutorialPackResponse struct {
	Tier                 string `json:"tier"`
	ManifestURL          string `json:"manifest_url"`
	EstimatedBytes       int64  `json:"estimated_bytes"`
	EstimatedMinutes     int    `json:"estimated_minutes"`
	RequiresLicenseCheck bool   `json:"requires_license_check"`
}

type tutorialResponse struct {
	ID                       string                 `json:"id"`
	Slug                     string                 `json:"slug"`
	Title                    string                 `json:"title"`
	Summary                  string                 `json:"summary"`
	Version                  string                 `json:"version"`
	Status                   string                 `json:"status"`
	Modality                 tutorial.Modality      `json:"modality"`
	Difficulty               string                 `json:"difficulty"`
	EstimatedDurationMinutes int                    `json:"estimated_duration_minutes"`
	SourceBenchmark          string                 `json:"source_benchmark"`
	SourceURL                string                 `json:"source_url"`
	ScenarioDimensions       []string               `json:"scenario_dimensions"`
	PipelineStages           []string               `json:"pipeline_stages"`
	RequiredCapabilities     []string               `json:"required_capabilities"`
	Packs                    []tutorialPackResponse `json:"packs"`
	ReplayAvailable          bool                   `json:"replay_available"`
}

func (s *Server) listTutorials(_ context.Context, c *app.RequestContext) {
	items := s.App.Tutorials.List()
	responses := make([]tutorialResponse, 0, len(items))
	for _, item := range items {
		response, err := s.tutorialResponse(item)
		if err != nil {
			writeError(c, consts.StatusInternalServerError, "tutorial_catalog_failed", "tutorial catalog is unavailable")
			return
		}
		responses = append(responses, response)
	}
	c.JSON(consts.StatusOK, map[string]any{"tutorials": responses})
}

func (s *Server) getTutorial(_ context.Context, c *app.RequestContext) {
	s.writeTutorial(c, c.Param("template_id"), "")
}

func (s *Server) getTutorialVersion(_ context.Context, c *app.RequestContext) {
	s.writeTutorial(c, c.Param("template_id"), c.Param("version"))
}

func (s *Server) writeTutorial(c *app.RequestContext, templateID, version string) {
	item, err := s.App.Tutorials.Get(templateID, version)
	if err != nil {
		writeTutorialError(c, err)
		return
	}
	response, err := s.tutorialResponse(item)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "tutorial_catalog_failed", "tutorial catalog is unavailable")
		return
	}
	c.JSON(consts.StatusOK, response)
}

func (s *Server) tutorialResponse(item tutorial.Template) (tutorialResponse, error) {
	response := tutorialResponse{
		ID:                       item.ID,
		Slug:                     item.Slug,
		Title:                    item.Title,
		Summary:                  item.Summary,
		Version:                  item.Version,
		Status:                   item.Status,
		Modality:                 item.Modality,
		Difficulty:               item.Difficulty,
		EstimatedDurationMinutes: item.EstimatedDurationMinutes,
		SourceBenchmark:          item.SourceBenchmark,
		SourceURL:                item.SourceURL,
		ScenarioDimensions:       item.ScenarioDimensions,
		PipelineStages:           item.PipelineStages,
		RequiredCapabilities:     item.RequiredCapabilities,
		ReplayAvailable:          item.ReplayAvailable,
		Packs:                    make([]tutorialPackResponse, 0, len(item.Packs)),
	}
	for _, pack := range item.Packs {
		manifestURL, err := resolveTutorialManifestURL(s.App.Config.Tutorial.CatalogBaseURL, pack.ManifestPath)
		if err != nil {
			return tutorialResponse{}, err
		}
		response.Packs = append(response.Packs, tutorialPackResponse{
			Tier:                 pack.Tier,
			ManifestURL:          manifestURL,
			EstimatedBytes:       pack.EstimatedBytes,
			EstimatedMinutes:     pack.EstimatedMinutes,
			RequiresLicenseCheck: pack.RequiresLicenseCheck,
		})
	}
	return response, nil
}

func resolveTutorialManifestURL(baseURL, manifestPath string) (string, error) {
	base, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || base.Scheme != "https" || base.Host == "" || base.User != nil {
		return "", fmt.Errorf("tutorial catalog base URL must be an absolute HTTPS URL")
	}
	if manifestPath == "" || strings.HasPrefix(manifestPath, "/") || strings.Contains(manifestPath, "\\") {
		return "", fmt.Errorf("tutorial manifest path is invalid")
	}
	decodedPath, err := url.PathUnescape(manifestPath)
	if err != nil {
		return "", fmt.Errorf("tutorial manifest path encoding is invalid")
	}
	cleaned := path.Clean(decodedPath)
	if cleaned != decodedPath || strings.HasPrefix(cleaned, "../") || strings.HasPrefix(cleaned, "/") {
		return "", fmt.Errorf("tutorial manifest path contains traversal")
	}
	base.Path = path.Join(base.Path, cleaned)
	base.RawPath = ""
	base.RawQuery = ""
	base.Fragment = ""
	return base.String(), nil
}

func writeTutorialError(c *app.RequestContext, err error) {
	switch {
	case errors.Is(err, tutorial.ErrTemplateNotFound):
		writeError(c, consts.StatusNotFound, "tutorial_not_found", "tutorial not found")
	case errors.Is(err, tutorial.ErrVersionNotFound):
		writeError(c, consts.StatusNotFound, "tutorial_version_not_found", "tutorial version not found")
	default:
		writeError(c, consts.StatusInternalServerError, "tutorial_catalog_failed", "tutorial catalog is unavailable")
	}
}
