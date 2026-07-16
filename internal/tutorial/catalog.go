package tutorial

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

//go:embed catalog.json
var embeddedCatalog []byte

var semanticVersionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

type Catalog struct {
	byID    map[string]map[string]Template
	replays map[string]ReplaySnapshot
}

func NewCatalog() (*Catalog, error) {
	decoder := json.NewDecoder(bytes.NewReader(embeddedCatalog))
	decoder.DisallowUnknownFields()
	var templates []Template
	if err := decoder.Decode(&templates); err != nil {
		return nil, fmt.Errorf("decode tutorial catalog: %w", err)
	}
	catalog, err := newCatalog(templates)
	if err != nil {
		return nil, err
	}
	if err := catalog.loadOfficialReplays(); err != nil {
		return nil, err
	}
	return catalog, nil
}

func newCatalog(templates []Template) (*Catalog, error) {
	catalog := &Catalog{byID: make(map[string]map[string]Template), replays: make(map[string]ReplaySnapshot)}
	for index, template := range templates {
		if err := validateTemplate(template); err != nil {
			return nil, fmt.Errorf("tutorial catalog entry %d: %w", index, err)
		}
		versions := catalog.byID[template.ID]
		if versions == nil {
			versions = make(map[string]Template)
			catalog.byID[template.ID] = versions
		}
		if _, exists := versions[template.Version]; exists {
			return nil, fmt.Errorf("duplicate tutorial %q version %q", template.ID, template.Version)
		}
		versions[template.Version] = cloneTemplate(template)
	}
	return catalog, nil
}

// Replay returns the immutable official snapshot for the latest available
// version of a template. A catalog entry is not enough to make Replay
// available; the separately validated snapshot must exist as well.
func (c *Catalog) Replay(id string) (ReplaySnapshot, error) {
	if c == nil {
		return ReplaySnapshot{}, ErrTemplateNotFound
	}
	if _, err := c.Get(id, ""); err != nil {
		return ReplaySnapshot{}, err
	}
	replay, ok := c.replays[id]
	if !ok {
		return ReplaySnapshot{}, ErrReplayNotFound
	}
	return cloneReplay(replay), nil
}

func (c *Catalog) List() []Template {
	if c == nil {
		return nil
	}
	items := make([]Template, 0, len(c.byID))
	for id := range c.byID {
		item, err := c.Get(id, "")
		if err == nil {
			items = append(items, item)
		}
	}
	slices.SortFunc(items, func(a, b Template) int { return strings.Compare(a.ID, b.ID) })
	return items
}

func (c *Catalog) Get(id, version string) (Template, error) {
	if c == nil {
		return Template{}, ErrTemplateNotFound
	}
	versions, ok := c.byID[id]
	if !ok {
		return Template{}, ErrTemplateNotFound
	}
	if version == "" {
		for candidate := range versions {
			if version == "" || compareVersions(candidate, version) > 0 {
				version = candidate
			}
		}
	}
	template, ok := versions[version]
	if !ok {
		return Template{}, ErrVersionNotFound
	}
	return cloneTemplate(template), nil
}

func validateTemplate(template Template) error {
	if strings.TrimSpace(template.ID) == "" || strings.TrimSpace(template.Slug) == "" {
		return fmt.Errorf("id and slug are required")
	}
	if template.ID != template.Slug {
		return fmt.Errorf("id %q must match slug %q", template.ID, template.Slug)
	}
	if strings.TrimSpace(template.Title) == "" || strings.TrimSpace(template.Summary) == "" {
		return fmt.Errorf("title and summary are required")
	}
	if !semanticVersionPattern.MatchString(template.Version) {
		return fmt.Errorf("version %q is not semantic major.minor.patch", template.Version)
	}
	if template.Status != "published" {
		return fmt.Errorf("status %q is not published", template.Status)
	}
	if !approvedModality(template.Modality) {
		return fmt.Errorf("modality %q is not approved", template.Modality)
	}
	if template.EstimatedDurationMinutes <= 0 {
		return fmt.Errorf("estimated duration must be positive")
	}
	if strings.TrimSpace(template.SourceBenchmark) == "" {
		return fmt.Errorf("source benchmark is required")
	}
	sourceURL, err := url.Parse(template.SourceURL)
	if err != nil || sourceURL.Scheme != "https" || sourceURL.Host == "" {
		return fmt.Errorf("source URL %q must be absolute HTTPS", template.SourceURL)
	}
	if len(template.ScenarioDimensions) == 0 || len(template.PipelineStages) == 0 || len(template.RequiredCapabilities) == 0 {
		return fmt.Errorf("scenario dimensions, pipeline stages, and required capabilities are required")
	}
	tiers := make(map[string]bool, len(template.Packs))
	for _, pack := range template.Packs {
		if pack.Tier != "quick" && pack.Tier != "benchmark" {
			return fmt.Errorf("pack tier %q is not supported", pack.Tier)
		}
		if tiers[pack.Tier] {
			return fmt.Errorf("duplicate pack tier %q", pack.Tier)
		}
		tiers[pack.Tier] = true
		if !validPackArtifactPath(pack.ManifestPath, template.Modality) {
			return fmt.Errorf("pack artifact path %q must be relative and traversal-free", pack.ManifestPath)
		}
	}
	if !tiers["quick"] || !tiers["benchmark"] {
		return fmt.Errorf("quick and benchmark packs are required")
	}
	return nil
}

func approvedModality(modality Modality) bool {
	return modality == ModalityText || modality == ModalityVisualDocument || modality == ModalityVideo
}

func validManifestPath(value string) bool {
	if value == "" || strings.HasPrefix(value, "/") || strings.Contains(value, "\\") {
		return false
	}
	cleaned := path.Clean(value)
	return cleaned == value && cleaned != "." && !strings.HasPrefix(cleaned, "../") && strings.HasSuffix(cleaned, "/manifest.json")
}

func validPackArtifactPath(value string, modality Modality) bool {
	if modality == ModalityVideo {
		if value == "" || strings.HasPrefix(value, "/") || strings.Contains(value, "\\") {
			return false
		}
		cleaned := path.Clean(value)
		return cleaned == value && cleaned != "." && !strings.HasPrefix(cleaned, "../") && strings.HasSuffix(cleaned, "/protocol.json")
	}
	return validManifestPath(value)
}

func compareVersions(a, b string) int {
	aParts := semanticVersionPattern.FindStringSubmatch(a)
	bParts := semanticVersionPattern.FindStringSubmatch(b)
	for index := 1; index <= 3; index++ {
		aValue, _ := strconv.Atoi(aParts[index])
		bValue, _ := strconv.Atoi(bParts[index])
		if aValue < bValue {
			return -1
		}
		if aValue > bValue {
			return 1
		}
	}
	return 0
}

func cloneTemplate(template Template) Template {
	cloned := template
	cloned.ScenarioDimensions = slices.Clone(template.ScenarioDimensions)
	cloned.PipelineStages = slices.Clone(template.PipelineStages)
	cloned.RequiredCapabilities = slices.Clone(template.RequiredCapabilities)
	cloned.Packs = slices.Clone(template.Packs)
	return cloned
}
