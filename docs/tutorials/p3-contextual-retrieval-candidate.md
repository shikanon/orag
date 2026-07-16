# P3：上下文化检索候选

`p3_contextual_retrieval` 是文本 Quick Pack 的实验性、Pack 声明候选。它只能直接引用完成且兼容的 P0 `baseline`；不会继承 P1 解析或 P2 分块。

## 单变量边界

P3 保持 P0 的 `basic` parser 和 800/120 递归分块，仅为每个 chunk 生成服务器拥有的上下文。固定提示词、长度上限、聊天模型和 `fail` 策略由服务端构建；生成失败会使运行失败，绝不把未上下文化的索引误标为 P3。P0/P1/P2 明确不启用上下文化。

浏览器只能提交 `variant=p3_contextual_retrieval` 与幂等键，不能提交提示词、模型、长度、失败策略、上下文内容或知识库坐标。

## 审计与比较

运行记录 `contextual_retrieval_enabled`、`contextualized_chunk_count` 与 `average_context_tokens`。后两项从实际入库返回的非空 `ContextualText` 测量，不是质量、成本、延迟或 token 用量声明。

只有直接 P0/P3、相同冻结运行环境、正常评测 Run、P0 基线形状和 P3 实际上下文化事实均成立时，比较才返回 `comparable=true`。`index_metrics` 额外显示 `contextualized_chunk_count` 和 `average_context_tokens`，不推断优劣。

## Pack 发布与回归

受控 `tests/fixtures/tutorial-packs/text-rag/1.0.3/quick` 仅供真实 PostgreSQL/Qdrant/浏览器回归临时映射。公开 `text-rag/1.1.0` 已包含 P1–P8 声明并完成匿名 HTTPS、MIME、长度和 SHA-256 验证；它不覆盖该 fixture。
