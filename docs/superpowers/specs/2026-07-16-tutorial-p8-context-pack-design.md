# Tutorial P8 Context Pack candidate design

## Goal

为 Text Quick Pack 提供实验性、Pack 声明的 `p8_context_pack` 候选。它只能直接引用兼容且完成的 P0 `baseline`，复用 P0 索引，并且只改变服务端拥有的上下文证据打包数量。

## Chosen contract

P8 固定为：

```json
{
  "id": "p8_context_pack",
  "chapter": "p8_context_pack",
  "parser_method": "basic",
  "chunk_size_tokens": 800,
  "chunk_overlap_tokens": 120,
  "retrieval_strategy": "hybrid",
  "reuse_baseline_index": true,
  "context_pack_top_n": 3,
  "context_pack_max_tokens": 6000
}
```

P0–P7 的 tutorial evaluator v5 统一固定 `ContextPacker{TopN: 5, MaxTokens: 6000}`；P8 从相同隔离 evaluator clone 只把 `TopN` 改为 `3`。P0 的 Quick Pack Top-K 已固定为 5，因此基线最多把五个常规检索结果打包进入模型；P8 最多打包前三个。两者都沿用相同 6000 token 上限、相同 hybrid 检索器、数据集、profile 与 Top-K。

不使用运行环境的 `RAG_CONTEXT_TOP_N` 或 `RAG_MAX_CONTEXT_TOKENS`：这些部署配置会改变未记录的实验变量。固定值和 evaluator v5 指纹让旧运行可读但不与新运行比较。

## Isolation and lineage

P8 和 P4–P6 一样复用兼容 P0 知识库和实测索引事实，创建后直接进入 `run_evaluation`，不读取 Pack 对象也不建立第二个索引。它只能选择 app 注册的 P8 evaluator；浏览器不能提交 Packer、上下文预算、Top-N、检索器、缓存、模型或知识库参数。

v5 延续 v4 的隔离：显式 hybrid 基础检索器、无 Pipeline、语义缓存和 Query Router，GraphBuilder/RAPTOR 关闭，rewrite/HyDE 关闭，P0–P5/P7 禁用 rerank，P5 仍是唯一启用三路 multi-query 的候选，P6 仍是唯一启用 rerank 的候选，P7 仍是唯一图检索候选。P8 只改变 Packer.TopN。

## Durable audit and comparison

`RuntimeCandidate`、公开变体、持久化运行和 definition fingerprint 增加只读 `context_pack_top_n` 与 `context_pack_max_tokens`。迁移为历史行默认 `0`；v5 P0–P7 都记录 `5`/`6000`，P8 记录 `3`/`6000`。这些字段描述打包配置，不宣称实际 token 消耗、延迟、成本或质量。

比较要求直接 P0/P8、同一 v5 comparison fingerprint、同一知识库/数据集/profile/Top-K、正常标准评测 Run、P0 `hybrid`/5/6000 与 P8 `hybrid`/3/6000、两者都复用 P0 索引且 P8 没有其它候选开关。`index_metrics` 相同仅说明索引被继承，不能解释 Context Pack 效果。

## Surface and verification

OpenAPI 在公开变体与运行中暴露只读 Context Pack 字段；Console 显示其实际值与 P8 的“复用 P0 索引、仅缩小证据包”说明，不提供配置控件。受控 fixture 从 `1.0.7` 复制为 `1.0.8`；真实 PostgreSQL + Qdrant + Console walkthrough 运行 P0→P8，并断言 v5 审计字段、P0 索引复用和可比结果。

测试还必须证明：基线 Packer 看到五个候选时产出五条 citation，P8 Packer 在相同结果上只产出前三条；P8 比较拒绝改变的预算、Top-N、索引复用、检索策略或其他实验开关。文档、CHANGELOG、ROADMAP 与托管静态页明确不对质量、成本或延迟做推断。
