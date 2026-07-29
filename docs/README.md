# ORAG 文档中心

这里是 ORAG 的文档导航页。仓库根 README 负责快速理解项目定位和启动路径，本目录按“快速上手、API、架构、评估、运维”拆分为子目录，顶层长文继续保留为兼容入口和完整参考。

对外托管入口为 [`https://www.tensorbytes.com/orag/`](https://www.tensorbytes.com/orag/)；[`https://shikanon.github.io/orag/`](https://shikanon.github.io/orag/) 保留为镜像。两者的 API Reference 都直接使用仓库 `api/openapi.yaml` 构建，并支持授权、筛选与 Try it out。本地启动 API 后，等价入口为 [`http://localhost:8080/docs`](http://localhost:8080/docs)，原始规范位于 [`http://localhost:8080/openapi.yaml`](http://localhost:8080/openapi.yaml)。

## 推荐阅读路径

| 场景 | 推荐顺序 | 目标 |
| --- | --- | --- |
| 第一次运行项目 | [`../README.md`](../README.md) -> [`getting-started/README.md`](./getting-started/README.md) -> [`getting-started/api-smoke.md`](./getting-started/api-smoke.md) | 完成本地启动、健康检查、登录、建库、入库、查询、trace、评估和优化 smoke。 |
| 嵌入公共 Go SDK | [`sdk/README.md`](./sdk/README.md) -> [`../examples/go/sdk`](../examples/go/sdk) -> [`compatibility.md`](./compatibility.md) | 使用无需 Key 的 mock 流程，理解显式配置、错误、并发、流式语义和 Beta 边界。 |
| 接入 HTTP API 或 MCP | [`api/README.md`](./api/README.md) -> [`api/auth-and-errors.md`](./api/auth-and-errors.md) -> [`api/ingestion-and-query.md`](./api/ingestion-and-query.md) -> [`api/agent-integrations.md`](./api/agent-integrations.md) -> [`../api/openapi.yaml`](../api/openapi.yaml) | 对齐认证、请求体、响应体、错误码、主业务 API、Ralph Loop MCP 和 Skill 集成。 |
| 理解 RAG 内部链路 | [`architecture/README.md`](./architecture/README.md) -> [`architecture/rag-pipeline.md`](./architecture/rag-pipeline.md) -> [`Go-RAG-框架技术方案.md`](./Go-RAG-框架技术方案.md) | 理解 HTTP、检索、重排、生成、引用、缓存和存储边界。 |
| 做 RAG 质量回归 | [`evaluation/README.md`](./evaluation/README.md) -> [`api/ingestion-and-query.md`](./api/ingestion-and-query.md) | 理解数据集、评估运行、deterministic/Judge/QAG 指标和 optimizer 行为。 |
| 追溯数据集与方法来源 | [`research-references.md`](./research-references.md) -> [`architecture/rag-pipeline.md`](./architecture/rag-pipeline.md) -> [`evaluation/README.md`](./evaluation/README.md) | 查看 CRUD-RAG、ViDoSeek、Video-MME 及 RAG、RRF、HyDE、RAPTOR、GraphRAG、Judge/QAG 等方法的原始来源和实现边界。 |
| 学习工程模块效果 | 控制台 `/tutorials` -> [`tutorials/clone-and-pack-install.md`](./tutorials/clone-and-pack-install.md) -> [`tutorials/text-rag-benchmark-run.md`](./tutorials/text-rag-benchmark-run.md) -> [`benchmarks/performance-baseline-contract.md`](./benchmarks/performance-baseline-contract.md) -> [`tutorials/text-rag-official-replay.md`](./tutorials/text-rag-official-replay.md) | `text-rag` Quick/Benchmark 可在冻结输入下运行 P0–P8；性能报告只有固定环境、build 与负载后才可比较；`video-rag` 支持所有者授权的私有导入、时序证据与 temporal P0。官方 text-rag Replay 是固定环境的只读快照；视觉 Live Run/Replay 与视频公开 Replay 尚未发布。 |
| 判断能力兼容性 | [`compatibility.md`](./compatibility.md) -> [`../ROADMAP.md`](../ROADMAP.md) | 区分 `experimental`、`beta`、`stable`，理解 pre-1.0 弃用和迁移规则。 |
| 部署或排障 | [`operations/README.md`](./operations/README.md) -> [`operations/troubleshooting.md`](./operations/troubleshooting.md) -> [`development.md`](./development.md) | 明确依赖、配置、健康检查、metrics 和常见故障处理。 |

## 分层文档地图

| 目录 | 主题 | 覆盖内容 |
| --- | --- | --- |
| [`getting-started/`](./getting-started) | 快速上手 | 本地启动、依赖说明、API smoke、状态目录和常见 smoke 失败。 |
| [`api/`](./api) | API 集成 | 认证、错误模型、知识库、入库任务、JSON/SSE 查询、trace 查询、Ralph Loop MCP 和 Skill 集成。 |
| [`sdk/`](./sdk) | Go SDK | 嵌入式客户端、无 Key 示例、生产配置、类型化错误与事件流。 |
| [`extensions.md`](./extensions.md) | 扩展契约 | Parser、Chunker、Embedder、Retriever、Reranker、Generator、Storage 的公共 Beta 契约与合规套件。 |
| [`architecture/`](./architecture) | 架构设计 | 模块地图、运行时依赖、RAG pipeline 和排查切入点。 |
| [`evaluation/`](./evaluation) | 评估与优化 | 数据集、评估运行、rule-based metrics、LLM-as-Judge/QAG 和目标驱动 optimizer。 |
| [`research-references.md`](./research-references.md) | 研究依据 | 数据集、RAG/检索方法、评估方法的论文链接，以及研究启发与严格复现的边界。 |
| [`operations/`](./operations) | 运维排障 | 健康检查、metrics、部署检查清单、Docker 配置和故障排查。 |

## 兼容长文

| 文档 | 主题 | 说明 |
| --- | --- | --- |
| [`api.md`](./api.md) | HTTP API 完整长文 | 保留所有已实现 API 的集中式示例。 |
| [`development.md`](./development.md) | 本地开发完整长文 | 保留环境准备、测试矩阵、集成测试和 live Ark 测试细节。 |
| [`evaluation.md`](./evaluation.md) | 评估完整长文 | 保留数据集结构、指标和 optimizer 说明。 |
| [`operations.md`](./operations.md) | 运维完整长文 | 保留部署依赖、配置安全、metrics 和故障排查全集。 |
| [`Go-RAG-框架技术方案.md`](./Go-RAG-框架技术方案.md) | 技术方案 | 保留整体 Go RAG 框架设计背景。 |
| [`compatibility.md`](./compatibility.md) | 兼容性与成熟度 | 定义公共能力的成熟度、弃用和 pre-1.0 迁移规则。 |

## 常用入口

| 入口 | 命令或路径 |
| --- | --- |
| 本地启动依赖 | `make dev-up` |
| 执行迁移 | `make migrate` |
| 启动 API | `make run` |
| 健康检查 | `curl -fsS http://localhost:8080/healthz` |
| 就绪检查 | `curl -fsS http://localhost:8080/readyz` |
| OpenAPI 源文件 | [`../api/openapi.yaml`](../api/openapi.yaml) |
| 托管文档站 | [`https://www.tensorbytes.com/orag/`](https://www.tensorbytes.com/orag/)；GitHub Pages 镜像 [`https://shikanon.github.io/orag/`](https://shikanon.github.io/orag/) |
| 交互式 API Reference | `GET /docs`（本地）或 [`https://www.tensorbytes.com/orag/api.html`](https://www.tensorbytes.com/orag/api.html)（托管）/ [`https://shikanon.github.io/orag/api.html`](https://shikanon.github.io/orag/api.html)（镜像） |
| 运行时 OpenAPI | `GET /openapi.yaml` |
| 教程实验室 | 控制台 `/tutorials`；目录与克隆/任务接口见 [`tutorials/clone-and-pack-install.md`](./tutorials/clone-and-pack-install.md) |
| curl smoke | [`../examples/curl/05_health_ready.sh`](../examples/curl/05_health_ready.sh) -> [`../examples/curl/50_optimize.sh`](../examples/curl/50_optimize.sh) |
| Ralph Loop MCP/Skill | [`api/agent-integrations.md`](./api/agent-integrations.md) -> [`../examples/mcp/README.md`](../examples/mcp/README.md) -> [`../examples/skills/README.md`](../examples/skills/README.md) |
| 契约测试 | `make openapi-validate` |

## 文档维护规则

- API 行为变更时，同步更新 [`api/`](./api)、[`api.md`](./api.md)、[`../api/openapi.yaml`](../api/openapi.yaml) 和 [`../tests/contract`](../tests/contract)。
- Ralph Loop 能力清单变更时，先更新 [`../api/openapi.yaml`](../api/openapi.yaml)，再运行 `make agent-sync`，并同步检查 [`api/agent-integrations.md`](./api/agent-integrations.md) 与 [`../examples/mcp`](../examples/mcp)。
- 本地启动、测试命令或依赖端口变更时，同步更新 [`getting-started/`](./getting-started)、[`development.md`](./development.md) 和 README 的快速开始。
- 部署变量、健康检查或 metrics 变更时，同步更新 [`operations/`](./operations) 和 [`operations.md`](./operations.md)。
- 评估指标、optimizer 策略或数据集结构变更时，同步更新 [`evaluation/`](./evaluation) 和 [`evaluation.md`](./evaluation.md)。
- 架构链路、模块边界或存储实现变更时，同步更新 [`architecture/`](./architecture) 和 [`Go-RAG-框架技术方案.md`](./Go-RAG-框架技术方案.md)。
- 文档示例默认保持无真实 Ark Key 可跑；需要真实模型的路径必须明确标注 `LIVE_ARK_TESTS=1`、`ARK_API_KEY` 和跳过条件。
