import { useState, type FormEvent } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { Link, useParams } from 'react-router-dom'
import { evaluationApi, knowledgeBaseApi, type EvaluationResult } from '../../api/client'

export function EvaluationCenter() {
  const { projectId = '' } = useParams()
  const [datasetId, setDatasetId] = useState('')
  const [datasetName, setDatasetName] = useState('')
  const [knowledgeBaseId, setKnowledgeBaseId] = useState('')
  const [sampleQuery, setSampleQuery] = useState('')
  const [groundTruth, setGroundTruth] = useState('')
  const [result, setResult] = useState<EvaluationResult | null>(null)
  const bases = useQuery({ queryKey: ['knowledge-bases'], queryFn: knowledgeBaseApi.list })
  const create = useMutation({ mutationFn: evaluationApi.createDataset, onSuccess: (dataset) => setDatasetId(dataset.id) })
  const addItem = useMutation({ mutationFn: ({ id, query, truth }: { id: string; query: string; truth: string }) => evaluationApi.addDatasetItem(id, { query, ground_truth: truth }), onSuccess: () => { setSampleQuery(''); setGroundTruth('') } })
  const run = useMutation({ mutationFn: evaluationApi.run, onSuccess: setResult })
  const projectBases = (bases.data?.items ?? []).filter((item) => !item.project_id || item.project_id === projectId)

  function createDataset(event: FormEvent<HTMLFormElement>) { event.preventDefault(); if (datasetName.trim()) create.mutate({ name: datasetName.trim(), kind: 'golden', project_id: projectId }) }
  function addSample(event: FormEvent<HTMLFormElement>) { event.preventDefault(); if (datasetId && sampleQuery.trim() && groundTruth.trim()) addItem.mutate({ id: datasetId, query: sampleQuery.trim(), truth: groundTruth.trim() }) }
  function runEvaluation(event: FormEvent<HTMLFormElement>) { event.preventDefault(); if (datasetId && knowledgeBaseId) run.mutate({ dataset_id: datasetId, knowledge_base_id: knowledgeBaseId, profile: 'realtime', holdout_gate: { enabled: true, min_sample_count: 1, min_quality: 0.8 } }) }

  return <main className="content evaluation-page"><Link className="back-link" to={`/projects/${projectId}/overview`}>← 返回项目概览</Link><header className="page-header"><div><span className="eyebrow">实验性 · Evaluation gate</span><h1>评测中心</h1><p>用真实查询链路运行可复现评测，并在发布前检查质量门禁。</p></div><code className="project-pill">{projectId}</code></header><section className="evaluation-grid"><section className="evaluation-card"><h2>1. 创建数据集</h2><form onSubmit={createDataset}><label>名称<input required value={datasetName} onChange={(event) => setDatasetName(event.target.value)} placeholder="例如：客服黄金集" /></label><button className="primary-button" disabled={create.isPending}>{create.isPending ? '创建中…' : datasetId ? '已创建' : '创建数据集'}</button></form>{datasetId ? <p className="success-note">Dataset ID: <code>{datasetId}</code></p> : null}</section><section className="evaluation-card"><h2>2. 添加样本</h2><form onSubmit={addSample}><label>问题<textarea required rows={3} value={sampleQuery} onChange={(event) => setSampleQuery(event.target.value)} placeholder="评测问题" /></label><label>期望答案<textarea required rows={3} value={groundTruth} onChange={(event) => setGroundTruth(event.target.value)} placeholder="可验证的参考答案" /></label><button className="secondary-button" disabled={!datasetId || addItem.isPending}>{addItem.isPending ? '添加中…' : '添加样本'}</button></form></section><section className="evaluation-card"><h2>3. 运行评测</h2><form onSubmit={runEvaluation}><label>Knowledge Base<select required value={knowledgeBaseId} onChange={(event) => setKnowledgeBaseId(event.target.value)}><option value="">选择知识库</option>{projectBases.map((item) => <option value={item.id} key={item.id}>{item.name} · {item.id}</option>)}</select></label><button className="primary-button" disabled={!datasetId || !knowledgeBaseId || run.isPending}>{run.isPending ? '运行中…' : '运行评测'}</button></form></section><section className="evaluation-card evaluation-result"><h2>结果与门禁</h2>{!result ? <p className="muted">完成以上步骤后，评测指标和 holdout gate 会显示在这里。</p> : <><div className={`gate-badge ${result.holdout_gate?.passed ? 'passed' : 'failed'}`}>{result.holdout_gate?.passed ? 'GATE PASSED' : 'GATE FAILED'}</div><div className="metric-grid"><Metric label="Accuracy" value={result.accuracy} /><Metric label="Hit rate" value={result.hit_rate} /><Metric label="Samples" value={result.total} /><Metric label="Run ID" value={result.id} /></div>{result.holdout_gate?.reasons?.length ? <p className="debugger-error">{result.holdout_gate.reasons.join(' · ')}</p> : null}</>}</section></section></main>
}

function Metric({ label, value }: { label: string; value: number | string }) { return <div><span>{label}</span><strong>{typeof value === 'number' ? value.toFixed(3) : value}</strong></div> }
