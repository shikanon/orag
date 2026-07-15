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

默认用户名和密码来自 [`.env.example`](../../.env.example)：

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `ADMIN_DEFAULT_USERNAME` | `admin` | 服务端校验的默认管理员用户名。 |
| `ADMIN_DEFAULT_PASSWORD` | `admin` | 服务端校验的默认管理员密码，生产必须替换。 |
| `AUTH_TOKEN_TTL` | `24h` | token 有效期，对应响应中的 `expires_in`。 |
| `JWT_SECRET` | 示例值 | token 签名密钥，生产必须替换。 |
| `API_KEY_PEPPER` | 示例值 | API Key 哈希的服务端 pepper，生产必须独立替换；轮换会使现有 API Key 失效。 |

## 鉴权请求

除 `GET /healthz`、`GET /readyz`、`GET /metrics`、`GET /docs` 和 `POST /v1/auth/login` 外，所有 `/v1/*` API 都需要：

```http
Authorization: Bearer <access_token>
```

Bearer 同时支持登录返回的用户 token 和 `orag_sk_` 机器 API Key。只有 `tenant_admin` 可以通过 `POST /v1/api-keys`、`GET /v1/api-keys` 和 `DELETE /v1/api-keys/{api_key_id}` 管理密钥；创建响应会返回一次完整 secret，后续列表只返回非敏感元数据。

`project_editor` 和 `project_viewer` 必须绑定项目。项目级密钥可以访问其项目下的知识库、上传、查询、数据集和评测；editor 可以写入这些资源，viewer 只能读取、查询和读取评测结果。创建知识库或数据集时省略 `project_id` 会自动使用密钥绑定的项目，显式指定其他项目会返回 `403 forbidden`。

尚未完成 Project 归属迁移的租户级路由仍会返回 `403 forbidden`，不会退化为租户全量访问。跨项目读取 Project 返回 `404 project_not_found`，跨项目读取知识库或数据集分别按资源不存在返回 `404`，避免泄漏资源是否存在。tenant admin 在 beta 兼容期仍可访问省略 `project_id` 的旧资源；项目级密钥永远不能访问这类未归属资源。

`API_KEY_PEPPER` 用于对高熵 API Key 计算 HMAC-SHA-256。它必须与数据库凭据分开管理；轮换该值会立即使所有现有 API Key 失效，因此应先创建并分发替代密钥，再执行计划内轮换。

成功认证会以 best-effort 方式更新 `last_used_at`。同一进程对同一密钥最多每 5 分钟尝试一次更新，数据库还会用时间条件抑制多实例重复写；审计更新时间失败不会让已经验证成功的请求失败。撤销、过期、格式错误或哈希不匹配的密钥不会更新使用时间。

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

调用方如果复用同一个 `X-Trace-ID`，HTTP 响应和日志仍会使用该 ID；PostgreSQL 中的持久化 RAG trace 按最后完成的请求整体覆盖，同一个 `trace_id` 不会追加或混合多次请求的 node spans。需要分别查询多次请求时，应为每次请求提供不同的 `X-Trace-ID`，或让服务自动生成。

SSE 查询在 RAG 查询阶段失败时，响应仍是 `text/event-stream`，事件名为 `error`，事件数据中包含 `code`、`message` 和 `trace_id`。SSE 的 `trace` 事件、后续 `chunk`/`citations`/`done` 事件和失败时的 `error` 事件使用同一个 `trace_id`。

排查持久化 RAG trace 时可以使用 HTTP API 或 CLI。HTTP API 会按当前 Bearer token 所属 tenant 过滤：

```http
GET /v1/traces?limit=20
GET /v1/traces/{trace_id}
Authorization: Bearer <access_token>
```

CLI 适合直接排查本地 PostgreSQL：

```bash
oragctl trace --trace-id trace_xxx
```

上述入口都会返回查询元数据、profile、总耗时、错误状态和 node span 列表。HTTP 列表接口支持 `profile`、`since`、`until`、`has_error`、`slow_ms` 和 `limit` 过滤；CLI 还支持 `--stats` 聚合 node latency 统计。

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
| `404` | `tutorial_not_found` | 教程模板 ID 不存在。 |
| `404` | `tutorial_version_not_found` | 教程存在，但请求的不可变版本不存在。 |
| `413` | `payload_too_large` | 入库内容超过 `INGEST_MAX_DOCUMENT_BYTES`。 |
| `500` | `knowledge_base_create_failed`、`knowledge_base_list_failed`、`knowledge_base_lookup_failed`、`knowledge_base_delete_failed`、`ingest_failed`、`query_failed`、`evaluation_failed`、`optimization_failed` | 知识库创建/列表/详情/删除后端失败，或后端入库、查询、评估、优化链路失败。 |
| `500` | `tutorial_catalog_failed` | 教程目录配置无效，或无法安全解析公开 Manifest URL。 |

## 安全建议

- 生产必须替换 `JWT_SECRET`、`ADMIN_DEFAULT_PASSWORD` 和所有外部依赖密钥。
- 不要把真实 `.env`、token 或 API key 写入文档、日志、issue 或 CI 输出。
- 如果轮换 `JWT_SECRET`，既有 token 会失效，需要重新登录。
- 默认账号只适合本地开发和 smoke，生产应通过部署平台注入强密码。
