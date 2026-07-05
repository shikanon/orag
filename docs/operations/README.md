# 运维文档

本目录面向部署、监控和故障排查。完整长文仍保留在 [`../operations.md`](../operations.md)，本目录提供更结构化的运维入口。

## 运行依赖

| 依赖 | 配置 | 说明 |
| --- | --- | --- |
| PostgreSQL | `DATABASE_URL` | 元数据、FTS、数据集、评估结果和 trace。 |
| Qdrant | `QDRANT_HOST`、`QDRANT_GRPC_PORT` | 主向量 collection 和语义缓存 collection。 |
| 模型 Provider | `LLM_CHAT_PROVIDER`、`LLM_EMBEDDING_PROVIDER`、`LLM_RERANK_PROVIDER`、`LLM_MULTIMODAL_PROVIDER` | 默认均为 `volcengine`，启动默认要求所选 provider 的 API Key。 |
| Ark/豆包 | `ARK_API_KEY` / `VOLCENGINE_API_KEY`、`ARK_BASE_URL`、模型变量 | 默认推荐 Doubao，提供 Chat、Embedding、Rerank、多模态解析。 |
| Provider Endpoint | `AZURE_OPENAI_BASE_URL`、`GOOGLE_CLOUD_BASE_URL`、可选 `<PROVIDER>_BASE_URL` | Azure OpenAI 和 Google Cloud 必填，其它 provider 可用来覆盖默认 endpoint。 |
| Rerank | `LLM_RERANK_PROVIDER` / `RERANK_PROVIDER` | 默认 `volcengine`，兼容旧的 `aliyun`/通义百炼路径。 |
| Observability | `OTEL_EXPORTER_OTLP_ENDPOINT`、`LANGFUSE_*` | 当前为空时不启用外部 exporter 或 LangFuse。 |

## 健康检查

| Endpoint | 用途 | 失败影响 |
| --- | --- | --- |
| `GET /healthz` | 进程存活检查。 | 失败通常表示 API 进程不可用或入口未转发。 |
| `GET /readyz` | 配置和依赖就绪检查。 | 失败表示依赖、collection 或 Qdrant vector 配置未就绪。 |
| `GET /metrics` | Prometheus 文本指标。 | 失败表示 metrics endpoint 不可用。 |

`/readyz` 会校验 Qdrant 主 collection 和 semantic cache collection 均存在，且使用单 unnamed vector、vector size 等于 `ARK_EMBEDDING_DIMENSIONS`、distance 为 cosine。若复用旧 Qdrant volume 或切换 embedding provider/model 后出现 vector config mismatch，应先确认当前 `ARK_EMBEDDING_DIMENSIONS` 是否要与旧数据对齐；如需切换维度，先备份数据，再迁移或重建受影响的 collection/volume。

`/readyz` 当前不会主动调用外部模型服务。`model_provider=configured` 只表示已配置所选 provider 的必需 key，不代表 key、模型名、额度或网络出口一定可用；`model_provider=mock` 只会在显式 deterministic mock 测试模式下出现。

## Metrics

| 指标 | 类型 | 关键 label | 含义 |
| --- | --- | --- | --- |
| `orag_up` | gauge | 无 | metrics endpoint 可渲染时固定为 `1`。 |
| `orag_http_requests_total` | counter | `method`、`route`、`status`、`status_class` | HTTP 请求总数；同时保留无 label 兼容总量。 |
| `orag_http_errors_total` | counter | `method`、`route`、`status`、`status_class` | HTTP 4xx/5xx 错误响应总数。 |
| `orag_rag_queries_total` | counter | `profile`、`cache_status`、`outcome` | RAG 查询总数；同时保留无 label 兼容总量。 |
| `orag_rag_errors_total` | counter | `profile`、`error_code` | RAG 查询失败总数。 |
| `orag_rag_cache_hits_total` | counter | 无 | 语义缓存命中的 RAG 查询总数。 |
| `orag_rag_cache_misses_total` | counter | 无 | 未命中语义缓存的 RAG 查询总数。 |
| `orag_rag_query_latency_ms` | histogram | `profile`、`cache_status`、`outcome`、`le` | RAG 查询耗时分桶，单位毫秒。 |
| `orag_rag_query_latency_ms_sum` | counter | 无或 `profile`、`cache_status`、`outcome` | RAG 查询耗时累计值，单位毫秒。 |

metrics label 只使用受控低基数字段。不要把 `trace_id`、tenant、用户输入、prompt、文档内容、模型响应或原始错误文本作为 Prometheus label；排查单次请求应使用日志和 trace 查询。

当前指标是进程内 counter/histogram，服务重启后从零开始；当前没有分位数预聚合、持久化或 OTel exporter。

## 日志字段

HTTP 请求完成日志统一使用 `http_request_completed`，主要字段：

| 字段 | 含义 |
| --- | --- |
| `method` | HTTP method。 |
| `route` | Hertz route 模板，优先用于聚合。 |
| `path` | 实际请求路径，仅用于定位单次请求。 |
| `status` | HTTP 状态码。 |
| `latency` | 请求耗时，单位毫秒。 |
| `trace_id` | 请求级 trace ID。 |
| `error_code` | 统一错误码，只有错误响应时出现。 |

RAG/Graph 失败日志会携带 `trace_id`、tenant、profile、node、latency 和 error 字段中的一部分。日志不应输出 token、原始 prompt、文档内容或模型响应；如果需要关联业务失败，优先用 `trace_id` 串联 HTTP 日志、RAG 日志和 PostgreSQL trace。

## Trace 查询

RAG 查询成功响应、SSE `trace`/`error` 事件和 JSON 错误响应都会返回 `trace_id`。查询持久化 trace：

```bash
oragctl trace --trace-id trace_xxx
```

输出说明：

| 字段 | 含义 |
| --- | --- |
| `found` | 是否在 PostgreSQL 找到该 trace。 |
| `trace.trace_id` | RAG trace 主记录 ID。 |
| `trace.tenant_id` | token 解析出的租户。 |
| `trace.profile` | 查询使用的 profile。 |
| `trace.latency_ms` | RAG pipeline 总耗时。 |
| `trace.has_error`、`trace.error_count` | node span 是否包含错误及错误数量。 |
| `trace.node_spans` | 按时间排序的节点 span，包含 `node_name`、`latency_ms`、`error` 和 `created_at`。 |

HTTP API 支持按当前 tenant 查询 trace 列表和详情：

```http
GET /v1/traces?limit=20
GET /v1/traces/{trace_id}
Authorization: Bearer <access_token>
```

CLI 支持本地 PostgreSQL 单条、列表和统计查询。当前仍不提供跨租户聚合、采样、跨服务拓扑或外部 APM 跳转。

## OTel 与 LangFuse 边界

| 能力 | 当前状态 | 后续可选增强 |
| --- | --- | --- |
| OpenTelemetry | 只保留 `OTEL_EXPORTER_OTLP_ENDPOINT` 等配置边界，当前未创建 OTel tracer/provider，也不导出 OTel spans 或 metrics。 | 接入 OTLP exporter，把 request trace、RAG trace 和 node span 映射为标准 span。 |
| LangFuse | 只保留 `LANGFUSE_*` 配置边界，当前没有 LangFuse client，也不上传 prompt、completion、score 或 trace。 | 在合规和脱敏策略明确后，将 RAG query、retrieval、rerank、generation 映射到 LangFuse trace/observation。 |
| Prompt 记录 | 生产默认保持 `OBSERVABILITY_RECORD_PROMPTS=false`。 | 仅在明确授权、脱敏和留存策略后开启。 |

## 部署检查清单

部署前确认：

- 已替换 `JWT_SECRET`、`ADMIN_DEFAULT_PASSWORD`、数据库密码、所选模型 provider key、所选 provider 的 base URL 和对象存储密钥。
- 容器内 `DATABASE_URL` 使用 Compose/Kubernetes 服务名，例如 `postgres`。
- 容器内 `QDRANT_HOST` 使用服务名，例如 `qdrant`。
- 已执行数据库迁移。
- Qdrant 主 collection 和 semantic cache collection 已存在，或 `QDRANT_AUTO_CREATE_COLLECTIONS=true`；两者 vector size 与 `ARK_EMBEDDING_DIMENSIONS` 一致，distance 为 cosine。
- 生产默认保持 `OBSERVABILITY_RECORD_PROMPTS=false`，除非已有合规和脱敏策略。

## Docker 本地部署

只启动依赖，宿主机运行 API：

```bash
docker compose -f deployments/docker-compose.yml up -d postgres qdrant
make migrate
make run
```

启动完整 Compose 栈：

```bash
docker compose -f deployments/docker-compose.yml up --build
```

容器内建议配置：

```dotenv
DATABASE_URL=postgres://orag:orag@postgres:5432/orag?sslmode=disable
QDRANT_HOST=qdrant
QDRANT_GRPC_PORT=6334
```

## 排障入口

| 现象 | 继续阅读 |
| --- | --- |
| `/readyz` 失败 | [`troubleshooting.md`](./troubleshooting.md) |
| 查询失败或无上下文 | [`../architecture/rag-pipeline.md`](../architecture/rag-pipeline.md) |
| 已知 `trace_id` 需要定位 RAG 节点 | `oragctl trace --trace-id <trace_id>` |
| API smoke 失败 | [`../getting-started/api-smoke.md`](../getting-started/api-smoke.md) |
| 镜像拉取或构建超时 | [`../operations.md`](../operations.md) |
