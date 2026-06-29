# API 文档

本目录面向 API 调用方、SDK 开发者和前端集成者。权威契约源文件是 `../../api/openapi.yaml`，服务内置入口是 `GET /docs`。

## 文档结构

| 文档 | 内容 |
| --- | --- |
| `auth-and-errors.md` | 认证、Bearer token、统一错误响应、状态码和 trace_id。 |
| `ingestion-and-query.md` | 知识库、文档入库、ingestion job、JSON 查询和 SSE 查询。 |
| `../api.md` | 兼容长文，保留所有当前 API 的完整示例。 |

## API 分组

| 分组 | 端点 | 是否鉴权 |
| --- | --- | --- |
| 系统检查 | `GET /healthz`、`GET /readyz`、`GET /metrics`、`GET /docs` | 否 |
| 认证 | `POST /v1/auth/login` | 否 |
| 知识库 | `/v1/knowledge-bases` | 是 |
| 文档入库 | `/v1/knowledge-bases/{id}/documents`、`/documents:import` | 是 |
| 入库任务 | `GET /v1/ingestion-jobs/{id}` | 是 |
| 查询 | `POST /v1/query`、`POST /v1/query:stream` | 是 |
| 数据集与评估 | `/v1/datasets`、`/v1/evaluations` | 是 |
| 优化 | `POST /v1/optimizations` | 是 |

## Trace 查询入口

当前 HTTP API 不提供 trace 查询端点。RAG 查询响应、SSE `trace`/`error` 事件和统一错误响应都会返回 `trace_id`，排查时使用 CLI 读取 PostgreSQL 中的持久化 trace：

```bash
oragctl trace --trace-id trace_xxx
```

成功命中时输出 `found=true` 和 `trace` 对象，包含 `tenant_id`、`profile`、`latency_ms`、`has_error`、`error_count` 和按时间排序的 `node_spans`；未命中时输出 `found=false` 和查询的 `trace_id`。当前边界是只支持按单个 `trace_id` 精确查询，不支持 HTTP 查询、列表、时间范围过滤或跨租户聚合。

## 通用调用约定

除系统检查和登录外，所有 `/v1/*` API 都需要 Bearer token：

```http
Authorization: Bearer <access_token>
```

JSON 请求需要发送：

```http
Content-Type: application/json
```

常见成功状态码：

| 状态码 | 含义 |
| --- | --- |
| `200 OK` | 查询、列表、详情类响应。 |
| `201 Created` | 创建知识库、创建数据集、添加数据集样本。 |
| `202 Accepted` | 文档入库、运行评估、运行优化。 |
| `204 No Content` | 删除知识库当前返回空响应。 |

## 契约维护

API 行为变更必须同步检查：

```bash
PATH="/usr/local/go/bin:/opt/homebrew/bin:$PATH" make openapi-validate
```

如果新增或修改 endpoint，应同步更新：

| 文件 | 责任 |
| --- | --- |
| `../../api/openapi.yaml` | 机器可读 API 契约。 |
| `../api.md` 或本目录专题页 | 人类可读示例和边界说明。 |
| `../../tests/contract` | 契约测试。 |
| `../../examples/curl/` | 主路径 smoke 示例。 |
