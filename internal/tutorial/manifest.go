package tutorial

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strings"
)

const (
	maxManifestObjects = 10_000
	maxManifestBytes   = 32 << 30
)

var (
	ErrManifestInvalid = errors.New("tutorial pack manifest is invalid")
	sha256Pattern      = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// Manifest describes one immutable, redistributable tutorial pack. It is
// validated against the selected catalog entry before any object is fetched.
type Manifest struct {
	TemplateID string           `json:"template_id"`
	Version    string           `json:"version"`
	Tier       string           `json:"tier"`
	License    License          `json:"license"`
	Objects    []PackObject     `json:"objects"`
	Runtime    *RuntimeManifest `json:"runtime,omitempty"`
}

type License struct {
	SPDX            string `json:"spdx"`
	SourceURL       string `json:"source_url"`
	Redistributable bool   `json:"redistributable"`
}

type PackObject struct {
	Path        string `json:"path"`
	SHA256      string `json:"sha256"`
	Bytes       int64  `json:"bytes"`
	ContentType string `json:"content_type"`
}

// RuntimeManifest is an optional, immutable declaration of the resource roots
// a Pack may create. It deliberately contains no object-storage location,
// model provider, or arbitrary client configuration. A missing declaration
// leaves the Pack installable but makes Live Run unavailable.
type RuntimeManifest struct {
	Baseline  RuntimeBaseline   `json:"baseline"`
	Documents []RuntimeDocument `json:"documents"`
	Dataset   RuntimeDataset    `json:"dataset"`
}

type RuntimeBaseline struct {
	Profile string `json:"profile"`
	TopK    int    `json:"top_k"`
}

type RuntimeDocument struct {
	ObjectPath string `json:"object_path"`
	Name       string `json:"name"`
}

type RuntimeDataset struct {
	Name  string               `json:"name"`
	Items []RuntimeDatasetItem `json:"items"`
}

type RuntimeDatasetItem struct {
	Query            string   `json:"query"`
	GroundTruth      string   `json:"ground_truth"`
	ExpectedEvidence []string `json:"expected_evidence,omitempty"`
	Split            string   `json:"split,omitempty"`
	Weight           float64  `json:"weight,omitempty"`
}

// ParseManifest rejects malformed or mismatched data before a clone worker
// has a chance to read any remote object. The template and pack come from the
// read-only catalog; callers never supply either of their identifying fields.
func ParseManifest(raw []byte, template Template, pack PackRef) (Manifest, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("%w: decode: %v", ErrManifestInvalid, err)
	}
	if decoder.More() {
		return Manifest{}, fmt.Errorf("%w: multiple JSON values", ErrManifestInvalid)
	}
	if strings.TrimSpace(manifest.TemplateID) != template.ID || strings.TrimSpace(manifest.Version) != template.Version || strings.TrimSpace(manifest.Tier) != pack.Tier {
		return Manifest{}, fmt.Errorf("%w: template, version, or tier does not match catalog", ErrManifestInvalid)
	}
	if !validLicense(manifest.License) {
		return Manifest{}, fmt.Errorf("%w: license must be redistributable with an HTTPS source", ErrManifestInvalid)
	}
	if len(manifest.Objects) == 0 || len(manifest.Objects) > maxManifestObjects {
		return Manifest{}, fmt.Errorf("%w: object count is outside permitted range", ErrManifestInvalid)
	}

	paths := make(map[string]struct{}, len(manifest.Objects))
	var total int64
	for index := range manifest.Objects {
		object := manifest.Objects[index]
		if !validObjectPath(object.Path) {
			return Manifest{}, fmt.Errorf("%w: object %d path is invalid", ErrManifestInvalid, index)
		}
		if !sha256Pattern.MatchString(object.SHA256) {
			return Manifest{}, fmt.Errorf("%w: object %d SHA-256 is invalid", ErrManifestInvalid, index)
		}
		if object.Bytes <= 0 || object.Bytes > maxManifestBytes || total > maxManifestBytes-object.Bytes {
			return Manifest{}, fmt.Errorf("%w: object %d byte size is invalid", ErrManifestInvalid, index)
		}
		if !allowedContentType(object.ContentType) {
			return Manifest{}, fmt.Errorf("%w: object %d content type is unsupported", ErrManifestInvalid, index)
		}
		if _, exists := paths[object.Path]; exists {
			return Manifest{}, fmt.Errorf("%w: duplicate object path", ErrManifestInvalid)
		}
		paths[object.Path] = struct{}{}
		total += object.Bytes
	}
	if pack.EstimatedBytes > 0 && total > pack.EstimatedBytes {
		return Manifest{}, fmt.Errorf("%w: object bytes exceed catalog estimate", ErrManifestInvalid)
	}
	if manifest.Runtime != nil {
		if err := validateRuntimeManifest(*manifest.Runtime, template, paths); err != nil {
			return Manifest{}, err
		}
	}
	return cloneManifest(manifest), nil
}

func validateRuntimeManifest(runtime RuntimeManifest, template Template, objectPaths map[string]struct{}) error {
	if template.Modality != ModalityText {
		return fmt.Errorf("%w: runtime declarations are only supported for text packs", ErrManifestInvalid)
	}
	if runtime.Baseline.Profile != "realtime" || runtime.Baseline.TopK < 1 || runtime.Baseline.TopK > 100 {
		return fmt.Errorf("%w: runtime baseline must use bounded realtime retrieval", ErrManifestInvalid)
	}
	if strings.TrimSpace(runtime.Dataset.Name) == "" || len(runtime.Dataset.Items) == 0 || len(runtime.Dataset.Items) > maxManifestObjects {
		return fmt.Errorf("%w: runtime dataset is invalid", ErrManifestInvalid)
	}
	if len(runtime.Documents) == 0 || len(runtime.Documents) > maxManifestObjects {
		return fmt.Errorf("%w: runtime documents are invalid", ErrManifestInvalid)
	}
	seenDocuments := make(map[string]struct{}, len(runtime.Documents))
	for index, document := range runtime.Documents {
		if strings.TrimSpace(document.Name) == "" || !validObjectPath(document.ObjectPath) {
			return fmt.Errorf("%w: runtime document %d is invalid", ErrManifestInvalid, index)
		}
		if _, found := objectPaths[document.ObjectPath]; !found {
			return fmt.Errorf("%w: runtime document %d is not a Pack object", ErrManifestInvalid, index)
		}
		if _, duplicate := seenDocuments[document.ObjectPath]; duplicate {
			return fmt.Errorf("%w: duplicate runtime document", ErrManifestInvalid)
		}
		seenDocuments[document.ObjectPath] = struct{}{}
	}
	for index, item := range runtime.Dataset.Items {
		if strings.TrimSpace(item.Query) == "" || strings.TrimSpace(item.GroundTruth) == "" {
			return fmt.Errorf("%w: runtime dataset item %d is invalid", ErrManifestInvalid, index)
		}
		switch strings.TrimSpace(item.Split) {
		case "", "eval", "gold", "holdout", "train":
		default:
			return fmt.Errorf("%w: runtime dataset item %d split is invalid", ErrManifestInvalid, index)
		}
		if item.Weight < 0 {
			return fmt.Errorf("%w: runtime dataset item %d weight is invalid", ErrManifestInvalid, index)
		}
	}
	return nil
}

func validLicense(license License) bool {
	if strings.TrimSpace(license.SPDX) == "" || !license.Redistributable {
		return false
	}
	source, err := url.Parse(strings.TrimSpace(license.SourceURL))
	return err == nil && source.Scheme == "https" && source.Host != "" && source.User == nil
}

func validObjectPath(value string) bool {
	if value == "" || strings.HasPrefix(value, "/") || strings.Contains(value, "\\") {
		return false
	}
	decoded, err := url.PathUnescape(value)
	if err != nil || decoded != value {
		return false
	}
	cleaned := path.Clean(value)
	return cleaned == value && cleaned != "." && !strings.HasPrefix(cleaned, "../")
}

func allowedContentType(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "text/plain", "application/json", "application/jsonl", "application/pdf", "image/jpeg", "image/png", "video/mp4", "audio/mpeg", "audio/wav":
		return true
	default:
		return false
	}
}

func cloneManifest(manifest Manifest) Manifest {
	cloned := manifest
	cloned.Objects = slices.Clone(manifest.Objects)
	if manifest.Runtime != nil {
		runtime := *manifest.Runtime
		runtime.Documents = slices.Clone(manifest.Runtime.Documents)
		runtime.Dataset.Items = slices.Clone(manifest.Runtime.Dataset.Items)
		for index := range runtime.Dataset.Items {
			runtime.Dataset.Items[index].ExpectedEvidence = slices.Clone(manifest.Runtime.Dataset.Items[index].ExpectedEvidence)
		}
		cloned.Runtime = &runtime
	}
	return cloned
}
