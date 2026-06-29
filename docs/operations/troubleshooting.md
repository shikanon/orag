# 故障排查

本文提供 ORAG 本地开发和部署时的常见故障定位路径。

## 快速定位顺序

```text
process -> healthz -> readyz -> auth -> ingestion -> retrieval -> generation -> metrics
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
| collection 缺失 | 主 collection 或 semantic cache collection 不存在。 | 开启 `QDRANT_AUTO_CREATE_COLLECTIONS=true` 或手动创建 collection。 |
| `ark=mock` | 未配置 `ARK_API_KEY`。 | 本地可接受；真实模型验证和生产必须配置 key。 |

注意：`/readyz` 不验证数据库迁移是否完整。如果接口报 SQL 表不存在，应执行 `make migrate`。

## 登录失败

| 错误 | 可能原因 | 处理 |
| --- | --- | --- |
| `invalid_credentials` | 用户名或密码错误。 | 检查 `ADMIN_DEFAULT_USERNAME`、`ADMIN_DEFAULT_PASSWORD` 和脚本覆盖变量。 |
| `missing_bearer_token` | 未带 token。 | 重新运行 `examples/curl/00_login.sh`。 |
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
| 查询 500 | 模型接口、rerank 接口、Qdrant 或 PostgreSQL 访问是否失败。 |
| 答案无引用 | 文档是否成功入库，Qdrant 主 collection 是否有向量，PostgreSQL FTS 是否有 chunk。 |
| 结果不稳定 | profile、top_k、rerank provider 和 mock/真实 Ark 配置是否一致。 |
| 延迟过高 | top-k 候选规模、rerank 超时、Ark timeout 和上下文 token 预算。 |

## Metrics 没变化

| 指标 | 说明 |
| --- | --- |
| `orag_up` | 只表示 metrics endpoint 可渲染，不代表依赖健康。 |
| `orag_rag_queries_total` | 只有发生 RAG 查询后才增长。 |
| cache hit/miss | 只有查询链路经过语义缓存后才增长。 |

进程重启后，当前 in-process counter 会从零开始。

## Docker 网络问题

| 场景 | 建议 |
| --- | --- |
| 容器内连不上 PostgreSQL | `DATABASE_URL` 使用 `postgres:5432`，不要用宿主机视角的 `localhost:5432`。 |
| 容器内连不上 Qdrant | `QDRANT_HOST=qdrant`，`QDRANT_GRPC_PORT=6334`。 |
| 镜像拉取超时 | 检查 Docker Desktop 代理、公司网络策略和 registry mirror。 |
| `go mod download` 超时 | 配置可用 Go proxy 或 CI 网络代理。 |
