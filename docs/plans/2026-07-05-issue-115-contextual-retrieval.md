# Issue 115 Contextual Retrieval 技术方案

## 背景与目标

Issue: https://github.com/shikanon/orag/issues/115

当前 ingestion 将 `chunk.Content` 直接送入 embedding，并将同一字段写入 PostgreSQL `chunks.content_tsvector` 做 sparse retrieval。短 chunk、指代密集 chunk、跨段依赖 chunk 脱离全文后容易丢失定位信息。

目标是在 ingestion 中为每个 chunk 生成简短 contextual text，并让 dense embedding 与 sparse/FTS 使用同一份 `contextual text + chunk text` 检索表示，同时保留原始 chunk 内容用于引用和回答。

## 当前代码落点

- [`internal/ingest/service.go`](../../internal/ingest/service.go): parse -> split -> embed -> build chunks -> index。
- [`internal/kb/types.go`](../../internal/kb/types.go): `kb.Chunk` 是检索与索引的公共数据结构。
- [`internal/storage/postgres/repository.go`](../../internal/storage/postgres/repository.go): chunks 持久化。
- [`internal/storage/postgres/fts.go`](../../internal/storage/postgres/fts.go): PostgreSQL FTS sparse retriever。
- [`internal/storage/qdrant/vector_store.go`](../../internal/storage/qdrant/vector_store.go): dense vector 写入和查询。
- [`internal/rag/context_pack.go`](../../internal/rag/context_pack.go): 生成回答上下文，必须继续展示原始 chunk 内容。

## 设计决策

1. 新增 `kb.Chunk.ContextualText` 字段，存储 LLM 生成的定位说明。
2. 新增 `kb.Chunk.SearchText()` 方法，返回 `ContextualText + "\n\n" + Content`，用于 embedding、BM25/FTS 和 fallback sparse scoring。
3. PostgreSQL 新增 `chunks.contextual_text` 与 `chunks.search_text_tsvector`，FTS 查询改为搜索 `search_text_tsvector`，返回仍保留 `content` 和 `contextual_text`。
4. Qdrant payload 增加 `contextual_text`，vector 仍由 ingestion 对 `SearchText()` 的 embedding 生成。
5. Ingestion 新增可选 contextualizer。失败时按配置降级为原始 chunk，不让整个 job 失败。
6. Context pack、citation quote、RAG answer 继续只使用 `Content`，避免把生成的 contextual text 当原文证据。

## 配置

新增 `Ingestion.ContextualRetrieval` 配置：

- `INGEST_CONTEXTUAL_RETRIEVAL_ENABLED=false`
- `INGEST_CONTEXTUAL_MAX_DOCUMENT_CHARS=12000`
- `INGEST_CONTEXTUAL_MAX_CHUNK_CHARS=2000`
- `INGEST_CONTEXTUAL_MAX_CONTEXT_CHARS=500`
- `INGEST_CONTEXTUAL_FAILURE_MODE=fallback`，可选 `fallback`、`fail`

默认关闭，保持兼容。启用后依赖当前 `rag.Model` 的 Chat 能力。

## 开发拆解

1. 测试 `kb.Chunk.SearchText()`
   - 文件：[`internal/kb/types_test.go`](../../internal/kb/types_test.go)
   - 断言无 contextual text 时返回原文，有 contextual text 时包含 context 和原文。

2. 扩展 chunk schema 和 repository
   - 文件：[`migrations/000013_contextual_retrieval.sql`](../../migrations/000013_contextual_retrieval.sql)
   - 文件：[`internal/storage/postgres/repository.go`](../../internal/storage/postgres/repository.go)
   - 文件：[`internal/storage/postgres/fts.go`](../../internal/storage/postgres/fts.go)
   - 测试：[`internal/storage/postgres/repository_test.go`](../../internal/storage/postgres/repository_test.go)、[`internal/storage/postgres/fts_test.go`](../../internal/storage/postgres/fts_test.go)
   - 验证 FTS 使用 context 命中，但返回 citation content 仍是原文。

3. 扩展 memory 和 qdrant 索引路径
   - 文件：[`internal/kb/retrievers.go`](../../internal/kb/retrievers.go)
   - 文件：[`internal/storage/qdrant/payload.go`](../../internal/storage/qdrant/payload.go)
   - 文件：[`internal/storage/qdrant/vector_store_test.go`](../../internal/storage/qdrant/vector_store_test.go)
   - 验证 sparse fallback 使用 `SearchText()`，Qdrant payload 保留 context。

4. 新增 contextualizer
   - 文件：[`internal/ingest/contextual.go`](../../internal/ingest/contextual.go)
   - 接口：`Contextualizer.Contextualize(ctx, Request) ([]string, []string, error)`
   - 实现：`LLMContextualizer`，对每个 chunk 调用 Chat，prompt 只要求输出一段定位说明。
   - 测试：成功、失败 fallback、失败 fail。

5. 接入 ingestion
   - 文件：[`internal/ingest/service.go`](../../internal/ingest/service.go)
   - 先 split，再 contextualize，再 embed `chunk.SearchText()`，最后 Store。
   - 测试：[`internal/ingest/service_test.go`](../../internal/ingest/service_test.go) 验证 embedding 输入使用 contextual text，store 保存 context。

6. 配置和 app wiring
   - 文件：[`internal/config/config.go`](../../internal/config/config.go)
   - 文件：[`internal/app/app.go`](../../internal/app/app.go)
   - 文件：[`.env.example`](../../.env.example)、[`configs/config.example.yaml`](../../configs/config.example.yaml)
   - 默认关闭，启用时注入 `LLMContextualizer`。

7. 文档
   - 文件：[`README.md`](../../README.md)
   - 文件：[`docs/operations.md`](../operations.md)
   - 文件：[`docs/architecture/rag-pipeline.md`](../architecture/rag-pipeline.md)
   - 说明成本、失败降级、适用场景。

## 验收证据

- `go test ./internal/kb ./internal/ingest ./internal/storage/postgres ./internal/storage/qdrant`
- `go test ./...`
- 手动或测试证明：contextualization 失败且 `fallback` 时 ingestion 成功；`fail` 时 job failed。
- FTS 可通过 contextual text 命中 chunk；citation quote 不包含 contextual text。

