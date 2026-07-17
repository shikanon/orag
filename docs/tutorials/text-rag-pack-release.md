# Text-RAG 1.1.0 公共数据包

`text-rag/1.1.0` 是 CRUD-RAG 的可复现公开发布，包含可直接 Clone 的 Quick/Benchmark Pack，以及完整上游 `data/` 归档。所有文件均为匿名 HTTPS 读取，不需要访问密钥。

公共根目录：`https://lensrhyme.tos-cn-hongkong.volces.com/tutorial-packs/text-rag/1.1.0/`

| 工件 | 用途 |
| --- | --- |
| `quick/manifest.json` | 约 65.9 MB 的快速运行 Pack。 |
| `benchmark/manifest.json` | 约 216.4 MB 的完整运行 Pack。 |
| `source/CRUD-RAG-1aace383994e.tar.gz` | 上游锁定提交 `1aace383994e1f68efa12cf2a8e2dadfb4102ceb` 的完整 `data/` 原始归档。 |
| `SOURCE.json` | 上游仓库、提交、许可证和构建元数据。 |
| `SHA256SUMS` | 全部发布工件的 SHA-256 校验表。 |

两个 Manifest 都声明了 P1–P8 候选能力和 2,394 条评测样本。Quick 用于低成本的端到端验证；Benchmark 用于完整实验。原始归档不会被运行时错误解析为文本，而是保留为可审计、可复取的完整数据副本。

校验示例：

```bash
base=https://lensrhyme.tos-cn-hongkong.volces.com/tutorial-packs/text-rag/1.1.0
curl -fsSLO "$base/SHA256SUMS"
# 下载 SHA256SUMS 中列出的工件后：
shasum -a 256 -c SHA256SUMS
```

发布器位于 `cmd/orag-pack-release`。构建与发布分离；发布只从环境变量读取对象存储凭证，使用禁止覆盖写入，并在 Manifest 最后上传。发布完成后应运行 `-verify-public`，它会匿名下载所有 `SHA256SUMS` 中的文件并逐个验证摘要。

仓库每天还会运行独立的 `public pack verification` workflow。任何人无需凭据均可复现相同检查：

```bash
make tutorial-pack-public-verify
```

该检查要求 HTTPS、HTTP 200、对象 MIME、声明长度和 SHA-256 都符合公开清单；它是发布物可用性证据，不是模型质量、生产性能或第二个公开 benchmark 的结论。
