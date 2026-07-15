import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import { tutorialApi, type TutorialCloneJob } from '../../api/client'

const stageLabels: Record<TutorialCloneJob['stage'], string> = {
  create_project: '正在创建实验项目',
  validate_manifest: '正在校验数据包清单',
  download_pack: '正在下载数据包',
  verify_pack: '正在校验数据包',
  write_private_store: '正在写入项目私有存储',
  pack_installed: '数据包已安装',
}

export function TutorialCloneProgress() {
  const { projectId = '' } = useParams()
  const [search] = useSearchParams()
  const jobId = search.get('job') ?? ''
  const queryClient = useQueryClient()
  const jobQuery = useQuery({
    queryKey: ['tutorial-clones', jobId],
    queryFn: () => tutorialApi.getCloneJob(jobId),
    enabled: jobId.length > 0,
    refetchInterval: (query) => terminal(query.state.data?.status) ? false : 1000,
  })
  const retry = useMutation({
    mutationFn: () => tutorialApi.retryClone(jobId),
    onSuccess: (job) => queryClient.setQueryData(['tutorial-clones', jobId], job),
  })
  const installed = jobQuery.data?.status === 'completed'
  const experimentQuery = useQuery({ queryKey: ['tutorial-experiment', projectId], queryFn: () => tutorialApi.getExperiment(projectId), enabled: installed && projectId.length > 0 })
  if (!jobId) return <main className="content tutorial-setup"><section className="tutorial-state" role="alert"><h1>缺少克隆任务</h1><p>请从教程目录创建实验项目。</p><Link className="secondary-button" to="/tutorials">返回教程库</Link></section></main>
  if (jobQuery.isLoading) return <main className="content tutorial-setup" aria-label="正在读取克隆进度" aria-busy="true"><div className="skeleton-line short" /><div className="skeleton-line" /></main>
  if (jobQuery.isError || !jobQuery.data) return <main className="content tutorial-setup"><section className="tutorial-state" role="alert"><h1>无法读取克隆进度</h1><p>请稍后重试，或返回教程库重新发起请求。</p><button className="secondary-button" onClick={() => void jobQuery.refetch()}>重新加载</button></section></main>
  const job = jobQuery.data
  return <main className="content tutorial-setup"><Link className="back-link" to="/tutorials">← 返回教程库</Link><header className="page-header"><div><p className="eyebrow">Tutorial experiment</p><h1>{job.project_name}</h1><p>基于不可变模板 {job.template_id} v{job.template_version} 创建。数据访问与校验由服务端完成。</p></div><span className={`tutorial-job-status ${job.status}`}>{statusLabel(job.status)}</span></header><section className="tutorial-progress-card"><p className="eyebrow">Pack setup</p><h2>{stageLabels[job.stage]}</h2><p>{job.status === 'completed' ? 'Pack 已安装，Live Run 即将开放。' : job.status === 'failed' ? '安装未完成；可在修复外部数据包或存储配置后安全重试。' : '任务正在后台运行。你可以离开本页面，稍后用同一项目查看状态。'}</p><ol className="tutorial-stage-list">{(['create_project', 'validate_manifest', 'download_pack', 'verify_pack', 'write_private_store', 'pack_installed'] as const).map((stage) => <li className={stageState(job, stage)} key={stage}><span>{stageLabels[stage]}</span><small>{stageState(job, stage) === 'done' ? '完成' : stage === job.stage ? statusLabel(job.status) : '等待'}</small></li>)}</ol>{job.status === 'failed' ? <div className="tutorial-failure" role="alert"><strong>服务端失败代码：{job.failure_code || 'clone_failed'}</strong><p>该代码不包含存储地址或凭证。修复后将从安全检查点继续。</p><button className="primary-button" disabled={retry.isPending} onClick={() => retry.mutate()}>{retry.isPending ? '重新排队中…' : '重试安装'}</button>{retry.isError ? <p>重试未被接受，请刷新后确认任务状态。</p> : null}</div> : null}{installed && experimentQuery.data ? <p className="tutorial-experiment-status">项目实验状态：{experimentQuery.data.pack_status === 'pack_installed' ? 'Pack 已安装' : experimentQuery.data.pack_status}</p> : null}</section></main>
}

function terminal(status?: TutorialCloneJob['status']) {
  return status === 'completed' || status === 'failed'
}

function statusLabel(status: TutorialCloneJob['status']) {
  return ({ queued: '已排队', running: '进行中', failed: '需要处理', completed: '已完成' } as const)[status]
}

function stageState(job: TutorialCloneJob, stage: TutorialCloneJob['stage']) {
  const stages: TutorialCloneJob['stage'][] = ['create_project', 'validate_manifest', 'download_pack', 'verify_pack', 'write_private_store', 'pack_installed']
  const current = stages.indexOf(job.stage)
  const index = stages.indexOf(stage)
  if (job.status === 'completed' || index < current) return 'done'
  if (index === current) return job.status === 'failed' ? 'failed' : 'active'
  return 'pending'
}
