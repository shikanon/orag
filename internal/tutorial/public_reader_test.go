package tutorial

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPublicPackReaderFetchesManifestAndVerifiedObject(t *testing.T) {
	content := []byte("pack object")
	hash := sha256.Sum256(content)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/packs/text-rag/1.0.0/quick/manifest.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"template_id":"text-rag"}`))
		case "/packs/text-rag/1.0.0/quick/corpus/data.txt":
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = w.Write(content)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	reader, err := NewPublicPackReader(server.URL+"/packs", 1024, 1024, time.Second, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := reader.FetchManifest(context.Background(), "text-rag/1.0.0/quick/manifest.json")
	if err != nil || string(manifest) != `{"template_id":"text-rag"}` {
		t.Fatalf("FetchManifest() = %q, %v", manifest, err)
	}
	object, err := reader.FetchObject(context.Background(), "text-rag/1.0.0/quick/manifest.json", PackObject{
		Path: "corpus/data.txt", Bytes: int64(len(content)), SHA256: hex.EncodeToString(hash[:]), ContentType: "text/plain",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(object.TempPath)
	if err != nil || string(got) != string(content) {
		t.Fatalf("temporary content = %q, %v", got, err)
	}
	if err := object.Remove(); err != nil || fileExists(object.TempPath) {
		t.Fatalf("Remove() error = %v, exists=%v", err, fileExists(object.TempPath))
	}
}

func TestPublicPackReaderRejectsRedirectChecksumAndContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/packs/text-rag/1.0.0/quick/manifest.json":
			http.Redirect(w, r, "https://evil.invalid/manifest.json", http.StatusFound)
		case "/packs/text-rag/1.0.0/quick/corpus/data.txt":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write([]byte("wrong"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	reader, err := NewPublicPackReader(server.URL+"/packs", 1024, 1024, time.Second, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reader.FetchManifest(context.Background(), "text-rag/1.0.0/quick/manifest.json"); !errors.Is(err, ErrPublicPackResponse) {
		t.Fatalf("redirect error = %v", err)
	}
	_, err = reader.FetchObject(context.Background(), "text-rag/1.0.0/quick/manifest.json", PackObject{
		Path: "corpus/data.txt", Bytes: 5, SHA256: strings.Repeat("0", 64), ContentType: "text/plain",
	})
	if !errors.Is(err, ErrPublicPackContentType) {
		t.Fatalf("content type error = %v", err)
	}
}

func TestLocalPrivateStoreCopiesVerifiedContentWithoutEscapingRoot(t *testing.T) {
	temp := t.TempDir()
	inputPath := filepath.Join(temp, "verified")
	content := []byte("verified pack")
	if err := os.WriteFile(inputPath, content, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := NewLocalPrivateStore(filepath.Join(temp, "output"), "tutorial-experiments")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutVerified(context.Background(), PrivateObject{
		TenantID: "tenant_a", ProjectID: "prj_a", JobID: "tclj_a",
		Object: VerifiedObject{PackObject: PackObject{SHA256: strings.Repeat("a", 64), Bytes: int64(len(content))}, TempPath: inputPath},
	}); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(temp, "output", "tutorial-experiments", "tenant_a", "prj_a", "tclj_a", strings.Repeat("a", 64))
	got, err := os.ReadFile(output)
	if err != nil || string(got) != string(content) {
		t.Fatalf("output = %q, %v", got, err)
	}
	if err := store.PutVerified(context.Background(), PrivateObject{TenantID: "../tenant", ProjectID: "prj_a", JobID: "tclj_a"}); !errors.Is(err, ErrPrivateStoreConfiguration) {
		t.Fatalf("escape error = %v", err)
	}
}

func fileExists(name string) bool {
	_, err := os.Stat(name)
	return err == nil
}
