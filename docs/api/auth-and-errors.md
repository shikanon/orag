# 认证与错误模型

ORAG 使用管理员账号登录换取 Bearer token。业务请求不需要在 body 中传 tenant，服务会从 token 中读取当前默认租户上下文。

## 登录

```http
POST /v1/auth/login
Content-Type: application/json
```

请求示例：

```json
{
  "username": "admin",
  "password": "admin"
}
```

响应示例：

```json
{
  "access_token": "eyJ...",
  "token_type": "Bearer",
  "expires_in": 86400
}
```

默认用户名和密码来自 `.env.example`：

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `ADMIN_DEFAULT_USERNAME` | `admin` | 服务端校验的默认管理员用户名。 |
| `ADMIN_DEFAULT_PASSWORD` | `admin` | 服务端校验的默认管理员密码，生产必须替换。 |
| `AUTH_TOKEN_TTL` | `24h` | token 有效期，对应响应中的 `expires_in`。 |
| `JWT_SECRET` | 示例值 | token 签名密钥，生产必须替换。 |

## 鉴权请求

除 `GET /healthz`、`GET /readyz`、`GET /metrics`、`GET /docs` 和 `POST /v1/auth/login` 外，所有 `/v1/*` API 都需要：

```http
Authorization: Bearer <access_token>
```

curl 示例：

```bash
TOKEN="$(cat .orag-demo/token)"
curl -fsS http://localhost:8080/v1/knowledge-bases \
  -H "Authorization: Bearer ${TOKEN}"
```

## 统一错误响应

普通 JSON API 的错误响应统一为：

```json
{
  "error": {
    "code": "invalid_json",
    "message": "invalid json body",
    "trace_id": "trace_xxx"
  }
}
```

`trace_id` 用于排查同一次请求链路。服务会优先使用请求头 `X-Trace-ID`；未传入时自动生成新的 ID，并把同一个值写入响应头 `X-Trace-ID`、JSON 错误体、SSE 事件、结构化日志和 RAG trace 持久化记录。调用方应把错误响应中的 `trace_id` 原样反馈给运维或研发，不需要解析其中含义。

SSE 查询在 RAG 查询阶段失败时，响应仍是 `text/event-stream`，事件名为 `error`，事件数据中包含 `code`、`message` 和 `trace_id`。SSE 的 `trace` 事件、后续 `chunk`/`citations`/`done` 事件和失败时的 `error` 事件使用同一个 `trace_id`。

排查持久化 RAG trace 时使用 CLI，而不是 HTTP API：

```bash
oragctl trace --trace-id trace_xxx
```

该命令按 `trace_id` 查询 PostgreSQL 的 `rag_traces` 和 `rag_node_spans`，返回查询元数据、profile、总耗时、错误状态和 node span 列表。当前 API 契约中没有 `/v1/traces` 这类管理端点。

## 常见错误码

| HTTP 状态码 | `code` 示例 | 触发场景 |
| --- | --- | --- |
| `400` | `invalid_json` | JSON body 解析失败。 |
| `400` | `invalid_request` | 必填字段缺失，例如创建知识库缺少 `name`。 |
| `400` | `invalid_credentials` | 登录缺少用户名或密码。 |
| `401` | `invalid_credentials` | 登录用户名或密码不正确。 |
| `401` | `missing_bearer_token` | 受保护 API 未带 Bearer token。 |
| `401` | `invalid_bearer_token` | Bearer token 无效或过期。 |
| `404` | `knowledge_base_not_found` | 查询不存在或不属于当前 tenant 的知识库，或向这类知识库导入/上传文档。 |
| `404` | `dataset_not_found` | 写入样本、运行评估或优化时，数据集不存在或不属于当前 tenant。 |
| `404` | `ingestion_job_not_found` | 查询不存在的入库 job。 |
| `404` | `evaluation_not_found` | 查询不存在的评估结果。 |
| `413` | `payload_too_large` | 入库内容超过 `INGEST_MAX_DOCUMENT_BYTES`。 |
| `500` | `knowledge_base_write_failed`、`ingest_failed`、`query_failed`、`evaluation_failed`、`optimization_failed` | 知识库创建写入失败，或后端入库、查询、评估、优化链路失败。 |

## 安全建议

- 生产必须替换 `JWT_SECRET`、`ADMIN_DEFAULT_PASSWORD` 和所有外部依赖密钥。
- 不要把真实 `.env`、token 或 API key 写入文档、日志、issue 或 CI 输出。
- 如果轮换 `JWT_SECRET`，既有 token 会失效，需要重新登录。
- 默认账号只适合本地开发和 smoke，生产应通过部署平台注入强密码。
