# 架构文档

本目录面向后端开发者、架构评审者和需要理解 ORAG 内部模块边界的维护者。

## 阅读顺序

| 顺序 | 文档 | 目标 |
| --- | --- | --- |
| 1 | `rag-pipeline.md` | 理解一次 RAG 查询从 HTTP 请求到答案返回的执行链路。 |
| 2 | `../Go-RAG-框架技术方案.md` | 查看更完整的技术方案和设计背景。 |
| 3 | `../development.md` | 回到本地开发、测试和调试命令。 |

## 模块地图

| 模块 | 路径 | 责任 |
| --- | --- | --- |
| API 服务入口 | `../../cmd/orag-api` | 启动 Hertz HTTP 服务。 |
| CLI 工具 | `../../cmd/oragctl` | 执行数据库迁移等运维动作。 |
| HTTP 层 | `../../internal/http` | 路由、鉴权中间件、错误响应、SSE。 |
| 应用组装 | `../../internal/app` | 组装配置、依赖、服务和路由。 |
| RAG 服务 | `../../internal/rag` | 查询编排、上下文打包、引用、语义缓存。 |
| Graph 编排 | `../../internal/graph` | Eino Graph 节点和 RAG 链路。 |
| 知识库能力 | `../../internal/kb` | 检索器、RRF、store 抽象和能力组合。 |
| 入库 | `../../internal/ingest` | loader、parser、chunker、jobs 和入库服务。 |
| 模型适配 | `../../internal/llm/ark` | Ark/豆包 chat、embedding、rerank、多模态适配。 |
| 存储 | `../../internal/storage` | PostgreSQL、Qdrant 真实后端实现。 |
| 评估 | `../../internal/eval` | 数据集、评估运行、metrics、optimizer。 |
| 观测 | `../../internal/observability` | metrics 和 tracing 入口。 |

## 运行时依赖

```text
orag-api
  |
  +-- PostgreSQL: metadata, FTS, dataset, evaluation, trace
  +-- Qdrant: vector collection, semantic cache collection
  +-- Ark/Doubao: chat, embedding, rerank, multimodal parser
```

默认真实后端是 `STORAGE_BACKEND=qdrant_postgres`。`STORAGE_BACKEND=memory` 只用于单测、本地无依赖调试或排查 HTTP 层问题。

## 当前边界

- 系统默认不依赖 ES/Neo4j。
- `/readyz` 不主动调用 Ark 外部接口，只根据 key 配置状态报告 `mock` 或 `configured`。
- 当前 metrics 是进程内 Prometheus 文本指标，已包含 HTTP/RAG counter、受控低基数 label、cache hit/miss 和 RAG latency histogram；仍不提供指标持久化、分位数预聚合或外部 exporter。
- metrics label 不包含 `trace_id`、tenant、用户输入、prompt、文档内容、模型响应或原始错误文本；单次请求排查应使用结构化日志和 `oragctl trace --trace-id <trace_id>`。
- 当前持久化的是应用内 RAG trace，不是 OpenTelemetry span 或 LangFuse trace；`OTEL_EXPORTER_OTLP_ENDPOINT` 与 `LANGFUSE_*` 仅保留配置边界。
- 当前评估是 deterministic rule-based metrics，完整 LLM-as-Judge 仍是后续增强方向。
