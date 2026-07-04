<div align="center">

# ORAG

**A Go-native RAG service framework for ingestion, hybrid retrieval, generation, evaluation, and optimization.**

<p>
  <a href="./README.md"><img alt="README in Chinese" src="https://img.shields.io/badge/简体中文-DBEDFA"></a>
  <a href="./README_EN.md"><img alt="README in English" src="https://img.shields.io/badge/English-F5F5F5"></a>
  <a href="./LICENSE"><img alt="License" src="https://img.shields.io/github/license/shikanon/orag?color=4e6b99"></a>
  <a href="https://github.com/shikanon/orag/actions/workflows/ci.yml"><img alt="CI" src="https://img.shields.io/github/actions/workflow/status/shikanon/orag/ci.yml?branch=main&label=CI"></a>
  <a href="./go.mod"><img alt="Go Version" src="https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white"></a>
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

ORAG 是一个 Go-native RAG service framework，用于构建知识库入库、混合检索、问答生成、评估回归和参数优化的完整闭环。它基于 Hertz 暴露 HTTP API，使用 Eino Graph 编排 RAG 链路，并默认以 PostgreSQL + Qdrant 作为真实运行时依赖。

> ORAG 默认要求真实模型 provider API Key。推荐使用火山引擎方舟/豆包，启动前至少配置 `ARK_API_KEY` 或 `VOLCENGINE_API_KEY`。Deterministic mock 仅用于显式测试模式，避免生产环境误用 mock 结果。

## 项目亮点

- **Go-native service stack**: 使用 Go、Hertz、Eino Graph、PostgreSQL 和 Qdrant 构建可嵌入现有后端体系的 RAG 服务。
- **Production-like defaults**: 默认 `qdrant_postgres` 后端，Qdrant 负责 dense retrieval 与 semantic cache，PostgreSQL 负责元数据、FTS sparse retrieval、trace 和评估结果。
- **Clear API contract**: OpenAPI、curl 示例、Go HTTP client 示例、契约测试和内置 `/docs` 保持一致。
- **Evaluation-first workflow**: 数据集、评估运行、profile/top-k optimizer 复用线上 RAG 查询路径，降低线上线下漂移。
- **Explicit provider boundary**: 内置 provider registry，按 chat、embedding、rerank、多模态能力选择厂商；真实 provider 默认校验 API key。

## 目录

- [核心能力](#核心能力)
- [架构概览](#架构概览)
- [快速开始](#快速开始)
- [示例](#示例)
- [配置](#配置)
- [验证](#验证)
- [文档](#文档)
- [项目边界](#项目边界)
- [许可证](#许可证)

## 核心能力

| 能力 | 说明 | API / 入口 |
| --- | --- | --- |
| Authentication | 管理员登录换取 Bearer token，业务请求从 token 获取默认 tenant。 | `POST /v1/auth/login` |
| Knowledge bases | 创建、列表、详情、删除知识库；删除会清理文档、chunks、向量索引和语义缓存。 | `/v1/knowledge-bases` |
| Document ingestion | 支持 JSON 文本导入和 multipart 文件上传，记录 ingestion job；支持 basic、MinerU、Docling 解析。 | `/documents:import`、`/documents`、`/ingestion-jobs/{id}` |
| Hybrid retrieval | Qdrant dense retrieval、PostgreSQL FTS sparse retrieval、RRF 融合和 rerank。 | `internal/kb`、`internal/rag` |
| RAG query | 支持 JSON 查询和 SSE 流式查询，返回答案、引用、trace、cache 状态和 warnings。 | `POST /v1/query`、`POST /v1/query:stream` |
| Evaluation | 数据集、评估运行、rule-based metrics、评估结果持久化。 | `/v1/datasets`、`/v1/evaluations` |
| Optimization | profile/top-k 网格搜索和优化运行状态管理。 | `/v1/optimizations` |
| Observability | 存活检查、就绪检查、Prometheus 文本指标、结构化日志和 trace。 | `/healthz`、`/readyz`、`/metrics` |

## 架构概览

```text
Client / curl / Go examples / SDK
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
        +--> Ark / Doubao chat, embedding, multimodal adapters
        |
        v
Answer + Citations + Trace + Metrics
```

| 层级 | 默认实现 | 说明 |
| --- | --- | --- |
| HTTP API | Hertz | API 服务入口在 `cmd/orag-api`，契约源文件为 `api/openapi.yaml`。 |
| RAG orchestration | Eino Graph | 编排检索、重排、生成、引用和 cache 链路。 |
| Vector search | Qdrant | 默认 collection 为 `orag_chunks`。 |
| Semantic cache | Qdrant | 默认 collection 为 `orag_semantic_cache`，按 tenant/profile/query 等维度隔离。 |
| Metadata & sparse retrieval | PostgreSQL | 存储知识库、文档、chunk 元数据、FTS、数据集、评估结果和 trace。 |
| Model providers | Ark / Doubao by default | 通过 provider registry 接入 Chat、Embedding、Rerank 和多模态解析。 |
| Local runtime | Docker Compose | `make dev-up` 启动 PostgreSQL 和 Qdrant，不启动 ES/Neo4j。 |

默认 `STORAGE_BACKEND=qdrant_postgres` 依赖 PostgreSQL 和 Qdrant。`STORAGE_BACKEND=memory` 仅适合本地无依赖调试、单测或排查 HTTP 层问题，不作为生产配置。

## 快速开始

### 环境要求

- Go `1.26+`
- Docker Desktop
- `docker compose`
- 一个真实模型 provider API key，推荐 `ARK_API_KEY` 或 `VOLCENGINE_API_KEY`

### 本地启动

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

### 本地 mock 模式

如只做本地 HTTP/单元测试，可显式启用 deterministic mock：

```bash
ALLOW_DETERMINISTIC_MOCK=true \
LLM_CHAT_PROVIDER=mock \
LLM_EMBEDDING_PROVIDER=mock \
LLM_RERANK_PROVIDER=mock \
LLM_MULTIMODAL_PROVIDER=mock \
STORAGE_BACKEND=memory \
make run
```

### 停止服务

```bash
make dev-down
```

## 示例

### Curl smoke 流程

服务启动后按顺序执行：

```bash
examples/curl/00_login.sh
examples/curl/10_create_kb.sh
examples/curl/20_upload_doc.sh
examples/curl/30_query.sh
examples/curl/40_eval.sh
```

脚本默认请求 `BASE_URL=http://localhost:8080`，可通过 `BASE_URL` 覆盖。运行状态保存在 `.orag-demo/`，包括 token、知识库 ID、文档 ID、数据集 ID 和入库 job ID；该目录不应提交。

### Go client 示例

如果调用方是其他 Go 服务或 Go 库，可参考 `examples/go/basic` 以 SDK 风格封装 ORAG API。示例只使用标准库 HTTP client，不依赖仓库内的 `internal/` 包：

```bash
go run ./examples/go/basic
```

OpenAPI 源文件为 `api/openapi.yaml`，服务内置文档入口为 `GET /docs`。

## 配置

`.env.example` 是本地配置模板，常用变量包括：

| 分类 | 变量 |
| --- | --- |
| Server | `HOST`、`PORT`、`PUBLIC_BASE_URL` |
| Storage | `STORAGE_BACKEND`、`DATABASE_URL` |
| Qdrant | `QDRANT_HOST`、`QDRANT_GRPC_PORT`、`QDRANT_COLLECTION`、`QDRANT_SEMANTIC_CACHE_COLLECTION`、`QDRANT_AUTO_CREATE_COLLECTIONS` |
| Auth | `JWT_SECRET`、`ADMIN_DEFAULT_USERNAME`、`ADMIN_DEFAULT_PASSWORD`、`AUTH_TOKEN_TTL` |
| Model providers | `LLM_CHAT_PROVIDER`、`LLM_EMBEDDING_PROVIDER`、`LLM_RERANK_PROVIDER`、`LLM_MULTIMODAL_PROVIDER`、`ALLOW_DETERMINISTIC_MOCK` |
| Ark / Doubao | `ARK_API_KEY`、`VOLCENGINE_API_KEY`、`ARK_BASE_URL`、`ARK_CHAT_MODEL`、`ARK_EMBEDDING_MODEL`、`ARK_MULTIMODAL_MODEL` |
| Rerank | `LLM_RERANK_PROVIDER`、`RERANK_PROVIDER`、`ARK_RERANK_BASE_URL`、`ARK_RERANK_MODEL`、`ALIYUN_RERANK_API_KEY`、`ALIYUN_RERANK_BASE_URL`、`ALIYUN_RERANK_MODEL` |
| Other provider keys | `OPENAI_API_KEY`、`AZURE_OPENAI_API_KEY`、`ANTHROPIC_API_KEY`、`GEMINI_API_KEY`、`COHERE_API_KEY`、`JINA_API_KEY`、`VOYAGE_API_KEY` 等 |
| Provider base URLs | `AZURE_OPENAI_BASE_URL`、`GOOGLE_CLOUD_BASE_URL`，以及可选的 `<PROVIDER>_BASE_URL` 覆盖 |
| RAG routing | `RAG_QUERY_ROUTER_ENABLED`、`RAG_QUERY_ROUTER_STRATEGY`、`RAG_QUERY_ROUTER_DIRECT_MAX_RUNES`、`RAG_QUERY_ROUTER_COMPLEX_MIN_SIGNALS` |
| Graph retrieval | `RAG_GRAPH_RETRIEVAL_ENABLED`、`RAG_GRAPH_RETRIEVAL_TOP_K`、`INGEST_GRAPH_MAX_ENTITIES_PER_CHUNK` |
| Ingestion parser | `INGEST_PARSER_METHOD`、`INGEST_CONTEXTUAL_RETRIEVAL_ENABLED`、`INGEST_CONTEXTUAL_FAILURE_MODE`、`INGEST_RAPTOR_ENABLED`、`INGEST_RAPTOR_BRANCH_FACTOR`、`INGEST_RAPTOR_MAX_LEVELS`、`MINERU_APISERVER`、`MINERU_SERVER_URL`、`MINERU_BACKEND`、`MINERU_PARSE_METHOD`、`MINERU_LANG`、`MINERU_FORMULA_ENABLE`、`MINERU_TABLE_ENABLE`、`DOCLING_SERVER_URL`、`DOCLING_TIMEOUT` |

默认 `.env.example` 中 `REQUIRE_EXTERNAL_PROVIDERS=false`，服务启动仍会校验所选 provider 的 API Key，除非显式启用 deterministic mock provider。`/readyz` 使用 `model_provider=configured` 或显式测试模式下的 `model_provider=mock` 表达模型层状态，不主动调用外部模型服务。

支持的 provider registry 覆盖 OpenAI、Azure OpenAI、Anthropic、Gemini、Google Cloud、xAI、Mistral、Cohere、DeepSeek、Moonshot、MiniMax、BaiChuan、ZHIPU-AI、Tongyi-Qianwen、VolcEngine、Tencent Hunyuan、XunFei Spark、BaiduYiyan、Xiaomi、Perplexity、Voyage AI 和 Jina。

`INGEST_PARSER_METHOD=basic` 是默认解析方法：文本、HTML 和 Office ZIP 文档在本进程内抽取文本，PDF、图片和 DOCX 内嵌图片会通过 `ARK_MULTIMODAL_MODEL` 生成 Markdown 描述。`INGEST_PARSER_METHOD=mineru` 会调用兼容 MinerU `/file_parse` 的远程服务；`INGEST_PARSER_METHOD=docling` 会调用 Docling Serve 的 `/v1/convert/source` 或 `/v1alpha/convert/source`。

`INGEST_CONTEXTUAL_RETRIEVAL_ENABLED=true` 会在 chunk embedding 和 PostgreSQL FTS 索引前，为每个 chunk 生成简短定位上下文，并将 `contextual_text + chunk content` 作为检索表示。默认 `INGEST_CONTEXTUAL_FAILURE_MODE=fallback`，LLM 生成失败时继续使用原始 chunk 入库；生产启用前应评估额外模型调用成本。

`INGEST_RAPTOR_ENABLED=true` 会在入库时生成递归摘要 chunk，摘要带 `raptor_summary` metadata 并与原始 chunk 一起进入 embedding/FTS 检索层。`RAG_QUERY_ROUTER_ENABLED=true` 会按 direct、single retrieval、multi-step retrieval 路由查询；direct 查询绕过检索直接生成，complex 查询会走高精检索扩展。`RAG_GRAPH_RETRIEVAL_ENABLED=true` 会在入库时抽取轻量实体关系，并在检索后按查询实体扩展相关 chunk。

## 验证

常用本地验证命令：

```bash
make fmt
make vet
make test
make openapi-validate
```

`Makefile` 默认为 Go 命令注入 `CGO_ENABLED=0` 和 `GOFLAGS=-tags=stdjson,gjson`，用于规避 Mac amd64 + Go 1.26 下 Hertz/Sonic native 与本地 cgo 链接产物的问题。直接运行原生命令时可显式带上：

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=go1.26.4 go test ./...
```

真实 PostgreSQL + Qdrant 集成测试默认跳过，显式运行：

```bash
make test-integration-up
make test-integration
make test-integration-down
```

真实 Ark smoke test 默认跳过，只在显式开启时运行：

```bash
LIVE_ARK_TESTS=1 ARK_API_KEY="$ARK_API_KEY" CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=go1.26.4 go test ./tests/live -v
```

## 文档

| 文档 | 适合读者 | 内容 |
| --- | --- | --- |
| `docs/README.md` | 第一次进入项目的开发者 | 文档地图、推荐阅读路径和维护规则。 |
| `docs/getting-started/` | 新开发者、API smoke 使用者 | 本地启动、依赖说明、API smoke 和状态目录。 |
| `docs/api/` | API 调用方、SDK/前端开发者 | 认证、错误模型、知识库、入库、查询和 SSE。 |
| `docs/architecture/` | 后端开发者、架构评审者 | 模块地图、运行时依赖和 RAG pipeline。 |
| `docs/evaluation/` | 评估/算法/质量负责人 | 数据集结构、rule-based metrics、LLM-as-Judge/QAG 和目标驱动 optimizer。 |
| `docs/operations/` | 运维、SRE、部署负责人 | 部署依赖、健康检查、metrics、配置安全和故障排查。 |

## 项目边界

- 默认不启动 ES/Neo4j；当前真实后端是 PostgreSQL + Qdrant。
- 评估默认保留 deterministic rule-based metrics；请求提供 `judge`/`qag` 配置时会启用 LLM-as-Judge、QAG 明细和校准相关指标。
- `/readyz` 不主动调用外部模型服务，`model_provider=configured` 只表示所选 provider 的必需 key 已注入。
- `STORAGE_BACKEND=memory` 只用于本地调试和单测，不作为生产配置。
- MinerU 和 Docling 作为远程解析服务接入，ORAG 不在 API 进程内启动它们的 Python runtime。

## 许可证

This project is licensed under the terms of the [LICENSE](./LICENSE) file.
