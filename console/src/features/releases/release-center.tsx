import { useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useParams } from 'react-router-dom'
import { releaseApi, type EnvironmentKind, type Environment, type Release } from '../../api/client'

const kinds: EnvironmentKind[] = ['development', 'staging', 'production']
const labels: Record<EnvironmentKind, string> = { development: '开发', staging: '预发', production: '生产' }

export function ReleaseCenter() {
  const { projectId = '' } = useParams()
  const client = useQueryClient()
  const environments = useQuery({ queryKey: ['release-environments', projectId], queryFn: () => releaseApi.environments(projectId) })
  const history = useQuery({ queryKey: ['releases', projectId], queryFn: () => releaseApi.list(projectId) })
  const [source, setSource] = useState<EnvironmentKind>('development')
  const [target, setTarget] = useState<EnvironmentKind>('staging')
  const [version, setVersion] = useState('')
  const [rollbackEnv, setRollbackEnv] = useState<EnvironmentKind>('staging')
  const [rollbackVersion, setRollbackVersion] = useState('')
  const [reason, setReason] = useState('')
  const promote = useMutation({ mutationFn: () => releaseApi.promote(projectId, { source_environment: source as 'development' | 'staging', target_environment: target as 'staging' | 'production', target_version_id: version, expected_active_version_id: findEnv(environments.data?.items, target)?.active_version_id ?? '' }), onSuccess: refresh })
  const rollback = useMutation({ mutationFn: () => releaseApi.rollback(projectId, rollbackEnv, { target_version_id: rollbackVersion, expected_active_version_id: findEnv(environments.data?.items, rollbackEnv)?.active_version_id ?? '', reason }), onSuccess: refresh })
  function refresh() { void client.invalidateQueries({ queryKey: ['release-environments', projectId] }); void client.invalidateQueries({ queryKey: ['releases', projectId] }); setVersion(''); setRollbackVersion(''); setReason('') }
  function submitPromotion(event: FormEvent<HTMLFormElement>) { event.preventDefault(); if (version.trim()) promote.mutate() }
  function submitRollback(event: FormEvent<HTMLFormElement>) { event.preventDefault(); if (rollbackVersion.trim() && reason.trim()) rollback.mutate() }
  const targetEnv = findEnv(environments.data?.items, target)

  return <main className="content release-page"><Link className="back-link" to={`/projects/${projectId}/overview`}>← 返回项目概览</Link><header className="page-header"><div><span className="eyebrow">实验性 · Hard gates</span><h1>发布中心</h1><p>只能按开发 → 预发 → 生产顺序晋级；回滚必须指向曾在目标环境验证过的版本。</p></div><code className="project-pill">{projectId}</code></header><section className="environment-grid" aria-label="环境状态">{kinds.map((kind) => <EnvironmentCard key={kind} environment={findEnv(environments.data?.items, kind)} />)}</section><section className="release-actions"><form className="release-card" onSubmit={submitPromotion}><h2>晋级版本</h2><label>来源<select value={source} onChange={(event) => setSource(event.target.value as EnvironmentKind)}><option value="development">开发</option><option value="staging">预发</option></select></label><label>目标<select value={target} onChange={(event) => setTarget(event.target.value as EnvironmentKind)}><option value="staging">预发</option><option value="production">生产</option></select></label><label>不可变版本 ID<input required value={version} onChange={(event) => setVersion(event.target.value)} placeholder="pv_..." /></label>{promote.isError ? <p className="debugger-error" role="alert">晋级被拒绝：门禁、绑定或并发版本校验未通过。</p> : null}<button className="primary-button" disabled={promote.isPending || !version.trim() || !targetEnv?.bound}>{promote.isPending ? '提交中…' : '执行晋级'}</button></form><form className="release-card" onSubmit={submitRollback}><h2>原子回滚</h2><label>环境<select value={rollbackEnv} onChange={(event) => setRollbackEnv(event.target.value as EnvironmentKind)}>{kinds.map((kind) => <option key={kind} value={kind}>{labels[kind]}</option>)}</select></label><label>目标版本 ID<input required value={rollbackVersion} onChange={(event) => setRollbackVersion(event.target.value)} placeholder="pv_..." /></label><label>原因<textarea required rows={3} value={reason} onChange={(event) => setReason(event.target.value)} placeholder="说明为什么恢复到已验证版本" /></label>{rollback.isError ? <p className="debugger-error" role="alert">回滚被拒绝：目标版本没有在该环境通过验证，或版本已发生并发变化。</p> : null}<button className="secondary-button" disabled={rollback.isPending || !rollbackVersion.trim() || !reason.trim()}>{rollback.isPending ? '提交中…' : '执行回滚'}</button></form></section><section className="release-history"><h2>追加式发布历史</h2>{history.isLoading ? <p className="muted">加载中…</p> : history.data?.items.length ? <ol>{history.data.items.map((item) => <li key={item.id}><strong>{item.action === 'rollback' ? '回滚' : '晋级'}</strong><span>{item.source_environment} → {item.target_environment}</span><code>{item.target_version_id}</code><small>{item.actor} · {new Date(item.created_at).toLocaleString('zh-CN')}</small></li>)}</ol> : <p className="muted">还没有发布记录。</p>}</section></main>
}

function findEnv(items: Environment[] | undefined, kind: EnvironmentKind) { return items?.find((item) => item.kind === kind) }
function EnvironmentCard({ environment }: { environment?: Environment }) { if (!environment) return <article className="environment-card"><h2>未知</h2><p className="muted">环境不可用</p></article>; return <article className="environment-card"><div><span className="eyebrow">{labels[environment.kind]}</span><h2>{environment.active_version_id || '尚未激活'}</h2></div><span className={`binding-state ${environment.bound ? 'bound' : 'unbound'}`}>{environment.bound ? '已绑定' : '缺少绑定'}</span><small>revision {environment.revision}</small></article> }
