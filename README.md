# ORAG

ORAG 是一个 Go 语言 RAG 服务框架，面向知识库入库、混合检索、问答生成、评估和参数优化的本地开发与 API 验证场景。服务基于 Hertz 暴露 HTTP API，使用 Eino Graph 编排 RAG 链路，默认以 Qdrant + PostgreSQL 作为真实依赖。

项目默认不要求真实 Ark Key。未配置 `ARK_API_KEY` 时，Ark/豆包适配层会使用 deterministic mock，便于本地开发、CI 和文档 smoke；需要真实模型调用或 live smoke 时再显式配置 Ark 环境变量和测试开关。

## 核心能力

- 知识库与文档入库：创建知识库，导入文本或上传文件，并写入检索索引。
- 混合检索：Qdrant dense vector retrieval、PostgreSQL FTS sparse retrieval、RRF 融合、Ark rerank 和 semantic cache。
- RAG 查询：支持普通 JSON 查询和 `POST /v1/query:stream` SSE 查询，返回答案、引用、trace、cache 状态和 warnings。
- 数据集与评估：支持数据集、样本、评估运行和评估结果持久化，当前指标为 deterministic rule-based metrics。
- 参数优化：`POST /v1/optimizations` 对候选 `profiles` 和 `top_ks` 做确定性网格搜索。
- 运维入口：提供 `GET /healthz`、`GET /readyz`、`GET /metrics` 和内置 `GET /docs`。

## 默认技术栈

| 层级 | 默认实现 | 说明 |
| --- | --- | --- |
| HTTP API | Hertz | API 服务入口在 `cmd/orag-api`，OpenAPI 源文件为 `api/openapi.yaml`。 |
| RAG 编排 | Eino Graph | 编排检索、重排、生成、引用和 cache 链路。 |
| 向量与语义缓存 | Qdrant | 默认 collection 为 `orag_chunks` 和 `orag_semantic_cache`。 |
| 元数据与稀疏检索 | PostgreSQL | 存储知识库、文档、chunk 元数据、FTS、数据集、评估结果和 trace。 |
| 模型接口 | 火山引擎方舟/豆包 | 通过 Ark 接入 Chat、Embedding、Rerank 和多模态解析；无 key 时使用 deterministic mock。 |
| 本地依赖 | Docker Compose | `make dev-up` 启动 PostgreSQL 和 Qdrant，不启动 ES/Neo4j。 |

默认 `STORAGE_BACKEND=qdrant_postgres` 依赖 PostgreSQL 和 Qdrant。`STORAGE_BACKEND=memory` 只适合本地无依赖调试、单测或排查 HTTP 层问题，不作为生产配置。

## 快速开始

准备本机 Go 环境和 Docker Compose 后：

```bash
cp .env.example .env
make dev-up
make migrate
make run
```

`make run` 会以前台方式启动 API 服务，默认监听 `http://localhost:8080`。服务启动后在另一个终端检查：

```bash
curl -fsS http://localhost:8080/healthz
curl -fsS http://localhost:8080/readyz
```

本地无依赖调试可临时使用 memory 后端：

```bash
STORAGE_BACKEND=memory make run
```

完成本地验证后可停止依赖：

```bash
make dev-down
```

## 配置与模型策略

`.env.example` 是本地配置模板，常用变量包括：

- 服务入口：`HOST`、`PORT`、`PUBLIC_BASE_URL`。
- 存储后端：`STORAGE_BACKEND`、`DATABASE_URL`。
- Qdrant：`QDRANT_HOST`、`QDRANT_GRPC_PORT`、`QDRANT_COLLECTION`、`QDRANT_SEMANTIC_CACHE_COLLECTION`、`QDRANT_AUTO_CREATE_COLLECTIONS`。
- 认证：`JWT_SECRET`、`ADMIN_DEFAULT_USERNAME`、`ADMIN_DEFAULT_PASSWORD`、`AUTH_TOKEN_TTL`。
- Ark/豆包：`ARK_API_KEY`、`ARK_BASE_URL`、`ARK_CHAT_MODEL`、`ARK_EMBEDDING_MODEL`、`ARK_MULTIMODAL_MODEL`。
- Rerank：`RERANK_PROVIDER` 可选 `volcengine` 或 `aliyun`；火山/方舟使用 `ARK_RERANK_BASE_URL`、`ARK_RERANK_MODEL`，阿里云百炼使用 `ALIYUN_RERANK_API_KEY`、`ALIYUN_RERANK_BASE_URL`、`ALIYUN_RERANK_MODEL`。

默认 `.env.example` 中 `ARK_API_KEY` 为空、`REQUIRE_EXTERNAL_PROVIDERS=false`。此时本地测试和 API smoke 会走 deterministic mock；`/readyz` 只报告 Ark 状态为 `mock` 或 `configured`，不会主动调用外部 Ark 服务。

需要真实模型调用时，在 `.env` 或运行环境中配置 `ARK_API_KEY` 和对应模型变量。默认 embedding 模型为火山 `doubao-embedding-vision-251215`，真实 Ark smoke test 还需要显式设置 `LIVE_ARK_TESTS=1`，否则默认跳过。

## API Smoke

服务启动后按顺序执行：

```bash
examples/curl/00_login.sh
examples/curl/10_create_kb.sh
examples/curl/20_upload_doc.sh
examples/curl/30_query.sh
examples/curl/40_eval.sh
```

脚本默认请求 `BASE_URL=http://localhost:8080`，可通过 `BASE_URL` 覆盖。登录默认账号来自 `.env.example` 中的 `ADMIN_DEFAULT_USERNAME=admin` 和 `ADMIN_DEFAULT_PASSWORD=admin`。

脚本运行状态保存在 `.orag-demo/`，包括本地 token、知识库 ID、文档 ID 和数据集 ID，不应提交。

OpenAPI 源文件为 `api/openapi.yaml`，服务内置文档入口为 `GET /docs`。

## 验证

常用本地验证命令：

```bash
make fmt
make vet
make test
make openapi-validate
```

`Makefile` 默认为 Go 命令注入 `CGO_ENABLED=0` 和 `GOFLAGS=-tags=stdjson,gjson`，用于规避 Mac amd64 + Go 1.22 下 Hertz/Sonic native 与本地 cgo 链接产物的问题。直接运行原生命令时可显式带上：

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./...
```

真实 PostgreSQL + Qdrant 集成测试默认跳过，显式运行：

```bash
make test-integration-up
make test-integration
make test-integration-down
```

`make test-integration` 会设置 `ORAG_INTEGRATION_TESTS=1`，并使用 `deployments/docker-compose.test.yml` 的测试端口和 collection。

真实 Ark smoke test 默认跳过，只在显式开启时运行：

```bash
LIVE_ARK_TESTS=1 ARK_API_KEY="$ARK_API_KEY" CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./tests/live -v
```

## 文档索引

- `docs/Go-RAG-框架技术方案.md`：整体技术方案。
- `docs/api.md`：认证、知识库、入库、查询、SSE、数据集、评估、优化和错误响应。
- `docs/development.md`：本地开发、依赖启动、测试矩阵、集成测试和 live Ark 测试。
- `docs/evaluation.md`：数据集结构、当前 rule-based metrics、optimizer 和后续增强边界。
- `docs/operations.md`：部署依赖、健康检查、metrics、配置安全和故障排查。
