# 运维文档

本目录面向部署、监控和故障排查。完整长文仍保留在 [`../operations.md`](../operations.md)，本目录提供更结构化的运维入口。

- [Project 资源归属迁移](project-ownership-migration.md)：说明 `000018` 的默认 Project 回填、验证与安全回滚边界。
- [Reference server deployment](server-deployment.md)：在 `orag.tensorbytes.com` 上使用 GHCR 发布镜像部署 API、Console、迁移和依赖。
- [Backup, restore, and disaster recovery](disaster-recovery.md)：PostgreSQL 与 Qdrant 的一致性备份、隔离恢复演练、校验和回滚流程。
- [Grafana dashboard](grafana.md)：导入 ORAG Prometheus 指标概览、关联基础告警并保持只读处置边界。

## 运行依赖

| 依赖 | 配置 | 说明 |
| --- | --- | --- |
| PostgreSQL | `DATABASE_URL` | 元数据、FTS、数据集、评估结果和 trace。 |
| Qdrant | `QDRANT_HOST`、`QDRANT_GRPC_PORT` | 主向量 collection 和语义缓存 collection。 |
| 模型 Provider | `LLM_CHAT_PROVIDER`、`LLM_EMBEDDING_PROVIDER`、`LLM_RERANK_PROVIDER`、`LLM_MULTIMODAL_PROVIDER` | 默认均为 `volcengine`，启动默认要求所选 provider 的 API Key。 |
| Ark/豆包 | `ARK_API_KEY` / `VOLCENGINE_API_KEY`、`ARK_BASE_URL`、模型变量 | 默认推荐 Doubao，提供 Chat、Embedding、Rerank、多模态解析。 |
| Provider Endpoint | `AZURE_OPENAI_BASE_URL`、`GOOGLE_CLOUD_BASE_URL`、可选 `<PROVIDER>_BASE_URL` | Azure OpenAI 和 Google Cloud 必填，其它 provider 可用来覆盖默认 endpoint。 |
| Rerank | `LLM_RERANK_PROVIDER` / `RERANK_PROVIDER` | 默认 `volcengine`，兼容旧的 `aliyun`/通义百炼路径。 |
| Observability | `OTEL_EXPORTER_OTLP_ENDPOINT`、`OTEL_EXPORTER_OTLP_METRICS_ENDPOINT`、`OTEL_TRACES_SAMPLER_ARG`、`OTEL_SERVICE_NAME`、`LANGFUSE_*` | 设置绝对 OTLP/HTTP endpoints 时导出受限 trace 与核心 metrics；trace 使用 W3C 传播和 parent-based sampling；LangFuse 仍未接入。 |

## 健康检查

| Endpoint | 用途 | 失败影响 |
| --- | --- | --- |
| `GET /healthz` | 进程存活检查。 | 失败通常表示 API 进程不可用或入口未转发。 |
| `GET /readyz` | 配置和依赖就绪检查。 | 失败表示依赖、collection 或 Qdrant vector 配置未就绪。 |
| `GET /metrics` | Prometheus 文本指标。 | 失败表示 metrics endpoint 不可用。 |

`/readyz` 会校验 Qdrant 主 collection 和 semantic cache collection 均存在，且使用单 unnamed vector、vector size 等于 `ARK_EMBEDDING_DIMENSIONS`、distance 为 cosine。若复用旧 Qdrant volume 或切换 embedding provider/model 后出现 vector config mismatch，应先确认当前 `ARK_EMBEDDING_DIMENSIONS` 是否要与旧数据对齐；如需切换维度，先备份数据，再迁移或重建受影响的 collection/volume。

`/readyz` 当前不会主动调用外部模型服务。`model_provider=configured` 只表示已配置所选 provider 的必需 key，不代表 key、模型名、额度或网络出口一定可用；`model_provider=mock` 只会在显式 deterministic mock 测试模式下出现。

## Agent 自检与自运维

Agent 能力以 capability manifest 为 SSOT。OpenAPI 只是 HTTP facet；MCP 工具、Skills 触发边界、风险等级、运维语义和生成元数据都从 manifest 生成。不要从 OpenAPI 反推 Skill 的安全边界、调用顺序或失败处理。

| 能力 | 入口 | 运维边界 |
| --- | --- | --- |
| 静态 drift gate | `make agent-sync-check` | CI/发布权威门禁，不依赖运行时 MCP Server。 |
| Runtime probe | `orag_check(scope=agent_sync, mode=focused)` | 便利探针，返回 stable check ID、evidence、trace 和 gate warning；不能替代 CI gate。 |
| 诊断 | `orag_diagnose`、`orag_trace_lookup`、`orag_runbook_suggest` | 只读，根据症状、trace、日志或失败命令给出 findings、recommended actions 和 verification commands。 |
| Dry-run plan | `orag_maintenance_plan` | 只生成计划，包含 snapshot hash、preconditions、idempotency key、lock key、rollback 和 verification commands。 |
| 低风险 apply | `orag_apply_low_risk_action` | 仅在明确授权后执行；apply 前复验 snapshot/preconditions，漂移时返回 `verdict=blocked`。 |

常用本地检查：

```bash
GOTOOLCHAIN=go1.26.5 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson make agent-sync-check
GOTOOLCHAIN=go1.26.5 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson make agent-artifact-tests
GOTOOLCHAIN=go1.26.5 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson make mcp-self-check-smoke
```

自运维 apply 使用 TOCTOU 防护：计划生成时记录 snapshot hash 和 preconditions，执行前重新捕获状态；idempotency key 防止重复执行，single-flight lock 防止并发执行。任何漂移、缺少授权或锁冲突都应阻断写操作并要求重新生成计划。

## Metrics

| 指标 | 类型 | 关键 label | 含义 |
| --- | --- | --- | --- |
| `orag_up` | gauge | 无 | metrics endpoint 可渲染时固定为 `1`。 |
| `orag_http_requests_total` | counter | `method`、`route`、`status`、`status_class` | HTTP 请求总数；同时保留无 label 兼容总量。 |
| `orag_http_errors_total` | counter | `method`、`route`、`status`、`status_class` | HTTP 4xx/5xx 错误响应总数。 |
| `orag_http_request_latency_ms` | histogram | `method`、`route`、`status_class`、`le` | HTTP 请求耗时分桶，单位毫秒。 |
| `orag_rag_queries_total` | counter | `profile`、`cache_status`、`outcome` | RAG 查询总数；同时保留无 label 兼容总量。 |
| `orag_rag_errors_total` | counter | `profile`、`error_code` | RAG 查询失败总数。 |
| `orag_rag_cache_hits_total` | counter | 无 | 语义缓存命中的 RAG 查询总数。 |
| `orag_rag_cache_misses_total` | counter | 无 | 未命中语义缓存的 RAG 查询总数。 |
| `orag_rag_query_latency_ms` | histogram | `profile`、`cache_status`、`outcome`、`le` | RAG 查询耗时分桶，单位毫秒。 |
| `orag_rag_query_latency_ms_sum` | counter | 无或 `profile`、`cache_status`、`outcome` | RAG 查询耗时累计值，单位毫秒。 |
| `orag_dependency_checks_total` | counter | `dependency`、`status` | `/readyz` 依赖检查结果，dependency 会归一化到 postgres/qdrant/model_provider/other。 |
| `orag_dependency_check_latency_ms` | histogram | `dependency`、`status`、`le` | 依赖检查耗时分桶。 |
| `orag_trace_store_total` | counter | `outcome` | trace 持久化尝试次数，outcome 为 success/error。 |
| `orag_trace_store_latency_ms` | histogram | `outcome`、`le` | trace 持久化耗时分桶。 |

metrics label 只使用受控低基数字段。不要把 `trace_id`、tenant、用户输入、prompt、文档内容、模型响应或原始错误文本作为 Prometheus label；排查单次请求应使用日志和 trace 查询。

当前指标是进程内 counter/histogram，服务重启后从零开始；仓库提供可导入的 [Grafana overview dashboard](grafana.md) 和基础 Prometheus 告警规则。设置 `OTEL_EXPORTER_OTLP_METRICS_ENDPOINT` 为绝对 OTLP/HTTP metrics endpoint 后，会导出核心 HTTP、RAG、依赖就绪和 trace-store metrics；当前仍没有分位数预聚合或指标持久化。

## 告警接入

仓库提供最小 Prometheus/Alertmanager 示例：

```bash
promtool check rules deployments/prometheus/alerts.yml
```

示例规则覆盖：

- `ORAGMetricsMissing`：`orag_up` 缺失。
- `ORAGAPIHigh5xxRate`：API 5xx 比例超过 5%。
- `ORAGRAGHighErrorRate`：RAG error rate 超过 5%。
- `ORAGRAGHighLatencyP95`：RAG p95 超过 5s。
- `ORAGTraceStoreFailures`：trace store 失败。
- `ORAGDependencyCheckFailing`：依赖检查持续失败。

`deployments/alertmanager/alertmanager.example.yml` 的 webhook 指向 `http://orag-api:8080/v1/ops/alerts`，当前仅作为集成示例；运行时不会自动执行修复动作。推荐处理路径是：告警 -> `orag_check` -> `orag_diagnose` -> `orag_runbook_suggest` -> `orag_maintenance_plan(dry_run=true)` -> 人工授权低风险动作。

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
GET /v1/traces:stats?limit=20
GET /v1/traces/{trace_id}
Authorization: Bearer <access_token>
```

CLI 支持本地 PostgreSQL 单条、列表和统计查询。HTTP `GET /v1/traces:stats` 返回当前 tenant 的 node 级 count、avg、p95、p99 和 error_count。当前仍不提供跨租户聚合或外部 APM 跳转。

Trace query 存储默认会做基础治理：保存前会截断到 2048 bytes，并对常见 `authorization: bearer`、`api_key`、`token` 片段做 `[redacted]` 替换。部署侧可通过 `TRACE_STORE_QUERY`、`TRACE_QUERY_MAX_BYTES`、`TRACE_RETENTION_DAYS` 表达策略；历史数据清理需要单独 job 或数据库保留策略。

## OTel 与 LangFuse 边界

| 能力 | 当前状态 | 后续可选增强 |
| --- | --- | --- |
| OpenTelemetry | `OTEL_EXPORTER_OTLP_ENDPOINT` 和 `OTEL_EXPORTER_OTLP_METRICS_ENDPOINT` 分别接受绝对 OTLP/HTTP traces/metrics endpoint（如 `http://otel-collector:4318/v1/traces` 与 `/v1/metrics`）。HTTP 接受并返回 W3C `traceparent`；`OTEL_TRACES_SAMPLER_ARG` 是 `[0,1]` 的根 trace 采样比例（默认 `1`），已采样/未采样远端父级均按 parent-based 语义继承，`OTEL_SERVICE_NAME` 默认 `orag`。trace 只包含受限 span 属性；metrics 导出核心 HTTP、RAG、依赖就绪和 trace-store counters/histograms，并仅使用低基数属性。两者都不导出 query、prompt、文档、模型输出、tenant、trace ID 或原始错误文本。 | 增加指标持久化；在 Collector 端按真实容量配置 retention 与 error/latency tail sampling。 |
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

启动本地无 Key 完整 Compose 栈并执行可重复 walkthrough：

```bash
make demo
```

该开发栈明确启用 deterministic mock，并启动迁移、API、Console 与一次性 demo。结果保存在 `.orag-demo/walkthrough.json`；生产部署必须替换默认管理员凭据、关闭 mock，并提供真实 provider 与密钥。

Compose 默认会覆盖为容器网络地址；如需显式覆盖，使用：

```dotenv
DOCKER_DATABASE_URL=postgres://orag:orag@postgres:5432/orag?sslmode=disable
DOCKER_QDRANT_HOST=qdrant
DOCKER_QDRANT_GRPC_PORT=6334
```

## 排障入口

| 现象 | 继续阅读 |
| --- | --- |
| `/readyz` 失败 | [`troubleshooting.md`](./troubleshooting.md) |
| 查询失败或无上下文 | [`../architecture/rag-pipeline.md`](../architecture/rag-pipeline.md) |
| 已知 `trace_id` 需要定位 RAG 节点 | `oragctl trace --trace-id <trace_id>` |
| Agent artifact drift 或 Skill/MCP 不一致 | `make agent-sync-check`，再参考 [`../api/agent-integrations.md`](../api/agent-integrations.md) |
| API smoke 失败 | [`../getting-started/api-smoke.md`](../getting-started/api-smoke.md) |
| 镜像拉取或构建超时 | [`../operations.md`](../operations.md) |
