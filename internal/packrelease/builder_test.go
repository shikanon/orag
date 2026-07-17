package packrelease

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shikanon/orag/internal/tutorial"
)

func TestBuildCreatesValidatedImmutableRelease(t *testing.T) {
	source := t.TempDir()
	writeTestFile(t, filepath.Join(source, "data", "crud_split", "split_merged.json"), `{"task":[{"questions":["问题一","问题二"],"answers":["答案一","答案二"]}]}`)
	writeTestFile(t, filepath.Join(source, "data", "80000_docs", "documents.txt"), "第一篇文档\n")
	writeTestFile(t, filepath.Join(source, "data", "crud", "merged.json"), `{"documents":[{"text":"第二篇文档"}]}`)
	git(t, source, "init")
	git(t, source, "config", "user.email", "test@example.com")
	git(t, source, "config", "user.name", "Test")
	git(t, source, "add", "data")
	git(t, source, "commit", "-m", "source")

	output := t.TempDir()
	release, err := Build(BuildConfig{SourceDir: source, OutputDir: output, Version: "1.1.0", QuickMaxBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	if release.QuickBytes == 0 || release.BenchmarkBytes == 0 {
		t.Fatalf("release sizes: %#v", release)
	}
	for _, tier := range []string{"quick", "benchmark"} {
		raw, err := os.ReadFile(filepath.Join(release.Root, tier, "manifest.json"))
		if err != nil {
			t.Fatal(err)
		}
		manifest, err := tutorial.ParseManifest(raw, tutorial.Template{ID: "text-rag", Version: "1.1.0", Modality: tutorial.ModalityText}, tutorial.PackRef{Tier: tier, EstimatedBytes: release.BenchmarkBytes})
		if err != nil {
			t.Fatalf("%s manifest: %v", tier, err)
		}
		if len(manifest.Runtime.Dataset.Items) != 2 || len(manifest.Runtime.Candidates) != 8 {
			t.Fatalf("%s runtime=%#v", tier, manifest.Runtime)
		}
	}
	sums, err := os.ReadFile(filepath.Join(release.Root, "SHA256SUMS"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(sums), "quick/manifest.json") || !strings.Contains(string(sums), "source/CRUD-RAG-") {
		t.Fatalf("checksums missing required artifacts: %s", sums)
	}
	assertArchiveContains(t, release.Root, "data/80000_docs/documents.txt")
	if _, err := Build(BuildConfig{SourceDir: source, OutputDir: output, Version: "1.1.0"}); err == nil {
		t.Fatal("expected existing immutable release to be rejected")
	}
}

func TestCollectQuestionsIgnoresMismatchedPairs(t *testing.T) {
	var value any
	if err := json.Unmarshal([]byte(`{"questions":["q",3],"answers":["a","b"]}`), &value); err != nil {
		t.Fatal(err)
	}
	var items []tutorial.RuntimeDatasetItem
	collectQuestions(value, &items)
	if len(items) != 0 {
		t.Fatalf("items=%#v", items)
	}
}

func TestCollectQuestionsAcceptsCRUDRAGStringPairs(t *testing.T) {
	var value any
	if err := json.Unmarshal([]byte(`{"questions":"问题","answers":"答案"}`), &value); err != nil {
		t.Fatal(err)
	}
	var items []tutorial.RuntimeDatasetItem
	collectQuestions(value, &items)
	if len(items) != 1 || items[0].Query != "问题" || items[0].GroundTruth != "答案" {
		t.Fatalf("items=%#v", items)
	}
}

func TestVerifyPublicUsesChecksumContract(t *testing.T) {
	root := filepath.Join(t.TempDir(), "text-rag", "1.1.0")
	writeTestFile(t, filepath.Join(root, "nested", "artifact.txt"), "public artifact")
	hash, err := hashFile(filepath.Join(root, "nested", "artifact.txt"))
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(root, "SHA256SUMS"), hash+"  nested/artifact.txt\n")
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tutorial-packs/text-rag/1.1.0/nested/artifact.txt" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("public artifact"))
	}))
	defer server.Close()
	if err := verifyPublicWithClient(t.Context(), root, mustParseURL(t, server.URL+"/tutorial-packs/text-rag/1.1.0/"), server.Client()); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyPublicRejectsMetadataDrift(t *testing.T) {
	root := filepath.Join(t.TempDir(), "text-rag", "1.1.0")
	writeTestFile(t, filepath.Join(root, "artifact.json"), `{"valid":true}`)
	hash, err := hashFile(filepath.Join(root, "artifact.json"))
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(root, "SHA256SUMS"), hash+"  artifact.json\n")
	for name, handler := range map[string]http.HandlerFunc{
		"mime": func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte(`{"valid":true}`))
		},
		"length": func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Content-Length", "99")
			_, _ = w.Write([]byte(`{"valid":true}`))
		},
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewTLSServer(handler)
			defer server.Close()
			if err := verifyPublicWithClient(t.Context(), root, mustParseURL(t, server.URL+"/tutorial-packs/text-rag/1.1.0/"), server.Client()); err == nil {
				t.Fatal("expected verification failure")
			}
		})
	}
}

func TestVerifyPublicRequiresHTTPS(t *testing.T) {
	root := filepath.Join(t.TempDir(), "text-rag", "1.1.0")
	writeTestFile(t, filepath.Join(root, "SHA256SUMS"), "")
	if err := VerifyPublic(t.Context(), root, "http://example.test/tutorial-packs"); err == nil {
		t.Fatal("expected HTTP public base to be rejected")
	}
}

func TestVerifyPublicRejectsHTTPSRedirectToHTTP(t *testing.T) {
	root := filepath.Join(t.TempDir(), "text-rag", "1.1.0")
	writeTestFile(t, filepath.Join(root, "artifact.txt"), "public artifact")
	hash, err := hashFile(filepath.Join(root, "artifact.txt"))
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(root, "SHA256SUMS"), hash+"  artifact.txt\n")
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("public artifact"))
	}))
	defer httpServer.Close()
	httpsServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		http.Redirect(w, request, httpServer.URL, http.StatusFound)
	}))
	defer httpsServer.Close()
	if err := verifyPublicWithClient(t.Context(), root, mustParseURL(t, httpsServer.URL+"/tutorial-packs/text-rag/1.1.0/"), httpsServer.Client()); err == nil {
		t.Fatal("expected HTTP redirect to be rejected")
	}
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func TestReleaseFilesOrdersManifestLast(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "quick", "manifest.json"), "{}")
	writeTestFile(t, filepath.Join(root, "quick", "corpus", "documents.json"), "{}")
	paths, err := releaseFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(paths[len(paths)-1]) != "manifest.json" {
		t.Fatalf("files=%v", paths)
	}
}

func TestReleasePrefixUsesTemplateAndVersion(t *testing.T) {
	prefix, err := releasePrefix(filepath.Join(t.TempDir(), "text-rag", "1.1.0"))
	if err != nil || prefix != "text-rag/1.1.0" {
		t.Fatalf("prefix=%q err=%v", prefix, err)
	}
}

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}

func assertArchiveContains(t *testing.T, root, expected string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(root, "source", "*.tar.gz"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("archive=%v err=%v", matches, err)
	}
	input, err := os.Open(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	gz, err := gzip.NewReader(input)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	archive := tar.NewReader(gz)
	for {
		header, err := archive.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatal(err)
		}
		if header.Name == expected {
			return
		}
	}
	t.Fatalf("archive does not contain %s", expected)
}
