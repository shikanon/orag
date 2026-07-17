# 可验证性能基线报告

性能基线不是一组脱离环境的数字。ORAG 的 `orag.performance-baseline.v1` 报告把一次受控运行的固定负载、数据指纹、运行环境、构建版本和六类指标绑定在同一 JSON 文档中；只有这些维度完全相同的报告才可比较。

当前契约面向 `text-rag` Benchmark Pack 的 deterministic mock 路径。它可在无真实 Key 的条件下运行，因而适合回归对比；它不代表生产模型、云网络或任意硬件的绝对性能。

## 必填口径

- `provenance`：`workload_id`、`dataset_fingerprint`、`runtime_environment_sha256`、`build_revision` 与 `pack_tier=benchmark`；并且必须显式为 `deterministic_mock=true`。
- `load`：预热请求数、至少 20 个测量请求和并发度。
- `metrics`：入库文档数、入库耗时与由二者精确推导的吞吐；查询 p50/p95；缓存命中率；评测耗时；模型调用数与 USD 成本。

验证会拒绝未知字段、多个 JSON 值、无效 SHA-256、非 mock 运行、少于 20 个测量请求、p95 小于 p50、超出 `[0,1]` 的缓存命中率，以及无法由原始值复算的吞吐。

```bash
make benchmark-report-run BENCHMARK_REPORT=.tmp/performance-baseline.json
make benchmark-report-verify BENCHMARK_REPORT=.tmp/performance-baseline.json
make performance-baseline-evidence-verify PERFORMANCE_BASELINE_EVIDENCE=docs-site/benchmarks/2026-07-17-darwin-arm64-main-75eda8f
make console-real-tutorial-benchmark-e2e
```

第一个命令使用公开 Go SDK 的 `MockConfig` 实际执行固定的三文档、三问题
`text-rag/mock-baseline-v1` workload：入库、10 次预热、20 次测量查询和一次评测。
它不读取真实 Key、服务环境变量或外部存储，并从观测到的 wall-clock 结果生成报告。
`model_calls` 是 mock 管线调用次数的明确记账（预热、测量和 evaluator 查询），
`cost_usd` 固定为零。第二个命令验证报告的比较前提和统计口径；第三个命令验证真实 PostgreSQL、Qdrant、迁移、API、Console 与固定 Benchmark fixture 的可复现教程路径。

本地 mock 结果只能作为同一机器、同一 Go 运行时、同一构建和同一负载下的回归基线。发布任何可比较的公开性能结果前，必须披露硬件、操作系统、Go 版本、provider/网络条件和完整命令；不得把该报告写成生产吞吐或跨硬件结论。

## 公开基线证据

首个公开工件是
[`2026-07-17-darwin-arm64-main-75eda8f`](../../docs-site/benchmarks/2026-07-17-darwin-arm64-main-75eda8f/manifest.json)。它记录了
`75eda8f80787d205e16e4ff7f65096bcd8926888` 的实际 deterministic-mock
SDK 运行，以及经 allowlist 处理的机器、运行时和系统披露。下载
`report.json`、`environment.json`、`manifest.json` 和 `SHA256SUMS` 后，可用上面的
Make 命令复核；完整性、报告 schema、mock 标记、build revision 和安全披露字段都必须通过。

新证据必须使用独立目录，不能覆盖旧报告：

```bash
./scripts/capture-performance-baseline-evidence.sh \
  --output .tmp/performance-baseline-evidence/my-evidence-id \
  --build-revision "$(git rev-parse HEAD)"
./scripts/verify-performance-baseline-evidence.sh \
  --dir .tmp/performance-baseline-evidence/my-evidence-id
```

该公开基线仍是本机回归证据，不得写成生产吞吐或跨硬件结论，也不替代有披露 provider 与网络条件的生产试点结果。

## 比较规则

两份报告只有在 schema、workload、Pack tier、mock 标记、数据指纹、运行环境 SHA、构建版本、预热数、测量数和并发度全部相同的情况下才可比较。环境或 build 改变时，应保留报告用于审计，但从新基线开始，而不是把差异归因于代码性能。
