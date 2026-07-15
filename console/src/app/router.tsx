import { lazy, Suspense } from 'react'
import { createBrowserRouter, createMemoryRouter, Navigate, NavLink, Outlet, useLocation, useParams } from 'react-router-dom'
import { ProjectSwitcher } from '../features/projects/project-switcher'
import { Login } from '../features/auth/login'
import { clearSession, useSession } from '../features/auth/session'

const ProjectList = lazy(() => import('../features/projects/project-list').then((module) => ({ default: module.ProjectList })))
const ProjectForm = lazy(() => import('../features/projects/project-form').then((module) => ({ default: module.ProjectForm })))
const APIKeyList = lazy(() => import('../features/api-keys/api-key-list').then((module) => ({ default: module.APIKeyList })))
const TutorialList = lazy(() => import('../features/tutorials/tutorial-list').then((module) => ({ default: module.TutorialList })))
const TutorialDetail = lazy(() => import('../features/tutorials/tutorial-detail').then((module) => ({ default: module.TutorialDetail })))
const APIDebugger = lazy(() => import('../features/debugger/api-debugger').then((module) => ({ default: module.APIDebugger })))
const EvaluationCenter = lazy(() => import('../features/evaluation/evaluation-center').then((module) => ({ default: module.EvaluationCenter })))
const ReleaseCenter = lazy(() => import('../features/releases/release-center').then((module) => ({ default: module.ReleaseCenter })))

function projectLoader({ params }: { params: { projectId?: string } }) {
  if (!params.projectId?.trim()) throw new Response('Project ID is required', { status: 400 })
  return null
}

function Shell() {
  const session = useSession()
  const location = useLocation()
  if (!session) return <Navigate to="/login" replace state={{ from: location.pathname }} />
  return <div className="app-shell"><aside className="rail"><a className="brand" href="/projects"><span>O</span><strong>ORAG</strong></a><ProjectSwitcher /><nav aria-label="主导航"><NavLink to="/projects">项目</NavLink><NavLink to="/tutorials">教程实验室</NavLink><NavLink to="/api-keys">API Keys</NavLink><span className="nav-heading">工作区</span><NavLink to="/projects/default/debug" className="debug-nav">API Debugger</NavLink><NavLink to="/projects/default/evaluations">评测中心</NavLink><NavLink to="/projects/default/releases">发布中心</NavLink><span className="nav-disabled">RAG Studio</span></nav><footer><span className="status-dot" /> API connected</footer></aside><section className="workspace"><div className="topbar"><span>ORAG Console</span><div className="topbar-actions"><span className="environment">Development</span><button type="button" onClick={clearSession}>退出</button></div></div><Suspense fallback={<RouteSkeleton />}><Outlet /></Suspense></section></div>
}

function RouteSkeleton() {
  return <main className="content" aria-label="正在加载页面" aria-busy="true"><div className="skeleton-line short" style={{ width: 220, height: 28 }} /><div className="skeleton-line" style={{ width: 380, marginTop: 14 }} /><div className="table-skeleton" style={{ marginTop: 48 }}><div className="skeleton-row"><span className="skeleton-line" /><span className="skeleton-line" /><span className="skeleton-line" /></div><div className="skeleton-row"><span className="skeleton-line" /><span className="skeleton-line" /><span className="skeleton-line" /></div></div></main>
}

function Overview() {
  const { projectId } = useParams()
  return <main className="content"><header className="page-header"><div><h1>项目概览</h1><p>项目 <code>{projectId}</code> 的编排、评测和发布入口。</p></div><NavLink className="primary-button" to={`/projects/${projectId}/debug`}>打开 API Debugger</NavLink></header><section className="empty-state"><div className="empty-symbol">⌁</div><h2>先验证一条真实查询</h2><p>使用 API Debugger 检查答案、引用和 trace，再开始构建完整流程。</p><NavLink className="secondary-button" to={`/projects/${projectId}/debug`}>运行第一条查询</NavLink></section></main>
}

export function createAppRouter(initialEntries?: string[]) {
  const routes = [{ path: '/login', element: <Login /> }, { path: '/', element: <Shell />, children: [
    { index: true, element: <Navigate to="/projects" replace /> },
    { path: 'projects', element: <ProjectList /> },
    { path: 'projects/new', element: <ProjectForm /> },
    { path: 'projects/:projectId/overview', loader: projectLoader, element: <Overview /> },
    { path: 'projects/:projectId/debug', loader: projectLoader, element: <APIDebugger /> },
    { path: 'projects/:projectId/evaluations', loader: projectLoader, element: <EvaluationCenter /> },
    { path: 'projects/:projectId/releases', loader: projectLoader, element: <ReleaseCenter /> },
    { path: 'api-keys', element: <APIKeyList /> },
    { path: 'tutorials', element: <TutorialList /> },
    { path: 'tutorials/:templateId', element: <TutorialDetail /> },
  ] }]
  const future = { v7_startTransition: true, v7_relativeSplatPath: true }
  return initialEntries ? createMemoryRouter(routes, { initialEntries, future }) : createBrowserRouter(routes, { future })
}
