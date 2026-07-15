# 教程克隆与 Pack 安装

教程目录中的模板是全局、只读且版本化的资源。克隆动作会创建一个普通的 tenant 项目，并通过持久化任务把指定的 Quick Pack 或 Benchmark Pack 校验后写入该项目的私有输出存储。

此能力仍是 `experimental`。本阶段只完成项目创建和 Pack 安装；知识库/数据集构建、P0–P8 执行、文本/视觉/视频 Live Run、Replay 和结果比较尚未开放。

## 使用流程

1. 使用具有 tenant admin 权限的账号打开 Console 的 `/tutorials`。
2. 打开教程，选择 Quick Pack 或 Benchmark Pack，确认上游许可。
3. 填写项目名称并创建。响应立即返回 `202 Accepted` 和轮询地址，不会等待下载完成。
4. Console 轮询任务，展示创建项目、清单校验、下载、校验、写入私有存储和完成状态。
5. 任务失败时页面仅显示稳定的失败代码；修复外部条件后可以从安全检查点重试。

创建、任务和项目实验响应不会包含 Access Key、Secret、私有 bucket、对象 key、签名 URL 或公共包的内部解析细节。只读目录仍会公开模板声明的 `manifest_url`，但浏览器不会把它用作写入凭证或私有下载源。

## API

所有接口均需要 Bearer token，并在 OpenAPI 中标记为 `experimental`：

| 操作 | 端点 | 权限 |
| --- | --- | --- |
| 创建克隆 | `POST /v1/tutorials/{template_id}/clones` | tenant admin |
| 查询任务 | `GET /v1/tutorial-clone-jobs/{job_id}` | 项目可读 |
| 重试失败任务 | `POST /v1/tutorial-clone-jobs/{job_id}:retry` | 项目可写 |
| 查询项目实验 | `GET /v1/projects/{project_id}/tutorial-experiment` | 项目可读 |

使用同一 tenant、主体、模板版本和 `idempotency_key` 重复提交会返回同一个项目和任务，而不是创建第二份 Pack。

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
```

该命令启动临时 PostgreSQL、Qdrant、API、Console 和只读本地 Pack fixture，运行真实浏览器流程，并确认私有输出目录确实写入了经 SHA-256 校验的对象。它不验证外部 OSS ACL；发布前仍须执行上面的匿名读取检查。
