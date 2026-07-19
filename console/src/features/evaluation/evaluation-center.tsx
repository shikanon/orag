import { useMemo, useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useParams } from 'react-router-dom'
import { evaluationApi, evaluationPolicyApi, knowledgeBaseApi, releaseApi, type EnvironmentKind, type EvaluationEvidence, type EvaluationMetricDefinition, type EvaluationResult } from '../../api/client'

const categoryOrder = ['answer_quality', 'retrieval', 'citation', 'diversity', 'judge', 'efficiency', 'evidence']
type MetricValue = { value: number; eligible_sample_count: number; total_sample_count: number; annotation_coverage: number; weighted_sample_count: number; effective_sample_count: number; confidence_interval?: { low: number; high: number } }
const categoryLabels: Record<string, string> = {
  answer_quality: '答案质量', retrieval: '检索质量', citation: '引用与证据', diversity: '多样性覆盖', judge: 'Judge 评审', efficiency: '效率与成本', evidence: '证据范围',
}

export function EvaluationCenter() {
  const { projectId = '' } = useParams()
  const client = useQueryClient()
  const [datasetId, setDatasetId] = useState('')
  const [datasetName, setDatasetName] = useState('')
  const [knowledgeBaseId, setKnowledgeBaseId] = useState('')
  const [sampleQuery, setSampleQuery] = useState('')
  const [groundTruth, setGroundTruth] = useState('')
  const [result, setResult] = useState<EvaluationResult | null>(null)
  const [policyName, setPolicyName] = useState('Release quality')
  const [policyThreshold, setPolicyThreshold] = useState('0.8')
  const [policyId, setPolicyId] = useState('')
  const [evidenceVersionId, setEvidenceVersionId] = useState('')
  const [evidenceEnvironment, setEvidenceEnvironment] = useState<EnvironmentKind>('development')
  const [evidence, setEvidence] = useState<EvaluationEvidence | null>(null)
  const bases = useQuery({ queryKey: ['knowledge-bases'], queryFn: knowledgeBaseApi.list })
  const metricDefinitions = useQuery({ queryKey: ['evaluation-metric-definitions'], queryFn: evaluationApi.metricDefinitions, enabled: Boolean(result) })
  const policies = useQuery({ queryKey: ['evaluation-policies', projectId], queryFn: () => evaluationPolicyApi.list(projectId), enabled: Boolean(projectId) })
  const versions = useQuery({ queryKey: ['pipeline-versions', projectId], queryFn: () => releaseApi.versions(projectId), enabled: Boolean(projectId) })
  const create = useMutation({ mutationFn: evaluationApi.createDataset, onSuccess: (dataset) => setDatasetId(dataset.id) })
  const addItem = useMutation({ mutationFn: ({ id, query, truth }: { id: string; query: string; truth: string }) => evaluationApi.addDatasetItem(id, { query, ground_truth: truth }), onSuccess: () => { setSampleQuery(''); setGroundTruth('') } })
  const run = useMutation({ mutationFn: evaluationApi.run, onSuccess: setResult })
  const createPolicy = useMutation({ mutationFn: () => evaluationPolicyApi.create(projectId, { dataset_id: datasetId, name: policyName.trim(), gates: [{ metric: 'answer_accuracy', comparator: 'gte', threshold: Number(policyThreshold) }] }), onSuccess: (policy) => { setPolicyId(policy.id); void client.invalidateQueries({ queryKey: ['evaluation-policies', projectId] }) } })
  const recordEvidence = useMutation({ mutationFn: () => evaluationPolicyApi.recordEvidence(projectId, evidenceVersionId, { policy_id: policyId, evaluation_run_id: result?.id ?? '', environment: evidenceEnvironment }), onSuccess: (item) => { setEvidence(item); void client.invalidateQueries({ queryKey: ['pipeline-versions', projectId] }); void client.invalidateQueries({ queryKey: ['release-environments', projectId] }) } })
  const projectBases = (bases.data?.items ?? []).filter((item) => !item.project_id || item.project_id === projectId)
  const validThreshold = Number.isFinite(Number(policyThreshold)) && Number(policyThreshold) >= 0

  function createDataset(event: FormEvent<HTMLFormElement>) { event.preventDefault(); if (datasetName.trim()) create.mutate({ name: datasetName.trim(), kind: 'golden', project_id: projectId }) }
  function addSample(event: FormEvent<HTMLFormElement>) { event.preventDefault(); if (datasetId && sampleQuery.trim() && groundTruth.trim()) addItem.mutate({ id: datasetId, query: sampleQuery.trim(), truth: groundTruth.trim() }) }
  function runEvaluation(event: FormEvent<HTMLFormElement>) { event.preventDefault(); if (datasetId && knowledgeBaseId) run.mutate({ dataset_id: datasetId, knowledge_base_id: knowledgeBaseId, profile: 'realtime', holdout_gate: { enabled: true, min_sample_count: 1, min_quality: 0.8 } }) }
  function submitPolicy(event: FormEvent<HTMLFormElement>) { event.preventDefault(); if (datasetId && policyName.trim() && validThreshold) createPolicy.mutate() }
  function submitEvidence(event: FormEvent<HTMLFormElement>) { event.preventDefault(); if (result?.id && policyId && evidenceVersionId) recordEvidence.mutate() }

  return <main className="content evaluation-page"><Link className="back-link" to={`/projects/${projectId}/overview`}>← 返回项目概览</Link><header className="page-header"><div><span className="eyebrow">实验性 · Evaluation gate</span><h1>评测中心</h1><p>用真实查询链路运行可复现评测，并由服务端把不可变策略和运行结果冻结为发布证据。</p></div><code className="project-pill">{projectId}</code></header><section className="evaluation-grid"><section className="evaluation-card"><h2>1. 创建数据集</h2><form onSubmit={createDataset}><label>名称<input required value={datasetName} onChange={(event) => setDatasetName(event.target.value)} placeholder="例如：客服黄金集" /></label><button className="primary-button" disabled={create.isPending}>{create.isPending ? '创建中…' : datasetId ? '已创建' : '创建数据集'}</button></form>{datasetId ? <p className="success-note">Dataset ID: <code>{datasetId}</code></p> : null}</section><section className="evaluation-card"><h2>2. 添加样本</h2><form onSubmit={addSample}><label>问题<textarea required rows={3} value={sampleQuery} onChange={(event) => setSampleQuery(event.target.value)} placeholder="评测问题" /></label><label>期望答案<textarea required rows={3} value={groundTruth} onChange={(event) => setGroundTruth(event.target.value)} placeholder="可验证的参考答案" /></label><button className="secondary-button" disabled={!datasetId || addItem.isPending}>{addItem.isPending ? '添加中…' : '添加样本'}</button></form></section><section className="evaluation-card"><h2>3. 运行评测</h2><form onSubmit={runEvaluation}><label>Knowledge Base<select required value={knowledgeBaseId} onChange={(event) => setKnowledgeBaseId(event.target.value)}><option value="">选择知识库</option>{projectBases.map((item) => <option value={item.id} key={item.id}>{item.name} · {item.id}</option>)}</select></label><button className="primary-button" disabled={!datasetId || !knowledgeBaseId || run.isPending}>{run.isPending ? '运行中…' : '运行评测'}</button></form></section><section className="evaluation-card evaluation-result"><h2>结果与可信度</h2>{!result ? <p className="muted">完成以上步骤后，评测指标、有效样本数、置信区间和 holdout gate 会显示在这里。</p> : <EvaluationResultPanel result={result} definitions={metricDefinitions.data?.items ?? []} definitionsLoading={metricDefinitions.isLoading} />}</section><section className="evaluation-card"><h2>4. 创建不可变策略</h2><p className="muted">策略只保存指标门槛；通过与否由服务端基于已存储评测结果计算。</p><form onSubmit={submitPolicy}><label>策略名称<input required value={policyName} onChange={(event) => setPolicyName(event.target.value)} /></label><label>质量阈值<input required type="number" min="0" step="0.01" value={policyThreshold} onChange={(event) => setPolicyThreshold(event.target.value)} /></label><button className="secondary-button" disabled={!datasetId || !policyName.trim() || !validThreshold || createPolicy.isPending}>{createPolicy.isPending ? '创建中…' : '创建策略'}</button></form>{policies.data?.items.length ? <p className="muted">已加载 {policies.data.items.length} 个不可变策略。</p> : null}{createPolicy.isError ? <p className="debugger-error" role="alert">策略创建失败，请确认数据集属于当前项目。</p> : null}</section><section className="evaluation-card"><h2>5. 派生环境证据</h2><p className="muted">仅提交资源 ID；服务端冻结策略、指标和版本 hash，客户端不能提交通过结果。</p><form onSubmit={submitEvidence}><label>策略<select required value={policyId} onChange={(event) => setPolicyId(event.target.value)}><option value="">选择策略</option>{policies.data?.items.map((item) => <option value={item.id} key={item.id}>{item.name} · v{item.version}</option>)}</select></label><label>冻结版本<select required value={evidenceVersionId} onChange={(event) => setEvidenceVersionId(event.target.value)}><option value="">选择版本</option>{versions.data?.items.map((item) => <option value={item.id} key={item.id}>{item.id}</option>)}</select></label><label>目标环境<select value={evidenceEnvironment} onChange={(event) => setEvidenceEnvironment(event.target.value as EnvironmentKind)}><option value="development">开发</option><option value="staging">预发</option><option value="production">生产</option></select></label><button className="primary-button" disabled={!result?.id || !policyId || !evidenceVersionId || recordEvidence.isPending}>{recordEvidence.isPending ? '冻结中…' : '派生环境证据'}</button></form>{evidence ? <p className={`success-note ${evidence.passed ? '' : 'debugger-error'}`}>Evidence {evidence.passed ? 'passed' : 'failed'} · {evidence.environment}</p> : null}{recordEvidence.isError ? <p className="debugger-error" role="alert">证据派生失败，请确认策略、评测运行和冻结版本均属于当前项目。</p> : null}</section></section></main>
}

function EvaluationResultPanel({ result, definitions, definitionsLoading }: { result: EvaluationResult; definitions: EvaluationMetricDefinition[]; definitionsLoading: boolean }) {
  const [expanded, setExpanded] = useState<Set<string>>(() => new Set())
  const definitionByName = useMemo(() => new Map(definitions.map((definition) => [definition.name, definition])), [definitions])
  const groups = useMemo(() => metricGroups(result, definitions, definitionByName), [result, definitions, definitionByName])
  const trustworthy = Object.values(result.metric_summaries ?? {}).filter((summary) => summary.eligible_sample_count >= 20 && summary.annotation_coverage >= 0.8).length
  function toggle(name: string) { setExpanded((current) => { const next = new Set(current); if (next.has(name)) next.delete(name); else next.add(name); return next }) }
  return <><div className={`gate-badge ${result.holdout_gate?.passed ? 'passed' : 'failed'}`}>{result.holdout_gate?.passed ? 'GATE PASSED' : 'GATE FAILED'}</div><div className="evaluation-evidence"><div><span>运行指纹</span><code>{shortFingerprint(result.evaluation_fingerprint)}</code></div><div><span>冻结样本</span><strong>{result.dataset_snapshot?.item_count ?? result.total}</strong></div><div><span>可信指标</span><strong>{trustworthy}/{Object.keys(result.metric_summaries ?? {}).length || '—'}</strong></div></div>{result.holdout_gate?.reasons?.length ? <p className="debugger-error">{result.holdout_gate.reasons.join(' · ')}</p> : null}{definitionsLoading ? <p className="muted">正在载入指标说明…</p> : null}<div className="metric-groups">{groups.map(([category, entries]) => <section className="metric-section" key={category}><h3>{categoryLabels[category] ?? '其他指标'}</h3>{entries.map(({ name, value, definition }) => <MetricRow key={name} name={name} value={value} definition={definition} expanded={expanded.has(name)} onToggle={toggle} />)}</section>)}</div></>
}

function metricGroups(result: EvaluationResult, definitions: EvaluationMetricDefinition[], definitionByName: Map<string, EvaluationMetricDefinition>) {
  const rawMetrics = result.metrics ?? { answer_accuracy: result.accuracy, hit_rate: result.hit_rate }
  const metricNames = Object.keys(result.metric_summaries ?? rawMetrics)
  const entries = metricNames.map((name) => ({ name, value: result.metric_summaries?.[name] ?? fallbackSummary(rawMetrics[name], result.total), definition: definitionByName.get(name) })).filter((entry) => entry.definition || entry.value)
  const groups = new Map<string, typeof entries>()
  for (const entry of entries) { const category = entry.definition?.category ?? 'other'; groups.set(category, [...(groups.get(category) ?? []), entry]) }
  return [...groups.entries()].sort(([left], [right]) => categoryOrder.indexOf(left) - categoryOrder.indexOf(right))
}

function fallbackSummary(value: number | undefined, total: number) { return value === undefined ? undefined : { value, eligible_sample_count: total, total_sample_count: total, annotation_coverage: 1, weighted_sample_count: total, effective_sample_count: total } }

function MetricRow({ name, value, definition, expanded, onToggle }: { name: string; value: MetricValue | undefined; definition?: EvaluationMetricDefinition; expanded: boolean; onToggle: (name: string) => void }) {
  if (!value) return null
  const detailID = `metric-detail-${name}`
  const title = definition?.display_name ?? name
  return <article className="metric-row"><div className="metric-row-main"><div><strong>{title}</strong><span className="metric-direction">{directionLabel(definition?.direction)}</span></div><div className="metric-value"><strong>{formatMetric(value.value)}</strong>{value.confidence_interval ? <small>95% CI {formatMetric(value.confidence_interval.low)}–{formatMetric(value.confidence_interval.high)}</small> : <small>区间待更多样本</small>}</div><div className="metric-coverage"><span>有效 {value.eligible_sample_count}/{value.total_sample_count}</span><span>标注 {formatPercent(value.annotation_coverage)}</span></div><button className="metric-explain-button" type="button" aria-expanded={expanded} aria-controls={detailID} onClick={() => onToggle(name)}>{expanded ? '收起说明' : '了解指标'}</button></div>{expanded ? <div className="metric-detail" id={detailID}><p>{definition?.description ?? '该指标来自历史运行，尚未找到服务端说明。'}</p>{definition?.formula ? <p><b>计算方式：</b>{definition.formula}</p> : null}{definition?.requires?.length ? <p><b>需要标注：</b>{definition.requires.join('、')}</p> : null}<p><b>本次证据：</b>{value.eligible_sample_count} 个有效样本，标注覆盖率 {formatPercent(value.annotation_coverage)}，有效样本量 {value.effective_sample_count.toFixed(1)}。</p>{definition?.caveats?.length ? <p><b>注意：</b>{definition.caveats.join(' ')}</p> : null}</div> : null}</article>
}

function directionLabel(direction?: string) { return direction === 'lower_is_better' ? '越低越好' : direction === 'context_dependent' ? '需结合场景' : '越高越好' }
function formatMetric(value: number) { return Number.isFinite(value) ? value.toFixed(3) : '—' }
function formatPercent(value: number) { return `${Math.round(value * 100)}%` }
function shortFingerprint(value?: string) { return value ? `${value.slice(0, 18)}…` : '历史运行未冻结' }
