# P8：Context Pack 候选

`p8_context_pack` 是 Text Quick Pack 的实验性、Pack 声明候选。它只能直接引用完成且兼容的 P0 `baseline`；不会继承 P1–P7 的结果。

P8 保持 P0 的 `basic` parser、800/120 分块、hybrid 检索、评测集、`realtime` profile 与 Top-K，并复用相同 P0 知识库和实测索引事实。P0–P7 的 tutorial evaluator v5 固定打包最多 5 条证据、6000 个文本单元；P8 仅固定为最多 3 条证据、相同 6000 个文本单元。

教程 evaluator v5 显式使用 hybrid 基础检索器，并移除 Pipeline、语义缓存和 Query Router，关闭 GraphBuilder、RAPTOR、rewrite 与 HyDE。P8 只替换 `ContextPacker.TopN`；P5、P6 和 P7 的固定 multi-query、rerank 和 graph retrieval 例外保持不变。生产服务保持自身运行时配置，不受教程隔离影响。

浏览器只能提交 `variant=p8_context_pack` 与幂等键，不能提交 Packer、上下文预算、模型、检索器、索引、profile、Top-K、缓存或知识库坐标。运行记录 `context_pack_top_n=3`、`context_pack_max_tokens=6000`、`retrieval_strategy=hybrid`、`reused_baseline_index=true`、`query_expansion_mode=none`、`multi_query_count=0`、`rerank_enabled=false` 与 `graph_retrieval_enabled=false`；这些是配置与执行事实，不是实际 token 消耗、质量、成本或延迟声明。

比较只在直接 P0/P8、同一 tutorial evaluator v5 指纹、同一知识库/数据集/profile/Top-K、正常标准评测 Run、P0 `context_pack_top_n=5`/`context_pack_max_tokens=6000` 与 P8 `3`/`6000` 均可验证时返回 `comparable=true`。相同 `index_metrics` 表示 P0 索引被继承，不能解释 Context Pack 效果。

受控 `tests/fixtures/tutorial-packs/text-rag/1.0.8/quick` 仅供真实 PostgreSQL/Qdrant/浏览器回归临时映射。公开 `text-rag/1.1.0` 已包含 P1–P8 声明并完成匿名 HTTPS、MIME、长度和 SHA-256 验证；它不覆盖该 fixture。
