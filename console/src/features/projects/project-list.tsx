import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { projectApi } from '../../api/client'

export function ProjectList() {
  const query = useQuery({ queryKey: ['projects', 'list'], queryFn: projectApi.list })
  return <main className="content"><header className="page-header"><div><h1>项目</h1><p>管理彼此隔离的 RAG 应用、评测资产与发布环境。</p></div><Link className="primary-button" to="/projects/new">新建项目</Link></header>{query.isLoading ? <ProjectListSkeleton /> : query.isError ? <section className="project-table project-state" role="alert"><div><h2>项目加载失败</h2><p>无法连接 ORAG API。请检查服务状态后重试。</p><button className="secondary-button" onClick={() => void query.refetch()}>重新加载</button></div></section> : query.data?.projects.length === 0 ? <section className="project-table project-state"><div><h2>创建第一个 RAG 项目</h2><p>项目用于隔离编排、测试集、环境配置和发布历史。</p><Link className="primary-button" to="/projects/new">新建项目</Link></div></section> : <section className="project-table" aria-label="项目列表"><div className="table-head"><span>名称</span><span>说明</span><span>最近更新</span><span /></div>{query.data?.projects.map((project) => <Link className="project-row" to={`/projects/${project.id}/overview`} key={project.id}><strong>{project.name}</strong><span>{project.description || '—'}</span><time>{new Date(project.updated_at).toLocaleDateString('zh-CN')}</time><span aria-hidden>→</span></Link>)}</section>}</main>
}

function ProjectListSkeleton() {
  return <section className="table-skeleton" aria-label="正在加载项目" aria-busy="true">{[0, 1, 2].map((row) => <div className="skeleton-row" key={row}><span className="skeleton-line short" /><span className="skeleton-line" /><span className="skeleton-line short" /></div>)}</section>
}
