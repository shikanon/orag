# 入库与查询 API

本文聚焦 ORAG 的主业务路径：创建知识库、导入文档、查看入库任务、执行 RAG 查询、使用 SSE 流式查询，以及通过 Offline Knowledge API 把历史 query/trace 转化为可审核的召回优化项。

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

## 断点续传上传

```http
POST /v1/knowledge-bases/{id}/uploads
Authorization: Bearer <access_token>
Content-Type: application/json
```

该入口用于大文件或弱网络场景。创建会话时传入 `name`、可选 `source_uri` 和可选 `total_bytes`。服务返回 `upload_url`、`complete_url` 和当前 `received_bytes`。

```json
{
  "name": "large.md",
  "source_uri": "upload://large.md",
  "total_bytes": 104857600
}
```

客户端按当前 offset 追加原始字节：

```http
PUT /v1/uploads/{id}
Authorization: Bearer <access_token>
Upload-Offset: 0
Content-Type: application/octet-stream
```

如果请求中断，客户端先调用 `GET /v1/uploads/{id}`，读取响应中的 `received_bytes`，再用该值作为下一次 `Upload-Offset` 继续上传。若 offset 不匹配，服务返回 `409 upload_offset_mismatch`，并在 `error.details.received_bytes` 中返回可续传位置。

上传完成后调用：

```http
POST /v1/uploads/{id}:complete
Authorization: Bearer <access_token>
```

完成接口会把已接收的临时文件交给现有入库管线处理，成功返回 `upload`、`document`、`chunks` 和 `job`。取消上传使用 `DELETE /v1/uploads/{id}`，会删除临时内容并让该会话不可再查询。

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

## Offline Knowledge API

Offline Knowledge API 面向离线知识优化闭环：按时间窗口读取真实历史问题和 trace，聚类高价值问题，重放当前 RAG 召回，调用受控 Codex 分析与只读工具，生成带证据的优化项，再通过人工审核、shadow、回归验证和发布流程影响后续召回。所有接口都需要 Bearer token，并按当前 tenant 隔离数据。

### 离线运行

```http
POST /v1/offline-knowledge/runs
Authorization: Bearer <access_token>
Content-Type: application/json
```

请求：

```json
{
  "kb_id": "kb_xxx",
  "window_start": "2026-07-07T00:00:00Z",
  "window_end": "2026-07-08T00:00:00Z",
  "config_hash": "cfg_sha256",
  "max_questions": 500,
  "max_clusters": 200
}
```

`window_start` 和 `window_end` 必填，且结束时间必须晚于开始时间。`kb_id` 可选，服务端也接受 `knowledge_base_id` 作为兼容字段。同一 tenant、知识库、时间窗口和 `config_hash` 已存在时，接口返回已有 run 并标记 `deduplicated=true`。创建 run 返回 `202`，只持久化 `pending` run；执行真实链路使用 `POST /v1/offline-knowledge/runs/{id}/execute`。

查询运行和问题簇：

```http
GET /v1/offline-knowledge/runs?kb_id=kb_xxx&status=pending&limit=20
GET /v1/offline-knowledge/runs/{id}
POST /v1/offline-knowledge/runs/{id}/execute
GET /v1/offline-knowledge/runs/{id}/questions?limit=100
POST /v1/offline-knowledge/scheduler:trigger
```

`GET /v1/offline-knowledge/runs/{id}/questions` 返回 `QuestionCluster` 列表，包含规范问题、样例问题、出现次数和 trace IDs，用于解释优化项来自哪些真实用户问题。

`POST /v1/offline-knowledge/runs/{id}/execute` 对 `pending` 或 `failed` run 执行真实 history extraction、cluster、recall replay、Codex analyze、validation 和 item persistence；非可执行状态返回 `409 offline_knowledge_run_execution_conflict`。`POST /v1/offline-knowledge/scheduler:trigger` 手动触发已启用 scheduler 的配置 targets；scheduler 禁用、无 targets、Codex/Judge/Regression 等依赖缺失时返回 `503`，而不是暴露看似成功的空实现。

### 优化项与来源校验

```http
GET /v1/optimization-items?kb_id=kb_xxx&status=verified&item_type=answer_item
GET /v1/optimization-items/{id}
POST /v1/optimization-items/{id}/{action}
POST /v1/optimization-items/revalidate
```

`OptimizationItem` 的核心字段包括 `item_type`、`status`、`canonical_question`、`recall_quality`、`failure_type`、`confidence`、`source_fingerprints`、`evidence`、`deep_search_steps` 和可选 `eval_report_json`。`item_type` 可为 `answer_item`、`query_rewrite_item` 或 `knowledge_gap_item`。

`answer_item` 只做召回引导，不做直接答案注入。它可以提供 `final_answer`、`source_fingerprints` 和 `evidence` 供审核、shadow matching、重排或 prompt 组装参考，但用户最终答案仍必须由当前真实召回 chunk 和生成链路产生，不能把 `final_answer` 直接返回给用户。

source 必须使用 `doc_version` 和 `chunk_content_hash` 共同校验：

```json
{
  "doc_id": "doc_xxx",
  "doc_version": "v3",
  "chunk_id": "chk_xxx",
  "chunk_content_hash": "sha256:abc123"
}
```

`doc_id` 和 `chunk_id` 只用于定位来源；`doc_version` 与 `chunk_content_hash` 用于判断内容是否已经变化。revalidate 发现版本或 hash 不匹配时，会把相关优化项从 `stale` 推进到 `deprecated` 或需要人工复核的状态，避免过期证据继续影响召回。

### 动作与 Shadow

`POST /v1/optimization-items/{id}/{action}` 支持 `verify`、`reject`、`enable-shadow`、`publish`、`disable`、`revalidate` 和 `run-regression`。非法状态迁移返回 `409 invalid_optimization_item_transition`，不存在或跨 tenant 的 item 返回 `404 optimization_item_not_found`。`run-regression` 使用配置的 `OFFLINE_KNOWLEDGE_ORGANIZER_REGRESSION_DATASET_ID` 作为真实回归数据集；未启用回归、dataset id 缺失或 eval/RAG 依赖不可用时返回 `503 offline_knowledge_dependency_unavailable`。

批量 revalidate 示例：

```json
{
  "kb_id": "kb_xxx",
  "status": "stale",
  "source_content_hash": "sha256:abc123",
  "limit": 100
}
```

响应包含 `matched`、`updated`、`skipped` 和逐项 `results`。每个 `RevalidateResult` 返回 `item`、`old_status`、`new_status`、`updated` 和 `skipped`。

shadow 默认非侵入：`shadow_retrieval_enabled` 可以记录 shadow 命中和效果指标，`shadow_inject_enabled` 默认关闭，因此优化库匹配不会直接进入用户可见上下文。只有显式开启注入后，匹配结果才作为召回引导参与链路；即使开启，`answer_item` 仍只能提供 source/evidence guidance，不能绕过 source 校验或直接替代生成答案。
