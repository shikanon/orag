# 可验证性能基线报告

性能基线不是一组脱离环境的数字。ORAG 的 `orag.performance-baseline.v1` 报告把一次受控运行的固定负载、数据指纹、运行环境、构建版本和六类指标绑定在同一 JSON 文档中；只有这些维度完全相同的报告才可比较。

当前契约面向 `text-rag` Benchmark Pack 的 deterministic mock 路径。它可在无真实 Key 的条件下运行，因而适合回归对比；它不代表生产模型、云网络或任意硬件的绝对性能。

## 必填口径

- `provenance`：`workload_id`、`dataset_fingerprint`、`runtime_environment_sha256`、`build_revision` 与 `pack_tier=benchmark`；并且必须显式为 `deterministic_mock=true`。
- `load`：预热请求数、至少 20 个测量请求和并发度。
- `metrics`：入库文档数、入库耗时与由二者精确推导的吞吐；查询 p50/p95；缓存命中率；评测耗时；模型调用数与 USD 成本。

验证会拒绝未知字段、多个 JSON 值、无效 SHA-256、非 mock 运行、少于 20 个测量请求、p95 小于 p50、超出 `[0,1]` 的缓存命中率，以及无法由原始值复算的吞吐。

```bash
make console-real-tutorial-benchmark-e2e
make benchmark-report-verify BENCHMARK_REPORT=path/to/report.json
```

前一个命令验证真实 PostgreSQL、Qdrant、迁移、API、Console 与固定 Benchmark fixture 的可复现教程路径。第二个命令验证报告的比较前提和统计口径；运行器生成正式公开报告前，不应在 README 或营销材料中宣称任何通用性能数字。

## 比较规则

两份报告只有在 schema、workload、Pack tier、mock 标记、数据指纹、运行环境 SHA、构建版本、预热数、测量数和并发度全部相同的情况下才可比较。环境或 build 改变时，应保留报告用于审计，但从新基线开始，而不是把差异归因于代码性能。
