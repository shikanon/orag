# P5：Multi-query 检索候选

`p5_multi_query_retrieval` 是文本 Quick Pack 的实验性、Pack 声明候选。它只能直接引用完成且兼容的 P0 `baseline`；不会继承 P1–P4 的结果。

P5 保持 P0 的 `basic` parser、800/120 分块、hybrid 检索器、评测集、`realtime` profile 与 Top-K，并复用同一个已完成 P0 知识库及其索引事实。唯一变化是在教程专用 evaluator 中固定生成三条检索查询：原始查询和最多两条模型生成的去重查询。

浏览器只能提交 `variant=p5_multi_query_retrieval` 与幂等键，不能提交查询数量、rewrite、HyDE、检索器、模型、profile、Top-K、缓存或知识库坐标。P5 evaluator 会清空 Pipeline、语义缓存和 Query Router，关闭 query rewrite 与 HyDE；这避免生产默认开关和缓存响应改变候选含义。

运行记录 `retrieval_strategy=hybrid`、`reused_baseline_index=true`、`query_expansion_mode=multi_query` 与 `multi_query_count=3`。这些字段描述执行定义与索引复用，不构成质量、成本或延迟结论。若模型无法生成足够的去重查询，评测的普通 warnings 会保留该事实。

比较只在直接 P0/P5、同一教程 evaluator v2 指纹、同一知识库/数据集/profile/Top-K、正常标准评测 Run、P0 `query_expansion_mode=none`，以及 P5 的三条 multi-query 声明都可验证时返回 `comparable=true`。相同的 `index_metrics` 表示 P0 索引被继承，不能解释为检索质量相同。

受控 `tests/fixtures/tutorial-packs/text-rag/1.0.5/quick` 仅供真实 PostgreSQL/Qdrant/浏览器回归临时映射；它不会覆盖公开 `1.0.0` Pack。正式公开 Pack 仍须经过独立的匿名 HTTPS、MIME、长度和 SHA-256 发布流程。
