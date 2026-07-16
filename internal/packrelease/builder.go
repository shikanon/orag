// Package packrelease builds immutable, auditable tutorial packs from a
// checked-out benchmark source. It deliberately has no storage credentials:
// building and publishing are separate operations.
package packrelease

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/shikanon/orag/internal/tutorial"
)

const (
	DefaultVersion       = "1.1.0"
	DefaultQuickMaxBytes = int64(64 << 20)
)

type BuildConfig struct {
	SourceDir     string
	OutputDir     string
	Version       string
	QuickMaxBytes int64
}

type Release struct {
	Root           string
	Version        string
	SourceCommit   string
	QuickBytes     int64
	BenchmarkBytes int64
}

type sourceRecord struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// Build creates quick and benchmark manifests plus a deterministic archive of
// the complete upstream data directory. Existing output is rejected so a
// release version can never be silently replaced.
func Build(config BuildConfig) (Release, error) {
	if config.Version == "" {
		config.Version = DefaultVersion
	}
	if config.QuickMaxBytes <= 0 {
		config.QuickMaxBytes = DefaultQuickMaxBytes
	}
	source, err := filepath.Abs(config.SourceDir)
	if err != nil {
		return Release{}, err
	}
	if _, err := os.Stat(filepath.Join(source, "data")); err != nil {
		return Release{}, fmt.Errorf("source data directory: %w", err)
	}
	commit, err := gitRevision(source)
	if err != nil {
		return Release{}, err
	}
	root := filepath.Join(config.OutputDir, "text-rag", config.Version)
	if _, err := os.Stat(root); err == nil {
		return Release{}, fmt.Errorf("release output already exists: %s", root)
	} else if !errors.Is(err, os.ErrNotExist) {
		return Release{}, err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return Release{}, err
	}
	paths, err := sourceFiles(source)
	if err != nil {
		return Release{}, err
	}
	if len(paths) == 0 {
		return Release{}, errors.New("benchmark source has no data files")
	}
	corpusPaths := textCorpusFiles(paths)
	if len(corpusPaths) == 0 {
		return Release{}, errors.New("benchmark source has no textual corpus files")
	}

	archivePath := filepath.Join(root, "source", "CRUD-RAG-"+commit[:12]+".tar.gz")
	if err := writeArchive(archivePath, source, paths); err != nil {
		return Release{}, err
	}
	dataset, err := extractDataset(filepath.Join(source, "data", "crud_split", "split_merged.json"))
	if err != nil {
		return Release{}, err
	}
	quickPath := filepath.Join(root, "quick", "corpus", "documents.json")
	benchmarkPath := filepath.Join(root, "benchmark", "corpus", "documents.json")
	quickBytes, err := writeCorpus(quickPath, source, corpusPaths, config.QuickMaxBytes)
	if err != nil {
		return Release{}, err
	}
	benchmarkBytes, err := writeCorpus(benchmarkPath, source, corpusPaths, 0)
	if err != nil {
		return Release{}, err
	}
	if err := writeManifest(filepath.Join(root, "quick", "manifest.json"), config.Version, "quick", quickBytes, dataset, "realtime", 5); err != nil {
		return Release{}, err
	}
	if err := writeManifest(filepath.Join(root, "benchmark", "manifest.json"), config.Version, "benchmark", benchmarkBytes, dataset, "high_precision", 8); err != nil {
		return Release{}, err
	}
	metadata := map[string]any{
		"format": "orag.tutorial-pack-source.v1", "template_id": "text-rag", "version": config.Version,
		"upstream": map[string]string{"repository": "https://github.com/IAAR-Shanghai/CRUD_RAG", "commit": commit, "data_root": "data"},
		"license":  map[string]any{"spdx": "Apache-2.0", "source_url": "https://github.com/IAAR-Shanghai/CRUD_RAG", "redistributable": true},
		"built_at": time.Unix(0, 0).UTC().Format(time.RFC3339),
	}
	if err := writeJSON(filepath.Join(root, "SOURCE.json"), metadata); err != nil {
		return Release{}, err
	}
	if err := writeSums(root); err != nil {
		return Release{}, err
	}
	return Release{Root: root, Version: config.Version, SourceCommit: commit, QuickBytes: quickBytes, BenchmarkBytes: benchmarkBytes}, nil
}

func gitRevision(source string) (string, error) {
	status := exec.Command("git", "-C", source, "status", "--porcelain")
	if output, err := status.Output(); err != nil {
		return "", fmt.Errorf("source must be a git checkout: %w", err)
	} else if len(output) != 0 {
		return "", errors.New("source checkout is dirty")
	}
	cmd := exec.Command("git", "-C", source, "rev-parse", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("source revision: %w", err)
	}
	commit := strings.TrimSpace(string(output))
	if len(commit) != 40 {
		return "", errors.New("source revision is invalid")
	}
	return commit, nil
}

func sourceFiles(source string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(filepath.Join(source, "data"), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(rel))
		return nil
	})
	sort.Strings(paths)
	return paths, err
}

// The archive contains every upstream data artifact. The runtime corpus keeps
// only directly indexable text/JSON inputs; binary ZIP snapshots remain
// available in the archive without being lossy-converted into text.
func textCorpusFiles(paths []string) []string {
	output := make([]string, 0, len(paths))
	for _, path := range paths {
		extension := strings.ToLower(filepath.Ext(path))
		if extension == ".zip" || extension == ".gz" || extension == ".tar" {
			continue
		}
		output = append(output, path)
	}
	return output
}

func writeArchive(destination, source string, paths []string) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	gz, err := gzip.NewWriterLevel(file, gzip.BestCompression)
	if err != nil {
		return err
	}
	gz.Header.ModTime = time.Unix(0, 0)
	tarWriter := tar.NewWriter(gz)
	for _, rel := range paths {
		info, err := os.Stat(filepath.Join(source, filepath.FromSlash(rel)))
		if err != nil {
			return err
		}
		header := &tar.Header{Name: rel, Mode: 0o644, Size: info.Size(), ModTime: time.Unix(0, 0).UTC(), Format: tar.FormatUSTAR}
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}
		input, err := os.Open(filepath.Join(source, filepath.FromSlash(rel)))
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(tarWriter, input)
		closeErr := input.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	if err := tarWriter.Close(); err != nil {
		return err
	}
	return gz.Close()
}

func writeCorpus(destination, source string, paths []string, maxBytes int64) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return 0, err
	}
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return 0, err
	}
	defer output.Close()
	if _, err := output.WriteString(`{"format":"orag.text-rag.documents.v1","documents":[`); err != nil {
		return 0, err
	}
	var written int64
	first := true
	for _, rel := range paths {
		input, err := os.Open(filepath.Join(source, filepath.FromSlash(rel)))
		if err != nil {
			return 0, err
		}
		content, readErr := io.ReadAll(input)
		closeErr := input.Close()
		if readErr != nil {
			return 0, readErr
		}
		if closeErr != nil {
			return 0, closeErr
		}
		encoded, err := json.Marshal(sourceRecord{Path: rel, Content: string(content)})
		if err != nil {
			return 0, err
		}
		if maxBytes > 0 && written > 0 && written+int64(len(encoded)) > maxBytes {
			break
		}
		if !first {
			if _, err := output.WriteString(","); err != nil {
				return 0, err
			}
		}
		first = false
		if _, err := output.Write(encoded); err != nil {
			return 0, err
		}
		written += int64(len(encoded))
	}
	if first {
		return 0, errors.New("quick corpus contains no source records")
	}
	if _, err := output.WriteString(`]}`); err != nil {
		return 0, err
	}
	info, err := output.Stat()
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func extractDataset(path string) ([]tutorial.RuntimeDatasetItem, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("decode split benchmark: %w", err)
	}
	items := make([]tutorial.RuntimeDatasetItem, 0, 1024)
	collectQuestions(value, &items)
	if len(items) == 0 {
		return nil, errors.New("split benchmark contains no question/answer pairs")
	}
	if len(items) > 10_000 {
		items = items[:10_000]
	}
	return items, nil
}

func collectQuestions(value any, output *[]tutorial.RuntimeDatasetItem) {
	switch typed := value.(type) {
	case map[string]any:
		if question, questionOK := typed["questions"].(string); questionOK {
			if answer, answerOK := typed["answers"].(string); answerOK && strings.TrimSpace(question) != "" && strings.TrimSpace(answer) != "" {
				*output = append(*output, tutorial.RuntimeDatasetItem{Query: question, GroundTruth: answer, Split: "eval"})
			}
		}
		questions, hasQuestions := stringList(typed["questions"])
		answers, hasAnswers := stringList(typed["answers"])
		if hasQuestions && hasAnswers {
			for i := 0; i < len(questions) && i < len(answers); i++ {
				if strings.TrimSpace(questions[i]) != "" && strings.TrimSpace(answers[i]) != "" {
					*output = append(*output, tutorial.RuntimeDatasetItem{Query: questions[i], GroundTruth: answers[i], Split: "eval"})
				}
			}
		}
		for _, child := range typed {
			collectQuestions(child, output)
		}
	case []any:
		for _, child := range typed {
			collectQuestions(child, output)
		}
	}
}

func stringList(value any) ([]string, bool) {
	items, ok := value.([]any)
	if !ok {
		return nil, false
	}
	output := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if !ok {
			return nil, false
		}
		output = append(output, text)
	}
	return output, true
}

func writeManifest(destination, version, tier string, corpusBytes int64, dataset []tutorial.RuntimeDatasetItem, profile string, topK int) error {
	hash, err := hashFile(filepath.Join(filepath.Dir(destination), "corpus", "documents.json"))
	if err != nil {
		return err
	}
	manifest := tutorial.Manifest{TemplateID: "text-rag", Version: version, Tier: tier, License: tutorial.License{SPDX: "Apache-2.0", SourceURL: "https://github.com/IAAR-Shanghai/CRUD_RAG", Redistributable: true}, Objects: []tutorial.PackObject{{Path: "corpus/documents.json", SHA256: hash, Bytes: corpusBytes, ContentType: "application/json"}}, Runtime: &tutorial.RuntimeManifest{Baseline: tutorial.RuntimeBaseline{Profile: profile, TopK: topK}, Documents: []tutorial.RuntimeDocument{{ObjectPath: "corpus/documents.json", Name: "CRUD-RAG 全量语料"}}, Dataset: tutorial.RuntimeDataset{Name: "CRUD-RAG 评测集", Items: dataset}, Candidates: candidates()}}
	if _, err := tutorial.ParseManifest(mustJSON(manifest), tutorial.Template{ID: "text-rag", Version: version, Modality: tutorial.ModalityText}, tutorial.PackRef{Tier: tier, EstimatedBytes: corpusBytes}); err != nil {
		return fmt.Errorf("generated manifest: %w", err)
	}
	return writeJSON(destination, manifest)
}

func candidates() []tutorial.RuntimeCandidate {
	return []tutorial.RuntimeCandidate{
		{ID: tutorial.TutorialP1StructuredJSONCandidateID, Chapter: tutorial.TutorialP1DocumentParserChapter, ParserMethod: tutorial.TutorialStructuredJSONParserMethod},
		{ID: tutorial.TutorialP2RecursiveChunkCandidateID, Chapter: tutorial.TutorialP2ChunkingChapter, ParserMethod: "basic", ChunkSizeTokens: tutorial.TutorialP2ChunkSizeTokens, ChunkOverlapTokens: tutorial.TutorialP2ChunkOverlapTokens},
		{ID: tutorial.TutorialP3ContextualCandidateID, Chapter: tutorial.TutorialP3ContextualChapter, ParserMethod: "basic", ChunkSizeTokens: tutorial.TutorialBaselineChunkSizeTokens, ChunkOverlapTokens: tutorial.TutorialBaselineChunkOverlapTokens, ContextualRetrieval: true},
		{ID: tutorial.TutorialP4SparseCandidateID, Chapter: tutorial.TutorialP4SparseChapter, ParserMethod: "basic", ChunkSizeTokens: tutorial.TutorialBaselineChunkSizeTokens, ChunkOverlapTokens: tutorial.TutorialBaselineChunkOverlapTokens, RetrievalStrategy: tutorial.TutorialRetrievalStrategySparse, ReuseBaselineIndex: true},
		{ID: tutorial.TutorialP5MultiQueryCandidateID, Chapter: tutorial.TutorialP5MultiQueryChapter, ParserMethod: "basic", ChunkSizeTokens: tutorial.TutorialBaselineChunkSizeTokens, ChunkOverlapTokens: tutorial.TutorialBaselineChunkOverlapTokens, RetrievalStrategy: tutorial.TutorialRetrievalStrategyHybrid, ReuseBaselineIndex: true, MultiQueryCount: 3},
		{ID: tutorial.TutorialP6RerankCandidateID, Chapter: tutorial.TutorialP6RerankChapter, ParserMethod: "basic", ChunkSizeTokens: tutorial.TutorialBaselineChunkSizeTokens, ChunkOverlapTokens: tutorial.TutorialBaselineChunkOverlapTokens, RetrievalStrategy: tutorial.TutorialRetrievalStrategyHybrid, ReuseBaselineIndex: true, RerankEnabled: true},
		{ID: tutorial.TutorialP7GraphCandidateID, Chapter: tutorial.TutorialP7GraphChapter, ParserMethod: "basic", ChunkSizeTokens: tutorial.TutorialBaselineChunkSizeTokens, ChunkOverlapTokens: tutorial.TutorialBaselineChunkOverlapTokens, RetrievalStrategy: tutorial.TutorialRetrievalStrategyGraph, GraphRetrievalEnabled: true},
		{ID: tutorial.TutorialP8ContextPackCandidateID, Chapter: tutorial.TutorialP8ContextPackChapter, ParserMethod: "basic", ChunkSizeTokens: tutorial.TutorialBaselineChunkSizeTokens, ChunkOverlapTokens: tutorial.TutorialBaselineChunkOverlapTokens, RetrievalStrategy: tutorial.TutorialRetrievalStrategyHybrid, ReuseBaselineIndex: true, ContextPackTopN: tutorial.TutorialP8ContextPackTopN, ContextPackMaxTokens: tutorial.TutorialContextPackMaxTokens},
	}
}

func writeJSON(path string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}
func mustJSON(value any) []byte {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return raw
}
func hashFile(path string) (string, error) {
	input, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer input.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, input); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func writeSums(root string) error {
	var paths []string
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && filepath.Base(path) != "SHA256SUMS" {
			paths = append(paths, path)
		}
		return nil
	}); err != nil {
		return err
	}
	sort.Strings(paths)
	output := make([]string, 0, len(paths))
	for _, path := range paths {
		hash, err := hashFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		output = append(output, hash+"  "+filepath.ToSlash(rel))
	}
	return os.WriteFile(filepath.Join(root, "SHA256SUMS"), []byte(strings.Join(output, "\n")+"\n"), 0o644)
}
