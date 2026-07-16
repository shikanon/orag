# Text RAG 可复现 Benchmark Run 设计

## 目标

将 `text-rag` 的 `benchmark` tier 从“可安装但没有运行时”的占位 Pack 变为一个可验证、可重复执行的实验产品。该阶段交付受控 Benchmark Pack、P0–P8 单变量运行、公开只读的复现证据、Console 工作流，以及不需要真实模型 Key 的 PostgreSQL/Qdrant/浏览器回归。

这不是 CRUD-RAG 全量公开发布。上游全量数据的许可确认、匿名 HTTPS、MIME/长度与 SHA-256 发布流水线仍是单独工作；本阶段只发布仓库内受控 fixture，用确定性 mock 验证产品契约。

## 范围与不变量

- 仅支持 `text-rag`，并且只接受 `quick` 或 `benchmark` tier 的已验证运行时 Manifest；视觉文档、视频和官方 Replay 不在本增量内。
- Benchmark Manifest 使用新的 `1.0.9/benchmark` fixture 目录，包含两份可再分发文本/JSON 语料、至少四条冻结评测项，以及已声明的 P1–P8 候选。它不替换 Quick fixture，也不修改生产 catalog 的公开 `1.0.0` URL。
- Quick 维持 `realtime`/Top-K 5；Benchmark 固定 `high_precision`/Top-K 8。它们的 tier、Manifest SHA-256、数据集、profile、Top-K、evaluator、模型环境和构建 revision 都进入比较输入，因此 Quick 与 Benchmark、不同 Pack 版本或不同运行环境绝不被标为可比。
- Benchmark 继续使用 evaluator v5 的显式隔离：基础 retrieval 是 hybrid；Pipeline、cache、Query Router、rewrite、HyDE、RAPTOR 与默认 graph ingest 关闭；P5/P6/P7/P8 只保留各自已声明的单变量例外；P0–P7 Context Pack 固定 5/6000，P8 固定 3/6000。
- 每个运行将不可变地记录 `pack_manifest_sha256`、`runtime_environment_sha256` 和 `build_revision`。`build_revision` 来自非秘密 `ORAG_BUILD_REVISION`，开发默认 `dev`；生产镜像必须传入不可变 git SHA 或镜像 digest。三个字段和既有 comparison/definition fingerprint 都是服务器生成，浏览器不能提交或覆盖。

## 架构与数据流

1. Clone 仍从 catalog 解析固定 tier 的 Manifest，并在下载、长度、MIME、对象 SHA-256 校验后写入私有输出。`supportsTextRuntime` 替代只允许 Quick 的门槛，使 `text-rag/benchmark` 与 Quick 一样创建项目知识库、数据集和候选根。
2. `ResourceInitializer` 从 Benchmark Manifest 建立项目私有根；根的 metadata 保留 template/version/tier，数据集 Kind 标注 `tutorial_benchmark`，避免将它误解为 Quick baseline。
3. `LiveRunService.runtimeDefinition` 从保存的 Manifest 与 app-owned `RuntimeEnvironment` 生成 definition。该 definition 计算三份复现证据和既有 fingerprint；运行启动时将它们与 P0/P1–P8 审计字段一起持久化。
4. comparison 只读取持久化 run：要求 P0 的直接血缘、同一 tier/Pack Manifest SHA、同一 Runtime Environment SHA/build revision、相同评测输入与既有单变量规则。无法证明时只返回两条 run，`comparable=false`，不推导质量、成本或延迟。
5. OpenAPI 在 `TutorialExperimentRun` 上声明只读复现字段。Console 在运行审计区显示 Pack tier、Manifest SHA、环境 SHA 和 build revision，并用“Benchmark Pack · 冻结复现输入”标识 benchmark 工作流。

## 持久化与兼容性

新增迁移为 `tutorial_experiment_runs` 增加三个非空文本列，历史记录默认空字符串。空值历史运行保持可读，但不能满足新的 Benchmark 可比性检查；不能通过补值让历史 Quick 运行伪装为可复现 Benchmark。

运行定义 fingerprint 继续覆盖现有解析、检索、Context Pack 与知识库坐标，同时添加 tier/Manifest/environment/build 维度。定义匹配、幂等重放和比较都采用同一字段集合，避免“可重放但不可审计”或“可比较但定义不同”的分裂状态。

## 受控复现路径

`make console-real-tutorial-benchmark-e2e` 启动临时 PostgreSQL、Qdrant、迁移、API、Console 和只读本地 catalog。它仅在临时目录把 `1.0.9/benchmark` 映射为嵌入 catalog 的 `text-rag/1.0.0/benchmark`，并把 Manifest version 改为 `1.0.0`；源 fixture、公开 Pack 和生产 catalog 均不改动。

该流程以 deterministic mock 模型和固定 `ORAG_BUILD_REVISION=benchmark-e2e-v1` 创建 Benchmark 项目，运行 P0 和 P8，验证：`high_precision`/Top-K 8、P0 索引复用、P0 `5/6000` 对 P8 `3/6000`、三份复现字段、直接 P0→P8 可比性，以及私有对象实际写入。它不会宣称公开基准质量，也不会验证外部 OSS ACL。

## 错误与安全

- 未声明运行时、非文本 template 或未知 tier 仍返回稳定 `tutorial_runtime_unavailable` / Manifest 失败，不创建伪造 Benchmark Run。
- 不同 Manifest/environment/build/tier、缺失 P0、非直接血缘或运行定义漂移都只得到不可比结果；候选启动所需的基线不满足时继续返回 `tutorial_baseline_required`。
- API、Console、日志和文档不泄露访问密钥、私有 bucket、对象 key、签名 URL 或原始 Manifest 下载位置。
- `ORAG_BUILD_REVISION` 是非秘密标识，禁止将 token、URL 凭证或任意运行配置写入该字段；长度受限并进行 trim，以保持指纹稳定。

## 验收

1. Manifest/catalog/clone/runtime-definition/comparison/storage 单元与迁移测试证明 Benchmark 可运行、Quick/Benchmark 不可混比、复现字段不能被客户端控制。
2. OpenAPI 契约、生成的 Console schema、Console 单测/类型检查/生产构建通过。
3. `go test ./...`、`make vet`、`make openapi-validate` 和新的真实 Benchmark 浏览器 E2E 通过。
4. 文档说明受控 Benchmark Pack 的复现命令、字段、边界与官方全量发布前置条件；ROADMAP 只移除已交付的 Benchmark Run，不将 Replay 或视觉/视频工作误标为完成。
