# 评估与自迭代

评估模块复用线上 RAG 服务路径，避免线上线下漂移。当前实现提供数据集创建、数据项写入、评估运行、评估查询和确定性参数网格优化。

## 数据集与评估结果存储

默认 `qdrant_postgres` 后端会把数据集、评估运行和逐样本评估结果持久化到 PostgreSQL；`memory` 后端只服务单测和本地无依赖调试，进程结束后数据不保留。

### 数据集

- `datasets`：数据集元信息，包含 `kind` 和 `version`。
- `dataset_items`：样本，包含 `query`、`ground_truth`、`relevant_doc_ids` 和可选 `diversity_annotations`。

`datasets` 由租户隔离，主要字段包括 `id`、`tenant_id`、`name`、`kind`、`version` 和 `created_at`。`version` 当前由创建时间生成，适合区分同名数据集的不同批次。

`dataset_items` 按 `dataset_id` 归属到数据集；写入和读取样本前都会校验数据集属于当前 tenant。每条样本包含：

- `query`：评估时传给 RAG 服务的用户问题。
- `ground_truth`：规则指标用于匹配的参考答案或关键事实文本。
- `relevant_doc_ids`：期望被检索或引用的文档 ID 列表，用于 `context_recall`、`citation_precision`、NDCG/Recall/MRR/MAP、coverage 和 retrieval failure 相关指标。
- `diversity_annotations`：可选多方面标注，用 aspect 或 subquestion 绑定 chunk、document 或 source，用于 `alpha_ndcg` 和 `aspect_coverage`。

### 评估运行

- `evaluation_runs`：一次评估运行的汇总结果，包含 `id`、`tenant_id`、`dataset_id`、`profile`、`metrics` 和 `created_at`。
- `evaluation_results`：每个样本的评估结果，包含 `run_id`、`dataset_item_id`、模型返回的 `answer` 和逐样本 `metrics`。

`evaluation_runs.metrics` 是 JSON 指标快照，当前会保存 `answer_accuracy`、`accuracy`、`hit_rate`、主指标 `pairwise_accuracy`、`citation_hit_rate`，以及聚合后的 `context_recall`、`citation_precision`、`ndcg_at_k`、`recall_at_k`、`mrr`、`map`、`coverage`、`retrieval_failure_rate`、`redundancy_rate`、`duplicate_count`、`deduped_top_k_count`、`alpha_ndcg`、`aspect_coverage`、`latency_p95_ms`、`cache_hit_rate`。`total` 是 evaluation run 的顶层字段。`GET /v1/evaluations/{id}` 查询的是运行级汇总，不返回逐样本 `evaluation_results` 明细。历史已存运行可能没有新增的指标键。

`evaluation_results.metrics` 保存逐样本指标，包括 `answer_accuracy`、`accuracy`、`citation_hit_rate`、`context_recall`、`citation_precision`、`latency_ms`、`cache_hit` 以及检索质量、冗余度和多样性指标。运行级指标会对可聚合的逐样本指标取平均；`latency_p95_ms` 使用本次运行所有样本的查询延迟计算 P95。

## 运行流程

`POST /v1/evaluations` 会按以下路径执行：

1. 根据当前 tenant 和 `dataset_id` 校验数据集归属并读取样本。
2. 对每个样本调用同一套 `rag.Service.Query`，传入 `tenant_id`、`knowledge_base_id`、`query`、`profile` 和可选 `top_k`。
3. 基于 RAG 响应中的答案、引用、检索 chunk、延迟和 cache 状态计算逐样本规则指标。
4. 聚合生成一次 `evaluation_run`，并在配置了 Repository 时写入运行汇总和逐样本结果。

因此评估结果反映的是当前线上查询链路在指定知识库、profile 和 `top_k` 下的行为，而不是离线 mock 检索器或独立评测流水线。

如果 `dataset_id` 不存在或不属于当前 tenant，添加样本、运行评估和运行 optimizer 都会返回 `404 dataset_not_found`。这些失败路径不会写入 `dataset_items`、`evaluation_runs` 或 `evaluation_results`。

## 当前指标

当前实现是 deterministic rule-based metrics，不依赖真实 Ark Key：

- `answer_accuracy`：逐样本答案包含 `ground_truth` 中长度大于 3 的关键项时为 1，否则为 0；响应中存在 citation 不会提升该指标。运行级 `answer_accuracy` 是答案命中样本数除以样本总数。
- `accuracy`：新运行中与 `answer_accuracy` 保持一致，是面向答案正确性的兼容别名。
- `hit_rate`：新运行中与 `answer_accuracy` 保持一致，是面向答案命中的兼容别名。
- `pairwise_accuracy`：当前优化器主质量指标。在未接入真实 pairwise judge 前，该字段由 `answer_accuracy` 填充；后续接入 A/B judge 后应表示候选回答在成对比较中胜出或不输基线的比例。
- `citation_hit_rate`：逐样本响应中存在至少一个 citation 时为 1，否则为 0；运行级指标是有 citation 样本数除以样本总数。
- `context_recall`：当 `relevant_doc_ids` 非空时，统计 retrieved chunks 覆盖了多少不同的相关文档 ID；当 `relevant_doc_ids` 为空时，有任意 retrieved chunk 记为 1，否则为 0。
- `citation_precision`：当响应有引用且 `relevant_doc_ids` 非空时，统计引用文档 ID 落在相关文档列表中的比例；没有引用时为 0，样本未标注 `relevant_doc_ids` 且存在引用时为 1。
- `ndcg_at_k`：基于相关文档在前 `top_k` 召回结果中的排名计算归一化 DCG。越靠前命中相关文档分数越高。
- `recall_at_k`：前 `top_k` 召回结果覆盖相关文档的比例。重复命中同一相关文档只计一次。
- `mrr`：第一个相关文档的 reciprocal rank。
- `map`：对相关文档命中位置的 precision 做平均，再按相关文档数归一。
- `coverage`：样本是否至少召回一个相关文档，运行级为样本平均值。
- `retrieval_failure_rate`：样本标注了相关文档但没有召回任何相关文档时为 1；未标注时为 0。
- `redundancy_rate`：重复 chunk 数除以召回结果数；重复判定优先使用 chunk ID，其次 metadata 中的 hash/dedupe key，再退化到规范化文本。
- `duplicate_count`：召回结果中的重复条数。
- `deduped_top_k_count`：去重后的召回结果数量。
- `alpha_ndcg`：多样性敏感的 NDCG，对重复覆盖同一 aspect/subquestion 的增益做衰减。
- `aspect_coverage`：已召回证据覆盖的 aspect/subquestion 数占全部标注 aspect/subquestion 数的比例。
- `latency_p95_ms`：本次评估内所有样本查询延迟的 P95，来自 RAG 响应的 `LatencyMS`。
- `cache_hit_rate`：本次评估中 `CacheStatus == "hit"` 的样本比例。

适用边界：

- 适合做基础回归门禁、不同 profile / `top_k` 的粗粒度对比，以及检索排序、覆盖率、冗余度、多样性、引用、延迟和 cache 行为的冒烟检查。
- `pairwise_accuracy` 是当前优化器主指标；在真正的 pairwise judge 接入前，它仍由 answer-focused 规则命中率填充，因此只能代表当前 deterministic 质量信号。
- `answer_accuracy` / `accuracy` 仍是弱规则指标，只基于 `ground_truth` 文本包含关系；它不再把任意引用计为答案正确，但仍对同义改写、复杂推理、多语种细粒度表达不敏感。
- `citation_hit_rate` 只说明响应带有引用，不能单独证明答案正确或引用相关。
- `context_recall` 和 `citation_precision` 只检查文档 ID，不验证 chunk 内容是否真的支撑答案，也不验证引用位置与回答论断的一致性。
- IR 指标依赖 `relevant_doc_ids`，多样性指标依赖 `diversity_annotations`。缺少标注时相关指标会返回 0 或跳过聚合，不应与完整标注数据集直接比较。
- 当前没有实现完整 LLM-as-Judge，不评估 faithfulness、groundedness、answer relevance、回答完整性或事实幻觉。

## 延迟与复杂召回策略权衡

复杂召回策略通常会提升覆盖率或排序质量，但也可能抬高 `latency_p95_ms`。评估调参时建议把 `pairwise_accuracy` 作为主排序信号，同时用 `latency_p95_ms` 设置可接受的尾延迟上限：

- 当 `high_precision`、更大的 `top_k`、dense+sparse 融合、rerank 或多样性召回带来更高 `pairwise_accuracy`、`ndcg_at_k`、`recall_at_k`，且 `latency_p95_ms` 仍在业务 SLO 内时，可以接受额外复杂度。
- 当质量指标只小幅提升但 `latency_p95_ms` 明显上升时，应优先保留更简单的 `realtime` profile 或较小 `top_k`，避免把长尾延迟转嫁给线上查询。
- 当 `retrieval_failure_rate` 下降明显但冗余度升高时，需要同时观察 `redundancy_rate` 和 `deduped_top_k_count`，避免通过堆叠重复 chunk 虚假提升召回。
- 小样本评估的 P95 波动较大，建议使用固定数据集、相同缓存状态和多次运行结果比较复杂策略收益。

## Optimizer

`POST /v1/optimizations` 对候选 `profiles` 和 `top_ks` 做确定性网格枚举。当前优化目标是运行级 `pairwise_accuracy`；在未接入真实 pairwise judge 前，该字段由 `answer_accuracy` 填充。

流程如下：

1. 读取请求中的 `profiles` 和 `top_ks`；未提供时默认使用 `realtime`、`high_precision` 和 `top_k = 8`。
2. 按 `profile × top_k` 双层循环枚举候选。
3. 每个候选都调用同一套 evaluation runner，因此会产生独立的 `evaluation_run` 和逐样本 `evaluation_results`。
4. 候选结果记录 `profile`、`top_k`、`score`、`pairwise_accuracy`、检索诊断字段、`latency_p95_ms` 和 `run_id`，其中 `score = metrics.pairwise_accuracy`；历史运行缺失时回退到 `answer_accuracy` / `accuracy`。
5. 返回 `status = "completed"`、完整候选列表和 `best` 候选；当前不额外落一张 optimization 表。

如果多个候选分数相同，当前实现会保留后遍历到的候选作为 `best`。因此 optimizer 更适合做小规模、确定性的参数对比，而不是长期实验管理。

请求示例：

```json
{
  "dataset_id": "ds_xxx",
  "knowledge_base_id": "kb_xxx",
  "profiles": ["realtime", "high_precision"],
  "top_ks": [5, 8]
}
```

当前 optimizer 的边界：

- 只优化 `profile` 和 `top_k`，不自动调整 prompt、embedding、reranker、chunk 策略、模型或索引参数。
- 不做 Bayesian optimization、bandit、早停或成本约束搜索。
- optimization result 只在本次响应中返回；可追溯数据来自每个候选关联的 `run_id`。
- 分数依赖当前 `pairwise_accuracy` 主指标；在当前 rule-based 填充阶段，它来自 `answer_accuracy`，不能等价为真实业务满意度或完整答案质量。

## 后续 LLM-as-Judge 增强边界

后续可以在保持 deterministic rule-based metrics 的基础上，增加可选的 Ark LLM-as-Judge 层：

- 为 `faithfulness`、`groundedness`、`answer_relevance` 等维度增加独立分数和理由，评估答案是否被检索证据支撑、是否回答了问题、是否存在明显幻觉。
- 记录 judge 模型、prompt 版本、评分维度和原始解释，避免不同评测版本之间不可比较。
- 保留无 Ark Key 的确定性评估路径，使本地测试和 CI 基础门禁不依赖外部模型。
- 将 LLM-as-Judge 结果作为增强指标接入回归集和人工校准流程，而不是把当前实现描述成已经具备完整主观评审能力。
