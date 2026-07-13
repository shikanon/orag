import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { tutorialApi, type TutorialTemplate } from '../../api/client'

const modalityLabels: Record<TutorialTemplate['modality'], string> = {
  text: '文本',
  visual_document: '视觉文档',
  video: '视频',
}

export function TutorialList() {
  const query = useQuery({ queryKey: ['tutorials', 'list'], queryFn: tutorialApi.list })
  return <main className="content tutorial-library"><header className="page-header tutorial-library-header"><div><p className="eyebrow">只读模板 · 可复现实验</p><h1>教程与实验室</h1><p>在同一数据集上逐步改变一项工程策略，比较质量、成本与时延，而不是孤立测试单个模型。</p></div><span className="catalog-version">3 个端到端场景</span></header>{query.isLoading ? <TutorialSkeleton /> : query.isError ? <section className="tutorial-state" role="alert"><h2>教程目录加载失败</h2><p>无法读取只读模板。请检查 ORAG API 后重试。</p><button className="secondary-button" onClick={() => void query.refetch()}>重新加载</button></section> : query.data?.tutorials.length === 0 ? <section className="tutorial-state"><h2>暂时没有可用教程</h2><p>发布后的系统模板会显示在这里。</p></section> : <section className="tutorial-grid" aria-label="教程列表">{query.data?.tutorials.map((tutorial, index) => <TutorialRow key={tutorial.id} tutorial={tutorial} index={index + 1} />)}</section>}</main>
}

function TutorialRow({ tutorial, index }: { tutorial: TutorialTemplate, index: number }) {
  const quick = tutorial.packs.find((pack) => pack.tier === 'quick')
  const benchmark = tutorial.packs.find((pack) => pack.tier === 'benchmark')
  return <Link className="tutorial-card" to={`/tutorials/${tutorial.id}`}><span className="tutorial-index">0{index}</span><div className="tutorial-card-main"><div className="tutorial-meta"><span>{modalityLabels[tutorial.modality]}</span><span>{tutorial.source_benchmark}</span><span>{tutorial.estimated_duration_minutes} 分钟</span></div><h2>{tutorial.title}</h2><p>{tutorial.summary}</p><div className="tutorial-tags">{tutorial.scenario_dimensions.slice(0, 5).map((dimension) => <span key={dimension}>{dimension}</span>)}</div></div><div className="tutorial-card-facts"><span><strong>{tutorial.pipeline_stages.length}</strong> 个渐进阶段</span><span>Quick {quick?.estimated_minutes ?? 0} 分钟</span><span>Benchmark {benchmark?.estimated_minutes ?? 0} 分钟</span><span className="tutorial-open">查看实验设计 <b aria-hidden>→</b></span></div></Link>
}

function TutorialSkeleton() {
  return <section className="tutorial-grid" aria-label="正在加载教程" aria-busy="true">{[0, 1, 2].map((row) => <div className="tutorial-card tutorial-card-skeleton" key={row}><span className="skeleton-line short" /><div><span className="skeleton-line" /><span className="skeleton-line short" /></div></div>)}</section>
}
