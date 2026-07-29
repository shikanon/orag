# 数据集与 RAG 方法研究依据

本页汇总 ORAG 当前数据集、检索方法和评估指标所对应的原始论文或官方技术来源，方便读者继续追溯设计背景。链接表示**研究依据或方法脉络**，不表示 ORAG 对论文系统的逐项复现，也不表示论文结论可以直接外推到 ORAG、任意模型或生产数据；实际效果仍应以冻结数据、配置和运行环境下的评测为准。

## 教程与评测数据集

| ORAG 教程 | 数据集来源 | 论文 | ORAG 使用边界 |
| --- | --- | --- | --- |
| `text-rag` | CRUD-RAG | [CRUD-RAG: A Comprehensive Chinese Benchmark for Retrieval-Augmented Generation of Large Language Models](https://arxiv.org/abs/2401.17043) | ORAG 发布可校验的 Quick/Benchmark Pack 和锁定版本的上游归档；详见 [`tutorials/text-rag-pack-release.md`](./tutorials/text-rag-pack-release.md)。 |
| `visual-document-rag` | ViDoSeek | [ViDoRAG: Visual Document Retrieval-Augmented Generation via Dynamic Iterative Reasoning Agents](https://arxiv.org/abs/2502.18017) | ORAG 只发布 Recipe，不镜像 ViDoSeek 文档、标注或派生语料；详见 [`tutorials/visual-document-rag.md`](./tutorials/visual-document-rag.md)。 |
| `video-rag` | Video-MME | [Video-MME: The First-Ever Comprehensive Evaluation Benchmark of Multi-modal LLMs in Video Analysis](https://openaccess.thecvf.com/content/CVPR2025/html/Fu_Video-MME_The_First-Ever_Comprehensive_Evaluation_Benchmark_of_Multi-modal_LLMs_in_CVPR_2025_paper.html) | ORAG 只发布私有导入与评测 Protocol，不分发视频、字幕、标注、问题或答案；详见 [`tutorials/video-rag-private-benchmark.md`](./tutorials/video-rag-private-benchmark.md)。 |

## RAG 与检索方法

| ORAG 能力 | 代表性原始来源 | 与当前实现的关系 |
| --- | --- | --- |
| Retrieval-Augmented Generation | [Retrieval-Augmented Generation for Knowledge-Intensive NLP Tasks](https://arxiv.org/abs/2005.11401) | RAG 的基础研究脉络；ORAG 采用工程化的检索、上下文打包和生成链路，并非该论文模型的逐项复现。 |
| Dense retrieval | [Dense Passage Retrieval for Open-Domain Question Answering](https://aclanthology.org/2020.emnlp-main.550/) | 为向量化 passage retrieval 提供代表性依据；ORAG 的实际召回行为取决于配置的 embedding provider 与 Qdrant。 |
| Sparse retrieval / BM25 脉络 | [The Probabilistic Relevance Framework: BM25 and Beyond](https://www.nowpublishers.com/article/Details/INR-019) | ORAG 当前 sparse 路径使用 PostgreSQL FTS，不应表述为对论文 BM25 公式的完整实现。 |
| Dense + sparse 排名融合 | [Reciprocal Rank Fusion Outperforms Condorcet and Individual Rank Learning Methods](https://doi.org/10.1145/1571941.1572114) | ORAG 使用 RRF 合并多路候选排名。 |
| 二阶段 rerank | [Document Ranking with a Pretrained Sequence-to-Sequence Model](https://aclanthology.org/2020.findings-emnlp.63/) | 为“先召回、再用模型重排”的研究脉络提供依据；ORAG 的具体 reranker 由 provider registry 决定。 |
| Query expansion / multi-query | [Query2doc: Query Expansion with Large Language Models](https://arxiv.org/abs/2303.07678) | 与使用 LLM 扩展检索表达的思路相关；ORAG P5 固定生成多条去重查询，但不复现 Query2doc 的训练或实验设置。 |
| HyDE | [Precise Zero-Shot Dense Retrieval without Relevance Labels](https://arxiv.org/abs/2212.10496) | `high_precision` 可生成假设文档参与检索；实际模型、融合和召回配置由 ORAG 运行时控制。 |
| Contextual Retrieval | [Introducing Contextual Retrieval](https://www.anthropic.com/news/contextual-retrieval) | 这是官方技术说明而非同行评审论文。ORAG 在索引前生成 chunk 定位上下文，但存储、FTS 和失败策略是 ORAG 自身实现。 |
| RAPTOR | [RAPTOR: Recursive Abstractive Processing for Tree-Organized Retrieval](https://openreview.net/forum?id=GN921JHCRw) | ORAG 生成递归摘要 chunk 并纳入普通 embedding/FTS 层；当前实现不是论文聚类、树构建和检索算法的完整复刻。 |
| Graph-based RAG | [From Local to Global: A Graph RAG Approach to Query-Focused Summarization](https://arxiv.org/abs/2404.16130) | 提供图式 RAG 的代表性研究背景。ORAG 当前是轻量实体共现关系和查询实体扩展，不等价于论文的社区检测、社区摘要或 global search。 |

## 评估方法与指标

| ORAG 能力或指标 | 代表性原始来源 | 使用说明 |
| --- | --- | --- |
| RAG 多维评估 | [RAGAS: Automated Evaluation of Retrieval Augmented Generation](https://arxiv.org/abs/2309.15217) | 支持将检索质量、答案相关性和忠实度拆开评估的整体方法论；ORAG 的同名或相近指标按自身代码定义计算。 |
| LLM-as-a-Judge / pairwise | [Judging LLM-as-a-Judge with MT-Bench and Chatbot Arena](https://arxiv.org/abs/2306.05685) | 支持模型评审与成对比较的研究依据；ORAG 额外保存 rubric、raw/parsed response，并用顺序交换和 gold set 校准降低偏差风险。 |
| Rubric-based generation evaluation | [G-Eval: NLG Evaluation using GPT-4 with Better Human Alignment](https://arxiv.org/abs/2303.16634) | 为结构化 rubric 评分提供代表性依据；不表示 ORAG 复现 G-Eval 的 prompt 或实验协议。 |
| QA-based factuality / QAG | [QAFactEval: Improved QA-Based Factual Consistency Evaluation for Summarization](https://aclanthology.org/2022.naacl-main.187/) | 与“从回答 claim 生成问题，再基于上下文验证”的评估思路相关；ORAG 的 verdict、coverage 和聚合规则以本仓库实现为准。 |
| NDCG | [Cumulated Gain-Based Evaluation of IR Techniques](https://doi.org/10.1145/582415.582418) | `ndcg_at_k` 的信息检索指标来源。 |
| 多样性敏感 alpha-NDCG | [Novelty and Diversity in Information Retrieval Evaluation](https://doi.org/10.1145/1390334.1390446) | `alpha_ndcg` 的研究来源；ORAG 依赖 `diversity_annotations` 中的 aspect/subquestion 标注。 |

## 如何引用 ORAG 的结果

- 同时记录数据集及版本、Pack/Recipe/Protocol 摘要、模型 provider/model、parser、chunk 配置、retrieval profile、Top-K、rerank 和 evaluator 版本。
- 区分“引用论文中的结果”“ORAG 官方 Replay”和“在自己环境运行的 Live Run”，三者不能直接混用。
- ORAG 的方法名称表示实现所处的研究脉络；需要声称“复现论文”时，应另外核对算法、prompt、模型、语料、超参数和评测协议。
- 论文链接不能替代数据许可。使用 CRUD-RAG、ViDoSeek 或 Video-MME 前仍须遵守各自数据源的许可与分发条款。
