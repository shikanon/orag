# 评估文档

本目录面向质量评估、算法调参和回归门禁维护。ORAG 当前评估模块复用线上 RAG 查询路径，默认提供 deterministic rule-based metrics；请求携带 `judge`/`qag` 配置时会额外执行 LLM-as-Judge 和 QAG claim verification，避免线上线下漂移。

评估方法的研究背景可从 [RAGAS](https://arxiv.org/abs/2309.15217)、[LLM-as-a-Judge](https://arxiv.org/abs/2306.05685)、[G-Eval](https://arxiv.org/abs/2303.16634) 和 [QAFactEval](https://aclanthology.org/2022.naacl-main.187/) 继续追溯。完整论文索引和“研究依据不等于严格复现”的边界见[数据集与 RAG 方法研究依据](../research-references.md)；ORAG 指标的权威定义仍以本页和当前代码为准。

## 当前能力

| 能力 | 说明 |
| --- | --- |
| 数据集 | 支持创建数据集和写入样本。 |
| 评估运行 | `POST /v1/evaluations` 对数据集样本逐条调用 RAG 查询，支持 split 过滤和样本 weight 加权聚合。 |
| 结果持久化 | 默认 `qdrant_postgres` 后端会写入 PostgreSQL。 |
| 指标 | 默认写入 `deterministic_answer_match` 等 deterministic rule-based metrics；启用 Judge/QAG 时写入 faithfulness、groundedness、citation_support、qag_score、token/cost 等增强指标。 |
| LLM-as-Judge/QAG | 支持 pairwise judge、A/B 顺序交换、raw/parsed 响应分离、QAG claim verdict、gold-set 校准和成本记录。 |
| Optimizer | 支持目标驱动异步优化、objective/constraints/tie-breaker、搜索空间、checkpoint、cancel/resume 和 holdout 复评/门禁；旧 `profiles/top_ks` 请求仍兼容。 |

## 数据模型

| 表或概念 | 说明 |
| --- | --- |
| `datasets` | 数据集元信息，包含 `kind` 和 `version`。 |
| `dataset_items` | 样本，包含 `query`、`ground_truth`、`relevant_doc_ids`，并支持 `split`、`weight`、`expected_evidence`、`human_scores` 等评估元数据。 |
| `evaluation_runs` | 一次评估运行的汇总结果。 |
| `evaluation_results` | 每个样本的答案和逐样本指标。 |
| `judge_runs` / `judge_results` / `pairwise_judge_results` | 可选 Judge/QAG 运行、逐样本结果和 pairwise 比较明细。 |
| `judge_calibration_runs` | gold-set 校准结果，记录 Spearman、Cohen's kappa、QAG coverage 和 waiver。 |
| `optimization_runs` / `optimization_candidates` / `harness_runs` | 异步优化 run、候选、checkpoint、预算、状态、holdout 和 external harness 结果。 |

数据集样本写入、评估运行和 optimizer 都会先按当前 tenant 校验 `dataset_id`。数据集不存在或属于其他 tenant 时返回 `404 dataset_not_found`，不会写入样本或评估结果。

`GET /v1/evaluations/{id}` 默认查询运行级汇总；传 `include_items=true`、`include_judge=true`、`include_pairwise=true` 时返回逐样本、Judge/QAG 和 pairwise 明细。

## 指标边界

| 指标 | 当前计算方式 | 注意事项 |
| --- | --- | --- |
| `answer_accuracy` | 答案包含 ground truth 关键项时命中。 | 弱规则指标，不把 citation 存在性计入答案正确。 |
| `accuracy` / `hit_rate` | 新运行中与 `answer_accuracy` 保持一致。 | 兼容别名；历史已存运行可能没有新增指标键。 |
| `deterministic_answer_match` | 规则型答案匹配指标，替代过去 rule-only 运行写入 `pairwise_accuracy` 的行为。 | 适合作为未启用真实 pairwise judge 时的 fallback 排序信号。 |
| `pairwise_accuracy` | 真实 pairwise judge 中候选胜出或不输比例。 | 仅真实成对评审或历史兼容结果使用；新 rule-only 运行不再写入。 |
| `citation_hit_rate` | 响应中存在至少一个 citation 时命中。 | 只说明证据存在性，不证明答案正确。 |
| `context_recall` | 检查 retrieved chunks 覆盖相关文档 ID 的比例。 | 只看文档 ID，不验证 chunk 内容是否真正支撑答案。 |
| `citation_precision` | 检查引用文档 ID 是否落在相关文档列表中。 | 不验证引用位置与回答论断的一致性。 |
| `ndcg_at_k` | 衡量相关文档在前 `top_k` 召回结果中的排名质量；指标来源见 [Cumulated Gain-Based Evaluation of IR Techniques](https://doi.org/10.1145/582415.582418)。 | 依赖 `relevant_doc_ids`，缺失标注时为 0。 |
| `recall_at_k` | 衡量前 `top_k` 覆盖相关文档的比例。 | 依赖 `relevant_doc_ids`，重复命中同一文档只计一次。 |
| `mrr` | 第一个相关文档的 reciprocal rank。 | 依赖 `relevant_doc_ids`，无相关召回时为 0。 |
| `map` | 对相关文档命中位置的 precision 做平均。 | 依赖 `relevant_doc_ids`，未标注时为 0。 |
| `coverage` | 样本是否至少召回一个相关文档。 | 依赖 `relevant_doc_ids`，运行级为样本平均。 |
| `retrieval_failure_rate` | 标注了相关文档但没有召回任何相关文档时记为失败。 | 未标注 `relevant_doc_ids` 时为 0，避免误报失败。 |
| `redundancy_rate` | 重复召回结果比例。 | 不依赖人工标注，重复判定基于 chunk ID、hash/dedupe key 或规范化文本。 |
| `duplicate_count` | 重复召回结果数量。 | 无召回结果时为 0。 |
| `deduped_top_k_count` | 去重后的召回结果数量。 | 用于判断 top_k 是否被重复内容浪费。 |
| `alpha_ndcg` | 多样性敏感 NDCG，对重复覆盖同一 aspect/subquestion 的收益做衰减；来源见 [Novelty and Diversity in Information Retrieval Evaluation](https://doi.org/10.1145/1390334.1390446)。 | 依赖 `diversity_annotations`，缺少有效标注时跳过。 |
| `aspect_coverage` | 召回证据覆盖 aspect/subquestion 的比例。 | 依赖 `diversity_annotations`，缺少有效标注时跳过。 |
| `latency_p95_ms` | 本次评估内样本查询延迟 P95。 | 来自 RAG 响应的 `LatencyMS`。 |
| `cache_hit_rate` | `CacheStatus == "hit"` 的样本比例。 | 依赖语义缓存状态。 |
| `faithfulness` / `groundedness` / `citation_support` / `hallucination` / `completeness` | LLM-as-Judge 根据证据、答案和 rubric 输出的质量指标。 | 仅在请求包含 `judge` 时产生；主观维度应结合 gold-set 校准。 |
| `qag_score` / `qag_claim_coverage` / `qag_question_count` / `qag_unverifiable_rate` | QAG 基于答案 claim 生成问题、用 context-only answer 校验支撑情况后的指标。 | 仅在请求包含 `qag` 时产生；用于识别 contradicted/unverifiable claim 和关键 claim 漏检。 |

缺失标注行为：

- `relevant_doc_ids` 缺失时，IR 排序指标为 0；`context_recall` 退化为是否有召回结果，`citation_precision` 在存在 citation 时为 1。
- `diversity_annotations` 缺失或没有有效 aspect/subquestion 绑定时，`alpha_ndcg` 和 `aspect_coverage` 不写入逐样本指标，也不参与运行级聚合。
- `latency_p95_ms`、`cache_hit_rate` 和冗余度指标不依赖人工标注，可用于无 golden 标注的冒烟检查。

## Optimizer 流程

```text
POST /v1/optimizations
        |
        v
create queued run + candidate set
        |
        v
async worker evaluates candidates and checkpoints progress
        |
        v
score objective + constraints + tie-breakers
        |
        v
optional holdout re-evaluation, then poll/cancel/resume by run_id
```

旧 `profiles/top_ks` 请求会映射为 profile 与 retrieval top-k 搜索空间；当没有真实 pairwise judge 结果时，optimizer 默认回退到 `deterministic_answer_match`，再兼容读取历史 `pairwise_accuracy`、`answer_accuracy` 和 `run.Accuracy`。

当前 optimizer 的边界：

- HTTP internal runner 已支持 retrieval top-k、reranker top-n、graph 开关等 overlay；涉及重分块、embedding 或索引变更的候选会注册临时 namespace，真实重建索引成本仍需离线控制。
- 搜索策略以 grid、seeded random 和 successive halving 计划为主，不是完整 Bayesian optimization 或 bandit。
- External harness runner 已实现 argv-array、安全 allowlist、脱敏和指标白名单；HTTP 示例默认使用 internal RAG runner。
- 候选排序由 `objective.maximize`、constraints 和 tie-breakers 决定；使用 `deterministic_answer_match` 等 rule-based fallback 时，不应等价为完整业务满意度。

## 延迟权衡

复杂召回策略通常会提升 `deterministic_answer_match`、真实 `pairwise_accuracy`、`ndcg_at_k` 或 `recall_at_k`，但也可能提高 `latency_p95_ms`。推荐明确配置 `objective.maximize`，再用 `latency_p95_ms` 作为 SLO 约束：若质量提升明显且 P95 仍达标，可以选择 `high_precision` 或更大的 `top_k`；若质量收益很小但 P95 明显上升，应优先保留更简单的 `realtime` profile 或较小 `top_k`。

## 推荐使用方式

| 场景 | 推荐做法 |
| --- | --- |
| PR 回归 | 准备小型 deterministic 数据集，观察 `deterministic_answer_match`、IR 指标、冗余度、多样性和 `latency_p95_ms` 是否退化；需要 promotion 时使用 holdout split 和 holdout gate。 |
| profile 对比 | 使用相同数据集跑 `realtime` 和 `high_precision`。 |
| top_k 调参 | 用 optimizer 枚举少量候选，优先看 `objective` 得分和 `pairwise_accuracy`，再用 `latency_p95_ms` 和冗余度做约束。 |
| 真实质量评审 | 在 deterministic 指标之外启用 `judge`/`qag`，结合 `faithfulness`、`groundedness`、`citation_support`、`qag_score`、raw/parsed 明细和人工抽检判断答案质量。 |

## Judge/QAG 使用建议

- `faithfulness`：答案是否被检索证据支撑。
- `groundedness`：回答是否避免未引用事实。
- `answer_relevance`：答案是否真正回应用户问题。
- `citation_support`：引用是否支撑对应回答论断。
- `qag_score`：claim 级别的上下文支撑比例。
- `judge`/`qag` 输出会保留 provider/model、prompt/rubric/config hash、raw/parsed response、token usage 和 cost，便于跨版本追踪和校准。
