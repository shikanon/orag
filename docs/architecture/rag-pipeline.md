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
        +-- optional query route
        +-- semantic cache lookup
        +-- dense retrieval from Qdrant
        +-- sparse retrieval from PostgreSQL FTS
        +-- RRF fusion
        +-- optional graph expansion
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

`rag_traces` 表示一次完整 RAG 查询；`rag_node_spans` 表示 Graph 中关键节点的执行片段。node span 按 `created_at, id` 排序后可还原节点执行顺序；任一 span 的 `error` 非空时，HTTP/CLI 查询结果会标记 `has_error=true` 并累计 `error_count`。如果调用方复用同一个 `X-Trace-ID`，持久化记录按最后完成的请求整体覆盖主记录和 node spans，不会把多次请求的 spans 追加到同一个 trace。当前持久化的是应用内 RAG trace，不是 OpenTelemetry span，也不是 LangFuse trace。

## 入库可见性与检索授权

PostgreSQL 的 `chunks.searchable` 是 sparse 与 dense 检索共同的唯一可见性权威。Qdrant 只负责召回 dense 候选；其 `searchable` payload 用于记录 staged/prepare 状态和诊断，不能独立授权某个 chunk 进入 RAG 上下文。

一次 PostgreSQL + Qdrant 入库按以下阶段执行：

```text
Store(false) -> Prepare(Qdrant) -> Commit(PostgreSQL) -> Finalize(Qdrant cleanup)
      |                |                    |                       |
      +----------------+--------------------+-- pre-commit error: abort candidate
                                               post-commit error: succeeded + warning
```

- `Store` 在两个存储中写入不可见候选，Qdrant 同时记录 `ingestion_job_id`。
- `PrepareActivation` 让 Qdrant 候选可供检索，但 PostgreSQL 屏障仍会拒绝未提交 chunk。
- `CommitActivation` 在 PostgreSQL advisory transaction lock 下原子替换同源旧版本，并将新 chunk 标记为 `searchable=true`。
- `FinalizeActivation` 删除 Qdrant 中同源旧文档点；失败时新版本已经提交，job 保持 `succeeded` 并记录清理 warning。
- dense retrieval 对每页 Qdrant 候选批量查询 PostgreSQL，只返回当前 `searchable=true` 的 chunk。可见性查询失败时 dense 查询 fail closed，不返回未经授权的候选。

历史 Qdrant 点即使缺少 `searchable` payload，也必须通过 PostgreSQL 授权。因此升级不要求同步回填 Qdrant payload，孤儿点和失败候选仍不会进入检索结果。

## 关键阶段

| 阶段 | 主要路径 | 说明 |
| --- | --- | --- |
| 请求接入 | [`../../internal/http`](../../internal/http) | 解析 JSON、校验 Bearer token、生成错误响应或 SSE 事件。 |
| 查询服务 | [`../../internal/rag/service.go`](../../internal/rag/service.go) | 组织一次 RAG 查询主流程。 |
| RAG Graph | [`../../internal/graph`](../../internal/graph) | 编排检索、融合、重排、打包、生成和引用节点，并记录 node span。 |
| 查询路由 | [`../../internal/rag/query_router.go`](../../internal/rag/query_router.go) | 可选启发式路由 direct、single retrieval 和 multi-step retrieval；direct 查询绕过检索，multi-step 查询提升到高精检索路径。 |
| 语义缓存 | [`../../internal/rag/semantic_cache.go`](../../internal/rag/semantic_cache.go)、[`../../internal/storage/qdrant/semantic_cache.go`](../../internal/storage/qdrant/semantic_cache.go) | 查询相似历史问题，命中时可减少生成成本。 |
| dense retrieval | [`../../internal/kb/retrievers.go`](../../internal/kb/retrievers.go)、[`../../internal/storage/qdrant`](../../internal/storage/qdrant) | 从 Qdrant 主 collection 检索候选，并由 PostgreSQL `chunks.searchable` 批量授权后返回。 |
| sparse retrieval | [`../../internal/storage/postgres/fts.go`](../../internal/storage/postgres/fts.go) | 使用 PostgreSQL FTS 检索文本候选；启用 Contextual Retrieval 时查询 `contextual_text + content` 生成的 search vector。 |
| RAPTOR 摘要层 | [`../../internal/ingest/raptor.go`](../../internal/ingest/raptor.go) | 可选生成递归摘要 chunk；摘要随普通 chunk 一起进入 embedding 和 FTS 检索。 |
| 图扩展 | [`../../internal/kb/graph.go`](../../internal/kb/graph.go)、[`../../internal/ingest/graph.go`](../../internal/ingest/graph.go) | 可选抽取 chunk 内实体共现关系，检索时按查询实体扩展相关 chunk。 |
| 融合 | [`../../internal/kb/rrf.go`](../../internal/kb/rrf.go) | 使用 RRF 合并 dense 和 sparse 候选。 |
| 重排 | [`../../internal/llm/ark`](../../internal/llm/ark) | 通过 Ark rerank 或 mock rerank 调整候选顺序。 |
| 上下文打包 | [`../../internal/rag/context_pack.go`](../../internal/rag/context_pack.go) | 控制上下文数量和 token 预算。 |
| 引用 | [`../../internal/rag/citation.go`](../../internal/rag/citation.go) | 将检索证据整理为 citations。 |
| 指标 | [`../../internal/observability`](../../internal/observability) | 更新查询次数、错误计数、cache hit/miss 和延迟 histogram。 |
| Trace 存储 | [`../../internal/storage/postgres/trace.go`](../../internal/storage/postgres/trace.go) | 将 `rag_traces` 和 `rag_node_spans` 写入 PostgreSQL，供 `GET /v1/traces`、`GET /v1/traces/{trace_id}` 和 `oragctl trace` 查询。 |

## Profile 影响

当前公开 profile：

| Profile | 用途 |
| --- | --- |
| `realtime` | 默认 profile，偏主路径和低延迟。 |
| `high_precision` | 用于质量优先的查询和评估对比。 |

`high_precision` 会启用查询改写、多查询扩展（`RAG_MULTI_QUERY_COUNT`，默认 3）和 HyDE，并对多路检索结果做 RRF 融合后再重排。profile 与 `top_k` 会影响候选规模、召回质量和延迟。optimizer 当前只对 `profiles` 和 `top_ks` 做确定性网格搜索。

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
| SSE 中途 error | 查看 `error` event 的 `trace_id`，再检查服务端日志、`GET /v1/traces/{trace_id}` 或 `oragctl trace --trace-id <trace_id>`。 |
