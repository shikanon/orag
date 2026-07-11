import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import type { FormEvent } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { projectApi } from '../../api/client'

export function ProjectForm() {
  const navigate = useNavigate(), queryClient = useQueryClient()
  const [name, setName] = useState(''), [description, setDescription] = useState('')
  const mutation = useMutation({ mutationFn: projectApi.create, onSuccess: (project) => { queryClient.setQueryData(['projects', project.id], project); void queryClient.invalidateQueries({ queryKey: ['projects', 'list'] }); navigate(`/projects/${project.id}/overview`) } })
  const submit = (event: FormEvent) => { event.preventDefault(); mutation.mutate({ name: name.trim(), description: description.trim() }) }
  return <main className="content narrow"><Link className="back-link" to="/projects">← 返回项目</Link><header className="page-header"><div><h1>新建项目</h1><p>项目将隔离编排、测试集、环境与发布历史。</p></div></header>
    <form className="project-form" onSubmit={submit}><label>项目名称<input autoFocus required value={name} onChange={(event) => setName(event.target.value)} placeholder="例如：客服知识助手" /></label><label>项目说明<textarea value={description} onChange={(event) => setDescription(event.target.value)} placeholder="简要说明项目用途" rows={4} /></label>{mutation.isError ? <p role="alert">创建失败，请稍后重试。</p> : null}<div className="form-actions"><Link className="secondary-button" to="/projects">取消</Link><button className="primary-button" disabled={!name.trim() || mutation.isPending}>{mutation.isPending ? '创建中…' : '创建项目'}</button></div></form>
  </main>
}
