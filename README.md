<div align="center">

# ORAG

**A Go-native RAG service framework for ingestion, hybrid retrieval, generation, evaluation, and optimization.**

<p>
  <a href="./README.md"><img alt="README in Chinese" src="https://img.shields.io/badge/简体中文-DBEDFA"></a>
  <a href="./LICENSE"><img alt="License" src="https://img.shields.io/github/license/shikanon/orag?color=4e6b99"></a>
  <a href="https://github.com/shikanon/orag/actions/workflows/ci.yml"><img alt="CI" src="https://img.shields.io/github/actions/workflow/status/shikanon/orag/ci.yml?branch=main&label=CI"></a>
  <a href="./go.mod"><img alt="Go Version" src="https://img.shields.io/badge/Go-1.22-00ADD8?logo=go&logoColor=white"></a>
  <a href="./api/openapi.yaml"><img alt="OpenAPI" src="https://img.shields.io/badge/OpenAPI-3.x-6BA539?logo=openapiinitiative&logoColor=white"></a>
</p>

<p>
  <a href="#快速开始">快速开始</a> ·
  <a href="#核心能力">核心能力</a> ·
  <a href="#架构概览">架构概览</a> ·
  <a href="./docs/README.md">文档中心</a> ·
  <a href="./api/openapi.yaml">OpenAPI</a>
</p>

</div>

ORAG 是一个面向本地开发、API 验证和 RAG 工程落地的 Go 服务框架。它基于 Hertz 暴露 HTTP API，使用 Eino Graph 编排 RAG 链路，并以 Qdrant + PostgreSQL 作为默认真实依赖，覆盖从知识库入库到混合检索、问答生成、评估回归和参数优化的完整闭环。

项目默认不要求真实 Ark Key。未配置 `ARK_API_KEY` 时，Ark/豆包适配层会使用 deterministic mock，便于本地开发、CI 和文档 smoke；需要真实模型调用或 live smoke 时再显式配置 Ark 环境变量和测试开关。

## 为什么是 ORAG

| 目标 | ORAG 的取舍 |
| --- | --- |
| Go-native RAG 服务 | 用 Go/Hertz/Eino 组织 API、Graph、存储和观测，便于接入现有后端工程体系。 |
| 真实依赖优先 | 默认走 `qdrant_postgres`，用 Qdrant 承载 dense retrieval 和 semantic cache，用 PostgreSQL 承载元数据与 FTS sparse retrieval。 |
| 本地可无 key 验证 | 未配置 Ark 时使用 deterministic mock，保证单测、契约测试和 smoke 流程稳定可跑。 |
| API 契约清晰 | OpenAPI、curl 示例、契约测试和内置 `/docs` 对齐，降低集成成本。 |
| 评估闭环内建 | 数据集、评估运行、结果持久化和 optimizer 都复用线上 RAG 查询路径，减少线上线下漂移。 |

## 核心能力

| 能力 | 当前实现 | 入口 |
| --- | --- | --- |
| 认证与租户上下文 | 管理员登录换取 Bearer token，业务请求从 token 获取默认 tenant。 | `POST /v1/auth/login` |
| 知识库管理 | 创建、列表、详情、删除知识库，支持 metadata。 | `/v1/knowledge-bases` |
| 文档入库 | 支持 JSON 文本导入和 multipart 文件上传，记录 ingestion job 结果；支持 basic、MinerU、Docling 解析方法，PDF/DOCX 中的图片可由多模态模型转写为可检索文本。 | `/documents:import`、`/documents`、`/ingestion-jobs/{id}` |
| 混合检索 | Qdrant dense retrieval、PostgreSQL FTS sparse retrieval、RRF 融合、Ark rerank。 | `internal/kb`、`internal/rag` |
| RAG 查询 | JSON 查询和 SSE 流式查询，返回答案、引用、trace、cache 状态和 warnings。 | `POST /v1/query`、`POST /v1/query:stream` |
| 评估与优化 | 数据集、评估运行、rule-based metrics、profile/top-k 网格搜索。 | `/v1/datasets`、`/v1/evaluations`、`/v1/optimizations` |
| 运维观测 | 存活检查、就绪检查、Prometheus 文本指标、结构化日志。 | `/healthz`、`/readyz`、`/metrics` |

## 架构概览

```text
Client / curl / SDK
        |
        v
Hertz HTTP API  ---->  Auth / Tenant / Error Model
        |
        v
Eino RAG Graph
        |
        +--> Parser / Chunker / Loader
        +--> Qdrant dense retrieval + semantic cache
        +--> PostgreSQL metadata + FTS sparse retrieval
        +--> RRF fusion + rerank
        +--> Ark/Doubao chat, embedding, multimodal adapters
        |
        v
Answer + Citations + Trace + Metrics
```

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

准备本机 Go 1.22、Docker Desktop 和 `docker compose` 后：

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

完成本地验证后停止依赖：

```bash
make dev-down
```

## API Smoke

完整示例入口见 [examples/README.md](./examples/README.md)。服务模式示例需要先启动 API 服务，默认请求 `BASE_URL=http://localhost:8080`，可通过 `BASE_URL` 覆盖：

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

登录脚本默认使用 `ADMIN_USERNAME=admin` 和 `ADMIN_PASSWORD=admin` 调用服务端默认管理员账号；如 `.env` 中修改了 `ADMIN_DEFAULT_USERNAME` 或 `ADMIN_DEFAULT_PASSWORD`，请同步覆盖客户端变量。

脚本运行状态保存在 `.orag-demo/`，包括本地 token、知识库 ID、文档 ID、trace ID、数据集 ID 和入库 job ID，不应提交；可通过 `STATE_DIR` 指向其它临时目录。

Go memory 示例不依赖 PostgreSQL、Qdrant 或 Ark，可直接运行：

```bash
GOTOOLCHAIN=local CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go run ./examples/go/memory
```

OpenAPI 源文件为 `api/openapi.yaml`，服务内置文档入口为 `GET /docs`。

## 配置与模型策略

`.env.example` 是本地配置模板，常用变量包括：

| 分类 | 变量 |
| --- | --- |
| 服务入口 | `HOST`、`PORT`、`PUBLIC_BASE_URL` |
| 存储后端 | `STORAGE_BACKEND`、`DATABASE_URL` |
| Qdrant | `QDRANT_HOST`、`QDRANT_GRPC_PORT`、`QDRANT_COLLECTION`、`QDRANT_SEMANTIC_CACHE_COLLECTION`、`QDRANT_AUTO_CREATE_COLLECTIONS` |
| 认证 | `JWT_SECRET`、`ADMIN_DEFAULT_USERNAME`、`ADMIN_DEFAULT_PASSWORD`、`AUTH_TOKEN_TTL` |
| Ark/豆包 | `ARK_API_KEY`、`ARK_BASE_URL`、`ARK_CHAT_MODEL`、`ARK_EMBEDDING_MODEL`、`ARK_MULTIMODAL_MODEL` |
| Rerank | `RERANK_PROVIDER`、`ARK_RERANK_BASE_URL`、`ARK_RERANK_MODEL`、`ALIYUN_RERANK_API_KEY`、`ALIYUN_RERANK_BASE_URL`、`ALIYUN_RERANK_MODEL` |
| 入库解析 | `INGEST_PARSER_METHOD`、`MINERU_APISERVER`、`MINERU_SERVER_URL`、`MINERU_BACKEND`、`MINERU_PARSE_METHOD`、`MINERU_LANG`、`MINERU_FORMULA_ENABLE`、`MINERU_TABLE_ENABLE`、`DOCLING_SERVER_URL`、`DOCLING_TIMEOUT` |

默认 `.env.example` 中 `ARK_API_KEY` 为空、`REQUIRE_EXTERNAL_PROVIDERS=false`。此时本地测试和 API smoke 会走 deterministic mock；`/readyz` 只报告 Ark 状态为 `mock` 或 `configured`，不会主动调用外部 Ark 服务。

需要真实模型调用时，在 `.env` 或运行环境中配置 `ARK_API_KEY` 和对应模型变量。默认 embedding 模型为火山 `doubao-embedding-vision-251215`，真实 Ark smoke test 还需要显式设置 `LIVE_ARK_TESTS=1`，否则默认跳过。

`INGEST_PARSER_METHOD=basic` 是默认解析方法：文本、HTML 和 Office ZIP 文档在本进程内抽取文本，PDF、图片和 DOCX 内嵌图片会通过 `ARK_MULTIMODAL_MODEL` 生成 Markdown 描述。`INGEST_PARSER_METHOD=mineru` 会调用兼容 MinerU `/file_parse` 的远程服务，当前用于 PDF；`INGEST_PARSER_METHOD=docling` 会调用 Docling Serve 的 `/v1/convert/source` 或 `/v1alpha/convert/source`，用于 PDF/DOCX。选择远程方法时必须配置对应的 `MINERU_APISERVER` 或 `DOCLING_SERVER_URL`。

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

真实 Ark smoke test 默认跳过，只在显式开启时运行：

```bash
LIVE_ARK_TESTS=1 ARK_API_KEY="$ARK_API_KEY" CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./tests/live -v
```

## 文档中心

| 文档 | 适合读者 | 内容 |
| --- | --- | --- |
| [docs/README.md](./docs/README.md) | 第一次进入项目的开发者 | 文档地图、推荐阅读路径和维护规则。 |
| [docs/getting-started/](./docs/getting-started/) | 新开发者、API smoke 使用者 | 本地启动、依赖说明、API smoke 和状态目录。 |
| [docs/api/](./docs/api/) | API 调用方、SDK/前端开发者 | 认证、错误模型、知识库、入库、查询和 SSE。 |
| [docs/architecture/](./docs/architecture/) | 后端开发者、架构评审者 | 模块地图、运行时依赖和 RAG pipeline。 |
| [docs/evaluation/](./docs/evaluation/) | 评估/算法/质量负责人 | 数据集结构、rule-based metrics、optimizer 和 LLM-as-Judge 增强边界。 |
| [docs/operations/](./docs/operations/) | 运维、SRE、部署负责人 | 部署依赖、健康检查、metrics、配置安全和故障排查。 |

## 当前边界

- 默认不启动 ES/Neo4j；当前真实后端是 PostgreSQL + Qdrant。
- 当前评估指标是 deterministic rule-based metrics，不等价于完整 LLM-as-Judge。
- `/readyz` 不主动调用 Ark 外部服务，`ark=configured` 只表示 key 已注入。
- `STORAGE_BACKEND=memory` 只用于本地调试和单测，不作为生产配置。
- MinerU 和 Docling 作为远程解析服务接入，ORAG 不在 API 进程内启动它们的 Python runtime。
