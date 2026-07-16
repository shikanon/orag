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

const (
	TutorialP1StructuredJSONCandidateID = "p1_structured_json"
	TutorialP1DocumentParserChapter     = "p1_document_parser"
	TutorialStructuredJSONParserMethod  = "structured_json"
	TutorialP2RecursiveChunkCandidateID = "p2_recursive_400_80"
	TutorialP2ChunkingChapter           = "p2_chunking"
	TutorialP3ContextualCandidateID     = "p3_contextual_retrieval"
	TutorialP3ContextualChapter         = "p3_contextual_retrieval"
	TutorialP4SparseCandidateID         = "p4_sparse_retrieval"
	TutorialP4SparseChapter             = "p4_sparse_retrieval"
	TutorialP5MultiQueryCandidateID     = "p5_multi_query_retrieval"
	TutorialP5MultiQueryChapter         = "p5_multi_query_retrieval"
	TutorialP6RerankCandidateID         = "p6_rerank_retrieval"
	TutorialP6RerankChapter             = "p6_rerank_retrieval"
	TutorialP7GraphCandidateID          = "p7_graph_retrieval"
	TutorialP7GraphChapter              = "p7_graph_retrieval"
	TutorialP8ContextPackCandidateID    = "p8_context_pack"
	TutorialP8ContextPackChapter        = "p8_context_pack"
	TutorialRetrievalStrategyHybrid     = "hybrid"
	TutorialRetrievalStrategySparse     = "sparse"
	TutorialRetrievalStrategyGraph      = "graph"
	TutorialQueryExpansionNone          = "none"
	TutorialQueryExpansionMultiQuery    = "multi_query"
	TutorialP3ContextualPromptVersion   = "tutorial_contextual_v1"
	TutorialP3MaxDocumentChars          = 12_000
	TutorialP3MaxChunkChars             = 2_000
	TutorialP3MaxContextChars           = 500
	TutorialBaselineChunkSizeTokens     = 800
	TutorialBaselineChunkOverlapTokens  = 120
	TutorialP2ChunkSizeTokens           = 400
	TutorialP2ChunkOverlapTokens        = 80
	TutorialBaselineContextPackTopN     = 5
	TutorialContextPackMaxTokens        = 6000
	TutorialP8ContextPackTopN           = 3
)

const TutorialP3ContextualSystemPrompt = "You are preparing a retrieval chunk for a controlled RAG experiment. Return concise factual context that situates the chunk within the supplied document. Do not answer questions, add facts, or use markdown."

// Manifest describes one immutable, redistributable tutorial pack. It is
// validated against the selected catalog entry before any object is fetched.
type Manifest struct {
	TemplateID       string                  `json:"template_id"`
	Version          string                  `json:"version"`
	Tier             string                  `json:"tier"`
	License          License                 `json:"license"`
	Objects          []PackObject            `json:"objects"`
	Runtime          *RuntimeManifest        `json:"runtime,omitempty"`
	VisualRuntime    *VisualRuntimeManifest  `json:"visual_runtime,omitempty"`
	VisualAssets     []PackObject            `json:"visual_assets,omitempty"`
	VideoProtocol    *VideoBenchmarkProtocol `json:"video_protocol,omitempty"`
	VideoSource      *VideoSource            `json:"video_source,omitempty"`
	TemporalSegments []TemporalSegment       `json:"temporal_segments,omitempty"`
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
	Baseline   RuntimeBaseline    `json:"baseline"`
	Documents  []RuntimeDocument  `json:"documents"`
	Dataset    RuntimeDataset     `json:"dataset"`
	Candidates []RuntimeCandidate `json:"candidates,omitempty"`
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

// RuntimeCandidate declares one immutable experiment variant. It deliberately
// contains no client-configurable model, retrieval, or storage settings.
type RuntimeCandidate struct {
	ID                    string `json:"id"`
	Chapter               string `json:"chapter"`
	ParserMethod          string `json:"parser_method"`
	ChunkSizeTokens       int    `json:"chunk_size_tokens,omitempty"`
	ChunkOverlapTokens    int    `json:"chunk_overlap_tokens,omitempty"`
	ContextualRetrieval   bool   `json:"contextual_retrieval,omitempty"`
	RetrievalStrategy     string `json:"retrieval_strategy,omitempty"`
	ReuseBaselineIndex    bool   `json:"reuse_baseline_index,omitempty"`
	MultiQueryCount       int    `json:"multi_query_count,omitempty"`
	RerankEnabled         bool   `json:"rerank_enabled,omitempty"`
	GraphRetrievalEnabled bool   `json:"graph_retrieval_enabled,omitempty"`
	ContextPackTopN       int    `json:"context_pack_top_n,omitempty"`
	ContextPackMaxTokens  int    `json:"context_pack_max_tokens,omitempty"`
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

	objectsByPath := make(map[string]PackObject, len(manifest.Objects))
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
		if _, exists := objectsByPath[object.Path]; exists {
			return Manifest{}, fmt.Errorf("%w: duplicate object path", ErrManifestInvalid)
		}
		objectsByPath[object.Path] = object
		total += object.Bytes
	}
	if pack.EstimatedBytes > 0 && total > pack.EstimatedBytes {
		return Manifest{}, fmt.Errorf("%w: object bytes exceed catalog estimate", ErrManifestInvalid)
	}
	if manifest.Runtime != nil {
		if err := validateRuntimeManifest(*manifest.Runtime, template, objectsByPath); err != nil {
			return Manifest{}, err
		}
	}
	return cloneManifest(manifest), nil
}

func validateRuntimeManifest(runtime RuntimeManifest, template Template, objectsByPath map[string]PackObject) error {
	if template.Modality != ModalityText {
		return fmt.Errorf("%w: runtime declarations are only supported for text packs", ErrManifestInvalid)
	}
	if !validRuntimeProfile(runtime.Baseline.Profile) || runtime.Baseline.TopK < 1 || runtime.Baseline.TopK > 100 {
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
		if _, found := objectsByPath[document.ObjectPath]; !found {
			return fmt.Errorf("%w: runtime document %d is not a Pack object", ErrManifestInvalid, index)
		}
		if _, duplicate := seenDocuments[document.ObjectPath]; duplicate {
			return fmt.Errorf("%w: duplicate runtime document", ErrManifestInvalid)
		}
		seenDocuments[document.ObjectPath] = struct{}{}
	}
	if err := validateRuntimeCandidates(runtime, objectsByPath); err != nil {
		return err
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

func validRuntimeProfile(profile string) bool {
	return profile == "realtime" || profile == "high_precision"
}

func validateRuntimeCandidates(runtime RuntimeManifest, objectsByPath map[string]PackObject) error {
	seen := make(map[string]struct{}, len(runtime.Candidates))
	for index, candidate := range runtime.Candidates {
		if _, duplicate := seen[candidate.ID]; duplicate {
			return fmt.Errorf("%w: duplicate runtime candidate", ErrManifestInvalid)
		}
		seen[candidate.ID] = struct{}{}
		switch {
		case validP1Candidate(candidate):
			hasJSONDocument := false
			for _, document := range runtime.Documents {
				object := objectsByPath[document.ObjectPath]
				if strings.EqualFold(object.ContentType, "application/json") || strings.HasSuffix(strings.ToLower(object.Path), ".json") {
					hasJSONDocument = true
					break
				}
			}
			if !hasJSONDocument {
				return fmt.Errorf("%w: runtime candidate %d requires a JSON document", ErrManifestInvalid, index)
			}
		case validP2Candidate(candidate):
			continue
		case validP3Candidate(candidate):
			continue
		case validP4Candidate(candidate):
			continue
		case validP5Candidate(candidate):
			continue
		case validP6Candidate(candidate):
			continue
		case validP7Candidate(candidate):
			continue
		case validP8Candidate(candidate):
			continue
		default:
			return fmt.Errorf("%w: runtime candidate %d is unsupported", ErrManifestInvalid, index)
		}
	}
	return nil
}

func validP1Candidate(candidate RuntimeCandidate) bool {
	return candidate.ID == TutorialP1StructuredJSONCandidateID &&
		candidate.Chapter == TutorialP1DocumentParserChapter &&
		candidate.ParserMethod == TutorialStructuredJSONParserMethod &&
		candidate.ChunkSizeTokens == 0 && candidate.ChunkOverlapTokens == 0 && !candidate.ContextualRetrieval && candidate.RetrievalStrategy == "" && !candidate.ReuseBaselineIndex && candidate.MultiQueryCount == 0 && !candidate.RerankEnabled && !candidate.GraphRetrievalEnabled && candidate.ContextPackTopN == 0 && candidate.ContextPackMaxTokens == 0
}

func validP2Candidate(candidate RuntimeCandidate) bool {
	return candidate.ID == TutorialP2RecursiveChunkCandidateID &&
		candidate.Chapter == TutorialP2ChunkingChapter &&
		candidate.ParserMethod == "basic" &&
		candidate.ChunkSizeTokens == TutorialP2ChunkSizeTokens &&
		candidate.ChunkOverlapTokens == TutorialP2ChunkOverlapTokens &&
		!candidate.ContextualRetrieval && candidate.RetrievalStrategy == "" && !candidate.ReuseBaselineIndex && candidate.MultiQueryCount == 0 && !candidate.RerankEnabled && !candidate.GraphRetrievalEnabled && candidate.ContextPackTopN == 0 && candidate.ContextPackMaxTokens == 0
}

func validP3Candidate(candidate RuntimeCandidate) bool {
	return candidate.ID == TutorialP3ContextualCandidateID &&
		candidate.Chapter == TutorialP3ContextualChapter &&
		candidate.ParserMethod == "basic" &&
		candidate.ChunkSizeTokens == TutorialBaselineChunkSizeTokens &&
		candidate.ChunkOverlapTokens == TutorialBaselineChunkOverlapTokens &&
		candidate.ContextualRetrieval && candidate.RetrievalStrategy == "" && !candidate.ReuseBaselineIndex && candidate.MultiQueryCount == 0 && !candidate.RerankEnabled && !candidate.GraphRetrievalEnabled && candidate.ContextPackTopN == 0 && candidate.ContextPackMaxTokens == 0
}

func validP4Candidate(candidate RuntimeCandidate) bool {
	return candidate.ID == TutorialP4SparseCandidateID &&
		candidate.Chapter == TutorialP4SparseChapter &&
		candidate.ParserMethod == "basic" &&
		candidate.ChunkSizeTokens == TutorialBaselineChunkSizeTokens &&
		candidate.ChunkOverlapTokens == TutorialBaselineChunkOverlapTokens &&
		!candidate.ContextualRetrieval &&
		candidate.RetrievalStrategy == TutorialRetrievalStrategySparse &&
		candidate.ReuseBaselineIndex && candidate.MultiQueryCount == 0 && !candidate.RerankEnabled && !candidate.GraphRetrievalEnabled && candidate.ContextPackTopN == 0 && candidate.ContextPackMaxTokens == 0
}

func validP5Candidate(candidate RuntimeCandidate) bool {
	return candidate.ID == TutorialP5MultiQueryCandidateID &&
		candidate.Chapter == TutorialP5MultiQueryChapter &&
		candidate.ParserMethod == "basic" &&
		candidate.ChunkSizeTokens == TutorialBaselineChunkSizeTokens &&
		candidate.ChunkOverlapTokens == TutorialBaselineChunkOverlapTokens &&
		!candidate.ContextualRetrieval &&
		candidate.RetrievalStrategy == TutorialRetrievalStrategyHybrid &&
		candidate.ReuseBaselineIndex && candidate.MultiQueryCount == 3 && !candidate.RerankEnabled && !candidate.GraphRetrievalEnabled && candidate.ContextPackTopN == 0 && candidate.ContextPackMaxTokens == 0
}

func validP6Candidate(candidate RuntimeCandidate) bool {
	return candidate.ID == TutorialP6RerankCandidateID &&
		candidate.Chapter == TutorialP6RerankChapter &&
		candidate.ParserMethod == "basic" &&
		candidate.ChunkSizeTokens == TutorialBaselineChunkSizeTokens &&
		candidate.ChunkOverlapTokens == TutorialBaselineChunkOverlapTokens &&
		!candidate.ContextualRetrieval &&
		candidate.RetrievalStrategy == TutorialRetrievalStrategyHybrid &&
		candidate.ReuseBaselineIndex && candidate.MultiQueryCount == 0 && candidate.RerankEnabled && !candidate.GraphRetrievalEnabled && candidate.ContextPackTopN == 0 && candidate.ContextPackMaxTokens == 0
}

func validP7Candidate(candidate RuntimeCandidate) bool {
	return candidate.ID == TutorialP7GraphCandidateID && candidate.Chapter == TutorialP7GraphChapter &&
		candidate.ParserMethod == "basic" && candidate.ChunkSizeTokens == TutorialBaselineChunkSizeTokens &&
		candidate.ChunkOverlapTokens == TutorialBaselineChunkOverlapTokens && !candidate.ContextualRetrieval &&
		candidate.RetrievalStrategy == TutorialRetrievalStrategyGraph && !candidate.ReuseBaselineIndex &&
		candidate.MultiQueryCount == 0 && !candidate.RerankEnabled && candidate.GraphRetrievalEnabled && candidate.ContextPackTopN == 0 && candidate.ContextPackMaxTokens == 0
}

func validP8Candidate(candidate RuntimeCandidate) bool {
	return candidate.ID == TutorialP8ContextPackCandidateID && candidate.Chapter == TutorialP8ContextPackChapter &&
		candidate.ParserMethod == "basic" && candidate.ChunkSizeTokens == TutorialBaselineChunkSizeTokens &&
		candidate.ChunkOverlapTokens == TutorialBaselineChunkOverlapTokens && !candidate.ContextualRetrieval &&
		candidate.RetrievalStrategy == TutorialRetrievalStrategyHybrid && candidate.ReuseBaselineIndex &&
		candidate.MultiQueryCount == 0 && !candidate.RerankEnabled && !candidate.GraphRetrievalEnabled &&
		candidate.ContextPackTopN == TutorialP8ContextPackTopN && candidate.ContextPackMaxTokens == TutorialContextPackMaxTokens
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
		runtime.Candidates = slices.Clone(manifest.Runtime.Candidates)
		runtime.Dataset.Items = slices.Clone(manifest.Runtime.Dataset.Items)
		for index := range runtime.Dataset.Items {
			runtime.Dataset.Items[index].ExpectedEvidence = slices.Clone(manifest.Runtime.Dataset.Items[index].ExpectedEvidence)
		}
		cloned.Runtime = &runtime
	}
	if manifest.VisualRuntime != nil {
		visual := *manifest.VisualRuntime
		visual.Pages = slices.Clone(manifest.VisualRuntime.Pages)
		visual.Dataset.Items = slices.Clone(manifest.VisualRuntime.Dataset.Items)
		for index := range visual.Dataset.Items {
			visual.Dataset.Items[index].ExpectedEvidence = slices.Clone(manifest.VisualRuntime.Dataset.Items[index].ExpectedEvidence)
		}
		cloned.VisualRuntime = &visual
	}
	cloned.VisualAssets = slices.Clone(manifest.VisualAssets)
	if manifest.VideoProtocol != nil {
		protocol := *manifest.VideoProtocol
		cloned.VideoProtocol = &protocol
	}
	if manifest.VideoSource != nil {
		source := *manifest.VideoSource
		source.Subtitles = slices.Clone(manifest.VideoSource.Subtitles)
		cloned.VideoSource = &source
	}
	cloned.TemporalSegments = slices.Clone(manifest.TemporalSegments)
	return cloned
}
