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

func TestParseManifestAcceptsOnlyDeclaredP1StructuredJSONCandidate(t *testing.T) {
	template, pack := testTemplateAndPack(t)
	valid := []byte(`{
		"template_id":"text-rag","version":"1.0.0","tier":"quick",
		"license":{"spdx":"CC-BY-4.0","source_url":"https://example.test/license","redistributable":true},
		"objects":[{"path":"corpus/service.json","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","bytes":2,"content_type":"application/json"}],
		"runtime":{"baseline":{"profile":"realtime","top_k":5},"documents":[{"object_path":"corpus/service.json","name":"服务配置"}],"dataset":{"name":"评测","items":[{"query":"端口","ground_truth":"8080"}]},"candidates":[{"id":"p1_structured_json","chapter":"p1_document_parser","parser_method":"structured_json"}]}
	}`)
	manifest, err := ParseManifest(valid, template, pack)
	if err != nil || manifest.Runtime == nil || len(manifest.Runtime.Candidates) != 1 {
		t.Fatalf("manifest=%#v err=%v", manifest, err)
	}
	manifest.Runtime.Candidates[0].ID = "mutated"
	reparsed, err := ParseManifest(valid, template, pack)
	if err != nil || reparsed.Runtime.Candidates[0].ID != "p1_structured_json" {
		t.Fatalf("candidate copy aliases parser state: %#v err=%v", reparsed, err)
	}

	for name, raw := range map[string][]byte{
		"duplicate_id":      []byte(strings.Replace(string(valid), `{"id":"p1_structured_json","chapter":"p1_document_parser","parser_method":"structured_json"}]`, `{"id":"p1_structured_json","chapter":"p1_document_parser","parser_method":"structured_json"},{"id":"p1_structured_json","chapter":"p1_document_parser","parser_method":"structured_json"}]`, 1)),
		"arbitrary_id":      []byte(strings.Replace(string(valid), `"p1_structured_json"`, `"p1_other"`, 1)),
		"wrong_chapter":     []byte(strings.Replace(string(valid), `"p1_document_parser"`, `"p2_chunking"`, 1)),
		"wrong_parser":      []byte(strings.Replace(string(valid), `"structured_json"`, `"docling"`, 1)),
		"non_json_document": []byte(strings.NewReplacer(`corpus/service.json`, `corpus/service.txt`, `"application/json"`, `"text/plain"`).Replace(string(valid))),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseManifest(raw, template, pack); !errors.Is(err, ErrManifestInvalid) {
				t.Fatalf("ParseManifest() error = %v, want ErrManifestInvalid", err)
			}
		})
	}
}

func TestParseManifestAcceptsOnlyDeclaredP2RecursiveChunkCandidate(t *testing.T) {
	template, pack := testTemplateAndPack(t)
	valid := []byte(`{
		"template_id":"text-rag","version":"1.0.0","tier":"quick",
		"license":{"spdx":"CC-BY-4.0","source_url":"https://example.test/license","redistributable":true},
		"objects":[{"path":"corpus/service.json","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","bytes":2,"content_type":"application/json"}],
		"runtime":{"baseline":{"profile":"realtime","top_k":5},"documents":[{"object_path":"corpus/service.json","name":"服务配置"}],"dataset":{"name":"评测","items":[{"query":"端口","ground_truth":"8080"}]},"candidates":[{"id":"p2_recursive_400_80","chapter":"p2_chunking","parser_method":"basic","chunk_size_tokens":400,"chunk_overlap_tokens":80}]}
	}`)
	manifest, err := ParseManifest(valid, template, pack)
	if err != nil || manifest.Runtime == nil || len(manifest.Runtime.Candidates) != 1 {
		t.Fatalf("manifest=%#v err=%v", manifest, err)
	}
	candidate := manifest.Runtime.Candidates[0]
	if candidate.ID != TutorialP2RecursiveChunkCandidateID || candidate.ChunkSizeTokens != TutorialP2ChunkSizeTokens || candidate.ChunkOverlapTokens != TutorialP2ChunkOverlapTokens {
		t.Fatalf("candidate=%#v", candidate)
	}

	for name, raw := range map[string][]byte{
		"wrong_parser":  []byte(strings.Replace(string(valid), `"parser_method":"basic"`, `"parser_method":"structured_json"`, 1)),
		"wrong_size":    []byte(strings.Replace(string(valid), `"chunk_size_tokens":400`, `"chunk_size_tokens":401`, 1)),
		"wrong_overlap": []byte(strings.Replace(string(valid), `"chunk_overlap_tokens":80`, `"chunk_overlap_tokens":400`, 1)),
		"p1_chunking":   []byte(strings.Replace(string(valid), `"id":"p2_recursive_400_80","chapter":"p2_chunking","parser_method":"basic","chunk_size_tokens":400,"chunk_overlap_tokens":80`, `"id":"p1_structured_json","chapter":"p1_document_parser","parser_method":"structured_json","chunk_size_tokens":400,"chunk_overlap_tokens":80`, 1)),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseManifest(raw, template, pack); !errors.Is(err, ErrManifestInvalid) {
				t.Fatalf("ParseManifest() error=%v, want ErrManifestInvalid", err)
			}
		})
	}
}

func TestParseManifestAcceptsOnlyDeclaredP3ContextualCandidate(t *testing.T) {
	template, pack := testTemplateAndPack(t)
	valid := []byte(`{
		"template_id":"text-rag","version":"1.0.0","tier":"quick",
		"license":{"spdx":"CC-BY-4.0","source_url":"https://example.test/license","redistributable":true},
		"objects":[{"path":"corpus/service.json","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","bytes":2,"content_type":"application/json"}],
		"runtime":{"baseline":{"profile":"realtime","top_k":5},"documents":[{"object_path":"corpus/service.json","name":"服务配置"}],"dataset":{"name":"评测","items":[{"query":"端口","ground_truth":"8080"}]},"candidates":[{"id":"p3_contextual_retrieval","chapter":"p3_contextual_retrieval","parser_method":"basic","chunk_size_tokens":800,"chunk_overlap_tokens":120,"contextual_retrieval":true}]}
	}`)
	manifest, err := ParseManifest(valid, template, pack)
	if err != nil || manifest.Runtime == nil || len(manifest.Runtime.Candidates) != 1 {
		t.Fatalf("manifest=%#v err=%v", manifest, err)
	}
	candidate := manifest.Runtime.Candidates[0]
	if candidate.ID != TutorialP3ContextualCandidateID || !candidate.ContextualRetrieval || candidate.ChunkSizeTokens != TutorialBaselineChunkSizeTokens || candidate.ChunkOverlapTokens != TutorialBaselineChunkOverlapTokens {
		t.Fatalf("candidate=%#v", candidate)
	}

	for name, raw := range map[string][]byte{
		"disabled_contextual": []byte(strings.Replace(string(valid), `"contextual_retrieval":true`, `"contextual_retrieval":false`, 1)),
		"wrong_size":          []byte(strings.Replace(string(valid), `"chunk_size_tokens":800`, `"chunk_size_tokens":400`, 1)),
		"wrong_overlap":       []byte(strings.Replace(string(valid), `"chunk_overlap_tokens":120`, `"chunk_overlap_tokens":80`, 1)),
		"unrelated_enabled":   []byte(strings.Replace(string(valid), `"id":"p3_contextual_retrieval"`, `"id":"p2_recursive_400_80"`, 1)),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseManifest(raw, template, pack); !errors.Is(err, ErrManifestInvalid) {
				t.Fatalf("ParseManifest() error=%v, want ErrManifestInvalid", err)
			}
		})
	}
}

func TestParseManifestAcceptsOnlyDeclaredP4SparseCandidate(t *testing.T) {
	template, pack := testTemplateAndPack(t)
	valid := []byte(`{
		"template_id":"text-rag","version":"1.0.0","tier":"quick",
		"license":{"spdx":"CC-BY-4.0","source_url":"https://example.test/license","redistributable":true},
		"objects":[{"path":"corpus/service.json","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","bytes":2,"content_type":"application/json"}],
		"runtime":{"baseline":{"profile":"realtime","top_k":5},"documents":[{"object_path":"corpus/service.json","name":"服务配置"}],"dataset":{"name":"评测","items":[{"query":"端口","ground_truth":"8080"}]},"candidates":[{"id":"p4_sparse_retrieval","chapter":"p4_sparse_retrieval","parser_method":"basic","chunk_size_tokens":800,"chunk_overlap_tokens":120,"retrieval_strategy":"sparse","reuse_baseline_index":true}]}
	}`)
	manifest, err := ParseManifest(valid, template, pack)
	if err != nil || manifest.Runtime == nil || len(manifest.Runtime.Candidates) != 1 {
		t.Fatalf("manifest=%#v err=%v", manifest, err)
	}
	candidate := manifest.Runtime.Candidates[0]
	if candidate.ID != TutorialP4SparseCandidateID || candidate.RetrievalStrategy != TutorialRetrievalStrategySparse || !candidate.ReuseBaselineIndex {
		t.Fatalf("candidate=%#v", candidate)
	}

	for name, raw := range map[string][]byte{
		"hybrid":        []byte(strings.Replace(string(valid), `"retrieval_strategy":"sparse"`, `"retrieval_strategy":"hybrid"`, 1)),
		"new_index":     []byte(strings.Replace(string(valid), `"reuse_baseline_index":true`, `"reuse_baseline_index":false`, 1)),
		"contextual":    []byte(strings.Replace(string(valid), `"parser_method":"basic"`, `"parser_method":"basic","contextual_retrieval":true`, 1)),
		"wrong_chapter": []byte(strings.Replace(string(valid), `"chapter":"p4_sparse_retrieval"`, `"chapter":"p5_multi_route_retrieval"`, 1)),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseManifest(raw, template, pack); !errors.Is(err, ErrManifestInvalid) {
				t.Fatalf("ParseManifest() error=%v, want ErrManifestInvalid", err)
			}
		})
	}
}

func TestParseManifestAcceptsOnlyDeclaredP5MultiQueryCandidate(t *testing.T) {
	template, pack := testTemplateAndPack(t)
	valid := []byte(`{
		"template_id":"text-rag","version":"1.0.0","tier":"quick",
		"license":{"spdx":"CC-BY-4.0","source_url":"https://example.test/license","redistributable":true},
		"objects":[{"path":"corpus/service.json","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","bytes":2,"content_type":"application/json"}],
		"runtime":{"baseline":{"profile":"realtime","top_k":5},"documents":[{"object_path":"corpus/service.json","name":"服务配置"}],"dataset":{"name":"评测","items":[{"query":"端口","ground_truth":"8080"}]},"candidates":[{"id":"p5_multi_query_retrieval","chapter":"p5_multi_query_retrieval","parser_method":"basic","chunk_size_tokens":800,"chunk_overlap_tokens":120,"retrieval_strategy":"hybrid","reuse_baseline_index":true,"multi_query_count":3}]}
	}`)
	manifest, err := ParseManifest(valid, template, pack)
	if err != nil || manifest.Runtime == nil || len(manifest.Runtime.Candidates) != 1 {
		t.Fatalf("manifest=%#v err=%v", manifest, err)
	}
	candidate := manifest.Runtime.Candidates[0]
	if candidate.ID != TutorialP5MultiQueryCandidateID || candidate.RetrievalStrategy != TutorialRetrievalStrategyHybrid || !candidate.ReuseBaselineIndex || candidate.MultiQueryCount != 3 {
		t.Fatalf("candidate=%#v", candidate)
	}
	for name, raw := range map[string][]byte{
		"sparse":      []byte(strings.Replace(string(valid), `"retrieval_strategy":"hybrid"`, `"retrieval_strategy":"sparse"`, 1)),
		"new_index":   []byte(strings.Replace(string(valid), `"reuse_baseline_index":true`, `"reuse_baseline_index":false`, 1)),
		"two_queries": []byte(strings.Replace(string(valid), `"multi_query_count":3`, `"multi_query_count":2`, 1)),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseManifest(raw, template, pack); !errors.Is(err, ErrManifestInvalid) {
				t.Fatalf("ParseManifest() error=%v, want ErrManifestInvalid", err)
			}
		})
	}
}

func TestParseManifestAcceptsOnlyDeclaredP6RerankCandidate(t *testing.T) {
	template, pack := testTemplateAndPack(t)
	valid := []byte(`{
		"template_id":"text-rag","version":"1.0.0","tier":"quick",
		"license":{"spdx":"CC-BY-4.0","source_url":"https://example.test/license","redistributable":true},
		"objects":[{"path":"corpus/service.json","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","bytes":2,"content_type":"application/json"}],
		"runtime":{"baseline":{"profile":"realtime","top_k":5},"documents":[{"object_path":"corpus/service.json","name":"服务配置"}],"dataset":{"name":"评测","items":[{"query":"端口","ground_truth":"8080"}]},"candidates":[{"id":"p6_rerank_retrieval","chapter":"p6_rerank_retrieval","parser_method":"basic","chunk_size_tokens":800,"chunk_overlap_tokens":120,"retrieval_strategy":"hybrid","reuse_baseline_index":true,"rerank_enabled":true}]}
	}`)
	manifest, err := ParseManifest(valid, template, pack)
	if err != nil || manifest.Runtime == nil || len(manifest.Runtime.Candidates) != 1 {
		t.Fatalf("manifest=%#v err=%v", manifest, err)
	}
	candidate := manifest.Runtime.Candidates[0]
	if candidate.ID != TutorialP6RerankCandidateID || candidate.RetrievalStrategy != TutorialRetrievalStrategyHybrid || !candidate.ReuseBaselineIndex || !candidate.RerankEnabled {
		t.Fatalf("candidate=%#v", candidate)
	}
	for name, raw := range map[string][]byte{
		"disabled":    []byte(strings.Replace(string(valid), `"rerank_enabled":true`, `"rerank_enabled":false`, 1)),
		"sparse":      []byte(strings.Replace(string(valid), `"retrieval_strategy":"hybrid"`, `"retrieval_strategy":"sparse"`, 1)),
		"multi_query": []byte(strings.Replace(string(valid), `"rerank_enabled":true`, `"rerank_enabled":true,"multi_query_count":3`, 1)),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseManifest(raw, template, pack); !errors.Is(err, ErrManifestInvalid) {
				t.Fatalf("ParseManifest() error=%v, want ErrManifestInvalid", err)
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
