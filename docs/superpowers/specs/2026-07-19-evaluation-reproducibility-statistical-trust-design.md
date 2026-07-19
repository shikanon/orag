# 评估复现性与统计可信度优化方案

## 1. 目标

本方案把 ORAG 评估体系从“保存一次运行的汇总分数”升级为“可以复现、可以比较、可以解释、可以安全用于发布门禁的评估证据”。

核心目标：

1. 任意评估运行都能说明使用了哪一版数据、知识、索引、Pipeline、模型和运行参数。
2. 只有口径兼容的运行才能直接比较；口径不兼容时必须阻止比较或明确提示。
3. 每个指标同时报告有效样本数、标注覆盖率和不确定性，缺失标注不再被当成 0 或 1。
4. 候选方案与基线采用逐样本配对比较，发布门禁关注差值和置信区间，而不只关注两个均值。
5. 用户可以从评估结果中点击任意指标，查看含义、计算方式、适用条件和当前结果的解释。

## 2. 设计原则

- 不可变输入：一次运行引用的评估输入必须可冻结、可哈希、可追溯。
- 缺失不是零分：没有相关性标注的样本不参与依赖该标注的指标聚合。
- 比较优先于单点分数：优化和发布决策优先使用 candidate-baseline 的配对差值。
- 区间优先于伪精确值：展示置信区间、有效样本数和实际效应阈值。
- 服务端定义口径：指标名称、方向、公式、资格条件和说明由统一 registry 管理。
- 渐进披露：结果页首先呈现决策信息，详细定义由用户点击展开。

## 3. 复现性设计

### 3.1 Evaluation Manifest

为每次评估新增不可变 `evaluation_manifest`。建议作为独立 JSONB 字段保存，并通过 `schema_version` 演进。

```json
{
  "schema_version": "1.0",
  "code": {
    "revision": "git-sha",
    "build_version": "v0.8.0"
  },
  "dataset": {
    "id": "ds_xxx",
    "version": "...",
    "content_hash": "sha256:...",
    "item_count": 120,
    "split": "holdout",
    "split_hash": "sha256:..."
  },
  "knowledge_base": {
    "id": "kb_xxx",
    "content_hash": "sha256:...",
    "index_version": "idx_xxx",
    "index_hash": "sha256:..."
  },
  "pipeline": {
    "version_id": "pv_xxx",
    "content_hash": "sha256:..."
  },
  "runtime": {
    "profile": "realtime",
    "top_k": 8,
    "retrieval": {},
    "reranker": {},
    "context_pack": {},
    "cache_mode": "cold",
    "random_seed": 42
  },
  "models": {
    "embedding": {"provider": "...", "model": "...", "revision": "..."},
    "reranker": {"provider": "...", "model": "...", "revision": "..."},
    "generator": {"provider": "...", "model": "...", "revision": "..."},
    "judge": {"config_hash": "sha256:..."}
  },
  "environment": {
    "kind": "staging",
    "region": "...",
    "architecture": "..."
  }
}
```

Manifest 使用规范化 JSON 计算 `evaluation_fingerprint`。Map key 排序、数值格式、空值规则必须固定。

### 3.2 数据集快照

公共数据集保留当前可编辑体验，但运行评估时必须创建不可变 snapshot：

- `dataset_snapshot_id`
- snapshot item ID 列表及顺序
- 每条样本规范化内容哈希
- split、weight、ground truth、相关文档和多样性标注
- 整体 Merkle root 或 canonical JSON SHA-256

历史运行始终指向 snapshot，而不是继续变化的数据集根资源。

### 3.3 逐样本证据

每个 `evaluation_result` 增加：

- `trace_id`
- `input_hash`
- `retrieved_chunk_ids` 与对应内容哈希
- `effective_runtime_config_hash`
- `status`、`error_type`
- `attempt_count`
- `started_at`、`finished_at`

默认不重复保存可能敏感的完整文档内容；通过 chunk 内容哈希验证是否与历史输入一致。需要离线完全回放的部署可以启用加密 evidence bundle。

### 3.4 可比性判断

新增服务端 `comparability` 结果：

```json
{
  "comparable": false,
  "hard_mismatches": ["dataset.split_hash", "generator.revision"],
  "soft_mismatches": ["environment.architecture"]
}
```

硬不一致默认阻止自动晋级；软不一致允许查看，但必须展示警告。候选实验中被主动搜索的参数不视为不一致，其余输入必须相同。

## 4. 统计可信度设计

### 4.1 指标资格与缺失值

统一指标结果结构：

```json
{
  "value": 0.82,
  "eligible_sample_count": 96,
  "total_sample_count": 120,
  "annotation_coverage": 0.8,
  "weighted_sample_count": 101.5,
  "effective_sample_count": 88.2,
  "missing_reason_counts": {
    "missing_relevant_doc_ids": 24
  }
}
```

资格规则示例：

| 指标 | 样本资格 |
|---|---|
| answer accuracy | 有非空 ground truth |
| context recall、Recall@K、NDCG@K、MRR、MAP | 有 relevant doc 标注 |
| citation precision | 有 relevant doc 标注；无 citation 是有效的 0 |
| alpha-NDCG、aspect coverage | 有 diversity annotation |
| QAG claim coverage | 有 expected evidence |
| latency、cache hit | 查询执行成功 |

无资格样本记为 `missing`，不参与分子或分母。结果页必须同时展示标注覆盖率，避免通过减少困难样本标注来抬高分数。

若使用样本权重，报告 Kish effective sample size：

```text
n_eff = (sum(weight))² / sum(weight²)
```

### 4.2 置信区间

- 二元比例指标：单次运行默认使用 Wilson 95% 区间。
- 连续评分指标：使用分层 bootstrap 95% 区间，默认 2,000 次重采样。
- 加权指标：bootstrap 时按样本重采样，统计量内部继续应用固定业务权重。
- P50/P95/P99 延迟：使用 quantile bootstrap；同时报告成功请求数。
- 数据量不足时不生成误导性区间，返回 `insufficient_sample`。

建议默认最小有效样本量：冒烟报告 20，发布门禁 50，Judge 校准 50。具体阈值由项目策略配置。

### 4.3 Candidate 与 Baseline 配对比较

同一 snapshot 的 candidate 与 baseline 按 `dataset_item_id` 配对，新增：

```json
{
  "baseline": 0.78,
  "candidate": 0.82,
  "absolute_delta": 0.04,
  "relative_delta": 0.0513,
  "confidence_low": 0.01,
  "confidence_high": 0.07,
  "paired_sample_count": 96,
  "decision": "improved"
}
```

决策规则同时使用统计区间和业务最小效应 `min_effect`：

- `improved`：差值区间下界大于等于 `min_effect`。
- `non_inferior`：差值区间下界大于等于 `-non_inferiority_margin`。
- `regressed`：差值区间上界小于 `-non_inferiority_margin`。
- `inconclusive`：区间跨越决策边界。

Optimizer 搜索多个候选时，必须保留最终 holdout 复评；若在同一 holdout 上反复挑选，应增加多重比较控制或使用新的冻结 holdout。

### 4.4 延迟与成本

- `cost_usd` 和 token usage 按真实调用求和，不能乘业务样本权重。
- 同时报告 `cost_per_eligible_sample`、`cost_per_successful_answer`。
- 延迟分别运行 cold-cache 和 warm-cache 场景，不混合聚合。
- 性能门禁至少重复 3 轮，并记录环境指纹；不同环境的延迟结果默认不可直接比较。

### 4.5 Judge 可信度

- Judge 校准必须绑定 `judge_config_hash`、gold snapshot hash 和 human score version。
- Spearman/Kappa 不足最小样本数时必须失败，而不是默认允许晋级。
- Pairwise 分别报告 win/tie/loss；tie 不再按完整胜利计分。
- 顺序交换不稳定的结果不进入主分，并报告 `pairwise_stability_rate`。
- Judge 配置记录必须来自实际 executor 的 effective config，而不是只保存请求参数。

## 5. 指标 Registry 与点击查看

### 5.1 服务端 Registry

扩展现有 Metric Registry，并通过 `GET /v1/evaluation-metrics` 暴露稳定定义：

```json
{
  "name": "ndcg_at_k",
  "display_name": "NDCG@K",
  "category": "retrieval_ranking",
  "summary": "衡量相关文档是否排在检索结果前部。",
  "direction": "higher_is_better",
  "range": {"min": 0, "max": 1},
  "formula": "DCG@K / IDCG@K",
  "requires": ["relevant_doc_ids"],
  "aggregation": "weighted_mean_over_eligible_samples",
  "good_for": ["比较检索排序策略"],
  "caveats": ["只使用文档级二元相关性标注时，无法表达相关程度差异"],
  "related_metrics": ["recall_at_k", "mrr", "map"]
}
```

Registry 是 OpenAPI、后端校验、控制台说明和文档的唯一口径来源，避免四处维护文案。

### 5.2 结果页交互

结果区按决策顺序分组：

1. 发布结论：gate、可比性、是否达到最小样本数。
2. 答案质量：answer accuracy、faithfulness、completeness。
3. 检索质量：Recall@K、NDCG@K、MRR、MAP、failure rate。
4. 引用与证据：citation precision/support、QAG。
5. 效率：P95 latency、cache hit、tokens、cost。

每个指标行包含名称、当前值、95% 区间、有效样本数和趋势方向。名称后的“了解指标”按钮可点击；点击后在该指标下方行内展开详情，不使用模态框。

展开内容包括：

- 一句话解释
- “越高越好”或“越低越好”
- 计算方式
- 本次运行的有效样本及标注覆盖率
- 当前值的自然语言解释
- 使用限制
- 相关指标快捷入口

按钮要求：

- 使用原生 `button`，提供 `aria-expanded` 和 `aria-controls`。
- 支持键盘操作及清晰的 focus-visible 状态。
- 同一时间允许展开多个指标，方便对照。
- URL 可选保存 `?metric=ndcg_at_k`，便于分享具体解释。

窄屏下指标从表格切换为紧凑列表，解释内容保持在对应指标下方，避免横向滚动。

### 5.3 首期内置指标说明

| 指标 | 用户说明 | 方向 |
|---|---|---|
| deterministic answer match | 回答是否包含参考答案中的关键文本，只适合基础回归 | 越高越好 |
| answer accuracy | 当前为 deterministic answer match 的兼容别名 | 越高越好 |
| citation hit rate | 回答是否带引用，不代表引用一定正确 | 越高越好 |
| context recall | 检索结果覆盖了多少相关文档 | 越高越好 |
| citation precision | 引用中有多少指向标注的相关文档 | 越高越好 |
| NDCG@K | 相关文档是否排在前 K 个结果的前部 | 越高越好 |
| Recall@K | 前 K 个结果找回了多少相关文档 | 越高越好 |
| MRR | 第一个相关文档出现得有多靠前 | 越高越好 |
| MAP | 综合多个相关文档命中位置的排序质量 | 越高越好 |
| retrieval failure rate | 完全没有召回相关文档的样本比例 | 越低越好 |
| redundancy rate | 检索结果中重复 chunk 的比例 | 越低越好 |
| alpha-NDCG | 同时考虑排序和不同信息方面覆盖的质量 | 越高越好 |
| aspect coverage | 标注的信息方面被覆盖了多少 | 越高越好 |
| faithfulness | 回答中的论断是否能被证据支持 | 越高越好 |
| hallucination | 回答中不受证据支持或虚构内容的程度 | 越低越好 |
| QAG score | 逐论断验证后被证据支持的比例 | 越高越好 |
| latency P95 | 95% 请求可在该延迟内完成 | 越低越好 |
| cache hit rate | 命中语义缓存的查询比例 | 依场景判断 |
| cost USD | 本次评估产生的真实模型调用成本 | 越低越好 |

## 6. API 与存储变更

建议新增：

- `evaluation_runs.manifest JSONB NOT NULL`
- `evaluation_runs.evaluation_fingerprint TEXT NOT NULL`
- `evaluation_runs.dataset_snapshot_id TEXT NOT NULL`
- `evaluation_runs.status TEXT NOT NULL`
- `evaluation_results.trace_id TEXT`
- `evaluation_results.status TEXT NOT NULL`
- `evaluation_results.evidence JSONB NOT NULL`
- `evaluation_metric_summaries(run_id, metric, summary JSONB)`
- `evaluation_comparisons(baseline_run_id, candidate_run_id, metric, result JSONB)`

现有 `metrics: map[string]float64` 在兼容期继续返回；新客户端优先读取结构化 `metric_summaries`。

## 7. 发布门禁建议

发布 Policy 从单值比较升级为组合条件：

```json
{
  "metric": "answer_accuracy",
  "comparator": "non_inferior",
  "baseline_run_id": "eval_base",
  "margin": 0.01,
  "confidence_level": 0.95,
  "min_eligible_samples": 50,
  "min_annotation_coverage": 0.9
}
```

生产环境默认还应要求：

- evaluation fingerprint 可比较；
- 所有必需指标都有足够样本；
- Judge 指标的 calibration config hash 匹配且仍有效；
- 延迟测试环境匹配；
- 没有未解释的失败样本或不稳定 Pairwise 结果。

## 8. 实施顺序

### 阶段 A：修正现有口径

- 无标注样本不再用 0/1 代替未知。
- Token 和成本改为真实求和。
- Pairwise 拆分 win/tie/loss/stability。
- UI 增加静态 Metric Registry 和行内点击说明，先覆盖现有指标。

### 阶段 B：不可变证据

- 增加 dataset snapshot、manifest、fingerprint。
- 保存 effective config 和逐样本 trace/chunk hash。
- 增加可比性判断。

### 阶段 C：统计汇总

- 增加 eligible count、coverage、effective N、Wilson/bootstrap CI。
- 增加 baseline-candidate 配对比较。
- 控制台展示区间、差值和 inconclusive 状态。

### 阶段 D：可信发布门禁

- Policy 支持 non-inferiority、样本量和覆盖率约束。
- Judge 校准与 promotion 强绑定。
- 性能门禁区分 cold/warm cache 和运行环境。

## 9. 验收标准

- 同一输入重复运行生成相同 fingerprint；任一关键输入变化都能指出具体差异。
- 追加数据集样本不会改变历史运行所引用的 snapshot。
- 无相关文档标注的样本不影响 Recall@K/NDCG/MRR/MAP 的值，但会降低 annotation coverage。
- 每个聚合指标均返回有效样本数；发布指标按类型返回可信区间。
- Candidate 与 baseline 只有在可比时才生成配对结论。
- 成本汇总等于逐次模型调用的真实成本之和，不受样本权重影响。
- 每个控制台指标均可通过键盘点击展开说明，并显示当前运行的适用性和限制。
- 小样本、低覆盖率、Judge 未校准或结果不确定时，生产门禁默认失败。
