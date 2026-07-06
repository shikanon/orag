# Tasks
- [x] Task 1: 同步远程并建立 issue 审计清单
  - [x] SubTask 1.1: 运行 `git fetch --prune origin`，确认当前分支、`origin/main`、相关 `origin/coding-loop/*` 分支状态。
  - [x] SubTask 1.2: 使用 `gh issue list --repo shikanon/orag --state open --limit 100 --json number,title,body,labels,url` 导出开放 issue。
  - [x] SubTask 1.3: 将 issue 合并到根因组：KB 删除、KB 写入错误、数据集/评估租户隔离、语义缓存 profile、入库 KB 校验、重新入库旧 chunk、trace 已完成验证、模型 API key 校验。
  - [x] SubTask 1.4: 记录每个 issue 的 close 决策和验证证据，未验证前不得关闭。

- [x] Task 2: 实现知识库 repository 错误传播
  - [x] SubTask 2.1: 修改 `internal/kb` repository 接口，使知识库写入、列表和读取支持 `context.Context` 与错误返回。
  - [x] SubTask 2.2: 修改 PostgreSQL repository，不再吞掉 `PutKnowledgeBase`、`ListKnowledgeBases`、`GetKnowledgeBase` 的数据库错误。
  - [x] SubTask 2.3: 修改 app/http 调用链，把 backend 错误映射为稳定 5xx，把 not-found 保留为 404。
  - [x] SubTask 2.4: 增加单元测试覆盖写入失败不返回 `201`、读取失败不被折叠为空列表或 not found。

- [x] Task 3: 实现真实知识库删除
  - [x] SubTask 3.1: 为 memory backend 实现租户作用域的 `DeleteKnowledgeBase(ctx, tenantID, id) (bool, error)`，删除 KB、documents 和 chunks。
  - [x] SubTask 3.2: 为 PostgreSQL backend 实现事务删除，按 tenant 和 KB id 删除 chunks、documents、ingestion_jobs 与 knowledge_bases。
  - [x] SubTask 3.3: 为 Qdrant backend 实现 tenant_id + knowledge_base_id 过滤删除，避免跨租户和全量删除。
  - [x] SubTask 3.4: 串接 app/http 的 `DELETE /v1/knowledge-bases/{id}`，成功返回 `204`，缺失返回稳定 404，backend 错误返回 5xx。
  - [x] SubTask 3.5: 更新 OpenAPI、API 文档和契约测试，说明删除范围与 404 语义。

- [x] Task 4: 修复数据集样本与评估租户隔离
  - [x] SubTask 4.1: 修改 dataset service/repository 的 `AddItem`、`Items`、PostgreSQL `DatasetItems` 调用，传递并校验 tenantID。
  - [x] SubTask 4.2: 修改 HTTP add-item handler，跨租户 dataset 返回 404 或稳定 not-found，不插入样本。
  - [x] SubTask 4.3: 修改 evaluation runner 和 optimizer 路径，读取 dataset item 前校验 tenant 归属。
  - [x] SubTask 4.4: 增加 memory、PostgreSQL、HTTP、eval 回归测试，覆盖跨租户 dataset 写入和评估拒绝。

- [x] Task 5: 修复入库目标校验与重新入库一致性
  - [x] SubTask 5.1: 在文档导入、上传和异步 ingestion job 创建前校验知识库存在且属于当前 tenant。
  - [x] SubTask 5.2: 让缺失 KB 的入库请求返回稳定 not-found，且不提交 documents/chunks/jobs。
  - [x] SubTask 5.3: 修复同一文档重新入库时旧 chunk 残留问题，保证新结果可见前旧 chunk 被删除或被版本过滤。
  - [x] SubTask 5.4: 增加 HTTP、ingest service、PostgreSQL 集成或可跳过集成测试，覆盖缺失 KB、失败 job、重新入库旧 chunk 清理。

- [x] Task 6: 修复语义缓存 profile 隔离
  - [x] SubTask 6.1: 检查 semantic cache key、PostgreSQL/Qdrant payload、memory store 的 tenant/profile/query 维度。
  - [x] SubTask 6.2: 修改 cache lookup/write，使不同 profile 的相同 query 不共享缓存。
  - [x] SubTask 6.3: 增加 RAG service 和 cache store 测试，覆盖同 tenant 不同 profile 的 cache miss 与同 profile 命中。

- [x] Task 7: 实现模型 API Key 默认校验
  - [x] SubTask 7.1: 检查当前 config/provider registry 对 Ark、mock、local/test provider 的校验差异。
  - [x] SubTask 7.2: 对真实 provider 默认要求必要 API key，保留测试和 mock 模式的显式豁免。
  - [x] SubTask 7.3: 更新 `.env.example`、README 或开发文档，说明本地 mock 与真实 provider 配置方式。
  - [x] SubTask 7.4: 增加 config/provider 测试，覆盖缺失 API key 失败和 mock 模式通过。

- [x] Task 8: 验证并关闭已完成 trace 类 issue
  - [x] SubTask 8.1: 复跑 trace 相关测试，确认失败 RAG 查询 trace 持久化、失败 node span 持久化、重复 trace_id 不混合 spans。
  - [x] SubTask 8.2: 若测试通过，关闭 trace 相关 issue；若失败，追加修复任务并先修复再关闭。

- [x] Task 9: 执行最终回归验证
  - [x] SubTask 9.1: 运行 `CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./internal/kb ./internal/storage/postgres ./internal/storage/qdrant ./internal/dataset ./internal/eval ./internal/http ./internal/ingest ./internal/rag ./internal/config ./internal/llm/provider -count=1`。
  - [x] SubTask 9.2: 运行 `CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./tests/contract -v`。
  - [x] SubTask 9.3: 运行 `CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./...`；若当前工具链或外部依赖限制失败，记录明确原因并修复可修复问题。
  - [x] SubTask 9.4: 运行可用的集成测试或记录缺少 PostgreSQL/Qdrant 时的跳过原因。

- [x] Task 10: 关闭所有已实现 GitHub issue
  - [x] SubTask 10.1: 对每个根因组列出覆盖的 issue 编号，确保修复、验证和文档完成。
  - [x] SubTask 10.2: 使用 `gh issue comment` 写入关闭说明，包含修复范围和验证命令。
  - [x] SubTask 10.3: 使用 `gh issue close` 关闭已完成 issue，重复 issue 引用主修复组。
  - [x] SubTask 10.4: 再次运行 `gh issue list --repo shikanon/orag --state open`，确认没有已完成但仍开放的 issue。

# Task Dependencies
- Task 1 必须先完成，作为所有 issue 分组和关闭操作的依据。
- Task 2、Task 3、Task 4、Task 5、Task 6、Task 7 可在 Task 1 之后并行开发，但同一文件的修改需要协调合并。
- Task 8 可在 Task 1 之后并行验证，因为 trace 类功能已有历史规格实现。
- Task 9 依赖 Task 2 到 Task 8。
- Task 10 依赖 Task 9，且只有通过验证的 issue 才能关闭。

## 第二轮 Issue 收口任务
- [x] Task 11: 重新同步远程并逐个审计当前开放 issue
  - [x] SubTask 11.1: 运行 `git fetch --prune origin`，确认本地 `main` 与 `origin/main` 状态、工作区是否干净、是否存在未合并 PR。
  - [x] SubTask 11.2: 导出当前开放 issue #115、#116、#117、#118、#119、#120、#121、#122、#123、#124、#129、#130、#142、#146、#147、#155、#156、#157 的标题、正文、标签和 URL。
  - [x] SubTask 11.3: 将 issue 按根因分组为检索功能完成度、KB 删除/写入重复项、查询参数校验、Docker 默认配置、optimizer 取消/预算/候选隔离、semantic cache 隔离。
  - [x] SubTask 11.4: 为每个 issue 记录 close 决策：已实现需验证、重复需引用主 issue、未实现需修复。

- [x] Task 12: 验证并关闭已完成或重复的历史功能 issue
  - [x] SubTask 12.1: 验证 Contextual Retrieval、RAPTOR、Query Router、Lightweight Graph Retrieval 对应 #115、#116、#117、#118 是否已由现有代码、文档和测试满足。
  - [x] SubTask 12.2: 验证 KB 删除和 Postgres KB 写入错误对应 #119、#122、#123、#124、#130 是否已由第一轮修复覆盖。
  - [x] SubTask 12.3: 验证 semantic cache profile 隔离 #129 是否已由第一轮修复覆盖，且不会与第二轮 optimizer candidate 隔离混淆。
  - [x] SubTask 12.4: 对验证通过的 issue 写入关闭说明并关闭；未通过则追加到后续修复任务。

- [x] Task 13: 修复查询参数校验和 top_k 覆盖问题
  - [x] SubTask 13.1: 修复 #120，确保 query handler 对必填字段、空 query、非法 knowledge_base_id 或缺失请求体返回稳定 4xx。
  - [x] SubTask 13.2: 修复 #121，确保请求级 `top_k` 不被 hybrid retrieval 默认值覆盖，并在不同 retriever 路径保持一致。
  - [x] SubTask 13.3: 增加 HTTP、RAG service 或 retriever 单元测试，覆盖缺失字段、空字段、请求 top_k 生效。

- [x] Task 14: 修复 Docker 和 docker-run 默认配置
  - [x] SubTask 14.1: 修复 #142，确保容器内默认配置不连接容器自身 `localhost` 上的 Postgres/Qdrant/依赖服务。
  - [x] SubTask 14.2: 修复 #147，确保 Docker Compose 默认不读取不适合容器网络的宿主机 localhost 配置。
  - [x] SubTask 14.3: 更新 `configs`、`deployments`、`.env.example` 或文档，并增加可验证的配置测试或静态检查。

- [x] Task 15: 修复 optimizer 候选、取消和预算一致性
  - [x] SubTask 15.1: 修复 #155，确保 optimizer candidate clone 不保留旧 RAG pipeline，候选参数能够真实参与评估。
  - [x] SubTask 15.2: 修复 #146，确保优化取消状态不会被最后一个候选完成覆盖。
  - [x] SubTask 15.3: 修复 #157，确保成本预算在候选执行前后都检查，最终超支不会错误标记为 completed。
  - [x] SubTask 15.4: 增加 optimizer 单元测试，覆盖候选参数生效、取消优先级、预算超支终态。

- [x] Task 16: 修复 semantic cache optimizer candidate 隔离
  - [x] SubTask 16.1: 修复 #156，确保 optimizer candidate 评估时 semantic cache key 包含候选或评估上下文维度，避免候选之间复用不兼容结果。
  - [x] SubTask 16.2: 保留正常在线查询的 profile-scoped cache 行为，不引入不必要的缓存碎片。
  - [x] SubTask 16.3: 增加 RAG/optimizer/cache 测试，覆盖相同 query 在不同 candidate 配置下不会互相污染。

- [x] Task 17: 执行第二轮最终验证并关闭 issue
  - [x] SubTask 17.1: 运行聚焦测试覆盖 Task 13-16 的修复范围。
  - [x] SubTask 17.2: 运行 `CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./tests/contract -v` 或记录 toolchain 兼容处理。
  - [x] SubTask 17.3: 运行 `CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson GOTOOLCHAIN=local go test ./...` 或记录明确环境限制。
  - [x] SubTask 17.4: 对所有已实现 issue 写入关闭说明并关闭，复查开放 issue 列表。

- [x] Task 18: 提交远程分支、创建 PR 并完成合并
  - [x] SubTask 18.1: 创建专用分支，提交本轮代码、测试、文档和规格更新，commit message 引用覆盖的 issue。
  - [x] SubTask 18.2: 推送分支到 `origin`，创建指向 `main` 的 PR，PR body 包含 `Closes #N` 或关闭说明与验证命令。
  - [x] SubTask 18.3: 等待或检查必要 CI 状态，若失败则修复后更新 PR。
  - [x] SubTask 18.4: 合并 PR 到远程仓库，更新本地 `main`，确认 PR merged 且目标 issue 均关闭。

## 第二轮 Task Dependencies
- Task 11 必须先完成，作为第二轮 issue 分组、修复和关闭的依据。
- Task 12 可在 Task 11 后独立执行，且只关闭已验证完成或重复的 issue。
- Task 13、Task 14、Task 15、Task 16 可在 Task 11 后并行开发，但同一文件修改需协调合并。
- Task 17 依赖 Task 12 到 Task 16。
- Task 18 依赖 Task 17，且只有全部验证通过后才能 PR 和 merge。
