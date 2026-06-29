# Go 语言可扩展智能问答(RAG)框架技术方案

> 基于 Eino(CloudWeGo Go 原生 LLM 框架)自建一套通用、可扩展的 RAG 系统:支持检索前优化(问题重写)、检索后优化(重排)、混合检索,底层可插拔接入各种知识库,具备效果评审、自迭代、数据集管理。语言:Golang。

---

## 1. 框架基座

**基座:Eino(CloudWeGo / ByteDance 出品的 Go 原生 LLM 应用框架)**

- 提供组件抽象:ChatModel / Embedding / Retriever / Indexer / Reranker / Loader / Transformer。
- 提供编排引擎:Graph / Chain / Workflow,支持流式、并发、分支、切面。
- 生态实现:OpenAI / Claude / Gemini / Ark / Ollama + ES / Milvus / Redis / VikingDB。
- 配套:EinoDev 可视化调试、回调切面、LangFuse 接入。

系统在 Eino 之上自建三层:知识库适配层、自迭代闭环层、应用接入层。

---

## 2. 整体架构(五层)

```
┌─────────────────────────────────────────────────────────────┐
│  L5 应用接入层   HTTP/gRPC API · 会话管理 · 多租户 · 鉴权        │
├─────────────────────────────────────────────────────────────┤
│  L4 自迭代闭环层 数据集管理 · 离线评估 · 指标回流 · 参数寻优      │
├─────────────────────────────────────────────────────────────┤
│  L3 RAG 编排层(Eino Graph)                                   │
│   SemanticCache → Query Rewrite → Hybrid Retrieve            │
│     → Fusion(RRF) → Rerank → Context Pack → Generate → Cite  │
├─────────────────────────────────────────────────────────────┤
│  L2 组件抽象层(Eino Components)                               │
│   ChatModel · Embedding · Retriever · Indexer · Reranker     │
│   Loader · Transformer(切分)                                  │
├─────────────────────────────────────────────────────────────┤
│  L1 知识库/存储层  向量库 · 倒排(BM25) · 图谱 · 对象存储         │
└─────────────────────────────────────────────────────────────┘
```

核心:**L3 检索链路是一张 Eino Graph,所有优化点(缓存/重写/混检/重排)都是图上可插拔节点;L1 知识库通过统一 Retriever 接口适配,换库不动上层。**

---

## 3. 知识库构建链路(L1 入库侧)

### 3.1 文档解析

- TXT / Word / Excel:SDK 直接提取文本、表格、图片。
- PDF / PPT / 网页:OCR + 版面分析;复杂版面/图表用视觉理解模型抽成 Markdown / JSON 结构化数据。
- 在线文档(飞书等):官方 API 提取。

### 3.2 知识单元切分

切分方法:分隔符/换行、文档结构与标题、自定义正则、LLM 语义切分。两条硬原则:

1. **知识完备**:不切断上下文,避免"半个知识块"导致召回缺失。
2. **词数精简**:单元词数控制在模型输入上限的 1/5 以内。

### 3.3 向量化与存储

- 文本:文本 Embedding 模型(bge-m3 等)。
- 图像:图像 Embedding 直接向量化,或 OCR/视觉理解转文本后再向量化。
- 音频:ASR 转文本后向量化。
- 向量 + 原文/资源链接以 KV 形式写入向量库。

### 3.4 自动化入库(事件驱动)

```
对象存储(S3/TOS)新文件上传
        │ Webhook
        ▼
Serverless 函数:解析 → 切分 → Embedding → 写库
        │
        ▼
向量库实时动态更新
```

---

## 4. RAG 编排链路(L3,Eino Graph)

### 4.1 语义缓存(Graph 入口)

查询向量化后先查缓存库(主向量库的独立 collection):

- 相似度超阈值 → 直接返回历史答案,绕过检索与 LLM。
- 未命中 → 走正常 RAG 链路,结果回写缓存。
- 关注**准确率**而非仅命中率,阈值设高;缓存容量用 LRU/FIFO 管理。

### 4.2 检索前优化:问题重写(Query Rewrite)

ChatModel 节点,策略可配置:

- **改写/澄清**:消解指代、补全上下文(多轮会话必需)。
- **多查询扩展(Multi-Query)**:生成 N 个等价 query 并行检索取并集。
- **HyDE**:生成假设答案再向量检索。
- **路由**:判断走哪个知识库 / 是否需要检索(寒暄直接跳过)。

### 4.3 混合检索 + 融合

Graph 并发分支同跑 dense / sparse,RRF 合并:

```
        ┌──> DenseRetriever  (TopK=50) ──┐
Query ──┤                                ├──> RRF Fusion ──> 候选集
        └──> SparseRetriever (TopK=50) ──┘
```

RRF:`score = Σ 1/(k + rank_i)`,k 取 60,无需调参、对不同打分尺度鲁棒。

### 4.4 检索后优化:重排 + 打包

- **Rerank**:统一 `Reranker` interface(Cohere / BGE-Reranker 本地 / Ark Rerank 可换),输入 TopK=100,输出 TopK=5~8。
- **Context Packing**:去重、按 token 预算裁剪、拼装引用元数据。

### 4.5 生成 + 引用

ChatModel 节点,Prompt 强约束"基于给定上下文回答 + 标注来源",输出 answer + cited chunk ids。

### 4.6 实时/高精 双档 Profile

用 Graph 的 Branch 按请求标志位切换,一套图覆盖两档:

| 档位 | 启用节点 | 适用 |
|---|---|---|
| **实时档**(默认) | 语义缓存 + 混检 + RRF + 轻量重排 | 在线问答(延迟敏感) |
| **高精档** | 额外开 问题重写 + Multi-Query + 检索结果总结 | 离线/非延迟敏感任务 |

> 凡引入额外 LLM 推理的优化(重写、扩展、总结)只进高精档;重排、融合检索不引入 LLM 推理,常驻实时档。

---

## 5. 大模型服务前缀缓存(Prefix / Prompt Cache)

区别于 §4.1 的语义缓存(命中即跳过推理),前缀缓存是**复用 prompt 公共前缀的 KV Cache**,在仍要走一次 LLM 推理时降低 TTFT 与 token 成本。RAG 场景的 prompt 通常由「固定 system 指令 + 大段知识上下文 + 用户问题」拼成,**前缀高度重复**,非常适合前缀缓存。

### 5.1 两种缓存模式

| 模式 | 触发方式 | 代表模型 | 接入要点 |
|---|---|---|---|
| **自动缓存** | 服务端自动识别重复前缀并命中,无需声明 | DeepSeek、部分 OpenAI / Ark 模型 | 无需改调用,只需**保证前缀字节级一致** |
| **主动缓存** | 调用方显式标记缓存断点 | Claude(`cache_control`)、部分需手动开启的模型 | 在请求体里对要缓存的段落打标记并设 TTL |

### 5.2 Prompt 结构设计(前缀缓存命中的前提)

把**稳定内容前置、可变内容后置**,最大化前缀重合:

```
[system 指令]        ← 完全固定,放最前(缓存命中率最高)
[少样本示例/工具描述]  ← 较稳定
[检索知识上下文]      ← 半稳定(同一文档多轮问答时可复用)
[历史对话]           ← 增量追加
[本轮用户问题]        ← 每次变化,放最后
```

任何前缀字节变动都会使缓存失效,因此:固定段不要内插时间戳/随机 id;序列化顺序、空白、字段顺序保持稳定。

### 5.3 统一缓存抽象(适配多模型)

在 L2 包一层 `PromptCacheStrategy`,屏蔽模型差异:

```go
type PromptCacheStrategy interface {
    // 按模型能力决定是否注入缓存标记
    Apply(req *ChatRequest, segments []PromptSegment) *ChatRequest
    Mode() CacheMode // Auto / Manual / None
}
```

- **Auto 模型**:策略只做"前缀稳定化"(排序、去随机化),不注入标记。
- **Manual 模型**:策略在 system / 知识上下文段注入 `cache_control` 等标记并配置 TTL。
- **None 模型**:直接透传。

调用方代码不感知差异,换模型只换 strategy 实现。

### 5.4 与语义缓存协同

```
请求 → 语义缓存(命中→直接返回,跳过 LLM)
        │ 未命中
        ▼
    RAG 检索 + Prompt 拼装(稳定前缀在前)
        │
        ▼
    LLM 调用(前缀缓存命中→降低 TTFT/成本)
        │
        ▼
    回写语义缓存
```

两层缓存正交叠加:语义缓存省"整次推理",前缀缓存省"重复前缀的计算"。

---

## 6. 可插拔知识库(L2 适配)

统一接口,各知识库实现 Eino 的 `Retriever` / `Indexer`:

```go
type KnowledgeBase interface {
    components.Retriever  // Retrieve(ctx, query, opts) ([]*schema.Document, error)
    components.Indexer    // Store(ctx, docs, opts) ([]string, error)
    Capabilities() KBCaps // 声明支持 dense/sparse/filter/hybrid
}
```

- 稠密向量:Milvus / Qdrant / VikingDB / ES dense_vector → dense retriever。
- 稀疏/关键词:Elasticsearch BM25 → sparse retriever。
- 图谱(GraphRAG):Neo4j adapter,query 转 Cypher,处理多跳关系推理。
- 通过 `Capabilities()` 让编排在运行时决定调用哪些通道;新增库只需实现接口并注册。

### 检索器进阶(按需)

- **递归检索(父子块)**:建索引时树结构切块,叶子块入索引;命中多个叶子指向同一父块时用父块替换送 LLM,适合长文档。
- **GraphRAG**:问题转图查询语句在知识图谱遍历,解决向量检索难处理的多跳关系。

---

## 7. 效果评审 + 自迭代闭环(L4)

### 7.1 数据集管理

```
datasets/
  ├── golden_set/        # 人工标注 (query, ground_truth, relevant_doc_ids)
  ├── regression_set/    # 回归集,每次发版必跑
  └── online_mined/      # 线上挖掘的真实 query
```

PostgreSQL 存数据集版本(version tag 可回溯)。线上按"低满意度/重问/点踩"挖掘 hard case → 标注 → 回流。

### 7.2 评估指标

| 层 | 指标 |
|---|---|
| 检索 | Context Recall / Precision、nDCG、MRR、Hit@K |
| 生成 | 答案忠实性(faithfulness)、答案相关性、Groundedness |
| 专项能力 | 噪声鲁棒性、拒答能力、信息整合、反事实鲁棒性 |
| 端到端 | 正确率、引用准确率、延迟 P95、Token 成本、缓存命中率 |

- **LLM-as-Judge** 对 (query, answer, context, ground_truth) 打分;Ragas / ARES 做自动化评测。
- Eino 回调切面采集每节点输入输出与耗时,直接喂评估器,无需埋点。

### 7.3 自迭代机制

```
线上 query 挖掘 ──> 标注回流 ──> 扩充 golden/regression set
        ↑                                    │
        │                                    ▼
   A/B 灰度上线 <── 参数寻优 <── 离线评估打分 <── 多配置批量跑
```

- 可调参数(配置化):chunk 大小/重叠、TopK、RRF 的 k、重排模型、是否开 Multi-Query/HyDE、缓存阈值、Prompt 模板、前缀缓存策略。
- **离线寻优**:参数网格在 regression_set 上批量评估,选最优组合(可用贝叶斯优化)。
- **在线灰度**:候选配置小流量 A/B,确认收益后全量。
- **回归门禁**:每次变更须在 regression_set 上不低于基线,否则阻断发布(接入 CI)。

---

## 8. 技术栈清单

| 层 | 选型 | 备注 |
|---|---|---|
| 框架基座 | Eino + eino-ext | 组件抽象 + Graph 编排 |
| 语言/服务 | Go 1.22+,Hertz/Kitex 或 net/http | 与 Eino 同属 CloudWeGo |
| 稠密向量库 | Milvus / Qdrant(自建)或 VikingDB(托管) | dense retriever |
| 关键词库 | Elasticsearch / OpenSearch | BM25 sparse retriever |
| 重排 | BGE-Reranker(本地)或 Cohere/Ark Rerank API | 接口可换 |
| 嵌入模型 | bge-m3 / OpenAI text-embedding-3 / Ark | bge-m3 同出稠密+稀疏 |
| 语义缓存 | 向量库独立 collection + Embedding | 命中直返,绕过 LLM |
| 前缀缓存 | 模型侧 KV Cache(自动/主动) | 降 TTFT 与 token 成本 |
| Agent 记忆(可选) | OpenViking | 自迭代/会话记忆层 |
| 元数据/数据集 | PostgreSQL | 数据集版本、评估结果 |
| 可观测 | OpenTelemetry + LangFuse | Eino 原生可接入 |
| 评估编排 | 自建 Go 服务 + LLM-as-Judge + Ragas/ARES | 复用 L3 Graph |


## 10. 成本最优要点

1. **不自研编排引擎**——Eino Graph 已解决类型校验、流式、并发、切面。
2. **嵌入模型选 bge-m3**——一个模型同出稠密+稀疏,省掉单独 BM25 维护。
3. **双层缓存**——语义缓存省整次推理,前缀缓存省重复前缀计算,叠加降本。
4. **重排先 API、后本地**——验证期用 API,流量上来切自建 BGE-Reranker。
5. **评估器复用线上 Graph**——离线评估调同一套 L3 编排,避免线上线下漂移。
6. **向量库分层**——主检索 VikingDB/Milvus,agent 记忆用 OpenViking。

---

## 参考资料

- Eino 官方文档:https://www.cloudwego.io/docs/eino/overview
- Eino GitHub:https://github.com/cloudwego/eino
- VikingDB Go SDK:https://github.com/volcengine/vikingdb-go-sdk
- OpenViking:https://github.com/volcengine/OpenViking
- Ragas:https://docs.ragas.io/en/latest/index.html
- ARES:https://github.com/stanford-futuredata/ARES
