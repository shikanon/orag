# 运维文档

本目录面向部署、监控和故障排查。完整长文仍保留在 `../operations.md`，本目录提供更结构化的运维入口。

## 运行依赖

| 依赖 | 配置 | 说明 |
| --- | --- | --- |
| PostgreSQL | `DATABASE_URL` | 元数据、FTS、数据集、评估结果和 trace。 |
| Qdrant | `QDRANT_HOST`、`QDRANT_GRPC_PORT` | 主向量 collection 和语义缓存 collection。 |
| Ark/豆包 | `ARK_API_KEY`、`ARK_BASE_URL`、模型变量 | Chat、Embedding、Rerank、多模态解析；无 key 时使用 deterministic mock。 |
| Rerank | `RERANK_PROVIDER` | 可选 `volcengine` 或 `aliyun`。 |
| Observability | `OTEL_EXPORTER_OTLP_ENDPOINT`、`LANGFUSE_*` | 当前为空时不启用外部 exporter 或 LangFuse。 |

## 健康检查

| Endpoint | 用途 | 失败影响 |
| --- | --- | --- |
| `GET /healthz` | 进程存活检查。 | 失败通常表示 API 进程不可用或入口未转发。 |
| `GET /readyz` | 配置和依赖就绪检查。 | 失败表示依赖或 collection 未就绪。 |
| `GET /metrics` | Prometheus 文本指标。 | 失败表示 metrics endpoint 不可用。 |

`/readyz` 当前不会主动调用 Ark 外部服务。`ark=configured` 只表示已配置 `ARK_API_KEY`，不代表 key、模型名、额度或网络出口一定可用。

## Metrics

| 指标 | 类型 | 含义 |
| --- | --- | --- |
| `orag_up` | gauge | metrics endpoint 可渲染时固定为 `1`。 |
| `orag_http_requests_total` | counter | HTTP 请求总数。 |
| `orag_rag_queries_total` | counter | RAG 查询总数。 |
| `orag_rag_cache_hits_total` | counter | 语义缓存命中的 RAG 查询总数。 |
| `orag_rag_cache_misses_total` | counter | 未命中语义缓存的 RAG 查询总数。 |
| `orag_rag_query_latency_ms_sum` | counter | RAG 查询耗时累计值，单位毫秒。 |

当前指标是进程内 counter，服务重启后从零开始；当前没有 label、histogram、分位数、持久化或 OTel exporter。

## 部署检查清单

部署前确认：

- 已替换 `JWT_SECRET`、`ADMIN_DEFAULT_PASSWORD`、数据库密码、模型 key 和对象存储密钥。
- 容器内 `DATABASE_URL` 使用 Compose/Kubernetes 服务名，例如 `postgres`。
- 容器内 `QDRANT_HOST` 使用服务名，例如 `qdrant`。
- 已执行数据库迁移。
- Qdrant 主 collection 和 semantic cache collection 已存在，或 `QDRANT_AUTO_CREATE_COLLECTIONS=true`。
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
| `/readyz` 失败 | `troubleshooting.md` |
| 查询失败或无上下文 | `../architecture/rag-pipeline.md` |
| API smoke 失败 | `../getting-started/api-smoke.md` |
| 镜像拉取或构建超时 | `../operations.md` |
