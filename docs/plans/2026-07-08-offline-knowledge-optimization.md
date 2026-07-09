# Offline Knowledge Optimization Design

**Goal:** 为 ORAG 增加离线知识整理与优化库能力。系统每天夜间基于用户历史问题自动发现召回缺陷，由 Codex 读取 ORAG 数据并执行多轮深度检索，生成有证据支撑的优化条目，进入独立优化库，并通过 shadow retrieval 与 eval runner 回归验证召回和答案质量收益。

**Architecture:** 新增夜间任务、历史问题抽取、召回重放、Codex 深度分析、优化条目入库、多轮深度检索、证据校验、状态机、管理 API、基础 metrics、shadow retrieval 和离线回归验证。Codex 作为离线智能分析执行器，通过受控只读工具读取 ORAG trace、知识库、chunks、documents、eval results 和召回结果；ORAG 负责调度、数据访问边界、证据校验、状态流转、入库、观测、API 和回归评估。

**Tech Stack:** Go, Hertz HTTP server, PostgreSQL JSONB, Qdrant, existing RAG retrievers, existing eval runner, existing optimizer/evaluation persistence patterns, Codex CLI or Codex-compatible offline analyzer, Prometheus metrics, contract tests.

---

## 设计原则

- 优化条目必须 evidence-grounded，不能只有 Codex 总结。
- Codex 负责深度分析和多轮检索决策，但不能绕过 ORAG 的权限、校验、状态机和入库约束。
- 优化库独立于主知识库，默认不污染主知识库。
- Shadow retrieval 默认不影响线上答案，只记录潜在命中、潜在上下文变化和评估收益。
- Eval runner 必须对 baseline 和 with-optimization 做成对回归评估，验证 `recall lift`、`answer quality lift` 和引用覆盖收益。
- 所有 Codex 深度检索步骤必须落库，便于审计条目来源和后续问题追踪。
- `answer_item` 默认只做召回增强和生成引导，答案仍必须由 context 中的真实 chunk 支撑生成，不能作为最终答案直接注入。
- 源引用必须同时保存 `doc_version` 和 `chunk_content_hash`，校验按内容指纹执行，不能只依赖可复用或可漂移的 `chunk_id`。
- 所有核心表必须包含 `tenant_id`，抽取、分析、入库、API、shadow retrieval 和 eval regression 都按 tenant 隔离。
- 源文档、chunk 或知识库版本变化后，关联优化条目必须重新校验、进入 stale 或 deprecated 状态。

## 目标状态

- 每天夜间自动创建离线知识整理任务。
- 从历史问题、RAG traces、evaluation results 和失败查询中抽取真实用户问题。
- 将用户显式负反馈作为一等信号源，并为长尾单次问题保留低优先队列，不直接丢弃。
- 对历史问题做归一化、聚类、去重，保留代表问题和原始样本。
- 使用当前线上召回配置做召回重放，得到 baseline recall evidence。
- 由 Codex 读取 ORAG 数据并执行多轮深度检索，判断当前召回是否准确。
- 对 Codex 输出进行证据校验，生成优化条目、query rewrite 建议或知识缺口记录。
- 使用状态机管理条目从 candidate 到 verified、shadow_enabled、regression_passed、published 或 rejected 的生命周期。
- 提供管理 API 查询任务、问题簇、优化条目、证据、Codex 分析过程和回归评估结果。
- 输出基础 Prometheus metrics，用于观察任务吞吐、失败、Codex 成本、条目质量和 shadow 命中。
- 接入 shadow retrieval，并基于 eval runner 做离线回归，验证优化库收益。

## 非目标

- 不让 Codex 直接修改主知识库。
- 不让 Codex 直接发布优化条目到正式召回。
- 不把没有证据的总结写入可召回优化库。
- 不把 shadow retrieval 结果默认注入线上回答上下文。
- 不以单次 LLM 判断替代 eval runner 的成对回归评估。

## 总体架构

```text
Nightly Scheduler
        |
        v
History Question Extractor
        |
        v
Question Cluster & Deduplicate
        |
        v
Recall Replay
        |
        v
Codex Deep Analyzer
        |
        +--> 读取 ORAG trace / KB / chunks / docs / eval results
        +--> 执行多轮深度检索
        +--> 判断召回是否准确
        +--> 校验证据是否充分
        +--> 生成结构化分析报告
        |
        v
Optimization Item Builder
        |
        v
Optimization Library
        |
        +--> State Machine
        +--> Management API
        +--> Metrics
        |
        v
Shadow Retrieval
        |
        v
Eval Runner Regression
        |
        +--> recall lift
        +--> answer quality lift
        +--> citation coverage
        +--> hallucination risk
```

## 核心模块

### OfflineKnowledgeOrganizer

- 负责夜间任务调度、手动触发、批次控制、checkpoint、重试、超时和并发限制。
- 每个 run 固化配置快照，包括时间窗口、知识库范围、Codex 并发、最大问题数、最大深度检索步数和评估阈值。
- 任务失败不影响线上查询链路，失败的问题簇记录错误详情并可在下一次任务中继续处理。
- 任务维度记录输入窗口、处理进度、生成条目数、校验通过数、拒绝数、shadow 启用数和回归收益。
- 每个 tenant 和 knowledge base 的 run 必须持有分布式锁，避免多实例重复执行同一窗口。
- run 创建时按 `(tenant_id, kb_id, window_start, window_end, config_hash)` 做窗口去重，重复触发返回已有 run。
- 召回重放、Codex 工具调用和 eval regression 使用只读副本或隔离资源池，不能挤占线上主链路资源。

### HistoryQuestionExtractor

- 从 `rag_traces`、query logs、evaluation results、失败 trace、低分 eval item、用户显式负反馈和真实 query 中抽取历史问题。
- 抽取字段包括 query、trace id、kb id、profile、topK、召回结果、最终答案、引用、latency、error、cache hit 和 node spans。
- 过滤空 query、健康检查、自动探测请求、明显闲聊、过短问题和重复请求。
- 长尾单次问题进入低优先队列，按采样、负反馈、业务标签或相似聚类提升优先级，不直接丢弃。
- 支持按时间窗口、tenant、knowledge base、profile、失败类型和评估分数过滤。

### QuestionClusterer

- 对问题做文本归一化、hash 去重和 embedding 相似聚类。
- 每个问题簇保留 canonical question、sample questions、trace ids、出现次数、首次出现时间和最近出现时间。
- Codex 分析以 canonical question 为主，同时读取 sample questions，避免丢失用户真实表达差异。
- 聚类结果用于避免同一类问题重复生成优化条目。
- 问题簇需要保存归一化 embedding 或 embedding reference，支持离线相似聚类、后续增量归并和可解释去重。
- 问题簇按 `(tenant_id, kb_id, question_hash)` 建唯一约束，避免重复 run 生成重复簇。

### RecallReplayer

- 使用当前线上配置重放召回链路，而不是只依赖历史召回快照。
- 重放 query rewrite、多查询、HyDE、dense retrieve、sparse retrieve、RRF、graph expansion、rerank 和 context pack。
- 重放结果保存 topK chunks、dense/sparse score、RRF score、rerank score、chunk text、doc id、chunk id 和引用覆盖情况。
- 召回重放不负责最终答案生成，目标是为 Codex 提供 baseline recall evidence。
- 召回重放必须走只读副本、离线 Qdrant collection alias 或隔离资源池，并设置独立 QPS、并发和超时。

### CodexDeepAnalyzer

- Codex 是离线优化的核心智能分析执行器。
- Codex 读取 ORAG 数据并执行多轮深度检索，判断当前召回是否足够回答问题。
- Codex 输出结构化 JSON，包括召回质量、失败类型、置信度、最终答案、证据、缺失证据、深度检索步骤和推荐动作。
- Codex 只能生成候选条目，不能直接写主知识库、不能直接发布、不能绕过证据校验。

### DeepKBSearch Tools

ORAG 为 Codex 暴露受控只读工具，而不是让 Codex 直接访问任意数据库写能力。

- `search_chunks_by_text`: 基于 lexical/FTS 查询 chunks。
- `search_chunks_by_vector`: 基于 embedding 查询 chunks。
- `get_chunk_neighbors`: 读取同文档相邻 chunks。
- `get_document_chunks`: 读取指定 document 的 chunk 列表。
- `get_related_graph_chunks`: 读取图扩展相关 chunks。
- `get_eval_results_by_question`: 查询相似问题的历史评估结果。
- `get_existing_optimization_items`: 查询已有优化条目，避免重复沉淀。
- `replay_recall_with_query`: 使用改写后的 query 重放召回。

工具层治理：

- 每个工具按 tenant、run、question cluster 和 Codex worker 设置 QPS、并发、token、返回行数和总耗时配额。
- 工具结果默认截断并保留摘要，大文本读取必须分页，避免单次请求拖垮数据库或模型上下文。
- 工具调用必须写入 audit event，记录输入摘要、输出摘要、资源消耗和错误类型。

### EvidenceValidator

- 校验 Codex 输出中的 evidence 是否属于目标知识库。
- 校验 source chunk 和 source document 是否存在且未过期。
- 校验 evidence quote 是否能在 chunk 文本中定位。
- 校验 source 引用的 `doc_version` 和 `chunk_content_hash` 是否与当前内容指纹一致。
- 校验 final answer 的关键结论是否有 evidence 支撑。
- 对 `answer_item` 增加结论级校验，由独立模型或独立 judge 判断每个关键结论是否可由 evidence quote 推出。
- 校验重复条目、低置信条目、无证据条目和与证据矛盾的条目。
- 校验失败时将条目转为 rejected、needs_review 或 knowledge_gap。
- 支持批量 re-validate 流程，按 tenant、kb、doc、chunk hash 或 item status 扫描并重新校验证据。
- 整库重建、document re-ingest、chunker 配置变化、embedding 版本变化或 graph rebuild 必须触发全量 re-verify。

### OptimizationLibrary

- 独立存储优化条目，不直接写入主知识库。
- 条目类型包括 `answer_item`、`query_rewrite_item` 和 `knowledge_gap_item`。
- 每个条目绑定 run id、question cluster id、source chunks、source docs、Codex analyzer report、validation report 和 eval report。
- 支持状态流转、审计事件、人工审核和发布记录。
- 发布的 `query_rewrite_item` 必须限定 tenant、knowledge base、意图和可选 profile 作用域。
- 单条 rewrite 规则必须支持快速禁用，并记录禁用原因和回滚操作者。

### ShadowOptimizationRetriever

- 查询链路增加优化库影子召回器。
- 默认 shadow 模式只记录命中，不注入线上 context，不改变答案。
- 在实验配置允许时，优化库候选可作为额外召回源进入 context pack，并标记来源为 `optimization_library`。
- 记录 query、命中的 optimization item、相似度、潜在引用变化、潜在答案覆盖和延迟成本。
- Shadow event 是高基数字段，必须分区、TTL、采样和归档，不能无限写入单表。
- Shadow event 写入失败不影响线上查询，失败计入 metrics 并按采样日志记录。

### EvalRegressionRunner

- 每次离线任务完成后，基于生成的优化条目构造 eval dataset。
- 使用现有 eval runner 分别运行 baseline 和 with-optimization 两套配置。
- baseline 只使用主知识库召回；with-optimization 启用优化库召回。
- 输出 `recall lift`、`answer quality lift`、`citation coverage lift`、`latency delta`、`token cost delta` 和 `hallucination risk`。
- 只有通过回归阈值的条目才允许进入 `regression_passed`。
- `query_rewrite_item` 发布前必须跑全量回归，不只评估触发问题，避免改写规则污染其他意图。
- 回归失败的条目进入 `regression_failed`，可修改后重新进入 verified 或直接 rejected。

## 端到端流程

1. 夜间 scheduler 创建 `offline_knowledge_run`。
2. HistoryQuestionExtractor 抽取历史问题和相关 trace/eval 信息。
3. QuestionClusterer 归一化、去重、聚类，生成 `offline_question_clusters`。
4. RecallReplayer 使用当前配置对每个问题簇做召回重放。
5. CodexDeepAnalyzer 读取 baseline recall results 和 ORAG 数据。
6. Codex 根据问题动态执行多轮深度检索。
7. Codex 判断召回质量并输出结构化分析报告。
8. EvidenceValidator 校验证据、答案、来源和重复性。
9. OptimizationItemBuilder 生成优化条目、query rewrite 建议或知识缺口记录。
10. 状态机根据校验结果流转到 verified、needs_review、knowledge_gap 或 rejected。
11. verified 条目进入 shadow retrieval。
12. EvalRegressionRunner 运行 baseline 与 with-optimization 成对回归评估。
13. 回归通过后条目进入 regression_passed，可按配置或人工操作进入 published。
14. 回归失败进入 regression_failed，人工或后续 run 可重新分析、降级为 needs_review 或 rejected。
15. 源内容变化、规则失效或运营下线时，条目进入 stale 或 deprecated。

## Codex 多轮深度检索

多轮深度检索交给 Codex 执行，ORAG 不写死固定轮次策略。ORAG 只提供受控只读工具、成本上限、最大步数和审计记录。

Codex 可以动态组合以下动作：

- 使用原问题执行 hybrid recall。
- 基于用户问题抽取实体、术语、模块名、错误码、接口名和配置键。
- 生成关键词查询并调用 text search。
- 生成语义改写查询并调用 vector search。
- 使用同义词、别名、英文术语和中文表达互转做召回。
- 读取候选 chunk 的邻居 chunks，处理 chunk boundary 导致的漏召。
- 读取同文档完整 chunk 列表，定位答案上下文。
- 使用图关系扩展相关 chunks。
- 对照相似问题的 eval results 和历史答案。
- 查询已有 optimization items，避免重复创建。
- 使用改写 query 重放召回，判断是否适合沉淀 query rewrite item。

Codex 停止条件：

- 找到足够证据支撑答案。
- 确认知识库内没有答案。
- 问题歧义，缺少必要上下文。
- 达到最大检索步数。
- 达到 token 或成本上限。
- 发现已有等价优化条目。

## Codex 输入与输出

### 输入

```json
{
  "canonical_question": "string",
  "sample_questions": ["string"],
  "kb_id": "string",
  "baseline_recall_results": {},
  "historical_answer": "string",
  "historical_citations": [],
  "trace_summaries": [],
  "kb_metadata": {},
  "constraints": {
    "max_deep_search_steps": 12,
    "max_tokens": 20000,
    "read_only": true
  }
}
```

### 输出

```json
{
  "recall_quality": "hit | partial_hit | miss | bad_answer | no_answer_in_kb | ambiguous | duplicate",
  "failure_type": "keyword_mismatch | semantic_gap | chunk_boundary | rerank_error | graph_missing | generation_error | knowledge_gap | unclear_question",
  "confidence": 0.0,
  "final_answer": "string",
  "evidence": [
    {
      "chunk_id": "string",
      "doc_id": "string",
      "quote": "string",
      "supports": "string"
    }
  ],
  "missing_evidence": "string",
  "deep_search_steps": [
    {
      "step": 1,
      "tool": "string",
      "query": "string",
      "observation": "string",
      "decision": "string"
    }
  ],
  "recommended_action": "ignore | create_answer_item | create_query_rewrite_item | create_knowledge_gap_item | needs_review"
}
```

## 召回质量分类

- `hit`: 当前召回已经命中足够证据，不需要创建优化条目。
- `partial_hit`: 当前召回相关但不完整，可以创建优化条目或 query rewrite 建议。
- `miss`: 知识库中存在答案但当前召回没有命中关键 item，需要创建优化条目。
- `bad_answer`: 召回证据足够但生成答案错误，归因到生成、context pack 或 prompt。
- `no_answer_in_kb`: Codex 深度读取后仍无证据，生成知识缺口记录。
- `ambiguous`: 问题语义不清或上下文不足，进入待审核，不自动优化。
- `duplicate`: 已有等价优化条目，不重复创建。

## 优化条目类型

### answer_item

用于沉淀有证据支撑的问答型优化条目。`answer_item` 默认只做召回增强和生成引导，最终答案仍由 RAG generation 基于 context 中的真实 chunk 生成，不能将 `final_answer` 直接作为用户答案注入。

- 必须包含 final answer。
- 必须包含 source chunk ids。
- 必须包含 evidence quote。
- 可以参与 shadow retrieval。
- 回归通过后可正式参与优化召回。
- context pack 中必须优先放入真实 source chunks，`final_answer` 只能作为辅助摘要、rerank hint 或 prompt-side guidance。
- 必须通过结论级校验，证明 final answer 的关键结论可由 evidence quote 推出。
- 必须保存 `doc_version` 和 `chunk_content_hash`，后续校验按内容指纹判断是否仍有效。

### query_rewrite_item

用于沉淀用户表达和知识库表达之间的桥接规则。

- 适用于 keyword mismatch、术语别名、中文英文表达差异、简称全称差异。
- 必须记录触发问题、改写 query、改写后命中的证据。
- 不直接作为答案注入 context，而是影响召回 query expansion。
- 必须限定 tenant、knowledge base、意图和可选 profile 作用域。
- 发布前必须进行全量回归，验证该 rewrite 不会降低非触发问题的召回或答案质量。
- 必须支持单条规则快速禁用，禁用后立即从 query expansion 运行时配置中移除。

### knowledge_gap_item

用于记录用户问过但知识库没有答案的问题。

- 不参与召回。
- 用于文档补充、知识库建设和运营报表。
- 必须记录 Codex 深度检索过程，说明为什么判断为知识缺口。

## 状态机

```text
candidate
   |
   v
evidence_validating
   |
   +--> rejected
   +--> knowledge_gap
   +--> needs_review
   +--> verified

verified
   |
   v
shadow_enabled
   |
   +--> regression_passed
   +--> regression_failed

regression_failed
   |
   +--> needs_review
   +--> rejected
   +--> verified

regression_passed
   |
   v
published
   |
   +--> stale
   +--> deprecated

stale
   |
   +--> evidence_validating
   +--> deprecated
```

状态说明：

- `candidate`: Codex 生成候选条目，尚未校验证据。
- `evidence_validating`: 系统正在校验证据、chunk、doc 和 answer consistency。
- `verified`: 证据充分，可进入 shadow retrieval。
- `shadow_enabled`: 参与影子召回，但不影响线上答案。
- `regression_failed`: 离线回归未达到收益阈值、引入质量退化或风险超限。
- `regression_passed`: 离线 eval runner 验证有收益且无明显风险。
- `published`: 允许正式参与优化召回。
- `needs_review`: 证据有价值但自动判断不足，需要人工确认。
- `knowledge_gap`: 知识库中没有答案，需要补文档。
- `rejected`: 无效、重复、幻觉或证据不足。
- `stale`: 源文档或 chunk 变化后需要重新验证。
- `deprecated`: 条目被运营下线、规则失效、知识被替代或长期无命中后归档。

终态和回退：

- `rejected` 是默认终态，但人工重开或新证据出现时可回到 `candidate`。
- `knowledge_gap` 是业务终态，补充文档后可转回 `candidate` 重新分析。
- `deprecated` 是归档终态，默认不可自动恢复，只允许人工复制为新 candidate。
- `stale` 不是终态，必须通过批量 re-validate 回到 `evidence_validating` 或转为 deprecated。
- `regression_failed` 不是终态，可通过调整条目、缩小 rewrite 作用域或补充证据后回到 verified。

## 数据模型

### offline_knowledge_runs

```sql
CREATE TABLE offline_knowledge_runs (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  status TEXT NOT NULL,
  kb_id TEXT NOT NULL,
  window_start TIMESTAMP NOT NULL,
  window_end TIMESTAMP NOT NULL,
  config_hash TEXT NOT NULL,
  config_json JSONB NOT NULL,
  total_questions INT DEFAULT 0,
  total_clusters INT DEFAULT 0,
  processed_clusters INT DEFAULT 0,
  created_items INT DEFAULT 0,
  verified_items INT DEFAULT 0,
  rejected_items INT DEFAULT 0,
  error TEXT,
  started_at TIMESTAMP NOT NULL,
  finished_at TIMESTAMP,
  UNIQUE (tenant_id, kb_id, window_start, window_end, config_hash)
);
```

### offline_question_clusters

```sql
CREATE TABLE offline_question_clusters (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  run_id TEXT NOT NULL,
  kb_id TEXT NOT NULL,
  canonical_question TEXT NOT NULL,
  normalized_question TEXT NOT NULL,
  question_hash TEXT NOT NULL,
  embedding_ref TEXT,
  embedding_json JSONB,
  occurrence_count INT NOT NULL,
  sample_questions_json JSONB NOT NULL,
  trace_ids_json JSONB NOT NULL,
  baseline_recall_json JSONB,
  created_at TIMESTAMP NOT NULL,
  UNIQUE (tenant_id, kb_id, question_hash)
);
```

### optimization_items

```sql
CREATE TABLE optimization_items (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  run_id TEXT NOT NULL,
  kb_id TEXT NOT NULL,
  question_cluster_id TEXT NOT NULL,
  item_type TEXT NOT NULL,
  status TEXT NOT NULL,
  canonical_question TEXT NOT NULL,
  final_answer TEXT,
  recall_quality TEXT NOT NULL,
  failure_type TEXT,
  confidence DOUBLE PRECISION NOT NULL,
  source_chunk_ids_json JSONB NOT NULL,
  source_doc_ids_json JSONB NOT NULL,
  source_fingerprints_json JSONB NOT NULL,
  evidence_json JSONB NOT NULL,
  deep_search_steps_json JSONB NOT NULL,
  analyzer_report_json JSONB NOT NULL,
  validation_report_json JSONB,
  eval_report_json JSONB,
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL,
  published_at TIMESTAMP
);
```

### optimization_item_events

```sql
CREATE TABLE optimization_item_events (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  item_id TEXT NOT NULL,
  event_type TEXT NOT NULL,
  operator TEXT,
  payload_json JSONB NOT NULL,
  created_at TIMESTAMP NOT NULL
);
```

### shadow_retrieval_events

```sql
CREATE TABLE shadow_retrieval_events (
  id TEXT NOT NULL,
  tenant_id TEXT NOT NULL,
  trace_id TEXT NOT NULL,
  kb_id TEXT NOT NULL,
  query TEXT NOT NULL,
  optimization_item_id TEXT NOT NULL,
  score DOUBLE PRECISION NOT NULL,
  would_inject BOOLEAN NOT NULL,
  would_change_context BOOLEAN NOT NULL,
  latency_ms INT NOT NULL,
  created_at TIMESTAMP NOT NULL,
  PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);
```

`source_fingerprints_json` 保存每个证据来源的内容指纹，格式如下：

```json
[
  {
    "doc_id": "doc_xxx",
    "doc_version": "v12",
    "chunk_id": "chunk_xxx",
    "chunk_content_hash": "sha256:..."
  }
]
```

### 索引与保留策略

```sql
CREATE INDEX idx_offline_runs_tenant_status
  ON offline_knowledge_runs (tenant_id, status, started_at DESC);

CREATE INDEX idx_question_clusters_tenant_kb_hash
  ON offline_question_clusters (tenant_id, kb_id, question_hash);

CREATE INDEX idx_optimization_items_tenant_status
  ON optimization_items (tenant_id, status, updated_at DESC);

CREATE INDEX idx_optimization_items_tenant_kb
  ON optimization_items (tenant_id, kb_id, updated_at DESC);

CREATE INDEX idx_optimization_items_tenant_type
  ON optimization_items (tenant_id, item_type, updated_at DESC);

CREATE INDEX idx_shadow_events_trace_created
  ON shadow_retrieval_events (tenant_id, trace_id, created_at DESC);

CREATE INDEX idx_shadow_events_created
  ON shadow_retrieval_events (created_at DESC);
```

- `shadow_retrieval_events` 按日或按周分区，默认 TTL 为 7 到 30 天。
- TTL 到期后聚合为小时级或天级统计表，再删除明细分区。
- 高流量租户可开启采样写入，只保留命中摘要和异常样本。
- API 查询默认限制时间窗口，禁止无时间范围扫描 shadow events。
- 如果 run 覆盖所有知识库，`kb_id` 使用固定哨兵值 `__all__`，避免 nullable unique 约束失效。

## 管理 API

- `POST /v1/offline-knowledge/runs`: 手动触发离线整理任务。
- `GET /v1/offline-knowledge/runs`: 查询任务列表。
- `GET /v1/offline-knowledge/runs/:id`: 查询任务详情、配置和统计。
- `GET /v1/offline-knowledge/runs/:id/questions`: 查看本次抽取和聚类的问题。
- `GET /v1/optimization-items`: 查询优化条目，支持按状态、知识库、质量分类过滤。
- `GET /v1/optimization-items/:id`: 查看条目详情、证据、Codex 分析过程。
- `POST /v1/optimization-items/:id/verify`: 人工确认条目。
- `POST /v1/optimization-items/:id/reject`: 拒绝条目。
- `POST /v1/optimization-items/:id/enable-shadow`: 启用 shadow retrieval。
- `POST /v1/optimization-items/:id/publish`: 发布到正式优化召回。
- `POST /v1/optimization-items/:id/disable`: 快速禁用单条 published 或 shadow_enabled 条目。
- `POST /v1/optimization-items/:id/revalidate`: 重新校验单条优化条目。
- `POST /v1/optimization-items/revalidate`: 按 tenant、kb、doc、chunk hash 或状态批量 re-validate。
- `GET /v1/optimization-items/:id/eval-results`: 查看该条目的回归评估结果。

## Metrics

- `offline_knowledge_run_total`: 夜间离线整理任务总数。
- `offline_knowledge_run_duration_seconds`: 任务耗时。
- `offline_knowledge_questions_extracted_total`: 抽取历史问题数。
- `offline_knowledge_question_clusters_total`: 聚类后问题数。
- `offline_knowledge_replay_total`: 召回重放次数。
- `offline_knowledge_codex_analysis_total`: Codex 分析次数。
- `offline_knowledge_codex_analysis_errors_total`: Codex 分析失败次数。
- `offline_knowledge_deep_search_steps_total`: Codex 深度检索步数。
- `offline_knowledge_evidence_validation_total`: 证据校验次数。
- `offline_knowledge_evidence_validation_errors_total`: 证据校验失败次数。
- `optimization_items_created_total`: 生成优化条目数。
- `optimization_items_verified_total`: 证据校验通过数。
- `optimization_items_rejected_total`: 拒绝条目数。
- `optimization_items_stale_total`: 过期条目数。
- `optimization_items_regression_failed_total`: 回归失败条目数。
- `optimization_items_deprecated_total`: 归档条目数。
- `optimization_revalidate_total`: 重新校验次数。
- `optimization_revalidate_errors_total`: 重新校验失败次数。
- `optimization_shadow_hit_total`: shadow retrieval 命中数。
- `optimization_shadow_write_dropped_total`: shadow event 因限流、采样或写入失败被丢弃次数。
- `optimization_shadow_latency_seconds`: shadow retrieval 延迟。
- `optimization_recall_lift`: 召回提升。
- `optimization_answer_quality_lift`: 答案质量提升。
- `optimization_citation_coverage_lift`: 引用覆盖提升。
- `optimization_hallucination_risk_total`: 疑似幻觉或证据不足次数。

## 配置

```yaml
maintenance:
  offline_knowledge_organizer:
    enabled: true
    schedule: "0 2 * * *"
    lookback_days: 7
    max_questions_per_run: 500
    max_clusters_per_run: 200
    max_codex_concurrency: 4
    max_codex_deep_search_steps: 12
    max_codex_tokens_per_question: 20000
    max_tool_qps_per_tenant: 20
    max_tool_rows_per_call: 50
    max_replay_concurrency: 8
    max_eval_concurrency: 4
    min_question_occurrence: 2
    long_tail_sampling_rate: 0.05
    explicit_negative_feedback_boost: 10
    min_verify_confidence: 0.8
    min_publish_confidence: 0.9
    evidence_validation_enabled: true
    conclusion_judge_enabled: true
    shadow_retrieval_enabled: true
    shadow_inject_enabled: false
    shadow_event_ttl_days: 14
    shadow_event_sampling_rate: 1.0
    auto_publish_enabled: false
    regression_eval_enabled: true
    full_regression_for_rewrite_enabled: true
    min_recall_lift: 0.05
    min_answer_quality_lift: 0.03
    max_latency_delta_ms: 300
```

## 与现有 ORAG 能力的关系

- RAG 查询链路继续复用现有 query rewrite、多查询、HyDE、hybrid retrieval、RRF、graph retrieval、rerank 和 context pack。
- 召回重放使用当前线上配置，确保分析的是当前系统真实表现。
- Trace 数据继续作为历史问题和诊断 evidence 的主要来源。
- Eval runner 继续作为质量回归的权威执行器。
- Optimizer 和 evaluation 的持久化模式可作为 offline knowledge run、item、report 的设计参考。
- Metrics 继续通过现有 `/metrics` 输出 Prometheus text format。

## 安全与治理

- Codex 只使用受控只读工具读取 ORAG 数据。
- 所有后台任务、工具调用、API 查询和 shadow events 都必须带 `tenant_id`，禁止跨租户扫描。
- Codex 工具调用需要记录 run id、question cluster id、tool name、query、结果摘要和耗时。
- Codex 输出需要 schema 校验，字段缺失或类型错误直接 rejected。
- evidence quote 必须能在源 chunk 中定位。
- source chunk 必须属于目标 knowledge base。
- source 引用必须通过 `doc_version` 和 `chunk_content_hash` 校验，内容指纹不一致时进入 stale。
- `answer_item` 的 final answer 不能直接返回给用户，只能作为召回增强、rerank hint、摘要提示或生成引导。
- 结论级 judge 必须独立于 Codex 分析执行，避免同一模型自证。
- final answer 与 evidence 矛盾时拒绝入库。
- 源文档或 chunk 更新、整库重建、chunker 配置变化、embedding 版本变化后，相关优化条目进入 stale 并触发批量 re-validate。
- query rewrite 规则必须限定作用域，回归失败或线上异常时支持单条规则快速禁用。
- shadow event 明细必须有分区、TTL、采样和归档策略，避免高流量租户爆表。
- 敏感 query 和 evidence 需要遵循 trace retention、脱敏和截断策略。

## 验收标准

- 可以手动触发一次离线知识整理 run，并查询 run 详情。
- run 可以从历史 traces 或 evaluation results 抽取问题并生成问题簇。
- run 可以抽取用户显式负反馈，并将长尾单次问题进入低优先队列。
- 同一 tenant、kb、窗口和配置重复触发时返回同一个 run，不重复执行。
- 每个问题簇可以完成召回重放并保存 baseline recall results。
- 召回重放走只读副本或隔离资源池，工具层具备限流、配额和审计记录。
- Codex 可以通过受控只读工具读取 ORAG 数据并完成多轮深度检索。
- Codex 输出的每个 deep search step 都会落库。
- EvidenceValidator 可以拒绝无 source chunk、无 quote、quote 不存在、内容指纹不一致或答案与证据矛盾的条目。
- `answer_item` 通过独立结论级 judge 校验，且最终答案仍由真实 chunks 生成。
- `query_rewrite_item` 发布前通过全量回归，并支持单条规则快速禁用。
- 核心表包含 `tenant_id`，并为 `status`、`kb_id`、`question_hash`、`item_type` 和 shadow event 查询字段建立索引。
- `shadow_retrieval_events` 具备分区、TTL、采样和归档策略。
- 整库重建或源文档重入库可以触发批量 re-validate 和全量 re-verify。
- verified 优化条目可以进入 shadow retrieval。
- shadow retrieval 默认不改变线上答案，但会记录 shadow hit 和潜在上下文变化。
- Eval runner 可以对 baseline 和 with-optimization 做成对回归评估。
- 回归报告包含 recall lift、answer quality lift、citation coverage lift、latency delta 和 hallucination risk。
- `/metrics` 输出离线整理、优化条目、shadow retrieval 和回归评估相关指标。
- API 契约测试覆盖新增 run、items、状态流转和 eval result 查询接口。

## 实现边界

- ORAG 内部实现调度、抽取、重放、存储、状态机、API、metrics、shadow retriever 和 eval regression。
- Codex 作为离线智能分析执行器，通过受控工具读取 ORAG 数据和调用检索接口。
- Codex 不直接修改主知识库、不直接发布条目、不绕过证据校验。
- 优化库是独立数据层，正式召回时以明确来源参与，方便观测、回滚和灰度。
