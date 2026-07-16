# Text RAG 官方 Replay

`GET /v1/tutorials/text-rag/replay` 提供 `text-rag/1.0.0/benchmark/replay-v1` 的离线、只读官方快照。它不调用模型、不创建项目，也不读取用户的私有对象存储。

快照固定引用受控 Benchmark Pack，并公开以下可审计信息：

- 模板/版本/Pack tier、Pack Manifest SHA-256；
- 运行环境 SHA-256、构建版本、evaluator 版本和生成时间；
- P0 与 P8 的 `high_precision`/Top-K 8 定义，P0 5/6000 与 P8 3/6000 的 Context Pack 合约；
- 聚合运行与索引事实，以及 snapshot fingerprint。

Replay 不是当前用户环境的 Live Run，也不应被当作模型质量承诺。要验证自己的环境，请克隆 Benchmark Pack 后执行 P0 与候选实验；服务端会持久化自己的 Manifest/环境/构建证据，并仅在完全可比时返回比较结果。

视觉文档和视频 Replay 尚未发布。对应 API 返回 `404 tutorial_replay_not_found`，而非展示推测结果或空白指标。
