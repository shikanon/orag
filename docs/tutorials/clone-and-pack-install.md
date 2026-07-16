# 教程克隆与 Pack 安装

教程目录中的模板是全局、只读且版本化的资源。克隆动作会创建一个普通的 tenant 项目，并通过持久化任务把指定的 Quick Pack 或 Benchmark Pack 校验后写入该项目的私有输出存储。

此能力仍是 `experimental`。具有受支持 `runtime` 声明的 `text-rag` Quick 与 Benchmark Pack 都能创建项目知识库/数据集并运行 P0–P8 单变量实验。Quick 固定 `realtime`/Top-K 5；受控 Benchmark Pack 固定 `high_precision`/Top-K 8，并持久化 Manifest SHA-256、运行环境 SHA-256 和构建版本。P1–P3 与 P7 使用独立候选索引，P4/P5/P6/P8 复用兼容 P0 索引。`text-rag` 另提供固定环境的只读官方 Replay；视觉文档/视频 Replay 与 Live Run 仍未开放。

## 使用流程

1. 使用具有 tenant admin 权限的账号打开 Console 的 `/tutorials`。
2. 打开教程，选择 Quick Pack 或 Benchmark Pack，确认上游许可。
3. 填写项目名称并创建。响应立即返回 `202 Accepted` 和轮询地址，不会等待下载完成。
4. Console 轮询任务，展示创建项目、清单校验、下载、校验、写入私有存储、基线资源创建和完成状态。
5. 对声明了文本运行时的 Quick 或 Benchmark Pack，安装完成后打开实验页并提交 P0 baseline。浏览器只提交 variant 与幂等键；知识库、数据集、检索 profile、Top-K、对象路径、模型配置和复现证据全部由服务端从不可变 Manifest 推导。
6. 仅当 Pack 声明候选且匹配的 P0 已完成，才可启动 P1–P8。每个候选只改变其 Pack 声明的单一变量；服务端冻结相同 Pack、模型、评测器、数据集、profile 和 Top-K，并按候选契约复用 P0 或建立独立知识库；比较端点返回持久化的标准评测指标，以及实际索引和运行时审计事实，不会推测成本、延迟或质量。
7. 任务或运行失败时页面仅显示稳定的失败代码；修复外部条件后可以从安全检查点重试。

创建、任务和项目实验响应不会包含 Access Key、Secret、私有 bucket、对象 key、签名 URL 或公共包的内部解析细节。只读目录仍会公开模板声明的 `manifest_url`，但浏览器不会把它用作写入凭证或私有下载源。

## API

所有接口均需要 Bearer token，并在 OpenAPI 中标记为 `experimental`：

| 操作 | 端点 | 权限 |
| --- | --- | --- |
| 创建克隆 | `POST /v1/tutorials/{template_id}/clones` | tenant admin |
| 查询任务 | `GET /v1/tutorial-clone-jobs/{job_id}` | 项目可读 |
| 重试失败任务 | `POST /v1/tutorial-clone-jobs/{job_id}:retry` | 项目可写 |
| 查询项目实验 | `GET /v1/projects/{project_id}/tutorial-experiment` | 项目可读 |
| 启动 P0/P1–P8 Live Run | `POST /v1/projects/{project_id}/tutorial-experiments/{experiment_id}/runs` | 项目可写 |
| 查询 P0/P1–P8 Live Run | `GET /v1/projects/{project_id}/tutorial-experiments/{experiment_id}/runs/{run_id}` | 项目可读 |
| 对比已完成候选与 P0 | `GET /v1/projects/{project_id}/tutorial-experiments/{experiment_id}/runs/{run_id}/comparison` | 项目可读 |
| 取消 P0/P1–P8 Live Run | `POST /v1/projects/{project_id}/tutorial-experiments/{experiment_id}/runs/{run_id}:cancel` | 项目可写 |

使用同一 tenant、主体、模板版本和 `idempotency_key` 重复提交会返回同一个项目和任务，而不是创建第二份 Pack。

运行使用其自身的 `idempotency_key`，其响应持久化 `evaluation_run_id`，可继续使用既有评测 API 查询真实结果。缺少已完成、输入一致的 P0 时启动候选返回 `409 tutorial_baseline_required`。未声明受支持运行时的已安装 Pack 返回稳定的 `tutorial_runtime_unavailable`，不会伪造 Replay 或评测结果。变量、审计证据和 Pack 发布契约见各候选指南（P1–P8）。

## 部署前检查

公共 Pack 源必须由匿名 HTTPS GET 访问；服务端只会从 `TUTORIAL_CATALOG_BASE_URL` 解析目录内相对路径，并拒绝重定向、跨源 URL、路径穿越、未知 Manifest 字段、超大响应、MIME/长度不匹配和 SHA-256 不匹配。

```bash
base_url="${TUTORIAL_CATALOG_BASE_URL:-https://orag.oss-cn-guangzhou.aliyuncs.com/tutorial-packs}"
curl --fail --location --max-time 30 \
  "$base_url/text-rag/1.0.0/quick/manifest.json" \
  -o /tmp/orag-text-rag-manifest.json
sha256sum /tmp/orag-text-rag-manifest.json
```

当前默认 OSS 地址在 2026-07-16 返回 `403 AccessDenied`。这表示外部发布前置条件尚未满足：需要公开对象及其匿名读 ACL。不要为绕过该错误向浏览器注入 OSS 凭证，也不要把私有输出 bucket 用作下载源；失败任务会保持可重试状态。

生产环境要求 `TUTORIAL_CATALOG_BASE_URL` 使用 HTTPS。`ORAG_TEST_MODE=true` 只允许受控本地测试 fixture 使用 HTTP。私有输出可使用本地目录，或使用与公共源不同的 `aliyun_oss` bucket；相关凭证只由服务端读取。

## 验证

```bash
make console-real-tutorial-clone-e2e
make console-real-tutorial-benchmark-e2e
```

两个命令均启动临时 PostgreSQL、Qdrant、API、Console 和只读本地 Pack fixture。Benchmark 命令运行 `high_precision`/Top-K 8 的 P0/P8，验证 P0 索引复用、Context Pack 审计、Manifest/环境 SHA-256 与构建版本。它们不验证外部 OSS ACL；发布前仍须执行上面的匿名读取检查。
