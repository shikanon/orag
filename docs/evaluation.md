# 评估与自迭代

评估模块复用线上 RAG 服务路径，避免线上线下漂移。当前实现提供数据集创建、数据项写入、评估运行、评估查询和确定性参数网格优化。

## 数据集与评估结果存储

默认 `qdrant_postgres` 后端会把数据集、评估运行和逐样本评估结果持久化到 PostgreSQL；`memory` 后端只服务单测和本地无依赖调试，进程结束后数据不保留。

### 数据集

- `datasets`：数据集元信息，包含 `kind` 和 `version`。
- `dataset_items`：样本，包含 `query`、`ground_truth`、`relevant_doc_ids`。

`datasets` 由租户隔离，主要字段包括 `id`、`tenant_id`、`name`、`kind`、`version` 和 `created_at`。`version` 当前由创建时间生成，适合区分同名数据集的不同批次。

`dataset_items` 按 `dataset_id` 归属到数据集。每条样本包含：

- `query`：评估时传给 RAG 服务的用户问题。
- `ground_truth`：规则指标用于匹配的参考答案或关键事实文本。
- `relevant_doc_ids`：期望被检索或引用的文档 ID 列表，用于 `context_recall` 和 `citation_precision`。

### 评估运行

- `evaluation_runs`：一次评估运行的汇总结果，包含 `id`、`tenant_id`、`dataset_id`、`profile`、`metrics` 和 `created_at`。
- `evaluation_results`：每个样本的评估结果，包含 `run_id`、`dataset_item_id`、模型返回的 `answer` 和逐样本 `metrics`。

`evaluation_runs.metrics` 是 JSON 指标快照，当前会保存 `total`、`hit_rate`、`accuracy` 以及聚合后的 `context_recall`、`citation_precision`、`latency_p95_ms`、`cache_hit_rate`。`GET /v1/evaluations/{id}` 查询的是运行级汇总，不返回逐样本 `evaluation_results` 明细。

`evaluation_results.metrics` 保存逐样本指标，包括 `accuracy`、`context_recall`、`citation_precision`、`latency_ms` 和 `cache_hit`。这些明细用于后续分析和扩展，目前公开 API 主要暴露运行级汇总。

## 运行流程

`POST /v1/evaluations` 会按以下路径执行：

1. 根据 `dataset_id` 读取数据集样本。
2. 对每个样本调用同一套 `rag.Service.Query`，传入 `tenant_id`、`knowledge_base_id`、`query`、`profile` 和可选 `top_k`。
3. 基于 RAG 响应中的答案、引用、检索 chunk、延迟和 cache 状态计算逐样本规则指标。
4. 聚合生成一次 `evaluation_run`，并在配置了 Repository 时写入运行汇总和逐样本结果。

因此评估结果反映的是当前线上查询链路在指定知识库、profile 和 `top_k` 下的行为，而不是离线 mock 检索器或独立评测流水线。

## 当前指标

当前实现是 deterministic rule-based metrics，不依赖真实 Ark Key：

- `accuracy`：逐样本规则命中为 1，否则为 0；当前命中条件是答案包含 `ground_truth` 中长度大于 3 的关键项，或响应中存在任意引用。运行级 `accuracy` 是命中样本数除以样本总数。
- `hit_rate`：当前与 `accuracy` 保持一致，都是同一组规则命中的聚合结果。
- `context_recall`：当 `relevant_doc_ids` 非空时，统计 retrieved chunks 覆盖了多少不同的相关文档 ID；当 `relevant_doc_ids` 为空时，有任意 retrieved chunk 记为 1，否则为 0。
- `citation_precision`：当响应有引用且 `relevant_doc_ids` 非空时，统计引用文档 ID 落在相关文档列表中的比例；没有引用时为 0，样本未标注 `relevant_doc_ids` 且存在引用时为 1。
- `latency_p95_ms`：本次评估内所有样本查询延迟的 P95，来自 RAG 响应的 `LatencyMS`。
- `cache_hit_rate`：本次评估中 `CacheStatus == "hit"` 的样本比例。

适用边界：

- 适合做基础回归门禁、不同 profile / `top_k` 的粗粒度对比，以及检索、引用、延迟和 cache 行为的冒烟检查。
- `accuracy` 是弱规则指标。只要有任意引用也会计为命中，因此不能单独证明答案正确；`ground_truth` 匹配基于文本包含关系，对同义改写、复杂推理、多语种细粒度表达不敏感。
- `context_recall` 和 `citation_precision` 只检查文档 ID，不验证 chunk 内容是否真的支撑答案，也不验证引用位置与回答论断的一致性。
- 当前没有实现完整 LLM-as-Judge，不评估 faithfulness、groundedness、answer relevance、回答完整性或事实幻觉。

## Optimizer

`POST /v1/optimizations` 对候选 `profiles` 和 `top_ks` 做确定性网格枚举。当前优化目标是运行级 `accuracy`，也就是上述规则命中率。

流程如下：

1. 读取请求中的 `profiles` 和 `top_ks`；未提供时默认使用 `realtime`、`high_precision` 和 `top_k = 8`。
2. 按 `profile × top_k` 双层循环枚举候选。
3. 每个候选都调用同一套 evaluation runner，因此会产生独立的 `evaluation_run` 和逐样本 `evaluation_results`。
4. 候选结果记录 `profile`、`top_k`、`score` 和 `run_id`，其中 `score = run.Accuracy`。
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
- 分数依赖当前 rule-based `accuracy`，不能等价为真实业务满意度或完整答案质量。

## 后续 LLM-as-Judge 增强边界

后续可以在保持 deterministic rule-based metrics 的基础上，增加可选的 Ark LLM-as-Judge 层：

- 为 `faithfulness`、`groundedness`、`answer_relevance` 等维度增加独立分数和理由，评估答案是否被检索证据支撑、是否回答了问题、是否存在明显幻觉。
- 记录 judge 模型、prompt 版本、评分维度和原始解释，避免不同评测版本之间不可比较。
- 保留无 Ark Key 的确定性评估路径，使本地测试和 CI 基础门禁不依赖外部模型。
- 将 LLM-as-Judge 结果作为增强指标接入回归集和人工校准流程，而不是把当前实现描述成已经具备完整主观评审能力。
