# P7：图检索候选

`p7_graph_retrieval` 是 Text Quick Pack 的实验性、Pack 声明候选。它只能直接引用完成且兼容的 P0 `baseline`；不会继承 P1–P6 的结果。

P7 保持 P0 的 `basic` parser、800/120 分块、评测集、`realtime` profile 与 Top-K，但会创建独立候选 Knowledge Base。候选入库只启用服务端固定的轻量图关系构建器；评测只把同一 hybrid 基础检索器包装为 `GraphRetriever`，在常规召回后按查询实体扩展图结果。

教程 evaluator v4 对 P0–P6 显式使用 app 创建的 hybrid retriever，并关闭 GraphBuilder、RAPTOR、Pipeline、语义缓存和 Query Router，同时关闭 rewrite、HyDE、rerank 和 realtime multi-query。P7 仅启用固定 GraphBuilder 与 GraphRetriever；生产服务的图开关保持原有行为，不受教程隔离影响。

浏览器只能提交 `variant=p7_graph_retrieval` 与幂等键，不能提交图实体数、图存储、模型、检索器、索引、profile、Top-K、缓存或知识库坐标。运行记录 `retrieval_strategy=graph`、`graph_retrieval_enabled=true`、`reused_baseline_index=false`、`query_expansion_mode=none`、`multi_query_count=0` 与 `rerank_enabled=false`；这些都是执行事实，不是质量、关系数量、成本或延迟声明。

比较只在直接 P0/P7、同一 tutorial evaluator v4 指纹、同一数据集/profile/Top-K、不同知识库、正常标准评测 Run、P0 `graph_retrieval_enabled=false` 与 P7 `graph_retrieval_enabled=true` 均可验证时返回 `comparable=true`。`index_metrics` 只展示实际候选索引事实，不能单独解释图检索效果。

受控 `tests/fixtures/tutorial-packs/text-rag/1.0.7/quick` 仅供真实 PostgreSQL/Qdrant/浏览器回归临时映射；不会覆盖公开 `1.0.0` Pack。正式公开 Pack 仍须经过独立匿名 HTTPS、MIME、长度和 SHA-256 发布流程。
