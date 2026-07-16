import { useMutation } from '@tanstack/react-query'
import { useState } from 'react'
import type { FormEvent } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { ApiError, tutorialApi, type StartTutorialCloneInput, type TutorialTemplate } from '../../api/client'

export function TutorialDetail() {
  const { templateId = '' } = useParams()
  const query = useQuery({ queryKey: ['tutorials', 'detail', templateId], queryFn: () => tutorialApi.get(templateId), enabled: templateId.length > 0 })
  if (query.isLoading) return <main className="content tutorial-detail" aria-label="正在加载教程" aria-busy="true"><div className="skeleton-line short" /><div className="skeleton-line" /></main>
  if (query.isError) {
    const missing = query.error instanceof ApiError && query.error.status === 404
    return <main className="content tutorial-detail"><section className="tutorial-state" role="alert"><h1>{missing ? '教程不存在' : '教程加载失败'}</h1><p>{missing ? '该模板可能已更名，或尚未发布。' : '无法读取教程详情，请稍后再试。'}</p><Link className="secondary-button" to="/tutorials">返回教程库</Link></section></main>
  }
  return query.data ? <TutorialContent tutorial={query.data} /> : null
}

function TutorialContent({ tutorial }: { tutorial: TutorialTemplate }) {
  const [cloneOpen, setCloneOpen] = useState(false)
  return <main className="content tutorial-detail">
    <Link className="back-link" to="/tutorials">← 返回教程库</Link>
    <header className="tutorial-detail-header">
      <div><p className="eyebrow">{tutorial.source_benchmark} · v{tutorial.version}</p><h1>{tutorial.title}</h1><p>{tutorial.summary}</p><div className="tutorial-statuses"><span>{tutorial.replay_available ? 'Replay 结果即将开放' : '仅 Live Run'}</span><span>预计 {tutorial.estimated_duration_minutes} 分钟</span><span>系统只读模板</span></div></div>
      <button className="primary-button" type="button" onClick={() => setCloneOpen(true)}>克隆教程</button>
    </header>
    <section className="tutorial-detail-section"><div className="section-heading"><p className="eyebrow">Dataset first</p><h2>逐项消融实验</h2><p>固定数据和评测口径，每次只改变一个模块，定位提升发生在哪类问题上。</p></div><ol className="pipeline-sequence">{tutorial.pipeline_stages.map((stage) => <li key={stage}><span>{stage.split(' ')[0]}</span><strong>{stage.slice(stage.indexOf(' ') + 1)}</strong></li>)}</ol></section>
    <section className="tutorial-detail-section split-section"><div><div className="section-heading"><p className="eyebrow">Scenario slices</p><h2>重点数据维度</h2></div><div className="tutorial-tags large">{tutorial.scenario_dimensions.map((dimension) => <span key={dimension}>{dimension}</span>)}</div></div><div><div className="section-heading"><p className="eyebrow">Source</p><h2>公开基准来源</h2></div><p className="source-copy">数据包基于公开基准的可再分发精选子集。安装前仍需确认上游许可。</p><a className="text-link" href={tutorial.source_url} target="_blank" rel="noreferrer">查看 {tutorial.source_benchmark} 原始项目 ↗</a></div></section>
    <section className="tutorial-detail-section"><div className="section-heading"><p className="eyebrow">Data packages</p><h2>选择实验规模</h2></div><div className="tutorial-pack-grid">{tutorial.packs.map((pack) => <article className="tutorial-pack" key={pack.tier}><div><p className="pack-tier">{pack.tier === 'quick' ? '快速验证' : '完整对比'}</p><h3>{pack.tier === 'quick' ? 'Quick Pack' : 'Benchmark Pack'}</h3></div><dl><div><dt>准备时间</dt><dd>约 {pack.estimated_minutes} 分钟</dd></div><div><dt>下载大小</dt><dd>{formatBytes(pack.estimated_bytes)}</dd></div><div><dt>许可确认</dt><dd>{pack.requires_license_check ? '需要' : '不需要'}</dd></div></dl><p className="pack-note">由 ORAG 服务端校验并写入项目私有存储。</p></article>)}</div></section>
    {cloneOpen ? <CloneTutorialDialog tutorial={tutorial} onClose={() => setCloneOpen(false)} /> : null}
  </main>
}

function CloneTutorialDialog({ tutorial, onClose }: { tutorial: TutorialTemplate; onClose: () => void }) {
  const navigate = useNavigate()
  const [tier, setTier] = useState<'quick' | 'benchmark' | ''>('')
  const [licenseAccepted, setLicenseAccepted] = useState(false)
  const [projectName, setProjectName] = useState(`${tutorial.title} 实验`)
  const [description, setDescription] = useState(`基于 ${tutorial.source_benchmark} 的 ${tutorial.title} 可复现实验`)
  const [idempotencyKey, setIdempotencyKey] = useState('')
  const selectedPack = tutorial.packs.find((pack) => pack.tier === tier)
  const mutation = useMutation({
    mutationFn: (input: StartTutorialCloneInput) => tutorialApi.startClone(tutorial.id, input),
    onSuccess: (accepted) => navigate(`/projects/${accepted.project_id}/tutorial/setup?job=${encodeURIComponent(accepted.job_id)}`),
  })
  const submit = (event: FormEvent) => {
    event.preventDefault()
    if (!tier || !selectedPack || !projectName.trim() || (selectedPack.requires_license_check && !licenseAccepted)) return
    const key = idempotencyKey || newIdempotencyKey()
    if (!idempotencyKey) setIdempotencyKey(key)
    mutation.mutate({ version: tutorial.version, pack_tier: tier, project: { name: projectName.trim(), description: description.trim() }, idempotency_key: key, license_accepted: licenseAccepted })
  }
  const ready = !!tier && !!projectName.trim() && (!selectedPack?.requires_license_check || licenseAccepted)
  return <div className="dialog-backdrop" role="presentation"><section className="tutorial-clone-dialog" role="dialog" aria-modal="true" aria-labelledby="tutorial-clone-title"><header><div><p className="eyebrow">创建实验项目</p><h2 id="tutorial-clone-title">克隆 {tutorial.title}</h2></div><button className="icon-button" type="button" aria-label="关闭" disabled={mutation.isPending} onClick={onClose}>×</button></header><form onSubmit={submit}><fieldset><legend>选择数据包</legend>{tutorial.packs.map((pack) => <label className={`tutorial-pack-choice ${tier === pack.tier ? 'selected' : ''}`} key={pack.tier}><input type="radio" name="pack-tier" value={pack.tier} checked={tier === pack.tier} onChange={() => { setTier(pack.tier); setLicenseAccepted(false) }} /><span><strong>{pack.tier === 'quick' ? 'Quick Pack' : 'Benchmark Pack'}</strong><small>{formatBytes(pack.estimated_bytes)} · 约 {pack.estimated_minutes} 分钟</small></span></label>)}</fieldset><label>项目名称<input autoFocus required value={projectName} onChange={(event) => setProjectName(event.target.value)} maxLength={120} /></label><label>项目说明 <span className="optional">可选</span><textarea value={description} onChange={(event) => setDescription(event.target.value)} maxLength={2000} rows={3} /></label>{selectedPack?.requires_license_check ? <label className="tutorial-license"><input type="checkbox" checked={licenseAccepted} onChange={(event) => setLicenseAccepted(event.target.checked)} />我已确认数据许可</label> : null}{mutation.isError ? <p role="alert">创建实验项目失败。请检查权限或稍后重试；已保留本次请求以便安全重试。</p> : null}<p className="tutorial-dialog-note">创建后由服务端异步验证数据包并写入项目私有存储。页面不会显示访问密钥、对象地址或 Manifest 细节。</p><div className="dialog-actions"><button className="secondary-button" type="button" disabled={mutation.isPending} onClick={onClose}>取消</button><button className="primary-button" disabled={!ready || mutation.isPending}>{mutation.isPending ? '创建中…' : '创建实验项目'}</button></div></form></section></div>
}

function newIdempotencyKey() {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') return crypto.randomUUID()
  return `console-${Date.now()}-${Math.random().toString(36).slice(2)}`
}

function formatBytes(bytes: number) {
  if (bytes >= 1024 ** 3) return `${(bytes / 1024 ** 3).toFixed(bytes % 1024 ** 3 === 0 ? 0 : 1)} GB`
  return `${Math.round(bytes / 1024 ** 2)} MB`
}
