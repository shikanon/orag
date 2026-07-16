package tutorial

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseManifestBindsTemplateAndRejectsHostileObjects(t *testing.T) {
	template, pack := testTemplateAndPack(t)
	got, err := ParseManifest(loadManifestFixture(t, "pack-manifest-valid.json"), template, pack)
	if err != nil {
		t.Fatal(err)
	}
	if got.TemplateID != template.ID || got.Version != template.Version || got.Tier != pack.Tier || len(got.Objects) != 2 {
		t.Fatalf("manifest = %#v", got)
	}

	for _, raw := range [][]byte{
		loadManifestFixture(t, "pack-manifest-invalid-sha.json"),
		[]byte(`{"template_id":"text-rag","version":"1.0.0","tier":"quick","license":{"spdx":"CC-BY-4.0","source_url":"https://example.test/license","redistributable":true},"objects":[{"path":"../secret","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","bytes":1,"content_type":"text/plain"}]}`),
		[]byte(`{"template_id":"text-rag","version":"1.0.0","tier":"quick","license":{"spdx":"CC-BY-4.0","source_url":"https://example.test/license","redistributable":true},"objects":[{"path":"data.txt","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","bytes":1,"content_type":"application/x-shellscript"}]}`),
		[]byte(`{"template_id":"text-rag","version":"1.0.0","tier":"quick","license":{"spdx":"CC-BY-4.0","source_url":"https://example.test/license","redistributable":true},"objects":[{"path":"data.txt","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","bytes":1,"content_type":"text/plain"}],"unexpected":true}`),
	} {
		if _, err := ParseManifest(raw, template, pack); !errors.Is(err, ErrManifestInvalid) {
			t.Fatalf("ParseManifest() error = %v, want ErrManifestInvalid", err)
		}
	}
}

func TestParseManifestRejectsCatalogMismatchAndOversizedPack(t *testing.T) {
	template, pack := testTemplateAndPack(t)
	for name, raw := range map[string][]byte{
		"version":  []byte(`{"template_id":"text-rag","version":"1.0.1","tier":"quick","license":{"spdx":"CC-BY-4.0","source_url":"https://example.test/license","redistributable":true},"objects":[{"path":"data.txt","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","bytes":1,"content_type":"text/plain"}]}`),
		"license":  []byte(`{"template_id":"text-rag","version":"1.0.0","tier":"quick","license":{"spdx":"CC-BY-4.0","source_url":"http://example.test/license","redistributable":false},"objects":[{"path":"data.txt","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","bytes":1,"content_type":"text/plain"}]}`),
		"estimate": []byte(`{"template_id":"text-rag","version":"1.0.0","tier":"quick","license":{"spdx":"CC-BY-4.0","source_url":"https://example.test/license","redistributable":true},"objects":[{"path":"data.txt","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","bytes":157286401,"content_type":"text/plain"}]}`),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseManifest(raw, template, pack); !errors.Is(err, ErrManifestInvalid) {
				t.Fatalf("ParseManifest() error = %v, want ErrManifestInvalid", err)
			}
		})
	}
}

func TestParseManifestReturnsDefensiveObjectCopies(t *testing.T) {
	template, pack := testTemplateAndPack(t)
	raw := loadManifestFixture(t, "pack-manifest-valid.json")
	first, err := ParseManifest(raw, template, pack)
	if err != nil {
		t.Fatal(err)
	}
	first.Objects[0].Path = "mutated.txt"
	second, err := ParseManifest(raw, template, pack)
	if err != nil {
		t.Fatal(err)
	}
	if second.Objects[0].Path == "mutated.txt" {
		t.Fatal("manifest object slice aliases caller state")
	}
}

func TestParseManifestValidatesRuntimeDeclaration(t *testing.T) {
	template, pack := testTemplateAndPack(t)
	valid := []byte(`{
		"template_id":"text-rag","version":"1.0.0","tier":"quick",
		"license":{"spdx":"CC-BY-4.0","source_url":"https://example.test/license","redistributable":true},
		"objects":[{"path":"corpus/data.txt","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","bytes":1,"content_type":"text/plain"}],
		"runtime":{"baseline":{"profile":"realtime","top_k":5},"documents":[{"object_path":"corpus/data.txt","name":"数据"}],"dataset":{"name":"验证集","items":[{"query":"问题","ground_truth":"答案","split":"eval"}]}}
	}`)
	manifest, err := ParseManifest(valid, template, pack)
	if err != nil || manifest.Runtime == nil || manifest.Runtime.Baseline.TopK != 5 {
		t.Fatalf("manifest=%#v err=%v", manifest, err)
	}
	manifest.Runtime.Dataset.Items[0].ExpectedEvidence = []string{"mutated"}
	reparsed, err := ParseManifest(valid, template, pack)
	if err != nil || len(reparsed.Runtime.Dataset.Items[0].ExpectedEvidence) != 0 {
		t.Fatalf("runtime copy aliases parser state: %#v err=%v", reparsed, err)
	}

	for name, raw := range map[string][]byte{
		"outside_object": []byte(strings.Replace(string(valid), `"corpus/data.txt","name":"数据"`, `"other.txt","name":"数据"`, 1)),
		"wrong_profile":  []byte(strings.Replace(string(valid), `"profile":"realtime"`, `"profile":"high_precision"`, 1)),
		"wrong_split":    []byte(strings.Replace(string(valid), `"split":"eval"`, `"split":"mystery"`, 1)),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseManifest(raw, template, pack); !errors.Is(err, ErrManifestInvalid) {
				t.Fatalf("ParseManifest() error = %v, want ErrManifestInvalid", err)
			}
		})
	}
}

func TestValidObjectPathRejectsEscapes(t *testing.T) {
	for _, value := range []string{"", "/root", "../root", "folder/../root", "folder\\root", "folder/%2e%2e/root"} {
		if validObjectPath(value) {
			t.Fatalf("validObjectPath(%q) = true", value)
		}
	}
	if !validObjectPath("corpus/documents.jsonl") {
		t.Fatal("expected canonical relative object path to be valid")
	}
}

func testTemplateAndPack(t *testing.T) (Template, PackRef) {
	t.Helper()
	catalog, err := NewCatalog()
	if err != nil {
		t.Fatal(err)
	}
	template, err := catalog.Get("text-rag", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	for _, pack := range template.Packs {
		if pack.Tier == "quick" {
			return template, pack
		}
	}
	t.Fatal("quick pack is missing")
	return Template{}, PackRef{}
}

func loadManifestFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(raw)) == "" {
		t.Fatalf("fixture %q is empty", name)
	}
	return raw
}
