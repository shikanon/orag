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

## 关键阶段

| 阶段 | 主要路径 | 说明 |
| --- | --- | --- |
| 请求接入 | `../../internal/http` | 解析 JSON、校验 Bearer token、生成错误响应或 SSE 事件。 |
| 查询服务 | `../../internal/rag/service.go` | 组织一次 RAG 查询主流程。 |
| 语义缓存 | `../../internal/rag/semantic_cache.go`、`../../internal/storage/qdrant/semantic_cache.go` | 查询相似历史问题，命中时可减少生成成本。 |
| dense retrieval | `../../internal/kb/retrievers.go`、`../../internal/storage/qdrant` | 从 Qdrant 主 collection 检索向量候选。 |
| sparse retrieval | `../../internal/storage/postgres/fts.go` | 使用 PostgreSQL FTS 检索文本候选。 |
| 融合 | `../../internal/kb/rrf.go` | 使用 RRF 合并 dense 和 sparse 候选。 |
| 重排 | `../../internal/llm/ark` | 通过 Ark rerank 或 mock rerank 调整候选顺序。 |
| 上下文打包 | `../../internal/rag/context_pack.go` | 控制上下文数量和 token 预算。 |
| 引用 | `../../internal/rag/citation.go` | 将检索证据整理为 citations。 |
| 指标 | `../../internal/observability` | 更新查询次数、cache hit/miss 和延迟累计值。 |

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
| SSE 中途 error | 查看 error event 的 `trace_id`，再检查服务端日志。 |
