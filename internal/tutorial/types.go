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

// ReplaySnapshot is an immutable, public summary of an official controlled
// tutorial run. It intentionally contains aggregate facts only: no user data,
// queries, answers, storage coordinates, or credentials are part of this API.
type ReplaySnapshot struct {
	ID                       string        `json:"id"`
	TemplateID               string        `json:"template_id"`
	TemplateVersion          string        `json:"template_version"`
	PackTier                 string        `json:"pack_tier"`
	PackManifestSHA256       string        `json:"pack_manifest_sha256"`
	RuntimeEnvironmentSHA256 string        `json:"runtime_environment_sha256"`
	BuildRevision            string        `json:"build_revision"`
	EvaluatorVersion         string        `json:"evaluator_version"`
	GeneratedAt              string        `json:"generated_at"`
	Summary                  string        `json:"summary"`
	Baseline                 ReplayVariant `json:"baseline"`
	Candidate                ReplayVariant `json:"candidate"`
	Fingerprint              string        `json:"fingerprint"`
}

type ReplayVariant struct {
	Variant              string         `json:"variant"`
	Profile              string         `json:"profile"`
	TopK                 int            `json:"top_k"`
	ContextPackTopN      int            `json:"context_pack_top_n"`
	ContextPackMaxTokens int            `json:"context_pack_max_tokens"`
	Metrics              []ReplayMetric `json:"metrics"`
	IndexMetrics         []ReplayMetric `json:"index_metrics"`
}

type ReplayMetric struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
}

var (
	ErrTemplateNotFound = errors.New("tutorial template not found")
	ErrVersionNotFound  = errors.New("tutorial template version not found")
	ErrReplayNotFound   = errors.New("tutorial replay not found")
)
