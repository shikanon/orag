import { lazy, Suspense } from 'react'
import { createBrowserRouter, createMemoryRouter, Navigate, NavLink, Outlet, useParams } from 'react-router-dom'
import { ProjectSwitcher } from '../features/projects/project-switcher'

const ProjectList = lazy(() => import('../features/projects/project-list').then((module) => ({ default: module.ProjectList })))
const ProjectForm = lazy(() => import('../features/projects/project-form').then((module) => ({ default: module.ProjectForm })))

function projectLoader({ params }: { params: { projectId?: string } }) {
  if (!params.projectId?.trim()) throw new Response('Project ID is required', { status: 400 })
  return null
}

function Shell() {
  return <div className="app-shell"><aside className="rail"><a className="brand" href="/projects"><span>O</span><strong>ORAG</strong></a><ProjectSwitcher /><nav aria-label="主导航"><NavLink to="/projects">项目</NavLink><span className="nav-heading">工作区</span><span className="nav-disabled">RAG Studio</span><span className="nav-disabled">评测中心</span><span className="nav-disabled">发布中心</span></nav><footer><span className="status-dot" /> API connected</footer></aside><section className="workspace"><div className="topbar"><span>ORAG Console</span><span className="environment">Development</span></div><Suspense fallback={<div className="route-loading">正在加载…</div>}><Outlet /></Suspense></section></div>
}

function Overview() {
  const { projectId } = useParams()
  return <main className="content"><header className="page-header"><div><h1>项目概览</h1><p>项目 <code>{projectId}</code> 的编排、评测和发布入口。</p></div><button className="primary-button">创建 Pipeline</button></header><section className="empty-state"><div className="empty-symbol">⌁</div><h2>开始构建第一条 RAG 流程</h2><p>使用内置节点组合查询链路，并在发布前通过评测门禁。</p><button className="secondary-button">打开 RAG Studio</button></section></main>
}

export function createAppRouter(initialEntries?: string[]) {
  const routes = [{ path: '/', element: <Shell />, children: [
    { index: true, element: <Navigate to="/projects" replace /> },
    { path: 'projects', element: <ProjectList /> },
    { path: 'projects/new', element: <ProjectForm /> },
    { path: 'projects/:projectId/overview', loader: projectLoader, element: <Overview /> },
  ] }]
  const future = { v7_startTransition: true, v7_relativeSplatPath: true }
  return initialEntries ? createMemoryRouter(routes, { initialEntries, future }) : createBrowserRouter(routes, { future })
}
