# 评估文档

本目录面向质量评估、算法调参和回归门禁维护。ORAG 当前评估模块复用线上 RAG 查询路径，避免线上线下漂移。

## 当前能力

| 能力 | 说明 |
| --- | --- |
| 数据集 | 支持创建数据集和写入样本。 |
| 评估运行 | `POST /v1/evaluations` 对数据集样本逐条调用 RAG 查询。 |
| 结果持久化 | 默认 `qdrant_postgres` 后端会写入 PostgreSQL。 |
| 指标 | 当前是 deterministic rule-based metrics。 |
| Optimizer | 对候选 `profiles` 和 `top_ks` 做确定性网格搜索。 |

## 数据模型

| 表或概念 | 说明 |
| --- | --- |
| `datasets` | 数据集元信息，包含 `kind` 和 `version`。 |
| `dataset_items` | 样本，包含 `query`、`ground_truth`、`relevant_doc_ids`。 |
| `evaluation_runs` | 一次评估运行的汇总结果。 |
| `evaluation_results` | 每个样本的答案和逐样本指标。 |

数据集样本写入、评估运行和 optimizer 都会先按当前 tenant 校验 `dataset_id`。数据集不存在或属于其他 tenant 时返回 `404 dataset_not_found`，不会写入样本或评估结果。

`GET /v1/evaluations/{id}` 当前查询的是运行级汇总，不返回逐样本明细。

## 指标边界

| 指标 | 当前计算方式 | 注意事项 |
| --- | --- | --- |
| `answer_accuracy` | 答案包含 ground truth 关键项时命中。 | 弱规则指标，不把 citation 存在性计入答案正确。 |
| `accuracy` / `hit_rate` | 新运行中与 `answer_accuracy` 保持一致。 | 兼容别名；历史已存运行可能没有新增指标键。 |
| `pairwise_accuracy` | 优化器主质量指标；当前未接入 pairwise judge 时由 `answer_accuracy` 填充。 | 推荐作为候选排序主指标，但当前仍是规则命中率语义，不等价于人工或 LLM 成对评审。 |
| `citation_hit_rate` | 响应中存在至少一个 citation 时命中。 | 只说明证据存在性，不证明答案正确。 |
| `context_recall` | 检查 retrieved chunks 覆盖相关文档 ID 的比例。 | 只看文档 ID，不验证 chunk 内容是否真正支撑答案。 |
| `citation_precision` | 检查引用文档 ID 是否落在相关文档列表中。 | 不验证引用位置与回答论断的一致性。 |
| `ndcg_at_k` | 衡量相关文档在前 `top_k` 召回结果中的排名质量。 | 依赖 `relevant_doc_ids`，缺失标注时为 0。 |
| `recall_at_k` | 衡量前 `top_k` 覆盖相关文档的比例。 | 依赖 `relevant_doc_ids`，重复命中同一文档只计一次。 |
| `mrr` | 第一个相关文档的 reciprocal rank。 | 依赖 `relevant_doc_ids`，无相关召回时为 0。 |
| `map` | 对相关文档命中位置的 precision 做平均。 | 依赖 `relevant_doc_ids`，未标注时为 0。 |
| `coverage` | 样本是否至少召回一个相关文档。 | 依赖 `relevant_doc_ids`，运行级为样本平均。 |
| `retrieval_failure_rate` | 标注了相关文档但没有召回任何相关文档时记为失败。 | 未标注 `relevant_doc_ids` 时为 0，避免误报失败。 |
| `redundancy_rate` | 重复召回结果比例。 | 不依赖人工标注，重复判定基于 chunk ID、hash/dedupe key 或规范化文本。 |
| `duplicate_count` | 重复召回结果数量。 | 无召回结果时为 0。 |
| `deduped_top_k_count` | 去重后的召回结果数量。 | 用于判断 top_k 是否被重复内容浪费。 |
| `alpha_ndcg` | 多样性敏感 NDCG，对重复覆盖同一 aspect/subquestion 的收益做衰减。 | 依赖 `diversity_annotations`，缺少有效标注时跳过。 |
| `aspect_coverage` | 召回证据覆盖 aspect/subquestion 的比例。 | 依赖 `diversity_annotations`，缺少有效标注时跳过。 |
| `latency_p95_ms` | 本次评估内样本查询延迟 P95。 | 来自 RAG 响应的 `LatencyMS`。 |
| `cache_hit_rate` | `CacheStatus == "hit"` 的样本比例。 | 依赖语义缓存状态。 |

缺失标注行为：

- `relevant_doc_ids` 缺失时，IR 排序指标为 0；`context_recall` 退化为是否有召回结果，`citation_precision` 在存在 citation 时为 1。
- `diversity_annotations` 缺失或没有有效 aspect/subquestion 绑定时，`alpha_ndcg` 和 `aspect_coverage` 不写入逐样本指标，也不参与运行级聚合。
- `latency_p95_ms`、`cache_hit_rate` 和冗余度指标不依赖人工标注，可用于无 golden 标注的冒烟检查。

## Optimizer 流程

```text
profiles x top_ks
        |
        v
run evaluation for each candidate
        |
        v
score = metrics.pairwise_accuracy
        |
        v
return candidates + best
```

当历史运行缺少 `pairwise_accuracy` 时，optimizer 会回退到 `answer_accuracy`，再回退到 `run.Accuracy`。

当前 optimizer 的边界：

- 只优化 `profile` 和 `top_k`。
- 不自动调整 prompt、embedding、reranker、chunk 策略、模型或索引参数。
- 不做 Bayesian optimization、bandit、早停或成本约束搜索。
- optimization result 只在本次响应中返回；可追溯数据来自候选关联的 `run_id`。
- 候选排序使用 `pairwise_accuracy`；Recall@k、NDCG@k、MRR、MAP、失败率、冗余度、多样性和 `latency_p95_ms` 是诊断字段。

## 延迟权衡

复杂召回策略通常会提升 `pairwise_accuracy`、`ndcg_at_k` 或 `recall_at_k`，但也可能提高 `latency_p95_ms`。推荐用 `pairwise_accuracy` 做主排序，再用 `latency_p95_ms` 作为 SLO 约束：若质量提升明显且 P95 仍达标，可以选择 `high_precision` 或更大的 `top_k`；若质量收益很小但 P95 明显上升，应优先保留更简单的 `realtime` profile 或较小 `top_k`。

## 推荐使用方式

| 场景 | 推荐做法 |
| --- | --- |
| PR 回归 | 准备小型 deterministic 数据集，观察 `answer_accuracy`、`pairwise_accuracy`、IR 指标、冗余度、多样性和 `latency_p95_ms` 是否退化。 |
| profile 对比 | 使用相同数据集跑 `realtime` 和 `high_precision`。 |
| top_k 调参 | 用 optimizer 枚举少量候选，优先看 `pairwise_accuracy`，再用 `latency_p95_ms` 和冗余度做约束。 |
| 真实质量评审 | 结合人工检查或后续 LLM-as-Judge，不要只看当前 rule-based `answer_accuracy`。 |

## 后续增强方向

后续可在保持 deterministic 门禁的基础上，增加可选 Ark LLM-as-Judge：

- `faithfulness`：答案是否被检索证据支撑。
- `groundedness`：回答是否避免未引用事实。
- `answer_relevance`：答案是否真正回应用户问题。
- `judge_explanation`：保留 judge 理由、prompt 版本和模型版本，避免跨版本不可比。
