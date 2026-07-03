# 开发指南

本文面向本地后端开发和验证，所有命令默认从仓库根目录 `/Users/bytedance/Documents/orag` 执行。当前仓库的默认真实后端为 `STORAGE_BACKEND=qdrant_postgres`，依赖 PostgreSQL 和 Qdrant；无依赖调试可临时切到 `STORAGE_BACKEND=memory`。

## 本地环境准备

本机需要准备：

- macOS + Docker Desktop，并确保 `docker compose` 可用。
- Go 1.22，仓库 `go.mod` 声明 `go 1.22`。
- `make`、`curl`，用于执行 Makefile 目标和健康检查。

初始化本地配置模板：

```bash
cp .env.example .env
```

`.env.example` 记录了本地默认配置，例如 `PORT=8080`、`DATABASE_URL=postgres://orag:orag@localhost:5432/orag?sslmode=disable`、`QDRANT_HOST=localhost`、`QDRANT_GRPC_PORT=6334` 和 Ark 模型变量。当前 Go 进程通过环境变量和代码默认值读取配置，直接执行 `make run` 或 `go run` 时如需覆盖默认值，请在命令前传入或先 `export`；`deployments/docker-compose.yml` 中的 `orag-api` 容器会读取 `../.env`。

示例：

```bash
PORT=18080 DEBUG=true make run
DATABASE_URL="postgres://orag:orag@localhost:5432/orag?sslmode=disable" make migrate
```

未配置 `ARK_API_KEY` 时，Ark/豆包适配层使用 deterministic mock，适合本地开发、单元测试和 CI。需要真实模型调用时再显式配置 `ARK_API_KEY`、模型变量和对应测试开关。

## 启动本地依赖

启动 PostgreSQL 和 Qdrant：

```bash
make dev-up
```

等价脚本入口：

```bash
scripts/dev-up.sh
```

`make dev-up` 只启动 `deployments/docker-compose.yml` 中的 `postgres` 和 `qdrant`，不启动 ES/Neo4j，也不启动 `orag-api` 容器。本地端口如下：

| 依赖 | 地址 | 说明 |
| --- | --- | --- |
| PostgreSQL | `localhost:5432` | 用户名、密码和库名均为 `orag`。 |
| Qdrant HTTP | `localhost:6333` | 可用 `curl -fsS http://localhost:6333/readyz` 检查。 |
| Qdrant gRPC | `localhost:6334` | 后端通过 `QDRANT_HOST` 和 `QDRANT_GRPC_PORT` 访问。 |

查看依赖状态：

```bash
docker compose -f deployments/docker-compose.yml ps
curl -fsS http://localhost:6333/readyz
```

停止依赖：

```bash
make dev-down
```

如需同时删除本地开发卷，可使用脚本开关：

```bash
DEV_DOWN_VOLUMES=1 scripts/dev-down.sh
```

## 迁移数据库

首次启动真实后端前运行迁移：

```bash
make migrate
```

`make migrate` 执行 `go run ./cmd/oragctl migrate`，使用当前 `DATABASE_URL` 连接 PostgreSQL，并按顺序执行 `migrations/*.sql` 中的 `-- +goose Up` 片段。默认数据库地址来自 `.env.example` 和代码默认值：

```bash
postgres://orag:orag@localhost:5432/orag?sslmode=disable
```

如果本地使用了非默认端口或数据库，请显式传入：

```bash
DATABASE_URL="postgres://orag:orag@localhost:5432/orag?sslmode=disable" make migrate
```

`STORAGE_BACKEND=memory` 只适合无依赖调试，不需要 PostgreSQL 迁移。

## 运行和调试后端

使用真实 PostgreSQL + Qdrant 后端：

```bash
make run
```

`make run` 执行 `go run ./cmd/orag-api`，默认监听 `http://localhost:8080`。启动后可在另一个终端检查：

```bash
curl -fsS http://localhost:8080/healthz
curl -fsS http://localhost:8080/readyz
scripts/wait-ready.sh http://localhost:8080/readyz
```

常用调试参数：

```bash
DEBUG=true make run
PORT=18080 DEBUG=true make run
STORAGE_BACKEND=memory DEBUG=true make run
```

说明：

- `DEBUG=true` 会让 `internal/platform/logger` 输出 debug 级别 JSON 日志。
- `PORT`、`HOST`、`PUBLIC_BASE_URL` 可覆盖服务监听和外部访问地址。
- `STORAGE_BACKEND=memory` 可用于排查 HTTP 层、认证、API smoke 或单测问题；它不代表生产配置，也不会验证 PostgreSQL/Qdrant 链路。
- 服务内置入口包括 `GET /healthz`、`GET /readyz`、`GET /metrics` 和 `GET /docs`。

需要通过 Docker 构建或运行完整服务时使用：

```bash
make docker-build
make docker-run
```

`make docker-build` 使用 `deployments/Dockerfile`，构建阶段基于 `golang:1.22-alpine`，运行阶段基于 `alpine:3.20`。

## 测试矩阵

### 格式化、静态检查和单元测试

```bash
make fmt
make vet
make test
```

`make test` 执行 `go test ./...`，并通过 Makefile 注入 `CGO_ENABLED=0` 和 `GOFLAGS=-tags=stdjson,gjson`。

直接运行原生命令时建议显式带上与 Makefile 一致的参数：

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./...
```

### 契约测试

OpenAPI 契约校验：

```bash
make openapi-validate
```

等价原生命令：

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./tests/contract -run TestOpenAPI -v
```

该测试读取 `api/openapi.yaml`，用于验证 OpenAPI 文档结构和关键安全约束，不会启动外部服务。

### 集成测试

真实 PostgreSQL + Qdrant 集成测试默认跳过，需要显式启动 test compose：

```bash
make test-integration-up
make test-integration
make test-integration-down
```

`make test-integration-up` 使用 `deployments/docker-compose.test.yml`，测试端口和 collection 与日常开发环境隔离：

| 依赖 | 地址或名称 |
| --- | --- |
| PostgreSQL | `localhost:55432`，数据库 `orag_test` |
| Qdrant HTTP | `localhost:6633` |
| Qdrant gRPC | `localhost:6634` |
| Qdrant chunk collection | `orag_chunks_test` |
| Qdrant semantic cache collection | `orag_semantic_cache_test` |

`make test-integration` 会设置 `ORAG_INTEGRATION_TESTS=1`、测试数据库 URL、Qdrant gRPC 端口和测试 collection。需要手动执行时使用：

```bash
docker compose -f deployments/docker-compose.test.yml up -d
ORAG_INTEGRATION_TESTS=1 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./tests/integration -v
docker compose -f deployments/docker-compose.test.yml down -v
```

如果要完全复现 Makefile 的测试环境变量，可执行：

```bash
ORAG_INTEGRATION_TESTS=1 \
DATABASE_URL="postgres://orag:orag@localhost:55432/orag_test?sslmode=disable" \
QDRANT_HOST="localhost" \
QDRANT_GRPC_PORT="6634" \
QDRANT_COLLECTION="orag_chunks_test" \
QDRANT_SEMANTIC_CACHE_COLLECTION="orag_semantic_cache_test" \
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local \
go test ./tests/integration -v
```

测试结束后执行 `make test-integration-down` 或 `docker compose -f deployments/docker-compose.test.yml down -v`，避免测试卷和端口占用影响下一次运行。

### Live Ark 测试

真实 Ark smoke test 默认跳过，只在显式开启时运行：

```bash
LIVE_ARK_TESTS=1 ARK_API_KEY="$ARK_API_KEY" CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./tests/live -v
```

当前 `tests/live/ark_live_test.go` 仍要求已开通的模型 endpoint 配置；没有补齐真实 endpoint 时，即使设置 `LIVE_ARK_TESTS=1` 也会按测试内逻辑 skip。不要把真实 `ARK_API_KEY` 提交到仓库。

## 常用 API smoke

完整示例索引见 [`../examples/README.md`](../examples/README.md)。服务模式示例需要先完成依赖启动、迁移并保持 API 服务运行：

```bash
make dev-up
make migrate
make run
```

在另一个终端按顺序执行 curl 示例：

```bash
examples/curl/05_health_ready.sh
examples/curl/00_login.sh
examples/curl/10_create_kb.sh
examples/curl/20_upload_doc.sh
examples/curl/25_upload_file.sh
examples/curl/30_query.sh
examples/curl/35_query_stream.sh
examples/curl/36_trace_lookup.sh
examples/curl/40_eval.sh
examples/curl/45_optimize.sh
```

脚本默认请求 `BASE_URL=http://localhost:8080`，登录参数默认为 `ADMIN_USERNAME=admin` 和 `ADMIN_PASSWORD=admin`；如果服务端 `.env` 覆盖了默认管理员账号，请同步覆盖这些客户端变量。脚本会把临时 token、KB ID、trace ID、dataset ID 等状态写入 `.orag-demo/`，该目录不应提交；可用 `STATE_DIR` 改到其它临时目录。

Go memory 示例展示无外部依赖的入库、查询和 trace/response 元数据读取，适合快速验证示例包可编译运行：

```bash
GOTOOLCHAIN=local CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go run ./examples/go/memory
```

## Mac、Go 1.22 和 Hertz 构建注意事项

本仓库使用 Hertz，间接依赖 Sonic。Mac 本地，尤其是 amd64 + Go 1.22 环境下，直接走 Sonic native/JIT 或本地 cgo 产物可能带来构建和链接问题。仓库统一约定：

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local
```

Makefile 已默认设置：

```makefile
GOFLAGS ?= -tags=stdjson,gjson
CGO_ENABLED ?= 0
```

因此优先使用 `make run`、`make test`、`make openapi-validate`、`make test-integration` 等目标。若直接执行 `go run`、`go test` 或 `go build`，请显式带上相同参数：

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go run ./cmd/orag-api
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go build ./cmd/orag-api
```

Docker 构建同样使用 `CGO_ENABLED=0` 和 `GOFLAGS=-tags=stdjson,gjson`：

```bash
docker build -f deployments/Dockerfile -t orag-api:local .
```

## Docker 镜像拉取注意事项

本地开发和测试会从 Docker Hub 拉取以下镜像：

- `postgres:16-alpine`
- `qdrant/qdrant:v1.11.5`
- `golang:1.22-alpine`
- `alpine:3.20`

如果首次执行 `make dev-up`、`make test-integration-up` 或 `make docker-build` 时拉取超时，先单独预拉取镜像，便于定位网络问题：

```bash
docker pull postgres:16-alpine
docker pull qdrant/qdrant:v1.11.5
docker pull golang:1.22-alpine
docker pull alpine:3.20
```

公司网络或代理环境下，如 Docker Hub 访问不稳定，需要在 Docker Desktop 中配置可用的 registry mirror 或代理后重试。镜像拉取失败不是 Go 测试失败；依赖镜像未就绪时，集成测试会因 PostgreSQL/Qdrant 连接失败或 ready 检查失败而跳过/失败。
