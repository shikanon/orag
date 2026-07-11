import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { projectApi } from '../../api/client'

export function ProjectList() {
  const query = useQuery({ queryKey: ['projects', 'list'], queryFn: projectApi.list })
  return <main className="content"><header className="page-header"><div><h1>项目</h1><p>管理彼此隔离的 RAG 应用、评测资产与发布环境。</p></div><Link className="primary-button" to="/projects/new">新建项目</Link></header>
    <section className="project-table" aria-label="项目列表"><div className="table-head"><span>名称</span><span>说明</span><span>最近更新</span><span /></div>
      {query.isLoading ? <p className="table-message">正在加载项目…</p> : query.data?.projects.map((project) => <Link className="project-row" to={`/projects/${project.id}/overview`} key={project.id}><strong>{project.name}</strong><span>{project.description || '—'}</span><time>{new Date(project.updated_at).toLocaleDateString('zh-CN')}</time><span aria-hidden>→</span></Link>)}
    </section>
  </main>
}
