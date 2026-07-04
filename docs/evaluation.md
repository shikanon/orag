# 评估与自迭代

评估模块复用线上 RAG 服务路径，避免线上线下漂移。当前实现提供数据集创建、数据项写入、规则评估、可选 LLM-as-Judge/QAG 明细、评估查询和目标驱动异步优化。

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
- `judge_runs` / `judge_results`：可选 Judge/QAG 运行与逐样本结果，保存 provider/model、prompt/rubric/config hash、raw response、parsed JSON、token usage 和 cost。
- `optimization_runs` / `optimization_candidates`：目标驱动 optimizer 的异步 run、candidate、checkpoint、预算、状态、临时 namespace 和 holdout 复评结果。

`evaluation_runs.metrics` 是 JSON 指标快照，当前会保存 `answer_accuracy`、`accuracy`、`hit_rate`、主指标 `pairwise_accuracy`、`citation_hit_rate`，以及聚合后的 `context_recall`、`citation_precision`、`ndcg_at_k`、`recall_at_k`、`mrr`、`map`、`coverage`、`retrieval_failure_rate`、`redundancy_rate`、`duplicate_count`、`deduped_top_k_count`、`alpha_ndcg`、`aspect_coverage`、`latency_p95_ms`、`cache_hit_rate`。启用 Judge/QAG 时还会聚合 `faithfulness`、`groundedness`、`citation_support`、`qag_score`、`qag_claim_coverage`、`prompt_tokens`、`completion_tokens`、`total_tokens`、`cost_usd` 等指标。`total` 是 evaluation run 的顶层字段。`GET /v1/evaluations/{id}` 默认查询运行级汇总；传 `include_items/include_judge/include_pairwise` 可返回明细。历史已存运行可能没有新增的指标键。

`evaluation_results.metrics` 保存逐样本指标，包括 `answer_accuracy`、`accuracy`、`citation_hit_rate`、`context_recall`、`citation_precision`、`latency_ms`、`cache_hit` 以及检索质量、冗余度和多样性指标。运行级指标会对可聚合的逐样本指标取平均；`latency_p95_ms` 使用本次运行所有样本的查询延迟计算 P95。

## 运行流程

`POST /v1/evaluations` 会按以下路径执行：

1. 根据当前 tenant 和 `dataset_id` 校验数据集归属并读取样本。
2. 对每个样本调用同一套 `rag.Service.Query`，传入 `tenant_id`、`knowledge_base_id`、`query`、`profile` 和可选 `top_k`。
3. 基于 RAG 响应中的答案、引用、检索 chunk、延迟和 cache 状态计算逐样本规则指标。
4. 如果请求包含 `judge`，执行 LLM-as-Judge 并记录分数、标签、理由、raw/parsed response、token 和 cost。
5. 如果请求包含 `qag`，执行 QAG claim verification，记录 claim verdict、coverage、unverifiable rate、token 和 cost。
6. 聚合生成一次 `evaluation_run`，并在配置了 Repository 时写入运行汇总、逐样本结果和可选 Judge/QAG 明细。

因此评估结果反映的是当前线上查询链路在指定知识库、profile 和 `top_k` 下的行为，而不是离线 mock 检索器或独立评测流水线。

如果 `dataset_id` 不存在或不属于当前 tenant，添加样本、运行评估和运行 optimizer 都会返回 `404 dataset_not_found`。这些失败路径不会写入 `dataset_items`、`evaluation_runs` 或 `evaluation_results`。

## 当前指标

未传 `judge`/`qag` 时，评估会执行 deterministic rule-based metrics，不依赖真实 Ark Key：

- `answer_accuracy`：逐样本答案包含 `ground_truth` 中长度大于 3 的关键项时为 1，否则为 0；响应中存在 citation 不会提升该指标。运行级 `answer_accuracy` 是答案命中样本数除以样本总数。
- `accuracy`：新运行中与 `answer_accuracy` 保持一致，是面向答案正确性的兼容别名。
- `hit_rate`：新运行中与 `answer_accuracy` 保持一致，是面向答案命中的兼容别名。
- `pairwise_accuracy`：当前优化器主质量指标。未执行 pairwise judge 时由 `answer_accuracy` 填充；启用 A/B pairwise judge 后表示候选回答在成对比较中胜出或不输基线的比例。
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
- `pairwise_accuracy` 是当前优化器主指标；未执行 pairwise judge 时它由 answer-focused 规则命中率填充，只代表 deterministic fallback 信号。启用 pairwise judge 后应结合 raw/parsed 明细、稳定性标记和校准结果判读。
- `answer_accuracy` / `accuracy` 仍是弱规则指标，只基于 `ground_truth` 文本包含关系；它不再把任意引用计为答案正确，但仍对同义改写、复杂推理、多语种细粒度表达不敏感。
- `citation_hit_rate` 只说明响应带有引用，不能单独证明答案正确或引用相关。
- `context_recall` 和 `citation_precision` 只检查文档 ID，不验证 chunk 内容是否真的支撑答案，也不验证引用位置与回答论断的一致性。
- IR 指标依赖 `relevant_doc_ids`，多样性指标依赖 `diversity_annotations`。缺少标注时相关指标会返回 0 或跳过聚合，不应与完整标注数据集直接比较。
- LLM-as-Judge/QAG 已作为可选增强路径接入；未传 `judge`/`qag` 时仍只执行 deterministic rule-based metrics。主观指标用于自动晋级前应使用 gold set 校准，并避免把低相关性的 judge 分数用于 promotion。

## 延迟与复杂召回策略权衡

复杂召回策略通常会提升覆盖率或排序质量，但也可能抬高 `latency_p95_ms`。评估调参时建议把 `pairwise_accuracy` 作为主排序信号，同时用 `latency_p95_ms` 设置可接受的尾延迟上限：

- 当 `high_precision`、更大的 `top_k`、dense+sparse 融合、rerank 或多样性召回带来更高 `pairwise_accuracy`、`ndcg_at_k`、`recall_at_k`，且 `latency_p95_ms` 仍在业务 SLO 内时，可以接受额外复杂度。
- 当质量指标只小幅提升但 `latency_p95_ms` 明显上升时，应优先保留更简单的 `realtime` profile 或较小 `top_k`，避免把长尾延迟转嫁给线上查询。
- 当 `retrieval_failure_rate` 下降明显但冗余度升高时，需要同时观察 `redundancy_rate` 和 `deduped_top_k_count`，避免通过堆叠重复 chunk 虚假提升召回。
- 小样本评估的 P95 波动较大，建议使用固定数据集、相同缓存状态和多次运行结果比较复杂策略收益。

## Optimizer

`POST /v1/optimizations` 提交目标驱动异步 optimizer run，返回 `202 Accepted`、`run_id` 和轮询/取消/续跑 URL。当前 HTTP 层默认使用 internal RAG runner，支持 `objective`、`search_space`、`search`、`budget`、`selection_split`、`holdout_split`；旧 `profiles/top_ks` 请求仍被接受并映射为 profile 与 retrieval top-k 搜索空间。

流程如下：

1. 校验当前 tenant 下的 `dataset_id` 和 `knowledge_base_id`，生成 `optimization_run` 和候选集合，初始状态为 `queued`。
2. 后台 worker 将 run 切到 `running`，按候选执行 evaluation runner，并把候选 metrics、cost、临时 namespace 和 checkpoint 写入 repository。
3. objective 模块按 `maximize`、`constraints`、`tie_breakers` 和预算归一化信息给候选打分，选出 `best_candidate_id`。
4. 如果传入 `holdout_split`，仅对最佳候选执行 holdout 复评，holdout 不参与 candidate proposal、mutation、search 或 selection。
5. Worker 中断后可调用 `POST /v1/optimizations/{id}:resume`，从 checkpoint 跳过已完成候选；调用 `:cancel` 会请求停止调度新候选。

如果多个候选分数相同，当前实现会保留后遍历到的候选作为 `best`。因此 optimizer 更适合做小规模、确定性的参数对比，而不是长期实验管理。

请求示例：

```json
{
  "dataset_id": "ds_xxx",
  "knowledge_base_id": "kb_xxx",
  "profile": "realtime",
  "objective": {
    "maximize": "pairwise_accuracy",
    "constraints": [
      {"expression": "latency_p95_ms <= 1000"}
    ]
  },
  "search_space": {
    "retrieval": {
      "dense_top_k": [5, 8]
    }
  },
  "search": {
    "strategy": "grid",
    "max_candidates": 4
  },
  "budget": {
    "max_judge_calls": 20,
    "max_cost_usd": 1
  },
  "selection_split": "eval",
  "holdout_split": "holdout"
}
```

当前 optimizer 的边界：

- HTTP internal RAG runner 已支持 retrieval top-k、reranker top-n、graph 开关等 overlay；涉及重分块、embedding 或索引变更的候选会注册临时 namespace，但真实重建索引成本仍需离线控制。
- 当前搜索策略以 grid/seeded random/successive halving 计划为主，不是完整 Bayesian optimization 或 bandit。
- 分数依赖 `objective.maximize` 指标；当仍使用 rule-based `pairwise_accuracy` 填充时，不能等价为真实业务满意度或完整答案质量。
- External harness runner 在底层模块中已实现 argv-array、安全 allowlist、脱敏和指标白名单；HTTP 示例默认使用 internal RAG runner。
