APP_NAME := orag-api
GOFLAGS ?= -tags=stdjson,gjson
CGO_ENABLED ?= 0

.PHONY: run test vet fmt tidy sdk-check compatibility-audit console-dev console-build console-test console-api-generate console-real-e2e console-real-tutorial-clone-e2e console-real-tutorial-benchmark-e2e backup-restore-drill credential-rotation-drill benchmark-report-run benchmark-report-verify performance-baseline-evidence-verify docs-build dev-up dev-down demo demo-down migrate openapi-validate agent-sync agent-sync-check agent-artifact-tests agent-gate mcp-self-check-smoke install-mcp install-skills-codex install-skills-trae install-skills install-agent docker-build docker-run test-integration test-integration-up test-integration-down tutorial-pack-build tutorial-pack-verify tutorial-pack-publish video-protocol-verify video-protocol-publish

TUTORIAL_PACK_SOURCE ?=
TUTORIAL_PACK_OUTPUT ?= .tmp/tutorial-packs
TUTORIAL_PACK_ROOT ?= $(TUTORIAL_PACK_OUTPUT)/text-rag/1.1.0

run:
	CGO_ENABLED="$(CGO_ENABLED)" GOFLAGS="$(GOFLAGS)" go run ./cmd/orag-api

test:
	CGO_ENABLED="$(CGO_ENABLED)" GOFLAGS="$(GOFLAGS)" go test ./...

vet:
	CGO_ENABLED="$(CGO_ENABLED)" GOFLAGS="$(GOFLAGS)" go vet ./...

sdk-check:
	CGO_ENABLED="$(CGO_ENABLED)" GOFLAGS="$(GOFLAGS)" go test . ./tests/contract -run 'Test(PublicSDK|SDK)' -v
	cd tests/consumer && GOWORK=off CGO_ENABLED="$(CGO_ENABLED)" GOFLAGS="$(GOFLAGS)" go test ./...
	cd tests/consumer && GOWORK=off CGO_ENABLED="$(CGO_ENABLED)" GOFLAGS="$(GOFLAGS)" go vet ./...

compatibility-audit:
	@test -n "$(COMPATIBILITY_BASE)" || (echo "COMPATIBILITY_BASE must name the previous published tag"; exit 2)
	CGO_ENABLED="$(CGO_ENABLED)" GOFLAGS="$(GOFLAGS)" go run ./cmd/oragctl compatibility-audit --base "$(COMPATIBILITY_BASE)"

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './vendor/*')

tidy:
	CGO_ENABLED="$(CGO_ENABLED)" GOFLAGS="$(GOFLAGS)" go mod tidy

console-dev:
	npm --prefix console run dev

console-build:
	npm --prefix console run build

console-test:
	npm --prefix console test -- --run

console-real-e2e:
	./scripts/console-real-backend-e2e.sh

console-real-tutorial-clone-e2e:
	./scripts/console-real-backend-tutorial-clone-e2e.sh

console-real-tutorial-benchmark-e2e:
	./scripts/console-real-backend-tutorial-benchmark-e2e.sh

backup-restore-drill:
	./scripts/backup-restore-drill.sh

credential-rotation-drill:
	./scripts/credential-rotation-drill.sh

benchmark-report-verify:
	@test -n "$(BENCHMARK_REPORT)" || (echo "BENCHMARK_REPORT must point to a performance baseline report JSON file"; exit 2)
	CGO_ENABLED="$(CGO_ENABLED)" GOFLAGS="$(GOFLAGS)" go run ./cmd/oragctl benchmark-report --file "$(BENCHMARK_REPORT)"

benchmark-report-run:
	@test -n "$(BENCHMARK_REPORT)" || (echo "BENCHMARK_REPORT must point to the output performance baseline JSON file"; exit 2)
	CGO_ENABLED="$(CGO_ENABLED)" GOFLAGS="$(GOFLAGS)" go run ./cmd/oragctl benchmark-run --output "$(BENCHMARK_REPORT)" --build-revision "$$(git rev-parse HEAD)"

performance-baseline-evidence-verify:
	@test -n "$(PERFORMANCE_BASELINE_EVIDENCE)" || (echo "PERFORMANCE_BASELINE_EVIDENCE must point to a public performance baseline evidence directory"; exit 2)
	./scripts/verify-performance-baseline-evidence.sh --dir "$(PERFORMANCE_BASELINE_EVIDENCE)"

console-api-generate:
	npm --prefix console run api:generate

docs-build:
	./scripts/build-docs-site.sh

tutorial-pack-build:
	@test -n "$(TUTORIAL_PACK_SOURCE)" || (echo "TUTORIAL_PACK_SOURCE must be a clean CRUD-RAG checkout"; exit 2)
	CGO_ENABLED="$(CGO_ENABLED)" GOFLAGS="$(GOFLAGS)" go run ./cmd/orag-pack-release -source "$(TUTORIAL_PACK_SOURCE)" -output "$(TUTORIAL_PACK_OUTPUT)" -version 1.1.0

tutorial-pack-verify:
	CGO_ENABLED="$(CGO_ENABLED)" GOFLAGS="$(GOFLAGS)" go run ./cmd/orag-pack-release -verify-public "$(TUTORIAL_PACK_ROOT)"

tutorial-pack-publish:
	@test "$(ORAG_PACK_PUBLISH)" = "1" || (echo "set ORAG_PACK_PUBLISH=1 to publish immutable public Pack objects"; exit 2)
	CGO_ENABLED="$(CGO_ENABLED)" GOFLAGS="$(GOFLAGS)" go run ./cmd/orag-pack-release -publish "$(TUTORIAL_PACK_ROOT)"

visual-recipe-verify:
	@test -n "$(VISUAL_RECIPE_ROOT)" || (echo "VISUAL_RECIPE_ROOT must point to a visual-document-rag Recipe version"; exit 2)
	CGO_ENABLED="$(CGO_ENABLED)" GOFLAGS="$(GOFLAGS)" go run ./cmd/orag-pack-release -verify-public "$(VISUAL_RECIPE_ROOT)"

visual-recipe-publish:
	@test "$(ORAG_PACK_PUBLISH)" = "1" || (echo "set ORAG_PACK_PUBLISH=1 to publish immutable public Recipe objects"; exit 2)
	@test -n "$(VISUAL_RECIPE_ROOT)" || (echo "VISUAL_RECIPE_ROOT must point to a visual-document-rag Recipe version"; exit 2)
	CGO_ENABLED="$(CGO_ENABLED)" GOFLAGS="$(GOFLAGS)" go run ./cmd/orag-pack-release -publish "$(VISUAL_RECIPE_ROOT)"

video-protocol-verify:
	@test -n "$(VIDEO_PROTOCOL_ROOT)" || (echo "VIDEO_PROTOCOL_ROOT must point to tutorial-protocols/video-rag/1.0.0"; exit 2)
	CGO_ENABLED="$(CGO_ENABLED)" GOFLAGS="$(GOFLAGS)" go run ./cmd/orag-pack-release -verify-public "$(VIDEO_PROTOCOL_ROOT)" -public-base-url "$(VIDEO_PROTOCOL_PUBLIC_BASE_URL)"

video-protocol-publish:
	@test "$(ORAG_PACK_PUBLISH)" = "1" || (echo "set ORAG_PACK_PUBLISH=1 to publish immutable public Protocol objects"; exit 2)
	@test -n "$(VIDEO_PROTOCOL_ROOT)" || (echo "VIDEO_PROTOCOL_ROOT must point to tutorial-protocols/video-rag/1.0.0"; exit 2)
	CGO_ENABLED="$(CGO_ENABLED)" GOFLAGS="$(GOFLAGS)" go run ./cmd/orag-pack-release -publish "$(VIDEO_PROTOCOL_ROOT)"

dev-up:
	docker compose -f deployments/docker-compose.yml up -d postgres qdrant

dev-down:
	docker compose -f deployments/docker-compose.yml down

demo:
	./scripts/mock-walkthrough.sh

demo-down:
	docker compose -f deployments/docker-compose.yml --profile demo down

migrate:
	CGO_ENABLED="$(CGO_ENABLED)" GOFLAGS="$(GOFLAGS)" go run ./cmd/oragctl migrate

openapi-validate:
	CGO_ENABLED="$(CGO_ENABLED)" GOFLAGS="$(GOFLAGS)" go test ./tests/contract -run TestOpenAPI -v

agent-sync:
	CGO_ENABLED="$(CGO_ENABLED)" GOFLAGS="$(GOFLAGS)" go run ./cmd/oragctl generate-agent-artifacts --manifest builtin --out .

agent-sync-check:
	CGO_ENABLED="$(CGO_ENABLED)" GOFLAGS="$(GOFLAGS)" go run ./cmd/oragctl generate-agent-artifacts --manifest builtin --out . --check

agent-artifact-tests:
	CGO_ENABLED="$(CGO_ENABLED)" GOFLAGS="$(GOFLAGS)" go test ./internal/mcp ./internal/agentskills ./internal/agentsync ./cmd/oragctl ./tests/contract -run 'Test(ServerListsAndRunsSelfCheckTool|ServerListsAndRunsDiagnosticTools|ServerRunsSelfOpsPlanAndApplyTools|GenerateFromManifest|WriteAndCheckFilesDetectsStaticDrift|GenerateAgentArtifactsCmdWritesMCPAndSkillOutputs|MCPAndSkillExamplesDocument|ExamplesReadmeIndex|ExamplesScriptPaths)' -v

mcp-self-check-smoke:
	@tmp="$$(mktemp)"; \
	CGO_ENABLED="$(CGO_ENABLED)" GOFLAGS="$(GOFLAGS)" go run ./cmd/orag-mcp --openapi api/openapi.yaml < examples/mcp/self-check-stdio-smoke.jsonl > "$$tmp"; \
	grep -q '"name":"orag_check"' "$$tmp"; \
	grep -q '"structuredContent"' "$$tmp"; \
	grep -q '"runtime_gate_warning"' "$$tmp"; \
	rm -f "$$tmp"

install-mcp:
	@mkdir -p .mcp/tools
	@cp agent/mcp/openapi-facet.json .mcp/openapi-facet.json
	@cp agent/mcp/tools/*.json .mcp/tools/
	@echo "installed MCP tools to .mcp/"

install-skills-codex:
	@mkdir -p .codex/skills
	@cp -R agent/skills/codex/* .codex/skills/
	@echo "installed Codex skills to .codex/skills/"

install-skills-claude:
	@mkdir -p .claude/skills
	@cp -R agent/skills/claude-code/* .claude/skills/
	@echo "installed Claude Code skills to .claude/skills/"

install-skills-trae:
	@mkdir -p .trae/skills
	@cp -R agent/skills/trae/* .trae/skills/
	@echo "installed Trae skills to .trae/skills/"

install-skills: install-skills-codex install-skills-claude install-skills-trae

install-agent: install-mcp install-skills
	@echo "installed all agent artifacts to hidden deployment directories"

agent-gate: agent-sync-check agent-artifact-tests mcp-self-check-smoke openapi-validate sdk-check test vet

test-integration:
	ORAG_INTEGRATION_TESTS=1 DATABASE_URL="postgres://orag:orag@localhost:55432/orag_test?sslmode=disable" QDRANT_HOST="localhost" QDRANT_GRPC_PORT="6634" QDRANT_COLLECTION="orag_chunks_test" QDRANT_SEMANTIC_CACHE_COLLECTION="orag_semantic_cache_test" CGO_ENABLED="$(CGO_ENABLED)" GOFLAGS="$(GOFLAGS)" go test ./tests/integration -v

test-integration-up:
	docker compose -f deployments/docker-compose.test.yml up -d

test-integration-down:
	docker compose -f deployments/docker-compose.test.yml down -v

docker-build:
	docker build -f deployments/Dockerfile -t $(APP_NAME):local .

docker-run:
	docker compose -f deployments/docker-compose.yml up --build
