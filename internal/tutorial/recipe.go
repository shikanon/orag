package tutorial

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var (
	ErrRecipeInvalid = errors.New("tutorial recipe is invalid")
	recipeRevision   = regexp.MustCompile(`^[0-9a-f]{40}$`)
)

const (
	ViDoSeekDataset  = "Qiuchen-Wang/ViDoSeek"
	ViDoSeekRevision = "e91a92ba5f38690696c7e66be5c5474b54c6e791"
)

// RecipeManifest declares upstream inputs that are fetched directly into a
// project-private store. Unlike Manifest, it never authorizes public Pack
// object download or object-store coordinates.
type RecipeManifest struct {
	TemplateID string                `json:"template_id"`
	Version    string                `json:"version"`
	Tier       string                `json:"tier"`
	License    License               `json:"license"`
	Source     RecipeSource          `json:"source"`
	Runtime    VisualRuntimeManifest `json:"runtime"`
}

type RecipeSource struct {
	Dataset  string               `json:"dataset"`
	Revision string               `json:"revision"`
	Objects  []RecipeSourceObject `json:"objects"`
}

type RecipeSourceObject struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

// VisualRuntimeManifest is intentionally separate from RuntimeManifest. It
// declares only immutable visual experiment facts, never a caller-selected
// model, URL, storage path, or retrieval configuration.
type VisualRuntimeManifest struct {
	Baseline VisualRuntimeBaseline `json:"baseline"`
	Pages    []VisualRuntimePage   `json:"pages"`
	Dataset  RuntimeDataset        `json:"dataset"`
}

type VisualRuntimeBaseline struct {
	Profile string `json:"profile"`
	TopK    int    `json:"top_k"`
}

type VisualRuntimePage struct {
	Document string `json:"document"`
	Page     int    `json:"page"`
	Evidence string `json:"evidence"`
}

// ParseRecipe validates a public visual Recipe before the clone worker is
// permitted to open an upstream connection.
func ParseRecipe(raw []byte, template Template, pack PackRef) (RecipeManifest, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var recipe RecipeManifest
	if err := decoder.Decode(&recipe); err != nil {
		return RecipeManifest{}, fmt.Errorf("%w: decode: %v", ErrRecipeInvalid, err)
	}
	if decoder.More() {
		return RecipeManifest{}, fmt.Errorf("%w: multiple JSON values", ErrRecipeInvalid)
	}
	if template.Modality != ModalityVisualDocument || recipe.TemplateID != template.ID || recipe.Version != template.Version || recipe.Tier != pack.Tier {
		return RecipeManifest{}, fmt.Errorf("%w: template identity", ErrRecipeInvalid)
	}
	if !validLicense(recipe.License) || !recipe.License.Redistributable || recipe.Source.Dataset != ViDoSeekDataset || recipe.Source.Revision != ViDoSeekRevision || !recipeRevision.MatchString(recipe.Source.Revision) {
		return RecipeManifest{}, fmt.Errorf("%w: source identity", ErrRecipeInvalid)
	}
	if len(recipe.Source.Objects) == 0 || len(recipe.Source.Objects) > 4 {
		return RecipeManifest{}, fmt.Errorf("%w: source objects", ErrRecipeInvalid)
	}
	seen := make(map[string]struct{}, len(recipe.Source.Objects))
	var total int64
	for _, object := range recipe.Source.Objects {
		if !validRecipeObject(object) || total > maxManifestBytes-object.Bytes {
			return RecipeManifest{}, fmt.Errorf("%w: source object", ErrRecipeInvalid)
		}
		if _, duplicate := seen[object.Path]; duplicate {
			return RecipeManifest{}, fmt.Errorf("%w: duplicate source object", ErrRecipeInvalid)
		}
		seen[object.Path] = struct{}{}
		total += object.Bytes
	}
	if pack.EstimatedBytes > 0 && total > pack.EstimatedBytes {
		return RecipeManifest{}, fmt.Errorf("%w: source bytes", ErrRecipeInvalid)
	}
	if err := validateVisualRuntime(recipe.Runtime, seen); err != nil {
		return RecipeManifest{}, err
	}
	return recipe, nil
}

func validRecipeObject(object RecipeSourceObject) bool {
	if object.Path != "vidoseek_pdf_document.zip" && object.Path != "vidoseek.json" {
		return false
	}
	return object.Bytes > 0 && object.Bytes <= maxManifestBytes && sha256Pattern.MatchString(object.SHA256)
}

func validateVisualRuntime(runtime VisualRuntimeManifest, objects map[string]struct{}) error {
	if runtime.Baseline.Profile != "visual_page" || runtime.Baseline.TopK < 1 || runtime.Baseline.TopK > 100 {
		return fmt.Errorf("%w: visual baseline", ErrRecipeInvalid)
	}
	if len(runtime.Pages) == 0 || len(runtime.Pages) > maxManifestObjects || len(runtime.Dataset.Items) == 0 || len(runtime.Dataset.Items) > maxManifestObjects || strings.TrimSpace(runtime.Dataset.Name) == "" {
		return fmt.Errorf("%w: visual runtime", ErrRecipeInvalid)
	}
	if _, ok := objects["vidoseek_pdf_document.zip"]; !ok {
		return fmt.Errorf("%w: visual archive missing", ErrRecipeInvalid)
	}
	if _, ok := objects["vidoseek.json"]; !ok {
		return fmt.Errorf("%w: visual annotations missing", ErrRecipeInvalid)
	}
	for _, page := range runtime.Pages {
		if strings.TrimSpace(page.Document) == "" || page.Page < 1 || strings.TrimSpace(page.Evidence) == "" {
			return fmt.Errorf("%w: visual page", ErrRecipeInvalid)
		}
	}
	for _, item := range runtime.Dataset.Items {
		if strings.TrimSpace(item.Query) == "" || strings.TrimSpace(item.GroundTruth) == "" {
			return fmt.Errorf("%w: visual dataset", ErrRecipeInvalid)
		}
	}
	return nil
}
