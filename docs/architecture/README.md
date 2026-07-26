# 架构文档

本目录面向后端开发者、架构评审者和需要理解 ORAG 内部模块边界的维护者。

## 阅读顺序

| 顺序 | 文档 | 目标 |
| --- | --- | --- |
| 1 | [`rag-pipeline.md`](./rag-pipeline.md) | 理解一次 RAG 查询从 HTTP 请求到答案返回的执行链路。 |
| 2 | [`../Go-RAG-框架技术方案.md`](../Go-RAG-框架技术方案.md) | 查看更完整的技术方案和设计背景。 |
| 3 | [`../development.md`](../development.md) | 回到本地开发、测试和调试命令。 |

## 模块地图

| 模块 | 路径 | 责任 |
| --- | --- | --- |
| 公共 Go SDK | [`../../client.go`](../../client.go)、[`../../knowledge.go`](../../knowledge.go)、[`../../ingestion.go`](../../ingestion.go)、[`../../query.go`](../../query.go)、[`../../trace.go`](../../trace.go) | 对外提供嵌入式客户端和按能力划分的稳定 DTO；实现继续委托给 `internal/`。 |
| API 服务入口 | [`../../cmd/orag-api`](../../cmd/orag-api) | 启动 Hertz HTTP 服务。 |
| CLI 工具 | [`../../cmd/oragctl`](../../cmd/oragctl) | 执行数据库迁移等运维动作。 |
| HTTP 层 | [`../../internal/http`](../../internal/http) | 路由、鉴权中间件、错误响应、SSE。 |
| 应用组装 | [`../../internal/app`](../../internal/app) | 组装配置、依赖、服务和路由。 |
| RAG 服务 | [`../../internal/rag`](../../internal/rag) | 查询编排、上下文打包、引用、语义缓存。 |
| Graph 编排 | [`../../internal/graph`](../../internal/graph) | Eino Graph 节点和 RAG 链路。 |
| 知识库能力 | [`../../internal/kb`](../../internal/kb) | 检索器、RRF、store 抽象和能力组合。 |
| 入库 | [`../../internal/ingest`](../../internal/ingest) | loader、parser、chunker、jobs 和入库服务。 |
| 模型适配 | [`../../internal/llm/provider`](../../internal/llm/provider)、[`../../internal/llm/ark`](../../internal/llm/ark) | Provider registry 和 adapter 选择；Ark/豆包仍作为默认推荐实现。 |
| 存储 | [`../../internal/storage`](../../internal/storage) | PostgreSQL、Qdrant 真实后端实现。 |
| 评估 | [`../../internal/eval`](../../internal/eval) | 数据集、评估运行、metrics、optimizer。 |
| 观测 | [`../../internal/observability`](../../internal/observability) | metrics 和 tracing 入口。 |

## 贡献入口

修改功能时，从下表横向定位公开契约、传输层、领域实现、持久化和测试。不要从
`internal/app` 开始堆叠业务逻辑；该包只负责依赖组装和生命周期。

| 想修改的能力 | 公共 SDK | HTTP 层 | 领域实现 | 存储实现 | 首选测试入口 |
| --- | --- | --- | --- | --- | --- |
| 客户端生命周期、配置 | `client.go`、`config.go` | `internal/http/model_readiness.go` | `internal/app`、`internal/config` | — | `orag_test.go`、`internal/app/*_test.go` |
| 项目与 API Key | `control_plane.go` | `internal/http/projects.go`、`api_keys.go` | `internal/project`、`internal/auth` | `internal/storage/postgres/project.go`、`api_key.go` | `control_plane_test.go`、对应包内测试 |
| 知识库 | `knowledge.go` | `internal/http/router.go` | `internal/kb` | `internal/storage/postgres`、`qdrant` | `workflow_test.go`、`internal/kb/*_test.go` |
| 文档入库 | `ingestion.go` | `internal/http/router.go`、`chunk_preview.go` | `internal/ingest` | `internal/storage/postgres`、`qdrant` | `workflow_test.go`、`internal/ingest/*_test.go` |
| 查询与生成 | `query.go`、`stream.go` | `internal/http/router.go`、`sse.go` | `internal/rag`、`internal/graph` | `internal/storage/qdrant` | `workflow_test.go`、`internal/rag/*_test.go` |
| Trace | `trace.go` | `internal/http/router.go` | `internal/observability` | `internal/storage/postgres/trace.go` | `workflow_test.go`、Trace 相关包内测试 |
| 数据集与评估 | `evaluation.go` | `internal/http/evaluation_*.go` | `internal/dataset`、`internal/eval` | `internal/storage/postgres/eval.go` | `evaluation_test.go`、`internal/eval/*_test.go` |
| Pipeline 与发布 | `release.go` | `internal/http/pipelines.go`、`releases.go` | `internal/pipeline`、`internal/release` | `internal/storage/postgres/pipeline.go`、`release.go` | `release_sdk_test.go`、对应包内测试 |

公共 Go SDK 只有一个正式入口：`github.com/shikanon/orag`。无外部依赖的示例也使用
`orag.New(ctx, orag.MockConfig())`。`github.com/shikanon/orag/pkg/memory`
仅作为旧调用方的 Deprecated 兼容适配层存在，不再维护独立的分块、检索、回答或
Trace 实现。

## 运行时依赖

```text
orag-api
  |
  +-- PostgreSQL: metadata, FTS, dataset, evaluation, trace
  +-- Qdrant: vector collection, semantic cache collection
  +-- Model providers: 默认 VolcEngine/Doubao，可按能力选择 chat、embedding、rerank、multimodal provider
```

默认真实后端是 `STORAGE_BACKEND=qdrant_postgres`。`STORAGE_BACKEND=memory` 只用于单测、本地无依赖调试或排查 HTTP 层问题。

## 当前边界

- 系统默认不依赖 ES/Neo4j。
- `/readyz` 不主动调用外部模型接口，只根据 provider 配置状态报告 `model_provider=mock` 或 `model_provider=configured`。
- 当前 metrics 是进程内 Prometheus 文本指标，已包含 HTTP/RAG counter、受控低基数 label、cache hit/miss 和 RAG latency histogram；仓库提供可导入的 Grafana overview dashboard 与基础告警规则，设置 `OTEL_EXPORTER_OTLP_METRICS_ENDPOINT` 后会导出核心 HTTP/RAG/依赖/trace-store metrics。仍不提供指标持久化或分位数预聚合。
- metrics label 不包含 `trace_id`、tenant、用户输入、prompt、文档内容、模型响应或原始错误文本；单次请求排查应使用结构化日志和 `oragctl trace --trace-id <trace_id>`。
- 当前持久化应用 RAG trace 是查询权威；设置 `OTEL_EXPORTER_OTLP_ENDPOINT` 时会额外导出受限 OpenTelemetry span，并接受/返回 W3C `traceparent` 以连接跨服务 trace。`OTEL_TRACES_SAMPLER_ARG`（默认 `1`）控制新根 trace 的 parent-based ratio head sampling；`LANGFUSE_*` 仍只保留配置边界。
- 当前评估默认执行 deterministic rule-based metrics；请求携带 `judge`/`qag` 配置时会启用 LLM-as-Judge、QAG claim verification、pairwise 明细和 token/cost 记录。
