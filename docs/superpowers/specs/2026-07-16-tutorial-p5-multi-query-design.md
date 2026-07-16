# P5 固定 Multi-query 候选设计

## 目标

为 Text Quick Pack 提供实验性、Pack 声明的 `p5_multi_query_retrieval` 候选。它只能直接引用兼容 P0，复用 P0 已完成索引与数据集，并只增加服务器拥有的三路查询扩展及结果融合。

## 审计结论与决策

普通应用的 `rag.Service` 默认装配 Hybrid Retriever、语义缓存、Pipeline，并可由部署配置启用 Query Router、Query Rewrite、Multi-query 与 HyDE。教程 P0 使用 `realtime` profile；现有实现中 Rewrite、Multi-query 和 HyDE 仅在 `high_precision` profile 生效。因此不能把应用默认配置本身当作可复现教程基线。

从本阶段起，教程运行使用固定的 v2 评测基线：保留 Hybrid Dense+Sparse+RRF 检索与 P0 原始查询，但清除 Pipeline、语义缓存与 Query Router，并关闭 Rewrite、Multi-query、HyDE。该变化会提升 P0–P4 的可复现性；运行环境的 `EvaluatorVersion` 升级为 `tutorial_eval_v2`，所以新运行不会和旧 `standard_eval_v1` 运行形成比较。

P5 在同一 v2 基线上只开启 `MultiQueryCount=3` 和一个显式的 realtime multi-query 开关。它不启用 Rewrite 或 HyDE、不更换 profile、Top-K、模型、索引、parser 或 chunking。每一条扩展查询走同一 Hybrid Retriever，结果使用固定 RRF 合并。这样 P5 的唯一变量是 multi-query expansion。

替代方案及取舍：

- 仅把 P5 设为 `high_precision` 被拒绝：它同时改变 profile prompt，并隐式开启 Rewrite/HyDE，无法归因。
- 直接复用应用默认 RAG 被拒绝：部署环境可改变 Router、缓存和扩展配置，且无法稳定复现。
- 新建一次候选索引被拒绝：会引入入库变量；P5 是查询期实验，必须复用 P0 索引。

## 运行时与持久化契约

新增候选必须严格为：

```json
{
  "id": "p5_multi_query_retrieval",
  "chapter": "p5_multi_query_retrieval",
  "parser_method": "basic",
  "chunk_size_tokens": 800,
  "chunk_overlap_tokens": 120,
  "retrieval_strategy": "hybrid",
  "reuse_baseline_index": true,
  "multi_query_count": 3
}
```

`RuntimeCandidate`、公开变体、运行记录和 definition fingerprint 都增加 `multi_query_count`。运行记录还保存 `query_expansion_mode`：P0–P4 为 `none`，P5 为 `multi_query`。迁移默认值为 `none`/`0`，以保护历史行。P5 在 `run_evaluation` 排队，复制 P0 的测量索引事实；不读取 Pack 对象，也不进入 `index_private_pack`。

Comparison 仍要求直接 P0 父运行、同一 comparison fingerprint、同一知识库/数据集/profile/Top-K、已完成标准评测和 P0 v2 形状。P5 额外要求 `retrieval_strategy=hybrid`、`reused_baseline_index=true`、`query_expansion_mode=multi_query`、`multi_query_count=3`。索引事实相同只代表复用，而非质量结论。

## RAG 边界

`rag.Service` 增加一个仅供服务器配置的 `MultiQueryForRealtime` 布尔字段。`BuildRetrievalQueries` 在 `high_precision` 或该字段为真时生成多查询；Rewrite 和 HyDE 仍严格只在 `high_precision` 下执行。P5 evaluator 的 shallow clone 显式设置：

```go
Pipeline = nil
Cache = nil
QueryRouter = nil
QueryRewriteEnabled = false
HyDEEnabled = false
MultiQueryCount = 3
MultiQueryForRealtime = true
```

P0–P4 新运行使用同一无环境漂移的 v2 baseline clone，但 `MultiQueryCount=1` 且 `MultiQueryForRealtime=false`。P4 从该基线 clone 改为 sparse retriever；P5 保持 Hybrid Retriever。

## API、Console、回归与文档

OpenAPI 暴露 read-only `multi_query_count` 和 `query_expansion_mode`。Console 显示 P5 的“3 路固定查询扩展 / 复用 P0 索引”与审计字段；不允许用户设置扩展数量。受控 1.0.5 fixture、mock handlers 和真实浏览器 E2E 运行 P0→P5，验证审计信息、复用知识库及无机密泄露。

新增教程 Markdown 和 hosted static page；README、docs index、ROADMAP 与 CHANGELOG 明确 P0–P5 采用 v2 教程评测器，旧运行不跨版本比较。完整验证覆盖 Go 全量测试/vet、OpenAPI、Console unit/build、真实 PostgreSQL/Qdrant/browser walkthrough 和部署后的 hosted page。
