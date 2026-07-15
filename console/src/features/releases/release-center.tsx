import { useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useParams } from 'react-router-dom'
import { releaseApi, type EnvironmentKind, type Environment } from '../../api/client'

const kinds: EnvironmentKind[] = ['development', 'staging', 'production']
const labels: Record<EnvironmentKind, string> = { development: '开发', staging: '预发', production: '生产' }

export function ReleaseCenter() {
  const { projectId = '' } = useParams()
  const client = useQueryClient()
  const environments = useQuery({ queryKey: ['release-environments', projectId], queryFn: () => releaseApi.environments(projectId) })
  const history = useQuery({ queryKey: ['releases', projectId], queryFn: () => releaseApi.list(projectId) })
  const versions = useQuery({ queryKey: ['pipeline-versions', projectId], queryFn: () => releaseApi.versions(projectId) })
  const [source, setSource] = useState<EnvironmentKind>('development')
  const [target, setTarget] = useState<EnvironmentKind>('staging')
  const [version, setVersion] = useState('')
  const [developmentVersion, setDevelopmentVersion] = useState('')
  const [rollbackEnv, setRollbackEnv] = useState<EnvironmentKind>('staging')
  const [rollbackVersion, setRollbackVersion] = useState('')
  const [reason, setReason] = useState('')
  const [newVersionID, setNewVersionID] = useState('')
  const [newVersionHash, setNewVersionHash] = useState('')
  const [evidenceVersion, setEvidenceVersion] = useState('')
  const [evidenceHash, setEvidenceHash] = useState('')
  const [evidenceEnvironment, setEvidenceEnvironment] = useState<EnvironmentKind>('staging')
  const [bindingEnvironment, setBindingEnvironment] = useState<EnvironmentKind>('development')
  const [bindingRef, setBindingRef] = useState('')
  const refresh = () => {
    void client.invalidateQueries({ queryKey: ['release-environments', projectId] })
    void client.invalidateQueries({ queryKey: ['releases', projectId] })
    void client.invalidateQueries({ queryKey: ['pipeline-versions', projectId] })
  }
  const createVersion = useMutation({ mutationFn: () => releaseApi.createVersion(projectId, { id: newVersionID || undefined, content_hash: newVersionHash }), onSuccess: () => { refresh(); setNewVersionID(''); setNewVersionHash('') } })
  const validateVersion = useMutation({ mutationFn: () => releaseApi.validateVersion(projectId, evidenceVersion, { environment: evidenceEnvironment, passed: true, content_hash: evidenceHash }), onSuccess: () => { refresh(); setEvidenceVersion(''); setEvidenceHash('') } })
  const bindEnvironment = useMutation({ mutationFn: () => releaseApi.bindEnvironment(projectId, bindingEnvironment, bindingRef.trim()), onSuccess: () => { refresh(); setBindingRef('') } })
  const developmentEnv = findEnv(environments.data?.items, 'development')
  const activateDevelopment = useMutation({ mutationFn: () => releaseApi.activateDevelopment(projectId, { target_version_id: developmentVersion, expected_active_version_id: developmentEnv?.active_version_id ?? '' }), onSuccess: () => { refresh(); setDevelopmentVersion('') } })
  const promote = useMutation({ mutationFn: () => releaseApi.promote(projectId, { source_environment: source as 'development' | 'staging', target_environment: target as 'staging' | 'production', target_version_id: version, expected_active_version_id: findEnv(environments.data?.items, target)?.active_version_id ?? '' }), onSuccess: refresh })
  const rollback = useMutation({ mutationFn: () => releaseApi.rollback(projectId, rollbackEnv, { target_version_id: rollbackVersion, expected_active_version_id: findEnv(environments.data?.items, rollbackEnv)?.active_version_id ?? '', reason }), onSuccess: refresh })
  const targetEnv = findEnv(environments.data?.items, target)
  function submitDevelopmentActivation(event: FormEvent<HTMLFormElement>) { event.preventDefault(); if (developmentVersion) activateDevelopment.mutate() }
  function submitPromotion(event: FormEvent<HTMLFormElement>) { event.preventDefault(); if (version.trim()) promote.mutate() }
  function submitRollback(event: FormEvent<HTMLFormElement>) { event.preventDefault(); if (rollbackVersion.trim() && reason.trim()) rollback.mutate() }
  function submitBinding(event: FormEvent<HTMLFormElement>) { event.preventDefault(); if (bindingRef.trim()) bindEnvironment.mutate() }

  return <main className="content release-page">
    <Link className="back-link" to={`/projects/${projectId}/overview`}>← 返回项目概览</Link>
    <header className="page-header"><div><span className="eyebrow">实验性 · Hard gates</span><h1>发布中心</h1><p>冻结版本必须具备服务端推导的环境评测证据，才能先激活到开发环境，再按开发 → 预发 → 生产顺序晋级。</p></div><code className="project-pill">{projectId}</code></header>
    <section className="release-card"><h2>环境资源绑定</h2><p className="muted">绑定引用只写入服务端，不会在 Console、trace 或发布历史中返回。</p><form onSubmit={submitBinding}><label>环境<select value={bindingEnvironment} onChange={(event) => setBindingEnvironment(event.target.value as EnvironmentKind)}>{kinds.map((kind) => <option key={kind} value={kind}>{labels[kind]}</option>)}</select></label><label>绑定引用<input required value={bindingRef} onChange={(event) => setBindingRef(event.target.value)} placeholder="例如：deployment://production" autoComplete="off" /></label>{bindEnvironment.isError ? <p className="debugger-error" role="alert">绑定失败，请确认环境和引用有效。</p> : null}<button className="secondary-button" disabled={bindEnvironment.isPending || !bindingRef.trim()}>{bindEnvironment.isPending ? '绑定中…' : '保存环境绑定'}</button></form></section>
    <section className="release-actions">
      <form className="release-card" onSubmit={submitDevelopmentActivation}><h2>激活 development</h2><p className="muted">仅接受由冻结 DAG 和已完成服务端评测产生的 development 证据。</p><label>选择要激活到 development 的版本<select required value={developmentVersion} onChange={(event) => setDevelopmentVersion(event.target.value)}><option value="">选择版本</option>{versions.data?.items.map((item) => <option key={item.id} value={item.id}>{item.id}</option>)}</select></label>{activateDevelopment.isError ? <p className="debugger-error" role="alert">激活被拒绝：请确认 development 绑定和服务端评测证据均已通过。</p> : null}<button className="primary-button" disabled={activateDevelopment.isPending || !developmentVersion || !developmentEnv?.bound}>激活 development</button></form>
      <form className="release-card" onSubmit={(event) => { event.preventDefault(); if (newVersionHash.trim()) createVersion.mutate() }}><h2>旧版候选版本（兼容）</h2><p className="muted">只用于既有手工版本记录；新版本请从 RAG Studio 冻结创建。</p><label>版本 ID（可选）<input value={newVersionID} onChange={(event) => setNewVersionID(event.target.value)} placeholder="自动生成" /></label><label>内容 hash<input required value={newVersionHash} onChange={(event) => setNewVersionHash(event.target.value)} placeholder="sha256:..." /></label><button className="secondary-button" disabled={createVersion.isPending || !newVersionHash.trim()}>创建旧版记录</button></form>
      <form className="release-card" onSubmit={(event) => { event.preventDefault(); if (evidenceVersion && evidenceHash) validateVersion.mutate() }}><h2>旧版验证证据（兼容）</h2><p className="muted">冻结 DAG 版本必须从已完成评测推导证据，不能在此手工录入。</p><label>版本<select required value={evidenceVersion} onChange={(event) => { setEvidenceVersion(event.target.value); setEvidenceHash(versions.data?.items.find((item) => item.id === event.target.value)?.content_hash ?? '') }}><option value="">选择版本</option>{versions.data?.items.map((item) => <option key={item.id} value={item.id}>{item.id}</option>)}</select></label><label>环境<select value={evidenceEnvironment} onChange={(event) => setEvidenceEnvironment(event.target.value as EnvironmentKind)}><option value="staging">预发</option><option value="production">生产</option></select></label><label>内容 hash<input required value={evidenceHash} onChange={(event) => setEvidenceHash(event.target.value)} /></label><button className="secondary-button" disabled={validateVersion.isPending || !evidenceVersion || !evidenceHash}>记录旧版证据</button></form>
    </section>
    <section className="release-card"><h2>候选版本</h2>{versions.isLoading ? <p className="muted">加载中…</p> : versions.data?.items.length ? <ul>{versions.data.items.map((item) => <li key={item.id}><code>{item.id}</code> <small>{item.content_hash}</small></li>)}</ul> : <p className="muted">还没有候选版本。</p>}</section>
    <section className="environment-grid" aria-label="环境状态">{kinds.map((kind) => <EnvironmentCard key={kind} environment={findEnv(environments.data?.items, kind)} />)}</section>
    <section className="release-actions"><form className="release-card" onSubmit={submitPromotion}><h2>晋级版本</h2><label>来源<select value={source} onChange={(event) => setSource(event.target.value as EnvironmentKind)}><option value="development">开发</option><option value="staging">预发</option></select></label><label>目标<select value={target} onChange={(event) => setTarget(event.target.value as EnvironmentKind)}><option value="staging">预发</option><option value="production">生产</option></select></label><label>不可变版本 ID<input required value={version} onChange={(event) => setVersion(event.target.value)} placeholder="pv_..." /></label>{promote.isError ? <p className="debugger-error" role="alert">晋级被拒绝：门禁、绑定或并发版本校验未通过。</p> : null}<button className="primary-button" disabled={promote.isPending || !version.trim() || !targetEnv?.bound}>执行晋级</button></form><form className="release-card" onSubmit={submitRollback}><h2>原子回滚</h2><label>环境<select value={rollbackEnv} onChange={(event) => setRollbackEnv(event.target.value as EnvironmentKind)}>{kinds.map((kind) => <option key={kind} value={kind}>{labels[kind]}</option>)}</select></label><label>目标版本 ID<input required value={rollbackVersion} onChange={(event) => setRollbackVersion(event.target.value)} placeholder="pv_..." /></label><label>原因<textarea required rows={3} value={reason} onChange={(event) => setReason(event.target.value)} placeholder="说明为什么恢复到已验证版本" /></label><button className="secondary-button" disabled={rollback.isPending || !rollbackVersion.trim() || !reason.trim()}>执行回滚</button></form></section>
    <section className="release-history"><h2>追加式发布历史</h2>{history.isLoading ? <p className="muted">加载中…</p> : history.data?.items.length ? <ol>{history.data.items.map((item) => <li key={item.id}><strong>{item.action === 'rollback' ? '回滚' : item.action === 'activate' ? '激活' : '晋级'}</strong><span>{item.source_environment} → {item.target_environment}</span><code>{item.target_version_id}</code><small>{item.actor} · {new Date(item.created_at).toLocaleString('zh-CN')}</small></li>)}</ol> : <p className="muted">还没有发布记录。</p>}</section>
  </main>
}

function findEnv(items: Environment[] | undefined, kind: EnvironmentKind) { return items?.find((item) => item.kind === kind) }
function EnvironmentCard({ environment }: { environment?: Environment }) { if (!environment) return <article className="environment-card"><h2>未知</h2><p className="muted">环境不可用</p></article>; return <article className="environment-card"><div><span className="eyebrow">{labels[environment.kind]}</span><h2>{environment.active_version_id || '尚未激活'}</h2></div><span className={`binding-state ${environment.bound ? 'bound' : 'unbound'}`}>{environment.bound ? '已绑定' : '缺少绑定'}</span><small>revision {environment.revision}</small></article> }
