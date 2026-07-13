import { useQuery } from '@tanstack/react-query'
import { Link, useParams } from 'react-router-dom'
import { ApiError, tutorialApi, type TutorialTemplate } from '../../api/client'

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
  return <main className="content tutorial-detail"><Link className="back-link" to="/tutorials">← 返回教程库</Link><header className="tutorial-detail-header"><div><p className="eyebrow">{tutorial.source_benchmark} · v{tutorial.version}</p><h1>{tutorial.title}</h1><p>{tutorial.summary}</p><div className="tutorial-statuses"><span>{tutorial.replay_available ? 'Replay 可用' : '仅 Live Run'}</span><span>预计 {tutorial.estimated_duration_minutes} 分钟</span><span>系统只读模板</span></div></div><button className="primary-button" disabled aria-label="克隆教程，即将开放">克隆教程 <small>即将开放</small></button></header><section className="tutorial-detail-section"><div className="section-heading"><p className="eyebrow">Dataset first</p><h2>逐项消融实验</h2><p>固定数据和评测口径，每次只改变一个模块，定位提升发生在哪类问题上。</p></div><ol className="pipeline-sequence">{tutorial.pipeline_stages.map((stage) => <li key={stage}><span>{stage.split(' ')[0]}</span><strong>{stage.slice(stage.indexOf(' ') + 1)}</strong></li>)}</ol></section><section className="tutorial-detail-section split-section"><div><div className="section-heading"><p className="eyebrow">Scenario slices</p><h2>重点数据维度</h2></div><div className="tutorial-tags large">{tutorial.scenario_dimensions.map((dimension) => <span key={dimension}>{dimension}</span>)}</div></div><div><div className="section-heading"><p className="eyebrow">Source</p><h2>公开基准来源</h2></div><p className="source-copy">数据包基于公开基准的可再分发精选子集。安装前仍需确认上游许可。</p><a className="text-link" href={tutorial.source_url} target="_blank" rel="noreferrer">查看 {tutorial.source_benchmark} 原始项目 ↗</a></div></section><section className="tutorial-detail-section"><div className="section-heading"><p className="eyebrow">Data packages</p><h2>选择实验规模</h2></div><div className="tutorial-pack-grid">{tutorial.packs.map((pack) => <article className="tutorial-pack" key={pack.tier}><div><p className="pack-tier">{pack.tier === 'quick' ? '快速验证' : '完整对比'}</p><h3>{pack.tier === 'quick' ? 'Quick Pack' : 'Benchmark Pack'}</h3></div><dl><div><dt>准备时间</dt><dd>约 {pack.estimated_minutes} 分钟</dd></div><div><dt>下载大小</dt><dd>{formatBytes(pack.estimated_bytes)}</dd></div><div><dt>许可确认</dt><dd>{pack.requires_license_check ? '需要' : '不需要'}</dd></div></dl><a className="text-link" href={pack.manifest_url}>查看 Manifest ↗</a></article>)}</div></section></main>
}

function formatBytes(bytes: number) {
  if (bytes >= 1024 ** 3) return `${(bytes / 1024 ** 3).toFixed(bytes % 1024 ** 3 === 0 ? 0 : 1)} GB`
  return `${Math.round(bytes / 1024 ** 2)} MB`
}
