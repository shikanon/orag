# Observability, Alerting, Self-Healing, and Tracing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 ORAG 现有的基础健康检查、metrics、trace 和 self-ops 骨架升级为可告警、可诊断、可验证、默认只读且可授权执行低风险动作的运维闭环。

**Architecture:** 保留现有 `/healthz`、`/readyz`、`/metrics`、RAG trace repository、MCP self-check/self-diagnose/self-ops 作为基础能力。新增告警规则、真实自检探针、诊断 evidence 聚合、trace 精细化和只读 controller，先形成“告警或巡检 -> 自检 -> 诊断 -> runbook -> dry-run plan”的闭环，再逐步开放低风险自愈动作。

**Tech Stack:** Go, Hertz HTTP server, PostgreSQL, Qdrant, Prometheus text format, Prometheus alert rules, Alertmanager examples, MCP tools, generated Skills, existing contract and integration tests.

---

## 当前现状

### 已具备能力

- HTTP 已注册 `/healthz`、`/readyz`、`/metrics`，入口在 `internal/http/router.go`。
- `/readyz` 可检查 memory backend、PostgreSQL ping、Qdrant collection 配置和 model provider 配置状态，核心逻辑在 `internal/app/app.go`。
- `/metrics` 已输出进程内 Prometheus text metrics，包含 HTTP 请求计数、HTTP 错误计数、RAG query 计数、RAG error 计数、semantic cache hit/miss、RAG latency histogram，核心实现位于 `internal/observability/metrics.go`。
- HTTP middleware 会记录 trace id、route、status、latency 和 error code 到结构化日志，逻辑位于 `internal/http/middleware.go`。
- RAG graph 已持久化 trace，并支持 Postgres 与 memory backend，核心路径位于 `internal/graph/rag_graph.go`、`internal/storage/postgres/trace.go`、`internal/app/memory_trace.go`。
- HTTP 已暴露 `/v1/traces` 和 `/v1/traces/:trace_id`，并按 tenant 做返回前隔离校验。
- `oragctl trace` 支持 trace list、lookup 和 node stats，便于本地诊断。
- MCP self-check 已支持 `health`、`contract`、`agent_sync`、`smoke`、`storage`、`config`、`release`、`all` scope，并返回结构化 verdict、check id、evidence 和 trace id。
- MCP self-ops 已具备 dry-run plan、显式 approval、snapshot drift check、idempotency key 和 single-flight lock 等安全骨架。

### 主要缺口

- 没有 Prometheus alert rules、Alertmanager 配置、通知路由、告警分级、告警静默/抑制策略或告警事件模型。
- 没有后台 controller 或 scheduler 消费告警、周期性执行 self-check，或驱动自检、诊断、runbook、dry-run plan 的闭环。
- `selfcheck` 的 `health`、`config`、`storage` 仍偏 placeholder，storage 明确说明尚未接入 live dependency probes。
- `diagnostics.TraceLookup` 当前返回 fixture evidence，尚未连接真实 trace repository、metrics、日志或存储状态。
- `selfops` 当前只支持 `agent_artifacts` -> `make agent-sync`，不支持 storage repair、migration check、Qdrant collection validation、remediation issue 创建等动作。
- capability manifest 暴露 `orag_create_remediation_issue`，但 MCP runtime 目前只路由 `orag_maintenance_plan` 和 `orag_apply_low_risk_action`。
- metrics 缺少 HTTP latency histogram、dependency latency/error、trace store failure、ingestion job、optimizer/eval 关键指标。
- `/readyz` 没有为 PostgreSQL/Qdrant 单独设置短 timeout，也没有检查 migration/schema 完整性或模型服务真实可用性。
- RAG node span 没有记录真实 `StartedAt`、`EndedAt` 和 `Sequence`，当前主要依赖存储层补齐时间和顺序。
- trace 查询有 node stats repository/CLI 能力，但 HTTP API 未暴露 stats endpoint。
- `rag_traces.query` 保存原始用户 query，当前没有脱敏、截断、retention 或敏感字段治理策略。

## 目标状态

- 告警可用：具备可直接接入 Prometheus/Alertmanager 的规则、示例配置和 runbook 映射。
- 自检可信：self-check 对 health/config/storage/release scope 使用真实探针，而不是只返回 executor 可响应。
- 诊断有证据：diagnostics 能聚合 trace、metrics、日志片段、failed command 和 dependency 状态。
- 自愈默认只读：自动流程只生成 diagnosis、runbook 和 dry-run plan；任何写动作必须用户或外部系统显式授权。
- tracing 可定位：trace 覆盖 RAG node、LLM provider、vector DB、Postgres、Qdrant 和 trace store 写入失败；span 时间和顺序准确。
- 隐私可控：trace query 和 error evidence 支持截断、脱敏和 retention 配置。
- 发布可验证：所有能力都有单元测试、契约测试、smoke 命令和文档入口。

## 非目标

- 不在第一阶段接入完整 OpenTelemetry collector 或商业 APM 平台。
- 不自动执行高风险修复，例如数据库写修复、自动迁移、自动重启生产服务、自动扩容。
- 不替代 CI gate；`make agent-sync-check`、contract tests、release tests 仍是发布权威门禁。
- 不在未授权情况下创建 issue、PR 或修改运行环境。

## 风险分级

### P1

- 告警系统缺失，生产问题只能依赖人工查看 `/metrics`、日志或 trace。
- 自愈闭环缺失，文档中的“告警触发”和“事故模式”尚不能自动运行。
- self-check 探针偏浅，容易给出“executor 正常”但依赖不可用的误导性 pass。
- diagnostics 未接真实 evidence，无法支撑可靠根因分析。

### P2

- tracing span 时间、顺序和粒度不足，慢点定位不够精细。
- metrics 覆盖不足，缺少 HTTP latency、dependency latency、trace store failure 等可告警指标。
- readiness 对 migration、schema、模型服务真实可用性覆盖不足。
- capability/runtime 能力存在差异，Agent 可能看到未实现的工具。

### P3

- trace 原始 query、error message、日志片段缺少脱敏和 retention 策略。
- self-ops plan/completed/lock 当前是进程内状态，重启后不适合生产审计和跨实例幂等。

## 文件结构规划

- Modify: `internal/observability/metrics.go`，增加 HTTP latency、dependency、trace store、self-check/self-ops 指标。
- Modify: `internal/observability/metrics_test.go`，覆盖新增指标、低基数 label 和敏感字段不进入 metrics。
- Modify: `internal/http/middleware.go`，将 HTTP latency 写入 metrics histogram。
- Modify: `internal/app/app.go`，为 readiness 增加 per-dependency timeout、migration/schema 可选检查和更细粒度 Qdrant check。
- Modify: `internal/selfcheck/selfcheck.go`，将 health/config/storage/release scope 接真实探针。
- Modify: `internal/diagnostics/diagnostics.go`，将 trace lookup、diagnose 接真实 repository 和 metrics/log evidence provider。
- Modify: `internal/selfops/selfops.go`，扩展低风险 action catalog，并明确 unsupported planned capability 的返回语义。
- Modify: `internal/mcp/server.go`，对 `orag_create_remediation_issue` 做 runtime 路由或显式不可用响应。
- Modify: `internal/graph/nodes.go`、`internal/graph/state.go`、`internal/graph/rag_graph.go`，补齐 span sequence、started_at、ended_at 和子调用 span bridge。
- Modify: `internal/storage/postgres/trace.go`，增加 tenant-aware lookup、query redaction/truncation 和 HTTP stats 查询支撑。
- Create: `deployments/prometheus/alerts.yml`，提供 ORAG 最小告警规则。
- Create: `deployments/alertmanager/alertmanager.example.yml`，提供本地示例路由和 webhook receiver。
- Create: `internal/operations/controller.go`，实现只读巡检 controller，不执行写动作。
- Create: `internal/operations/controller_test.go`，覆盖 controller 状态机和只读边界。
- Modify: `api/openapi.yaml`，补充 trace stats endpoint、diagnostics endpoint 状态说明和 error schema。
- Modify: `docs/operations/README.md`、`docs/operations/troubleshooting.md`，补充告警、诊断、自愈边界和 runbook。
- Modify: `deployments/docker-compose.yml`，为 `orag-api` 增加 healthcheck，并挂载 Prometheus/Alertmanager 示例时保持可选。

## Task 1: 补齐可告警 metrics

**Files:**
- Modify: `internal/observability/metrics.go`
- Modify: `internal/observability/metrics_test.go`
- Modify: `internal/http/middleware.go`

- [ ] **Step 1: 写 HTTP latency histogram 测试**

新增测试覆盖：
- `ObserveHTTPRequest` 记录 request counter。
- 新增 `ObserveHTTPLatency(method, route, status, latencyMS)` 或扩展现有 `ObserveHTTPRequest` 记录 latency。
- 输出包含 `orag_http_request_latency_ms_bucket`、`sum`、`count`。
- label 只允许 `method`、`route`、`status_class`，不允许 query、trace_id、tenant、user_id。

Run:

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/observability -run TestHTTPMetrics -v
```

Expected: FAIL，直到 metrics 支持 HTTP latency histogram。

- [ ] **Step 2: 实现 HTTP latency histogram**

在 `Metrics` 中增加 HTTP latency histogram map，bucket 建议使用：

```go
var httpLatencyBucketsMS = []int64{10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000}
```

输出指标：
- `orag_http_request_latency_ms_bucket`
- `orag_http_request_latency_ms_sum`
- `orag_http_request_latency_ms_count`

Expected: `go test ./internal/observability -run TestHTTPMetrics -v` PASS。

- [ ] **Step 3: 接入 middleware**

在 `metricsMiddleware` 中复用 `time.Since(start).Milliseconds()`，调用新增 latency observe 方法。

Run:

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/http -run TestHealthReadyMetrics -v
```

Expected: PASS，且现有 HTTP metrics 契约不回退。

- [ ] **Step 4: 增加 dependency 和 trace store 指标设计**

在 `Metrics` 中增加低基数指标：
- `orag_dependency_checks_total{dependency,status}`
- `orag_dependency_check_latency_ms_bucket{dependency,status}`
- `orag_trace_store_total{outcome}`
- `orag_trace_store_latency_ms_bucket{outcome}`

Allowed labels:
- `dependency`: `postgres`、`qdrant`、`model_provider`、`other`
- `status`: `ready`、`error`、`timeout`
- `outcome`: `success`、`error`

Run:

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/observability -v
```

Expected: PASS。

## Task 2: 增加最小告警规则

**Files:**
- Create: `deployments/prometheus/alerts.yml`
- Create: `deployments/alertmanager/alertmanager.example.yml`
- Modify: `docs/operations/README.md`
- Modify: `docs/operations/troubleshooting.md`

- [ ] **Step 1: 新增 Prometheus alert rules**

创建 `deployments/prometheus/alerts.yml`，包含以下规则：
- `ORAGAPIHigh5xxRate`: 5 分钟内 5xx 比例超过 5%。
- `ORAGRAGHighErrorRate`: 5 分钟内 RAG error 比例超过 5%。
- `ORAGRAGHighLatencyP95`: RAG p95 超过 5s 持续 10 分钟。
- `ORAGTraceStoreFailures`: trace store error 在 5 分钟内大于 0。
- `ORAGDependencyCheckFailing`: dependency readiness error 持续 3 分钟。
- `ORAGMetricsMissing`: `orag_up` absent 超过 2 分钟。

Run:

```bash
promtool check rules deployments/prometheus/alerts.yml
```

Expected: SUCCESS。如果本机没有 `promtool`，记录为未安装，并用 YAML parser 测试兜底。

- [ ] **Step 2: 新增 Alertmanager 示例**

创建 `deployments/alertmanager/alertmanager.example.yml`，包含：
- 默认 receiver: `orag-webhook`
- route group_by: `alertname`、`service`、`severity`
- inhibit rule: critical 抑制 warning
- webhook URL 示例: `http://orag-api:8080/v1/ops/alerts`

Expected: 该文件是示例，不要求当前 runtime 已实现 `/v1/ops/alerts`。

- [ ] **Step 3: 更新运维文档**

在 `docs/operations/README.md` 增加“告警接入”章节，说明：
- 当前提供规则和示例配置。
- 告警自动写操作未启用。
- 告警处理路径是 self-check、diagnose、runbook、dry-run plan。
- 每条 alert 对应一个 runbook 条目。

在 `docs/operations/troubleshooting.md` 增加每个 alert 的排查步骤。

Run:

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./tests/contract -run TestOpenAPI -v
```

Expected: PASS，文档变更不影响 API 契约。

## Task 3: 将 self-check placeholder 接真实探针

**Files:**
- Modify: `internal/selfcheck/selfcheck.go`
- Modify: `internal/selfcheck/selfcheck_test.go`
- Modify: `internal/app/app.go`
- Modify: `internal/app/readiness_test.go`

- [ ] **Step 1: 定义 self-check probe 接口**

在 `internal/selfcheck/selfcheck.go` 增加只读 probe 抽象：

```go
type Probe interface {
	Health(ctx context.Context) CheckResult
	Config(ctx context.Context) CheckResult
	Storage(ctx context.Context) CheckResult
}
```

`Executor` 默认使用 builtin probe；测试可注入 fake probe。

Run:

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/selfcheck -run TestStorageProbe -v
```

Expected: FAIL，直到 storage scope 不再返回 placeholder。

- [ ] **Step 2: 实现 storage probe**

Storage probe 读取 readiness 结果或直接调用 dependency check：
- PostgreSQL: ping 成功、耗时、错误。
- Qdrant: 主 collection 和 semantic cache collection 分开返回。
- Memory backend: 返回 `storage=ready`，evidence 说明是 memory backend。

Expected evidence:
- `type=dependency`
- `message=postgres ready` 或 `qdrant collection invalid`
- `output` 使用截断后的结构化摘要。

- [ ] **Step 3: 实现 config probe**

Config probe 检查：
- 必需 URL、host、port、embedding dimensions、model provider 名称。
- timeout 必须为正数。
- `AllowDeterministicMock=true` 时返回 warning evidence，而不是 critical fail。

Run:

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/selfcheck -v
```

Expected: PASS。

- [ ] **Step 4: 拆分 readiness Qdrant checks**

将 `checks["qdrant"]` 拆为：
- `qdrant.main_collection`
- `qdrant.semantic_cache_collection`

保留兼容字段策略：
- 如果 API 契约要求旧字段，增加 `qdrant` aggregate。
- 新字段用于诊断和告警。

Run:

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/app -run TestReadiness -v
```

Expected: PASS。

## Task 4: 接入真实 diagnostics evidence

**Files:**
- Modify: `internal/diagnostics/diagnostics.go`
- Modify: `internal/diagnostics/diagnostics_test.go`
- Modify: `internal/mcp/server.go`
- Modify: `internal/app/app.go`

- [ ] **Step 1: 定义 diagnostics evidence provider**

增加只读接口：

```go
type EvidenceProvider interface {
	GetTrace(ctx context.Context, traceID string) (TraceEvidence, bool, error)
	MetricsSnapshot(ctx context.Context, scope string) ([]selfcheck.Evidence, error)
	RecentLogs(ctx context.Context, traceID string, limit int) ([]selfcheck.Evidence, error)
}
```

测试要求：
- trace_id 为空返回 blocked。
- trace 不存在返回 `Found=false`。
- trace 存在时 findings 包含 node span、error_count、slowest_node。

- [ ] **Step 2: 替换 fixture trace lookup**

将 `TraceLookup` 从固定 fixture 改为调用 `EvidenceProvider.GetTrace`。

Run:

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/diagnostics -run TestTraceLookup -v
```

Expected: PASS，且返回内容不再包含 `trace.fixture`。

- [ ] **Step 3: Diagnose 聚合 metrics 和 failed command evidence**

`Diagnose` 规则：
- 如果 failed command 失败，verdict 为 fail，保留原有逻辑。
- 如果 trace evidence 有 node error，verdict 为 fail。
- 如果 metrics snapshot 显示 error rate 或 latency 异常，verdict 为 fail 或 warning。
- 否则返回 pass，并建议补充 trace/log/failed command。

Run:

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/diagnostics -v
```

Expected: PASS。

## Task 5: 建立只读告警/巡检 controller

**Files:**
- Create: `internal/operations/controller.go`
- Create: `internal/operations/controller_test.go`
- Modify: `internal/capabilities/builtin.go`
- Modify: `agent/mcp/tools/orag-self-check.json`
- Modify: `agent/skills/trae/orag-self-check/SKILL.md`

- [ ] **Step 1: 定义 controller 状态机**

Controller 输入：
- alert name
- severity
- labels
- trace id
- optional symptom

Controller 输出：
- self-check request
- diagnostics request
- runbook suggestion
- optional dry-run maintenance plan request

状态机：
- `received`
- `checked`
- `diagnosed`
- `runbook_selected`
- `plan_generated`
- `blocked`

Controller 必须只读，不调用 `Apply`。

- [ ] **Step 2: 编写只读边界测试**

测试：
- critical alert 会调用 self-check 和 diagnose。
- storage alert 会选择 storage scope。
- controller 不会调用 `orag_apply_low_risk_action`。
- controller 在 evidence 不足时返回 blocked，并给出需要补充的 evidence。

Run:

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/operations -v
```

Expected: PASS。

- [ ] **Step 3: 将 controller 暴露为 planned capability**

先在 capability manifest 中标记为 planned 或 experimental，不默认生成到生产工具列表；避免 Agent 误认为已可调用生产告警 webhook。

Run:

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local make agent-sync-check
```

Expected: PASS。

## Task 6: 扩展低风险 self-ops action catalog

**Files:**
- Modify: `internal/selfops/selfops.go`
- Modify: `internal/selfops/selfops_test.go`
- Modify: `internal/mcp/server.go`
- Modify: `internal/capabilities/builtin.go`

- [ ] **Step 1: 增加 dry-run only actions**

新增 action scopes：
- `agent_artifacts`: 已有，继续支持 `make agent-sync`。
- `migration_status`: 只读，运行 migration status check，不执行 migration。
- `qdrant_collection_validation`: 只读，验证 collection，不创建或删除 collection。
- `remediation_issue`: 需要 approved，创建 issue；如果 runtime 未实现，返回 blocked。

- [ ] **Step 2: 测试授权和幂等**

测试：
- `approved=false` 必须 blocked。
- 同一 idempotency key 重放返回 skipped。
- drift 后 blocked。
- unsupported scope blocked，错误信息包含 scope。

Run:

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/selfops -v
```

Expected: PASS。

- [ ] **Step 3: 修复 manifest/runtime 差异**

对 `orag_create_remediation_issue` 选择一种策略：
- 实现 runtime route，并在未配置 issue backend 时返回 `blocked`。
- 或从生成工具中移除该 tool，只保留 planned 文档说明。

Run:

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/mcp ./internal/capabilities ./internal/agentsync ./internal/agentskills -v
```

Expected: PASS。

## Task 7: 提升 tracing 准确性和覆盖面

**Files:**
- Modify: `internal/graph/state.go`
- Modify: `internal/graph/nodes.go`
- Modify: `internal/graph/rag_graph.go`
- Modify: `internal/storage/postgres/trace.go`
- Modify: `internal/app/memory_trace.go`
- Modify: `internal/observability/tracing.go`
- Modify: `internal/graph/rag_graph_test.go`
- Modify: `internal/storage/postgres/repository_test.go`

- [ ] **Step 1: 补齐 NodeSpan 时间和顺序**

`withSpan` 生成 span 时写入：
- `Sequence`: collector 分配或 state 递增。
- `StartedAt`: node 开始时间。
- `EndedAt`: node 结束时间。
- `LatencyMS`: `EndedAt - StartedAt`。

Run:

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/graph -run TestGraphTraceSpans -v
```

Expected: PASS。

- [ ] **Step 2: 接入 `observability.StartSpan`**

在 graph node wrapper 中调用 `observability.StartSpan(ctx, name)`，确保外部 tracer 可以收到 node span。

要求：
- PostgreSQL trace store 行为不变。
- 外部 tracer 失败或 nil 时不影响 query。
- span.End(err) 总会执行。

Run:

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/observability ./internal/graph -v
```

Expected: PASS。

- [ ] **Step 3: 增加 dependency 子 span**

优先覆盖：
- embedding provider 调用。
- vector retrieval。
- rerank。
- LLM generate。
- trace store write。

每个子 span label 控制：
- `component`
- `operation`
- `outcome`

不允许 query、prompt、document content 进入 label。

- [ ] **Step 4: 增加 tenant-aware trace lookup**

Repository 增加：

```go
GetTraceForTenant(ctx context.Context, tenantID string, traceID string) (TraceRecord, bool, error)
```

HTTP handler 改为调用 tenant-aware lookup，避免先读取跨租户记录再做应用层过滤。

Run:

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/storage/postgres ./internal/http -run TestTrace -v
```

Expected: PASS。

## Task 8: 增加 trace stats HTTP endpoint

**Files:**
- Modify: `internal/http/router.go`
- Modify: `internal/http/router_test.go`
- Modify: `api/openapi.yaml`
- Modify: `docs/api/ingestion-and-query.md`
- Modify: `examples/curl/36_trace_lookup.sh`

- [ ] **Step 1: 新增 endpoint**

新增：

```text
GET /v1/traces:stats
```

Query 参数复用 `/v1/traces`：
- `profile`
- `from`
- `to`
- `has_error`
- `slow_ms`
- `limit`

Response:
- `items`: node stats list
- `tenant_id`: current tenant

- [ ] **Step 2: 编写 HTTP 测试**

测试：
- 未授权返回 401。
- 授权后只返回当前 tenant stats。
- filter 传递到 repository。

Run:

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/http -run TestTraceStats -v
```

Expected: PASS。

- [ ] **Step 3: 更新 OpenAPI 契约**

在 `api/openapi.yaml` 增加 `/v1/traces:stats` 和 schema。

Run:

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./tests/contract -run TestOpenAPI -v
```

Expected: PASS。

## Task 9: 增加 trace 隐私与 retention 策略

**Files:**
- Modify: `internal/config/config.go`
- Modify: `configs/config.example.yaml`
- Modify: `internal/storage/postgres/trace.go`
- Modify: `internal/app/memory_trace.go`
- Modify: `docs/operations/README.md`

- [ ] **Step 1: 增加 trace privacy config**

新增配置：

```yaml
tracing:
  store_query: true
  query_max_bytes: 2048
  redact_patterns:
    - "(?i)authorization:\\s*bearer\\s+[^\\s]+"
    - "(?i)api[_-]?key\\s*[:=]\\s*[^\\s]+"
  retention_days: 30
```

默认：
- `store_query=true` 保持兼容。
- `query_max_bytes=2048`。
- `retention_days=30`。

- [ ] **Step 2: 实现 query 截断和脱敏**

存储 trace 前执行：
- 按 byte 截断。
- 应用 redact patterns。
- 如果 `store_query=false`，存储空字符串或固定值 `[redacted]`。

Run:

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/storage/postgres ./internal/app -run TestTracePrivacy -v
```

Expected: PASS。

- [ ] **Step 3: 文档化 retention 边界**

在运维文档说明：
- 当前仅提供配置和存储前处理。
- 历史数据清理需要单独 migration/job。
- 多租户环境应在部署侧配置数据库级备份与保留策略。

## Task 10: 发布验证与文档收敛

**Files:**
- Modify: `docs/operations/README.md`
- Modify: `docs/operations/troubleshooting.md`
- Modify: `docs/api/agent-integrations.md`
- Modify: `examples/skills/self-check-diagnose-ops.md`
- Modify: `README.md`
- Modify: `README_EN.md`

- [ ] **Step 1: 更新总览文档**

说明四层能力：
- Monitoring: `/healthz`、`/readyz`、`/metrics`
- Alerting: Prometheus rules and Alertmanager examples
- Diagnosis: trace lookup、diagnose、runbook suggest
- Self-ops: dry-run plan、approved low-risk apply

- [ ] **Step 2: 更新 troubleshooting runbook**

每个 runbook 条目包含：
- 触发 alert。
- 影响。
- 检查命令。
- 常见原因。
- 只读诊断步骤。
- 需要授权的恢复动作。
- 验证命令。

- [ ] **Step 3: 运行完整验证**

Run:

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/observability ./internal/app ./internal/selfcheck ./internal/diagnostics ./internal/selfops ./internal/mcp -v
```

Expected: PASS。

Run:

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./tests/contract -v
```

Expected: PASS。

Run:

```bash
CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local make agent-sync-check
```

Expected: PASS。

## 里程碑

### Milestone 1: 可告警

完成 Task 1 和 Task 2。

验收：
- `/metrics` 有 HTTP latency 和 trace store failure 指标。
- `deployments/prometheus/alerts.yml` 可被 `promtool` 校验。
- 运维文档能把 alert 映射到 runbook。

### Milestone 2: 自检可信

完成 Task 3 和 Task 4。

验收：
- `orag_check(scope=storage)` 返回真实 dependency evidence。
- `orag_trace_lookup` 不再返回 fixture。
- `orag_diagnose` 能基于 trace 和 metrics 形成 fail/warning/pass。

### Milestone 3: 只读闭环

完成 Task 5 和 Task 6。

验收：
- alert 或巡检输入能驱动 self-check、diagnose、runbook 和 dry-run plan。
- controller 不执行写动作。
- self-ops unsupported capability 不再误导 Agent。

### Milestone 4: Trace 可定位且可治理

完成 Task 7、Task 8 和 Task 9。

验收：
- node span 有真实时间和顺序。
- HTTP 暴露 trace stats。
- trace query 支持截断、脱敏和 retention 配置。

### Milestone 5: 文档与发布门禁收敛

完成 Task 10。

验收：
- README、operations、troubleshooting、agent integrations 都描述同一套边界。
- contract tests、targeted unit tests、agent-sync-check 全部通过。

## 验证矩阵

| 范围 | 命令 | 期望 |
| --- | --- | --- |
| Metrics | `CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/observability -v` | PASS |
| Readiness | `CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/app -run TestReadiness -v` | PASS |
| Self-check | `CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/selfcheck -v` | PASS |
| Diagnostics | `CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/diagnostics -v` | PASS |
| Self-ops | `CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/selfops -v` | PASS |
| MCP | `CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/mcp -v` | PASS |
| Contract | `CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./tests/contract -v` | PASS |
| Agent artifacts | `CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local make agent-sync-check` | PASS |
| Alert rules | `promtool check rules deployments/prometheus/alerts.yml` | SUCCESS |

## 实施原则

- 先只读，后写入；先 evidence，后 automation。
- 告警和 controller 不直接执行修复动作。
- 所有写动作必须显式 `approved=true`，并保留 drift check、idempotency key 和 lock。
- Metrics label 必须低基数，不允许 trace id、query、prompt、user id、document id。
- Trace evidence 必须支持截断和脱敏。
- runtime 能力必须和 capability manifest、MCP tools、Skills 保持一致。
- 每个阶段完成后运行对应最小测试，再进入下一阶段。
