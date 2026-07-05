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

未被 curl 脚本覆盖但已实现的接口包括：`GET /v1/knowledge-bases`、`GET /v1/knowledge-bases/{id}`、`DELETE /v1/knowledge-bases/{id}`、`GET /v1/ingestion-jobs/{id}`、`GET /v1/evaluations/{id}`、`POST /v1/optimizations/{id}:cancel` 和 `POST /v1/optimizations/{id}:resume`。

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
| `413` | `payload_too_large` | 入库内容超过 `INGEST_MAX_DOCUMENT_BYTES`。 |
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
- `cache_status`：`hit`、`miss` 或 `error`。
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
  "top_k": 8
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
  "metrics": {
    "answer_accuracy": 1,
    "accuracy": 1,
    "hit_rate": 1,
    "pairwise_accuracy": 1,
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

当前评估 runner 会对数据集中的每个样本调用同一条 `POST /v1/query` 背后的 RAG 查询链路，并计算运行级汇总指标。请求可选 `judge` 和 `qag` 配置；启用后会额外执行 LLM-as-Judge 或 QAG claim verification，并持久化 judge run、raw/parsed response、token usage 和 cost。

- `answer_accuracy`：答案包含 `ground_truth` 中长度大于 3 的关键项时记为命中，citation 不会提升该指标。
- `accuracy` / `hit_rate`：新运行中与 `answer_accuracy` 保持一致，作为兼容字段保留；历史已存运行可能没有 `answer_accuracy`、`citation_hit_rate` 或 `pairwise_accuracy`。
- `pairwise_accuracy`：优化器主质量指标；未执行 pairwise judge 时由 `answer_accuracy` 填充，启用成对比较后表示候选在成对比较中的胜出或不输比例。
- `faithfulness`、`groundedness`、`citation_support`、`hallucination`、`completeness` 等 Judge 指标：仅当请求包含 `judge` 时产生，分数和理由会进入评估明细。
- `qag_score`、`qag_claim_coverage`、`qag_question_count`、`qag_unverifiable_rate`：仅当请求包含 `qag` 时产生，用于基于 claim 的上下文支撑验证。
- `citation_hit_rate`：响应存在至少一个 citation 时记为 `1`，用于单独观察证据存在性。
- `context_recall`：召回 chunk 的 `document_id` 覆盖 `relevant_doc_ids` 的比例；如果样本没有 `relevant_doc_ids`，但有召回结果，则记为 `1`。
- `citation_precision`：citation 命中 `relevant_doc_ids` 的比例；如果样本没有 `relevant_doc_ids` 且存在 citation，则记为 `1`。
- `ndcg_at_k` / `recall_at_k` / `mrr` / `map`：基于 `relevant_doc_ids` 的 IR 排序指标；缺失标注时为 `0`。
- `coverage` / `retrieval_failure_rate`：基于 `relevant_doc_ids` 判断是否至少召回一个相关文档；未标注时不会报失败。
- `redundancy_rate` / `duplicate_count` / `deduped_top_k_count`：基于 chunk ID、hash/dedupe key 或规范化文本判断重复召回，不依赖人工标注。
- `alpha_ndcg` / `aspect_coverage`：基于 `diversity_annotations` 的多样性指标；缺少有效 aspect/subquestion 标注时跳过，不参与运行级聚合。
- `latency_p95_ms`：样本查询延迟的 p95。
- `cache_hit_rate`：查询响应中 `cache_status=hit` 的比例。

`pairwise_accuracy` 是候选排序的主指标，`accuracy` 和 `hit_rate` 保留用于向后兼容。复杂召回策略如更大的 `top_k`、高精度 profile、融合召回或 rerank 可能提升 `pairwise_accuracy`、`ndcg_at_k`、`recall_at_k`，但也可能提高 `latency_p95_ms`；线上调参时建议先按 `pairwise_accuracy` 选候选，再用 `latency_p95_ms` 约束尾延迟是否满足业务 SLO。

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
    "maximize": "pairwise_accuracy",
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
