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
		"## Scenario Demos",
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

func TestExamplesReadmeScenarioPositioning(t *testing.T) {
	readme := readExamplesReadme(t)

	for _, want := range []string{
		"product-user scenario demo entry point",
		"Start with the role-based scenario demos below to decide why and when a support, engineering, platform, product, or agent team should use ORAG",
		"lower-level curl, Go, MCP, and Skill examples are supporting assets",
		"Each scenario directory contains a focused README, sample input, expected output, and command references back to the maintained examples",
		"supporting assets",
		"Scenario demos cover customer support, engineering runbooks, platform onboarding, product launch review, agent development, knowledge-base Q&A, streaming assistant, trace/diagnostics, evaluation/optimization, in-process Go embedding, and agent/MCP integration from the user perspective.",
	} {
		if !strings.Contains(readme, want) {
			t.Fatalf("examples README missing scenario positioning %q", want)
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

func TestExamplesScenarioDemos(t *testing.T) {
	readme := readExamplesReadme(t)

	for _, scenario := range []struct {
		name     string
		dir      string
		title    string
		runnable bool
		assets   []string
	}{
		{
			name:     "customer support",
			dir:      "examples/scenarios/customer-support",
			title:    "# Customer Support Scenario",
			runnable: true,
			assets: []string{
				"examples/curl/05_health_ready.sh",
				"examples/curl/00_login.sh",
				"examples/curl/10_create_kb.sh",
				"examples/curl/20_upload_doc.sh",
				"examples/curl/25_upload_file.sh",
				"examples/curl/30_query.sh",
				"examples/curl/36_trace_lookup.sh",
			},
		},
		{
			name:     "engineering runbook",
			dir:      "examples/scenarios/engineering-runbook",
			title:    "# Engineering Runbook Scenario",
			runnable: true,
			assets: []string{
				"examples/curl/05_health_ready.sh",
				"examples/curl/00_login.sh",
				"examples/curl/10_create_kb.sh",
				"examples/curl/20_upload_doc.sh",
				"examples/curl/30_query.sh",
				"examples/curl/36_trace_lookup.sh",
				"examples/skills/self-check-diagnose-ops.md",
			},
		},
		{
			name:     "platform team",
			dir:      "examples/scenarios/platform-team",
			title:    "# Platform Team Scenario",
			runnable: true,
			assets: []string{
				"examples/curl/05_health_ready.sh",
				"examples/curl/00_login.sh",
				"examples/curl/10_create_kb.sh",
				"examples/curl/20_upload_doc.sh",
				"examples/curl/30_query.sh",
				"examples/curl/36_trace_lookup.sh",
				"examples/curl/40_eval.sh",
				"examples/curl/45_optimize.sh",
				"examples/mcp/README.md",
				"examples/skills/README.md",
			},
		},
		{
			name:     "product team",
			dir:      "examples/scenarios/product-team",
			title:    "# Product Team Scenario",
			runnable: true,
			assets: []string{
				"examples/curl/05_health_ready.sh",
				"examples/curl/00_login.sh",
				"examples/curl/10_create_kb.sh",
				"examples/curl/20_upload_doc.sh",
				"examples/curl/30_query.sh",
				"examples/curl/40_eval.sh",
				"examples/curl/45_optimize.sh",
			},
		},
		{
			name:     "agent developer",
			dir:      "examples/scenarios/agent-developer",
			title:    "# Agent Developer Scenario",
			runnable: true,
			assets: []string{
				"examples/mcp/README.md",
				"examples/mcp/stdio-client-config.json",
				"examples/mcp/ralph-loop-stdio-smoke.jsonl",
				"examples/mcp/self-check-stdio-smoke.jsonl",
				"examples/skills/README.md",
				"examples/skills/codex-ralph-loop.md",
				"examples/skills/claude-code-ralph-loop.md",
				"examples/skills/trae-ralph-loop.md",
				"examples/skills/self-check-diagnose-ops.md",
			},
		},
		{
			name:  "knowledge-base Q&A",
			dir:   "examples/scenarios/kb-qa",
			title: "# Knowledge-base Q&A Scenario",
			assets: []string{
				"examples/curl/05_health_ready.sh",
				"examples/curl/00_login.sh",
				"examples/curl/10_create_kb.sh",
				"examples/curl/20_upload_doc.sh",
				"examples/curl/25_upload_file.sh",
				"examples/curl/30_query.sh",
			},
		},
		{
			name:  "streaming assistant",
			dir:   "examples/scenarios/streaming-assistant",
			title: "# Streaming Assistant Scenario",
			assets: []string{
				"examples/curl/35_query_stream.sh",
				"examples/curl/20_upload_doc.sh",
			},
		},
		{
			name:  "trace and diagnostics",
			dir:   "examples/scenarios/trace-diagnostics",
			title: "# Trace and Diagnostics Scenario",
			assets: []string{
				"examples/curl/36_trace_lookup.sh",
				"examples/mcp/self-check-stdio-smoke.jsonl",
				"examples/skills/self-check-diagnose-ops.md",
			},
		},
		{
			name:  "evaluation and optimization",
			dir:   "examples/scenarios/eval-optimization",
			title: "# Evaluation and Optimization Scenario",
			assets: []string{
				"examples/curl/40_eval.sh",
				"examples/curl/45_optimize.sh",
			},
		},
		{
			name:  "in-process Go embedding",
			dir:   "examples/scenarios/go-embedding",
			title: "# In-process Go Embedding Scenario",
			assets: []string{
				"examples/go/memory/main.go",
				"examples/go/memory/main_test.go",
			},
		},
		{
			name:  "agent and MCP integration",
			dir:   "examples/scenarios/agent-mcp-integration",
			title: "# Agent and MCP Integration Scenario",
			assets: []string{
				"examples/mcp/README.md",
				"examples/mcp/stdio-client-config.json",
				"examples/mcp/ralph-loop-stdio-smoke.jsonl",
				"examples/skills/README.md",
			},
		},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			readmePath := scenario.dir + "/README.md"
			if scenario.runnable {
				runPath := scenario.dir + "/run.sh"
				assertReferencedPathExists(t, readme, runPath)
				assertRepoFileExecutable(t, runPath)
				assertRepoFileExists(t, scenario.dir+"/demo-data.md")
			} else {
				assertReferencedPathExists(t, readme, readmePath)
			}
			assertRepoFileExists(t, readmePath)
			assertRepoFileExists(t, scenario.dir+"/sample-input.md")
			assertRepoFileExists(t, scenario.dir+"/expected-output.md")

			scenarioReadme := readRepoFile(t, readmePath)
			for _, want := range []string{
				scenario.title,
				"## Role",
				"## Why Use ORAG",
				"## When To Use It",
				"## Scenario Files",
				"## Run",
				"## Reused Assets",
				"## Expected Output",
				"sample-input.md",
				"expected-output.md",
				"Commands below reference maintained examples instead of duplicating raw API calls.",
			} {
				if !strings.Contains(scenarioReadme, want) {
					t.Fatalf("%s missing %q", readmePath, want)
				}
			}

			for _, asset := range scenario.assets {
				assertReferencedPathExists(t, scenarioReadme, asset)
			}
			if scenario.runnable {
				for _, want := range []string{
					"demo-data.md",
					"run.sh",
					"## Demo Implementation",
				} {
					if !strings.Contains(scenarioReadme, want) {
						t.Fatalf("%s missing runnable demo marker %q", readmePath, want)
					}
				}
			}
		})
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

func assertRepoFileExists(t *testing.T, path string) {
	t.Helper()

	info, err := os.Stat(filepath.Join("..", "..", filepath.FromSlash(path)))
	if err != nil {
		t.Fatalf("example path %s does not exist: %v", path, err)
	}
	if info.IsDir() {
		t.Fatalf("example path %s is a directory", path)
	}
}

func assertRepoFileExecutable(t *testing.T, path string) {
	t.Helper()

	info, err := os.Stat(filepath.Join("..", "..", filepath.FromSlash(path)))
	if err != nil {
		t.Fatalf("example path %s does not exist: %v", path, err)
	}
	if info.IsDir() {
		t.Fatalf("example path %s is a directory", path)
	}
	if info.Mode()&0111 == 0 {
		t.Fatalf("example path %s is not executable", path)
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
