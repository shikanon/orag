# API Smoke 流程

本页说明如何用 `examples/curl/` 完成一轮本地端到端验证。它适合确认认证、知识库创建、文档入库、查询和评估主路径是否可用。

## 前置状态

先确保 API 服务已启动：

```bash
curl -fsS http://localhost:8080/healthz
curl -fsS http://localhost:8080/readyz
```

如果 `go` 不在当前 shell 的 `PATH` 中，可临时使用：

```bash
PATH="/usr/local/go/bin:/opt/homebrew/bin:$PATH" make run
```

## 执行顺序

从仓库根目录依次执行：

```bash
examples/curl/00_login.sh
examples/curl/10_create_kb.sh
examples/curl/20_upload_doc.sh
examples/curl/30_query.sh
examples/curl/40_eval.sh
```

每个脚本的职责如下：

| 脚本 | API | 写入状态 |
| --- | --- | --- |
| `00_login.sh` | `POST /v1/auth/login` | `.orag-demo/token` |
| `10_create_kb.sh` | `POST /v1/knowledge-bases` | `.orag-demo/kb_id` |
| `20_upload_doc.sh` | `POST /v1/knowledge-bases/{id}/documents:import` | `.orag-demo/document_id`、`.orag-demo/job_id` |
| `30_query.sh` | `POST /v1/query` | 读取 token 和 knowledge base ID |
| `40_eval.sh` | `POST /v1/datasets`、`POST /v1/datasets/{id}/items`、`POST /v1/evaluations` | `.orag-demo/dataset_id` |

## 常用覆盖变量

| 变量 | 默认值 | 用途 |
| --- | --- | --- |
| `BASE_URL` | `http://localhost:8080` | API 服务地址。 |
| `ADMIN_USERNAME` | `admin` | 登录用户名。 |
| `ADMIN_PASSWORD` | `admin` | 登录密码。 |
| `KB_NAME` | 脚本内默认值 | 知识库名称。 |
| `DOC_NAME` | 脚本内默认值 | 文档名称。 |
| `DOC_CONTENT` | 脚本内默认值 | JSON 文本入库内容。 |
| `QUERY` | 脚本内默认值 | 查询问题。 |
| `PROFILE` | `realtime` | RAG 查询 profile。 |

示例：

```bash
BASE_URL=http://localhost:8080 QUERY="ORAG 支持哪些检索方式？" examples/curl/30_query.sh
```

## 状态目录

脚本运行状态保存在 `.orag-demo/`，包括 token、知识库 ID、文档 ID、数据集 ID 和入库 job ID。该目录只用于本地 smoke，不应提交到 Git。

如需重新开始：

```bash
rm -rf .orag-demo
```

## 常见失败

| 现象 | 可能原因 | 处理 |
| --- | --- | --- |
| `missing_bearer_token` | 未先运行登录脚本，或 `.orag-demo/token` 不存在。 | 重新运行 `examples/curl/00_login.sh`。 |
| `knowledge_base_not_found` | `.orag-demo/kb_id` 已过期或后端数据被清空。 | 重新运行建库和入库脚本。 |
| `/readyz` 不通过 | PostgreSQL、Qdrant 或 collection 未就绪。 | 查看 `../operations/troubleshooting.md`。 |
| 查询无上下文 | 文档未成功入库或 collection 为空。 | 查询 `.orag-demo/job_id` 对应的 ingestion job。 |
