# 快速上手

本目录面向第一次运行 ORAG 的开发者，目标是在本机完成依赖启动、数据库迁移、API 服务启动和一轮端到端 smoke。

## 前置条件

| 依赖 | 要求 | 检查方式 |
| --- | --- | --- |
| macOS | 当前开发默认环境 | `sw_vers` |
| Go | `go.mod` 声明 Go 1.22 | `go version` |
| Docker Desktop | 需要支持 `docker compose` | `docker compose version` |
| make | 执行项目封装命令 | `make -v` |
| curl | 调用健康检查和 smoke 脚本 | `curl --version` |

当前项目的 Go 命令建议带上 `CGO_ENABLED=0` 和 `GOFLAGS=-tags=stdjson,gjson`。`Makefile` 已默认注入这些参数，用于规避 Mac amd64 + Go 1.22 下 Hertz/Sonic native 与本地 cgo 链接产物的问题。

## 5 分钟启动路径

从仓库根目录执行：

```bash
cp .env.example .env
make dev-up
make migrate
make run
```

`make run` 会以前台方式启动 API 服务，默认监听 `http://localhost:8080`。保持该终端运行，在另一个终端检查：

```bash
curl -fsS http://localhost:8080/healthz
curl -fsS http://localhost:8080/readyz
```

期望结果：

| 接口 | 成功含义 |
| --- | --- |
| `GET /healthz` | API 进程可响应。 |
| `GET /readyz` | PostgreSQL、Qdrant collection 和 Ark 配置状态可被服务识别。 |

## 本地依赖

`make dev-up` 只启动 PostgreSQL 和 Qdrant：

| 组件 | 地址 | 用途 |
| --- | --- | --- |
| PostgreSQL | `localhost:5432` | 元数据、FTS、数据集、评估结果和 trace。 |
| Qdrant HTTP | `localhost:6333` | Qdrant 自身 ready 检查。 |
| Qdrant gRPC | `localhost:6334` | ORAG 后端向量检索和语义缓存访问。 |

停止依赖：

```bash
make dev-down
```

如需删除本地开发卷：

```bash
DEV_DOWN_VOLUMES=1 scripts/dev-down.sh
```

## 无依赖调试

如果只排查 HTTP 层、认证或 API smoke，可以临时使用 memory 后端：

```bash
STORAGE_BACKEND=memory make run
```

注意：`STORAGE_BACKEND=memory` 不验证 PostgreSQL、Qdrant、FTS、向量检索或数据持久化链路，不作为生产配置。

## 示例入口

完整示例索引见 [`../../examples/README.md`](../../examples/README.md)。服务模式示例需要保持 API 服务运行，按顺序执行 `examples/curl/05_health_ready.sh`、`examples/curl/00_login.sh`、知识库、入库、查询、SSE、trace、评估和优化脚本；脚本默认使用 `BASE_URL=http://localhost:8080`、`ADMIN_USERNAME=admin`、`ADMIN_PASSWORD=admin` 和 `.orag-demo/` 状态目录，均可通过环境变量覆盖。

Go memory 示例用于无外部依赖体验入库、查询和 trace/response 元数据读取，不需要启动 PostgreSQL、Qdrant 或 Ark：

```bash
GOTOOLCHAIN=local CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go run ./examples/go/memory
```

## 下一步

| 想做什么 | 继续阅读 |
| --- | --- |
| 跑完整 API smoke 和示例 | `../../examples/README.md` |
| 查看 smoke 说明 | `api-smoke.md` |
| 理解 API 结构 | `../api/README.md` |
| 理解 RAG 执行链路 | `../architecture/rag-pipeline.md` |
| 排查启动失败 | `../operations/troubleshooting.md` |
