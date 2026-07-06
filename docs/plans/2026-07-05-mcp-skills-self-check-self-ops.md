# MCP 与 Skills 驱动的自检和自运维方案

## 背景与目标

ORAG 已具备 HTTP API、OpenAPI 契约、MCP Server 与面向 Codex、Claude Code、Trae 的 Skill 生成能力。下一步可以把这些 Agent 集成面从“对外调用入口”扩展为“系统自检和自运维入口”，让 Agent 能主动发现能力、执行健康检查、诊断故障、生成运维计划，并把经验沉淀回文档、检查项、测试和生成产物。

目标：

- 基于 MCP 暴露标准化、机器可读的自检和运维工具。
- 基于 Skills 描述 Agent 何时使用、如何使用、边界在哪里、如何验证结果。
- 以能力清单作为单一事实来源，OpenAPI 只是 HTTP request/response facet，避免把 Skill 行为语义强塞进 OpenAPI。
- 默认只读和 dry-run，任何有副作用的运维动作必须显式授权。
- 形成“检查失败 -> 诊断报告 -> 修复计划 -> 验证命令 -> 知识沉淀”的闭环。

## 核心概念

### 自检

自检是系统主动或被 Agent 触发，对自身 API、配置、依赖、数据、任务、契约、生成产物和关键路径进行健康检查与一致性验证。

典型问题：

- API 是否健康，`health`、`ready`、版本和构建信息是否正常。
- 能力清单、OpenAPI facet、MCP tools、Codex/Claude Code/Trae Skills 是否同步。
- PostgreSQL、Qdrant、迁移、索引和集合状态是否满足运行条件。
- 关键路径 smoke test 是否可执行，例如 ingestion、query、eval、Ralph Loop fake downstream。
- trace、日志、指标和错误码是否足够支持问题定位。

### 自运维

自运维是在自检发现异常后，输出结构化诊断、修复建议、执行计划、回滚方案和验证步骤。初期不直接修改系统，只生成 dry-run 计划；后续在用户明确授权后，允许执行低风险动作。

典型动作：

- 生成诊断报告和风险等级。
- 生成可执行 runbook 和验证命令。
- 重新生成 Agent artifacts 并做 drift check。
- 创建 GitHub issue 或 PR，但不自动合并。
- 在运维授权下执行低风险本地命令或触发 CI。

### MCP 与 Skills 分工

- MCP 是能力接口：负责工具发现、参数 schema、返回结构、错误模型、trace 透传和机器可验证调用。
- Skills 是操作认知：负责告诉 Agent 触发条件、使用步骤、安全边界、示例 prompt、失败处理和验证路径。
- 能力清单是单一事实来源：MCP tools、OpenAPI facet、Skills schema 片段、示例和契约测试都从同一源生成。

### SSOT 与生成边界

能力清单必须是 OpenAPI 的超集，而不是 OpenAPI 的别名。

能力清单负责承载：

- HTTP 能力：method、path、request schema、response schema、error schema、auth scheme。
- MCP 能力：tool name、description、input schema、output schema、annotations、trace metadata。
- Skill 行为语义：触发条件、安全边界、调用顺序、失败处理、示例 prompt、风险等级。
- 运维语义：是否只读、是否支持 dry-run、是否需要授权、幂等键、锁粒度、可回滚性。
- 生成元数据：schema version、capability version、生成器版本、产物路径、drift 校验规则。

OpenAPI 只作为能力清单的 HTTP facet 生成或校验，描述 HTTP request/response schema。Skills 中最有价值的行为部分不应试图从 OpenAPI 推导；应在能力清单的 Skill 行为语义字段中维护，并和生成产物一起纳入 drift 校验。

建议关系：

```text
capability manifest
  -> OpenAPI facet
  -> MCP tools
  -> Codex/Claude Code/Trae Skill schema sections
  -> Skill behavior sections
  -> contract and drift checks
```

其中 Skill 的 schema、环境变量、工具名、参数说明可以生成；触发条件、安全边界、调用顺序、失败处理和示例 prompt 来自能力清单中的行为语义字段，不从 OpenAPI 反推。

## 能力分层

1. L0 只读观测层：读取健康状态、配置摘要、版本、依赖状态、队列状态、最近错误、trace 和指标。
2. L1 自检验证层：执行 API 合约检查、MCP/Skill drift 检查、数据库迁移状态检查、依赖连通性检查、关键路径 smoke test。
3. L2 诊断分析层：结合日志、trace、指标、最近变更和测试结果，归因问题类型并生成诊断报告。
4. L3 建议修复层：输出修复建议、命令建议、配置建议、回滚建议、风险等级和验证步骤，但默认不直接修改系统。
5. L4 授权运维层：在用户明确授权下执行有限动作，例如重新生成 Agent 产物、重跑迁移检查、触发 CI、创建 issue 或 PR。
6. L5 闭环优化层：将自检结果沉淀为规则、检查项、回归测试、Skills 文档或 MCP 工具 schema。

## MCP 工具设计

### `orag_check`

用途：统一执行只读自检，避免 `health_check`、`contract_check`、`smoke_test`、`agent_sync_check` 多个语义相近工具互相重叠。具体检查范围通过 `scope` 枚举表达。

建议输入：

```json
{
  "scope": "health | contract | agent_sync | smoke | storage | config | release | all",
  "mode": "focused | broad",
  "overall_deadline_seconds": 120,
  "per_check_timeout_seconds": 30
}
```

建议输出：

```json
{
  "schema_version": "selfcheck.v1",
  "verdict": "pass | fail | blocked",
  "summary": "string",
  "checks": [],
  "trace_id": "string"
}
```

首批 scope：

- `health`：检查 API、数据库、向量库、配置、版本、构建信息和依赖连通性。
- `contract`：运行 OpenAPI facet、API 示例、MCP schema 和 Skill manifest 相关契约检查。
- `agent_sync`：验证能力清单与 `.mcp`、`.codex`、`.claude`、`.trae/skills` 生成产物是否一致。
- `smoke`：执行 MCP stdio `initialize`、`tools/list`、fake downstream `tools/call`、API `health/ready` 等关键路径 smoke。
- `storage`：检查 PostgreSQL、Qdrant、迁移状态、索引和集合状态。
- `config`：检查必需配置项、环境变量、timeout 和安全红线。
- `release`：组合执行发布前只读检查，包括 agent sync、contract、test、vet、build、smoke 的门禁摘要。
- `all`：Broad 模式下执行所有只读检查，Focused 模式下只执行和当前任务直接相关的检查。

`agent_sync` scope 应覆盖：

- MCP tool name、description、input schema、output schema、annotations。
- Codex Skill 输出是否存在且字段完整。
- Claude Code Skill 输出是否存在且字段完整。
- Trae workspace Skill 是否存在且 frontmatter 合法。
- 生成命令 `generate-agent-artifacts --check` 是否能捕获 drift。

### Drift 权威边界

运行时 `orag_check(scope=agent_sync)` 只能作为便利性探针，不能作为发布门禁的权威判定。原因是该 MCP 工具本身也可能来自能力清单生成；如果生成链路或已发布产物过期，它给出的“一致”结论不应被视为可信 gate。

权威 drift 判定必须放在 CI 侧的静态检查中，对已提交文件和当前生成器直接运行，与运行时 MCP Server 无关。

发布门禁建议命令：

```bash
make agent-sync-check
go test ./tests/contract -run TestOpenAPI -v
go test ./internal/mcp ./internal/agentskills ./internal/agentsync
```

### `orag_trace_lookup`

用途：根据 `trace_id` 查询请求链路、错误阶段、下游调用和耗时分布。

输出应包含：

- 请求入口。
- 阶段耗时。
- 下游依赖。
- 错误码和错误消息。
- 关联日志或 artifact。

### `orag_diagnose`

用途：输入症状、trace、错误码或失败命令，输出结构化诊断报告。

建议输入：

```json
{
  "scope": "api | mcp | skills | storage | ingestion | query | eval | deployment",
  "symptom": "string",
  "trace_id": "string",
  "since": "duration",
  "mode": "focused | broad",
  "allow_commands": false
}
```

建议输出：

```json
{
  "status": "completed | failed | blocked",
  "severity": "info | warning | error | critical",
  "verdict": "pass | fail | blocked",
  "summary": "string",
  "findings": [],
  "recommended_actions": [],
  "verification_commands": [],
  "trace_id": "string",
  "artifacts": []
}
```

### 结果信封与 CI 判定

所有自检、诊断和运维计划类工具必须返回稳定结果信封，便于 CI、Agent 和人工报告使用同一套判定规则。

建议结果信封：

```json
{
  "schema_version": "orag.selfops.result.v1",
  "run_id": "selfcheck_20260705_001",
  "capability_version": "2026-07-05",
  "verdict": "pass | fail | blocked",
  "summary": "string",
  "checks": [
    {
      "id": "agent_sync.generated_artifacts_match",
      "title": "Generated MCP and Skill artifacts match manifest",
      "status": "pass | fail | blocked | skipped",
      "severity": "info | warning | error | critical",
      "evidence": [],
      "duration_ms": 1234
    }
  ],
  "findings": [],
  "trace_id": "string",
  "artifacts": []
}
```

稳定 check ID 规则：

- 使用 `<domain>.<object>.<assertion>` 格式，例如 `mcp.tools_list.has_ralph_loop_run`。
- check ID 一经发布不得随意重命名；需要替换时保留 deprecated alias 至少一个版本。
- CI、文档、runbook 和 Skills 引用 check ID，而不是引用易变的人类标题。

severity 到 verdict 的默认映射：

- `critical`：整体 `verdict=fail`，CI 退出码 `1`。
- `error`：整体 `verdict=fail`，CI 退出码 `1`。
- `warning`：默认不阻断，整体可为 `pass`；发布门禁可按 policy 将指定 warning 升级为 fail。
- `info`：不阻断。
- `blocked` status：表示检查未能完成；CI 门禁中默认为退出码 `2`，人工自检中返回 `verdict=blocked`。

退出码约定：

- `0`：所有 gate 级检查通过。
- `1`：存在 fail 级检查。
- `2`：存在 blocked，无法给出可信结论。
- `3`：调用参数、配置或权限错误导致检查未开始。

### 超时与部分结果

自检工具必须同时支持整体 deadline 和单项 timeout。

规则：

- `overall_deadline_seconds` 控制本次工具调用总时长。
- `per_check_timeout_seconds` 控制单个 check 的最大时长。
- 单项超时只将该 check 标记为 `blocked`，并保留已完成检查的结果。
- 整体 deadline 到达时，未完成 check 标记为 `blocked`，工具返回部分结果，不允许挂死。
- 如果 blocked check 属于 CI gate，整体 verdict 为 `blocked`，退出码为 `2`。
- smoke test 必须支持 context cancellation；外部命令必须有 timeout 和进程清理策略。

### `orag_runbook_suggest`

用途：根据诊断结果匹配运维手册，输出下一步操作、风险和验证方式。

### `orag_maintenance_plan`

用途：生成自运维执行计划，包含 dry-run、影响范围、回滚步骤、验证命令和人工确认点。

计划输出必须携带 TOCTOU 防护信息：

```json
{
  "plan_id": "plan_20260705_001",
  "idempotency_key": "selfops:agent-artifacts-regenerate:manifest-sha256",
  "lock_key": "selfops:agent-artifacts",
  "snapshot": {
    "manifest_hash": "sha256:...",
    "git_head": "b2e173f",
    "config_hash": "sha256:...",
    "generated_artifacts_hash": "sha256:..."
  },
  "preconditions": [
    "git_head == snapshot.git_head",
    "manifest_hash == snapshot.manifest_hash",
    "no_uncommitted_changes_outside_declared_paths"
  ],
  "steps": [],
  "rollback": [],
  "verification_commands": []
}
```

要求：

- `snapshot` 记录计划生成时的关键状态，例如 manifest hash、git HEAD、配置摘要、生成产物 hash。
- `preconditions` 明确 apply 前必须重新校验的条件。
- `idempotency_key` 用于重复提交同一动作时返回同一结果或安全跳过。
- `lock_key` 用于单飞锁，防止两个 Agent 并发执行同一类写操作。
- plan 过期或前置条件不满足时，不允许继续 apply，必须重新生成计划。

### `orag_apply_low_risk_action`

用途：仅在授权下执行低风险动作。

执行前必须：

- 校验用户授权范围覆盖 plan 中的全部写动作。
- 重新计算 `snapshot` 中的 hash，并逐条验证 `preconditions`。
- 获取 `lock_key` 对应的单飞锁。
- 检查 `idempotency_key` 是否已有完成记录；如有，返回已有结果而不是重复执行。
- 前置条件漂移时返回 `verdict=blocked`，并提示重新生成 plan。

允许的首批动作：

- 重新生成 Agent artifacts。
- 执行 drift check。
- 执行只读 health/contract/smoke 检查。
- 创建本地诊断报告文件。

禁止的默认动作：

- 自动合并 PR。
- 自动推送生产配置。
- 自动修改数据库数据。
- 自动重启生产服务。
- 绕过 review 直接修改远程 main。

### `orag_create_remediation_issue`

用途：通过 `gh` 创建问题追踪，不手写 GitHub REST/GraphQL client。

## Skills 设计

Skill 数量必须少而边界清晰。`agent-sync`、`release-readiness`、`incident-review` 不作为独立 Skill 并列发布，避免和 self-check/self-diagnose 互相抢触发；它们应作为三个核心 Skill 下的 `mode` 或 workflow。

最终只保留三个互斥 Skill：

- `orag-self-check`：只读检查，不做根因推理，不执行写操作。
- `orag-self-diagnose`：基于检查、trace、日志和错误做诊断，不执行写操作。
- `orag-self-ops`：生成或执行授权运维计划，唯一允许进入写操作链路的 Skill。

### `orag-self-check`

面向系统自检。

触发场景：

- 用户要求“检查系统健康”。
- 用户要求“验证 MCP/Skills 是否同步”。
- 发布前检查。
- CI 失败后的初步定位。

支持 mode：

- `health`：基础健康检查。
- `agent_sync`：能力清单与 MCP/Skills 产物同步检查。
- `contract`：OpenAPI facet、MCP schema、Skill manifest 契约检查。
- `smoke`：关键路径 smoke。
- `release`：发布前只读检查组合，相当于 self-check 的扩展模式。

调用顺序：

1. 发现 MCP tools。
2. 根据 mode 调用 `orag_check(scope, mode)`。
3. 对 `agent_sync` mode 明确提示：运行时结果只是便利探针，CI `make agent-sync-check` 才是发布门禁。
4. 汇总 PASS/FAIL/BLOCKED、稳定 check ID、证据、风险和下一步建议。

### `orag-self-diagnose`

面向故障诊断。

输入：

- 错误日志。
- trace ID。
- 失败命令。
- 用户描述的症状。

输出：

- 根因假设。
- 证据链。
- 严重等级和 verdict。
- 建议操作。
- 验证命令。
- 如果需要写操作，只能建议切换到 `orag-self-ops` 生成 dry-run plan。

### `orag-self-ops`

面向自运维计划。

核心要求：

- 默认生成 dry-run plan。
- 明确副作用。
- 明确回滚方案。
- 明确验证命令。
- 有副作用动作必须等待用户授权。
- plan 必须携带快照、前置条件、幂等键和锁。
- apply 前必须重新校验前置条件，漂移则中止。

`orag-self-ops` 支持 workflow：

- `apply_low_risk_action`：执行授权低风险动作。
- `create_remediation_issue`：创建修复 issue。
- `incident_review`：输出故障复盘模板、时间线、影响面、根因、恢复动作和预防动作。

## 自检流程

```text
用户或定时任务触发
  -> Agent 读取 Skill
  -> MCP tools/list 发现可用工具
  -> 根据 focused/broad 选择检查范围
  -> 调用 orag_check(scope, mode)
  -> 聚合命令输出、trace、错误码、测试结果和 artifacts
  -> 输出 PASS/FAIL/BLOCKED、稳定 check ID 和证据
  -> 将新缺口转化为 checklist、runbook、测试或 Skill 说明
```

## 自运维流程

```text
自检失败、用户反馈、CI 失败或告警触发
  -> orag_diagnose 归因问题
  -> orag_runbook_suggest 匹配处理步骤
  -> orag_maintenance_plan 生成 dry-run 计划
  -> 用户确认授权边界
  -> 执行低风险动作或创建 issue/PR
  -> 运行验证命令
  -> 更新 runbook、Skills、MCP 能力清单或测试
```

## 运行模式

- 本地开发模式：优先使用只读命令、Go 测试、MCP stdio smoke、agent-sync-check。
- CI 模式：自动运行契约检查、drift 检查、测试、vet、构建和产物一致性检查。
- 运维模式：连接部署环境，读取健康状态、日志、trace 和指标，默认只读。
- 事故模式：快速聚合 trace、错误、最近变更和告警，输出恢复优先的 runbook。
- 发布前模式：检查是否满足发布门禁，包括 API 文档、MCP/Skills 同步、迁移和 smoke test。
- 定时巡检模式：定期运行自检，生成趋势报告和风险项。

## 安全边界

- 默认所有 MCP 自检工具为只读。
- 所有会修改本地文件、远程服务、数据库、GitHub 或 git 状态的动作必须支持 `dry_run`。
- 自运维工具不得自动合并 PR，不得自动推送生产配置，不得绕过人工 review。
- 不在 MCP 或 Skills 输出中暴露 token、密码、完整 Authorization header 或敏感环境变量。
- Go 侧只做编排、状态管理、命令调用、日志和契约校验，不实现 LLM 推理逻辑。
- GitHub 操作统一委托 `gh`，不手写 GitHub REST/GraphQL client。
- Agent 产物统一从能力清单生成；OpenAPI 只是 HTTP facet，避免人工维护多个 schema 和行为说明。

## 建议产物结构

```text
.mcp/
  tools/
    check.json
    diagnose.json
    maintenance-plan.json
    apply-low-risk-action.json

.trae/
  skills/
    orag-self-check/
      SKILL.md
    orag-self-diagnose/
      SKILL.md
    orag-self-ops/
      SKILL.md

.codex/
  skills/
    orag-self-check/
      SKILL.md
    orag-self-diagnose/
      SKILL.md
    orag-self-ops/
      SKILL.md

.claude/
  skills/
    orag-self-check/
      SKILL.md
    orag-self-diagnose/
      SKILL.md
    orag-self-ops/
      SKILL.md

internal/
  capabilities/
    manifest.go
    manifest_test.go
  selfcheck/
    checker.go
    result.go
    checker_test.go
  selfops/
    planner.go
    runbook.go
    planner_test.go
  agentsync/
    generator.go

docs/
  operations/
    self-check.md
    self-diagnose.md
    self-ops-runbook.md

examples/
  mcp/
    self-check-stdio-smoke.jsonl
  skills/
    self-check.md
```

## 核心检查项

- API 可用性：`health`、`ready`、版本、构建信息。
- OpenAPI 合法性：schema、路径、鉴权、错误模型、示例。
- MCP 同步性：tool name、description、input schema、output schema、annotations。
- Skills 同步性：Codex、Claude Code、Trae 产物存在且字段完整。
- 关键路径：ingestion、query、eval、Ralph Loop fake downstream。
- 配置完整性：base URL、token、tenant、timeout、storage、vector DB。
- 数据依赖：PostgreSQL、Qdrant、迁移状态、集合状态。
- 可观测性：trace ID、日志、指标、错误码。
- 文档同步：README、agent integration、examples、operations runbook。
- 发布安全：测试、vet、构建、drift check、镜像构建和回滚说明。

## 最小落地路径

1. 新增能力清单 schema，明确 HTTP facet、MCP annotations、Skill 行为语义、风险等级、运维语义和生成元数据。
2. 生成 MCP tools 和 Codex/Claude Code/Trae Skills，先支持只读自检。
3. 接入 CI，把静态 `make agent-sync-check`、contract check、Go test、vet 作为发布门禁；运行时 `orag_check(scope=agent_sync)` 只作为便利探针。
4. 新增 `diagnose` 工具，聚合 trace、命令输出和错误码。
5. 新增 `self-ops` dry-run planner，计划必须携带快照 hash、前置条件、幂等键和单飞锁。
6. 在用户授权下支持少量低风险操作，apply 前必须重新校验前置条件，漂移则中止。
7. 沉淀运维手册和事故复盘 workflow，形成持续演进机制。

## 示例工作流

### Agent 集成健康检查

```text
用户：帮我检查当前 ORAG 的 Agent 集成是否健康。

Agent：
1. 读取 orag-self-check Skill。
2. 通过 MCP tools/list 发现 orag_check。
3. 调用 orag_check(scope=agent_sync, mode=focused)。
4. 调用 orag_check(scope=contract, mode=focused)。
5. 调用 orag_check(scope=smoke, mode=focused)，范围为 mcp_discovery。
6. 汇总 PASS/FAIL/BLOCKED、稳定 check ID、证据、风险和下一步建议。
7. 如结论用于发布门禁，提醒必须以 CI 静态 make agent-sync-check 为准。
```

### Query 变慢诊断

```text
用户：线上 query 变慢了，帮我自诊断。

Agent：
1. 调用 orag_diagnose，scope=query，输入 trace_id 或时间范围。
2. 查询 trace、日志、存储延迟、向量检索耗时、LLM 调用耗时。
3. 判断瓶颈属于 retrieval、rerank、LLM、DB、网络或配置。
4. 输出根因假设、证据、风险等级、验证命令和回滚建议。
5. 如需执行修复，只生成 dry-run plan，等待用户确认。
```

## 验收标准

- MCP tools 能暴露自检、自诊断、自运维计划相关能力，并具备完整输入输出 schema。
- Skills 能描述主流 Agent 的触发条件、调用顺序、安全边界、示例 prompt 和失败处理。
- 能力清单是 SSOT，OpenAPI 作为 HTTP facet，Skill 行为语义由能力清单维护并纳入 drift 校验。
- CI 静态 `make agent-sync-check` 能发现能力清单与 MCP/Skills/OpenAPI facet 产物漂移，运行时 MCP 检查不能替代发布门禁。
- 自检结果包含结构化 verdict、证据、风险和下一步建议。
- 自检结果包含 schema version、稳定 check ID、severity 到 verdict 映射和退出码约定。
- 自运维计划默认 dry-run，并明确副作用、回滚方式、验证命令、快照 hash、前置条件、幂等键和单飞锁。
- CI 至少覆盖 agent sync、OpenAPI contract、Go test、vet 和 MCP/Skill 生成测试。

## 推荐结论

MCP 是 ORAG 的 Agent 原生能力接口，Skills 是 Agent 原生操作认知。自检负责发现问题，自运维负责把问题处理闭环标准化。建议先落地只读自检和 drift 检查，再扩展诊断报告和 dry-run 运维计划，最后在明确授权下开放少量低风险自运维动作。
