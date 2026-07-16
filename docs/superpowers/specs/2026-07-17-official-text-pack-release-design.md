# 正式 Text RAG 全量 Pack 发布设计

## 目标

将用户授权分发的 CRUD-RAG 全量来源数据构建为可匿名读取、版本化、可审计的 `text-rag` 正式 Pack，并把教程 catalog 从失效的 OSS 地址切换到新的 TOS 公共前缀。发布过程可在本地或 CI 重跑，但不会提交数据、凭据或私有对象坐标。

## 发布物与版本

- 以锁定的上游 Git commit 获取来源，构建 `text-rag/1.1.0` 的 `quick` 和 `benchmark`；旧 `1.0.0` 路径不覆盖。
- 每个 tier 都有 `manifest.json`、`SHA256SUMS`、`SOURCE.json` 与许可证说明。Manifest 继续是唯一被服务端 clone reader 信任的对象清单。
- Quick 是全量来源中的小型确定性切片；Benchmark 包含用户授权的完整来源数据与冻结评测集。两者均记录上游 commit、构建器版本、对象大小、MIME 和 SHA-256。

## 构建与校验

1. 发布器把上游仓库克隆到临时目录，要求指定 commit 可解析，且只从许可允许的 `data/` 路径取输入。
2. 构建阶段将原始文档与评测数据转换为 ORAG Manifest 支持的 `corpus/` 对象；排序、换行和 JSON 编码固定，保证相同输入产生相同摘要。
3. 本地验证器重新计算每个对象的长度、MIME 和 SHA-256，解析 Manifest，并验证 Quick/Benchmark 的 runtime 契约与 P0–P8 声明。
4. 上传器仅接受显式 `--publish`，拒绝已有目标版本目录；先上传不可变对象与校验文件，最后上传 Manifest。所有 TOS 凭据仅从环境读入。
5. 上传后以未认证 HTTPS 对每个对象回读并重算 SHA-256；任何失败都使发布失败，并保留可诊断但不泄露凭据的结果。

## TOS 与 catalog

- 公共前缀固定为 `https://lensrhyme.tos-cn-hongkong.volces.com/tutorial-packs`。
- 上传工具使用 TOS 的 S3 兼容 API；公开读取仅使用 HTTPS URL，不携带签名参数或凭据。
- catalog 发布新 `1.1.0` 不可变模板版本，manifest 相对路径继续在该公共前缀下解析。

## 安全与回滚

- 密钥不进入命令输出、shell history、仓库、文档或 CI 日志；错误文本只输出 bucket 无关的对象相对路径和校验状态。
- 发布不覆盖对象。若 catalog 合并前验证失败，用户继续得到先前版本；若 catalog 合并后出现问题，回退为先前模板版本而不删除公开对象。
- 构建输入和生成物均在临时目录或 `.gitignore` 的 release 输出目录，禁止 `git add` 数据产物。

## 验收

- 单元测试覆盖来源锁、可重复构建、Manifest/校验清单、已存在版本拒绝与匿名回读。
- 发布实际执行后，Quick/Benchmark 的 Manifest 及全部对象在无凭据 HTTPS 下可读且 SHA-256 匹配。
- 服务端目录 API 返回 `1.1.0`，Clone 能对新公开 Pack 完成匿名下载与服务器侧验证。
- 文档站在新服务器上线发布指南、公开前缀与可复现验证命令。
