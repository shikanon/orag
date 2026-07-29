# P4：稀疏检索候选

`p4_sparse_retrieval` 是文本 Quick Pack 的实验性、Pack 声明候选。它只能直接引用完成且兼容的 P0 `baseline`；不会继承 P1、P2 或 P3 的运行结果。

稀疏概率检索的研究脉络可追溯到 [The Probabilistic Relevance Framework: BM25 and Beyond](https://www.nowpublishers.com/article/Details/INR-019)，但 P4 使用 PostgreSQL FTS，不是 BM25 公式的严格复现。

## 单变量边界

P4 保持 P0 的 `basic` parser、800/120 分块、评测集、`realtime` profile 与 Top-K。它不重新入库，而是复用兼容 P0 的知识库和已测量索引事实；唯一变化是评测时使用纯 `sparse` 检索器。服务端构造该检索器时会清空 Pipeline 和语义缓存，避免 P0 的 hybrid/图执行或缓存响应混入候选结果。

浏览器只能提交 `variant=p4_sparse_retrieval` 与幂等键，不能提交检索器、索引、模型、profile、Top-K、缓存设置或知识库坐标。

## 审计与比较

运行记录 `retrieval_strategy=sparse` 与 `reused_baseline_index=true`，同时复制其 P0 父运行的实际索引事实。这些字段说明 P4 没有第二次索引，不是性能、成本或质量声明。

只有直接 P0/P4、同一冻结运行环境、同一知识库、相同数据集/profile/Top-K、正常标准评测 Run，以及稀疏策略和 P0 索引复用声明均可验证时，比较才返回 `comparable=true`。`index_metrics` 的相同值表示继承的 P0 索引事实，并不说明检索效果相同。

## Pack 发布与回归

受控 `tests/fixtures/tutorial-packs/text-rag/1.0.4/quick` 仅供真实 PostgreSQL/Qdrant/浏览器回归临时映射。公开 `text-rag/1.1.0` 已包含 P1–P8 声明并完成匿名 HTTPS、MIME、长度和 SHA-256 验证；它不覆盖该 fixture。
