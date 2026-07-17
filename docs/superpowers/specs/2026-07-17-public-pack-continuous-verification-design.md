# 公共 Pack 持续验证设计

## 目标

把 `text-rag/1.1.0` 发布后的一次性人工回读，变成可重复、可见的 GitHub Actions 门禁。该门禁只验证已公开的只读工件，不上传、替换或删除对象，也不读取任何对象存储凭据。

## 方案选择

1. **仅保留发布时验证**：成本最低，但 CDN、权限或对象元数据后续漂移不可见。
2. **在常规 CI 的每个 PR 验证**：反馈最快，但把外部对象存储可用性变成贡献者 PR 的不稳定依赖。
3. **独立的定时和手动验证 workflow（采用）**：发布后持续发现公共工件漂移，同时保持 PR CI 可重复且不依赖外部网络。

## 架构与数据流

workflow 在 GitHub-hosted runner 中匿名下载已发布版本根目录的 `SHA256SUMS`，写入临时目录 `text-rag/1.1.0/`。现有 `orag-pack-release -verify-public` 从该本地校验表派生固定公开前缀，并逐项 GET 其中声明的对象。

每个响应必须满足：HTTP 200、HTTPS URL、响应体 SHA-256 与清单匹配、已声明的 `Content-Length` 等于实际读取字节数、以及按扩展名确定的 MIME 类型（JSON 为 `application/json`、gzip 为 `application/gzip`、其余文本为 `text/plain`，允许 MIME 参数）。验证器不接受重定向后的非 HTTPS URL。workflow 不打印 URL 中不存在的凭据，也不下载到仓库工作树。

## 失败与运维

任一对象缺失、摘要不符、长度不符、MIME 漂移或网络超时都会使独立 workflow 失败。workflow 名称明确为公共发布物健康检查，不能据此推断 Benchmark 的生产性能、模型质量或第二份公开 benchmark。修复路径是诊断 CDN/对象权限或发布新不可变版本；已发布对象绝不原地覆盖。

## 验收

- 单元测试覆盖 MIME、长度、非 HTTPS 重定向和既有 SHA-256 合同。
- `make tutorial-pack-public-verify` 无凭据运行并验证当前公开 `text-rag/1.1.0`。
- workflow 支持手动运行和每日定时运行，且不成为 PR 必需 check。
- 文档说明其范围和本地复现命令。
