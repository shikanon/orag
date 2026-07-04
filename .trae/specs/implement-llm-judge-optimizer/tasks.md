# Tasks
- [x] Task 1: 建立数据集评估元数据与可回滚迁移。
  - [x] SubTask 1.1: 新增 split、weight、expected_evidence、human_scores 的 dataset model、memory repo、PostgreSQL repo 支持。
  - [x] SubTask 1.2: 拆分创建 up/down migration，覆盖 dataset item metadata、judge 表、optimizer 表、harness 表。
  - [x] SubTask 1.3: 编写 dataset 和 migration/repository 测试，验证旧数据兼容、tenant 隔离和 rollback 语义。

- [x] Task 2: 建立 metric registry 与确定性指标白名单。
  - [x] SubTask 2.1: 在 `internal/eval` 增加指标常量、metric registry、score map 校验函数。
  - [x] SubTask 2.2: 将现有 `ScoreItemWithOptions`、Runner 聚合、optimizer candidate 映射接入白名单校验。
  - [x] SubTask 2.3: 编写单元测试覆盖合法指标、未知指标、拼写漂移和 harness/judge 输出拒绝场景。

- [x] Task 3: 实现 Judge 类型、Prompt 渲染和安全 JSON 解析。
  - [x] SubTask 3.1: 新增 JudgeConfig、JudgeInput/Output、PairwiseJudgeInput/Output、QAGOutput、token/cost 类型。
  - [x] SubTask 3.2: 实现 pairwise、absolute evidence-check、QAG 的 prompt renderer，要求最终严格 JSON。
  - [x] SubTask 3.3: 实现 raw_response TEXT 与 parsed_response JSONB 的解析模型，保留结构化 rationale/findings，不公开 raw CoT。
  - [x] SubTask 3.4: 编写 fake LLM 单元测试覆盖合法 JSON、畸形 JSON、hash/version、token/cost 和指标白名单。

- [x] Task 4: 实现 Pairwise Judge、A/B 顺序交换和 ensemble/repeat 聚合。
  - [x] SubTask 4.1: 实现 PairwiseJudge 接口，支持原始顺序与交换顺序双跑。
  - [x] SubTask 4.2: 实现多数票、偏好强度平均、unstable 标记和 ensemble/repeat 聚合。
  - [x] SubTask 4.3: 接入 provider-specific timeout、429/503 backoff、jitter、retry cap 和 circuit breaker。
  - [x] SubTask 4.4: 编写测试覆盖稳定胜出、顺序偏差冲突、ensemble 分歧、限流退避和预算停止。

- [x] Task 5: 实现 QAG Score 与 QAG 覆盖质量检查。
  - [x] SubTask 5.1: 实现 claim/question generation、context-only answer、support 分类和 qag_score 汇总。
  - [x] SubTask 5.2: 输出 qag_claim_coverage、qag_question_count、qag_unverifiable_rate 等质量指标。
  - [x] SubTask 5.3: 编写测试覆盖 supported、contradicted、unverifiable、漏掉关键 claim 和指标白名单校验。

- [x] Task 6: 实现 Judge 校准与 gold-set 质量门禁。
  - [x] SubTask 6.1: 实现 Spearman、Cohen’s κ、QAG claim coverage 校准计算。
  - [x] SubTask 6.2: 实现 evidence-checkable 与 subjective 维度的分层阈值和 human-review waiver。
  - [x] SubTask 6.3: 编写测试覆盖达标、不达标、waiver、禁止自动 promotion 的场景。

- [x] Task 7: 扩展评估 Runner 与评估明细持久化。
  - [x] SubTask 7.1: 将 Judge/QAG 作为可选模块接入现有 Runner，保持 rule-based 默认行为兼容。
  - [x] SubTask 7.2: 新增 judge_runs、judge_results、pairwise_judge_results、judge_calibration_runs 的 repository 方法。
  - [x] SubTask 7.3: 实现 `GET /v1/evaluations/{id}?include_items=true&include_judge=true&include_pairwise=true` 明细查询。
  - [x] SubTask 7.4: 编写 eval/repository/http 测试覆盖兼容行为、明细查询、raw/parsed 分离和 token/cost。

- [x] Task 8: 实现 objective 模块与受限表达式引擎。
  - [x] SubTask 8.1: 新增 `internal/optimizer/objective.go` 和 `expression.go`，实现白名单变量和受限算子。
  - [x] SubTask 8.2: 实现 pairwise win-rate、constraints、tie-breaker、fixed-budget latency/cost normalization。
  - [x] SubTask 8.3: 实现 paired bootstrap 或等价显著性判断。
  - [x] SubTask 8.4: 编写测试覆盖未知变量、函数调用拒绝、属性访问拒绝、约束失败、显著性晋级。

- [x] Task 9: 实现 candidate search space 和 dependency-aware sampling。
  - [x] SubTask 9.1: 新增 CandidateConfig、SearchSpace、SearchStrategy 类型和 deterministic candidate ID/hash。
  - [x] SubTask 9.2: 实现 seeded random、grid 小空间、successive halving 基础流程和 search_space_size 警告。
  - [x] SubTask 9.3: 实现依赖剪枝：chunking/embedding 默认 disabled，reranker provider/model 依赖校验。
  - [x] SubTask 9.4: 编写测试覆盖大笛卡尔积警告、seed 稳定性、依赖剪枝和 candidate ID 幂等。

- [x] Task 10: 实现 internal RAG candidate runner 与临时 namespace 管理。
  - [x] SubTask 10.1: 实现候选配置 overlay，不修改生产 RAG 配置。
  - [x] SubTask 10.2: 对 index-affecting candidate 使用独立 namespace，登记 owner、expires_at、cleanup_status。
  - [x] SubTask 10.3: 实现 completion cleanup 与周期 GC fallback。
  - [x] SubTask 10.4: 编写测试覆盖配置隔离、namespace 登记、cleanup、失败后 GC。

- [x] Task 11: 实现 external harness runner 安全执行。
  - [x] SubTask 11.1: 实现 argv-array runner、executable allowlist、timeout 和工作目录隔离。
  - [x] SubTask 11.2: 实现 env/argv/stdout/stderr/artifact manifest 脱敏。
  - [x] SubTask 11.3: 解析 harness metrics 并通过 metric registry 校验。
  - [x] SubTask 11.4: 编写测试覆盖 shell 字符串拒绝、allowlist、脱敏、超时、指标拒绝。

- [x] Task 12: 实现异步 optimizer service、checkpoint、cancel/resume。
  - [x] SubTask 12.1: 新增 optimization_runs、optimization_candidates、harness_runs repository 方法。
  - [x] SubTask 12.2: 实现 queued/running/evaluated/judged/scored/promoted/holdout_evaluated/cleanup_done/failed 状态机。
  - [x] SubTask 12.3: 实现 checkpoint、resume、cancel、budget hard stop、provider rate limit 和 cost/token 汇总。
  - [x] SubTask 12.4: 编写 service 测试覆盖异步提交、轮询、取消、续跑、断点恢复、预算停止、holdout 复评。

- [x] Task 13: 更新 HTTP API、OpenAPI、curl 示例和文档。
  - [x] SubTask 13.1: 更新 `POST /v1/evaluations`、`GET /v1/evaluations/{id}` 的 Judge/QAG schema。
  - [x] SubTask 13.2: 将 `POST /v1/optimizations` 改为异步提交，并新增 `GET`、`:cancel`、`:resume` 接口。
  - [x] SubTask 13.3: 更新 OpenAPI、`docs/evaluation.md`、`docs/api.md`、`examples/curl/40_eval.sh`，新增优化示例脚本。
  - [x] SubTask 13.4: 编写 HTTP 和契约测试覆盖新增 API 和兼容请求。

- [x] Task 14: 端到端验证、文档收口和远程 PR。
  - [x] SubTask 14.1: 运行 `CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/eval ./internal/optimizer -v`。
  - [x] SubTask 14.2: 运行 `CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/storage/postgres ./internal/http ./tests/contract -v`。
  - [x] SubTask 14.3: 运行 `CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./...`，若环境限制失败需记录原因并修复可修复问题。
  - [x] SubTask 14.4: 更新任务与检查清单，提交代码，推送远程分支并创建 PR。

# Task Dependencies
- Task 2 depends on Task 1 only for migration-aware persistence checks, but registry implementation can parallel with Task 1.
- Task 3 depends on Task 2.
- Task 4 depends on Task 3.
- Task 5 depends on Task 3.
- Task 6 depends on Task 3 and Task 5.
- Task 7 depends on Task 1, Task 2, Task 4, Task 5 and Task 6.
- Task 8 depends on Task 2.
- Task 9 depends on Task 8.
- Task 10 depends on Task 9.
- Task 11 depends on Task 2 and Task 9.
- Task 12 depends on Task 7, Task 8, Task 9, Task 10 and Task 11.
- Task 13 depends on Task 7 and Task 12.
- Task 14 depends on all implementation tasks.

# Parallelization Notes
- Task 1 and Task 2 can start in parallel.
- Task 4 and Task 5 can run in parallel after Task 3.
- Task 8 and Task 9 can run in parallel with Judge work after metric registry exists.
- Task 10 and Task 11 can run in parallel after Task 9.
