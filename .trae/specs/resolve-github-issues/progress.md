# Progress Evidence

## 2026-07-04 - Task 1: 同步远程并建立 issue 审计清单

### 远程同步证据
- 执行 `git fetch --prune origin`，命令退出码 `0`。
- 当前分支：`task14-llm-judge-optimizer-validation`，本地 HEAD `e2a251d`。
- `origin/main`：`fe011ea`。
- `HEAD...origin/main` 差异：本地 ahead `2`，behind `2`。
- `HEAD...origin/task14-llm-judge-optimizer-validation` 差异：ahead `0`，behind `0`。
- `git status --short --branch` 输出：`## task14-llm-judge-optimizer-validation...origin/task14-llm-judge-optimizer-validation`，未显示未提交业务代码改动。
- 相关 `origin/coding-loop/*` 分支已确认存在：`origin/coding-loop/issue-104`、`origin/coding-loop/issue-105`、`origin/coding-loop/issue-100`、`origin/coding-loop/issue-101`、`origin/coding-loop/issue-97`、`origin/coding-loop/issue-90`、`origin/coding-loop/issue-91`、`origin/coding-loop/issue-85`、`origin/coding-loop/issue-87`、`origin/coding-loop/issue-82`、`origin/coding-loop/issue-78`、`origin/coding-loop/issue-76`、`origin/coding-loop/issue-71`、`origin/coding-loop/issue-70`、`origin/coding-loop/issue-68`、`origin/coding-loop/issue-64`、`origin/coding-loop/issue-62`。

### Issue 导出证据
- 执行 `gh issue list --repo shikanon/orag --state open --limit 100 --json number,title,body,labels,url`，命令退出码 `0`。
- 当前开放 issue 数量：`25`。
- 精简导出命令：`gh issue list --repo shikanon/orag --state open --limit 100 --json number,title,labels,url --jq '.[] | [.number, .title, (.labels | map(.name) | join(",")), .url] | @tsv'`。

### 根因分组与关闭决策

| 根因组 | Issue | 状态分类 | Close 决策 | 验证证据/后续任务 |
| --- | --- | --- | --- | --- |
| KB 删除 | #2, #14, #38, #42, #57, #70, #90, #101, #105 | 需要开发；重复根因 | 暂不关闭 | 由 Task 3 实现真实 tenant-scoped 删除，并在 Task 9/10 验证后批量关闭。 |
| KB 写入错误 | #3, #78 | 需要开发；重复根因 | 暂不关闭 | 由 Task 2 实现 repository 错误传播，验证写入/读取失败不会返回假成功。 |
| 数据集/评估租户隔离 | #4, #39, #76, #91, #104 | 需要开发；重复根因 | 暂不关闭 | 由 Task 4 修复 dataset item、evaluation runner、optimizer 的 tenant scope。 |
| 语义缓存 profile 隔离 | #100 | 需要开发 | 暂不关闭 | 由 Task 6 验证并修复 cache key、payload、memory store 的 tenant/profile/query 维度。 |
| 入库 KB 校验 | #59, #62 | 需要开发；重复根因 | 暂不关闭 | 由 Task 5 在导入、上传、异步 ingestion job 创建前校验 KB 存在且属于当前 tenant。 |
| 入库失败 job chunk 可见性 | #94 | 需要开发 | 暂不关闭 | 纳入 Task 5 的入库目标校验与原子可见性验证，防止失败 job 暴露 chunk。 |
| 重新入库旧 chunk | #45 | 需要开发 | 暂不关闭 | 由 Task 5 修复同一文档重新入库时旧 chunk 残留问题。 |
| Trace 已完成验证 | #58, #87, #97 | 待验证；可能已实现 | 暂不关闭 | 由 Task 8 复跑 trace 回归测试；只有失败 query trace、失败 node span、重复 trace_id 行为均通过后才关闭。 |
| 模型 API key 校验 | #7 | 需要开发或确认当前实现 | 暂不关闭 | 由 Task 7 检查真实 provider、mock/test provider 校验差异，补齐文档和测试。 |

### Task 1 结论
- Task 1 只完成审计与记录，不关闭 GitHub issue，不修改业务代码。
- 所有开放 issue 均已归入根因组并记录 close/no-close 决策。
- 未验证前不关闭任何 issue；后续关闭动作依赖 Task 8、Task 9、Task 10 的验证结果。

## 2026-07-04 - 合并 origin/main 到当前分支

### 合并证据
- 执行 `git fetch --prune origin`，命令退出码 `0`。
- 合并前 `HEAD...origin/main` 差异：本地 ahead `2`，behind `2`。
- 执行 `git merge --no-edit origin/main`，命令退出码 `0`。
- 合并结果：无冲突，未进行手工冲突修复。
- 新 HEAD：`45e6f39`。
- Merge commit 父提交：当前分支原 HEAD `e2a251d`，`origin/main` `fe011ea`。
- 合并后 `HEAD...origin/main` 差异：本地 ahead `3`，behind `0`。
- 合并涉及文件：`internal/http/router.go`、`internal/http/router_test.go`、`tests/integration/ingest_query_test.go`。
- 合并后 `git status --short --branch` 输出：`## task14-llm-judge-optimizer-validation...origin/task14-llm-judge-optimizer-validation [ahead 3]`。

### 边界说明
- 本次仅执行安全合并与证据记录，没有启动 Task 2-10 的 feature 开发。
- 未关闭 GitHub issue，未执行 feature 任务验证。

## 2026-07-04 - Task 8: 验证并关闭已完成 trace 类 issue

### 验证范围
- 失败 RAG 查询 trace 持久化：`TestRAGGraphInvokePersistsTraceOnNodeFailure` 覆盖失败 retriever 返回 error 后仍调用 `TraceStore.StoreTrace`，并持久化 `hybrid_retrieve` 失败 span。
- 失败 node span 持久化：同一测试断言持久化 spans 中包含 `hybrid_retrieve`，且错误文本包含 `retrieval unavailable`，同时不泄露原始 query。
- 重复 `trace_id` 不混合 spans：`TestRAGGraphInvokeRepeatedTraceIDPersistsSeparateSpanBatches` 覆盖 graph 每次 invocation 的 span batch 独立；`TestRepositoryStoreTraceReplacesSpansForRepeatedTraceID` 覆盖 PostgreSQL store 对重复 `trace_id` 替换 metadata 与 spans，不保留第一次调用的 error span。
- trace 读取错误统计兼容性：`TestRepositoryGetTraceFound` 覆盖 span error 会映射为 `HasError=true` 与 `ErrorCount=1`。

### 测试证据
- 执行 `CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/graph -run 'TestRAGGraphInvokePersistsTraceOnNodeFailure|TestRAGGraphInvokeRepeatedTraceIDPersistsSeparateSpanBatches|TestRAGGraphInvokeUsesRequestTraceIDInPersistence' -count=1 -v`，命令退出码 `1`；失败原因是当前环境在强制 `GOTOOLCHAIN=local` 时选择 `go1.22.5`，而 `go.mod` 要求 `go >= 1.26`，测试未实际执行。
- 执行 `CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/storage/postgres -run 'TestRepositoryStoreTraceReplacesSpansForRepeatedTraceID|TestRepositoryGetTraceMapsErrorSpans' -count=1 -v`，命令退出码 `1`；失败原因同上，测试未实际执行。
- 执行 `go version`，输出 `go version go1.26.4 darwin/amd64`；执行 `go env GOVERSION GOTOOLCHAIN`，输出 `go1.26.4` 与 `auto`。
- 执行 `CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./internal/graph -run 'TestRAGGraphInvokePersistsTraceOnNodeFailure|TestRAGGraphInvokeRepeatedTraceIDPersistsSeparateSpanBatches|TestRAGGraphInvokeUsesRequestTraceIDInPersistence' -count=1 -v`，命令退出码 `0`；三个 focused graph trace 测试全部 PASS。
- 执行 `CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./internal/storage/postgres -run 'TestRepositoryStoreTraceReplacesSpansForRepeatedTraceID|TestRepositoryGetTraceFound' -count=1 -v`，命令退出码 `0`；两个 focused PostgreSQL trace 测试全部 PASS。
- 执行 `CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./internal/graph ./internal/storage/postgres -count=1`，命令退出码 `0`；受影响 trace 包级测试全部 PASS。

### Issue 关闭证据
- 执行 `gh issue close 58 --repo shikanon/orag --reason completed --comment ...`，命令退出码 `0`；#58 已关闭。
- 执行 `gh issue close 87 --repo shikanon/orag --reason completed --comment ...`，命令退出码 `0`；#87 已关闭。
- 执行 `gh issue close 97 --repo shikanon/orag --reason completed --comment ...`，命令退出码 `0`；#97 已关闭。
- 执行 `gh issue view 58 --repo shikanon/orag --json number,state,closed,url`，确认 `state=CLOSED`、`closed=true`。
- 执行 `gh issue view 87 --repo shikanon/orag --json number,state,closed,url`，确认 `state=CLOSED`、`closed=true`。
- 执行 `gh issue view 97 --repo shikanon/orag --json number,state,closed,url`，确认 `state=CLOSED`、`closed=true`。

### Task 8 结论
- Task 8 仅执行 trace 验证、关闭 #58/#87/#97、更新 tasks/progress；未修改业务代码。
- 失败 query trace、失败 node span、重复 `trace_id` span 隔离均已通过当前测试验证。
- #58、#87、#97 均已写入关闭说明并关闭；未关闭其他 issue。

## 2026-07-04 - Task 9: 执行最终回归验证

### 工具链说明
- 直接执行 `CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ...` 初始失败，退出码 `1`；原因是当前本机底层 local Go 为 `go1.22.5`，而 `go.mod` 要求 `go >= 1.26`。
- `go env` 显示 `GOTOOLCHAIN=auto` 时使用已下载的 `go1.26.4` toolchain：`/Users/bytedance/gopath/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.4.darwin-amd64`。
- 为保留 `GOTOOLCHAIN=local` 且避免混用旧 GOROOT，实际验证命令显式设置 `GOROOT=/Users/bytedance/gopath/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.4.darwin-amd64` 并直接调用该 toolchain 的 `bin/go`。

### 回归测试证据
- 执行 `GOROOT=/Users/bytedance/gopath/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.4.darwin-amd64 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local /Users/bytedance/gopath/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.4.darwin-amd64/bin/go test ./internal/kb ./internal/storage/postgres ./internal/storage/qdrant ./internal/dataset ./internal/eval ./internal/http ./internal/ingest ./internal/rag ./internal/config ./internal/llm/provider -count=1`，命令退出码 `0`；Task 9.1 指定内部包全部 PASS。
- 执行 `GOROOT=/Users/bytedance/gopath/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.4.darwin-amd64 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local /Users/bytedance/gopath/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.4.darwin-amd64/bin/go test ./tests/contract -v`，命令退出码 `0`；`TestOpenAPI`、`TestOptimizationAndJudgeSchemasExposeAsyncContract`、`TestEvaluationSchemasExposeQualityMetrics` 全部 PASS。
- 执行 `GOROOT=/Users/bytedance/gopath/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.4.darwin-amd64 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local /Users/bytedance/gopath/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.4.darwin-amd64/bin/go test ./...`，命令退出码 `0`；全量包测试全部 PASS。

### 集成测试证据
- 执行 `GOROOT=/Users/bytedance/gopath/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.4.darwin-amd64 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local /Users/bytedance/gopath/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.4.darwin-amd64/bin/go test ./tests/integration -v`，命令退出码 `0`。
- `tests/integration` 包内用例全部按测试前置条件跳过，跳过原因均为 `integration tests require ORAG_INTEGRATION_TESTS=1 and docker compose test services`。
- 跳过用例包括 evaluation tenant isolation、Postgres/Qdrant ingest/query、missing KB ingest、failed Qdrant ingest visibility、HTTP missing KB ingest、KB delete cleanup。

### Task 9 结论
- Task 9 的必需回归测试在 `go1.26.4` local toolchain 下全部通过，无需修复业务代码。
- 可用集成测试包已运行；因当前环境未启用 `ORAG_INTEGRATION_TESTS=1` 且未启动 docker compose PostgreSQL/Qdrant 测试服务，外部依赖集成用例按设计跳过。
- 本次仅更新 Task 9 的 tasks/progress 证据，不关闭 GitHub issue，不修改业务实现文件。

## 2026-07-04 - Task 10: 关闭所有已实现 GitHub issue

### 关闭范围
- KB 删除根因组：#2、#14、#38、#42、#57、#70、#90、#101、#105。
- KB 写入错误传播根因组：#3、#78。
- 数据集/评估租户隔离根因组：#4、#39、#76、#91、#104。
- 语义缓存 profile 隔离根因组：#100。
- 入库目标校验、失败 job 可见性、重新入库旧 chunk 根因组：#59、#62、#94、#45。
- 模型 API key 默认校验根因组：#7。

### 关闭证据
- 执行 `gh issue list --repo shikanon/orag --state open --limit 100 --json number,title,url --jq '.[] | [.number, .title, .url] | @tsv'`，关闭前列出剩余开放 issue 22 个：#105、#104、#101、#100、#94、#91、#90、#78、#76、#70、#62、#59、#57、#45、#42、#39、#38、#14、#7、#4、#3、#2。
- 使用 `gh issue close <number> --repo shikanon/orag --reason completed --comment ...` 对上述 22 个 issue 逐一写入关闭说明并关闭；每条说明均包含修复范围、Task 9 验证命令和分支上下文 `task14-llm-judge-optimizer-validation`。
- 关闭命令退出码 `0`，输出确认 #2、#14、#38、#42、#57、#70、#90、#101、#105、#3、#78、#4、#39、#76、#91、#104、#100、#59、#62、#94、#45、#7 均已关闭。
- 再次执行 `gh issue list --repo shikanon/orag --state open --limit 100 --json number,title,url --jq '.[] | [.number, .title, .url] | @tsv'`，命令退出码 `0`，输出为空。
- 再次执行 `gh issue list --repo shikanon/orag --state open --limit 100 --json number --jq 'length'`，命令退出码 `0`，输出 `0`。

### Task 10 结论
- Task 10 仅执行已验证 remaining issue 的评论、关闭、open 列表复查和 tasks/progress 更新。
- 包含 Task 8 已关闭的 trace issue #58、#87、#97 在内，`shikanon/orag` 当前开放 issue 数为 `0`。
- 本次未修改业务实现文件，未重新运行测试；关闭依据复用 Task 8/Task 9 已记录的验证证据。

## 2026-07-04 - Checklist 全量复核

### 复核范围
- 读取 `spec.md`、`tasks.md`、`checklist.md`、`progress.md`，确认 `tasks.md` 中 Task 1-10 已全部勾选，`checklist.md` 中 17 个 checkpoint 尚未勾选。
- 本轮按 checkpoint 逐项对照当前代码、测试和 GitHub issue 状态，只勾选已验证通过的检查项。

### 代码与测试证据
- KB repository 错误传播：`internal/storage/postgres/repository.go` 的 `PutKnowledgeBase`、`ListKnowledgeBases`、`GetKnowledgeBase` 返回 backend 错误；`internal/http/router_test.go` 覆盖 create/list/get storage error 不返回假成功。
- KB 真实删除：`internal/kb/types.go`、`internal/storage/postgres/repository.go`、`internal/storage/qdrant/vector_store.go` 均按 `tenant_id` + `knowledge_base_id` 删除；`internal/http/router_test.go` 覆盖删除后 get/list/chunks 不再可见和缺失/跨租户 not-found。
- OpenAPI/文档/契约：`api/openapi.yaml`、`docs/api.md`、`docs/api/ingestion-and-query.md` 已描述 KB 删除和入库错误语义；`tests/contract/openapi_test.go` 验证 OpenAPI contract。
- Dataset/eval/optimizer 隔离：`internal/dataset/service.go` 在 `AddItem`/`Items` 前执行 tenant-scoped `GetDataset`；`internal/eval/service.go` 和 `internal/optimizer/service.go` 通过 tenant 读取 dataset/run/candidates；相关测试覆盖跨租户 evaluation 和 optimization 返回 `dataset_not_found`。
- 入库一致性：`internal/ingest/service.go` 在创建 job 前校验 KB 存在，重新入库同 source 前删除旧文档/旧 chunk，失败 indexing 走 failed job 且不使 chunk searchable；`internal/ingest/service_test.go` 覆盖缺失 KB、失败 job 不可检索、重新入库旧 chunk 清理。
- Semantic cache 隔离：`internal/rag/semantic_cache.go` 与 `internal/storage/qdrant/semantic_cache.go` 的 cache key/filter/payload 包含 tenant、KB、profile、top_k；`internal/rag/semantic_cache_test.go` 覆盖同 query 不同 profile miss。
- 模型 API key 校验：`internal/config/config.go` 和 `internal/llm/provider/client.go` 对真实 provider 要求 API key，mock 需显式 `ALLOW_DETERMINISTIC_MOCK=true`；`.env.example` 记录真实 provider 与 mock/test 例外；`internal/config/config_test.go`、`internal/llm/provider/client_test.go` 覆盖缺失 key 失败和 mock 通过。
- Trace issue：沿用 Task 8 已记录的 `internal/graph` 与 `internal/storage/postgres` trace 测试证据，覆盖失败 query trace、失败 node span、重复 `trace_id` span 隔离。

### 命令验证
- 执行 `git status --short --branch`，确认当前分支为 `task14-llm-judge-optimizer-validation...origin/task14-llm-judge-optimizer-validation [ahead 3]`；工作区存在与本 spec 相关的未提交代码/文档改动，本轮不回退既有改动。
- 执行 `gh issue list --repo shikanon/orag --state open --limit 100 --json number,title,url --jq 'length'`，命令退出码 `0`，输出 `0`。
- 执行 `gh issue list --repo shikanon/orag --state closed --limit 200 --json number,state --jq '[.[] | select(.number as $n | [2,3,4,7,14,38,39,42,45,57,58,59,62,70,76,78,87,90,91,94,97,100,101,104,105] | index($n))] | {closed_count:length,numbers:map(.number)|sort}'`，命令退出码 `0`，输出 `closed_count=25` 且编号完整。
- 执行 `GOROOT=/Users/bytedance/gopath/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.4.darwin-amd64 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local /Users/bytedance/gopath/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.4.darwin-amd64/bin/go test ./internal/kb ./internal/storage/postgres ./internal/storage/qdrant ./internal/dataset ./internal/eval ./internal/http ./internal/ingest ./internal/rag ./internal/config ./internal/llm/provider -count=1`，命令退出码 `0`。
- 执行 `GOROOT=/Users/bytedance/gopath/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.4.darwin-amd64 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local /Users/bytedance/gopath/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.4.darwin-amd64/bin/go test ./tests/contract -v`，命令退出码 `0`。
- 执行 `GOROOT=/Users/bytedance/gopath/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.4.darwin-amd64 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local /Users/bytedance/gopath/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.4.darwin-amd64/bin/go test ./...`，命令退出码 `0`。
- 执行 `GOROOT=/Users/bytedance/gopath/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.4.darwin-amd64 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local /Users/bytedance/gopath/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.4.darwin-amd64/bin/go test ./tests/integration -v -count=1`，命令退出码 `0`；Postgres/Qdrant 集成用例因未设置 `ORAG_INTEGRATION_TESTS=1` 且未启动 docker compose test services 按设计跳过。

### 复核结论
- `checklist.md` 的 17 个 checkpoint 均已通过代码、测试或 issue 状态核验，已全部勾选。
- `shikanon/orag` 最终开放 issue 数为 `0`，已实现 issue #2/#3/#4/#7/#14/#38/#39/#42/#45/#57/#58/#59/#62/#70/#76/#78/#87/#90/#91/#94/#97/#100/#101/#104/#105 均处于 closed 状态。
- 本轮仅更新 `checklist.md` 与 `progress.md`，未修改业务实现文件。

## Round 1

- 完成远程同步、开放 issue 审计、根因分组、功能修复、回归验证、GitHub issue 关闭和 checklist 全量验收。
- 修复并验证 KB 错误传播、真实删除、dataset/eval/optimizer 租户隔离、ingest 缺失 KB/失败 job/重新入库一致性、semantic cache profile 隔离、模型 API key 校验和 trace 相关行为。
- 关键决策：按根因合并重复 issue 后统一实现和关闭；使用 go1.26.4 toolchain 运行验证，记录本机 `GOTOOLCHAIN=local` 对 go1.22.5 的限制；未启用外部 PostgreSQL/Qdrant 时集成测试按设计跳过。
- 变更文件包括 `.env.example`、`api/openapi.yaml`、`docs/api*`、`internal/app`、`internal/http`、`internal/ingest`、`internal/kb`、`internal/rag`、`internal/storage/postgres`、`internal/storage/qdrant`、`tests/contract`、`tests/integration` 以及 `.trae/specs/resolve-github-issues/*`。

## Round 2

- **裁定**: PASS
- **复核范围**: 远程/GitHub issue 状态、KB 错误传播与真实删除、dataset/eval/optimizer 租户隔离、ingest 缺失 KB/失败 job/重新入库一致性、semantic cache profile 隔离、模型 API key 校验、trace 行为、OpenAPI 契约、集成测试入口。
- **验证结果**:
  - Build/Runtime: PASS；`CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go build ./...`、`CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go vet ./...` 均退出码 `0`；`go version` 为 `go1.26.4 darwin/amd64`。
  - Tests/Coverage: PASS；`CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./... -count=1` 退出码 `0`；`go test ./tests/contract -v` 退出码 `0`；对抗性聚焦用例覆盖跨租户 dataset/evaluation、缺失 KB 入库、semantic cache profile/top_k 隔离、Qdrant cache profile filter、真实 provider API key 缺失校验，命令退出码 `0`；`go test ./tests/integration -v -count=1` 退出码 `0` 且外部 PostgreSQL/Qdrant 用例因未设置 `ORAG_INTEGRATION_TESTS=1` 按设计跳过。
  - Checklist audit: 17/17 passed, 0 failed；`checklist.md` 所有 checkpoint 已勾选，`gh issue list --repo shikanon/orag --state open --limit 100 --json number,title,url --jq 'length'` 输出 `0`，25 个目标 issue 关闭编号复查完整。
- **风险与问题**: 未发现本轮范围内阻塞问题；剩余风险为外部 PostgreSQL/Qdrant 真实集成场景本轮未启动 docker compose 服务，只验证了集成测试入口的跳过语义。

## 2026-07-06 - Task 11: 重新同步远程并逐个审计当前开放 issue

### 远程同步证据
- 执行 `git status --short --branch && git fetch --prune origin`，命令退出码 `0`。
- 审计前工作区状态：`## main...origin/main`，未显示未提交改动。
- 当前分支：`main`。
- 本地 HEAD：`c641470`；`origin/main`：`c641470`。
- `HEAD...origin/main` 差异：本地 ahead `0`，behind `0`。
- 执行 `gh pr list --repo shikanon/orag --state open --limit 50 --json number,title,headRefName,baseRefName,url`，命令退出码 `0`，输出为空，当前无开放 PR。
- `git fetch --prune origin` 同步到新增远程分支 `origin/coding-loop/issue-135`、`origin/coding-loop/issue-136`、`origin/coding-loop/issue-137`、`origin/coding-loop/issue-141`、`origin/coding-loop/issue-143`、`origin/coding-loop/issue-148`、`origin/coding-loop/issue-150`、`origin/coding-loop/issue-151`，并清理已删除的 `origin/feat/self-check-self-ops`。

### Issue 导出证据
- 执行 `gh issue list --repo shikanon/orag --state open --limit 200 --json number,title,body,labels,url` 并按 Task 11 指定编号过滤，命令退出码 `0`。
- 精简导出命令 `gh issue list --repo shikanon/orag --state open --limit 200 --json number,title,labels,url --jq '[.[] | select(.number as $n | [115,116,117,118,119,120,121,122,123,124,129,130,142,146,147,155,156,157] | index($n))] | sort_by(.number) | .[] | [.number, .title, (.labels|map(.name)|join(",")), .url] | @tsv'` 返回 18 条目标 issue。
- 执行目标编号计数命令，输出 `18`，确认 #115、#116、#117、#118、#119、#120、#121、#122、#123、#124、#129、#130、#142、#146、#147、#155、#156、#157 均为当前开放 issue。
- 对 #115、#116、#117、#118 额外执行 `gh issue view <number> --repo shikanon/orag --json number,title,body,labels,url`，读取正文中的背景、目标、建议方案和验收标准，避免只按标题分类。

### 根因分组与 close/fix 决策

| 根因组 | Issue | 当前审计结论 | Close 决策 | 后续任务 |
| --- | --- | --- | --- | --- |
| 检索功能完成度 | #115 Contextual Retrieval、#116 RAPTOR、#117 Query Router、#118 Lightweight Graph Retrieval | 当前 `main` 已出现相关实现痕迹：`internal/app/app.go` 接入 `buildContextualizer`、`buildRAPTORBuilder`、`buildGraphBuilder`、`buildQueryRouter`；`internal/storage/postgres/repository.go` 持久化 `contextual_text` 和 graph relations；但仍需按 issue 验收标准验证配置、文档和测试覆盖。 | 暂不关闭；归类为已实现需验证。 | Task 12.1 |
| KB 删除/写入重复项 | #119、#122、#123、#124、#130 | 与第一轮 KB 错误传播和真实删除根因重复；当前代码显示 `PutKnowledgeBase(ctx, item) error` 已向 HTTP 返回 `knowledge_base_create_failed`，`deleteKnowledgeBase` 已调用 tenant-scoped `DeleteKnowledgeBase` 并对 missing 返回 404。 | 暂不关闭；归类为重复项且需引用第一轮主修复证据复核后关闭。 | Task 12.2 |
| 查询参数校验 | #120、#121 | #120 当前代码已有 `validateQueryRequest`，覆盖 `knowledge_base_id`、`query`、`profile`、`top_k` 4xx 校验；#121 当前 `retrieveOne` 仅在请求未设置 `TopK` 时写入 dense/sparse 默认值，显示请求级 `top_k` 可能已修复，但仍需聚焦测试确认 hybrid 和 optimizer 路径。 | 暂不关闭；归类为已实现迹象但需验证，不通过则修复。 | Task 13 |
| Docker 默认配置 | #142、#147 | `deployments/docker-compose.yml` 的 `orag-api` 仍只读取 `../.env`，未覆盖容器网络中的 `DATABASE_URL` 和 `QDRANT_HOST`；若 `.env` 沿用 `.env.example` 的 localhost 默认值，issue 风险仍成立。 | 暂不关闭；归类为未实现需修复。 | Task 14 |
| Optimizer 取消/预算/候选隔离 | #146、#155、#157 | #155 当前 `InternalRAGRunner.configureCandidateService` 对 `BaseRAG` 浅拷贝后未清空或重建 `Pipeline`，候选参数可能被旧 pipeline 绕过；#146/#157 当前 `shouldStop` 主要在候选执行前检查，候选完成后进入评分/完成前缺少明确的取消和成本预算再检查。 | 暂不关闭；归类为未实现或需修复验证。 | Task 15 |
| Semantic cache 隔离 | #129、#156 | #129 与第一轮 profile-scoped semantic cache 修复重复，当前 `LookupSemanticCache`/`StoreSemanticCache` 已传递 profile/top_k；#156 是 optimizer candidate 维度隔离问题，当前 `InternalRAGRunner` 仍复用 `BaseRAG.Cache`，未发现 candidate/run namespace 注入或候选评估禁用共享 cache。 | #129 暂不关闭，需引用第一轮主修复证据复核；#156 暂不关闭，需修复。 | Task 12.3、Task 16 |

### Task 11 结论
- Task 11 已完成远程同步、18 个目标开放 issue 的标题/正文/标签/URL 导出、根因分组和 close/fix 决策记录。
- 本次只更新 `.trae/specs/resolve-github-issues/tasks.md`、`.trae/specs/resolve-github-issues/checklist.md` 和 `.trae/specs/resolve-github-issues/progress.md`；未修改业务代码，未关闭 GitHub issue。
- `checklist.md` 中“第二轮当前开放 issue 已逐个导出、分组，并记录 fix-or-close 决策”已勾选。
- 后续关闭动作必须在 Task 12、Task 13、Task 14、Task 15、Task 16 的验证或修复完成后执行。

## 2026-07-06 - Task 14: 修复 Docker 和 docker-run 默认配置

### 修复范围
- 覆盖 issue：#142、#147。
- 修改 `deployments/docker-compose.yml`：`orag-api` 继续可读取 `../.env` 作为本地密钥来源，但将该文件设为 `required: false`，并通过 `environment` 显式覆盖容器网络依赖地址。
- Compose 默认值改为 `DATABASE_URL=${DOCKER_DATABASE_URL:-postgres://orag:orag@postgres:5432/orag?sslmode=disable}`、`QDRANT_HOST=${DOCKER_QDRANT_HOST:-qdrant}`、`QDRANT_GRPC_PORT=${DOCKER_QDRANT_GRPC_PORT:-6334}`，避免容器内继承宿主 `.env.example` 风格的 `localhost` 依赖地址。
- 更新 `docs/development.md`、`docs/operations.md`、`docs/operations/README.md`，说明完整 Compose 栈默认使用容器服务名，若需覆盖容器依赖地址应使用 `DOCKER_DATABASE_URL`、`DOCKER_QDRANT_HOST`、`DOCKER_QDRANT_GRPC_PORT`。
- 新增 `tests/contract/docker_compose_test.go`，用静态契约测试锁定 Docker Compose 的容器网络默认配置，防止回归到宿主 `localhost` 默认值。

### 缺陷验证过程
- 先新增 `TestDockerComposeAPIUsesContainerNetworkDefaults` 后执行 `CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./tests/contract -run TestDockerComposeAPIUsesContainerNetworkDefaults -count=1 -v`，命令退出码 `1`；失败原因是当前 `docker-compose.yml` 缺少 `DATABASE_URL` 的容器网络默认覆盖，证明 #142/#147 风险存在。
- 修复 `deployments/docker-compose.yml` 后再次执行同一命令，命令退出码 `0`，新增契约测试通过。

### 验证证据
- 执行 `docker compose -f deployments/docker-compose.yml config`，命令退出码 `0`；解析后的 `orag-api.environment` 中 `DATABASE_URL` 为 `postgres://orag:orag@postgres:5432/orag?sslmode=disable`，`QDRANT_HOST` 为 `qdrant`，`QDRANT_GRPC_PORT` 为 `6334`。
- 执行 `CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./tests/contract -count=1`，命令退出码 `0`；契约测试包全部通过。
- 执行 `gofmt -w tests/contract/docker_compose_test.go`，命令退出码 `0`。

### Task 14 结论
- Task 14 已完成 #142/#147 的 Docker/docker-run 默认配置修复、文档更新和配置静态测试覆盖。
- `tasks.md` 中 Task 14 及 SubTask 14.1-14.3 已勾选。
- `checklist.md` 中“#142 和 #147 的 Docker/docker-run 默认配置不会在容器网络内错误连接 localhost，并有文档或配置检查覆盖”已勾选。
- 本次未关闭 GitHub issue；最终关闭动作仍依赖 Task 17/18 的聚焦测试、全量验证、PR 和 issue 关闭流程。

## 2026-07-06 - Task 15: 修复 optimizer 候选、取消和预算一致性

### 修复范围
- 覆盖 issue：#155、#146、#157。
- #155：修复 `internal/optimizer/internal_runner.go` 的 `InternalRAGRunner.configureCandidateService`，候选 RAG service clone 后显式清空 `Pipeline`，避免 `rag.Service.Query` 优先走生产旧 pipeline 而绕过候选的 retrieval、reranker、graph 参数。
- #146：修复 `internal/optimizer/service.go` 的 run loop，在候选执行期间发生取消时合并当前 run 控制状态，并在候选完成后再次执行停止检查，防止最后一个候选完成后的评分/完成流程把 cancel 覆盖成 completed。
- #157：修复 `internal/optimizer/service.go` 的候选后预算检查，候选执行后立即检查 judge call 和 cost budget；最终成本超支时进入 `budget_stopped`，不再继续评分并标记 completed。候选前仍保留 wall-time 预算检查，避免改变已有 resume 语义。
- 新增 `internal/optimizer/internal_runner_test.go` 的 `TestInternalRAGRunnerClearsBasePipelineForCandidateClone`，覆盖候选 clone 不复用旧 pipeline 且不污染 base service。
- 新增 `internal/optimizer/service_test.go` 的 `TestServiceCancelDuringLastCandidateWinsOverCompleted` 和 `TestServiceCostBudgetExceededAfterCandidateStopsRun`，覆盖取消优先级和成本预算超支终态。

### 缺陷验证过程
- 先新增 Task15 三个回归测试后执行 `GOROOT=/Users/bytedance/gopath/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.4.darwin-amd64 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local /Users/bytedance/gopath/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.4.darwin-amd64/bin/go test ./internal/optimizer -run 'TestInternalRAGRunnerClearsBasePipelineForCandidateClone|TestServiceCancelDuringLastCandidateWinsOverCompleted|TestServiceCostBudgetExceededAfterCandidateStopsRun' -count=1 -v`，命令退出码 `1`。
- 失败现象分别为：candidate clone 仍保留 `optimizer.fakePipeline{}`；候选运行中取消后 run 被标记为 `completed`；最终成本 `0.1` 超过 `MaxCostUSD=0.05` 后 run 仍被标记为 `completed`。三项均稳定复现 Task15 对应缺陷。
- 修复后再次执行同一 focused 命令，命令退出码 `0`；三项新增回归测试全部 PASS。
- 扩大执行 `GOROOT=/Users/bytedance/gopath/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.4.darwin-amd64 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local /Users/bytedance/gopath/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.4.darwin-amd64/bin/go test ./internal/optimizer -count=1 -v` 首次退出码 `1`，既有 `TestServiceEmptyResumeUsesStoredRunConfig` 显示 resume 后被 wall-time 后置检查拦为 `budget_stopped`。
- 第二轮修复将候选后停止检查限定为取消、judge call 和 cost budget，wall-time 只在候选执行前检查；随后 `go test ./internal/optimizer -count=1 -v` 命令退出码 `0`，optimizer 包级测试全部 PASS。

### Task 15 结论
- Task 15 已完成 #155/#146/#157 的 optimizer candidate pipeline、取消优先级和成本预算终态修复。
- `tasks.md` 中 Task 15 及 SubTask 15.1-15.4 已勾选。
- `checklist.md` 中 #155、#146、#157 三个 checkpoint 已勾选。
- 本次未关闭 GitHub issue；最终关闭动作仍依赖 Task 17/18 的聚焦测试、全量验证、PR 和 issue 关闭流程。

## 2026-07-06 - Task 13: 修复查询参数校验和 top_k 覆盖问题

### 修复范围
- #120 查询参数校验：当前 `internal/http/router.go` 已在 `query` 和 `queryStream` 中于调用 RAG 前执行 `validateQueryRequest`，覆盖缺失/空白 `knowledge_base_id`、缺失/空白 `query`、非法 `profile`、非法 `top_k`，并由 `internal/http/router_test.go` 的 invalid query 测试验证稳定 `400 invalid_request` 且不调用 RAG pipeline。
- #121 请求级 `top_k` 优先级：修复 `internal/kb/retrievers.go` 的 `HybridRetriever.RetrieveWithWarnings`，使 dense/sparse 候选请求构造遵循 `request DenseTopK/SparseTopK` > request `TopK` > configured `DenseTopK/SparseTopK` 的优先级。显式请求 `TopK` 不再被全局配置默认值覆盖；请求未提供 `TopK` 时仍使用配置默认候选数量。

### 失败复现证据
- 新增 `TestHybridRetrieverRequestTopKOverridesConfiguredCandidateTopK` 和 `TestHybridRetrieverUsesConfiguredCandidateTopKWhenRequestTopKAbsent`，先运行 `go test ./internal/kb -run 'TestHybridRetriever(RequestTopKOverridesConfiguredCandidateTopK|UsesConfiguredCandidateTopKWhenRequestTopKAbsent)' -count=1 -v`，命令退出码 `1`。
- 失败用例 `TestHybridRetrieverRequestTopKOverridesConfiguredCandidateTopK` 显示 dense 下游收到 `SearchRequest{TopK:50, DenseTopK:50}`，而期望显式请求 `TopK:5` 不被 configured `DenseTopK:50` 覆盖，确认 #121 缺陷可稳定复现。

### 测试验证证据
- 执行 `go test ./internal/kb -run 'TestHybridRetriever(RequestTopKOverridesConfiguredCandidateTopK|UsesConfiguredCandidateTopKWhenRequestTopKAbsent)' -count=1 -v`，命令退出码 `0`；新增 top_k 优先级测试全部 PASS。
- 执行 `go test ./internal/http -run 'TestQueryRejectsInvalidRequests|TestQueryStreamRejectsInvalidRequests|TestInvalidQueryRequestsDoNotIncrementRAGSuccessMetrics' -count=1`，命令退出码 `0`；#120 HTTP 查询校验聚焦测试 PASS。
- 执行 `go test ./internal/rag -run 'TestRetrieveExpandedUsesEffectiveTopKWithHybridRetrieverDefault|TestExecuteExplicitTopKNotTruncatedByContextTopN' -count=1`，命令退出码 `0`；RAG service top_k 传播聚焦测试 PASS。
- 执行 `go test ./internal/kb -count=1`，命令退出码 `0`；KB 包级回归 PASS。
- 执行 `go test ./internal/rag ./internal/graph ./internal/eval -run 'TopK|Optimizer|Retrieve' -count=1`，命令退出码 `0`；top_k/retrieve/optimizer 相关聚焦回归 PASS。
- 执行 `go test ./internal/http -count=1`，命令退出码 `0`；HTTP 包级回归 PASS。

### Task 13 结论
- Task 13 已完成 #120 验证和 #121 修复，`tasks.md` 中 Task 13 及其 3 个 SubTask 已勾选。
- `checklist.md` 中 #120 和 #121 对应 checkpoint 已勾选。
- 本次未关闭 GitHub issue；关闭动作保留到 Task 17 的第二轮最终验证与关闭阶段统一执行。

## 2026-07-06 - Task 12: 验证并关闭已完成或重复的历史功能 issue

### 验证范围
- 检索功能完成度：#115 Contextual Retrieval、#116 RAPTOR、#117 Query Router、#118 Lightweight Graph Retrieval。
- 第一轮重复项：#119、#123 的 PostgreSQL KB 写入错误传播；#122、#124、#130 的真实 KB 删除。
- Semantic cache profile 隔离：#129；与 #156 optimizer candidate cache 隔离保持区分，#156 仍留待 Task 16 修复。

### 代码与文档证据
- Contextual Retrieval：`internal/app/app.go` 接入 `buildContextualizer`；`internal/ingest/service.go` 将 `ContextualText` 写入 chunk，并以 contextual search text 作为 embedding 输入；`internal/storage/postgres/repository.go` 持久化 `contextual_text`；`docs/plans/2026-07-05-issue-115-contextual-retrieval.md` 记录实现计划。
- RAPTOR：`internal/app/app.go` 接入 `buildRAPTORBuilder`；`internal/ingest/raptor.go` 生成递归摘要 chunk；`internal/ingest/service.go` 将 RAPTOR summary chunk 纳入索引；`docs/plans/2026-07-05-issue-116-raptor-index.md` 记录实现计划。
- Query Router：`internal/app/app.go` 接入 `buildQueryRouter`；`internal/rag/service.go` 调用 `QueryRouter.Route` 并支持 direct route bypass retrieval；`docs/plans/2026-07-05-issue-117-query-router.md` 记录实现计划。
- Lightweight Graph Retrieval：`internal/app/app.go` 接入 graph builder/retriever；`internal/ingest/graph.go` 抽取轻量 entity relations；`internal/kb/graph.go` 和 `internal/storage/postgres/repository.go` 支持 graph relation 存取；`docs/plans/2026-07-05-issue-118-lightweight-graph-retrieval.md` 记录实现计划。
- KB 写入错误传播：`internal/storage/postgres/repository.go` 的 `PutKnowledgeBase`、`ListKnowledgeBases`、`GetKnowledgeBase` 返回 backend 错误，不再静默吞错。
- 真实 KB 删除：`internal/kb/types.go`、`internal/storage/postgres/repository.go`、`internal/storage/qdrant/vector_store.go` 均按 tenant 与 KB id 删除对应状态；HTTP 删除后 get/list/query 返回稳定 not-found。
- Semantic cache profile 隔离：`internal/rag/semantic_cache.go` 和 `internal/storage/qdrant/semantic_cache.go` 的 key/filter/payload 覆盖 tenant、KB、profile、top_k；mismatched stored profile 会被拒绝。

### 测试证据
- 执行 `CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./internal/ingest ./internal/rag ./internal/kb ./internal/http ./internal/storage/postgres ./internal/storage/qdrant -run 'TestIngestEmbedsContextualSearchTextAndStoresContext|TestIngestIndexesRAPTORSummaryChunks|TestLLMRAPTORBuilderCreatesRecursiveSummaryChunks|TestLightweightGraphBuilderExtractsEntityRelations|TestIngestStoresGraphRelationsWhenIndexerSupportsGraph|TestExecuteDirectRouteBypassesRetrieval|TestInMemorySemanticCacheIsolatesProfileAndTopK|TestCacheKeyIncludesProfileAndTopK|TestSemanticCacheStoreUsesRequestProfile|TestLookupSemanticCacheRejectsMismatchedStoredProfile|TestMemoryStoreDeleteKnowledgeBaseIsTenantScopedAndCleansChunks|TestDeleteKnowledgeBaseRemovesItFromGetListAndMemoryChunks|TestRepositoryPutKnowledgeBaseReturnsExecError|TestRepositoryListKnowledgeBasesReturnsQueryError|TestRepositoryGetKnowledgeBaseReturnsScanError|TestRepositoryDeleteKnowledgeBaseLocksAndDeletesChildrenInTransaction|TestRepositoryDeleteKnowledgeBaseMissingDoesNotDeleteChildren|TestRepositoryDeleteKnowledgeBaseRollsBackOnChildDeleteError|TestQdrantSemanticCacheFilterIncludesProfileAndTopK|TestQdrantSemanticCacheStorePayloadIncludesProfileAndTopK' -count=1 -v`，命令退出码 `0`；`internal/ingest`、`internal/rag`、`internal/kb`、`internal/http`、`internal/storage/postgres` 聚焦用例全部 PASS；`internal/storage/qdrant` 因测试名不匹配显示 no tests to run。
- 补跑 `CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./internal/storage/qdrant -run 'TestSemanticCachePayloadRoundTrip|TestSemanticCacheLookupFilterIncludesProfile|TestSemanticCachePointKeyUsesResolvedProfile|TestSemanticCacheLookupPayloadRequiresMatchingProfile|TestSemanticCacheDeleteKnowledgeBaseUsesTenantScopedFilter' -count=1 -v`，命令退出码 `0`；Qdrant semantic cache profile/filter/payload/delete 聚焦用例全部 PASS。

### Issue 关闭证据
- 执行 `gh issue list --repo shikanon/orag --state open --limit 200 --json number,title,url --jq '[.[] | select(.number as $n | [115,116,117,118,119,122,123,124,129,130] | index($n))] | sort_by(.number) | .[] | [.number, .title, .url] | @tsv'`，关闭前确认 #115、#116、#117、#118、#119、#122、#123、#124、#129、#130 均为开放状态。
- 使用 `gh issue close <number> --repo shikanon/orag --reason completed --comment ...` 对 #115、#116、#117、#118、#119、#122、#123、#124、#129、#130 逐一写入关闭说明并关闭；每条说明均包含修复或重复项范围、验证命令和分支上下文 `main` at `c641470`。
- 执行 `gh issue list --repo shikanon/orag --state open --limit 200 --json number,title,url --jq '[.[] | select(.number as $n | [115,116,117,118,119,122,123,124,129,130] | index($n))] | length'`，命令退出码 `0`，输出 `0`。
- 执行 `gh issue list --repo shikanon/orag --state closed --limit 200 --json number,state,closed,url --jq '[.[] | select(.number as $n | [115,116,117,118,119,122,123,124,129,130] | index($n))] | {closed_count:length,numbers:map(.number)|sort}'`，命令退出码 `0`，输出 `closed_count=10` 且编号为 `[115,116,117,118,119,122,123,124,129,130]`。

### Task 12 结论
- Task 12 已完成现有功能和重复 issue 的验证、关闭说明写入、关闭操作与状态复查。
- `tasks.md` 中 Task 12 及 SubTask 12.1-12.4 已全部勾选；`checklist.md` 中 #115-#118、#119/#122/#123/#124/#130、#129 三项已勾选。
- 本次未修改业务实现文件；仍未处理 #120、#121、#142、#146、#147、#155、#156、#157，后续继续由 Task 13-16/17 收口。

## 2026-07-06 - Task 16: 修复 semantic cache optimizer candidate 隔离

### 修复范围
- 覆盖 issue：#156。
- 修改 `internal/rag/types.go`、`internal/rag/semantic_cache.go` 和 `internal/rag/service.go`，为 semantic cache lookup/store 增加可选 `SemanticCacheNamespace` 维度。
- 修改 `internal/optimizer/internal_runner.go`，让 `InternalRAGRunner.configureCandidateService` 为每个 optimizer candidate clone 设置稳定命名空间 `optimizer_candidate:<candidate_id>`，避免相同 tenant/KB/profile/top_k/query 在不同候选配置之间互相命中缓存。
- 修改 `internal/storage/qdrant/semantic_cache.go`，在 Qdrant semantic cache point key、filter 和 payload 中支持非空 candidate namespace；非空 namespace 使用独立 `v3` payload/filter，空 namespace 继续使用既有 `v2` key/version 和旧 filter/payload，保留正常在线 profile-scoped cache 行为，避免生产查询产生不必要的缓存碎片或命中候选缓存。

### 测试覆盖
- 新增/扩展 `internal/optimizer/internal_runner_test.go`，覆盖 candidate clone 写入不同 semantic cache namespace，且 base RAG 仍保持空生产 namespace。
- 新增/扩展 `internal/rag/semantic_cache_test.go` 和 `internal/rag/service_test.go`，覆盖内存 semantic cache 按 namespace 隔离、空 namespace 不命中 candidate 缓存、cache key 仍按 tenant/KB/profile/top_k/query 隔离。
- 新增/扩展 `internal/storage/qdrant/semantic_cache_test.go`，覆盖 Qdrant filter/payload/point key 包含非空 candidate namespace，并确认非空 namespace 使用 `v3`、空 namespace 继续使用 `v2` 且不额外添加 `cache_namespace` filter 或 payload 字段。

### 验证证据
- 执行 `gofmt -w internal/rag/types.go internal/rag/semantic_cache.go internal/rag/service.go internal/rag/service_test.go internal/rag/semantic_cache_test.go internal/storage/qdrant/semantic_cache.go internal/storage/qdrant/semantic_cache_test.go internal/optimizer/internal_runner.go internal/optimizer/internal_runner_test.go`，命令退出码 `0`。
- 执行 `CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./internal/optimizer ./internal/rag ./internal/storage/qdrant -run 'TestInternalRAGRunnerScopesSemanticCacheByCandidateID|TestInternalRAGRunnerAppliesCandidateToClonedService|TestInMemorySemanticCacheIsolatesNamespace|TestInMemorySemanticCacheIsolatesProfileAndTopK|TestCacheKeyIncludesProfileAndTopK|TestLookupSemanticCachePreservesCachedProfile|TestSemanticCacheStoreUsesRequestProfile|TestSemanticCachePayloadRoundTrip|TestSemanticCacheLookupFilterIncludesProfile|TestSemanticCacheLookupFilterOmitsEmptyNamespace|TestSemanticCachePointKeyUsesResolvedProfile' -count=1 -v`，命令退出码 `0`；Task16 聚焦测试全部 PASS。
- 执行 `CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./internal/optimizer ./internal/rag ./internal/storage/qdrant -count=1`，命令退出码 `0`；受影响包级测试全部 PASS。

### Task 16 结论
- Task 16 已完成 #156 的 optimizer candidate semantic cache 隔离修复，同时保留正常在线查询的 profile-scoped cache 行为。
- `tasks.md` 中 Task 16 及 SubTask 16.1-16.3 已勾选。
- `checklist.md` 中 #156 checkpoint 已勾选。
- 本次未关闭 GitHub issue；关闭动作仍保留到 Task 17 的第二轮最终验证与关闭阶段统一执行。

## 2026-07-06 - Task 17: 执行第二轮最终验证并关闭 issue

### 验证范围
- 覆盖 Task 13-16 的剩余第二轮 issue：#120、#121、#142、#146、#147、#155、#156、#157。
- 验证查询必填字段和 `top_k` 优先级、Docker Compose 容器网络默认配置、optimizer candidate pipeline/取消/成本预算终态、semantic cache optimizer candidate namespace 隔离。

### Toolchain 兼容记录
- 先按 Task 17 原始命令执行 `CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ...`，命令退出码 `1`。
- 失败原因：`go.mod` 要求 `go >= 1.26`，但本机 `GOTOOLCHAIN=local` 解析到系统 `go1.22.5`，输出 `go: go.mod requires go >= 1.26 (running go 1.22.5; GOTOOLCHAIN=local)`。
- 后续验证显式使用已安装工具链 `GOROOT=/Users/bytedance/gopath/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.4.darwin-amd64` 和对应 `bin/go`，保持 `CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local`。

### 测试验证证据
- 执行 Task 13-16 聚焦测试：`GOROOT=/Users/bytedance/gopath/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.4.darwin-amd64 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local /Users/bytedance/gopath/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.4.darwin-amd64/bin/go test ./internal/kb ./internal/http ./internal/rag ./internal/optimizer ./internal/storage/qdrant ./tests/contract -run 'TestHybridRetriever(RequestTopKOverridesConfiguredCandidateTopK|UsesConfiguredCandidateTopKWhenRequestTopKAbsent)|TestQueryRejectsInvalidRequests|TestQueryStreamRejectsInvalidRequests|TestInvalidQueryRequestsDoNotIncrementRAGSuccessMetrics|TestRetrieveExpandedUsesEffectiveTopKWithHybridRetrieverDefault|TestExecuteExplicitTopKNotTruncatedByContextTopN|TestInternalRAGRunnerClearsBasePipelineForCandidateClone|TestInternalRAGRunnerScopesSemanticCacheByCandidateID|TestServiceCancelDuringLastCandidateWinsOverCompleted|TestServiceCostBudgetExceededAfterCandidateStopsRun|TestInMemorySemanticCacheIsolatesNamespace|TestSemanticCachePayloadRoundTrip|TestSemanticCacheLookupFilterIncludesProfile|TestSemanticCacheLookupFilterOmitsEmptyNamespace|TestSemanticCachePointKeyUsesResolvedProfile|TestDockerComposeAPIUsesContainerNetworkDefaults' -count=1 -v`，命令退出码 `0`。
- 执行契约测试：`GOROOT=/Users/bytedance/gopath/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.4.darwin-amd64 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local /Users/bytedance/gopath/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.4.darwin-amd64/bin/go test ./tests/contract -v`，命令退出码 `0`。
- 执行全量测试：`GOROOT=/Users/bytedance/gopath/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.4.darwin-amd64 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local /Users/bytedance/gopath/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.4.darwin-amd64/bin/go test ./...`，命令退出码 `0`；`tests/contract`、`tests/integration`、`tests/live` 均通过。
- 执行 Docker Compose 配置复核：`docker compose -f deployments/docker-compose.yml config` 后检查 `DATABASE_URL`、`QDRANT_HOST`、`QDRANT_GRPC_PORT` 默认值，命令退出码 `0`，输出 `compose defaults verified`。

### Issue 关闭证据
- 执行 `gh issue list --repo shikanon/orag --state open --limit 200 --json number,title,url --jq '[.[] | select(.number as $n | [120,121,142,146,147,155,156,157] | index($n))] | sort_by(.number) | .[] | [.number, .title, .url] | @tsv'`，关闭前确认 #120、#121、#142、#146、#147、#155、#156、#157 均为开放状态。
- 使用 `gh issue close <number> --repo shikanon/orag --reason completed --comment ...` 对 #120、#121、#142、#146、#147、#155、#156、#157 逐一写入关闭说明并关闭；每条说明均包含修复范围、验证命令和上下文 `Task17 final verification on main working tree at c641470`。
- 执行 `gh issue list --repo shikanon/orag --state open --limit 200 --json number,title,url --jq '[.[] | select(.number as $n | [120,121,142,146,147,155,156,157] | index($n))] | length'`，命令退出码 `0`，输出 `0`。
- 执行 `gh issue list --repo shikanon/orag --state closed --limit 200 --json number,state,closed,url --jq '[.[] | select(.number as $n | [120,121,142,146,147,155,156,157] | index($n))] | {closed_count:length,numbers:map(.number)|sort}'`，命令退出码 `0`，输出 `closed_count=8` 且编号为 `[120,121,142,146,147,155,156,157]`。
- 执行 `gh issue list --repo shikanon/orag --state open --limit 200 --json number,title,url --jq '.[] | [.number, .title, .url] | @tsv'`，命令退出码 `0`，输出为空，当前仓库开放 issue 列表为空。

### Task 17 结论
- Task 17 已完成第二轮最终聚焦测试、契约测试、全量 Go 测试、Docker Compose 配置复核和剩余已完成 issue 关闭。
- `tasks.md` 中 Task 17 及 SubTask 17.1-17.4 已全部勾选。
- `checklist.md` 中“第二轮聚焦测试、契约测试和全量 Go 测试通过”和“所有第二轮已完成 issue 已写入关闭说明并关闭”已勾选。
- Task 18 的 PR、merge 和本地 `main` 同步检查仍未执行，相关 checklist 项保持未勾选。
