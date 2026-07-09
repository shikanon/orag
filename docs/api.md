# API

本文描述当前服务已经实现的 HTTP API。契约源文件是 [`api/openapi.yaml`](../api/openapi.yaml)，服务内置入口是 `GET /docs`，该入口只返回一个最小 HTML 页面并指向 OpenAPI 源文件；真实可调用路由以 [`internal/http/router.go`](../internal/http/router.go) 和 [`api/openapi.yaml`](../api/openapi.yaml) 保持一致为准。

[`examples/curl/`](../examples/curl) 下的脚本是本地 smoke 示例，默认使用 `BASE_URL=http://localhost:8080`，并把 token、知识库 ID、数据集 ID、文档 ID、入库 job ID、trace ID、评估 ID 和优化 ID 写入 `.orag-demo/`。这些脚本没有覆盖全部 API，但覆盖了健康检查、登录、建库、文本/文件入库、普通查询、SSE 查询、trace 查询、评估和优化主路径。

| 脚本 | 覆盖的 API | 说明 |
| --- | --- | --- |
| [`examples/curl/05_health_ready.sh`](../examples/curl/05_health_ready.sh) | `GET /healthz`、`GET /readyz` | 在状态脚本前检查进程和依赖就绪状态。 |
| [`examples/curl/00_login.sh`](../examples/curl/00_login.sh) | `POST /v1/auth/login` | 使用 `ADMIN_USERNAME`、`ADMIN_PASSWORD`，默认 `admin`/`admin`，写入 `.orag-demo/token`。 |
| [`examples/curl/10_create_kb.sh`](../examples/curl/10_create_kb.sh) | `POST /v1/knowledge-bases` | 使用 `KB_NAME`、`KB_DESCRIPTION`，写入 `.orag-demo/kb_id`。 |
| [`examples/curl/20_upload_doc.sh`](../examples/curl/20_upload_doc.sh) | `POST /v1/knowledge-bases/{id}/documents:import` | 使用 `DOC_NAME`、`DOC_SOURCE_URI`、`DOC_CONTENT`，写入 `.orag-demo/document_id` 和 `.orag-demo/job_id`。 |
| [`examples/curl/25_upload_file.sh`](../examples/curl/25_upload_file.sh) | `POST /v1/knowledge-bases/{id}/documents` | 通过 multipart 上传本地 Markdown 文件，写入 `.orag-demo/document_id` 和 `.orag-demo/job_id`。 |
| [`examples/curl/30_query.sh`](../examples/curl/30_query.sh) | `POST /v1/query` | 使用 `QUERY`、`PROFILE`，默认查询 `realtime` profile，并写入 `.orag-demo/trace_id`。 |
| [`examples/curl/35_query_stream.sh`](../examples/curl/35_query_stream.sh) | `POST /v1/query:stream` | 使用 SSE 查询，保存 stream trace ID。 |
| [`examples/curl/36_trace_lookup.sh`](../examples/curl/36_trace_lookup.sh) | `GET /v1/traces`、`GET /v1/traces/{trace_id}` | 列出最近 trace 并读取单条 trace 明细。 |
| [`examples/curl/40_eval.sh`](../examples/curl/40_eval.sh) | `POST /v1/datasets`、`POST /v1/datasets/{id}/items`、`POST /v1/evaluations` | 使用 `DATASET_NAME`、`DATASET_KIND`、`EVAL_QUERY`、`GROUND_TRUTH`、`PROFILE`、`TOP_K`；可用 `ENABLE_JUDGE=true`、`ENABLE_QAG=true` 附加 Judge/QAG 配置。 |
| [`examples/curl/45_optimize.sh`](../examples/curl/45_optimize.sh) | `POST /v1/optimizations` | 兼容旧 `profiles`/`top_ks` 请求，便于快速优化 smoke。 |
| [`examples/curl/50_optimize.sh`](../examples/curl/50_optimize.sh) | `POST /v1/optimizations`、`GET /v1/optimizations/{id}` | 提交目标驱动异步优化 run，保存 `.orag-demo/optimization_id` 并轮询状态。 |
| [`examples/curl/lib.sh`](../examples/curl/lib.sh) | 无直接业务端点 | 提供 `BASE_URL`、`.orag-demo/` 状态文件、`TOKEN`/`KB_ID` 覆盖和简单 JSON 转义。 |

未被 curl 脚本覆盖但已实现的接口包括：`GET /v1/knowledge-bases`、`GET /v1/knowledge-bases/{id}`、`DELETE /v1/knowledge-bases/{id}`、`GET /v1/ingestion-jobs/{id}`、`GET /v1/evaluations/{id}`、`POST /v1/optimizations/{id}:cancel`、`POST /v1/optimizations/{id}:resume`、`GET/POST /v1/offline-knowledge/runs`、`POST /v1/offline-knowledge/scheduler:trigger`、`GET /v1/offline-knowledge/runs/{id}`、`POST /v1/offline-knowledge/runs/{id}/execute`、`GET /v1/offline-knowledge/runs/{id}/questions`、`GET /v1/optimization-items`、`GET /v1/optimization-items/{id}`、`POST /v1/optimization-items/{id}/{action}` 和 `POST /v1/optimization-items/revalidate`。

## 通用约定

公开无需鉴权的端点：

- `GET /healthz`：进程存活检查，成功返回 `{"status":"ok"}`。
- `GET /readyz`：依赖就绪检查，返回 `status` 和各依赖 `checks`；依赖未就绪时 HTTP 状态码为 `503`。
- `GET /metrics`：Prometheus text format 指标。
- `GET /docs`：最小 HTML 文档入口。
- `POST /v1/auth/login`：登录换取 Bearer token。

除上述端点外，所有 `/v1/*` API 都需要 Bearer token：

```http
Authorization: Bearer <access_token>
```

JSON 请求需要发送：

```http
Content-Type: application/json
```

常见成功状态码：

- `200 OK`：查询、列表、详情类响应。
- `201 Created`：创建知识库、创建数据集、添加数据集样本。
- `202 Accepted`：文档入库、运行评估、运行优化。
- `204 No Content`：删除知识库成功，响应体为空。

可选的 RAG profile 目前只有两个枚举值：

- `realtime`
- `high_precision`

当请求未显式传 `profile` 时，服务使用配置项 `RAG_DEFAULT_PROFILE`，默认是 `realtime`。当请求未显式传 `top_k` 时，服务使用配置的 dense top-k，默认来自 `RAG_DENSE_TOP_K=50`；显式传入的 `top_k` 必须在 `1..100` 范围内。`top_k` 控制最终融合/重排后的 `retrieved_chunks` 数量；`RAG_CONTEXT_TOP_N` 只控制打包进回答 prompt 和 `citations` 的 chunk 数量。底层 dense/sparse 候选规模仍分别受 `RAG_DENSE_TOP_K` 和 `RAG_SPARSE_TOP_K` 影响。

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

已实现的常见错误码包括：

| HTTP 状态码 | `code` 示例 | 触发场景 |
| --- | --- | --- |
| `400` | `invalid_json` | JSON body 解析失败。 |
| `400` | `invalid_request` | 必填字段缺失，例如创建知识库缺少 `name`，multipart 上传缺少 `file`。 |
| `400` | `invalid_credentials` | 登录缺少用户名或密码。 |
| `401` | `invalid_credentials` | 登录用户名或密码不正确。 |
| `401` | `missing_bearer_token` | 受保护的 `/v1/*` API 未带 Bearer token。 |
| `401` | `invalid_bearer_token` | Bearer token 无效或过期。 |
| `404` | `knowledge_base_not_found` | 查询/删除不存在或不属于当前 tenant 的知识库，或向这类知识库导入/上传文档。 |
| `404` | `dataset_not_found` | 写入样本、运行评估或优化时，数据集不存在或不属于当前 tenant。 |
| `404` | `ingestion_job_not_found` | 查询不存在的入库 job。 |
| `404` | `evaluation_not_found` | 查询不存在的评估结果。 |
| `409` | `offline_knowledge_run_execution_conflict`、`invalid_optimization_item_transition` | Offline Knowledge run 当前状态不可执行，或优化项动作不满足状态机。 |
| `413` | `payload_too_large` | 入库内容超过 `INGEST_MAX_DOCUMENT_BYTES`。 |
| `503` | `offline_knowledge_scheduler_disabled`、`offline_knowledge_dependency_unavailable` | Offline Knowledge scheduler 被禁用，或真实运行依赖未启用/不可用，例如 Codex、source reader、validator、judge、regression dataset id。 |
| `500` | `knowledge_base_create_failed`、`knowledge_base_list_failed`、`knowledge_base_lookup_failed`、`knowledge_base_delete_failed`、`ingest_failed`、`query_failed`、`evaluation_failed`、`optimization_failed` | 知识库创建/列表/详情/删除后端失败，或后端入库、查询、评估、优化链路失败。 |

`trace_id` 用于排查同一次请求链路。服务优先复用请求头 `X-Trace-ID`；未传入时自动生成，并把同一个值写入响应头 `X-Trace-ID`、JSON 错误体、SSE 事件、结构化日志和 RAG trace 持久化记录。`POST /v1/query:stream` 在 RAG 查询阶段失败时返回 `text/event-stream`，事件名为 `error`，事件数据仍包含 `code`、`message`、`trace_id`。

Trace 可以通过 HTTP API 或 CLI 查询。HTTP API 会按当前 Bearer token 所属 tenant 过滤 trace；CLI 适合直接排查本地 PostgreSQL：

```http
GET /v1/traces?limit=20
GET /v1/traces/{trace_id}
Authorization: Bearer <access_token>
```

`GET /v1/traces` 支持 `profile`、`since`、`until`、`has_error`、`slow_ms` 和 `limit` 查询参数，返回 `items` 列表；`GET /v1/traces/{trace_id}` 返回单条 `TraceRecord`，不存在或不属于当前 tenant 时返回 `404 trace_not_found`。

```bash
oragctl trace --trace-id trace_xxx
```

CLI 命中时返回 `found=true` 和 `trace` 对象，包含 `tenant_id`、`profile`、`latency_ms`、`has_error`、`error_count` 和按时间排序的 `node_spans`；未命中时返回 `found=false` 和查询的 `trace_id`。

## 认证

### 登录

```http
POST /v1/auth/login
```

服务端校验配置中的 `ADMIN_DEFAULT_USERNAME` 和 `ADMIN_DEFAULT_PASSWORD`，默认值都是 `admin`。curl 脚本 `00_login.sh` 的 `ADMIN_USERNAME` 和 `ADMIN_PASSWORD` 只是客户端请求参数，默认同样是 `admin`/`admin`。

请求：

```json
{
  "username": "admin",
  "password": "admin"
}
```

响应：

```json
{
  "access_token": "eyJ...",
  "token_type": "Bearer",
  "expires_in": 86400
}
```

`expires_in` 来自服务端 `AUTH_TOKEN_TTL`，默认 24 小时。token 内部携带当前默认 tenant，业务请求不需要也不能通过 body 传 tenant。

## 知识库

### 创建知识库

```http
POST /v1/knowledge-bases
```

请求：

```json
{
  "name": "Demo KB",
  "description": "local demo",
  "metadata": {
    "owner": "orag"
  }
}
```

响应 `201 Created`：

```json
{
  "id": "kb_xxx",
  "tenant_id": "tenant_default",
  "name": "Demo KB",
  "description": "local demo",
  "metadata": {
    "owner": "orag"
  },
  "created_at": "2026-06-29T10:00:00Z",
  "updated_at": "2026-06-29T10:00:00Z"
}
```

`name` 是必填字段。[`examples/curl/10_create_kb.sh`](../examples/curl/10_create_kb.sh) 只发送 `name` 和 `description`，不发送 `metadata`。

### 列出知识库

```http
GET /v1/knowledge-bases
```

响应 `200 OK`：

```json
{
  "items": [
    {
      "id": "kb_xxx",
      "tenant_id": "tenant_default",
      "name": "Demo KB",
      "description": "local demo",
      "created_at": "2026-06-29T10:00:00Z",
      "updated_at": "2026-06-29T10:00:00Z"
    }
  ]
}
```

### 查询知识库详情

```http
GET /v1/knowledge-bases/{id}
```

响应体同创建知识库响应。找不到知识库时返回 `404 knowledge_base_not_found`。

### 删除知识库

```http
DELETE /v1/knowledge-bases/{id}
```

删除成功返回 `204 No Content`，不返回 JSON body；找不到知识库时返回 `404 knowledge_base_not_found`。删除会清理当前 tenant 下该知识库的文档、chunks、向量索引和语义缓存；清理失败时返回 JSON error。

## 文档入库

文档入库有两种入口：JSON 文本导入和 multipart 文件上传。两者成功后都返回同一个 `IngestionResponse`，包含文档摘要、chunk 数量和入库 job。若 `{id}` 对应的知识库不存在或当前 tenant 不可访问，两个入口都返回 `404 knowledge_base_not_found`。

### 导入文本

```http
POST /v1/knowledge-bases/{id}/documents:import
```

请求：

```json
{
  "name": "orag.md",
  "source_uri": "example://orag",
  "content": "ORAG 是 Go RAG 框架，支持 Qdrant、PostgreSQL sparse retrieval、RRF、Ark Rerank 和豆包生成。"
}
```

`content` 是 OpenAPI 中唯一必填字段。实现中如果 `name` 为空，会使用 `imported.md`。[`examples/curl/20_upload_doc.sh`](../examples/curl/20_upload_doc.sh) 使用的就是该 JSON 导入接口。

响应 `202 Accepted`：

```json
{
  "document": {
    "id": "doc_xxx",
    "tenant_id": "tenant_default",
    "knowledge_base_id": "kb_xxx",
    "source_uri": "example://orag",
    "title": "orag.md",
    "content_hash": "sha256_hex",
    "created_at": "2026-06-29T10:01:00Z"
  },
  "chunks": 1,
  "job": {
    "id": "job_xxx",
    "tenant_id": "tenant_default",
    "knowledge_base_id": "kb_xxx",
    "status": "succeeded",
    "source_uri": "example://orag",
    "document_id": "doc_xxx",
    "chunk_count": 1,
    "created_at": "2026-06-29T10:01:00Z",
    "updated_at": "2026-06-29T10:01:00Z"
  }
}
```

`metadata` 来自解析器，具体键会随文档内容和解析结果变化；没有解析到元数据时该字段可能不返回。

### 上传文件

```http
POST /v1/knowledge-bases/{id}/documents
Content-Type: multipart/form-data
```

请求字段：

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `file` | binary | 是 | 上传的文件内容。 |

示例：

```bash
curl -sS "$BASE_URL/v1/knowledge-bases/$KB_ID/documents" \
  -H "Authorization: Bearer $TOKEN" \
  -F "file=@./orag.md"
```

响应同文本导入接口。上传文件时 `source_uri` 由服务设置为 `upload://<filename>`，`title` 使用上传文件名。

### 断点续传上传

大文件可先创建上传会话，再按字节 offset 上传原始分片。任一分片请求中断后，客户端调用状态查询接口读取 `received_bytes`，再从该 offset 继续上传。

创建会话：

```http
POST /v1/knowledge-bases/{id}/uploads
Content-Type: application/json
```

```json
{
  "name": "large.md",
  "source_uri": "upload://large.md",
  "total_bytes": 104857600
}
```

响应 `201 Created`：

```json
{
  "id": "upl_xxx",
  "tenant_id": "tenant_default",
  "knowledge_base_id": "kb_xxx",
  "name": "large.md",
  "source_uri": "upload://large.md",
  "total_bytes": 104857600,
  "received_bytes": 0,
  "status": "uploading",
  "upload_url": "/v1/uploads/upl_xxx",
  "complete_url": "/v1/uploads/upl_xxx:complete",
  "created_at": "2026-07-07T10:01:00Z",
  "updated_at": "2026-07-07T10:01:00Z"
}
```

追加分片：

```http
PUT /v1/uploads/{id}
Upload-Offset: 0
Content-Type: application/octet-stream
```

服务只接受 `Upload-Offset` 等于当前 `received_bytes` 的分片。若客户端 offset 过期或重复，返回 `409 upload_offset_mismatch`，`details.received_bytes` 给出服务端当前可续传位置。

查询进度：

```http
GET /v1/uploads/{id}
```

完成入库：

```http
POST /v1/uploads/{id}:complete
```

如果创建会话时传了 `total_bytes`，完成前必须已收到相同字节数。成功响应为 `202 Accepted`，包含 `upload`、`document`、`chunks` 和 `job`。取消会话使用 `DELETE /v1/uploads/{id}`，会删除临时上传内容。

### 查询入库 job

```http
GET /v1/ingestion-jobs/{id}
```

响应 `200 OK`：

```json
{
  "id": "job_xxx",
  "tenant_id": "tenant_default",
  "knowledge_base_id": "kb_xxx",
  "status": "succeeded",
  "source_uri": "example://orag",
  "document_id": "doc_xxx",
  "chunk_count": 1,
  "created_at": "2026-06-29T10:01:00Z",
  "updated_at": "2026-06-29T10:01:00Z"
}
```

`status` 取值为 `running`、`succeeded`、`failed`。当前入库处理是在请求内完成的，同步成功时响应里的 job 通常已经是 `succeeded`；失败时 job 会记录 `error`。

## 查询

### 普通 JSON 查询

```http
POST /v1/query
```

请求：

```json
{
  "knowledge_base_id": "kb_xxx",
  "query": "ORAG 支持哪些检索能力？",
  "profile": "realtime",
  "session_id": "sess_demo",
  "top_k": 8
}
```

必填字段是 `knowledge_base_id` 和 `query`。`profile` 可选值为 `realtime`、`high_precision`；`session_id` 当前作为请求字段透传给 RAG 层，不在 OpenAPI 响应中返回；`top_k` 控制本次请求最终融合/重排后的 `retrieved_chunks` 数量，显式传入时必须在 `1..100` 范围内。未传 `top_k` 时使用服务 top-k，默认来自 `RAG_DENSE_TOP_K=50`；`RAG_CONTEXT_TOP_N` 独立限制进入回答 prompt 和 `citations` 的 chunk 数量，因此较大的 `top_k` 可能增加检索/重排工作，但不会自动扩大上下文打包数量。

响应 `200 OK`：

```json
{
  "answer": "ORAG 支持 Qdrant dense retrieval、PostgreSQL sparse retrieval、RRF 融合、Ark Rerank 和豆包生成。[chk_xxx]",
  "citations": [
    {
      "chunk_id": "chk_xxx",
      "document_id": "doc_xxx",
      "source_uri": "example://orag",
      "section": "Overview",
      "quote": "ORAG 是 Go RAG 框架..."
    }
  ],
  "retrieved_chunks": [
    {
      "chunk": {
        "id": "chk_xxx",
        "tenant_id": "tenant_default",
        "knowledge_base_id": "kb_xxx",
        "document_id": "doc_xxx",
        "content": "ORAG 是 Go RAG 框架...",
        "source_uri": "example://orag",
        "section": "Overview",
        "offset": 0,
        "metadata": {
          "document_title": "orag.md"
        }
      },
      "score": 0.98,
      "rank": 1,
      "from": "ark_rerank"
    }
  ],
  "trace_id": "trace_xxx",
  "cache_status": "miss",
  "profile": "realtime",
  "warnings": [],
  "latency_ms": 42,
  "created_at": "2026-06-29T10:02:00Z"
}
```

字段说明：

- `answer`：生成答案；当有 citation 且答案未包含第一个 chunk ID 时，实现会补充一个 chunk ID 提示。
- `citations`：引用列表，包含 `chunk_id`、`document_id`、`source_uri`，可选 `section` 和 `quote`；数量由 `RAG_CONTEXT_TOP_N` 和上下文 token 限制决定。
- `retrieved_chunks`：最终融合/重排后的召回结果，数量由 `top_k` 决定，包含 chunk、分数、排序和来源；重排成功时 `from` 可能为 `ark_rerank`。
- `trace_id`：本次查询 trace。
- `cache_status`：`hit`、`miss`、`error` 或 direct route 跳过检索与缓存时的 `bypass`。
- `warnings`：可选警告，例如无召回上下文时会包含 `no_retrieved_context`。

[`examples/curl/30_query.sh`](../examples/curl/30_query.sh) 调用该接口，不发送 `top_k`，默认由服务配置决定。

### SSE 查询

```http
POST /v1/query:stream
Accept: text/event-stream
```

请求体与 `POST /v1/query` 相同。当前实现不是上游模型 token streaming 透传，而是先执行同一条 RAG 查询链路，再把完整答案按固定大小切分成 SSE `chunk` 事件输出。

成功时事件顺序：

```text
event: trace
data: {"trace_id":"trace_xxx"}

event: chunk
data: {"text":"答案分块文本"}

event: citations
data: [{"chunk_id":"chk_xxx","document_id":"doc_xxx","source_uri":"example://orag"}]

event: done
data: {"cache_status":"miss","latency_ms":42,"profile":"realtime","trace_id":"trace_xxx","warnings":[]}
```

失败时响应状态码为 `500`，内容类型仍为 `text/event-stream`：

```text
event: error
data: {"code":"query_failed","message":"...","trace_id":"trace_xxx"}
```

OpenAPI 中已标注 `trace`、`chunk`、`citations`、`done`、`error` 这些事件名；curl 示例目录目前没有提供 SSE 脚本。

## 数据集

数据集接口用于组织评估样本。当前实现支持创建数据集和追加样本，评估接口按数据集 ID 读取样本。

### 创建数据集

```http
POST /v1/datasets
```

请求：

```json
{
  "name": "orag regression",
  "kind": "golden"
}
```

`name` 是 OpenAPI 中的必填字段；`kind` 为空时服务使用 `golden`。

响应 `201 Created`：

```json
{
  "id": "ds_xxx",
  "tenant_id": "tenant_default",
  "name": "orag regression",
  "kind": "golden",
  "version": "20260629100300",
  "created_at": "2026-06-29T10:03:00Z"
}
```

### 添加数据集样本

```http
POST /v1/datasets/{id}/items
```

请求：

```json
{
  "query": "ORAG 使用什么向量库？",
  "ground_truth": "Qdrant",
  "relevant_doc_ids": ["doc_xxx"],
  "diversity_annotations": [
    {
      "aspect": "vector store",
      "document_ids": ["doc_xxx"]
    }
  ]
}
```

响应 `201 Created`：

```json
{
  "id": "dsi_xxx",
  "dataset_id": "ds_xxx",
  "query": "ORAG 使用什么向量库？",
  "ground_truth": "Qdrant",
  "relevant_doc_ids": ["doc_xxx"],
  "diversity_annotations": [
    {
      "aspect": "vector store",
      "document_ids": ["doc_xxx"]
    }
  ]
}
```

如果 `{id}` 对应的数据集不存在或不属于当前 tenant，返回 `404 dataset_not_found`，不会写入样本。

`relevant_doc_ids` 是检索质量和引用质量指标的主要标注来源；`diversity_annotations` 是可选多样性标注，可用 `aspect` 或 `subquestion` 绑定 `chunk_id` / `chunk_ids`、`document_id` / `document_ids` 或 `source_uri` / `source_uris`。[`examples/curl/40_eval.sh`](../examples/curl/40_eval.sh) 会先创建数据集，再添加一个样本；如果 `.orag-demo/document_id` 存在，会把该文档 ID 放入 `relevant_doc_ids`。

## 评估

### 运行评估

```http
POST /v1/evaluations
```

请求：

```json
{
  "dataset_id": "ds_xxx",
  "knowledge_base_id": "kb_xxx",
  "profile": "realtime",
  "top_k": 8,
  "split": "holdout",
  "scoped_shadow_item_id": "opt_xxx",
  "holdout_gate": {
    "enabled": true,
    "min_sample_count": 1,
    "min_weighted_sample_count": 1,
    "quality_metric": "deterministic_answer_match",
    "min_quality": 0.8
  }
}
```

如果 `dataset_id` 不存在或不属于当前 tenant，返回 `404 dataset_not_found`，不会创建 `evaluation_runs` 或 `evaluation_results`。

响应 `202 Accepted`：

```json
{
  "id": "eval_xxx",
  "dataset_id": "ds_xxx",
  "profile": "realtime",
  "total": 1,
  "hit_rate": 1,
  "accuracy": 1,
  "weighted_sample_count": 1,
  "unweighted_sample_count": 1,
  "split": "holdout",
  "split_summary": {
    "holdout": {
      "unweighted_sample_count": 1,
      "weighted_sample_count": 1
    }
  },
  "holdout_gate": {
    "enabled": true,
    "passed": true,
    "split": "holdout",
    "quality_metric": "deterministic_answer_match",
    "quality": 1,
    "min_quality": 0.8,
    "sample_count": 1,
    "weighted_sample_count": 1
  },
  "metrics": {
    "deterministic_answer_match": 1,
    "answer_accuracy": 1,
    "accuracy": 1,
    "hit_rate": 1,
    "citation_hit_rate": 1,
    "context_recall": 1,
    "citation_precision": 1,
    "ndcg_at_k": 1,
    "recall_at_k": 1,
    "mrr": 1,
    "map": 1,
    "coverage": 1,
    "retrieval_failure_rate": 0,
    "redundancy_rate": 0,
    "duplicate_count": 0,
    "deduped_top_k_count": 1,
    "alpha_ndcg": 1,
    "aspect_coverage": 1,
    "latency_p95_ms": 42,
    "cache_hit_rate": 0
  },
  "created_at": "2026-06-29T10:04:00Z"
}
```

当前评估 runner 会对数据集中的每个样本调用同一条 `POST /v1/query` 背后的 RAG 查询链路，并计算运行级汇总指标。请求可选 `split` 过滤 `train` / `eval` / `holdout` / `gold` 样本；样本 `weight` 会参与 answer、retrieval、citation、latency、cost、Judge 和 QAG 等可聚合指标。请求可选 `holdout_gate`，用于记录 holdout 质量门禁是否通过；`scoped_shadow_item_id` 只用于 offline regression 的候选路径约束。请求可选 `judge` 和 `qag` 配置；启用后会额外执行 LLM-as-Judge 或 QAG claim verification，并持久化 judge run、raw/parsed response、token usage 和 cost。

- `deterministic_answer_match`：规则型答案命中指标，替代过去未执行 pairwise judge 时写入 `pairwise_accuracy` 的行为。
- `answer_accuracy`：答案包含 `ground_truth` 中长度大于 3 的关键项时记为命中，citation 不会提升该指标。
- `accuracy` / `hit_rate`：新运行中与 `answer_accuracy` 保持一致，作为兼容字段保留；历史已存运行可能没有 `answer_accuracy`、`citation_hit_rate` 或 `deterministic_answer_match`。
- `pairwise_accuracy`：仅表示真实 pairwise judge 的胜出或不输比例；历史已存运行可能仍包含规则型兼容值，新 rule-only 运行不再写入该字段。
- `faithfulness`、`groundedness`、`citation_support`、`hallucination`、`completeness` 等 Judge 指标：仅当请求包含 `judge` 时产生，分数和理由会进入评估明细。
- `qag_score`、`qag_claim_coverage`、`qag_question_count`、`qag_unverifiable_rate`：仅当请求包含 `qag` 时产生，用于基于 claim 的上下文支撑验证。
- `citation_hit_rate`：响应存在至少一个 citation 时记为 `1`，用于单独观察证据存在性。
- `context_recall`：召回 chunk 的 `document_id` 覆盖 `relevant_doc_ids` 的比例；如果样本没有 `relevant_doc_ids`，但有召回结果，则记为 `1`。
- `citation_precision`：citation 命中 `relevant_doc_ids` 的比例；如果样本没有 `relevant_doc_ids` 且存在 citation，则记为 `1`。
- `ndcg_at_k` / `recall_at_k` / `mrr` / `map`：基于 `relevant_doc_ids` 的 IR 排序指标；缺失标注时为 `0`。
- `coverage` / `retrieval_failure_rate`：基于 `relevant_doc_ids` 判断是否至少召回一个相关文档；未标注时不会报失败。
- `redundancy_rate` / `duplicate_count` / `deduped_top_k_count`：基于 chunk ID、hash/dedupe key 或规范化文本判断重复召回，不依赖人工标注。
- `alpha_ndcg` / `aspect_coverage`：基于 `diversity_annotations` 的多样性指标；缺少有效 aspect/subquestion 标注时跳过，不参与运行级聚合。
- `weighted_sample_count` / `unweighted_sample_count`：分别表示参与本次运行的样本权重总和与样本条数；`split_summary` 返回各 split 的样本统计。
- `holdout_gate`：当请求启用门禁时返回 `passed`、失败 `reasons`、质量指标、样本数和阈值；缺失 split、样本不足或质量低于阈值都会失败。
- `latency_p95_ms`：样本查询延迟的加权 p95。
- `cache_hit_rate`：查询响应中 `cache_status=hit` 的比例。

`deterministic_answer_match` 是未启用真实 pairwise judge 时的默认规则排序信号，`pairwise_accuracy` 只用于真实成对评审或读取历史兼容结果。复杂召回策略如更大的 `top_k`、高精度 profile、融合召回或 rerank 可能提升 `deterministic_answer_match`、`pairwise_accuracy`、`ndcg_at_k`、`recall_at_k`，但也可能提高 `latency_p95_ms`；线上调参时建议明确配置 `objective.maximize`，再用 `latency_p95_ms` 约束尾延迟是否满足业务 SLO。

### 查询评估结果

```http
GET /v1/evaluations/{id}
```

默认响应体同运行评估响应。传 `include_items=true` 会返回逐样本明细；传 `include_judge=true` 会返回 Judge/QAG run 和结果；传 `include_pairwise=true` 会返回 pairwise judge 结果。找不到评估结果时返回 `404 evaluation_not_found`。

## 优化

### 提交异步优化

```http
POST /v1/optimizations
```

目标驱动 optimizer 是异步任务。提交成功返回 `202 Accepted`、`run_id` 和轮询/取消/续跑 URL；调用方用 `GET /v1/optimizations/{id}` 查询状态和候选明细。

请求：

```json
{
  "dataset_id": "ds_xxx",
  "knowledge_base_id": "kb_xxx",
  "profile": "realtime",
  "objective": {
    "maximize": "deterministic_answer_match",
    "constraints": [
      {"expression": "latency_p95_ms <= 1000"}
    ],
    "tie_breakers": [
      {"metric": "latency_p95_ms", "direction": "asc"}
    ]
  },
  "search_space": {
    "retrieval": {
      "dense_top_k": [5, 8]
    }
  },
  "search": {
    "strategy": "grid",
    "max_candidates": 4
  },
  "budget": {
    "max_judge_calls": 20,
    "max_cost_usd": 1.0
  },
  "selection_split": "eval",
  "holdout_split": "holdout"
}
```

兼容旧请求：仍可提交 `profiles` 和 `top_ks`。HTTP 层会把第一个 `profiles` 值作为内部 RAG runner profile，并把 `top_ks` 映射为 `search_space.retrieval.dense_top_k`。建议新调用方直接使用 `objective` 和 `search_space`。

如果 `dataset_id` 不存在或不属于当前 tenant，返回 `404 dataset_not_found`，不会创建候选评估运行。

提交响应 `202 Accepted`：

```json
{
  "run_id": "opt_xxx",
  "status": "queued",
  "poll_url": "/v1/optimizations/opt_xxx",
  "cancel_url": "/v1/optimizations/opt_xxx:cancel",
  "resume_url": "/v1/optimizations/opt_xxx:resume"
}
```

状态查询：

```http
GET /v1/optimizations/{id}
```

响应包含 `run` 和 `candidates`。`run.status` 取值包括 `queued`、`running`、`completed`、`canceling`、`canceled`、`budget_stopped`、`failed`；每个 candidate 包含配置、状态、关联 `evaluation_run_id`、指标、objective score、holdout score、token/cost 和临时 namespace 清理状态。

取消与续跑：

```http
POST /v1/optimizations/{id}:cancel
POST /v1/optimizations/{id}:resume
```

取消会把 run 标记为 `canceling` 并写入 checkpoint；worker 观察到取消请求后停止调度新候选。续跑会清除取消标记，并从 checkpoint 跳过已完成候选，仅重试未完成候选。

## Offline Knowledge

Offline Knowledge API 用于离线分析历史 query/trace，聚类高价值问题，重放召回，产出带证据的优化项，并通过人工审核、shadow、回归验证和发布流程把优化建议安全引入线上召回链路。所有接口都按当前 Bearer token 所属 tenant 过滤。

### 创建与查询运行

```http
POST /v1/offline-knowledge/runs
POST /v1/offline-knowledge/scheduler:trigger
GET /v1/offline-knowledge/runs
GET /v1/offline-knowledge/runs/{id}
POST /v1/offline-knowledge/runs/{id}/execute
GET /v1/offline-knowledge/runs/{id}/questions
```

创建运行请求：

```json
{
  "kb_id": "kb_xxx",
  "window_start": "2026-07-07T00:00:00Z",
  "window_end": "2026-07-08T00:00:00Z",
  "config_hash": "cfg_sha256",
  "config_json": {
    "lookback_days": 1,
    "max_deep_search_steps": 12
  },
  "max_questions": 500,
  "max_clusters": 200
}
```

`window_start` 和 `window_end` 必填，且 `window_end` 必须晚于 `window_start`。`kb_id` 可为空；服务端也接受兼容字段 `knowledge_base_id`。同一 tenant、知识库、时间窗口和配置 hash 已存在时，响应为 `200 OK` 且 `deduplicated=true`；新建运行返回 `202 Accepted`。创建 run 只写入 `pending` 状态；`POST /v1/offline-knowledge/runs/{id}/execute` 才执行真实 runtime 链路。

运行响应：

```json
{
  "run": {
    "id": "run_xxx",
    "tenant_id": "tenant_default",
    "kb_id": "kb_xxx",
    "status": "pending",
    "window_start": "2026-07-07T00:00:00Z",
    "window_end": "2026-07-08T00:00:00Z",
    "config_hash": "cfg_sha256",
    "started_at": "2026-07-08T02:00:00Z"
  },
  "deduplicated": false
}
```

`GET /v1/offline-knowledge/runs` 支持 `kb_id`、`knowledge_base_id`、`status` 和 `limit` 查询参数；`status` 取值为 `pending`、`running`、`completed`、`failed`。`GET /v1/offline-knowledge/runs/{id}/questions` 返回本次运行产生的 `QuestionCluster` 列表，包含 `canonical_question`、`question_hash`、`occurrence_count`、`sample_questions` 和 `trace_ids`，用于解释优化项来自哪些真实问题。

`POST /v1/offline-knowledge/runs/{id}/execute` 只接受 `pending` 或 `failed` run；其他状态返回 `409 offline_knowledge_run_execution_conflict`。执行过程会读取真实 trace/history、执行问题聚类、重放 RAG 召回、调用 Codex analyzer/tool、做 source fingerprint/evidence validation 并写入 metrics。`POST /v1/offline-knowledge/scheduler:trigger` 手动触发已启用 scheduler 的 targets；scheduler 禁用或核心依赖缺失时返回 `503`。

### 优化项

```http
GET /v1/optimization-items
GET /v1/optimization-items/{id}
POST /v1/optimization-items/{id}/{action}
POST /v1/optimization-items/revalidate
```

列表接口支持 `kb_id`、`knowledge_base_id`、`run_id`、`status`、`item_type` 和 `limit`。`item_type` 取值包括 `answer_item`、`query_rewrite_item`、`knowledge_gap_item`；`status` 覆盖 `candidate`、`evidence_validating`、`needs_review`、`verified`、`shadow_enabled`、`regression_passed`、`regression_failed`、`published`、`knowledge_gap`、`rejected`、`stale` 和 `deprecated`。

优化项示例：

```json
{
  "id": "item_xxx",
  "tenant_id": "tenant_default",
  "run_id": "run_xxx",
  "kb_id": "kb_xxx",
  "question_cluster_id": "cluster_xxx",
  "item_type": "answer_item",
  "status": "verified",
  "canonical_question": "ORAG 如何处理 stale source？",
  "final_answer": "通过 doc_version 和 chunk_content_hash 校验来源。",
  "recall_quality": "miss",
  "failure_type": "semantic_gap",
  "confidence": 0.92,
  "source_fingerprints": [
    {
      "doc_id": "doc_xxx",
      "doc_version": "v3",
      "chunk_id": "chk_xxx",
      "chunk_content_hash": "sha256:abc123"
    }
  ],
  "evidence": [
    {
      "chunk_id": "chk_xxx",
      "doc_id": "doc_xxx",
      "quote": "source validation uses doc_version and chunk_content_hash",
      "supports": "source fingerprint validation"
    }
  ],
  "deep_search_steps": [
    {
      "step": 1,
      "tool": "trace_lookup",
      "query": "trace_xxx",
      "observation": "baseline recall missed the source chunk",
      "decision": "create answer_item guidance"
    }
  ],
  "created_at": "2026-07-08T02:01:00Z",
  "updated_at": "2026-07-08T02:01:00Z"
}
```

`answer_item` 只做召回引导，不是直接答案注入机制。shadow retrieval 可以把 `source_fingerprints`、`evidence` 和 `guidance_metadata` 暴露给召回/重排链路，但线上回答仍必须由真实召回 chunk 和生成链路完成，不能直接把 `final_answer` 当作用户答案返回。

source 校验必须使用 `doc_version` 和 `chunk_content_hash`。`doc_id` 或 `chunk_id` 只能定位来源，不能证明内容未变；revalidate 会对比当前 chunk 的版本和内容 hash，过期或缺失时把优化项推进到 `deprecated` 或需要复核的状态。

### 状态动作与 Revalidate

`POST /v1/optimization-items/{id}/{action}` 支持：

| action | 目标状态/行为 |
| --- | --- |
| `verify` | 将可人工确认的项推进到 `verified`。 |
| `reject` | 将当前项标记为 `rejected`。 |
| `enable-shadow` | 将 `verified` 项推进到 `shadow_enabled`。 |
| `publish` | 将通过回归的项推进到 `published`。 |
| `disable` | 将已发布或 stale 项推进到 `deprecated`。 |
| `revalidate` | 对单个 stale 项重新执行 source/evidence 校验。 |
| `run-regression` | 使用真实 eval/RAG runner 和配置的 regression dataset id 执行 baseline/with-optimization 对比。 |

非法状态迁移返回 `409 invalid_optimization_item_transition`，不存在或跨 tenant 的 item 返回 `404 optimization_item_not_found`。`run-regression` 依赖 `OFFLINE_KNOWLEDGE_ORGANIZER_REGRESSION_DATASET_ID`；回归禁用、dataset id 缺失或 eval/RAG 依赖不可用时返回 `503 offline_knowledge_dependency_unavailable`。

批量 revalidate 请求：

```json
{
  "kb_id": "kb_xxx",
  "status": "stale",
  "source_content_hash": "sha256:abc123",
  "limit": 100
}
```

也可以传 `source_fingerprint`、`source_doc_id` 或 `source_chunk_id` 作为过滤条件。响应包含 `matched`、`updated`、`skipped` 和逐项 `results`；单项结果包含 `old_status`、`new_status`、`updated`、`skipped` 和当前 `item`。

### Shadow 默认行为

shadow retrieval 默认是非侵入模式：`shadow_retrieval_enabled` 可记录匹配、rank、score 和效果指标，`shadow_inject_enabled` 默认关闭，因此不会把优化库内容直接注入用户可见上下文。只有显式开启注入后，系统才会把匹配结果作为召回引导参与后续链路；即使开启，`answer_item` 也只能提供 evidence/source guidance，不能绕过真实 source 校验和生成流程。
