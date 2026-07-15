# 开发指南

本文面向本地后端开发和验证，所有命令默认从仓库根目录 [`/Users/bytedance/Documents/orag`](..) 执行。当前仓库的默认真实后端为 `STORAGE_BACKEND=qdrant_postgres`，依赖 PostgreSQL 和 Qdrant；无依赖调试可临时切到 `STORAGE_BACKEND=memory`。

## 本地环境准备

本机需要准备：

- macOS + Docker Desktop，并确保 `docker compose` 可用。
- Go 1.26，仓库 [`go.mod`](../go.mod) 声明 `go 1.26`。
- `make`、`curl`，用于执行 Makefile 目标和健康检查。

初始化本地配置模板：

```bash
cp .env.example .env
```

[`.env.example`](../.env.example) 记录了本地默认配置，例如 `PORT=8080`、`DATABASE_URL=postgres://orag:orag@localhost:5432/orag?sslmode=disable`、`QDRANT_HOST=localhost`、`QDRANT_GRPC_PORT=6334` 和模型 provider 变量。当前 Go 进程通过环境变量和代码默认值读取配置，直接执行 `make run` 或 `go run` 时如需覆盖默认值，请在命令前传入或先 `export`；[`deployments/docker-compose.yml`](../deployments/docker-compose.yml) 中的 `orag-api` 容器会读取 `../.env`。

示例：

```bash
PORT=18080 DEBUG=true make run
DATABASE_URL="postgres://orag:orag@localhost:5432/orag?sslmode=disable" make migrate
```

默认运行要求真实模型 provider API Key。默认推荐火山引擎/方舟/Doubao，启动前至少配置 `ARK_API_KEY` 或 `VOLCENGINE_API_KEY`。只有在显式测试模式下才允许 deterministic mock，例如：

```bash
ALLOW_DETERMINISTIC_MOCK=true \
LLM_CHAT_PROVIDER=mock \
LLM_EMBEDDING_PROVIDER=mock \
LLM_RERANK_PROVIDER=mock \
LLM_MULTIMODAL_PROVIDER=mock \
STORAGE_BACKEND=memory make run
```

切换到其它 provider 时，先设置对应 `LLM_*_PROVIDER` 和 API key。大多数 provider 内置默认 endpoint；Azure OpenAI 还必须设置 `AZURE_OPENAI_BASE_URL`，Google Cloud 还必须设置 `GOOGLE_CLOUD_BASE_URL`。如需代理或私有网关，其它 provider 可用 `<PROVIDER>_BASE_URL` 覆盖默认 endpoint。

## 启动本地依赖

启动 PostgreSQL 和 Qdrant：

```bash
make dev-up
```

等价脚本入口：

```bash
scripts/dev-up.sh
```

`make dev-up` 只启动 [`deployments/docker-compose.yml`](../deployments/docker-compose.yml) 中的 `postgres` 和 `qdrant`，不启动 ES/Neo4j，也不启动 `orag-api` 容器。本地端口如下：

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

`make migrate` 执行 `go run ./cmd/oragctl migrate`，使用当前 `DATABASE_URL` 连接 PostgreSQL，并按顺序执行 `migrations/*.sql` 中的 `-- +goose Up` 片段。默认数据库地址来自 [`.env.example`](../.env.example) 和代码默认值：

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
ALLOW_DETERMINISTIC_MOCK=true LLM_CHAT_PROVIDER=mock LLM_EMBEDDING_PROVIDER=mock LLM_RERANK_PROVIDER=mock LLM_MULTIMODAL_PROVIDER=mock STORAGE_BACKEND=memory DEBUG=true make run
```

说明：

- `DEBUG=true` 会让 [`internal/platform/logger`](../internal/platform/logger) 输出 debug 级别 JSON 日志。
- `PORT`、`HOST`、`PUBLIC_BASE_URL` 可覆盖服务监听和外部访问地址。
- `STORAGE_BACKEND=memory` 可用于排查 HTTP 层、认证、API smoke 或单测问题；它不代表生产配置，也不会验证 PostgreSQL/Qdrant 链路。
- 服务内置入口包括 `GET /healthz`、`GET /readyz`、`GET /metrics` 和 `GET /docs`。

需要通过 Docker 构建或运行完整服务时使用：

```bash
make docker-build
make docker-run
```

`make docker-build` 使用 [`deployments/Dockerfile`](../deployments/Dockerfile)，构建阶段基于 `golang:1.26-alpine`，运行阶段基于 `alpine:3.20`。
`make docker-run` 使用 [`deployments/docker-compose.yml`](../deployments/docker-compose.yml) 启动完整栈；`orag-api` 会默认覆盖为容器网络地址 `DATABASE_URL=postgres://orag:orag@postgres:5432/orag?sslmode=disable`、`QDRANT_HOST=qdrant` 和 `QDRANT_GRPC_PORT=6334`，避免继承宿主 `.env` 中用于本机运行的 `localhost` 依赖地址。如需覆盖容器内依赖地址，请使用 `DOCKER_DATABASE_URL`、`DOCKER_QDRANT_HOST` 和 `DOCKER_QDRANT_GRPC_PORT`。

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
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=go1.26.5 go test ./...
```

### 安全门禁

[`security.yml`](../.github/workflows/security.yml) 在 Pull Request、`main` push、手动触发和每周计划任务中执行以下门禁：

- CodeQL 分别分析 Go 与 JavaScript/TypeScript，并把 SARIF 写入 GitHub code scanning；
- 使用 Go 1.26.5 运行根模块和独立 SDK consumer 的 `govulncheck`；
- 对 Console 执行可复现的 `npm ci` 与 production dependency audit；
- Gitleaks 扫描可达 Git 历史，不申请 PR comment 写权限；
- 构建 API 与 Console 镜像，并用 Trivy 阻止已有修复版本的 HIGH/CRITICAL 漏洞。

[`scorecard.yml`](../.github/workflows/scorecard.yml) 独立运行 OpenSSF Scorecard，以最小权限发布签名结果并上传 SARIF。两个 workflow 中新增的 action 都固定到完整 commit SHA，版本注释放在同一行供 Dependabot 和 reviewer 审查。

仓库中的所有 GitHub Actions 都固定到完整 commit SHA，Docker 构建的基础镜像固定到多架构 manifest digest；同行保留可读版本或镜像 tag，便于 Dependabot 提交可审查的更新。常规 CI 的 `GITHUB_TOKEN` 只具备 `contents: read`，只有 Pages 部署、GHCR 发布与 attestations、以及创建 GitHub Release 的对应 job 具备所需写权限。新增 workflow 或镜像阶段时必须延续这一策略，不能使用可移动的 action tag 或未固定 digest 的基础镜像。

本地可执行的等价检查：

```bash
GOTOOLCHAIN=local go install golang.org/x/vuln/cmd/govulncheck@v1.6.0
GOTOOLCHAIN=local govulncheck ./...
GOTOOLCHAIN=local GOWORK=off govulncheck -C tests/consumer ./...
npm --prefix console ci
npm --prefix console audit --omit=dev --audit-level=high
```

GitHub 原生 provider secret scanning 和 push protection 已启用。当前仓库属于个人账号；GitHub Secret Protection 才提供的 non-provider patterns 与 validity checks 不适用于该仓库，因此由 Gitleaks 全历史门禁补足 private key、generic credential 等检测，而不是把不可用设置描述为已开启。

#### GO-2026-5932 评估记录

Go 漏洞库将未维护且设计上不安全的 `golang.org/x/crypto/openpgp` 全版本标记为 [GO-2026-5932](https://pkg.go.dev/vuln/GO-2026-5932)，因此不存在可升级到的 x/crypto 修复版本。ORAG 的模块图因间接依赖仍包含 `golang.org/x/crypto`，但源码和编译依赖不导入 `openpgp`、`openpgp/packet` 等受影响包：`go mod why golang.org/x/crypto/openpgp` 返回 `(main module does not need package ...)`，使用 Go 1.26.5 的 `govulncheck ./...` 对 59 个根包和 55 个实际使用模块报告 `No vulnerabilities found`。

该记录解释 Scorecard 的模块级告警，不是漏洞豁免。任何新增 OpenPGP import 都必须使安全门禁失败并改用受维护实现。Eino 已从 0.6.0 升级到 0.9.12，专门回归测试验证其 Jinja `file`/`fileset` 过滤器被禁用，不能读取 API 或 CI 文件系统。

### 模糊测试

[`fuzz.yml`](../.github/workflows/fuzz.yml) 使用 Go 原生 coverage-guided fuzzing 持续探索两个直接处理不可信输入的边界：`BasicParser` 的文本、HTML、XML 与 Office ZIP 解析，以及 optimizer 表达式的 lexer、parser 和求值器。每个 Pull Request 和 `main` push 对两个 target 并行执行 20 秒；每周计划任务各执行 5 分钟。发生崩溃时，workflow 保存输入 artifact 供复现，但 corpus 与 crash 文件不提交到仓库。

本地快速复现：

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./internal/ingest/parser -run='^$' -fuzz='^FuzzBasicParser$' -fuzztime=30s
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./internal/optimizer -run='^$' -fuzz='^FuzzCompileExpression$' -fuzztime=30s
```

若 CI 发现 crash，下载 artifact 后把输入放入对应的 `testdata/fuzz/<Target>/`，先使用 `go test <package> -run='<Target>/<hash>'` 定点复现。修复后应把最小化、脱敏后的输入转换为具名单测或代码内 seed；不要提交自动生成的 corpus、原始 crash 或敏感文档内容。

### 契约测试

OpenAPI 契约校验：

```bash
make openapi-validate
```

等价原生命令：

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=go1.26.5 go test ./tests/contract -run TestOpenAPI -v
```

该测试读取 [`api/openapi.yaml`](../api/openapi.yaml)，用于验证 OpenAPI 文档结构和关键安全约束，不会启动外部服务。

### 集成测试

真实 PostgreSQL + Qdrant 集成测试默认跳过，需要显式启动 test compose：

```bash
make test-integration-up
make test-integration
make test-integration-down
```

`make test-integration-up` 使用 [`deployments/docker-compose.test.yml`](../deployments/docker-compose.test.yml)，测试端口和 collection 与日常开发环境隔离：

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
ORAG_INTEGRATION_TESTS=1 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=go1.26.5 go test ./tests/integration -v
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
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=go1.26.5 \
go test ./tests/integration -v
```

测试结束后执行 `make test-integration-down` 或 `docker compose -f deployments/docker-compose.test.yml down -v`，避免测试卷和端口占用影响下一次运行。

### Live Ark 测试

真实 Ark smoke test 默认跳过，只在显式开启时运行：

```bash
LIVE_ARK_TESTS=1 ARK_API_KEY="$ARK_API_KEY" CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=go1.26.5 go test ./tests/live -v
```

当前 [`tests/live/ark_live_test.go`](../tests/live/ark_live_test.go) 仍要求已开通的模型 endpoint 配置；没有补齐真实 endpoint 时，即使设置 `LIVE_ARK_TESTS=1` 也会按测试内逻辑 skip。不要把真实 `ARK_API_KEY` 提交到仓库。

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
GOTOOLCHAIN=go1.26.5 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go run ./examples/go/memory
```

## Mac、Go 1.26 和 Hertz 构建注意事项

本仓库使用 Hertz，间接依赖 Sonic。Mac 本地，尤其是 amd64 + Go 1.26 环境下，直接走 Sonic native/JIT 或本地 cgo 产物可能带来构建和链接问题。仓库统一约定：

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=go1.26.5
```

Makefile 已默认设置：

```makefile
GOFLAGS ?= -tags=stdjson,gjson
CGO_ENABLED ?= 0
```

因此优先使用 `make run`、`make test`、`make openapi-validate`、`make test-integration` 等目标。若直接执行 `go run`、`go test` 或 `go build`，请显式带上相同参数：

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=go1.26.5 go run ./cmd/orag-api
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=go1.26.5 go build ./cmd/orag-api
```

Docker 构建同样使用 `CGO_ENABLED=0` 和 `GOFLAGS=-tags=stdjson,gjson`：

```bash
docker build -f deployments/Dockerfile -t orag-api:local .
```

## Docker 镜像拉取注意事项

本地开发和测试会从 Docker Hub 拉取以下镜像：

- `postgres:16-alpine`
- `qdrant/qdrant:v1.11.5`
- `golang:1.26-alpine`
- `alpine:3.20`

如果首次执行 `make dev-up`、`make test-integration-up` 或 `make docker-build` 时拉取超时，先单独预拉取镜像，便于定位网络问题：

```bash
docker pull postgres:16-alpine
docker pull qdrant/qdrant:v1.11.5
docker pull golang:1.26-alpine
docker pull alpine:3.20
```

公司网络或代理环境下，如 Docker Hub 访问不稳定，需要在 Docker Desktop 中配置可用的 registry mirror 或代理后重试。镜像拉取失败不是 Go 测试失败；依赖镜像未就绪时，集成测试会因 PostgreSQL/Qdrant 连接失败或 ready 检查失败而跳过/失败。
