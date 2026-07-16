# ORAG Open Source Roadmap

[English](./ROADMAP_EN.md) | 简体中文

最后更新：2026-07-16

## 定位

ORAG 是面向 Go 与后端平台团队的 Go-native RAG 服务与控制面。项目以评测优先为核心原则，把知识入库、混合检索、生成、可观测、离线评测、参数优化和受控发布放在同一条可复现链路中。

ORAG 不以“支持最多模型或最多页面”为目标。项目优先解决三个问题：

1. 让 Go 团队能够以服务或公共 Go SDK 的方式可靠接入 RAG。
2. 让每次检索、生成和配置变更都能被评测、追踪和复现。
3. 让通过评测的版本才能进入预发和生产，并且可以审计和回滚。

## 路线图原则

- **可靠性先于功能数量**：数据一致性、租户隔离、幂等和恢复问题优先于新增 provider、页面或检索策略。
- **评测与线上同链路**：评测、优化和发布门禁复用真实查询路径，避免单独维护演示链路。
- **API 与 SDK 同源**：HTTP 服务和公共 Go SDK 复用应用层，不复制业务规则。
- **开箱可验证**：没有真实模型 Key 的用户也能通过显式 mock 模式完成完整 walkthrough；mock 不得被生产配置隐式启用。
- **诚实标注成熟度**：实验能力不能以稳定能力的方式宣传；成熟度变化必须经过公开退出门槛。
- **社区可参与**：公开决策、可复现问题、可审查变更和清晰的贡献入口与代码同等重要。

## 能力成熟度

所有用户可见能力将在 README、文档、OpenAPI 扩展字段和 capability manifest 中使用以下状态：

| 状态 | 含义 | 兼容性承诺 |
| --- | --- | --- |
| `experimental` | 正在验证问题与接口，可能不完整 | 可以在 minor 版本中调整或移除，但必须写入 release notes |
| `beta` | 主要流程完整，适合真实试用和受控生产试点 | 避免无迁移路径的破坏性变更；破坏性变化必须提供说明和迁移指南 |
| `stable` | 有稳定接口、升级路径、运维文档和生产采用证据 | 遵循 SemVer；破坏性变化只进入 major 版本 |
| `planned` | 已进入公开路线图但尚未形成可依赖契约 | 不应被文档或 UI 表述为可用能力 |

当前基线：

| 能力域 | 当前状态 | 晋级条件 |
| --- | --- | --- |
| HTTP API、知识库、入库、JSON/SSE 查询 | `beta` | 数据一致性问题关闭，完成版本化契约与生产试点 |
| PostgreSQL + Qdrant 混合检索、RRF、rerank、语义缓存 | `beta` | 完成负载、故障恢复和兼容矩阵验证 |
| 数据集、评测运行、LLM-as-Judge、optimizer | `beta` | 公开可复现 benchmark，完成预算与并发保护 |
| 应用内 trace、Prometheus 指标、ready/health | `beta` | 完成指标持久化/采样策略与跨服务拓扑 |
| Contextual Retrieval、RAPTOR、Query Router、Graph Retrieval | `experimental` | 每项具备独立消融结果、成本说明和回退行为 |
| Offline Knowledge、MCP 自检/诊断/自运维 | `experimental` | 消除 fixture/占位依赖，完成安全边界与人工批准审计 |
| ORAG Console | `experimental` | 完成编排、API 调试、评测门禁和发布回滚黄金路径 |
| 教程实验室 | `experimental` | 支持 clone、Quick Run、Replay 和结果对比 |
| 公共 Go SDK | `beta` | 已在 `v0.1.0-beta.2` 发布；以外部 consumer gate 和兼容政策持续验证 |
| GHCR 镜像、完整 Compose、托管文档站 | `beta` | 已在 `v0.1.0-beta.2` 发布；持续验证签名、walkthrough 和文档契约 |

在 `v1.0.0` 之前，ORAG 不把任何能力标记为 `stable`。

## 推进阶段

各阶段按质量门禁和实际资源推进，不绑定目标日期。阶段之间可以重叠；满足退出条件后即可进入下一阶段。

| 阶段 | 结果 |
| --- | --- |
| 阶段一：可信开源基线 | 建立社区治理、安全入口、成熟度标注和受保护的主分支 |
| 阶段二：发布 `v0.1.0-beta.1` | 发布可下载、可运行、可嵌入、无需真实 Key 即可体验的首个 Beta |
| 阶段三：生产试点基线 | 完成一致性、安全、可观测和 CI/CD 加固，支持参考部署 |
| 阶段四：评测优先控制面 | 完成编排、评测门禁、发布回滚和教程实验闭环 |
| 阶段五：生态与 `v1.0` 准备 | 稳定扩展点、治理和兼容政策，积累生产与社区证据 |

## 阶段一：可信开源基线

### 社区与治理

- 创建 `CONTRIBUTING.md`，覆盖开发环境、测试矩阵、提交规范、PR 流程、文档同步规则和首个贡献路径。
- 创建 `SECURITY.md`，声明支持版本、私密漏洞报告方式、响应时限和披露流程，并启用 GitHub Private Vulnerability Reporting。
- 采用 Contributor Covenant 2.1 作为 `CODE_OF_CONDUCT.md`，明确执行责任和举报渠道。
- 创建结构化 Issue 模板：Bug、Feature、Documentation、RFC；创建 PR 模板，要求说明测试、文档、安全、兼容性和成熟度变化。
- 建立 `good first issue`、`help wanted`、`area/*`、`maturity/*`、`priority/*` 标签和公开 triage 规则。
- 开启 GitHub Discussions，至少配置 Announcements、Q&A、Ideas 和 Show and tell 分类。
- 配置仓库 Topics：`rag`、`retrieval-augmented-generation`、`golang`、`llm-evaluation`、`qdrant`、`postgresql`、`openapi`、`mcp`、`eino`、`hertz`。
- 保护 `main`：禁止 force push 和删除，要求分支为最新、要求 CI checks、要求解决 review conversation。第二位 maintainer 加入前不强制一名外部 reviewer，避免单维护者仓库被锁死。
- 启用 Dependabot，按周检查 Go modules、npm、GitHub Actions 和 Docker 依赖；安全更新单独即时处理。

### 成熟度与发布纪律

- 在 OpenAPI operation 和 schema 上增加统一的 `x-orag-maturity`，只接受 `experimental`、`beta`、`stable`。
- 在 capability manifest 中复用同一成熟度枚举，并增加契约测试防止 README、OpenAPI 和生成产物漂移。
- 建立 SemVer、弃用、迁移和 release notes 规则；实验能力的变更也必须进入 changelog。
- 建立 `CHANGELOG.md` 和公开 roadmap 更新流程。每次 minor release 更新能力矩阵，并在优先级或项目状态变化时审阅本文件。

### 阶段退出门槛

- GitHub community profile 达到 90% 或以上。
- `main` 保护和必需 checks 生效，Dependabot 能创建验证 PR。
- 所有当前开放的数据一致性与并发问题被修复，或有 owner、优先级、目标版本和可验证临时缓解措施。
- README、OpenAPI 和 capability manifest 对成熟度的表达一致并由 CI 检查。

## 阶段二：发布 `v0.1.0-beta.1`

### 可复现发行物

- 建立 tag 驱动的 release workflow；只有 `v*` tag 生成 GitHub Release 和 GHCR 镜像，普通 `main` push 只运行 CI。
- 发布 `linux/amd64` 和 `linux/arm64` 镜像，至少包括 `orag-api` 与 `orag-console`。
- 为镜像生成 SBOM、provenance 和校验信息，并使用 keyless signing；GitHub Release 记录镜像 digest、变更、迁移和已知限制。
- `orag-api --version`、`oragctl version` 和运行时版本端点返回同一版本、commit 和 build time。

### 一条命令的完整体验

- Compose 完整栈包含 PostgreSQL、Qdrant、一次性 migration、API、Console 和 demo/walkthrough。
- `docker compose --profile demo up --wait` 使用显式 deterministic mock 配置，不要求真实模型 Key，并自动生成可查询的示例知识库和评测数据。
- 默认生产 profile 不得继承 mock provider 或弱口令；demo 数据与生产 volume、凭据和端口策略明确隔离。
- walkthrough 覆盖登录、入库、查询与引用、trace、评测和一次参数对比，并能在 Console 中查看结果。
- 在干净的 macOS、Linux amd64 和 Linux arm64 环境验证完整流程。

### 交互文档与托管站点

- 将 `/docs` 替换为基于仓库 OpenAPI 的交互式 API UI，支持认证、请求示例和 SSE 使用说明。
- 建设托管文档站，优先通过 GitHub Pages 发布；README、站点和内置 `/docs` 均从同一 OpenAPI 与示例来源生成或校验。
- 提供首页架构图、真实 Console 截图、五分钟 walkthrough GIF、部署指南、SDK 指南和能力成熟度页面。
- 文档构建、内部链接、代码片段和 OpenAPI 覆盖进入 CI。

### 真正的公共 Go SDK

- 在模块根包提供 `github.com/shikanon/orag` 公共 facade；公共签名不得泄漏 `internal/*` 类型。
- HTTP 服务与 SDK 共用应用装配、入库、查询、评测和 trace 服务，HTTP 层只负责协议适配。
- 首个 Beta SDK 覆盖：客户端创建与关闭、知识库管理、文本/文件入库、同步查询、流式查询、评测提交与状态、trace 查询。
- 同时支持显式内存/mock 配置和 PostgreSQL + Qdrant 真实配置；测试辅助模型必须清楚标注，不得伪装成生产 provider。
- 使用仓库外部测试包和独立 consumer module 证明用户无需导入 `internal/*`；提供 pkg.go.dev 文档、可运行示例和兼容性说明。
- SDK 错误使用 `errors.Is`/`errors.As` 可判定的稳定类别，并保留 trace ID、可重试性和底层 cause。

### 阶段退出门槛

- `v0.1.0-beta.1` tag、GitHub Release、双架构 GHCR 镜像、SBOM 和签名均可公开验证。
- 新用户从 clone 到第一次带引用回答的中位时间低于 10 分钟；至少 10 名非维护者完成测试，成功率不低于 90%。
- 公共 SDK 能被独立 Go module 引用，核心示例、race test、API 文档和升级检查通过。
- Console、交互 `/docs`、托管文档站和 mock walkthrough 使用同一版本契约。

## 阶段三：生产试点基线

当前进展：[#175](https://github.com/shikanon/orag/issues/175) 已按[跨存储 staged visibility 设计](./docs/superpowers/specs/2026-07-15-qdrant-staged-visibility-design.md)实现。PostgreSQL 现在统一授权 sparse/dense 可见性，失败候选不会进入检索，并已通过真实 PostgreSQL + Qdrant 的失败、替换、legacy、清理告警和并发测试。[#177](https://github.com/shikanon/orag/issues/177) 也已按[可重试知识库删除设计](./docs/superpowers/specs/2026-07-15-kb-delete-retry-design.md)实现：外部索引清理失败时保留 metadata 作为持久重试入口，真实存储测试证明重复 DELETE 可完成清理。[#176](https://github.com/shikanon/orag/issues/176) 按[optimizer single-flight 状态迁移设计](./docs/superpowers/specs/2026-07-15-optimizer-singleflight-design.md)完成跨实例 CAS：并发 resume/run/candidate 领取只有一个 winner，loser 得到可诊断的 `409`，并由真实 PostgreSQL 并发测试覆盖。可观测性方面，[#166](https://github.com/shikanon/orag/issues/166) 已通过现有真实 TraceGetter/MCP HTTP wiring 和 found/not-found/unavailable 测试核验后关闭；[#163](https://github.com/shikanon/orag/issues/163) 已确认 graph span 在持久化前带有连续 sequence 与真实 UTC 执行窗口，并补充专门回归测试。安全方面，[#211](https://github.com/shikanon/orag/issues/211) 已升级 27 个 Dependabot 告警涉及的六个 Go 依赖族，并通过公共 SDK、OpenAPI、race 及真实 PostgreSQL + Qdrant 集成门禁；[#213](https://github.com/shikanon/orag/issues/213) 增加 SHA 固定的 CodeQL、`govulncheck`、npm audit、全历史 secret、双镜像 Trivy 和独立 Scorecard workflow，并把 Go 运行时升级到修复 GO-2026-5856 的 1.26.5；[#215](https://github.com/shikanon/orag/issues/215) 对首次 CodeQL 发现的 Fake Ark 不受控向量分配增加显式上限与回归测试；[#217](https://github.com/shikanon/orag/issues/217) 将全部 workflow action 和基础镜像固定到不可变 SHA/digest，并把常规 CI token 收紧为只读。上述进展只完成下方一致性、安全与可观测条目的一部分，不代表阶段三已完成。

持续安全探索方面，[#219](https://github.com/shikanon/orag/issues/219) 为文档/Office 解析和 optimizer 表达式增加 Go 原生 fuzz target、PR smoke 与周期性长时探索。

依赖边界方面，[#221](https://github.com/shikanon/orag/issues/221) 将 Eino 升级到包含 Jinja 文件访问修复的 0.9.12，增加 `file`/`fileset` 回归，并记录 GO-2026-5932 不可达且无上游修复版本的证据。

### 数据一致性与执行安全

- 入库使用 staged/active 可见性或等价事务协议，失败文档和向量不能提前进入检索结果。
- 知识库删除、上传恢复和 optimizer resume 具备幂等、并发保护、补偿和可重试状态。
- 增加数据库迁移完整性检查、Qdrant collection 兼容检查、备份恢复演练和灾难恢复文档。
- 已为 ingestion、query、evaluation 和 release 写入口定义独立的 fail-fast 并发预算与 deadline，满载返回可退避的 `429`、超时返回 `504` 并向下游传播取消；待在多副本试点中以真实容量和 provider 配额校准全局限流与重试策略。

### 安全与租户边界

- 增加面向机器调用的 API Key、最小 RBAC 和项目级授权；默认管理员账号只用于 bootstrap。
- 对 secret 注入、轮换、日志脱敏、prompt/文档记录和多租户查询进行威胁建模与测试。
- CI 增加 CodeQL、`govulncheck`、npm audit、secret scanning、容器扫描和 OpenSSF Scorecard。

### 可观测和质量门禁

- 已接入可选 OpenTelemetry trace/metrics exporter，并提供可导入的 Prometheus/Grafana 资源和基础告警规则；待完成指标持久化、采样与跨服务拓扑。
- CI 覆盖 Go 单测/vet/race、OpenAPI、Console typecheck/unit/build/E2E、PostgreSQL + Qdrant 集成测试和双架构镜像 smoke。
- 已发布可验证的性能基线报告契约：固定 Benchmark Pack、mock 运行、环境/构建/负载指纹与入库吞吐、查询 p50/p95、缓存命中、评测耗时、模型调用和成本口径；待用已披露硬件与 provider 条件的 runner 生成可比较公开结果。

### 阶段退出门槛

- 连续 30 天生产试点无未缓解 P0，已知 P1 有 owner 和目标版本。
- 至少两个独立参考部署完成升级、备份恢复和回滚演练。
- 安全、集成、Console 和 release checks 成为 `main` 必需门禁。

## 阶段四：评测优先控制面

当前进展：教程实验室已支持持久化、幂等、可恢复的模板 clone 与 Pack 安装，以及 `text-rag` Quick/Benchmark 的 P0–P8 单变量运行。受控 Benchmark Pack 固定 `high_precision`/Top-K 8，持久化 Manifest SHA-256、运行环境 SHA-256、构建版本、评测输入和 evaluator v5 指纹；比较只接受直接 P0 血缘和完全一致的复现证据。`text-rag` 官方 Replay 已作为内嵌、只读、离线快照发布，包含版本化 Pack/环境/构建证据和 P0/P8 审计事实，不代表用户 Live Run。`text-rag/1.1.0` 已从锁定的 CRUD-RAG 提交构建并公开发布到匿名 HTTPS：Quick、Benchmark、完整 `data/` 归档、`SOURCE.json` 和 `SHA256SUMS` 均已逐项下载验证。`visual-document-rag` 已支持私有 Recipe PDF 资产、`visual_page` Live Run 与评测；`video-rag` 已支持所有者授权的私有媒体导入、固定时序证据、项目内评测集冻结与 temporal P0。视觉与视频公开 Replay 仍待有可合法公开、可审计的聚合结果后发布。

最新进展：Pack 声明的 P2 `p2_recursive_400_80`、P3 `p3_contextual_retrieval`、P4 `p4_sparse_retrieval`、P5 `p5_multi_query_retrieval`、P6 `p6_rerank_retrieval`、P7 `p7_graph_retrieval` 与 P8 `p8_context_pack` 均为 P0 的直接子实验。P7 保持 P0 的 Basic/800/120 形状、评测集、profile 与 Top-K，但使用独立知识库、固定轻量图关系构建和 GraphRetriever；P8 复用 P0 索引，只改变 Context Pack 的 Top-N（5→3）。教程 evaluator v5 对 P0–P6 强制 explicit hybrid 并关闭 graph/RAPTOR 入库，使 P7 成为唯一图检索变量；P6 仍是唯一 rerank 变量，P8 仍是唯一 Context Pack 变量。运行持久化上下文化、索引复用、查询扩展、rerank、图检索与 Context Pack 事实，比较将 `index_metrics` 与普通评测指标严格分开。受控 `1.0.8` fixture 与真实 PostgreSQL/Qdrant/浏览器 E2E 证明 P0→P8 的隔离、P0 索引复用与 Context Pack 审计；官方公开 `1.0.8` Pack 仍需独立匿名 HTTPS、MIME、长度和 SHA-256 发布流水线。

### Project 到 Release 的黄金路径

- 完成项目级 RAG Studio、受约束 DAG、API Debugger 和不可变 pipeline version。
- 完成项目级数据集、冻结评测运行、硬指标门禁和候选版本对比。
- 完成开发到预发到生产的顺序晋级、不可绕过门禁、乐观并发、追加式审计和原子回滚。
- 生产查询解析到明确的 active version；trace 记录 pipeline、模型、检索参数、数据集和 release lineage。

### 教程实验闭环

- 已完成官方教程的 clone 与 Pack 安装、Quick P0–P8 候选、受控 `text-rag` Benchmark Run（冻结 `high_precision`/Top-K 8 输入、真实评测指标、P0→P8 复现、Manifest/环境 SHA-256 与构建版本审计），以及离线、只读的官方 text-rag Replay。官方 `text-rag/1.1.0` Pack 已公开并通过匿名 SHA-256 验证；待完成视觉/视频 Replay 与执行。
- 文本、视觉文档和视频教程均使用真实工程/evaluation 数据，不引入模型训练工作流。
- 每个检索增强策略提供独立消融、成本、延迟、失败回退和推荐场景说明。

### 阶段退出门槛

- 从创建项目到发布和回滚的浏览器 E2E 在真实 PostgreSQL + Qdrant 环境通过。
- 至少两个公开 benchmark 可由 tag 对应的镜像、配置和数据集完整复现。
- 至少五个外部团队持续使用，三个外部 PR 合并，两个生产案例可公开引用。

## 阶段五：生态与 `v1.0` 准备

### 稳定扩展点

- 为 parser、chunker、embedding、retriever、reranker、model provider 和 storage adapter 定义最小稳定接口及合规测试套件。
- 只根据真实用户需求扩大集成面；官方支持矩阵区分 certified、community 和 experimental。
- 发布 SDK/API 兼容政策、弃用周期、升级工具和长期支持范围。

### 社区治理与传播

- 建立 RFC 流程、maintainer/committer 角色、决策记录和安全响应轮值。
- 维持可预测、质量门禁驱动的 release 流程和公开 changelog，并根据项目状态及时审阅 roadmap。
- 通过真实 benchmark、架构文章、教程、公开案例、会议分享和社区 demo 传播，而不是依赖功能列表或 star campaign。
- 当出现稳定 Kubernetes 需求后再发布 Helm chart 和云参考架构；在此之前保持 Docker/Compose 为主路径。

### `v1.0` 退出门槛

- 至少 10 个可确认的生产部署，包含升级和恢复证据；至少两个公开案例。
- 至少 20 名外部贡献者和 3 名能够独立 review/release 的 maintainer。
- 核心 API 与 Go SDK 完成兼容审计，连续两个 minor release 没有无迁移路径的破坏性变化。
- 安全响应、依赖更新、release、备份恢复、容量和故障处置均有演练记录。

## 项目指标

路线图优先衡量采用和信任，star 只作为滞后品牌指标。

| 维度 | 指标 |
| --- | --- |
| 激活 | 首次成功时间、walkthrough 成功率、文档到运行的退出率 |
| 可靠性 | P0/P1 数量、入库失败恢复率、发布失败率、回滚时间、SLO 达成率 |
| 采用 | 外部活跃部署、30/90 日留存、生产升级数量、公开案例 |
| 社区 | 外部贡献者、首次响应时间、PR 合并周期、可独立发布 maintainer 数量 |
| 质量 | benchmark 可复现率、契约兼容性、覆盖率、漏洞修复时长 |
| 影响力 | 文档月活、自然搜索和引用、技术内容采用、GitHub star/fork |

## 明确非目标

- 不建设模型训练平台。
- 不在可靠性、发布和评测闭环完成前复制大而全的通用 AI 应用平台。
- 不以 provider 数量作为主要竞争指标；没有合规测试和维护者的 provider 不进入 certified 列表。
- 不在 `v1.0` 前承诺长期稳定接口。
- 不让自动修复或 Agent 自运维绕过人工批准、审计和回滚边界。

## 参与路线图

- Bug 和明确需求使用 GitHub Issues。
- 跨模块接口、兼容性或治理变化使用 RFC Issue，并在实现前进入 Discussions。
- 每个阶段拆为独立、可测试、可审查的 implementation plan；路线图本身不替代工程设计。
- Maintainer 根据生产反馈、社区需求、项目状态和维护能力及时更新阶段状态与优先级。
- 路线图变化应通过 PR 完成，并说明变化原因、影响的阶段和指标。
