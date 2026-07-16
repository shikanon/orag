# P6 固定 Rerank 候选设计

## 目标

为 Text Quick Pack 提供实验性、Pack 声明的 `p6_rerank_retrieval` 候选。它只能直接引用兼容 P0，复用 P0 已完成索引和数据集，并只启用服务器拥有的现有 reranker。

## 审计结论与决策

现有 `rag.Service.Execute` 在检索有结果后无条件调用 `ApplyRerank`。因此当前 tutorial evaluator v2 的 P0–P5 都已 rerank；若直接添加一个“启用 rerank”的 P6，无法形成单变量实验。

教程 evaluator 升级为 v3。它保留 v2 的隔离：无 Pipeline、语义缓存、Query Router、Rewrite、HyDE；P0–P5 还显式设置 `DisableRerank=true`。P6 从同一 clone 只将该字段设回 `false`。生产服务保持零值行为，仍执行 rerank，因此不会因教程改动改变线上默认链路。

替代方案：

- 让 P6 更换为 high-precision profile 被拒绝：这会改变 profile prompt，并重新引入 rewrite、HyDE 和 multi-query。
- 让 P6 使用新的索引被拒绝：rerank 是查询期排序实验，不应引入入库变量。
- 保持 P0–P5 当前 rerank 并仅记录 P6 被拒绝：这无法证明 P6 改变了任何执行行为。

## 运行时与持久化契约

P6 必须严格为：

```json
{
  "id": "p6_rerank_retrieval",
  "chapter": "p6_rerank_retrieval",
  "parser_method": "basic",
  "chunk_size_tokens": 800,
  "chunk_overlap_tokens": 120,
  "retrieval_strategy": "hybrid",
  "reuse_baseline_index": true,
  "rerank_enabled": true
}
```

`RuntimeCandidate`、公开变体、运行记录和 definition fingerprint 增加 `rerank_enabled`。迁移默认 `false`，P0–P5 的 v3 新运行都记录 `false`，P6 记录 `true`。P6 直接进入 `run_evaluation`，复制 P0 的索引事实，不读取 Pack 对象也不进入 `index_private_pack`。

EvaluatorVersion 从 `tutorial_eval_v2` 升至 `tutorial_eval_v3`，使旧 v2 运行保持可读、但永不与 v3 运行比较。比较仍要求直接 P0 父运行、相同 fingerprint、知识库、数据集、profile、Top-K、v3 P0 形状与完成评测。P6 额外要求 hybrid、P0 索引复用、无 multi-query 和 `rerank_enabled=true`；P1–P5 都要求 `rerank_enabled=false`。相同 index metrics 仅代表索引复用。

## RAG 边界

`rag.Service` 增加 server-owned `DisableRerank bool`。`ApplyRerank` 在其为真时原样返回检索结果，且不调用模型的 rerank API；零值继续保留生产 rerank 语义。v3 baseline clone 设置该字段为真；P6 clone 设置为假。P5 的 realtime multi-query 保持三路扩展，但其 rerank 仍禁用，因此它仍只改变 query expansion。

## API、Console、回归与文档

OpenAPI 增加只读 `rerank_enabled` 到公开变体和运行。Console 基于 API ID/chapter 标识 P6，显示 rerank 与 P0 索引复用事实，不提供任何 rerank 开关或参数。受控 1.0.6 fixture、mock handlers 和真实浏览器 E2E 运行 P0→P6，验证 rerank 审计和可比较结果。

新增 P6 Markdown 与 hosted static page；README、ROADMAP、CHANGELOG 说明 v3 重新定义 P0–P5 为“无 rerank”基线。所有实现必须通过 Go 全量测试/vet、OpenAPI、Console unit/build、真实 PostgreSQL/Qdrant/browser walkthrough 及 hosted-page curl 校验。
