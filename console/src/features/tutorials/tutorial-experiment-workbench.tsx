import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useParams } from 'react-router-dom'
import { tutorialApi, type TutorialExperimentRun } from '../../api/client'

const stageLabel: Record<TutorialExperimentRun['stage'], string> = {
  index_private_pack: '正在从私有 Pack 构建基线索引',
  run_evaluation: '正在运行基线评测',
  completed: '基线评测已完成',
}

const statusLabels: Record<TutorialExperimentRun['status'], string> = {
  queued: '已排队', running: '进行中', cancel_requested: '正在取消', cancelled: '已取消', failed: '需要处理', completed: '已完成',
}

export function TutorialExperimentWorkbench() {
  const { projectId = '', experimentId = '' } = useParams()
  const queryClient = useQueryClient()
  const experiment = useQuery({ queryKey: ['tutorial-experiment', projectId], queryFn: () => tutorialApi.getExperiment(projectId), enabled: projectId.length > 0 })
  const start = useMutation({
    mutationFn: () => tutorialApi.startLiveRun(projectId, experimentId, { idempotency_key: newIdempotencyKey() }),
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
  if (experiment.isLoading) return <main className="content tutorial-workbench" aria-label="正在读取教程实验" aria-busy="true"><div className="skeleton-line short" /><div className="skeleton-line" /></main>
  if (experiment.isError || !experiment.data || experiment.data.id !== experimentId) return <main className="content tutorial-workbench"><section className="tutorial-state" role="alert"><h1>教程实验不存在</h1><Link className="secondary-button" to="/tutorials">返回教程库</Link></section></main>
  const ready = experiment.data.pack_status === 'pack_installed' && experiment.data.runtime_status === 'ready'
  const active = run.data && !terminal(run.data.status)
  return <main className="content tutorial-workbench"><Link className="back-link" to={`/projects/${projectId}/tutorial/setup`}>← 返回安装进度</Link><header className="page-header"><div><p className="eyebrow">Baseline Live Run</p><h1>文本 Quick 基线</h1><p>服务端从项目私有 Pack 读取已校验文本，使用固定的 realtime 基线配置创建索引并复用标准评测引擎。不会将对象地址或模型密钥发送给浏览器。</p></div><span className={`tutorial-job-status ${run.data?.status ?? (ready ? 'completed' : 'failed')}`}>{run.data ? statusText(run.data.status) : ready ? '可以运行' : '不可运行'}</span></header><section className="tutorial-workbench-grid"><section className="tutorial-progress-card"><p className="eyebrow">固定基线</p><h2>Baseline · realtime</h2><dl className="tutorial-run-facts"><div><dt>知识库</dt><dd><code>{experiment.data.knowledge_base_id || '未创建'}</code></dd></div><div><dt>评测集</dt><dd><code>{experiment.data.dataset_id || '未创建'}</code></dd></div><div><dt>Top-K</dt><dd>{experiment.data.baseline_top_k ?? '—'}</dd></div></dl>{!ready ? <p className="tutorial-failure" role="alert">当前 Pack 没有可运行的文本基线声明。可查看安装状态，但不能把它当成已评测结果。</p> : <button className="primary-button" disabled={start.isPending || !!active} onClick={() => start.mutate()}>{start.isPending ? '正在提交…' : active ? '基线运行中…' : '运行基线评测'}</button>}{start.isError ? <p className="debugger-error" role="alert">运行未被接受。请确认 Pack 已安装、项目权限和模型运行环境。</p> : null}</section><section className="tutorial-progress-card"><p className="eyebrow">运行进度</p><h2>{run.data ? stageLabel[run.data.stage] : '尚未开始'}</h2>{run.data ? <><p>状态：{statusText(run.data.status)}</p><ol className="tutorial-stage-list"><li className={run.data.stage === 'index_private_pack' ? 'active' : 'done'}><span>构建基线索引</span><small>{run.data.stage === 'index_private_pack' ? statusText(run.data.status) : '完成'}</small></li><li className={run.data.stage === 'run_evaluation' ? 'active' : run.data.stage === 'completed' ? 'done' : 'pending'}><span>执行标准评测</span><small>{run.data.stage === 'run_evaluation' ? statusText(run.data.status) : run.data.stage === 'completed' ? '完成' : '等待'}</small></li></ol>{run.data.status === 'failed' ? <div className="tutorial-failure" role="alert">服务端失败代码：{run.data.failure_code || 'live_run_failed'}</div> : null}{run.data.evaluation_run_id ? <p className="tutorial-experiment-status">标准评测 Run：<code>{run.data.evaluation_run_id}</code></p> : null}{active ? <button className="secondary-button" disabled={cancel.isPending} onClick={() => cancel.mutate()}>{cancel.isPending ? '正在取消…' : '取消运行'}</button> : null}</> : <p className="muted">启动后会显示私有 Pack 索引和评测两个阶段。</p>}</section></section></main>
}

function terminal(status?: TutorialExperimentRun['status']) {
  return status === 'cancelled' || status === 'failed' || status === 'completed'
}

function statusText(status: TutorialExperimentRun['status']) {
  return statusLabels[status]
}

function newIdempotencyKey() {
  return `tutorial-baseline-${Date.now()}-${Math.random().toString(36).slice(2)}`
}
