package eval

import "strings"

type metricMetadata struct {
	DisplayName string
	Category    string
	Direction   string
	Formula     string
	Requires    []string
	Caveats     []string
	Related     []string
	Hidden      bool
}

var defaultMetricMetadata = map[string]metricMetadata{
	PrimaryMetricDeterministicAnswerMatch: {"Deterministic answer match", "answer_quality", "higher_is_better", "命中 reference answer 中长度大于 3 的任一关键项", []string{"ground_truth"}, []string{"只适合基础回归，不识别同义改写、复杂推理或多语种细粒度表达。"}, []string{"answer_accuracy"}, false},
	"answer_accuracy":                     {"Answer accuracy", "answer_quality", "higher_is_better", "deterministic answer match 的兼容别名", []string{"ground_truth"}, []string{"当前不是语义等价判断。"}, []string{PrimaryMetricDeterministicAnswerMatch}, false},
	"accuracy":                            {"Accuracy", "answer_quality", "higher_is_better", "answer_accuracy 的兼容别名", []string{"ground_truth"}, []string{"优先使用 answer_accuracy。"}, []string{"answer_accuracy"}, true},
	"hit_rate":                            {"Hit rate", "answer_quality", "higher_is_better", "answer_accuracy 的兼容别名", []string{"ground_truth"}, []string{"优先使用 answer_accuracy。"}, []string{"answer_accuracy"}, true},
	PrimaryMetricPairwiseAccuracy:         {"Pairwise non-loss rate", "answer_quality", "higher_is_better", "候选回答在成对评审中获胜或平局的比例", []string{"pairwise_judge"}, []string{"必须结合稳定性和具体基线解释。"}, []string{"faithfulness", "answer_accuracy"}, false},
	"pairwise_stability_rate":             {"Pairwise stability", "answer_quality", "higher_is_better", "顺序交换后仍一致的成对评审结果比例", []string{"pairwise_judge", "pairwise_swap"}, []string{"不稳定结果不会计入 pairwise_accuracy 主分。"}, []string{PrimaryMetricPairwiseAccuracy}, false},
	"citation_hit_rate":                   {"Citation hit rate", "citation", "higher_is_better", "返回至少一条引用的样本比例", nil, []string{"有引用不代表引用一定相关或足以支撑回答。"}, []string{"citation_precision", "citation_support"}, false},
	"context_recall":                      {"Context recall", "retrieval", "higher_is_better", "召回的相关文档数 / 标注相关文档数", []string{"relevant_doc_ids"}, []string{"依赖文档级标注，不验证 chunk 是否直接支撑回答。"}, []string{"recall_at_k", "coverage"}, false},
	"citation_precision":                  {"Citation precision", "citation", "higher_is_better", "指向标注相关文档的引用数 / 全部引用数", []string{"relevant_doc_ids"}, []string{"不验证引用位置与具体论断的一致性。"}, []string{"citation_hit_rate", "citation_support"}, false},
	"ndcg_at_k":                           {"NDCG@K", "retrieval", "higher_is_better", "DCG@K / IDCG@K，相关文档越靠前得分越高", []string{"relevant_doc_ids"}, []string{"当前是文档级二元相关性。"}, []string{"recall_at_k", "mrr", "map"}, false},
	"recall_at_k":                         {"Recall@K", "retrieval", "higher_is_better", "前 K 个结果覆盖的相关文档比例", []string{"relevant_doc_ids"}, []string{"不反映相关文档的具体排序位置。"}, []string{"ndcg_at_k", "coverage"}, false},
	"mrr":                                 {"MRR", "retrieval", "higher_is_better", "第一个相关文档排名的倒数", []string{"relevant_doc_ids"}, []string{"只关注第一个相关结果。"}, []string{"ndcg_at_k", "map"}, false},
	"map":                                 {"MAP", "retrieval", "higher_is_better", "相关文档命中位置 precision 的平均值", []string{"relevant_doc_ids"}, []string{"需要多个相关文档时信息更充分。"}, []string{"ndcg_at_k", "mrr"}, false},
	"coverage":                            {"Retrieval coverage", "retrieval", "higher_is_better", "至少召回一个相关文档的样本比例", []string{"relevant_doc_ids"}, []string{"不衡量全部相关文档是否被召回。"}, []string{"retrieval_failure_rate", "recall_at_k"}, false},
	"retrieval_failure_rate":              {"Retrieval failure rate", "retrieval", "lower_is_better", "未召回任一相关文档的样本比例", []string{"relevant_doc_ids"}, []string{"需要与标注覆盖率一起查看。"}, []string{"coverage", "recall_at_k"}, false},
	"redundancy_rate":                     {"Redundancy rate", "retrieval", "lower_is_better", "重复 chunk 数 / 召回结果数", nil, []string{"重复判定优先使用 chunk ID，其次使用 hash 或规范化文本。"}, []string{"duplicate_count", "deduped_top_k_count"}, false},
	"duplicate_count":                     {"Duplicate count", "retrieval", "lower_is_better", "召回结果中的重复 chunk 数", nil, nil, []string{"redundancy_rate"}, false},
	"deduped_top_k_count":                 {"Deduped top-K count", "retrieval", "higher_is_better", "去重后的召回结果数量", nil, []string{"仅反映数量，不代表质量。"}, []string{"redundancy_rate"}, false},
	"alpha_ndcg":                          {"α-NDCG", "diversity", "higher_is_better", "对重复覆盖相同 aspect 的增益进行衰减的 NDCG", []string{"diversity_annotations"}, []string{"依赖 aspect 或 subquestion 标注。"}, []string{"aspect_coverage", "ndcg_at_k"}, false},
	"aspect_coverage":                     {"Aspect coverage", "diversity", "higher_is_better", "已召回证据覆盖的标注 aspect 比例", []string{"diversity_annotations"}, []string{"不衡量 aspect 内证据质量。"}, []string{"alpha_ndcg"}, false},
	"faithfulness":                        {"Faithfulness", "judge", "higher_is_better", "Judge 对回答论断是否受证据支持的评分", []string{"llm_judge"}, []string{"自动晋级前需要与人工 gold set 校准。"}, []string{"groundedness", "hallucination"}, false},
	"groundedness":                        {"Groundedness", "judge", "higher_is_better", "Judge 对回答是否基于检索上下文的评分", []string{"llm_judge"}, []string{"Judge 配置变化时不可直接比较。"}, []string{"faithfulness", "citation_support"}, false},
	"answer_relevance":                    {"Answer relevance", "judge", "higher_is_better", "Judge 对回答是否直接回应问题的评分", []string{"llm_judge"}, []string{"不等同于事实正确性。"}, []string{"completeness"}, false},
	"hallucination":                       {"Hallucination", "judge", "lower_is_better", "Judge 识别出的不受证据支持或虚构内容程度", []string{"llm_judge"}, []string{"需要与 faithfulness 一起、并经过校准后使用。"}, []string{"faithfulness", "groundedness"}, false},
	"completeness":                        {"Completeness", "judge", "higher_is_better", "Judge 对必需回答内容覆盖程度的评分", []string{"llm_judge"}, []string{"依赖 rubric 和 ground truth 的完整性。"}, []string{"answer_relevance"}, false},
	"citation_support":                    {"Citation support", "citation", "higher_is_better", "Judge 对引用是否支撑回答论断的评分", []string{"llm_judge", "citations"}, []string{"比 citation_hit_rate 更接近引用质量。"}, []string{"citation_precision", "faithfulness"}, false},
	"qag_score":                           {"QAG score", "judge", "higher_is_better", "逐论断验证后被证据支持的比例", []string{"qag"}, []string{"需要检查 unverifiable rate。"}, []string{"qag_claim_coverage", "qag_unverifiable_rate"}, false},
	"qag_claim_coverage":                  {"QAG claim coverage", "judge", "higher_is_better", "expected evidence 被 QAG claims 覆盖的比例", []string{"expected_evidence", "qag"}, []string{"依赖 expected evidence 标注。"}, []string{"qag_score"}, false},
	"qag_question_count":                  {"QAG question count", "judge", "higher_is_better", "QAG 生成并验证的问题数", []string{"qag"}, []string{"数量本身不代表质量。"}, []string{"qag_score"}, false},
	"qag_unverifiable_rate":               {"QAG unverifiable rate", "judge", "lower_is_better", "无法从上下文验证的 claim 比例", []string{"qag"}, []string{"过高可能表示证据不足或 Judge 不能判定。"}, []string{"qag_score"}, false},
	"instruction_following":               {"Instruction following", "judge", "higher_is_better", "Judge 对回答是否遵循任务指令的评分", []string{"llm_judge"}, []string{"依赖 rubric 定义。"}, nil, false},
	"safety":                              {"Safety", "judge", "higher_is_better", "Judge 对安全策略合规性的评分", []string{"llm_judge"}, []string{"应与专门安全评测集结合使用。"}, nil, false},
	"latency_ms":                          {"Mean latency", "efficiency", "lower_is_better", "符合资格查询的平均端到端延迟", nil, []string{"请结合 P95 和 cache 场景查看。"}, []string{"latency_p95_ms", "cache_hit_rate"}, false},
	"latency_p95_ms":                      {"P95 latency", "efficiency", "lower_is_better", "95% 查询在该延迟内完成", nil, []string{"小样本 P95 波动较大。"}, []string{"latency_ms", "cache_hit_rate"}, false},
	"cache_hit":                           {"Cache hit", "efficiency", "higher_is_better", "单样本是否命中缓存", nil, []string{"通常作为诊断信息。"}, []string{"cache_hit_rate"}, true},
	"cache_hit_rate":                      {"Cache hit rate", "efficiency", "context_dependent", "命中语义缓存的查询比例", nil, []string{"cache 状态不同会影响延迟和成本的可比性。"}, []string{"latency_p95_ms"}, false},
	"cost_usd":                            {"Evaluation cost", "efficiency", "lower_is_better", "本次评估真实模型调用成本总和", nil, []string{"不受业务样本权重影响。"}, []string{"total_tokens"}, false},
	"prompt_tokens":                       {"Prompt tokens", "efficiency", "lower_is_better", "本次评估实际输入 token 总数", nil, nil, []string{"total_tokens"}, false},
	"completion_tokens":                   {"Completion tokens", "efficiency", "lower_is_better", "本次评估实际输出 token 总数", nil, nil, []string{"total_tokens"}, false},
	"total_tokens":                        {"Total tokens", "efficiency", "lower_is_better", "本次评估实际 token 总数", nil, nil, []string{"cost_usd"}, false},
	"weighted_sample_count":               {"Weighted sample count", "evidence", "higher_is_better", "参与运行的样本权重之和", nil, []string{"不等同于有效样本数。"}, []string{"unweighted_sample_count"}, false},
	"unweighted_sample_count":             {"Sample count", "evidence", "higher_is_better", "参与运行的样本数量", nil, []string{"不同指标可能因缺少标注而有更小的有效样本数。"}, []string{"weighted_sample_count"}, false},
	"missing_split":                       {"Missing split", "evidence", "lower_is_better", "请求的 dataset split 是否为空或不存在", nil, []string{"生产门禁应视为失败。"}, nil, true},
}

func enrichMetricDefinition(def MetricDefinition) MetricDefinition {
	metadata, ok := defaultMetricMetadata[def.Name]
	if !ok {
		return def
	}
	if metadata.DisplayName != "" {
		def.DisplayName = metadata.DisplayName
	}
	def.Category = metadata.Category
	def.Direction = metadata.Direction
	def.Formula = metadata.Formula
	def.Requires = append([]string(nil), metadata.Requires...)
	def.Caveats = append([]string(nil), metadata.Caveats...)
	def.Related = append([]string(nil), metadata.Related...)
	def.Hidden = metadata.Hidden
	return def
}

func fallbackDisplayName(name string) string {
	return strings.ReplaceAll(name, "_", " ")
}
