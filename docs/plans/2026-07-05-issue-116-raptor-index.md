# Issue 116 RAPTOR 递归分层摘要索引技术方案

## 背景与目标

Issue: https://github.com/shikanon/orag/issues/116

当前检索以扁平 chunk 为基本粒度，适合局部事实查询。跨段综合、全局概括、多跳问题需要更高层的语义节点。目标是构建 RAPTOR-style 多层摘要树，让检索可同时命中原始 chunk 层和摘要层。

## 当前代码落点

- [`internal/ingest/service.go`](../../internal/ingest/service.go): 适合在原始 chunk 构建后追加层级摘要 chunk。
- [`internal/ingest/raptor.go`](../../internal/ingest/raptor.go): RAPTOR-style 递归摘要 builder。
- [`internal/kb/types.go`](../../internal/kb/types.go): `Chunk.Metadata` 可表达 summary node 与 child chunk 关系。
- [`internal/storage/postgres/repository.go`](../../internal/storage/postgres/repository.go): 复用 chunks 表持久化 summary chunk。
- [`internal/storage/qdrant/payload.go`](../../internal/storage/qdrant/payload.go): 复用同一 vector collection 写入 summary chunk payload。

## 设计决策

1. 对外 `SearchResult` 继续返回 `kb.Chunk`，摘要节点通过 `Chunk.Metadata["kind"]="raptor_summary"` 标记。
2. 摘要 chunk 写入现有 chunks 表和 Qdrant collection，不新增单独 RAPTOR 表。
3. 层级关系通过 metadata 持久化：`level` 和 `child_chunk_ids`。
4. 初版聚类采用 deterministic contiguous grouping，不引入新依赖：按 `INGEST_RAPTOR_BRANCH_FACTOR` 顺序分组；后续可替换为 embedding 聚类。
5. 摘要生成使用当前 model Chat。失败时返回 ingestion error；默认关闭以避免额外模型成本。
6. 检索时摘要 chunk 与原始 chunk 同时参与 dense 和 sparse 检索，因此无需额外 retriever。

## 配置

新增 `Ingestion.RAPTOR` 配置：

- `INGEST_RAPTOR_ENABLED=false`
- `INGEST_RAPTOR_BRANCH_FACTOR=4`
- `INGEST_RAPTOR_MAX_LEVELS=2`
- `INGEST_RAPTOR_MAX_SUMMARY_CHARS=1000`

默认关闭，保证现有查询行为不变。

## 开发拆解

1. 数据结构
   - 文件：[`internal/kb/types.go`](../../internal/kb/types.go)
   - 摘要 chunk metadata 存储 `kind`、`level`、`child_chunk_ids`。

2. RAPTOR builder
   - 文件：[`internal/ingest/raptor.go`](../../internal/ingest/raptor.go)
   - 输入：document、chunks、summarizer。
   - 输出：summary chunks。
   - 测试：固定 chunks，branch_factor=2，生成两层摘要；child_chunk_ids 可追踪。

3. PostgreSQL/Qdrant 持久化
   - 文件：[`internal/storage/postgres/repository.go`](../../internal/storage/postgres/repository.go)
   - 文件：[`internal/storage/qdrant/payload.go`](../../internal/storage/qdrant/payload.go)
   - 摘要 chunk 与普通 chunk 一起写入，payload/metadata 包含 `kind`、`level`、`child_chunk_ids`。
   - 测试：payload encode/decode。

4. Ingestion 接入
   - 文件：[`internal/ingest/service.go`](../../internal/ingest/service.go)
   - 原始 chunks 构建后追加 RAPTOR summary chunks，一起 embedding 和 Store。

5. 文档
   - 文件：[`docs/architecture/rag-pipeline.md`](../architecture/rag-pipeline.md)
   - 说明成本和适用场景。

## 验收证据

- ingestion 可构建多层摘要 chunk，并通过 metadata 持久化层级关系。
- query 可召回原始 chunk 和 summary chunk。
- `go test ./internal/ingest ./internal/kb ./internal/storage/postgres ./internal/storage/qdrant ./internal/rag`
- 文档说明配置、成本、限制与评估方式。
