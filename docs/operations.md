# 运维指南

## 部署依赖

默认生产形态使用 `STORAGE_BACKEND=qdrant_postgres`，服务启动和查询依赖以下组件：

- PostgreSQL：默认连接串来自 `DATABASE_URL`，示例为 `postgres://orag:orag@localhost:5432/orag?sslmode=disable`。部署后需要执行数据库迁移。
- Qdrant：向量检索使用 gRPC 端口 `6334`，Docker Compose 同时暴露 REST 端口 `6333` 供 Qdrant 自身 healthcheck 使用。默认主 collection 为 `orag_chunks`，语义缓存 collection 为 `orag_semantic_cache`。
- 模型 provider：由 `LLM_CHAT_PROVIDER`、`LLM_EMBEDDING_PROVIDER`、`LLM_RERANK_PROVIDER`、`LLM_MULTIMODAL_PROVIDER` 选择不同厂商；默认均为 `volcengine`。
- Ark/豆包模型接口：由 `ARK_API_KEY` 或 `VOLCENGINE_API_KEY`、`ARK_BASE_URL`、`ARK_CHAT_MODEL`、`ARK_EMBEDDING_MODEL`、`ARK_MULTIMODAL_MODEL`、`ARK_TIMEOUT` 和 `ARK_RETRY_TIMES` 配置。默认推荐火山 Doubao，默认 embedding 模型为 `doubao-embedding-vision-251215`，调用 `/embeddings/multimodal`。
- Provider Endpoint：大多数 provider 内置默认 endpoint；Azure OpenAI 和 Google Cloud 需分别配置 `AZURE_OPENAI_BASE_URL`、`GOOGLE_CLOUD_BASE_URL`。其它 provider 如走代理、私有网关或不同区域，可用 `<PROVIDER>_BASE_URL` 覆盖默认值。
- Rerank 接口：由 `LLM_RERANK_PROVIDER` 或兼容变量 `RERANK_PROVIDER` 选择 provider。火山/方舟使用 `ARK_RERANK_BASE_URL`、`ARK_RERANK_MODEL`；阿里云百炼/通义可使用 `ALIYUN_RERANK_API_KEY`、`ALIYUN_RERANK_BASE_URL`、`ALIYUN_RERANK_MODEL`。未配置所选 provider 的 key 时默认启动失败；deterministic mock 只允许显式测试模式启用。
- 文档解析：默认 `INGEST_PARSER_METHOD=basic` 不依赖额外解析服务；PDF、图片和 DOCX 内嵌图片会通过 Ark 多模态模型生成描述。`INGEST_CONTEXTUAL_RETRIEVAL_ENABLED=true` 会额外为每个 chunk 生成 contextual text，用于 embedding 和 FTS/BM25 表示，默认失败降级为原始 chunk。`INGEST_RAPTOR_ENABLED=true` 会生成递归摘要 chunk，`RAG_GRAPH_RETRIEVAL_ENABLED=true` 会抽取轻量实体关系并在查询时扩展相关 chunk。`mineru` 需要 `MINERU_APISERVER`，可选 `MINERU_SERVER_URL` 用于 VLM HTTP backend；`docling` 需要 `DOCLING_SERVER_URL`。
- 对象存储和观测平台：默认 `OBJECT_STORAGE_PROVIDER=local`、`OBJECT_STORAGE_MOCK_UPLOAD=true`，`OTEL_EXPORTER_OTLP_ENDPOINT`、`LANGFUSE_*` 为空时不启用外部 exporter 或 LangFuse。

系统默认不依赖 ES/Neo4j。`STORAGE_BACKEND=memory` 仅用于本地无依赖调试或单元测试，不作为生产配置。

## 配置安全

真实 `.env` 已被 `.gitignore` 忽略，不应提交。示例变量维护在 `.env.example`，结构化配置示例维护在 `configs/config.example.yaml`。

生产环境建议：

- 替换所有示例弱口令和默认密钥，至少包括 `JWT_SECRET`、`ADMIN_DEFAULT_PASSWORD`、`DATABASE_URL` 中的数据库密码、`ARK_API_KEY` / `VOLCENGINE_API_KEY`、其它模型 provider key、所选 provider 的 base URL、`ALIYUN_RERANK_API_KEY`、`QDRANT_API_KEY`、对象存储密钥和 `LANGFUSE_SECRET_KEY`。
- 通过部署平台的 Secret、KMS 或环境变量注入敏感值，不把真实 `.env` 打进镜像，也不在 CI 日志中打印完整配置。
- 除非已有数据合规和脱敏策略，否则保持 `OBSERVABILITY_RECORD_PROMPTS=false`，避免将用户问题、上下文片段或 prompt 明文写入外部观测系统。
- 生产服务建议显式设置 `DEBUG=false`，并根据入口域名设置 `PUBLIC_BASE_URL`。
- 在容器内运行 `orag-api` 时，`DATABASE_URL` 和 `QDRANT_HOST` 应使用 Compose/Kubernetes 服务名，例如 `postgres`、`qdrant`；在宿主机直接运行时才使用 `localhost` 和映射端口。

## 健康检查

- `GET /healthz`：进程存活。
- `GET /readyz`：配置和依赖就绪状态。
- `GET /metrics`：Prometheus 文本指标入口。

`/readyz` 的检查项与当前实现保持一致：

- `STORAGE_BACKEND=memory`：只返回本地 `storage=ready`。
- `STORAGE_BACKEND=qdrant_postgres`：检查 `postgres` 是否可 ping 通，检查 Qdrant 主 collection `QDRANT_COLLECTION` 和语义缓存 collection `QDRANT_SEMANTIC_CACHE_COLLECTION` 是否存在。
- `model_provider`：只根据配置校验结果报告 `configured` 或显式测试模式下的 `mock`，不主动调用外部模型接口，避免第三方波动影响本地服务就绪。

模型 provider 状态只可能报告为：

- `mock`：显式设置 `ALLOW_DETERMINISTIC_MOCK=true` 且选择 `mock` provider。
- `configured`：已配置所选 provider 的必需 key。

如果 PostgreSQL 不可达、Qdrant 不可达或必需 collection 缺失，`/readyz` 会返回未就绪；但它不会验证数据库迁移是否完整，也不会验证模型 key、模型名、额度或网络出口是否真实可用。

## Metrics

当前内置轻量 in-process Prometheus 文本指标：

| 指标 | 类型 | 含义 |
| --- | --- | --- |
| `orag_up` | gauge | 进程可渲染 metrics 时固定为 `1`，不代表 PostgreSQL、Qdrant 或 Ark 已就绪。 |
| `orag_http_requests_total` | counter | 服务处理过的 HTTP 请求总数；同时输出带 `method`、`route`、`status`、`status_class` 的低基数 label 版本。 |
| `orag_http_errors_total` | counter | HTTP 4xx/5xx 错误响应总数，label 同 HTTP 请求计数。 |
| `orag_rag_queries_total` | counter | RAG 查询总数；同时输出带 `profile`、`cache_status`、`outcome` 的低基数 label 版本。 |
| `orag_rag_errors_total` | counter | RAG 查询失败总数，label 为 `profile` 和受控 `error_code`。 |
| `orag_rag_cache_hits_total` | counter | 语义缓存命中的 RAG 查询总数。 |
| `orag_rag_cache_misses_total` | counter | 未命中语义缓存的 RAG 查询总数；非 `hit` 的 cache 状态都会计为 miss。 |
| `orag_rag_query_latency_ms` | histogram | RAG 查询耗时分桶，单位毫秒，label 为 `profile`、`cache_status`、`outcome`。 |
| `orag_rag_query_latency_ms_sum` | counter | RAG 查询耗时累计值，单位毫秒。可与 `orag_rag_queries_total` 粗略计算平均耗时。 |

这些指标是进程内 counter/histogram，服务重启后会从零开始；当前没有分位数预聚合、持久化或 OTel exporter。metrics label 不包含 `trace_id`、tenant、用户输入、prompt、文档内容、模型响应或原始错误文本，单次请求排查请使用日志和 trace 查询。

## 日志、Trace 与外部观测边界

HTTP 请求完成日志使用 `http_request_completed`，包含 `method`、`route`、`path`、`status`、`latency`、`trace_id`，错误响应会额外包含 `error_code`。RAG/Graph 失败日志会携带 `trace_id`、tenant、profile、node、latency 和 error 字段中的一部分；日志不应输出 token、原始 prompt、文档内容或模型响应。

RAG 查询响应、SSE `trace`/`error` 事件和统一错误响应都会返回 `trace_id`。查询 PostgreSQL 中的持久化 trace：

```bash
oragctl trace --trace-id trace_xxx
```

命中时输出 `trace` 对象，包含 `tenant_id`、`profile`、`latency_ms`、`has_error`、`error_count` 和按时间排序的 `node_spans`；未命中时输出 `found=false` 和查询的 `trace_id`。当前只支持 CLI 按单个 `trace_id` 精确查询，不支持 HTTP trace 查询、列表、时间范围过滤或跨服务拓扑。

`OTEL_EXPORTER_OTLP_ENDPOINT` 和 `LANGFUSE_*` 当前只是配置边界：服务未创建 OTel tracer/provider，不导出 OTel spans 或 metrics；也未创建 LangFuse client，不上传 prompt、completion、score 或 trace。后续如需接入，应先明确脱敏、采样、留存和 `OBSERVABILITY_RECORD_PROMPTS` 策略。

## 本地部署检查

```bash
make dev-up
make migrate
make run
```

服务启动后检查：

```bash
curl -fsS http://localhost:8080/healthz
curl -fsS http://localhost:8080/readyz
curl -fsS http://localhost:8080/metrics
```

## Docker 部署与构建

只启动本地依赖，宿主机运行 API：

```bash
docker compose -f deployments/docker-compose.yml up -d postgres qdrant
make migrate
make run
```

构建 API 镜像：

```bash
docker build -f deployments/Dockerfile -t orag-api:local .
```

使用 Compose 启动完整栈：

```bash
docker compose -f deployments/docker-compose.yml up --build
```

完整栈运行时，`.env` 中应使用容器网络地址：

```dotenv
DATABASE_URL=postgres://orag:orag@postgres:5432/orag?sslmode=disable
QDRANT_HOST=qdrant
QDRANT_GRPC_PORT=6334
```

如果在 Mac 宿主机直接运行 API，则继续使用 `.env.example` 风格的 `localhost`：

```dotenv
DATABASE_URL=postgres://orag:orag@localhost:5432/orag?sslmode=disable
QDRANT_HOST=localhost
QDRANT_GRPC_PORT=6334
```

镜像拉取或构建超时的处理建议：

- 先单独拉取基础镜像和依赖镜像，定位是哪个 registry 超时：`docker pull postgres:16-alpine`、`docker pull qdrant/qdrant:v1.11.5`、`docker pull golang:1.26-alpine`、`docker pull alpine:3.20`。
- 如果出现 `i/o timeout`、`TLS handshake timeout`、`context deadline exceeded`，优先检查 Docker Desktop 代理、公司网络策略和 registry mirror 配置，再重试 `docker compose pull` 或 `docker build`。
- 如果 CI/CD 网络无法稳定访问 Docker Hub，建议在内部镜像仓库缓存 `postgres:16-alpine`、`qdrant/qdrant:v1.11.5`、`golang:1.26-alpine` 和 `alpine:3.20`，并在部署清单中替换为内部镜像地址。
- 如果 `go mod download` 在 Docker build 阶段超时，确认构建环境能访问 Go module proxy，或在 CI 中配置可用的 Go proxy 和网络代理。
- 如果 compose 中 `orag-api` 启动后连接依赖失败，先确认 `.env` 使用的是容器服务名而不是 `localhost`，再检查 `postgres`、`qdrant` healthcheck 是否 healthy。

## 常见故障排查清单

### `/healthz` 失败

- API 进程可能未启动、端口未暴露或入口反向代理未转发到 `PORT`。
- 本地优先检查 `make run` 输出；Docker 优先检查 `docker compose -f deployments/docker-compose.yml ps` 和 `docker compose -f deployments/docker-compose.yml logs orag-api`。

### `/readyz` 返回未就绪

- `postgres` 报错：检查 `DATABASE_URL`、数据库账号密码、网络连通性和 PostgreSQL 服务状态。注意 `/readyz` 只做 ping，不验证迁移完整性；接口出现 SQL 表不存在时再执行 `make migrate` 或对应迁移命令。
- `qdrant` 报错：检查 `QDRANT_HOST`、`QDRANT_GRPC_PORT`、`QDRANT_API_KEY`、`QDRANT_USE_TLS` 和 Qdrant 服务状态。默认 gRPC 端口是 `6334`，不要误填 REST 端口 `6333`。
- `required collection is missing`：检查 `QDRANT_COLLECTION`、`QDRANT_SEMANTIC_CACHE_COLLECTION` 名称是否与环境变量一致；需要自动创建时确认 `QDRANT_AUTO_CREATE_COLLECTIONS=true`，否则手动创建两个 collection，并保持向量维度与 `ARK_EMBEDDING_DIMENSIONS` 一致。
- `model_provider=mock`：说明当前显式启用了 deterministic mock。生产或真实模型验证必须改为真实 provider 并配置对应 key。`/readyz` 不主动调用外部模型接口，因此 `model_provider=configured` 只代表必需 key 已注入，不代表 key、额度或模型名一定可用。

### API 可以启动但查询失败

- 401/403：检查登录 token、`JWT_SECRET` 是否发生轮换、`AUTH_TOKEN_TTL` 是否过期，以及默认管理员密码是否已按部署环境更新。
- 入库或查询返回外部模型错误：检查 `LLM_*_PROVIDER`、对应 provider API key、`ARK_BASE_URL`、`AZURE_OPENAI_BASE_URL`、`GOOGLE_CLOUD_BASE_URL`、其它 `<PROVIDER>_BASE_URL`、`ARK_RERANK_BASE_URL`、`ALIYUN_RERANK_BASE_URL`、模型名、超时和重试配置；用真实外部模型前应确保网络出口、账号额度和模型权限可用。
- 查询结果总是无上下文：检查文档是否成功入库、PostgreSQL 元数据是否存在、Qdrant 主 collection 是否有向量，以及 `RAG_CONTEXT_TOP_N`、`RAG_MAX_CONTEXT_TOKENS` 是否被设置得过低。
- 语义缓存命中异常：查看 `orag_rag_cache_hits_total`、`orag_rag_cache_misses_total` 和 `RAG_SEMANTIC_CACHE_THRESHOLD`；collection 缺失会在 `/readyz` 暴露。
- 已知错误响应或 SSE `error` 事件中的 `trace_id`：先在 HTTP 日志中搜索该值，再运行 `oragctl trace --trace-id <trace_id>` 查看 `node_spans[].error` 和节点耗时。

### Docker Compose 依赖异常

- PostgreSQL unhealthy：检查 `POSTGRES_USER`、`POSTGRES_PASSWORD`、`POSTGRES_DB` 与 `DATABASE_URL` 是否匹配，并查看 `postgres` 容器日志。
- Qdrant unhealthy：检查 REST healthcheck `http://localhost:6333/readyz` 在容器内是否可用，以及宿主机端口是否被占用。
- API 容器连不上依赖：在同一个 Compose 网络内使用 `postgres:5432` 和 `qdrant:6334`，不要使用宿主机视角的 `localhost`。

### Metrics 看不到变化

- `orag_up` 只表示 metrics endpoint 正常渲染，不表示依赖健康；依赖状态以 `/readyz` 为准。
- RAG 相关 counter 只有发生 RAG 查询后才会增长。
- 指标是进程内累计值，容器重启、进程重启或重新部署后会归零。
