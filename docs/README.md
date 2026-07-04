# ORAG 文档中心

这里是 ORAG 的文档导航页。仓库根 README 负责快速理解项目定位和启动路径，本目录按“快速上手、API、架构、评估、运维”拆分为子目录，顶层长文继续保留为兼容入口和完整参考。

## 推荐阅读路径

| 场景 | 推荐顺序 | 目标 |
| --- | --- | --- |
| 第一次运行项目 | `../README.md` -> `getting-started/README.md` -> `getting-started/api-smoke.md` | 完成本地启动、登录、建库、入库、查询和评估 smoke。 |
| 接入 API 或编写 SDK | `api/README.md` -> `api/auth-and-errors.md` -> `api/ingestion-and-query.md` -> `../api/openapi.yaml` | 对齐认证、请求体、响应体、错误码和主业务 API。 |
| 理解 RAG 内部链路 | `architecture/README.md` -> `architecture/rag-pipeline.md` -> `Go-RAG-框架技术方案.md` | 理解 HTTP、检索、重排、生成、引用、缓存和存储边界。 |
| 做 RAG 质量回归 | `evaluation/README.md` -> `api/ingestion-and-query.md` | 理解数据集、评估运行、deterministic/Judge/QAG 指标和 optimizer 行为。 |
| 部署或排障 | `operations/README.md` -> `operations/troubleshooting.md` -> `development.md` | 明确依赖、配置、健康检查、metrics 和常见故障处理。 |

## 分层文档地图

| 目录 | 主题 | 覆盖内容 |
| --- | --- | --- |
| `getting-started/` | 快速上手 | 本地启动、依赖说明、API smoke、状态目录和常见 smoke 失败。 |
| `api/` | API 集成 | 认证、错误模型、知识库、入库任务、JSON 查询和 SSE 查询。 |
| `architecture/` | 架构设计 | 模块地图、运行时依赖、RAG pipeline 和排查切入点。 |
| `evaluation/` | 评估与优化 | 数据集、评估运行、rule-based metrics、LLM-as-Judge/QAG 和目标驱动 optimizer。 |
| `operations/` | 运维排障 | 健康检查、metrics、部署检查清单、Docker 配置和故障排查。 |

## 兼容长文

| 文档 | 主题 | 说明 |
| --- | --- | --- |
| `api.md` | HTTP API 完整长文 | 保留所有已实现 API 的集中式示例。 |
| `development.md` | 本地开发完整长文 | 保留环境准备、测试矩阵、集成测试和 live Ark 测试细节。 |
| `evaluation.md` | 评估完整长文 | 保留数据集结构、指标和 optimizer 说明。 |
| `operations.md` | 运维完整长文 | 保留部署依赖、配置安全、metrics 和故障排查全集。 |
| `Go-RAG-框架技术方案.md` | 技术方案 | 保留整体 Go RAG 框架设计背景。 |

## 常用入口

| 入口 | 命令或路径 |
| --- | --- |
| 本地启动依赖 | `make dev-up` |
| 执行迁移 | `make migrate` |
| 启动 API | `make run` |
| 健康检查 | `curl -fsS http://localhost:8080/healthz` |
| 就绪检查 | `curl -fsS http://localhost:8080/readyz` |
| OpenAPI 源文件 | `../api/openapi.yaml` |
| 内置文档页 | `GET /docs` |
| curl smoke | `../examples/curl/00_login.sh` -> `../examples/curl/40_eval.sh` |
| 契约测试 | `make openapi-validate` |

## 文档维护规则

- API 行为变更时，同步更新 `api/`、`api.md`、`../api/openapi.yaml` 和 `../tests/contract`。
- 本地启动、测试命令或依赖端口变更时，同步更新 `getting-started/`、`development.md` 和 README 的快速开始。
- 部署变量、健康检查或 metrics 变更时，同步更新 `operations/` 和 `operations.md`。
- 评估指标、optimizer 策略或数据集结构变更时，同步更新 `evaluation/` 和 `evaluation.md`。
- 架构链路、模块边界或存储实现变更时，同步更新 `architecture/` 和 `Go-RAG-框架技术方案.md`。
- 文档示例默认保持无真实 Ark Key 可跑；需要真实模型的路径必须明确标注 `LIVE_ARK_TESTS=1`、`ARK_API_KEY` 和跳过条件。
