# Text RAG 可复现 Benchmark Run

`text-rag` Benchmark Pack 是受控、实验性的可复现路径，不是公开 CRUD-RAG 全量数据发布。它固定 `high_precision` profile、Top-K 8、evaluator v5 与 Pack 声明的 P0–P8 候选；Quick 运行与 Benchmark 运行不能混合比较。

每个运行由服务器持久化 `pack_manifest_sha256`、`runtime_environment_sha256` 与 `build_revision`。浏览器不能提供这些值。P0 与候选仅在直接血缘、同一已验证 Pack、相同冻结评测输入、环境 SHA 和构建版本一致时返回 `comparable=true`。

本地无真实 Key 复现：

```bash
make console-real-tutorial-benchmark-e2e
```

该命令运行真实 PostgreSQL、Qdrant、迁移、API、Console 和固定 `1.0.9/benchmark` fixture，临时映射到 catalog 的 `1.0.0/benchmark` 路径。它创建 Benchmark 项目，执行 P0 与 P8，并验证 P0 的 5/6000、P8 的 3/6000、索引复用及三项复现证据。

正式公开全量 Pack 仍须单独完成上游许可、匿名 HTTPS、MIME/长度和 SHA-256 发布流程；不要向浏览器注入 OSS 凭证或把私有输出作为下载源。
