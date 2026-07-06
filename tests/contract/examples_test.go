package contract

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func TestExamplesReadmeIndex(t *testing.T) {
	readme := readExamplesReadme(t)

	for _, want := range []string{
		"# ORAG Examples",
		"## Prerequisites",
		"## Commands",
		"## Service/Curl Examples",
		"## MCP and Skill Examples",
		"## Go Examples",
		"## Covered Modules",
		"GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./tests/contract -run TestExamples -v",
		"GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go run ./examples/go/memory",
		"GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson make agent-sync-check",
		"GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson make mcp-self-check-smoke",
		"public `pkg/memory` facade",
		"ralph_loop_run",
		"orag_check",
	} {
		if !strings.Contains(readme, want) {
			t.Fatalf("examples README missing %q", want)
		}
	}

	for _, want := range []string{
		"Auth",
		"Knowledge base",
		"Document import",
		"Document upload",
		"Query",
		"SSE query",
		"Trace list/detail",
		"Dataset and evaluation",
		"Optimization",
		"MCP stdio",
		"Codex Skill",
		"Claude Code Skill",
		"Trae Skill",
	} {
		if !strings.Contains(readme, want) {
			t.Fatalf("examples README missing covered module %q", want)
		}
	}
}

func TestExamplesScriptPaths(t *testing.T) {
	readme := readExamplesReadme(t)

	for _, path := range []string{
		"scripts/dev-up.sh",
		"scripts/wait-ready.sh",
		"scripts/dev-down.sh",
		"examples/curl/lib.sh",
		"examples/curl/00_login.sh",
		"examples/curl/05_health_ready.sh",
		"examples/curl/10_create_kb.sh",
		"examples/curl/20_upload_doc.sh",
		"examples/curl/25_upload_file.sh",
		"examples/curl/30_query.sh",
		"examples/curl/35_query_stream.sh",
		"examples/curl/36_trace_lookup.sh",
		"examples/curl/40_eval.sh",
		"examples/curl/45_optimize.sh",
		"examples/go/memory/main.go",
		"examples/mcp/README.md",
		"examples/mcp/stdio-client-config.json",
		"examples/mcp/ralph-loop-stdio-smoke.jsonl",
		"examples/mcp/self-check-stdio-smoke.jsonl",
		"examples/skills/README.md",
		"examples/skills/self-check-diagnose-ops.md",
		"examples/skills/codex-ralph-loop.md",
		"examples/skills/claude-code-ralph-loop.md",
		"examples/skills/trae-ralph-loop.md",
	} {
		assertReferencedPathExists(t, readme, path)
	}
}

func TestMCPAndSkillExamplesDocumentRalphLoop(t *testing.T) {
	mcpSmoke := readRepoFile(t, "examples/mcp/ralph-loop-stdio-smoke.jsonl")
	for _, want := range []string{
		`"method":"initialize"`,
		`"method":"tools/list"`,
		`"method":"tools/call"`,
		`"name":"ralph_loop_run"`,
		`"task_id":"Task 5"`,
	} {
		if !strings.Contains(mcpSmoke, want) {
			t.Fatalf("MCP smoke example missing %q", want)
		}
	}

	selfCheckSmoke := readRepoFile(t, "examples/mcp/self-check-stdio-smoke.jsonl")
	for _, want := range []string{
		`"method":"initialize"`,
		`"method":"tools/list"`,
		`"method":"tools/call"`,
		`"name":"orag_check"`,
		`"scope":"agent_sync"`,
		`"mode":"focused"`,
		`"trace_id":"trace_self_check_agent_sync_smoke"`,
	} {
		if !strings.Contains(selfCheckSmoke, want) {
			t.Fatalf("self-check MCP smoke example missing %q", want)
		}
	}

	clientConfig := readRepoFile(t, "examples/mcp/stdio-client-config.json")
	for _, want := range []string{
		`"orag-ralph-loop"`,
		`"./cmd/orag-mcp"`,
		`"ORAG_API_BASE_URL"`,
		`"ORAG_API_TOKEN"`,
		`"ORAG_TENANT_ID"`,
	} {
		if !strings.Contains(clientConfig, want) {
			t.Fatalf("MCP client config missing %q", want)
		}
	}

	for _, path := range []string{
		"examples/skills/codex-ralph-loop.md",
		"examples/skills/claude-code-ralph-loop.md",
		"examples/skills/trae-ralph-loop.md",
	} {
		body := readRepoFile(t, path)
		for _, want := range []string{
			"ORAG_API_BASE_URL",
			"ORAG_API_TOKEN",
			"ORAG_TENANT_ID",
			"Task 5",
			"trace_id",
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("%s missing %q", path, want)
			}
		}
	}

	opsGuide := readRepoFile(t, "examples/skills/self-check-diagnose-ops.md")
	for _, want := range []string{
		"orag-self-check",
		"orag-self-diagnose",
		"orag-self-ops",
		"mutually exclusive",
		"orag_check",
		"orag_diagnose",
		"orag_maintenance_plan",
		"approved",
		"verdict=blocked",
	} {
		if !strings.Contains(opsGuide, want) {
			t.Fatalf("operational Skill guide missing %q", want)
		}
	}
}

func TestExamplesCurlEndpointsMatchOpenAPI(t *testing.T) {
	doc, err := openapi3.NewLoader().LoadFromFile(filepath.Join("..", "..", "api", "openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatalf("openapi validation failed: %v", err)
	}

	for _, endpoint := range []struct {
		script      string
		method      string
		openAPIPath string
		scriptPath  string
	}{
		{"examples/curl/05_health_ready.sh", http.MethodGet, "/healthz", "/healthz"},
		{"examples/curl/05_health_ready.sh", http.MethodGet, "/readyz", "/readyz"},
		{"examples/curl/00_login.sh", http.MethodPost, "/v1/auth/login", "/v1/auth/login"},
		{"examples/curl/10_create_kb.sh", http.MethodPost, "/v1/knowledge-bases", "/v1/knowledge-bases"},
		{"examples/curl/20_upload_doc.sh", http.MethodPost, "/v1/knowledge-bases/{id}/documents:import", "/v1/knowledge-bases/$kb_id/documents:import"},
		{"examples/curl/25_upload_file.sh", http.MethodPost, "/v1/knowledge-bases/{id}/documents", "/v1/knowledge-bases/$kb_id/documents"},
		{"examples/curl/30_query.sh", http.MethodPost, "/v1/query", "/v1/query"},
		{"examples/curl/35_query_stream.sh", http.MethodPost, "/v1/query:stream", "/v1/query:stream"},
		{"examples/curl/36_trace_lookup.sh", http.MethodGet, "/v1/traces", "/v1/traces"},
		{"examples/curl/36_trace_lookup.sh", http.MethodGet, "/v1/traces/{trace_id}", "/v1/traces/$trace_id"},
		{"examples/curl/40_eval.sh", http.MethodPost, "/v1/datasets", "/v1/datasets"},
		{"examples/curl/40_eval.sh", http.MethodPost, "/v1/datasets/{id}/items", "/v1/datasets/$dataset_id/items"},
		{"examples/curl/40_eval.sh", http.MethodPost, "/v1/evaluations", "/v1/evaluations"},
		{"examples/curl/45_optimize.sh", http.MethodPost, "/v1/optimizations", "/v1/optimizations"},
	} {
		t.Run(endpoint.script+" "+endpoint.method+" "+endpoint.openAPIPath, func(t *testing.T) {
			script := readRepoFile(t, endpoint.script)
			if !strings.Contains(script, endpoint.scriptPath) {
				t.Fatalf("%s missing endpoint %s", endpoint.script, endpoint.scriptPath)
			}
			item := doc.Paths.Find(endpoint.openAPIPath)
			if item == nil {
				t.Fatalf("openapi missing path %s", endpoint.openAPIPath)
			}
			if item.GetOperation(endpoint.method) == nil {
				t.Fatalf("openapi missing operation %s %s", endpoint.method, endpoint.openAPIPath)
			}
		})
	}
}

func TestEvalCurlDocumentsQualityMetrics(t *testing.T) {
	script := readRepoFile(t, "examples/curl/40_eval.sh")

	for _, want := range []string{
		"pairwise_accuracy",
		"primary quality metric",
		"ndcg_at_k",
		"recall_at_k",
		"retrieval_failure_rate",
		"redundancy_rate",
		"alpha_ndcg",
		"aspect_coverage",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("examples/curl/40_eval.sh missing quality metric hint %q", want)
		}
	}
}

func readExamplesReadme(t *testing.T) string {
	t.Helper()

	return readRepoFile(t, "examples/README.md")
}

func assertReferencedPathExists(t *testing.T, readme, path string) {
	t.Helper()

	if !strings.Contains(readme, path) {
		t.Fatalf("examples README does not reference %s", path)
	}

	info, err := os.Stat(filepath.Join("..", "..", filepath.FromSlash(path)))
	if err != nil {
		t.Fatalf("referenced example path %s does not exist: %v", path, err)
	}
	if info.IsDir() {
		t.Fatalf("referenced example path %s is a directory", path)
	}
}

func readRepoFile(t *testing.T, path string) string {
	t.Helper()

	content, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(path)))
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}
