# Issue 118 轻量图结构增强复杂多跳检索技术方案

## 背景与目标

Issue: https://github.com/shikanon/orag/issues/118

当前 dense 和 sparse 检索主要围绕 chunk 文本相似度，复杂多跳、实体关系、跨文档关联查询容易遗漏中间连接。目标是在不引入 Neo4j 等重型依赖的前提下，使用 PostgreSQL 存储轻量实体关系图，并在 query 阶段做图增强召回。

## 当前代码落点

- [`internal/ingest/service.go`](../../internal/ingest/service.go): ingestion 可在 chunk store 后抽取实体关系。
- [`internal/storage/postgres/repository.go`](../../internal/storage/postgres/repository.go): 已是默认元数据与 FTS 存储，可增加 graph relation persistence。
- [`internal/kb/graph.go`](../../internal/kb/graph.go): graph retriever decorator 扩展 hybrid 结果。
- [`internal/ingest/graph.go`](../../internal/ingest/graph.go): ingestion 阶段轻量实体/共现关系抽取。
- [`internal/app/app.go`](../../internal/app/app.go): 按配置装配 graph builder 和 graph retriever。

## 设计决策

1. 采用 PostgreSQL-based graph relation storage，避免新增部署组件。
2. 初版不使用 LLM JSON 抽取，采用轻量启发式实体抽取，chunk 内实体共现生成 relation。
3. PostgreSQL 新增单表 `graph_relations`，避免多表 JOIN 和新部署组件。
4. Query 识别使用同一启发式实体抽取，从 query 中匹配 subject/object。
5. `GraphRetriever` 作为 base retriever decorator，先保留 dense/sparse/RAPTOR 候选，再追加 graph 命中的相关 chunks，From 标记 `graph`。
6. 关联通过 `document_id`、`source_chunk_id`、`target_chunk_id` 可追踪。

## 配置

新增：

- `RAG_GRAPH_RETRIEVAL_ENABLED=false`
- `RAG_GRAPH_RETRIEVAL_TOP_K=8`
- `INGEST_GRAPH_MAX_ENTITIES_PER_CHUNK=6`

默认关闭，保持兼容。

## 开发拆解

1. 数据结构和迁移
   - 文件：[`migrations/000014_lightweight_graph_retrieval.sql`](../../migrations/000014_lightweight_graph_retrieval.sql)
   - 文件：[`internal/kb/types.go`](../../internal/kb/types.go)
   - 测试：memory store graph expansion。

2. Entity/relation extractor
   - 文件：[`internal/ingest/graph.go`](../../internal/ingest/graph.go)
   - 启发式实体抽取和 chunk 内共现关系。
   - 测试：英文/中文实体可生成 relation。

3. PostgreSQL graph repository
   - 文件：[`internal/storage/postgres/repository.go`](../../internal/storage/postgres/repository.go)
   - 方法：`StoreGraphRelations(ctx, relations)`、`ExpandGraph(ctx, request)`。
   - 测试：通过编译和现有 repository scan tests 覆盖。

4. Ingestion 接入
   - 文件：[`internal/ingest/service.go`](../../internal/ingest/service.go)
   - Store chunks 后抽取和持久化 graph；默认关闭。

5. Graph retriever
   - 文件：[`internal/kb/graph.go`](../../internal/kb/graph.go)
   - 输入 query，输出 SearchResult，From=`graph`。
   - 作为 decorator 包装 `kb.HybridRetriever`。

6. Query 路由联动
   - 文件：[`internal/app/app.go`](../../internal/app/app.go)
   - 当 `RAG_GRAPH_RETRIEVAL_ENABLED=true` 时启用，路由器可独立控制 single/multi 检索强度。
   - 测试：graph retriever 能追加相关 chunk。

7. 评估与文档
   - 文件：[`docs/evaluation.md`](../evaluation.md)
   - 文件：[`docs/architecture/rag-pipeline.md`](../architecture/rag-pipeline.md)
   - 增加两个跨实体关系样例，说明部署影响为 PostgreSQL 表和索引。

## 验收证据

- ingestion 可抽取并持久化实体/关系。
- query 可通过实体/关系扩展召回相关 chunks。
- 删除文档或知识库时图数据被清理。
- `go test ./internal/ingest ./internal/kb ./internal/storage/postgres ./internal/rag`
- 文档说明存储方案、索引构建流程、检索流程和部署影响。
