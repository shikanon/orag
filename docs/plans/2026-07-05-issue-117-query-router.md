# Issue 117 查询复杂度路由技术方案

## 背景与目标

Issue: https://github.com/shikanon/orag/issues/117

当前所有 query 都进入同一 RAG Graph，只通过 profile 控制 rewrite、multi-query、HyDE 等能力。目标是在 query 入口增加可选复杂度路由，在不启用时保持兼容，启用时按 query 复杂度选择 direct、single retrieval 或 multi-step retrieval。

## 当前代码落点

- [`internal/http/router.go`](../../internal/http/router.go): HTTP query request 转 `rag.QueryRequest`。
- [`internal/rag/types.go`](../../internal/rag/types.go): query request/response 需要带 routing metadata。
- [`internal/rag/service.go`](../../internal/rag/service.go): Query/Execute 入口。
- [`internal/graph/nodes.go`](../../internal/graph/nodes.go): Graph 节点适合插入 route classification。
- [`internal/observability/tracing.go`](../../internal/observability/tracing.go) 和 trace storage: 需要把路由结果进入 spans/warnings。

## 设计决策

1. 新增 `rag.Route` 枚举：
   - `direct`
   - `single_retrieval`
   - `multi_step_retrieval`
2. 默认实现规则分类器，不强依赖额外 LLM：
   - 短问题、问候、无需知识库词汇 -> `direct`
   - 普通事实问答 -> `single_retrieval`
   - 包含比较、总结、多个实体、原因链、多步骤信号 -> `multi_step_retrieval`
3. 预留 LLM classifier 接口：`QueryRouter.Route(ctx, QueryRequest) (RouteDecision, error)`。
4. 不启用 router 时行为完全兼容。
5. 启用 router 后：
   - `direct`: 跳过 retrieval，直接 Chat，但系统提示明确“不使用知识库上下文”。
   - `single_retrieval`: 禁用 rewrite/multi-query/HyDE，仅单次 hybrid retrieve。
   - `multi_step_retrieval`: 使用 high_precision 等价增强，允许 rewrite/multi-query/HyDE，后续可叠加 RAPTOR/Graph。
6. `QueryResponse` 增加 `RouteDecision` 字段；Graph span 增加 `query_route` 节点；日志和 warnings 带 route reason。

## 配置

新增：

- `QUERY_ROUTER_ENABLED=false`
- `QUERY_ROUTER_STRATEGY=heuristic`
- `QUERY_ROUTER_DIRECT_MAX_RUNES=32`
- `QUERY_ROUTER_COMPLEX_MIN_SIGNALS=2`

默认关闭，满足“可选增强”。

## 开发拆解

1. 路由类型与启发式分类器
   - 文件：[`internal/rag/query_router.go`](../../internal/rag/query_router.go)
   - 测试：[`internal/rag/query_router_test.go`](../../internal/rag/query_router_test.go)
   - 覆盖 direct/single/multi_step 和 reason。

2. QueryRequest/Response 扩展
   - 文件：[`internal/rag/types.go`](../../internal/rag/types.go)
   - 字段：`Route *RouteDecision`
   - HTTP JSON 保持向后兼容。

3. Service 接入
   - 文件：[`internal/rag/service.go`](../../internal/rag/service.go)
   - 在 Execute 开头 route，按 route 调整 retrieval behavior。
   - 测试：router disabled 时结果等同旧行为；direct 不调用 retriever；single 不触发 multi-query。

4. Graph 接入
   - 文件：[`internal/graph/nodes.go`](../../internal/graph/nodes.go)
   - 新增 `QueryRoute` 节点，在 `Init` 后运行。
   - State 增加 `RouteDecision`。
   - 测试 graph spans 包含 `query_route`。

5. 配置和 app wiring
   - 文件：[`internal/config/config.go`](../../internal/config/config.go)
   - 文件：[`internal/app/app.go`](../../internal/app/app.go)
   - 默认关闭，启用时注入 heuristic router。

6. 文档
   - 文件：[`README.md`](../../README.md)
   - 文件：[`docs/architecture/rag-pipeline.md`](../architecture/rag-pipeline.md)
   - 文件：[`docs/operations.md`](../operations.md)
   - 说明路由类型、默认策略、风险和排查。

## 验收证据

- router disabled 时现有 query 行为和测试不变。
- router enabled 时 direct/single/multi_step 路径可测试。
- route decision 出现在 response、warnings 或 trace span 中。
- `go test ./internal/rag ./internal/graph ./internal/http ./internal/app`

