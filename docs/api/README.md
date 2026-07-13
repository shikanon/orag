# API 文档

本目录面向 API 调用方、SDK 开发者和前端集成者。权威契约源文件是 [`../../api/openapi.yaml`](../../api/openapi.yaml)，服务内置入口是 `GET /docs`。

## 文档结构

| 文档 | 内容 |
| --- | --- |
| [`auth-and-errors.md`](./auth-and-errors.md) | 认证、Bearer token、统一错误响应、状态码和 trace_id。 |
| [`ingestion-and-query.md`](./ingestion-and-query.md) | 知识库、文档入库、ingestion job、JSON 查询和 SSE 查询。 |
| [`../api.md`](../api.md) | 兼容长文，保留所有当前 API 的完整示例。 |

## API 分组

| 分组 | 端点 | 是否鉴权 |
| --- | --- | --- |
| 系统检查 | `GET /healthz`、`GET /readyz`、`GET /metrics`、`GET /docs` | 否 |
| 认证 | `POST /v1/auth/login` | 否 |
| 教程目录 | `GET /v1/tutorials`、`GET /v1/tutorials/{template_id}`、`GET /v1/tutorials/{template_id}/versions/{version}` | 是 |
| 知识库 | `/v1/knowledge-bases` | 是 |
| 文档入库 | `/v1/knowledge-bases/{id}/documents`、`/documents:import` | 是 |
| 入库任务 | `GET /v1/ingestion-jobs/{id}` | 是 |
| 查询 | `POST /v1/query`、`POST /v1/query:stream` | 是 |
| Trace | `GET /v1/traces`、`GET /v1/traces/{trace_id}` | 是 |
| 数据集与评估 | `/v1/datasets`、`/v1/evaluations` | 是 |
| 优化 | `POST /v1/optimizations` | 是 |

## 教程目录

教程目录是内嵌在服务二进制中的版本化只读资源。列表返回每个模板的最新版本，详情端点可以读取当前版本或指定的不可变语义版本。首批模板固定为 `text-rag`、`visual-document-rag` 和 `video-rag`，分别基于 CRUD-RAG、ViDoSeek 和 Video-MME 的精选子集设计。

响应中的 Pack `manifest_url` 由非密钥配置 `TUTORIAL_CATALOG_BASE_URL` 和模板内的相对路径组合而成，默认位于公开 OSS。当前 API 不会下载、安装或写入对象；克隆、Live Run、数据集生成与视频入库不属于本阶段。

## Trace 查询入口

RAG 查询响应、SSE `trace`/`error` 事件和统一错误响应都会返回 `trace_id`。排查时可以通过 HTTP API 查询当前 tenant 可见的持久化 trace，也可以在本地用 CLI 直接读取 PostgreSQL。

```http
GET /v1/traces?limit=20
GET /v1/traces/{trace_id}
Authorization: Bearer <access_token>
```

列表接口支持 `profile`、`since`、`until`、`has_error`、`slow_ms` 和 `limit` 查询参数。详情接口返回 `tenant_id`、`profile`、`latency_ms`、`has_error`、`error_count` 和按时间排序的 `node_spans`；trace 不存在或不属于当前 tenant 时返回 `404 trace_not_found`。

```bash
oragctl trace --trace-id trace_xxx
```

CLI 成功命中时输出 `found=true` 和 `trace` 对象；未命中时输出 `found=false` 和查询的 `trace_id`。当前边界是不提供跨租户聚合、采样、跨服务拓扑或外部 APM 跳转。

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
| `204 No Content` | 删除知识库成功，响应体为空。 |

## 契约维护

API 行为变更必须同步检查：

```bash
PATH="/usr/local/go/bin:/opt/homebrew/bin:$PATH" make openapi-validate
```

如果新增或修改 endpoint，应同步更新：

| 文件 | 责任 |
| --- | --- |
| [`../../api/openapi.yaml`](../../api/openapi.yaml) | 机器可读 API 契约。 |
| [`../api.md`](../api.md) 或本目录专题页 | 人类可读示例和边界说明。 |
| [`../../tests/contract`](../../tests/contract) | 契约测试。 |
| [`../../examples/curl/`](../../examples/curl) | 主路径 smoke 示例。 |
