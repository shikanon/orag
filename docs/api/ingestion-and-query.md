# 入库与查询 API

本文聚焦 ORAG 的主业务路径：创建知识库、导入文档、查看入库任务、执行 RAG 查询和使用 SSE 流式查询。

## 主路径

```text
login -> create knowledge base -> import document -> query -> evaluate
```

对应脚本：

```bash
examples/curl/00_login.sh
examples/curl/10_create_kb.sh
examples/curl/20_upload_doc.sh
examples/curl/30_query.sh
examples/curl/40_eval.sh
```

## 创建知识库

```http
POST /v1/knowledge-bases
Authorization: Bearer <access_token>
Content-Type: application/json
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

`name` 是必填字段。响应中会返回 `id`，后续文档入库和查询都需要使用该知识库 ID。

## 文本导入

```http
POST /v1/knowledge-bases/{id}/documents:import
Authorization: Bearer <access_token>
Content-Type: application/json
```

请求：

```json
{
  "name": "orag.md",
  "source_uri": "example://orag",
  "content": "ORAG 是 Go RAG 框架，支持 Qdrant、PostgreSQL sparse retrieval、RRF、Ark Rerank 和豆包生成。"
}
```

实现中如果 `name` 为空，会使用 `imported.md`。`content` 超过 `INGEST_MAX_DOCUMENT_BYTES` 时会返回 `413 payload_too_large`。

成功响应为 `202 Accepted`，包含：

| 字段 | 含义 |
| --- | --- |
| `document` | 文档摘要和归属知识库。 |
| `chunks` | 本次切分并写入索引的 chunk 数量。 |
| `job` | 入库任务状态和结果摘要。 |

## 文件上传

```http
POST /v1/knowledge-bases/{id}/documents
Authorization: Bearer <access_token>
Content-Type: multipart/form-data
```

该入口用于上传文件。当前实现会复用同一套 parser、chunker、store 和 ingestion job 记录逻辑。

## 查询入库任务

```http
GET /v1/ingestion-jobs/{id}
Authorization: Bearer <access_token>
```

入库 job 可用于确认文档是否已经成功写入元数据、FTS 和向量索引。找不到任务时返回 `404 ingestion_job_not_found`。

## JSON 查询

```http
POST /v1/query
Authorization: Bearer <access_token>
Content-Type: application/json
```

请求：

```json
{
  "knowledge_base_id": "kb_xxx",
  "query": "ORAG 支持哪些检索方式？",
  "profile": "realtime",
  "top_k": 8
}
```

响应包含：

| 字段 | 含义 |
| --- | --- |
| `answer` | 生成答案。 |
| `citations` | 引用文档和片段信息。 |
| `trace_id` | 请求链路 ID。 |
| `cache_status` | 语义缓存状态，例如 `hit` 或非命中状态。 |
| `warnings` | 查询过程中的非致命提醒。 |

`trace_id` 会贯穿本次 HTTP 请求、RAG pipeline、结构化日志和 PostgreSQL trace 记录。需要查看持久化 trace 时运行：

```bash
oragctl trace --trace-id trace_xxx
```

命中结果会包含 `node_spans`，用于判断失败或耗时集中在哪个 RAG 节点。

## SSE 流式查询

```http
POST /v1/query:stream
Authorization: Bearer <access_token>
Content-Type: application/json
Accept: text/event-stream
```

SSE 查询用于逐步返回生成过程。服务会先发送 `trace` 事件告知本次 `trace_id`；RAG 查询阶段失败时会发送 `error` 事件，事件数据仍包含统一错误模型中的 `code`、`message` 和同一个 `trace_id`。

## Profile 与 top_k

当前 profile 枚举：

| Profile | 说明 |
| --- | --- |
| `realtime` | 默认 profile，适合低延迟主路径。 |
| `high_precision` | 高精度 profile，用于评估和参数对比。 |

如果请求未传 `profile`，服务使用 `RAG_DEFAULT_PROFILE`，默认是 `realtime`。如果请求未传 `top_k` 或传入非正数，服务使用配置的 dense top-k，默认来自 `RAG_DENSE_TOP_K=50`。
