# LLM-as-Judge 与目标驱动优化器 Spec

## Why
当前项目已具备确定性评估和简单 `profile × top_k` 优化器，但无法稳定评估答案忠实度、证据支撑、事实幻觉、完整性，也无法按用户目标自动比较和优化 RAG 策略。基于方案文档 `docs/plans/2026-07-04-llm-judge-and-goal-driven-optimizer.md`，需要将 LLM-as-Judge、QAG、pairwise 比较、目标函数、异步优化器和外部 harness 拆成边界清晰的模块逐步落地。

## What Changes
- 新增数据集 split、expected evidence、human scores、item weight 等评估元数据，支持 `train/eval/holdout/gold`。
- 新增 metric registry，对所有确定性、Judge、QAG、harness 指标名做白名单校验。
- 新增 LLM-as-Judge 模块，支持 pairwise judge、A/B 顺序交换、coarse absolute evidence-check、ensemble/repeat、raw/parsed 响应分离、token/cost 记录。
- 新增 QAG Score 模块，基于答案 claim 生成验证问题、从上下文作答并核对 `supported/contradicted/unverifiable`。
- 新增 gold-set 校准模块，按维度计算 Spearman、Cohen’s κ、QAG claim coverage，并使用分维度阈值与人工复核豁免。
- 新增 objective 模块，支持 pairwise win-rate、受限表达式、约束、tie-breaker、固定预算归一化和 bootstrap 显著性。
- 新增异步 optimizer 模块，支持 run/candidate 持久化、checkpoint、cancel/resume、holdout 复评、预算与 provider rate limit。
- 新增 external harness runner，强制 argv-array 执行、可执行文件 allowlist、stdout/stderr/env/argv/artifact 脱敏、指标白名单校验。
- 新增可回滚数据库迁移和 API/OpenAPI/文档/curl 示例，保持旧评估接口兼容。
- **BREAKING**: `POST /v1/optimizations` 对新目标驱动优化器采用异步语义，提交返回 `202` 和 `run_id`，调用方需要轮询 `GET /v1/optimizations/{id}`；旧 `profiles/top_ks` 请求作为兼容快捷方式保留。

## Impact
- Affected specs: evaluation、dataset、optimizer、observability、api-contract、operations、security
- Affected code: `internal/dataset`、`internal/eval`、新增 `internal/optimizer`、`internal/storage/postgres`、`internal/http/router.go`、`internal/app/app.go`、`api/openapi.yaml`、`migrations/`、`docs/evaluation.md`、`docs/api.md`、`examples/curl/`

## ADDED Requirements
### Requirement: 数据集 split 与校准元数据
系统 SHALL 支持 evaluation item 的 `split`、`weight`、`expected_evidence`、`human_scores` 字段，并在读取/写入时保持 tenant 隔离和旧数据兼容。

#### Scenario: 追加带 split 的评估样本
- **WHEN** 用户向数据集追加带 `split=gold`、`expected_evidence` 和 `human_scores` 的样本
- **THEN** 系统 SHALL 持久化这些字段，并在后续评估、校准和优化中按 split 正确筛选

### Requirement: 指标注册与白名单校验
系统 SHALL 为所有指标建立集中 registry，任何 score map 在聚合、落库、objective 计算前都必须校验指标名。

#### Scenario: harness 返回未知指标
- **WHEN** 外部 harness 返回未注册指标名
- **THEN** 系统 SHALL 拒绝该指标进入评分和持久化，并返回可诊断错误

### Requirement: Pairwise LLM-as-Judge
系统 SHALL 支持 Judge 在同一 query/evidence 下比较两个答案，并通过 A/B 顺序交换降低位置偏差。

#### Scenario: A/B 顺序交换一致
- **WHEN** Judge 原始顺序和交换顺序均判定同一候选更优
- **THEN** 系统 SHALL 记录稳定 pairwise preference，并计入 win-rate

#### Scenario: A/B 顺序交换冲突
- **WHEN** 两次顺序交换结果互相矛盾
- **THEN** 系统 SHALL 标记该 pairwise 结果为 unstable，不直接作为强晋级依据

### Requirement: Evidence-check 与 QAG Score
系统 SHALL 支持 evidence-check Judge 和 QAG Score，用于评估 faithfulness、groundedness、citation_support、hallucination 和 answer completeness。

#### Scenario: QAG 发现上下文矛盾
- **WHEN** QAG 从答案 claim 生成问题后，context-only answer 与原 claim 矛盾
- **THEN** 系统 SHALL 将该 claim 标记为 `contradicted`，降低 `qag_score`

### Requirement: Judge 校准与质量门禁
系统 SHALL 对 `gold` split 做人工标注校准，按维度报告 Spearman、Cohen’s κ 和 QAG claim coverage，并按 evidence-checkable 与 subjective 维度使用不同阈值。

#### Scenario: 主观维度 κ 未达标但人工豁免
- **WHEN** `answer_relevance` κ 低于目标阈值但人工复核允许用于报告
- **THEN** 系统 SHALL 允许报告该指标，但不得自动用于 optimizer promotion，除非记录明确 waiver

### Requirement: 受限 Objective 表达式
系统 SHALL 使用受限表达式引擎，仅允许白名单指标变量和 `+ - * / ()`，禁止函数调用、属性访问、索引、插值和未知变量。

#### Scenario: 目标表达式包含函数调用
- **WHEN** 用户提交 `max(faithfulness, 0.8)` 这类函数调用
- **THEN** 系统 SHALL 在解析期拒绝并返回 validation error

### Requirement: 异步目标驱动优化器
系统 SHALL 将目标驱动优化作为异步任务，支持提交、轮询、取消、续跑、checkpoint、预算、限流和 holdout 复评。

#### Scenario: 提交优化任务
- **WHEN** 用户调用 `POST /v1/optimizations`
- **THEN** 系统 SHALL 返回 `202 Accepted`、`run_id` 和轮询/取消/续跑 URL，不阻塞等待所有候选完成

#### Scenario: Worker 中断后续跑
- **WHEN** 优化任务在第 N 个候选后中断
- **THEN** 系统 SHALL 从 checkpoint 继续，跳过已完成候选，只重试未完成候选

### Requirement: External Harness 安全执行
系统 SHALL 使用 argv-array 执行外部 harness，禁止 shell 模板和 `${VAR}` 插值，并对 env、argv、stdout、stderr、artifact manifest 做脱敏。

#### Scenario: harness 配置包含 shell 字符串
- **WHEN** 用户提交 `command: "codex-cli eval ${DATASET_PATH}"`
- **THEN** 系统 SHALL 拒绝该配置，要求使用 `argv` 数组

### Requirement: Token 与 Cost 可观测性
系统 SHALL 分别记录 RAG 生成、Judge 调用、QAG 生成、QAG context answering、external harness 的 token usage 和 cost，并汇总到 candidate/run。

#### Scenario: Judge 调用超过预算
- **WHEN** `max_judge_calls` 或 `max_cost_usd` 达到上限
- **THEN** 系统 SHALL 停止调度新 Judge 调用，持久化当前 checkpoint，并将 run 标记为 budget stopped

### Requirement: 可回滚数据库迁移
系统 SHALL 将 schema 改动拆分为小迁移，并为每个迁移提供 down rollback。

#### Scenario: schema-only 发布需回滚
- **WHEN** 发布新表后尚未启用行为变更且需要回滚
- **THEN** 系统 SHALL 可以通过 down migration 删除新增表、索引和列

## MODIFIED Requirements
### Requirement: 评估运行
系统 SHALL 保持现有 rule-based evaluation 行为兼容；当请求包含 Judge/QAG 配置时，额外执行 LLM Judge、QAG、校准和明细持久化。

### Requirement: 优化器
系统 SHALL 保留旧 `profiles/top_ks` 简单优化语义作为兼容快捷方式，同时新增目标驱动异步优化能力。

### Requirement: API 查询
系统 SHALL 保持 `GET /v1/evaluations/{id}` 默认返回运行级汇总；当 `include_items/include_judge/include_pairwise` 启用时返回明细。

## REMOVED Requirements
### Requirement: Shell command 模板执行 harness
**Reason**: shell 模板和变量插值存在注入面，即使有 allowlist 仍不安全。
**Migration**: 外部 harness 配置迁移为 argv-array，使用 `exec.CommandContext(ctx, argv[0], argv[1:]...)` 或等价机制。
