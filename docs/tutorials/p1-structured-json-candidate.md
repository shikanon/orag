# P1：结构化 JSON 解析候选

`p1_structured_json` 是教程实验室的第一个候选策略，状态为 `experimental`。它用于验证**文档解析方式**对检索与标准评测结果的影响；它不是通用 JSON ETL，也不代表该策略已在任意数据集或生产流量中胜出。

## 可比较的运行

只能在同一项目内已完成的 P0 `baseline` 之后启动 P1。浏览器只提交候选 `variant` 和幂等键；服务端从克隆时保存、并已校验的 Pack Manifest 推导全部运行输入：

- 相同的模板、Pack 层级与版本；
- 相同的数据集、`realtime` profile、Top-K、模型提供方/模型、提示缓存模式和标准评测器版本；
- 独立的项目知识库和索引命名空间，避免 P1 复用 P0 的向量或文档；
- 唯一变化为 P1 对 Pack 声明的 `.json` 文档使用 `structured_json` 解析器。P0 始终使用 `basic` 解析器，不受服务全局 `INGEST_PARSER_METHOD` 影响。

服务端为这些不变量生成 comparison fingerprint，并在候选入队和实际执行前都进行验证。若 P0 不存在、尚未完成，或其 comparison fingerprint 不匹配，P1 会返回 `409 tutorial_baseline_required`，不会创建一个看似可比较的运行。

成功的 P1 Run 会保存 P0 父 Run、parser method、知识库/数据集 ID、profile、Top-K 和两个 fingerprint。`GET /v1/projects/{project_id}/tutorial-experiments/{experiment_id}/runs/{run_id}/comparison` 仅在这些持久化证据完整时返回 `comparable=true`；指标来自已完成普通评测 Run 的真实结果，并按指标给出 P0/P1 绝对值、绝对差和可计算时的相对差。没有成本、延迟或置信度证据时，API 不会虚构这些数字。

## Pack 声明与边界

候选不会由客户端、查询参数或全局环境变量开启。一个 Pack 必须在其不可变 Manifest 的 `runtime.candidates` 中显式声明：

```json
{
  "id": "p1_structured_json",
  "chapter": "p1_document_parser",
  "parser_method": "structured_json"
}
```

同一 Manifest 还必须声明至少一个 `application/json` 或 `.json` 文档。未知候选、重复候选、错误 chapter/parser，或不含 JSON 文档的声明都会在 Pack 校验时被拒绝。

当前受控测试 fixture 位于 `tests/fixtures/tutorial-packs/text-rag/1.0.1/quick/`。它只用于本地 PostgreSQL + Qdrant/浏览器回归，不会自动发布，也不会修改已发布的 `text-rag/1.0.0` 公共 Pack。创建官方 `1.0.1` 版本时，必须通过独立的 Pack 发布流水线写入新的语义版本目录，并验证匿名 HTTPS 读取、Manifest/对象 MIME、长度和 SHA-256；不得覆盖 `1.0.0`，也不得把公开 Pack 凭证、私有输出 bucket 或签名 URL 暴露给浏览器。

## Console 操作

1. 克隆带有该候选声明的 Text Quick Pack，并等待 Pack 安装完成。
2. 在“文本 Quick 解析候选”先运行 P0 基线。
3. P0 状态为 completed 后运行 P1 解析候选。
4. 打开候选 Run 的对比表，核对 P0 父 Run、parser、comparison fingerprint，以及真实标准评测指标差异。

无真实模型 Key 的 `make console-real-tutorial-clone-e2e` 使用 deterministic mock 验证克隆、安装、独立私有索引、P0/P1 运行和 UI 对比流程。它证明协议和隔离边界，不构成 P1 相对 P0 的质量结论；质量结论必须来自版本化公开 Pack、固定模型配置和可复现的标准评测结果。
