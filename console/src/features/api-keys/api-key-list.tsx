import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import type { FormEvent } from 'react'
import { apiKeyApi, projectApi } from '../../api/client'
import type { APIKey, CreateAPIKeyInput, CreateAPIKeyResponse } from '../../api/client'

const roleLabels = {
  tenant_admin: '租户管理员',
  project_editor: '项目编辑者',
  project_viewer: '项目查看者',
} as const

export function APIKeyList() {
  const queryClient = useQueryClient()
  const keysQuery = useQuery({ queryKey: ['api-keys', 'list'], queryFn: apiKeyApi.list })
  const projectsQuery = useQuery({ queryKey: ['projects', 'list'], queryFn: projectApi.list })
  const [createOpen, setCreateOpen] = useState(false)
  const [revokeTarget, setRevokeTarget] = useState<APIKey | null>(null)
  const revokeMutation = useMutation({
    mutationFn: apiKeyApi.revoke,
    onSuccess: (_result, apiKeyId) => {
      queryClient.setQueryData<{ api_keys: APIKey[] }>(['api-keys', 'list'], (current) => ({ api_keys: (current?.api_keys ?? []).map((key) => key.id === apiKeyId ? { ...key, revoked_at: new Date().toISOString() } : key) }))
      setRevokeTarget(null)
    },
  })

  const projectNames = new Map(projectsQuery.data?.projects.map((project) => [project.id, project.name]))
  const keys = keysQuery.data?.api_keys ?? []
  return <main className="content"><header className="page-header"><div><h1>API Keys</h1><p>为自动化和服务集成创建可撤销的机器凭证。</p></div><button className="primary-button" type="button" onClick={() => setCreateOpen(true)}>创建 API Key</button></header>
    <p className="security-note"><strong>安全提示</strong> 完整密钥只在创建后显示一次。请立即存入密钥管理器，切勿提交到代码仓库。</p>
    {keysQuery.isLoading ? <APIKeyListSkeleton /> : keysQuery.isError ? <section className="api-key-state" role="alert"><h2>API Key 加载失败</h2><p>无法读取机器凭证，请检查 API 状态后重试。</p><button className="secondary-button" type="button" onClick={() => void keysQuery.refetch()}>重新加载</button></section> : keys.length === 0 ? <section className="api-key-state"><h2>还没有 API Key</h2><p>创建项目级密钥，让自动化任务使用最小权限访问 ORAG。</p><button className="primary-button" type="button" onClick={() => setCreateOpen(true)}>创建第一个 API Key</button></section> : <section className="api-key-table" aria-label="API Key 列表"><div className="api-key-head"><span>名称 / Prefix</span><span>权限</span><span>范围</span><span>最近使用</span><span>状态</span><span /></div>{keys.map((key) => <div className="api-key-row" key={key.id}><span><strong>{key.name}</strong><code>{key.prefix}…</code></span><span>{roleLabels[key.role]}</span><span>{key.project_id ? projectNames.get(key.project_id) ?? key.project_id : '全部项目'}</span><time>{key.last_used_at ? formatDate(key.last_used_at) : '从未使用'}</time><KeyStatus item={key} /><span>{key.revoked_at ? null : <button className="danger-link" type="button" onClick={() => setRevokeTarget(key)}>撤销</button>}</span></div>)}</section>}
    {createOpen ? <CreateAPIKeyDialog projects={projectsQuery.data?.projects ?? []} onClose={() => setCreateOpen(false)} /> : null}
    {revokeTarget ? <ConfirmRevokeDialog item={revokeTarget} pending={revokeMutation.isPending} failed={revokeMutation.isError} onCancel={() => { revokeMutation.reset(); setRevokeTarget(null) }} onConfirm={() => revokeMutation.mutate(revokeTarget.id)} /> : null}
  </main>
}

function KeyStatus({ item }: { item: APIKey }) {
  if (item.revoked_at) return <span className="key-status revoked">已撤销</span>
  if (item.expires_at && new Date(item.expires_at).getTime() <= Date.now()) return <span className="key-status expired">已过期</span>
  return <span className="key-status active">有效</span>
}

function CreateAPIKeyDialog({ projects, onClose }: { projects: Array<{ id: string; name: string }>; onClose: () => void }) {
  const queryClient = useQueryClient()
  const [name, setName] = useState('')
  const [role, setRole] = useState<CreateAPIKeyInput['role']>('project_editor')
  const [projectId, setProjectId] = useState('')
  const [expiresAt, setExpiresAt] = useState('')
  const [created, setCreated] = useState<CreateAPIKeyResponse | null>(null)
  const [copied, setCopied] = useState(false)
  const mutation = useMutation({
    mutationFn: apiKeyApi.create,
    onSuccess: (result) => {
      setCreated(result)
      queryClient.setQueryData<{ api_keys: APIKey[] }>(['api-keys', 'list'], (current) => ({ api_keys: [result.api_key, ...(current?.api_keys ?? [])] }))
    },
  })
  const projectRequired = role !== 'tenant_admin'
  const submit = (event: FormEvent) => {
    event.preventDefault()
    const input: CreateAPIKeyInput = { name: name.trim(), role }
    if (projectId) input.project_id = projectId
    if (expiresAt) input.expires_at = new Date(expiresAt).toISOString()
    mutation.mutate(input)
  }
  const copySecret = async () => {
    if (!created) return
    try {
      await navigator.clipboard.writeText(created.secret)
      setCopied(true)
    } catch {
      setCopied(false)
    }
  }
  const close = () => {
    setCreated(null)
    onClose()
  }

  return <div className="dialog-backdrop" role="presentation"><section className="api-key-dialog" role="dialog" aria-modal="true" aria-labelledby="api-key-dialog-title">
    {created ? <><header><div><span className="eyebrow">创建成功</span><h2 id="api-key-dialog-title">立即保存 API Key</h2></div></header><p className="one-time-warning">关闭此窗口后，完整密钥将无法再次查看。</p><label className="secret-field">API Key<div><code data-testid="created-api-key-secret">{created.secret}</code><button className="secondary-button" type="button" onClick={() => void copySecret()}>{copied ? '已复制' : '复制'}</button></div></label><p className="secret-metadata">{created.api_key.name} · {roleLabels[created.api_key.role]} · Prefix {created.api_key.prefix}</p><div className="dialog-actions"><button className="primary-button" type="button" onClick={close}>我已安全保存</button></div></> : <><header><div><span className="eyebrow">机器凭证</span><h2 id="api-key-dialog-title">创建 API Key</h2></div><button className="icon-button" type="button" aria-label="关闭" onClick={close}>×</button></header><form onSubmit={submit}><label>名称<input autoFocus required value={name} onChange={(event) => setName(event.target.value)} placeholder="例如：CI evaluation runner" /></label><label>权限<select value={role} onChange={(event) => { const nextRole = event.target.value as CreateAPIKeyInput['role']; setRole(nextRole); if (nextRole === 'tenant_admin') setProjectId('') }}><option value="project_editor">项目编辑者</option><option value="project_viewer">项目查看者</option><option value="tenant_admin">租户管理员</option></select></label>{projectRequired ? <label>项目<select required value={projectId} onChange={(event) => setProjectId(event.target.value)}><option value="">选择项目</option>{projects.map((project) => <option value={project.id} key={project.id}>{project.name}</option>)}</select></label> : null}<label>过期时间 <span className="optional">可选</span><input type="datetime-local" value={expiresAt} onChange={(event) => setExpiresAt(event.target.value)} /></label>{mutation.isError ? <p role="alert">创建失败，请检查权限、项目和过期时间。</p> : null}<div className="dialog-actions"><button className="secondary-button" type="button" onClick={close}>取消</button><button className="primary-button" disabled={!name.trim() || (projectRequired && !projectId) || mutation.isPending}>{mutation.isPending ? '创建中…' : '创建'}</button></div></form></>}
  </section></div>
}

function ConfirmRevokeDialog({ item, pending, failed, onCancel, onConfirm }: { item: APIKey; pending: boolean; failed: boolean; onCancel: () => void; onConfirm: () => void }) {
  return <div className="dialog-backdrop" role="presentation"><section className="api-key-dialog compact" role="alertdialog" aria-modal="true" aria-labelledby="revoke-title"><header><div><span className="eyebrow danger">不可恢复</span><h2 id="revoke-title">撤销 “{item.name}”</h2></div></header><p>使用该密钥的所有自动化请求会立即失效。历史记录仍会保留。</p>{failed ? <p role="alert">撤销失败，请稍后重试。</p> : null}<div className="dialog-actions"><button className="secondary-button" type="button" onClick={onCancel}>取消</button><button className="danger-button" type="button" disabled={pending} onClick={onConfirm}>{pending ? '撤销中…' : '确认撤销'}</button></div></section></div>
}

function APIKeyListSkeleton() {
  return <section className="table-skeleton" aria-label="正在加载 API Key" aria-busy="true">{[0, 1, 2].map((row) => <div className="skeleton-row" key={row}><span className="skeleton-line short" /><span className="skeleton-line" /><span className="skeleton-line short" /></div>)}</section>
}

function formatDate(value: string) {
  return new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium' }).format(new Date(value))
}
