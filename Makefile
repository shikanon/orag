APP_NAME := orag-api
GOFLAGS ?= -tags=stdjson,gjson
CGO_ENABLED ?= 0

.PHONY: run test vet fmt tidy dev-up dev-down migrate openapi-validate agent-sync agent-sync-check docker-build docker-run test-integration test-integration-up test-integration-down

run:
	CGO_ENABLED="$(CGO_ENABLED)" GOFLAGS="$(GOFLAGS)" go run ./cmd/orag-api

test:
	CGO_ENABLED="$(CGO_ENABLED)" GOFLAGS="$(GOFLAGS)" go test ./...

vet:
	CGO_ENABLED="$(CGO_ENABLED)" GOFLAGS="$(GOFLAGS)" go vet ./...

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './vendor/*')

tidy:
	CGO_ENABLED="$(CGO_ENABLED)" GOFLAGS="$(GOFLAGS)" go mod tidy

dev-up:
	docker compose -f deployments/docker-compose.yml up -d postgres qdrant

dev-down:
	docker compose -f deployments/docker-compose.yml down

migrate:
	CGO_ENABLED="$(CGO_ENABLED)" GOFLAGS="$(GOFLAGS)" go run ./cmd/oragctl migrate

openapi-validate:
	CGO_ENABLED="$(CGO_ENABLED)" GOFLAGS="$(GOFLAGS)" go test ./tests/contract -run TestOpenAPI -v

agent-sync:
	CGO_ENABLED="$(CGO_ENABLED)" GOFLAGS="$(GOFLAGS)" go run ./cmd/oragctl generate-agent-artifacts --openapi api/openapi.yaml --out .

agent-sync-check:
	CGO_ENABLED="$(CGO_ENABLED)" GOFLAGS="$(GOFLAGS)" go run ./cmd/oragctl generate-agent-artifacts --openapi api/openapi.yaml --out . --check

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
