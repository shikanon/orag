APP_NAME := orag-api
GOFLAGS ?= -tags=stdjson,gjson
CGO_ENABLED ?= 0

.PHONY: run test vet fmt tidy sdk-check console-dev console-build console-test console-api-generate dev-up dev-down migrate openapi-validate agent-sync agent-sync-check agent-artifact-tests agent-gate mcp-self-check-smoke install-mcp install-skills-codex install-skills-claude install-skills-trae install-skills install-agent docker-build docker-run test-integration test-integration-up test-integration-down

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

console-api-generate:
	npm --prefix console run api:generate

dev-up:
	docker compose -f deployments/docker-compose.yml up -d postgres qdrant

dev-down:
	docker compose -f deployments/docker-compose.yml down

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
