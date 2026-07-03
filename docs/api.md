# API

本文描述当前服务已经实现的 HTTP API。契约源文件是 `api/openapi.yaml`，服务内置入口是 `GET /docs`，该入口只返回一个最小 HTML 页面并指向 OpenAPI 源文件；真实可调用路由以 `internal/http/router.go` 和 `api/openapi.yaml` 保持一致为准。

`examples/curl/` 下的脚本是本地 smoke 示例，默认使用 `BASE_URL=http://localhost:8080`，并把 token、知识库 ID、数据集 ID、文档 ID、入库 job ID 写入 `.orag-demo/`。这些脚本没有覆盖全部 API，但覆盖了登录、建库、文本入库、查询和评估主路径。

| 脚本 | 覆盖的 API | 说明 |
| --- | --- | --- |
| `examples/curl/00_login.sh` | `POST /v1/auth/login` | 使用 `ADMIN_USERNAME`、`ADMIN_PASSWORD`，默认 `admin`/`admin`，写入 `.orag-demo/token`。 |
| `examples/curl/10_create_kb.sh` | `POST /v1/knowledge-bases` | 使用 `KB_NAME`、`KB_DESCRIPTION`，写入 `.orag-demo/kb_id`。 |
| `examples/curl/20_upload_doc.sh` | `POST /v1/knowledge-bases/{id}/documents:import` | 使用 `DOC_NAME`、`DOC_SOURCE_URI`、`DOC_CONTENT`，写入 `.orag-demo/document_id` 和 `.orag-demo/job_id`。 |
| `examples/curl/30_query.sh` | `POST /v1/query` | 使用 `QUERY`、`PROFILE`，默认查询 `realtime` profile。 |
| `examples/curl/40_eval.sh` | `POST /v1/datasets`、`POST /v1/datasets/{id}/items`、`POST /v1/evaluations` | 使用 `DATASET_NAME`、`DATASET_KIND`、`EVAL_QUERY`、`GROUND_TRUTH`、`PROFILE`、`TOP_K`。 |
| `examples/curl/lib.sh` | 无直接业务端点 | 提供 `BASE_URL`、`.orag-demo/` 状态文件、`TOKEN`/`KB_ID` 覆盖和简单 JSON 转义。 |

未被 curl 脚本覆盖但已实现的接口包括：`GET /v1/knowledge-bases`、`GET /v1/knowledge-bases/{id}`、`DELETE /v1/knowledge-bases/{id}`、`POST /v1/knowledge-bases/{id}/documents`、`GET /v1/ingestion-jobs/{id}`、`POST /v1/query:stream`、`GET /v1/evaluations/{id}` 和 `POST /v1/optimizations`。

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

当请求未显式传 `profile` 时，服务使用配置项 `RAG_DEFAULT_PROFILE`，默认是 `realtime`。当请求未显式传 `top_k` 时，服务使用配置的 dense top-k，默认来自 `RAG_DENSE_TOP_K=50`；显式传入的 `top_k` 必须在 `1..100` 范围内。底层 dense/sparse 候选规模仍分别受 `RAG_DENSE_TOP_K` 和 `RAG_SPARSE_TOP_K` 影响。

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
| `404` | `ingestion_job_not_found` | 查询不存在的入库 job。 |
| `404` | `evaluation_not_found` | 查询不存在的评估结果。 |
| `413` | `payload_too_large` | 入库内容超过 `INGEST_MAX_DOCUMENT_BYTES`。 |
| `500` | `knowledge_base_write_failed`、`ingest_failed`、`query_failed`、`evaluation_failed`、`optimization_failed` | 知识库创建写入失败，或后端入库、查询、评估、优化链路失败。 |

`trace_id` 用于排查同一次请求链路。服务优先复用请求头 `X-Trace-ID`；未传入时自动生成，并把同一个值写入响应头 `X-Trace-ID`、JSON 错误体、SSE 事件、结构化日志和 RAG trace 持久化记录。`POST /v1/query:stream` 在 RAG 查询阶段失败时返回 `text/event-stream`，事件名为 `error`，事件数据仍包含 `code`、`message`、`trace_id`。

当前 HTTP API 不提供 trace 查询端点。排查 RAG trace 时使用 CLI 查询 PostgreSQL：

```bash
oragctl trace --trace-id trace_xxx
```

命中时返回 `found=true` 和 `trace` 对象，包含 `tenant_id`、`profile`、`latency_ms`、`has_error`、`error_count` 和按时间排序的 `node_spans`；未命中时返回 `found=false` 和查询的 `trace_id`。

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

`name` 是必填字段。`examples/curl/10_create_kb.sh` 只发送 `name` 和 `description`，不发送 `metadata`。

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

`content` 是 OpenAPI 中唯一必填字段。实现中如果 `name` 为空，会使用 `imported.md`。`examples/curl/20_upload_doc.sh` 使用的就是该 JSON 导入接口。

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

必填字段是 `knowledge_base_id` 和 `query`。`profile` 可选值为 `realtime`、`high_precision`；`session_id` 当前作为请求字段透传给 RAG 层，不在 OpenAPI 响应中返回；`top_k` 控制本次请求最终融合后的检索结果数量，显式传入时必须在 `1..100` 范围内。

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
- `citations`：引用列表，包含 `chunk_id`、`document_id`、`source_uri`，可选 `section` 和 `quote`。
- `retrieved_chunks`：召回结果，包含 chunk、分数、排序和来源；重排成功时 `from` 可能为 `ark_rerank`。
- `trace_id`：本次查询 trace。
- `cache_status`：`hit`、`miss` 或 `error`。
- `warnings`：可选警告，例如无召回上下文时会包含 `no_retrieved_context`。

`examples/curl/30_query.sh` 调用该接口，不发送 `top_k`，默认由服务配置决定。

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
  "relevant_doc_ids": ["doc_xxx"]
}
```

响应 `201 Created`：

```json
{
  "id": "dsi_xxx",
  "dataset_id": "ds_xxx",
  "query": "ORAG 使用什么向量库？",
  "ground_truth": "Qdrant",
  "relevant_doc_ids": ["doc_xxx"]
}
```

`examples/curl/40_eval.sh` 会先创建数据集，再添加一个样本；如果 `.orag-demo/document_id` 存在，会把该文档 ID 放入 `relevant_doc_ids`。

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
  "top_k": 8
}
```

响应 `202 Accepted`：

```json
{
  "id": "eval_xxx",
  "dataset_id": "ds_xxx",
  "profile": "realtime",
  "total": 1,
  "hit_rate": 1,
  "accuracy": 1,
  "metrics": {
    "answer_accuracy": 1,
    "accuracy": 1,
    "hit_rate": 1,
    "citation_hit_rate": 1,
    "context_recall": 1,
    "citation_precision": 1,
    "latency_p95_ms": 42,
    "cache_hit_rate": 0
  },
  "created_at": "2026-06-29T10:04:00Z"
}
```

当前评估 runner 会对数据集中的每个样本调用同一条 `POST /v1/query` 背后的 RAG 查询链路，并计算 rule-based 指标：

- `answer_accuracy`：答案包含 `ground_truth` 中长度大于 3 的关键项时记为命中，citation 不会提升该指标。
- `accuracy` / `hit_rate`：新运行中与 `answer_accuracy` 保持一致，作为兼容字段保留；历史已存运行可能没有 `answer_accuracy` 和 `citation_hit_rate`。
- `citation_hit_rate`：响应存在至少一个 citation 时记为 `1`，用于单独观察证据存在性。
- `context_recall`：召回 chunk 的 `document_id` 覆盖 `relevant_doc_ids` 的比例；如果样本没有 `relevant_doc_ids`，但有召回结果，则记为 `1`。
- `citation_precision`：citation 命中 `relevant_doc_ids` 的比例；如果样本没有 `relevant_doc_ids` 且存在 citation，则记为 `1`。
- `latency_p95_ms`：样本查询延迟的 p95。
- `cache_hit_rate`：查询响应中 `cache_status=hit` 的比例。

### 查询评估结果

```http
GET /v1/evaluations/{id}
```

响应体同运行评估响应。找不到评估结果时返回 `404 evaluation_not_found`。

## 优化

### 运行参数优化

```http
POST /v1/optimizations
```

请求：

```json
{
  "dataset_id": "ds_xxx",
  "knowledge_base_id": "kb_xxx",
  "profiles": ["realtime", "high_precision"],
  "top_ks": [5, 8]
}
```

`profiles` 和 `top_ks` 都是可选字段。未传 `profiles` 时，优化器默认尝试 `realtime` 和 `high_precision`；未传 `top_ks` 时，默认尝试 `[8]`。

响应 `202 Accepted`：

```json
{
  "id": "opt_xxx",
  "status": "completed",
  "best": {
    "profile": "high_precision",
    "top_k": 8,
    "score": 1,
    "run_id": "eval_best"
  },
  "candidates": [
    {
      "profile": "realtime",
      "top_k": 5,
      "score": 0.8,
      "run_id": "eval_xxx"
    },
    {
      "profile": "high_precision",
      "top_k": 8,
      "score": 1,
      "run_id": "eval_best"
    }
  ]
}
```

优化器会对 `profiles × top_ks` 做确定性网格枚举，每个候选都会运行一次评估；候选 `score` 优先使用该次评估的 `answer_accuracy`，仅在历史运行缺少该指标时回退到 `accuracy`，`run_id` 是对应的 evaluation run ID。当前优化结果即时返回，不额外提供 `GET /v1/optimizations/{id}` 查询端点。
