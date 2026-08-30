import '../../governance.css'
import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useParams } from 'react-router-dom'
import { governanceApi, type GovernanceTask } from '../../api/client'

const activeStatuses = new Set(['queued', 'leased', 'running', 'cancelling'])
const retryableStatuses = new Set(['failed_retryable', 'failed_terminal', 'dead_letter'])
const statusLabels: Record<string, string> = { queued: '排队中', leased: '已租约', running: '运行中', succeeded: '已完成', failed_retryable: '可重试', failed_terminal: '失败', cancelling: '取消中', cancelled: '已取消', dead_letter: '死信' }
const poolLabels: Record<string, string> = { ingestion_core: '文档入库', evaluation: '评测', optimizer: '优化', release: '发布' }

export function GovernanceCenter() {
  const { projectId = '' } = useParams()
  const client = useQueryClient()
  const [status, setStatus] = useState('')
  const [pool, setPool] = useState('')
  const [selectedTask, setSelectedTask] = useState<GovernanceTask | null>(null)
  const taskQueryKey = ['governance-tasks', projectId, status, pool]
  const tasks = useQuery({ queryKey: taskQueryKey, queryFn: () => governanceApi.listTasks(projectId, { status, pool }), enabled: Boolean(projectId), refetchInterval: 15_000 })
  const audit = useQuery({ queryKey: ['governance-audit', projectId], queryFn: () => governanceApi.projectAudit(projectId), enabled: Boolean(projectId), refetchInterval: 30_000 })
  const events = useQuery({ queryKey: ['governance-task-events', selectedTask?.id], queryFn: () => governanceApi.taskEvents(selectedTask!.id), enabled: Boolean(selectedTask?.id), refetchInterval: activeStatuses.has(selectedTask?.status ?? '') ? 5_000 : false })
  const retry = useMutation({ mutationFn: governanceApi.retryTask, onSuccess: () => void refresh(client, taskQueryKey, projectId, selectedTask?.id) })
  const cancel = useMutation({ mutationFn: governanceApi.cancelTask, onSuccess: () => void refresh(client, taskQueryKey, projectId, selectedTask?.id) })
  const rows = tasks.data?.data ?? []
  const summary = useMemo(() => ({ queued: rows.filter((task) => task.status === 'queued').length, running: rows.filter((task) => task.status === 'running' || task.status === 'leased').length, deadLetter: rows.filter((task) => task.status === 'dead_letter').length }), [rows])
  const pools = [...new Set(rows.map((task) => task.pool))]

  return <main className="content governance-page">
    <Link className="back-link" to={`/projects/${projectId}/overview`}>← 返回项目概览</Link>
    <header className="page-header"><div><span className="eyebrow">实验性 · Durable work</span><h1>任务治理</h1><p>长任务脱离 HTTP 生命周期运行；在这里查看项目内的排队、重试、死信与可追溯操作。</p></div><code className="project-pill">{projectId}</code></header>
    <section className="governance-summary" aria-label="项目任务摘要"><SummaryStat label="排队中" value={summary.queued} /><SummaryStat label="运行中" value={summary.running} /><SummaryStat label="死信任务" value={summary.deadLetter} danger /></section>
    <section className="governance-layout">
      <section className="governance-panel task-panel">
        <div className="governance-toolbar"><label>队列<select aria-label="队列" value={pool} onChange={(event) => setPool(event.target.value)}><option value="">全部队列</option>{pools.map((item) => <option value={item} key={item}>{poolLabels[item] ?? item}</option>)}</select></label><label>状态<select aria-label="状态" value={status} onChange={(event) => setStatus(event.target.value)}><option value="">全部状态</option>{Object.entries(statusLabels).map(([value, label]) => <option value={value} key={value}>{label}</option>)}</select></label><button className="secondary-button" type="button" onClick={() => void Promise.all([tasks.refetch(), audit.refetch()])}>刷新</button></div>
        {tasks.isLoading ? <p className="governance-message">正在读取项目任务…</p> : tasks.isError ? <div className="governance-message" role="alert">任务列表暂时不可用。<button className="secondary-link" type="button" onClick={() => void tasks.refetch()}>重新加载</button></div> : <TaskTable tasks={rows} selectedID={selectedTask?.id} onSelect={setSelectedTask} />}
      </section>
      <aside className="governance-side">
        <section className="governance-panel timeline-panel"><div className="governance-section-title"><h2>任务时间线</h2>{selectedTask ? <button className="secondary-link" type="button" onClick={() => setSelectedTask(null)}>清除选择</button> : null}</div>{selectedTask ? <><p className="selected-task"><strong>{selectedTask.type}</strong><code>{selectedTask.id}</code></p><TaskActions task={selectedTask} retrying={retry.isPending} cancelling={cancel.isPending} onRetry={() => retry.mutate(selectedTask.id)} onCancel={() => cancel.mutate(selectedTask.id)} />{events.isLoading ? <p className="muted">读取事件…</p> : <TaskTimeline events={events.data?.data ?? []} />}</> : <p className="muted">选择一条任务后，可以查看不可变的状态时间线，并对可控任务执行取消或重试。</p>}</section>
        <section className="governance-panel audit-panel"><div className="governance-section-title"><h2>最近审计</h2><span>项目范围</span></div>{audit.isError ? <p className="muted">审计事件暂不可用。</p> : <AuditList events={audit.data?.data ?? []} loading={audit.isLoading} />}</section>
      </aside>
    </section>
  </main>
}

function SummaryStat({ label, value, danger = false }: { label: string; value: number; danger?: boolean }) { return <div className={danger && value > 0 ? 'summary-stat danger' : 'summary-stat'}><span>{label}</span><strong>{value}</strong></div> }

function TaskTable({ tasks, selectedID, onSelect }: { tasks: GovernanceTask[]; selectedID?: string; onSelect: (task: GovernanceTask) => void }) {
  if (!tasks.length) return <p className="governance-message">尚无符合当前筛选条件的任务。</p>
  return <div className="governance-table" aria-label="项目任务"><div className="governance-table-head"><span>任务</span><span>队列</span><span>状态</span><span>尝试</span><span>更新时间</span></div>{tasks.map((task) => <button className={`governance-task-row ${selectedID === task.id ? 'selected' : ''}`} type="button" key={task.id} onClick={() => onSelect(task)}><span><strong>{task.type}</strong><code>{task.id}</code></span><span>{poolLabels[task.pool] ?? task.pool}</span><span><Status status={task.status} /></span><span>{task.attempt}/{task.max_attempts}</span><time>{formatTime(task.updated_at)}</time></button>)}</div>
}

function Status({ status }: { status: string }) { return <span className={`task-status status-${status}`}>{statusLabels[status] ?? status}</span> }

function TaskTimeline({ events }: { events: import('../../api/client').TaskEvent[] }) { return <ol className="task-timeline">{events.map((event) => <li key={event.id}><time>{formatTime(event.created_at)}</time><strong>{event.type}</strong>{event.message ? <span>{event.message}</span> : null}</li>)}{!events.length ? <li className="muted">尚未记录可展示的任务事件。</li> : null}</ol> }
function AuditList({ events, loading }: { events: import('../../api/client').AuditEvent[]; loading: boolean }) { return <ol className="audit-list">{events.map((event) => <li key={event.id}><strong>{event.action}</strong><span>{event.actor_type} · {event.actor_id || 'system'}</span><time>{formatTime(event.created_at)}</time></li>)}{!events.length && !loading ? <li className="muted">暂时没有项目审计事件。</li> : null}</ol> }

function TaskActions({ task, retrying, cancelling, onRetry, onCancel }: { task: GovernanceTask; retrying: boolean; cancelling: boolean; onRetry: () => void; onCancel: () => void }) { return <div className="task-actions">{retryableStatuses.has(task.status) ? <button className="primary-button" type="button" disabled={retrying} onClick={onRetry}>{retrying ? '重试中…' : '重试任务'}</button> : null}{activeStatuses.has(task.status) ? <button className="secondary-button" type="button" disabled={cancelling} onClick={onCancel}>{cancelling ? '请求取消…' : '取消任务'}</button> : null}{task.error_code ? <p className="task-error">{task.error_code}{task.error_message ? ` · ${task.error_message}` : ''}</p> : null}</div> }

async function refresh(client: ReturnType<typeof useQueryClient>, taskQueryKey: readonly unknown[], projectId: string, taskID?: string) { await Promise.all([client.invalidateQueries({ queryKey: taskQueryKey }), client.invalidateQueries({ queryKey: ['governance-audit', projectId] }), taskID ? client.invalidateQueries({ queryKey: ['governance-task-events', taskID] }) : Promise.resolve()]) }
function formatTime(value: string) { const date = new Date(value); return Number.isNaN(date.getTime()) ? '—' : new Intl.DateTimeFormat('zh-CN', { month: 'numeric', day: 'numeric', hour: '2-digit', minute: '2-digit' }).format(date) }
