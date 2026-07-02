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
| `citation_hit_rate` | 响应中存在至少一个 citation 时命中。 | 只说明证据存在性，不证明答案正确。 |
| `context_recall` | 检查 retrieved chunks 覆盖相关文档 ID 的比例。 | 只看文档 ID，不验证 chunk 内容是否真正支撑答案。 |
| `citation_precision` | 检查引用文档 ID 是否落在相关文档列表中。 | 不验证引用位置与回答论断的一致性。 |
| `latency_p95_ms` | 本次评估内样本查询延迟 P95。 | 来自 RAG 响应的 `LatencyMS`。 |
| `cache_hit_rate` | `CacheStatus == "hit"` 的样本比例。 | 依赖语义缓存状态。 |

## Optimizer 流程

```text
profiles x top_ks
        |
        v
run evaluation for each candidate
        |
        v
score = run.Metrics["answer_accuracy"]
        |
        v
return candidates + best
```

当历史运行缺少 `answer_accuracy` 时，optimizer 会回退到 `run.Accuracy`。

当前 optimizer 的边界：

- 只优化 `profile` 和 `top_k`。
- 不自动调整 prompt、embedding、reranker、chunk 策略、模型或索引参数。
- 不做 Bayesian optimization、bandit、早停或成本约束搜索。
- optimization result 只在本次响应中返回；可追溯数据来自候选关联的 `run_id`。

## 推荐使用方式

| 场景 | 推荐做法 |
| --- | --- |
| PR 回归 | 准备小型 deterministic 数据集，观察 `answer_accuracy`、`citation_hit_rate`、`context_recall`、`citation_precision` 是否退化。 |
| profile 对比 | 使用相同数据集跑 `realtime` 和 `high_precision`。 |
| top_k 调参 | 用 optimizer 枚举少量候选，避免大规模搜索拖慢本地验证。 |
| 真实质量评审 | 结合人工检查或后续 LLM-as-Judge，不要只看当前 rule-based `answer_accuracy`。 |

## 后续增强方向

后续可在保持 deterministic 门禁的基础上，增加可选 Ark LLM-as-Judge：

- `faithfulness`：答案是否被检索证据支撑。
- `groundedness`：回答是否避免未引用事实。
- `answer_relevance`：答案是否真正回应用户问题。
- `judge_explanation`：保留 judge 理由、prompt 版本和模型版本，避免跨版本不可比。
