package tutorial

import "errors"

type Modality string

const (
	ModalityText           Modality = "text"
	ModalityVisualDocument Modality = "visual_document"
	ModalityVideo          Modality = "video"
)

type PackRef struct {
	Tier                 string `json:"tier"`
	ManifestPath         string `json:"manifest_path"`
	EstimatedBytes       int64  `json:"estimated_bytes"`
	EstimatedMinutes     int    `json:"estimated_minutes"`
	RequiresLicenseCheck bool   `json:"requires_license_check"`
}

type Template struct {
	ID                       string    `json:"id"`
	Slug                     string    `json:"slug"`
	Title                    string    `json:"title"`
	Summary                  string    `json:"summary"`
	Version                  string    `json:"version"`
	Status                   string    `json:"status"`
	Modality                 Modality  `json:"modality"`
	Difficulty               string    `json:"difficulty"`
	EstimatedDurationMinutes int       `json:"estimated_duration_minutes"`
	SourceBenchmark          string    `json:"source_benchmark"`
	SourceURL                string    `json:"source_url"`
	ScenarioDimensions       []string  `json:"scenario_dimensions"`
	PipelineStages           []string  `json:"pipeline_stages"`
	RequiredCapabilities     []string  `json:"required_capabilities"`
	Packs                    []PackRef `json:"packs"`
	ReplayAvailable          bool      `json:"replay_available"`
}

var (
	ErrTemplateNotFound = errors.New("tutorial template not found")
	ErrVersionNotFound  = errors.New("tutorial template version not found")
)
