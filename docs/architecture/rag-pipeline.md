# RAG Pipeline

本文说明 ORAG 中一次查询的主要执行链路，帮助开发者定位问题应该从 HTTP、检索、重排、生成、缓存还是存储层切入。

## 查询路径

```text
POST /v1/query or /v1/query:stream
        |
        v
HTTP middleware: auth, tenant, trace, error model
        |
        v
rag.Service.Query
        |
        +-- semantic cache lookup
        +-- dense retrieval from Qdrant
        +-- sparse retrieval from PostgreSQL FTS
        +-- RRF fusion
        +-- optional rerank
        +-- context packing
        +-- chat generation
        +-- citation building
        +-- metrics and trace persistence
        |
        v
answer + citations + trace_id + cache_status + warnings
```

## Trace Context 与 Node Span

一次请求只有一个 `trace_id`。HTTP trace middleware 会先读取调用方传入的 `X-Trace-ID`；如果没有传入，则生成新的请求级 trace ID，并写回响应头 `X-Trace-ID`。后续认证、错误响应、JSON 查询、SSE 查询、RAG service、Graph 节点、结构化日志和 PostgreSQL trace 存储都复用这个 ID。

RAG trace 的层级关系：

```text
trace context (trace_id)
        |
        +-- rag_traces: one row per RAG query
        |       tenant_id, profile, latency_ms, created_at
        |
        +-- rag_node_spans: many rows per trace
                node_name, latency_ms, error, created_at
```

`rag_traces` 表示一次完整 RAG 查询；`rag_node_spans` 表示 Graph 中关键节点的执行片段。node span 按 `created_at, id` 排序后可还原节点执行顺序；任一 span 的 `error` 非空时，CLI 查询结果会标记 `has_error=true` 并累计 `error_count`。如果调用方复用同一个 `X-Trace-ID`，持久化记录按最后完成的请求整体覆盖主记录和 node spans，不会把多次请求的 spans 追加到同一个 trace。当前持久化的是应用内 RAG trace，不是 OpenTelemetry span，也不是 LangFuse trace。

## 关键阶段

| 阶段 | 主要路径 | 说明 |
| --- | --- | --- |
| 请求接入 | `../../internal/http` | 解析 JSON、校验 Bearer token、生成错误响应或 SSE 事件。 |
| 查询服务 | `../../internal/rag/service.go` | 组织一次 RAG 查询主流程。 |
| RAG Graph | `../../internal/graph` | 编排检索、融合、重排、打包、生成和引用节点，并记录 node span。 |
| 语义缓存 | `../../internal/rag/semantic_cache.go`、`../../internal/storage/qdrant/semantic_cache.go` | 查询相似历史问题，命中时可减少生成成本。 |
| dense retrieval | `../../internal/kb/retrievers.go`、`../../internal/storage/qdrant` | 从 Qdrant 主 collection 检索向量候选。 |
| sparse retrieval | `../../internal/storage/postgres/fts.go` | 使用 PostgreSQL FTS 检索文本候选。 |
| 融合 | `../../internal/kb/rrf.go` | 使用 RRF 合并 dense 和 sparse 候选。 |
| 重排 | `../../internal/llm/ark` | 通过 Ark rerank 或 mock rerank 调整候选顺序。 |
| 上下文打包 | `../../internal/rag/context_pack.go` | 控制上下文数量和 token 预算。 |
| 引用 | `../../internal/rag/citation.go` | 将检索证据整理为 citations。 |
| 指标 | `../../internal/observability` | 更新查询次数、错误计数、cache hit/miss 和延迟 histogram。 |
| Trace 存储 | `../../internal/storage/postgres/trace.go` | 将 `rag_traces` 和 `rag_node_spans` 写入 PostgreSQL，供 `oragctl trace` 查询。 |

## Profile 影响

当前公开 profile：

| Profile | 用途 |
| --- | --- |
| `realtime` | 默认 profile，偏主路径和低延迟。 |
| `high_precision` | 用于质量优先的查询和评估对比。 |

profile 与 `top_k` 会影响候选规模、召回质量和延迟。optimizer 当前只对 `profiles` 和 `top_ks` 做确定性网格搜索。

## Cache 状态

响应中的 `cache_status` 用于判断语义缓存是否命中。当前 metrics 中：

| 指标 | 含义 |
| --- | --- |
| `orag_rag_cache_hits_total` | 语义缓存命中的 RAG 查询总数。 |
| `orag_rag_cache_misses_total` | 非 `hit` 状态都会计为 miss。 |

## 排查建议

| 现象 | 优先检查 |
| --- | --- |
| 查询 401 | 登录 token、`JWT_SECRET`、token 有效期。 |
| 查询无引用 | 文档是否成功入库、Qdrant collection 是否有向量、PostgreSQL FTS 是否有 chunk。 |
| 查询延迟高 | dense/sparse top-k、rerank provider、Ark timeout、上下文 token 预算。 |
| cache 命中异常 | 语义缓存 collection、`RAG_SEMANTIC_CACHE_THRESHOLD`、embedding 维度配置。 |
| SSE 中途 error | 查看 `error` event 的 `trace_id`，再检查服务端日志和 `oragctl trace --trace-id <trace_id>`。 |
