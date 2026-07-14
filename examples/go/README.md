# Go 调用示例

本目录同时演示 ORAG 公共嵌入式 Go SDK 和 HTTP/OpenAPI 调用。两种方式都不导入仓库内的 `internal/` 包。

## 示例列表

| 示例 | 覆盖能力 |
| --- | --- |
| `sdk/` | 公共 `github.com/shikanon/orag` SDK；无需真实 Key 完成入库、查询和评测。 |
| `basic/` | 登录、创建知识库、导入文本、轮询 ingestion job、发起 RAG 查询。 |

## 运行公共 SDK 示例

无需启动 API、数据库或向量库：

```bash
go run ./examples/go/sdk
```

生产配置、错误和 Beta 边界见 [`../../docs/sdk/README.md`](../../docs/sdk/README.md)。

## 运行前置

先启动 ORAG API 服务，并确保本地账号可登录：

```bash
STORAGE_BACKEND=memory ALLOW_DETERMINISTIC_MOCK=true make run
```

默认示例请求 `http://localhost:8080`，登录账号为 `.env.example` 中的 `admin` / `admin`。

## 运行 basic 示例

```bash
go run ./examples/go/basic
```

常用环境变量：

| 变量 | 默认值 | 用途 |
| --- | --- | --- |
| `ORAG_BASE_URL` | `http://localhost:8080` | API 服务地址。 |
| `ORAG_USERNAME` | `admin` | 登录用户名。 |
| `ORAG_PASSWORD` | `admin` | 登录密码。 |
| `ORAG_KB_NAME` | `Go SDK Example KB` | 示例知识库名称。 |
| `ORAG_DOC_NAME` | `orag-go-sdk-example.md` | 示例文档名称。 |
| `ORAG_DOC_SOURCE_URI` | `example://go-sdk/orag` | 示例文档来源 URI。 |
| `ORAG_DOC_CONTENT` | 内置中文说明 | 导入到知识库的文本。 |
| `ORAG_QUERY` | `ORAG 支持哪些能力？` | 查询问题。 |
| `ORAG_PROFILE` | `realtime` | RAG profile，可设为 `high_precision`。 |

示例：

```bash
ORAG_QUERY="ORAG 如何做混合检索？" go run ./examples/go/basic
```

## HTTP 封装建议

- 将 `Client`、请求/响应结构体和 `doJSON` 复制到业务项目的 SDK 包中。
- 生产环境不要硬编码账号密码，建议从安全配置系统读取 token 或凭据。
- 对外服务可复用 `context.Context` 控制超时和取消，并按业务需要调整 `http.Client.Timeout`。
- API 字段以 `api/openapi.yaml` 为准，升级 ORAG 后建议对照 OpenAPI 更新结构体。
