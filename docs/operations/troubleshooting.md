# 故障排查

本文提供 ORAG 本地开发和部署时的常见故障定位路径。

## 快速定位顺序

```text
process -> healthz -> readyz -> auth -> ingestion -> retrieval -> generation -> metrics/logs/trace
```

建议先确认 API 是否启动，再确认依赖是否就绪，最后进入业务链路。

## API 进程不可用

检查：

```bash
curl -fsS http://localhost:8080/healthz
```

常见原因：

| 现象 | 检查 |
| --- | --- |
| 连接失败 | `make run` 是否仍在运行，`PORT` 是否被改动。 |
| 端口占用 | `lsof -i :8080`。 |
| Docker 中不可访问 | Compose 是否暴露端口，反向代理是否转发到 `PORT`。 |

## `/readyz` 未就绪

检查：

```bash
curl -fsS http://localhost:8080/readyz
```

常见原因：

| 检查项 | 可能原因 | 处理 |
| --- | --- | --- |
| `postgres` | `DATABASE_URL` 错误、数据库未启动、账号密码不匹配。 | 检查 `make dev-up`、`docker compose ps` 和数据库连接串。 |
| `qdrant` | `QDRANT_HOST`、`QDRANT_GRPC_PORT` 或 TLS/API key 配置错误。 | 确认后端使用 gRPC 端口 `6334`，不是 REST 端口 `6333`。 |
| collection 缺失 | 主 collection 或 semantic cache collection 不存在。 | 开启 `QDRANT_AUTO_CREATE_COLLECTIONS=true` 或手动创建 collection，并保持 vector size 等于 `ARK_EMBEDDING_DIMENSIONS`、distance 为 cosine。 |
| `vector config mismatch` | 旧 Qdrant volume、手动创建的 collection，或 embedding provider/model 变更导致 collection vector size/distance 与当前配置不兼容。 | 对齐 `ARK_EMBEDDING_DIMENSIONS`/embedding provider 与现有数据；如需切换维度或模型，先备份数据，再迁移或重建受影响的 Qdrant collection/volume。 |
| `model_provider=mock` | 显式启用了 deterministic mock provider。 | 只用于测试或本地无外部模型调试；真实模型验证和生产必须配置所选 provider 的 key。 |
| provider base URL 缺失 | 选择了 Azure OpenAI 或 Google Cloud，但没有配置 `AZURE_OPENAI_BASE_URL` 或 `GOOGLE_CLOUD_BASE_URL`。 | 补齐对应 base URL；其它 provider 如走代理或私有网关，检查 `<PROVIDER>_BASE_URL`。 |

注意：`/readyz` 会验证 Qdrant 主 collection 和 semantic cache collection 的 vector 配置，但不验证数据库迁移是否完整。如果接口报 SQL 表不存在，应执行 `make migrate`。

## 登录失败

| 错误 | 可能原因 | 处理 |
| --- | --- | --- |
| `invalid_credentials` | 用户名或密码错误。 | 检查 `ADMIN_DEFAULT_USERNAME`、`ADMIN_DEFAULT_PASSWORD` 和脚本覆盖变量。 |
| `missing_bearer_token` | 未带 token。 | 重新运行 [`examples/curl/00_login.sh`](../../examples/curl/00_login.sh)。 |
| `invalid_bearer_token` | token 过期或 `JWT_SECRET` 已轮换。 | 重新登录并更新 `.orag-demo/token`。 |

## 入库失败

| 现象 | 可能原因 | 处理 |
| --- | --- | --- |
| `payload_too_large` | 文档内容超过 `INGEST_MAX_DOCUMENT_BYTES`。 | 减小输入或调大配置。 |
| `knowledge_base_not_found` | 知识库 ID 不存在或不属于当前 tenant。 | 重新运行建库脚本。 |
| job 失败 | parser、chunker、embedding、store 任一阶段失败。 | 使用 `GET /v1/ingestion-jobs/{id}` 查看 job 结果。 |

## 查询失败或无上下文

| 现象 | 优先检查 |
| --- | --- |
| 查询 500 | 从错误响应或 SSE `error` 事件取 `trace_id`，再检查模型接口、rerank 接口、Qdrant 或 PostgreSQL 访问是否失败。 |
| 答案无引用 | 文档是否成功入库，Qdrant 主 collection 是否有向量，PostgreSQL FTS 是否有 chunk。 |
| 结果不稳定 | profile、top_k、rerank provider、provider base URL 和 mock/真实 provider 配置是否一致。 |
| 延迟过高 | top-k 候选规模、rerank 超时、模型 provider timeout 和上下文 token 预算。 |

已知 `trace_id` 时先查询持久化 RAG trace：

```bash
TOKEN="$(cat .orag-demo/token)"
curl -fsS "http://localhost:8080/v1/traces/${TRACE_ID}" \
  -H "Authorization: Bearer ${TOKEN}"
```

或直接读取本地 PostgreSQL：

```bash
oragctl trace --trace-id trace_xxx
```

排查顺序：

| 步骤 | 操作 | 目的 |
| --- | --- | --- |
| 1 | 在 HTTP 日志中搜索 `trace_id`。 | 确认请求 `route`、`status`、`latency` 和 `error_code`。 |
| 2 | 调用 `GET /v1/traces/{trace_id}` 或运行 `oragctl trace --trace-id <trace_id>`。 | 查看 `profile`、RAG 总耗时和 `node_spans`。 |
| 3 | 检查 `node_spans[].error`。 | 判断失败发生在检索、重排、打包、生成还是引用阶段。 |
| 4 | 对照 `node_spans[].latency_ms` 和 metrics histogram。 | 判断是单次异常还是整体延迟升高。 |
| 5 | 回到依赖日志或配置。 | 针对 Qdrant、PostgreSQL、模型 provider、rerank provider 或 token/tenant 问题处理。 |

## Metrics 没变化

| 指标 | 说明 |
| --- | --- |
| `orag_up` | 只表示 metrics endpoint 可渲染，不代表依赖健康。 |
| `orag_http_requests_total{method,route,status,status_class}` | 经过 HTTP middleware 的请求才增长；无 label 总量用于兼容。 |
| `orag_http_errors_total{method,route,status,status_class}` | 只有 4xx/5xx 响应增长。 |
| `orag_http_request_latency_ms_bucket` | HTTP 请求延迟分桶，可按 route/status_class 聚合。 |
| `orag_rag_queries_total{profile,cache_status,outcome}` | 只有发生 RAG 查询后才增长；失败时 `outcome="error"`。 |
| `orag_rag_errors_total{profile,error_code}` | 只有 RAG 查询失败后增长。 |
| `orag_rag_query_latency_ms_bucket` | 只有 RAG 查询后按延迟分桶增长。 |
| `orag_dependency_checks_total{dependency,status}` | `/readyz` 被调用后增长，依赖失败时 status 为 error/timeout。 |
| `orag_trace_store_total{outcome}` | RAG trace 持久化尝试次数，error 表示 trace 证据退化。 |
| cache hit/miss | 只有查询链路经过语义缓存后才增长。 |

进程重启后，当前 in-process counter 会从零开始。

metrics 不包含 `trace_id`、tenant、用户输入、prompt、文档内容或模型响应等高基数字段。单次请求排查请使用结构化日志中的 `trace_id`、HTTP trace API 或 `oragctl trace`。

## 告警排查

| Alert | 影响 | 只读排查 | 恢复建议 |
| --- | --- | --- | --- |
| `ORAGMetricsMissing` | Prometheus 无法抓取 ORAG 指标。 | `curl -fsS http://localhost:8080/metrics`，检查服务和网络。 | 恢复 API 进程或修正 scrape target。 |
| `ORAGAPIHigh5xxRate` | API 服务端错误比例升高。 | 查看 `orag_http_errors_total`、HTTP 日志和 `trace_id`。 | 根据错误码定位依赖或代码路径，先生成 diagnosis，再考虑 dry-run plan。 |
| `ORAGRAGHighErrorRate` | RAG 查询失败比例升高。 | 查询失败响应的 `trace_id`，调用 `GET /v1/traces/{trace_id}` 或 `oragctl trace`。 | 检查模型 provider、Qdrant、PostgreSQL 和 rerank 配置。 |
| `ORAGRAGHighLatencyP95` | 查询延迟整体升高。 | 调用 `GET /v1/traces:stats` 查看慢 node，结合 `orag_rag_query_latency_ms_bucket`。 | 降低 top_k、检查 rerank/model timeout、确认 Qdrant/PostgreSQL 负载。 |
| `ORAGTraceStoreFailures` | trace 证据可能缺失，影响诊断。 | 查看 `orag_trace_store_total{outcome="error"}` 和 API 日志。 | 检查 PostgreSQL 连接、trace 表迁移和写入权限。 |
| `ORAGDependencyCheckFailing` | `/readyz` 依赖失败，服务可能不能处理查询。 | `curl -fsS http://localhost:8080/readyz`，查看 postgres/qdrant/model_provider。 | 先恢复依赖，再运行 `orag_check(scope=storage)` 验证。 |

告警处理默认只读：先运行 self-check 和 diagnose，再查看 runbook；任何 self-ops apply 都必须先展示 dry-run plan，并由用户明确授权。

## Agent artifact drift

| 现象 | 优先检查 | 处理 |
| --- | --- | --- |
| `make agent-sync-check` 失败 | capability manifest、生成模板、`.mcp/tools/*.json`、`.mcp/openapi-facet.json`、`.codex/skills`、`.claude/skills`、`.trae/skills` 是否不一致。 | 运行 `make agent-sync`，审核生成 diff，再运行 `make agent-sync-check`。 |
| `orag_check(scope=agent_sync)` 通过但 CI 失败 | runtime probe 只表示当前运行时便利检查结果，CI static gate 仍是权威。 | 以 CI 中的 `make agent-sync-check` 输出为准，不用 runtime probe 覆盖 CI 结论。 |
| `tools/list` 缺少 `orag_check` 或诊断/自运维工具 | 运行目录不在仓库根目录，或 `.mcp/tools/orag-self-*.json` 未生成。 | 从仓库根目录运行 `make agent-sync-check`，必要时运行 `make agent-sync`。 |
| Skill 触发边界不清晰 | 生成的 `orag-self-check`、`orag-self-diagnose`、`orag-self-ops` 不是最新。 | 检查 `examples/skills/self-check-diagnose-ops.md`，重新生成并审核 Skill diff。 |

## Self-ops apply 被阻断

| 现象 | 可能原因 | 处理 |
| --- | --- | --- |
| `verdict=blocked` 且提示 precondition drift | 计划生成后 manifest、生成产物、配置或工作区状态变化。 | 丢弃旧计划，重新运行 `orag_maintenance_plan`。 |
| `verdict=blocked` 且提示 approval missing | apply 请求没有明确授权。 | 先展示 dry-run plan、snapshot hash、rollback 和 verification commands，再由用户明确批准。 |
| `verdict=blocked` 且提示 lock conflict | 同一 scope 的 single-flight apply 正在运行。 | 等待已有 apply 结束，再用新的状态重新计划。 |
| 重复请求返回已完成结果 | idempotency key 命中，系统阻止重复写入。 | 使用原始结果作为审计证据，不要生成新的 apply 请求绕过幂等保护。 |

## Docker 网络问题

| 场景 | 建议 |
| --- | --- |
| 容器内连不上 PostgreSQL | `DATABASE_URL` 使用 `postgres:5432`，不要用宿主机视角的 `localhost:5432`。 |
| 容器内连不上 Qdrant | `QDRANT_HOST=qdrant`，`QDRANT_GRPC_PORT=6334`。 |
| 镜像拉取超时 | 检查 Docker Desktop 代理、公司网络策略和 registry mirror。 |
| `go mod download` 超时 | 配置可用 Go proxy 或 CI 网络代理。 |
