# Text RAG 官方 Replay 设计

## 目标

为 `text-rag` 提供一个离线、只读、可验证的官方 Replay。它让未配置模型或私有存储的用户查看固定 P0–P8 实验摘要；它不是 Live Run、不会执行模型、不会创建项目，也不会声称来自当前用户环境。

## 契约

- Snapshot 随源码内嵌，版本化为 `text-rag/1.0.0/benchmark/replay-v1`，并包含 template/version、Pack tier、Manifest SHA、运行环境 SHA、build revision、evaluator 版本、生成时间、P0 与 P8 指标/索引事实及可读说明。
- API 只读返回 snapshot 和 SHA-256；服务端解析时拒绝未知字段、错误版本、空指标、非 SHA、私有坐标、凭证样式字段或不满足 P0→P8 Context Pack 合约的数据。
- Console 在教程详情页显示“官方 Replay 可用”，提供只读摘要页；明确区分官方固定环境与用户 Live Run，且不显示下载/克隆凭证。
- 仅 text-rag 开放。视觉文档和视频的 catalog 继续声明 `replay_available=false`，防止 UI 暗示未发布的结果。

## 数据与安全

Snapshot 不保存 query、答案全文、Trace、对象 key、URL 参数、API Key 或用户标识。它只展示聚合的、公开可审计的指标和运行定义。Snapshot fingerprint 由 canonical JSON 的 SHA-256 得出，API 与 Console 都只读使用。

## 验收

单元测试覆盖解析、污染字段拒绝和 API；OpenAPI/Console 测试覆盖 read-only 返回与页面；真实 Console 回归在不启动模型或 Pack clone 的条件下读取 Replay。ROADMAP 只移除官方 text-rag Replay，视觉/视频仍待完成。
