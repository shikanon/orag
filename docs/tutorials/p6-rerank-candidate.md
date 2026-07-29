# P6：Rerank 候选

`p6_rerank_retrieval` 是 Text Quick Pack 的实验性、Pack 声明候选。它只能直接引用完成且兼容的 P0 `baseline`；不会继承 P1–P5 的结果。

“先召回、再用模型重排”的代表性研究见 [Document Ranking with a Pretrained Sequence-to-Sequence Model (monoT5)](https://aclanthology.org/2020.findings-emnlp.63/)。P6 调用 ORAG provider registry 中配置的 reranker，并不表示使用或复现 monoT5。

P6 保持 P0 的 `basic` parser、800/120 分块、hybrid 检索、评测集、`realtime` profile 与 Top-K，并复用相同 P0 知识库和测量索引事实。唯一变化是启用服务端固定的既有 reranker。

教程 evaluator v5 对 P0–P5 显式禁用 rerank，并显式使用 hybrid retriever，同时固定 Context Pack 为 5/6000、移除 Pipeline、语义缓存和 Query Router，关闭 GraphBuilder、RAPTOR、rewrite 与 HyDE。P6 仅将 `DisableRerank` 设为 false；生产服务保留原有零值 rerank 行为，不受此教程隔离开关影响。

浏览器只能提交 `variant=p6_rerank_retrieval` 与幂等键，不能提交 reranker、模型、Top-N、检索器、索引、profile、Top-K、缓存或知识库坐标。运行记录 `retrieval_strategy=hybrid`、`reused_baseline_index=true`、`query_expansion_mode=none`、`multi_query_count=0` 与 `rerank_enabled=true`；这些都是执行事实，不是质量、成本或延迟声明。

比较只在直接 P0/P6、同一 tutorial evaluator v5 指纹、同一知识库/数据集/profile/Top-K、正常标准评测 Run、P0 `rerank_enabled=false` 与 P6 `rerank_enabled=true` 均可验证时返回 `comparable=true`。相同 `index_metrics` 表示 P0 索引被继承，不能解释为 rerank 效果。

受控 `tests/fixtures/tutorial-packs/text-rag/1.0.6/quick` 仅供真实 PostgreSQL/Qdrant/浏览器回归临时映射。公开 `text-rag/1.1.0` 已包含 P1–P8 声明并完成匿名 HTTPS、MIME、长度和 SHA-256 验证；它不覆盖该 fixture。
