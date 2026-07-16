# P2：递归分块 400/80 候选

`p2_recursive_400_80` 是文本 Quick Pack 的实验性、Pack 声明候选。它与 P1 是 P0 的平级子实验：只能引用同一项目中已完成且兼容的 P0 `baseline`，不会继承、要求或复用 P1。

## 单变量边界

P0 与 P1 的教程专用递归分块固定为 800 大小、120 重叠；P2 仅将该分块器固定为 400 大小、80 重叠。P2 仍使用 `basic` parser。模型、嵌入、重排、评测器、评测集、profile、Top-K、私有 Pack 和对象校验结果都由服务端冻结。通用入库环境变量不会改变教程实验的这些固定值。

每个候选都使用独立的项目知识库。浏览器只提交 `variant=p2_recursive_400_80` 与幂等键，不能提交 parser、chunk 大小、重叠、知识库或模型配置。

## 审计与比较

运行记录会持久化 `chunk_size_tokens`、`chunk_overlap_tokens`、`indexed_chunk_count` 和 `average_chunk_tokens`。后两项来自本次私有 Pack 入库实际返回的 chunk，不是模型 token、成本、延迟或质量估计。

比较端点在以下条件均可证明时返回 `comparable=true`：

- 候选的 `baseline_run_id` 直接指向完成的 P0；
- Pack、运行环境、数据集、profile、Top-K 和评测器 fingerprint 相同；
- P0/P2 的固定 parser 与分块形状符合声明，并且两边都有已持久化的索引事实和标准评测 Run。

`metrics` 只包含普通评测 Run 已保存的真实指标差异；`index_metrics` 单独包含 `chunk_count` 与 `average_chunk_tokens` 的绝对值和变化。它们描述索引形状，不构成 P2 的质量、成本或延迟结论。

## Pack 发布与回归

受控 `tests/fixtures/tutorial-packs/text-rag/1.0.2/quick` 使用超过 P0 大小的 JSON 语料，确保 800/120 与 400/80 会产生可测量的不同索引形状。真实浏览器回归会把该不可变 fixture 临时映射到本地测试目录。公开 `text-rag/1.1.0` 已作为包含 P1–P8 的独立版本发布；它不替换此 fixture。

后续官方 Pack 仍需通过独立发布流水线：新建语义版本目录，校验 Manifest 和对象 SHA-256、MIME、长度与匿名 HTTPS 读取，再更新 catalog。不得覆盖旧版本，也不得把对象存储凭证放入仓库或 Console。
