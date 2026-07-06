# 入库与查询 API

本文聚焦 ORAG 的主业务路径：创建知识库、导入文档、查看入库任务、执行 RAG 查询和使用 SSE 流式查询。

## 主路径

```text
health/ready -> login -> create knowledge base -> import/upload document -> query/SSE -> trace -> evaluate -> optimize
```

对应脚本：

```bash
examples/curl/05_health_ready.sh
examples/curl/00_login.sh
examples/curl/10_create_kb.sh
examples/curl/20_upload_doc.sh
examples/curl/25_upload_file.sh
examples/curl/30_query.sh
examples/curl/35_query_stream.sh
examples/curl/36_trace_lookup.sh
examples/curl/40_eval.sh
examples/curl/45_optimize.sh
examples/curl/50_optimize.sh
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

## 删除知识库

```http
DELETE /v1/knowledge-bases/{id}
Authorization: Bearer <access_token>
```

成功删除当前 tenant 拥有的知识库时返回 `204 No Content`。删除会清理该知识库直接拥有的 documents、chunks、ingestion jobs、Qdrant 向量点和语义缓存条目；缺失或其他 tenant 的知识库返回 `404 knowledge_base_not_found`，不会影响其他 tenant 数据。

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

实现中如果 `name` 为空，会使用 `imported.md`。如果 `{id}` 对应的知识库不存在或当前 tenant 不可访问，会返回 `404 knowledge_base_not_found`；`content` 超过 `INGEST_MAX_DOCUMENT_BYTES` 时会返回 `413 payload_too_large`。

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

该入口用于上传文件。当前实现会复用同一套 parser、chunker、store 和 ingestion job 记录逻辑。如果 `{id}` 对应的知识库不存在或当前 tenant 不可访问，会返回 `404 knowledge_base_not_found`。

解析方法由服务端环境变量 `INGEST_PARSER_METHOD` 决定：

| 方法 | 行为 | 适用格式 |
| --- | --- | --- |
| `basic` | 本进程内抽取文本；PDF、图片和 DOCX 内嵌图片会调用 `ARK_MULTIMODAL_MODEL` 生成 Markdown 描述。 | txt、md、csv、json、html、docx、pptx、xlsx、pdf、图片 |
| `mineru` | 调用 MinerU 兼容 `/file_parse` 服务，读取返回的 `content_list.json` 并归一化为 Markdown。 | pdf |
| `docling` | 调用 Docling Serve `/v1/convert/source` 或 `/v1alpha/convert/source`，读取 `md_content`、`text_content` 或 chunk 结果。 | pdf、docx |

`mineru` 需要配置 `MINERU_APISERVER`；`docling` 需要配置 `DOCLING_SERVER_URL`。如果上传格式不属于远程解析器支持范围，服务会回退到 `basic` 解析。

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
| `cache_status` | 语义缓存状态，例如 `hit`、`miss`，或 direct route 跳过检索与缓存时的 `bypass`。 |
| `warnings` | 查询过程中的非致命提醒。 |

`trace_id` 会贯穿本次 HTTP 请求、RAG pipeline、结构化日志和 PostgreSQL trace 记录。需要查看持久化 trace 时，可用 HTTP API 查询当前 tenant 内的记录：

```http
GET /v1/traces/{trace_id}
Authorization: Bearer <access_token>
```

也可以在本地直接运行 CLI：

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

如果请求未传 `profile`，服务使用 `RAG_DEFAULT_PROFILE`，默认是 `realtime`。如果请求未传 `top_k`，服务使用配置的 dense top-k，默认来自 `RAG_DENSE_TOP_K=50`；显式传入的 `top_k` 必须在 `1..100` 范围内。`top_k` 控制最终融合/重排后的 `retrieved_chunks` 数量；`RAG_CONTEXT_TOP_N` 独立控制进入回答 prompt 和 `citations` 的 chunk 数量，dense/sparse 候选池规模仍分别由 `RAG_DENSE_TOP_K` 和 `RAG_SPARSE_TOP_K` 配置决定。
