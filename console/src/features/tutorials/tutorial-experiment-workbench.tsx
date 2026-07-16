import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useParams } from 'react-router-dom'
import { ApiError, tutorialApi, type TutorialExperimentRun, type TutorialExperimentRunComparison, type TutorialExperimentVariant } from '../../api/client'

const stageLabel: Record<TutorialExperimentRun['stage'], string> = {
  index_private_pack: '正在从私有 Pack 构建独立索引',
  run_evaluation: '正在运行标准评测',
  completed: '标准评测已完成',
}

const statusLabels: Record<TutorialExperimentRun['status'], string> = {
  queued: '已排队', running: '进行中', cancel_requested: '正在取消', cancelled: '已取消', failed: '需要处理', completed: '已完成',
}

export function TutorialExperimentWorkbench() {
  const { projectId = '', experimentId = '' } = useParams()
  const queryClient = useQueryClient()
  const experiment = useQuery({ queryKey: ['tutorial-experiment', projectId], queryFn: () => tutorialApi.getExperiment(projectId), enabled: projectId.length > 0 })
  const start = useMutation({
    mutationFn: (variant: string) => tutorialApi.startLiveRun(projectId, experimentId, { variant, idempotency_key: newIdempotencyKey(variant) }),
    onSuccess: (accepted) => queryClient.setQueryData(['tutorial-live-run', projectId, experimentId, accepted.run_id], accepted.run),
  })
  const runId = start.data?.run_id ?? ''
  const run = useQuery({
    queryKey: ['tutorial-live-run', projectId, experimentId, runId],
    queryFn: () => tutorialApi.getLiveRun(projectId, experimentId, runId),
    enabled: runId.length > 0,
    refetchInterval: (query) => terminal(query.state.data?.status) ? false : 1000,
  })
  const cancel = useMutation({
    mutationFn: () => tutorialApi.cancelLiveRun(projectId, experimentId, runId),
    onSuccess: (updated) => queryClient.setQueryData(['tutorial-live-run', projectId, experimentId, runId], updated),
  })
  const comparison = useQuery({
    queryKey: ['tutorial-live-run-comparison', projectId, experimentId, runId],
    queryFn: () => tutorialApi.getLiveRunComparison(projectId, experimentId, runId),
    enabled: run.data?.variant !== 'baseline' && run.data?.status === 'completed',
  })

  if (experiment.isLoading) return <main className="content tutorial-workbench" aria-label="正在读取教程实验" aria-busy="true"><div className="skeleton-line short" /><div className="skeleton-line" /></main>
  if (experiment.isError || !experiment.data || experiment.data.id !== experimentId) return <main className="content tutorial-workbench"><section className="tutorial-state" role="alert"><h1>教程实验不存在</h1><Link className="secondary-button" to="/tutorials">返回教程库</Link></section></main>

  const ready = experiment.data.pack_status === 'pack_installed' && experiment.data.runtime_status === 'ready'
  const variants = experiment.data.variants
  const baseline = variants.find((variant) => variant.id === 'baseline')
  const candidates = variants.filter((variant) => variant.id !== 'baseline')
  const active = !!run.data && !terminal(run.data.status)
  const startError = start.error instanceof ApiError ? start.error : null

  return <main className="content tutorial-workbench">
    <Link className="back-link" to={`/projects/${projectId}/tutorial/setup`}>← 返回安装进度</Link>
    <header className="page-header">
      <div><p className="eyebrow">P0 → P1 Live Run</p><h1>文本 Quick 解析候选</h1><p>每次运行只从项目私有 Pack 读取已校验内容。P1 仅替换声明的 JSON 解析器，并使用独立索引；模型、评测集和检索配置均由服务端固定。</p></div>
      <span className={`tutorial-job-status ${run.data?.status ?? (ready ? 'completed' : 'failed')}`}>{run.data ? statusText(run.data.status) : ready ? '可以运行' : '不可运行'}</span>
    </header>
    <section className="tutorial-workbench-grid">
      <section className="tutorial-progress-card">
        <p className="eyebrow">不可变实验阶梯</p>
        <h2>P0 基线与 P1 候选</h2>
        <dl className="tutorial-run-facts"><div><dt>评测集</dt><dd><code>{experiment.data.dataset_id || '未创建'}</code></dd></div><div><dt>固定 Top-K</dt><dd>{experiment.data.baseline_top_k ?? '—'}</dd></div></dl>
        {!ready ? <p className="tutorial-failure" role="alert">当前 Pack 没有可运行的文本运行声明。它不能被解释为已评测结果。</p> : <>
          {baseline ? <VariantCard variant={baseline} disabled={start.isPending || active || !baseline.available} pending={start.isPending && start.variables === baseline.id} onStart={start.mutate} /> : null}
          {candidates.length > 0 ? candidates.map((variant) => <VariantCard key={variant.id} variant={variant} disabled={start.isPending || active || !variant.available} pending={start.isPending && start.variables === variant.id} onStart={start.mutate} />) : <p className="muted">此不可变 Pack 未声明 P1 解析候选。</p>}
          {candidates.length > 0 ? <p className="tutorial-candidate-note">P1 提交时由服务器检查是否已有同一 Pack、模型指纹和评测设置下完成的 P0；不满足条件不会生成可比结果。</p> : null}
        </>}
        {start.isError ? <p className="debugger-error" role="alert">{startError?.status === 409 && start.variables !== 'baseline' ? 'P1 需要已完成且兼容的 P0 基线；请先运行基线或保持运行环境不变。' : '运行未被接受。请确认 Pack、项目权限和模型运行环境。'}</p> : null}
      </section>
      <RunProgress run={run.data} active={active} cancelPending={cancel.isPending} onCancel={cancel.mutate} />
    </section>
    {comparison.isSuccess ? <ComparisonPanel comparison={comparison.data} /> : null}
    {comparison.isError ? <section className="tutorial-progress-card tutorial-comparison" role="alert"><h2>暂时无法形成可比结果</h2><p>该候选缺少服务端可验证的 P0 父运行或标准评测记录，因此不会显示推断出的提升。</p></section> : null}
  </main>
}

function VariantCard({ variant, disabled, pending, onStart }: { variant: TutorialExperimentVariant; disabled: boolean; pending: boolean; onStart: (variant: string) => void }) {
  const baseline = variant.id === 'baseline'
  return <article className={`tutorial-variant-card ${baseline ? 'baseline' : 'candidate'}`}>
    <p className="eyebrow">{baseline ? 'P0 · 固定基线' : `${variant.chapter || 'P1'} · 单变量候选`}</p>
    <h3>{baseline ? 'Basic parser' : 'Structured JSON parser'}</h3>
    <p>解析器：<code>{variant.parser_method}</code></p>
    {!baseline ? <p>独立 Knowledge Base；仅改变 JSON 文档到 Markdown 的确定性结构化解析。</p> : <p>服务端固定使用 Basic parser，作为唯一可比较的 P0 父运行。</p>}
    <button className={baseline ? 'primary-button' : 'secondary-button'} disabled={disabled} onClick={() => onStart(variant.id)}>{pending ? '正在提交…' : baseline ? '运行 P0 基线' : '运行 P1 解析候选'}</button>
  </article>
}

function RunProgress({ run, active, cancelPending, onCancel }: { run?: TutorialExperimentRun; active: boolean; cancelPending: boolean; onCancel: () => void }) {
  return <section className="tutorial-progress-card">
    <p className="eyebrow">运行进度</p>
    <h2>{run ? stageLabel[run.stage] : '尚未开始'}</h2>
    {run ? <>
      <p>变体：<code>{run.variant}</code> · 状态：{statusText(run.status)}</p>
      <ol className="tutorial-stage-list"><li className={run.stage === 'index_private_pack' ? 'active' : 'done'}><span>构建独立索引</span><small>{run.stage === 'index_private_pack' ? statusText(run.status) : '完成'}</small></li><li className={run.stage === 'run_evaluation' ? 'active' : run.stage === 'completed' ? 'done' : 'pending'}><span>执行标准评测</span><small>{run.stage === 'run_evaluation' ? statusText(run.status) : run.stage === 'completed' ? '完成' : '等待'}</small></li></ol>
      {run.status === 'failed' ? <div className="tutorial-failure" role="alert">服务端失败代码：{run.failure_code || 'live_run_failed'}</div> : null}
      <RunAudit run={run} />
      {active ? <button className="secondary-button" disabled={cancelPending} onClick={onCancel}>{cancelPending ? '正在取消…' : '取消运行'}</button> : null}
    </> : <p className="muted">选择 P0 或由 Pack 声明的 P1 后，会显示私有 Pack 索引和标准评测阶段。</p>}
  </section>
}

function RunAudit({ run }: { run: TutorialExperimentRun }) {
  return <dl className="tutorial-run-facts tutorial-run-audit">
    <div><dt>标准评测 Run</dt><dd><code>{run.evaluation_run_id || '—'}</code></dd></div>
    <div><dt>解析器</dt><dd><code>{run.parser_method || '—'}</code></dd></div>
    {run.baseline_run_id ? <div><dt>P0 父运行</dt><dd><code>{run.baseline_run_id}</code></dd></div> : null}
    {run.comparison_fingerprint ? <div><dt>比较指纹</dt><dd><code>{shortFingerprint(run.comparison_fingerprint)}</code></dd></div> : null}
  </dl>
}

function ComparisonPanel({ comparison }: { comparison: TutorialExperimentRunComparison }) {
  return <section className="tutorial-progress-card tutorial-comparison">
    <p className="eyebrow">可审计对比</p>
    <h2>{comparison.comparable ? 'P0 与 P1 使用相同的对比输入' : '对比输入不兼容'}</h2>
    <p>基线 <code>{comparison.baseline.id}</code> · 候选 <code>{comparison.candidate.id}</code></p>
    {comparison.comparable ? <div className="tutorial-comparison-table" role="region" aria-label="P0 P1 指标对比"><table><thead><tr><th>指标</th><th>P0</th><th>P1</th><th>绝对变化</th><th>相对变化</th></tr></thead><tbody>{(comparison.metrics || []).map((metric) => <tr key={metric.name}><td><code>{metric.name}</code></td><td>{formatMetric(metric.baseline)}</td><td>{formatMetric(metric.candidate)}</td><td>{formatDelta(metric.absolute_delta)}</td><td>{metric.relative_delta == null ? '—' : formatPercent(metric.relative_delta)}</td></tr>)}</tbody></table></div> : <p className="tutorial-failure">缺少可验证的同一运行环境、P0 父运行或标准评测记录；不会推断模块增益。</p>}
  </section>
}

function terminal(status?: TutorialExperimentRun['status']) { return status === 'cancelled' || status === 'failed' || status === 'completed' }
function statusText(status: TutorialExperimentRun['status']) { return statusLabels[status] }
function newIdempotencyKey(variant: string) { return `tutorial-${variant}-${Date.now()}-${Math.random().toString(36).slice(2)}` }
function shortFingerprint(value: string) { return value.length > 16 ? `${value.slice(0, 16)}…` : value }
function formatMetric(value: number) { return Number.isInteger(value) ? String(value) : value.toFixed(4) }
function formatDelta(value: number) { return `${value > 0 ? '+' : ''}${formatMetric(value)}` }
function formatPercent(value: number) { return `${value > 0 ? '+' : ''}${(value * 100).toFixed(1)}%` }
