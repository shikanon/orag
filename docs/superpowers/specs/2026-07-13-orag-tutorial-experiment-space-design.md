# ORAG 教程与实验空间设计

## 1. 摘要

ORAG 将新增一个面向学习、验证和策略迭代的“教程与实验空间”。它不是单模型或单 API 的性能排行榜，而是围绕少量端到端数据包，逐步演进同一条 RAG Pipeline，让用户观察文档解析、Chunking、Contextual Retrieval、多路召回、Rewrite、Rerank、Graph/RAPTOR、Context Pack、Citation 和 Cache 在不同数据场景中的质量收益、延迟与成本代价。

系统提供不可变的只读教程模板。用户可以先查看官方 Replay，也可以一键克隆为独立实验 Project，在自己的私有对象存储、知识库、Dataset、Pipeline、Trace 和评测结果上执行 Live Run。没有现成评测集的用户可以从自己的文本、图片或视频生成评测草稿，补充否定、冲突和应拒答样本，审核后使用相同实验框架迭代策略。

教程公开数据由阿里云 OSS 托管：

- Endpoint：`oss-cn-guangzhou.aliyuncs.com`
- Bucket：`orag`
- 公开域名：`https://orag.oss-cn-guangzhou.aliyuncs.com`

官方公开 Bucket 只用于匿名下载不可变教程数据，不接收用户写入。用户自部署产生的数据写入用户自己提供的私有 Bucket。

## 2. 背景与设计原则

ORAG 已具备文本、HTML、Office、PDF 和图片的入库路径，以及 Chunking、Contextual Retrieval、RAPTOR、轻量图关系、Dense/Sparse/RRF、Rerank、Query Rewrite、Multi-query、Query Router、Semantic Cache、生成、引用、评测与优化能力。控制台使用 Project 作为 Pipeline、Dataset、环境和发布资源的一级隔离边界。

现有视频示例主要验证素材和上传边界，尚未形成真实的视频理解、结构化时间片段和时序检索链路。视频教程将扩展现有多模态能力，使用 Doubao Seed 2.1 的视频输入完成结构化理解与打标，再将带时间戳的片段转为可检索证据。火山方舟支持通过 Files API、Base64 或公网 URL 传入视频；ORAG 默认采用可复用 File ID，并将传输、预处理和模型调用参数记录在 Trace 中。

设计遵循以下原则：

1. 数据集优先，而不是模块优先。使用少量端到端数据包观察多个模块在不同场景下的效果。
2. 一次实验只改变一个主要变量。Baseline 和 Candidate 的数据、模型、Prompt、随机参数与评测版本保持一致。
3. 质量、延迟、Token、费用和索引体积同时展示，不只展示提升项。
4. Replay 和 Live Run 明确分离。Replay 用于低门槛学习，Live Run 用于验证用户环境和策略。
5. 官方模板不可变，用户副本完全隔离。模板升级不会覆盖用户实验。
6. 自动生成数据默认只是草稿。经过证据校验和用户审核后才可晋级为 Golden。
7. 未检索到证据不等于事实不存在。否定、反驳和信息不足必须分别标注和评测。
8. 开源运行时不携带官方 OSS 写凭证，也不把长期 AK/SK 或模型 Key 暴露给浏览器。

## 3. 目标与非目标

### 3.1 目标

- 提供文本、视觉文档和视频三类端到端 RAG 教程。
- 让用户看到同一模块对不同数据场景的提升、无效和退化情况。
- 让用户看到同一数据场景随 Pipeline 演进的累计质量与工程成本变化。
- 支持官方只读模板、官方 Replay 和一键克隆的用户实验 Project。
- 支持 Quick Pack 与 Benchmark Pack 分层下载。
- 支持用户导入自己的素材并构建可审核的评测数据集。
- 支持否定语义、错误前提、冲突证据和应拒答问题。
- 保存可复现的运行环境、配置差异、Trace、指标、Token 和费用。
- 复用现有 Project、Knowledge Base、Dataset、Evaluation、Optimizer 和 Pipeline 能力。

### 3.2 非目标

- 不训练、微调或蒸馏模型。
- 不建设单模型、多模型或 Provider 的通用排行榜。
- 不为每个 ORAG 节点引入一个互不相关的小数据集。
- 不允许用户修改系统模板或官方 Replay。
- 不把用户实验数据写入官方公开 OSS Bucket。
- 不把模型 Judge 的输出直接视为不可质疑的 Golden 真值。
- 不保证所有第三方数据都可由 ORAG 重新分发；受限数据只发布下载适配器和转换说明。

## 4. 产品模型

### 4.1 空间模型

“教程空间”建立在现有 Project 模型之上，不新增平行的资源隔离体系。

```text
系统教程库
└── 只读模板 Project
    ├── TutorialTemplate
    ├── TutorialChapter
    ├── Baseline/Candidate Pipeline
    ├── TutorialPack 引用
    ├── ReplaySnapshot
    └── 预期指标范围
          │ 一键克隆
          ▼
用户 Tutorial Experiment Project
├── 独立 Knowledge Base 和 Dataset
├── 独立 Pipeline 草稿与索引 Namespace
├── 独立 Run、Trace 和 Evaluation
└── template_id + template_version 来源记录
```

系统模板 Project 由平台拥有并只读。用户克隆后得到普通 Project，但附带教程来源、当前章节和实验进度信息。模板升级只创建新版本；用户可以查看版本差异并主动克隆新版，系统不在原副本上执行覆盖式升级。

### 4.2 核心实体

#### TutorialTemplate

- `id`、`slug`、`title`、`summary`
- `version`、`status`、`published_at`
- `modality`：`text`、`visual_document`、`video`
- `difficulty`、`estimated_duration`
- `required_capabilities`、`required_providers`
- `quick_pack_ref`、`benchmark_pack_ref`
- `replay_refs`
- `project_template_id`

#### TutorialChapter

- 学习目标和适用场景。
- 父 Pipeline、Candidate Pipeline 和唯一主要变量。
- 推荐数据切片和指标。
- 预期收益、可能退化和工程代价说明。
- Replay、Live Run 和完成条件。

#### TutorialPack

- `pack_id`、`version`、`tier`、`manifest_url`
- 来源、论文、主页、许可证和再分发限制。
- 文件对象、媒体类型、大小和 SHA-256。
- Corpus、Query、Qrels、答案、证据和场景切片描述。
- 预计下载量、索引时间、模型调用量与费用范围。

#### TutorialCloneJob

- 来源模板和版本、目标 Project、用户与租户。
- 幂等键、当前阶段、完成比例和阶段历史。
- 重试次数、最后错误、可恢复动作和时间戳。

#### TutorialExperiment

- 用户副本与来源模板的关联。
- 当前章节、已完成章节和数据包安装状态。
- 默认 Replay 和用户 Live Run 的引用。

#### ExperimentVariant

- `baseline` 或 `candidate` 角色。
- Pipeline 定义、配置摘要和内容哈希。
- 相对于父版本的结构化差异。
- 索引 Namespace 与资源绑定。

#### ExperimentRun

- 数据包、切片、Pipeline 和环境版本。
- 模型、Prompt、温度、随机种子和代码提交。
- 指标、Trace、Token、费用、延迟和索引规模。
- 重复运行批次和置信区间。

#### ReplaySnapshot

- 官方固定环境下的只读结果。
- 完整的版本与参数摘要。
- 脱敏 Trace、指标、失败案例和成本。
- 生成时间与是否仍匹配当前模板。

#### DatasetDraft

- 从用户素材生成的候选问题、答案和证据。
- `generated`、`reviewed`、`golden` 状态。
- 生成模型、Prompt 版本、审核人和审核时间。
- 正负样本 `pair_id`、证据极性和冲突关系。

## 5. 分层数据包

### 5.1 三个端到端数据包

首期只维护三个核心数据包，所有 Pipeline 模块都在这些数据包的适用切片上运行。

#### 中文文本知识库

以 [CRUD-RAG](https://github.com/IAAR-Shanghai/CRUD_RAG) 的可再分发精选子集为主要来源。原始 Benchmark 提供中文新闻文档、问答、摘要、续写和幻觉修改任务；ORAG 将其转换为统一 Corpus、Query、Answer、Qrels 和 Evidence 格式。

场景切片：

- 精确事实和实体查询。
- 语义改写和口语化查询。
- 多文档综合。
- 长文档和证据跨 Chunk。
- 重复、近重复和干扰文档。
- 冲突、否定和信息不足。

#### 视觉文档知识库

以 [ViDoSeek](https://modelscope.cn/datasets/iic/ViDoSeek) 的可再分发精选子集为主要来源。它面向大规模视觉丰富文档的检索、推理和回答，覆盖 Text、Table、Chart、2D Layout，以及单跳和多跳问题。

场景切片：

- 纯文本页面。
- 表格与跨行列关系。
- 图表、图例和视觉数值。
- 复杂布局与阅读顺序。
- 跨页和跨文档证据。
- 单跳、多跳、否定和信息不足。

#### 视频知识库

以 [Video-MME](https://video-mme.github.io/home_page.html) 的可再分发精选子集为主要来源。它覆盖短、中、长视频，以及视觉、字幕和音频信息。ORAG 在合法再分发范围内增加面向 RAG 的结构化事件、时间片段和证据标注。

场景切片：

- 短、中、长视频。
- 有字幕、无字幕与字幕干扰。
- 动作、事件、顺序和跨片段推理。
- 整段理解与分段理解。
- 否定事件、错误前提和信息不足。

### 5.2 Quick Pack 与 Benchmark Pack

每个数据包提供两个层级：

- Quick Pack：公开 OSS 托管的小型、授权明确、经过校验的子集。目标是在几十秒到数分钟内完成教程实验。
- Benchmark Pack：更大、更可信的评测子集。允许再分发的文件托管在公开 OSS；受限文件由用户从原始站点下载，ORAG 只提供 manifest、校验和、下载适配器和转换脚本。

两种层级使用相同 schema 和场景标签，因此实验结果可以用相同页面展示，但不同数据包版本的分数不得直接声明为模块提升。

### 5.3 统一数据格式

```text
TutorialPack
├── corpus / media
├── queries
├── answers
├── relevant_documents
├── relevant_chunks_or_time_segments
├── supporting_evidence
├── refuting_evidence
├── scenario_dimensions
└── negative_or_insufficient_labels
```

文本证据使用文档与字符或 Chunk 范围；视觉文档证据使用文档、页码和区域；视频证据使用视频、开始时间、结束时间和事件标签。所有导入适配器必须保留原始来源 ID，以便追溯许可与标注。

## 6. OSS 存储设计

### 6.1 职责分离

```text
官方公开 OSS
https://orag.oss-cn-guangzhou.aliyuncs.com
└── manifest、公开数据、许可证、Replay 和校验信息

用户私有对象存储
└── 用户上传、克隆文件、索引产物、运行产物和评测结果
```

官方数据下载不需要凭证：

```dotenv
TUTORIAL_CATALOG_BASE_URL=https://orag.oss-cn-guangzhou.aliyuncs.com/tutorial-packs
```

用户部署时自行提供私有存储配置：

```dotenv
OBJECT_STORAGE_PROVIDER=aliyun_oss
OBJECT_STORAGE_ENDPOINT=
OBJECT_STORAGE_BUCKET_NAME=
OBJECT_STORAGE_ACCESS_KEY_ID=
OBJECT_STORAGE_ACCESS_KEY_SECRET=
```

官方数据包发布所需写凭证只存在于维护者 CI 或发布环境，不属于 ORAG 服务运行配置。浏览器不得接收长期 AK/SK。

### 6.2 对象布局

```text
tutorial-packs/
├── manifests/{template-id}/{version}/manifest.json
├── blobs/sha256/{prefix}/{checksum}
├── replays/{template-id}/{version}/{runtime-profile}/
├── licenses/{dataset-id}/NOTICE.md
└── indexes/{dataset-id}/{pack-version}/
```

数据对象以 SHA-256 内容寻址，多个模板可以引用同一对象。发布后不得覆盖；修订必须产生新 manifest 版本和对象 Key。

`manifest.json` 至少包含：

- 数据来源、论文、主页、许可证和再分发结论。
- Pack 层级、媒体类型和场景切片。
- Object Key、大小、MIME 和 SHA-256。
- Schema 版本和最低 ORAG 版本。
- Corpus、Query、Qrels、答案和证据文件关系。
- Replay 的模型、参数、代码版本和生成时间。
- 预计下载量、索引时间、运行耗时和费用范围。

## 7. Pipeline 演进与实验方法

### 7.1 演进阶梯

```text
P0 Basic Baseline
 ├─ P1 文档/媒体解析优化
 ├─ P2 Chunking/时间片段优化
 ├─ P3 Contextual Retrieval
 ├─ P4 Dense + Sparse + RRF
 ├─ P5 Query Rewrite / Multi-query
 ├─ P6 Rerank
 ├─ P7 RAPTOR / Graph Retrieval
 └─ P8 Context Pack / Citation / Cache
```

阶梯表达学习顺序，不要求每个数据切片都执行所有模块。例如视频打标和时间片段索引适用于视频 Pack，而 MinerU 表格解析适用于视觉文档 Pack。页面必须显示“适用”“不适用”或“未运行”，不能用零分代替不适用。

### 7.2 单变量消融

每个 Candidate 只比父 Pipeline 改变一个主要模块或一组不可分割参数。运行器在提交前校验差异：

- 数据包、切片和版本相同。
- Chat、Embedding、Rerank 和多模态模型相同。
- Prompt、温度、随机种子和评测版本相同。
- 除当前章节允许的节点、边和参数外，不存在其它差异。
- 需要重建索引的 Candidate 使用独立 Namespace。

如果条件不满足，结果仍可保存，但标记为 Exploratory，不进入模块增益汇总。

### 7.3 两种分析视图

按数据集查看同一场景随 Pipeline 演进的累计变化：

```text
视觉文档 / 表格
P0 Basic
P1 + MinerU
P2 + 结构化 Chunking
P4 + Hybrid Retrieval
P6 + Rerank
```

按模块查看同一改动在不同场景的收益与代价：

```text
Rewrite
├── 中文文本 / 口语化查询
├── 中文文本 / 精确实体
├── 视觉文档 / 多跳
├── 视频 / 上下文依赖
└── 延迟、Token 和费用变化
```

汇总必须同时显示提升、无显著变化、退化和样本不足。

## 8. 指标体系

### 8.1 入库与解析

- `parse_success_rate`
- `text_coverage`
- `layout_structure_preservation`
- `table_cell_preservation`
- `visual_evidence_coverage`
- `index_build_latency`
- `index_build_cost`

### 8.2 Chunking 与片段化

- `evidence_integrity`
- `boundary_truncation_rate`
- `redundancy_rate`
- `chunk_count`
- `average_chunk_tokens`
- `segment_evidence_coverage`

### 8.3 检索与排序

- `recall_at_k`
- `ndcg_at_k`
- `mrr`
- `map`
- `retrieval_failure_rate`
- `alpha_ndcg`
- `aspect_coverage`
- `rerank_gain`

### 8.4 回答与引用

- `answer_accuracy`
- `faithfulness`
- `groundedness`
- `citation_precision`
- `citation_support`
- `qag_score`
- `abstention_precision`
- `abstention_recall`

### 8.5 视频

- `temporal_segment_recall`
- `timestamp_error_ms`
- `event_coverage`
- `temporal_order_accuracy`
- `subtitle_alignment_accuracy`

### 8.6 工程指标

- P50/P95 延迟。
- Chat、Embedding、Rerank、多模态输入与输出 Token。
- 单样本和单次 Run 费用。
- Cache 命中率。
- 索引体积、对象存储流量和构建时间。

报告使用绝对值、相对变化、重复运行均值和置信区间。样本量不足时隐藏趋势结论并显示证据不足。

## 9. 否定语义与拒答评测

否定语义是三个端到端数据包中的横向场景切片，而不是额外的小型模块数据集。

### 9.1 样本类型

- 显式否定：“该功能不支持什么？”
- 量词与范围：“并非所有”“只有”“除……以外”。
- 条件否定：“未配置 Key 时是否仍能启动？”
- 错误前提：问题声称图片或视频中存在实际未出现的对象或事件。
- 证据反驳：知识库明确反驳用户陈述。
- 信息不足：证据既不能支持也不能反驳。
- 冲突证据：不同版本或来源给出相反结论。
- 时间否定：某事件在指定时间前后没有发生。
- 图片否定：查询未佩戴安全帽或不存在的视觉对象。
- 视频否定：查询未发生的动作、反转的事件顺序或缺失片段。

### 9.2 标签与指标

统一极性标签为 `SUPPORTED`、`REFUTED`、`INSUFFICIENT`。主要指标包括：

- `negation_accuracy`
- `refutation_recall`
- `contradictory_evidence_recall`
- `insufficient_evidence_accuracy`
- `unsupported_affirmation_rate`
- `evidence_polarity_consistency`
- `temporal_negation_accuracy`
- `false_premise_rejection_rate`

### 9.3 自动生成流程

1. 从已定位证据的正向样本提取实体、关系、量词、条件和时间范围。
2. 生成关系反转、否定量词、错误实体、时间反转和不存在事件。
3. 在完整 Corpus 中搜索支持与反驳证据。
4. 有明确反驳证据时标记 `REFUTED`；无法判断时标记 `INSUFFICIENT`。
5. 保存正负样本 `pair_id` 和证据极性。
6. 使用确定性规则、证据定位和模型 Judge 交叉验证。
7. 用户审核后才可从 `generated` 晋级为 `reviewed` 或 `golden`。

模型 Judge 不能单独决定真值，未召回证据不能直接转换为否定事实。

## 10. 视频理解链路

视频入库增加独立的 `VideoUnderstandingAdapter`，不把现有图片 `MultimodalParse` 简单改名后复用。

```text
视频对象
→ Files API 上传或复用 File ID
→ Doubao Seed 2.1 结构化理解
→ 标题、摘要、字幕、事件与时间段
→ 时间片段 Chunk
→ 文本/多模态 Embedding
→ Dense/Sparse/Hybrid/Rerank
→ 带时间戳的回答与引用
```

结构化输出至少包含：

- `start_time`、`end_time`
- `event`、`entities`、`actions`
- `transcript`、`visual_description`
- `source_video_id`、`file_id`
- `confidence`、`model`、`prompt_version`

教程对比整段理解和分段理解、不同 FPS、字幕拼接与时间对齐、事件标签粒度、Top-K 和 Rerank。费用报告必须包含视频预处理和多模态 Token。

## 11. 克隆与安装流程

克隆采用异步、幂等、可续跑的工作流：

```text
创建 Project
→ 检查用户对象存储
→ 获取并校验 manifest
→ 下载公开 Pack
→ 校验 SHA-256
→ 复制到用户私有 Bucket
→ 创建 Knowledge Base 和 Dataset
→ 构建 Baseline 索引
→ 导入 Pipeline 变体
→ 导入 Replay
→ 标记实验空间就绪
```

幂等键由租户、用户、模板版本、目标 Project 和客户端请求键组成。每个阶段必须先检查已完成状态和产物内容哈希，再决定复用或重做。失败不会把 Project 标记为 Ready；用户可以从最后一个安全检查点继续。

常见失败和恢复动作：

- 用户私有 Bucket 未配置或无写权限：修复配置后重试存储检查。
- 空间或下载配额不足：清理空间或切换 Quick Pack。
- SHA-256 不匹配：丢弃临时对象并重新下载。
- 数据许可证需确认：暂停在导入前，不下载受限文件。
- 模型 Provider 未配置：允许完成 Replay 安装，阻止 Live Run。
- 视频超出处理限制：提示切换 File ID/TOS 路径或缩小 Pack。
- 部分索引构建失败：删除 Candidate 临时 Namespace 后从该阶段续跑。

## 12. 用户自有数据集构建

数据集构建实验室提供以下流程：

```text
上传素材
→ 解析质量预览
→ Chunk/时间片段预览
→ 生成候选问答
→ 生成否定、冲突和拒答样本
→ 定位支持或反驳证据
→ 用户审核
→ 发布 Dataset
→ 运行相同 P0–P8 实验
```

页面并排展示原始文档页、图片或视频时间轴，Basic 与 Candidate 解析结果、Chunk 边界、证据截断、重复片段以及问题与证据的对应关系。

自动生成数据与人工 Golden 分开聚合。默认评测报告同时给出：

- Golden-only 指标。
- Reviewed + Golden 指标。
- Generated-only 诊断，不进入发布门禁。

## 13. 控制台信息架构

### 13.1 路由

```text
/tutorials
└── /:templateId

/projects/:projectId/tutorial
├── /setup
├── /chapters/:chapterId
├── /experiments/:experimentId
├── /dataset-builder
└── /runs/:runId
```

### 13.2 教程库

全局教程库只展示三个端到端教程。卡片显示数据规模、Quick/Benchmark 层级、可学习模块、Replay 状态、Live Run 依赖、预计耗时、下载量和费用。

### 13.3 教程详情

详情页包含：

- 教程目标和适用用户。
- 数据场景矩阵。
- P0–P8 演进阶梯。
- 官方 Replay 摘要。
- 模块历史增益与退化。
- 费用、依赖和许可证。
- 一键克隆入口。

### 13.4 克隆进度页

显示所有安装阶段、当前进度、阶段耗时、错误原因、可恢复动作和重试按钮。即使 Live Provider 未配置，也允许完成 Replay-only 实验空间。

### 13.5 实验工作台

- 顶部：教程进度、数据切片和 Replay/Live 模式。
- 左侧：P0–P8 Pipeline 演进阶梯。
- 中部：Baseline/Candidate 指标、趋势和场景矩阵。
- 右侧：唯一变量、配置差异、运行要求和学习解释。
- 底部：Trace、检索结果、引用、失败案例、延迟和费用。

工作台支持两种透视：按数据集查看 Pipeline 演进，按模块查看跨场景收益。默认同时展示提升和退化样本。

## 14. 后端边界

控制面新增以下教程模板、克隆任务和实验运行 API；实施计划可以拆分交付顺序，但不得改变这些资源边界：

```text
GET  /v1/tutorials
GET  /v1/tutorials/{template_id}
GET  /v1/tutorials/{template_id}/versions/{version}
POST /v1/tutorials/{template_id}/clones

GET  /v1/tutorial-clone-jobs/{job_id}
POST /v1/tutorial-clone-jobs/{job_id}:retry

GET  /v1/projects/{project_id}/tutorial-experiment
GET  /v1/projects/{project_id}/tutorial-experiments/{experiment_id}
POST /v1/projects/{project_id}/tutorial-experiments/{experiment_id}/runs
GET  /v1/projects/{project_id}/tutorial-experiments/{experiment_id}/runs/{run_id}
POST /v1/projects/{project_id}/tutorial-experiments/{experiment_id}/runs/{run_id}:cancel

POST  /v1/projects/{project_id}/tutorial-dataset-drafts
GET   /v1/projects/{project_id}/tutorial-dataset-drafts/{draft_id}
PATCH /v1/projects/{project_id}/tutorial-dataset-drafts/{draft_id}/items/{item_id}
POST  /v1/projects/{project_id}/tutorial-dataset-drafts/{draft_id}:publish
```

`POST /v1/tutorials/{template_id}/clones` 接收模板版本、Pack 层级、目标 Project 元数据和客户端幂等键，返回 `202 Accepted`、目标 Project ID、Clone Job ID 和轮询 URL。实验 Run 同样返回 `202 Accepted`，并通过 Run 资源暴露阶段进度、取消状态、指标和失败恢复信息。

这些 API 必须满足以下边界：

- 模板读取是全局只读操作。
- 用户实验资源仍受 tenant 和 Project 鉴权。
- 克隆任务只能写入目标用户 Project 和用户配置的对象存储。
- 官方公开 OSS 客户端只提供匿名读取，不接受运行时写入。
- Pack 下载校验、克隆状态和实验差异由服务端执行，不能信任浏览器提交的完成状态。
- Pipeline 单变量约束由服务端验证。
- Replay 是不可变快照，Live Run 是独立运行记录。

现有 `ObjectStorageConfig` 只有配置骨架；实施需要提供阿里云 OSS 客户端、匿名教程 Pack 读取器和用户私有对象存储写入器。两者使用不同接口和配置，避免官方公开 Bucket 被误用为用户存储。

## 15. 安全、许可与隐私

- `.env.example` 和文档只记录非敏感 Endpoint、Bucket 和公开目录地址。
- 官方发布 AK/SK 只进入维护者 CI Secret。
- 用户私有 AK/SK 只从运行环境或云身份链读取。
- 浏览器不得接收长期对象存储凭证或模型 Key。
- Pack manifest 必须包含许可证和再分发结论。
- 不允许再分发的数据不上传官方 OSS。
- Replay 在发布前移除用户数据、原始私密 Prompt、凭证和不可公开模型响应。
- 外部 URL 下载使用 manifest allowlist、大小限制、MIME 检查和 SHA-256，防止任意 SSRF 和无界下载。
- 用户素材、生成样本、Trace 和实验结果遵守 Project/tenant 隔离与保留策略。

## 16. 可观测性与成本

克隆和实验 Run 记录结构化阶段事件：

- `tutorial_clone_stage_started/completed/failed`
- `tutorial_pack_downloaded/verified`
- `tutorial_index_build_started/completed`
- `tutorial_experiment_run_started/completed/failed`
- `tutorial_replay_loaded`

指标覆盖下载字节、校验失败、克隆耗时、索引构建耗时、模型 Token、费用、运行延迟、失败率和重试次数。日志不得输出 AK/SK、签名 URL、原始私密内容或完整模型响应。

## 17. 测试与验收

### 17.1 契约测试

- Manifest schema、许可证、SHA-256 和媒体类型校验。
- 三个 Pack 使用统一数据格式。
- Replay 引用的模板、数据包和代码版本存在。
- 公开 Pack 中不存在受限文件或凭证。

### 17.2 单元测试

- 模板版本不可变。
- 单变量 Pipeline diff 校验。
- 否定语义配对、极性和状态晋级规则。
- 费用、置信区间和场景聚合。
- 视频时间段和证据引用转换。

### 17.3 集成测试

- 阿里云 OSS 匿名 Pack 下载和 SHA-256 校验。
- 用户私有对象存储写入与权限失败。
- 克隆幂等、断点续跑和失败清理。
- 独立索引 Namespace 不污染 Baseline。
- tenant/Project 跨边界访问被拒绝。
- Provider 缺失时 Replay 可用、Live Run 被明确阻止。

### 17.4 端到端测试

- 浏览教程、查看 Replay、选择 Pack 并克隆。
- 克隆失败后修复配置并续跑。
- 在三个 Quick Pack 上完成一次 Baseline/Candidate 配对实验。
- 切换数据集透视和模块透视。
- 查看提升、退化、Trace、引用、Token 和费用。
- 从用户素材生成包含否定问题的 DatasetDraft，并审核为 Golden。
- 保存 Candidate 为用户策略版本。

### 17.5 验收标准

1. 官方公开 Pack 下载不需要任何凭证。
2. 用户数据只写入用户提供的私有存储。
3. 三个 Quick Pack 均可从克隆运行到端到端评测。
4. 同一模块可以比较其在多个场景切片上的收益与代价。
5. 同一数据场景可以查看 P0–P8 演进结果。
6. 非单变量 Run 不进入模块增益汇总。
7. Replay 离线可读且不调用外部模型。
8. Live Run 的模型、数据、配置、Trace、Token 和费用可追溯。
9. 否定语义保留 `pair_id`、极性和证据；自动样本未经审核不进入 Golden 门禁。
10. 浏览器、日志、Replay 和公开 OSS 中不存在长期 AK/SK 或用户敏感数据。

## 18. 分阶段交付边界

完整功能包含多个可独立验证的子系统，实施计划应拆分为以下阶段：

1. 教程目录、模板版本与公开 Pack manifest。
2. 阿里云 OSS 匿名下载、用户私有存储与异步幂等克隆。
3. 中文文本端到端 Quick Pack 和 P0–P8 实验运行器。
4. 视觉文档端到端 Pack、解析/Chunking 对比和证据区域。
5. Doubao Seed 2.1 视频理解、时间片段索引和视频 Pack。
6. 数据集构建实验室、否定语义生成与审核。
7. 跨数据集模块分析、Replay 发布和完整验收。

每个阶段都必须具备独立迁移、OpenAPI、后端测试、控制台测试和可回滚边界。详细文件级任务、API 路径与数据库迁移将在用户审阅本设计后写入实施计划。
