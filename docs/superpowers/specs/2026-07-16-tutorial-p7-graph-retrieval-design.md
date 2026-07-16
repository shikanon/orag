# P7 轻量 Graph Retrieval 候选设计

## 目标

为 Text Quick Pack 提供实验性、Pack 声明的 `p7_graph_retrieval`。它是 P0 的直接子实验：保持 Basic/800/120、realtime、hybrid 基础检索、数据集、模型和 Top-K，只增加入库期轻量实体关系与查询期 GraphRetriever 扩展。

## 审计结论与决策

现有 `kb.GraphRetriever` 在基础检索结果之外，按查询实体向 `GraphStore` 扩展关联 chunk。关联由 `ingest.LightweightGraphBuilder` 在入库后写入。P0 索引不能被假设拥有图关系：它受部署时 `RAG_GRAPH_RETRIEVAL_ENABLED` 控制，且默认关闭。因此 P7 必须建立独立候选知识库；图关系生成与图查询是同一模块不可分割的执行单元。

教程 evaluator 升级到 v4。它必须显式使用 app 建立的 hybrid retriever，而不是 shallow-copy `ragSvc.Retriever`；这样部署环境启用 GraphRetriever 时不会悄然污染 P0–P6。v4 保留 P6 的无 Pipeline、缓存、路由、Rewrite、HyDE、rerank 和 realtime multi-query 基线。P7 evaluator 把同一个 hybrid retriever包装为 `kb.GraphRetriever`，并使用项目存储的 `GraphStore`。

替代方案：

- RAPTOR 被拒绝作为本阶段实现：它需要模型生成摘要，改变索引内容和模型调用成本，适合单独的后续候选。
- P0 索引复用被拒绝：不能证明现有 P0 有 GraphRelation，也不能在不改写 P0 的情况下补齐关系。
- 直接使用生产 `ragSvc.Retriever` 被拒绝：部署配置可能开启 graph，破坏 P0 及已存在候选的单变量语义。

## 运行时与持久化契约

P7 严格声明：

```json
{
  "id": "p7_graph_retrieval",
  "chapter": "p7_graph_retrieval",
  "parser_method": "basic",
  "chunk_size_tokens": 800,
  "chunk_overlap_tokens": 120,
  "retrieval_strategy": "graph",
  "graph_retrieval_enabled": true
}
```

P7 不复用 P0 索引，使用独立候选 KB；P0–P6 `graph_retrieval_enabled=false`。运行、公开变体和 definition fingerprint 保存该布尔值；迁移默认 false。P7 正常执行 `index_private_pack`，随后评测。它的实际 chunk metrics 仍来自候选索引；不把关系数量误写成质量或成本结论。

比较要求直接 P0 父运行、同一 comparison fingerprint、数据集/profile/Top-K、完成的标准评测、P0 v4 hybrid/no-graph 形状，以及 P7 Basic/800/120、独立 KB、`retrieval_strategy=graph` 与 `graph_retrieval_enabled=true`。P7 索引事实与 P0 可以不同；这只表明独立入库。

## API、Console、验证与文档

OpenAPI 增加只读 `graph_retrieval_enabled`。Console 展示 P7 的“独立图索引 / 轻量实体图扩展”事实，不暴露实体上限或检索策略控制。受控 1.0.7 fixture、mock、真实 PostgreSQL/Qdrant/browser E2E 运行 P0→P7，验证独立 KB、graph 审计、普通索引阶段与可比较结果。

新增 P7 文档和 hosted page；README、ROADMAP 和 CHANGELOG 说明 evaluator v4 的 hybrid 基线钳制。所有改动通过 Go 全量/vet/OpenAPI、Console 和真实教程 E2E，并在合并后部署到 `www.tensorbytes.com/orag/`。
