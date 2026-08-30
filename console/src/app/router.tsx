import { lazy, Suspense, useSyncExternalStore } from 'react'
import { useIsFetching } from '@tanstack/react-query'
import { createBrowserRouter, createMemoryRouter, Navigate, NavLink, Outlet, useLocation, useParams } from 'react-router-dom'
import { ProjectSwitcher } from '../features/projects/project-switcher'
import { Login } from '../features/auth/login'
import { clearSession, useSession } from '../features/auth/session'

const loadProjectList = () => import('../features/projects/project-list')
const loadProjectForm = () => import('../features/projects/project-form')
const loadAPIKeyList = () => import('../features/api-keys/api-key-list')
const loadTutorialList = () => import('../features/tutorials/tutorial-list')
const loadTutorialDetail = () => import('../features/tutorials/tutorial-detail')
const loadTutorialCloneProgress = () => import('../features/tutorials/tutorial-clone-progress')
const loadTutorialExperimentWorkbench = () => import('../features/tutorials/tutorial-experiment-workbench')
const loadAPIDebugger = () => import('../features/debugger/api-debugger')
const loadEvaluationCenter = () => import('../features/evaluation/evaluation-center')
const loadReleaseCenter = () => import('../features/releases/release-center')
const loadRAGStudio = () => import('../features/studio/rag-studio')
const loadGovernanceCenter = () => import('../features/governance/governance-center')

const ProjectList = lazy(() => loadProjectList().then((module) => ({ default: module.ProjectList })))
const ProjectForm = lazy(() => loadProjectForm().then((module) => ({ default: module.ProjectForm })))
const APIKeyList = lazy(() => loadAPIKeyList().then((module) => ({ default: module.APIKeyList })))
const TutorialList = lazy(() => loadTutorialList().then((module) => ({ default: module.TutorialList })))
const TutorialDetail = lazy(() => loadTutorialDetail().then((module) => ({ default: module.TutorialDetail })))
const TutorialCloneProgress = lazy(() => loadTutorialCloneProgress().then((module) => ({ default: module.TutorialCloneProgress })))
const TutorialExperimentWorkbench = lazy(() => loadTutorialExperimentWorkbench().then((module) => ({ default: module.TutorialExperimentWorkbench })))
const APIDebugger = lazy(() => loadAPIDebugger().then((module) => ({ default: module.APIDebugger })))
const EvaluationCenter = lazy(() => loadEvaluationCenter().then((module) => ({ default: module.EvaluationCenter })))
const ReleaseCenter = lazy(() => loadReleaseCenter().then((module) => ({ default: module.ReleaseCenter })))
const RAGStudio = lazy(() => loadRAGStudio().then((module) => ({ default: module.RAGStudio })))
const GovernanceCenter = lazy(() => loadGovernanceCenter().then((module) => ({ default: module.GovernanceCenter })))

const preloadProjectList = () => { void loadProjectList() }
const preloadTutorialList = () => { void loadTutorialList() }
const preloadAPIKeyList = () => { void loadAPIKeyList() }
const preloadRAGStudio = () => { void loadRAGStudio() }
const preloadAPIDebugger = () => { void loadAPIDebugger() }
const preloadEvaluationCenter = () => { void loadEvaluationCenter() }
const preloadReleaseCenter = () => { void loadReleaseCenter() }
const preloadGovernanceCenter = () => { void loadGovernanceCenter() }

const subscribeToNetwork = (callback: () => void) => {
  window.addEventListener('online', callback)
  window.addEventListener('offline', callback)
  return () => {
    window.removeEventListener('online', callback)
    window.removeEventListener('offline', callback)
  }
}

const getNetworkSnapshot = () => navigator.onLine

function projectLoader({ params }: { params: { projectId?: string } }) {
  if (!params.projectId?.trim()) throw new Response('Project ID is required', { status: 400 })
  return null
}

function Shell() {
  const session = useSession()
  const location = useLocation()
  if (!session) return <Navigate to="/login" replace state={{ from: location.pathname }} />
  return <div className="app-shell"><aside className="rail"><a className="brand" href="/projects"><span>O</span><strong>ORAG</strong></a><ProjectSwitcher /><nav aria-label="主导航"><NavLink to="/projects" onPointerEnter={preloadProjectList} onFocus={preloadProjectList}>项目</NavLink><NavLink to="/tutorials" onPointerEnter={preloadTutorialList} onFocus={preloadTutorialList}>教程实验室</NavLink><NavLink to="/api-keys" onPointerEnter={preloadAPIKeyList} onFocus={preloadAPIKeyList}>API Keys</NavLink><span className="nav-heading">工作区</span><NavLink to="/projects/default/studio" onPointerEnter={preloadRAGStudio} onFocus={preloadRAGStudio}>RAG Studio</NavLink><NavLink to="/projects/default/debug" className="debug-nav" onPointerEnter={preloadAPIDebugger} onFocus={preloadAPIDebugger}>API Debugger</NavLink><NavLink to="/projects/default/evaluations" onPointerEnter={preloadEvaluationCenter} onFocus={preloadEvaluationCenter}>评测中心</NavLink><NavLink to="/projects/default/releases" onPointerEnter={preloadReleaseCenter} onFocus={preloadReleaseCenter}>发布中心</NavLink></nav><ConnectionStatus /></aside><section className="workspace"><div className="topbar"><span>ORAG Console</span><div className="topbar-actions"><span className="environment">Development</span><button type="button" onClick={clearSession}>退出</button></div></div><Suspense fallback={<RouteSkeleton />}><Outlet /></Suspense></section></div>
}

function ConnectionStatus() {
  const online = useSyncExternalStore(subscribeToNetwork, getNetworkSnapshot, () => true)
  const fetching = useIsFetching()
  const state = online ? fetching > 0 ? 'syncing' : 'online' : 'offline'
  const label = state === 'syncing' ? '正在同步' : state === 'online' ? '网络在线' : '网络离线'
  return <footer aria-live="polite"><span className={`status-dot ${state}`} />{label}</footer>
}

function RouteSkeleton() {
  return <main className="content" aria-label="正在加载页面" aria-busy="true"><div className="skeleton-line short" style={{ width: 220, height: 28 }} /><div className="skeleton-line" style={{ width: 380, marginTop: 14 }} /><div className="table-skeleton" style={{ marginTop: 48 }}><div className="skeleton-row"><span className="skeleton-line" /><span className="skeleton-line" /><span className="skeleton-line" /></div><div className="skeleton-row"><span className="skeleton-line" /><span className="skeleton-line" /><span className="skeleton-line" /></div></div></main>
}

function Overview() {
  const { projectId } = useParams()
  return <main className="content"><header className="page-header"><div><h1>项目概览</h1><p>项目 <code>{projectId}</code> 的编排、评测、治理和发布入口。</p></div><NavLink className="primary-button" to={`/projects/${projectId}/debug`} onPointerEnter={preloadAPIDebugger} onFocus={preloadAPIDebugger}>打开 API Debugger</NavLink></header><section className="empty-state"><div className="empty-symbol">⌁</div><h2>先验证一条真实查询</h2><p>使用 API Debugger 检查答案、引用和 trace，再开始构建完整流程。</p><div className="overview-actions"><NavLink className="secondary-button" to={`/projects/${projectId}/debug`} onPointerEnter={preloadAPIDebugger} onFocus={preloadAPIDebugger}>运行第一条查询</NavLink><NavLink className="secondary-button" to={`/projects/${projectId}/governance`} onPointerEnter={preloadGovernanceCenter} onFocus={preloadGovernanceCenter}>打开任务治理</NavLink></div></section></main>
}

export function createAppRouter(initialEntries?: string[]) {
  const routes = [{ path: '/login', element: <Login /> }, { path: '/', element: <Shell />, children: [
    { index: true, element: <Navigate to="/projects" replace /> },
    { path: 'projects', element: <ProjectList /> },
    { path: 'projects/new', element: <ProjectForm /> },
    { path: 'projects/:projectId/overview', loader: projectLoader, element: <Overview /> },
    { path: 'projects/:projectId/debug', loader: projectLoader, element: <APIDebugger /> },
    { path: 'projects/:projectId/studio', loader: projectLoader, element: <RAGStudio /> },
    { path: 'projects/:projectId/evaluations', loader: projectLoader, element: <EvaluationCenter /> },
    { path: 'projects/:projectId/releases', loader: projectLoader, element: <ReleaseCenter /> },
    { path: 'projects/:projectId/governance', loader: projectLoader, element: <GovernanceCenter /> },
    { path: 'api-keys', element: <APIKeyList /> },
    { path: 'tutorials', element: <TutorialList /> },
    { path: 'tutorials/:templateId', element: <TutorialDetail /> },
    { path: 'projects/:projectId/tutorial/setup', loader: projectLoader, element: <TutorialCloneProgress /> },
    { path: 'projects/:projectId/tutorial/experiments/:experimentId', loader: projectLoader, element: <TutorialExperimentWorkbench /> },
  ] }]
  const future = { v7_startTransition: true, v7_relativeSplatPath: true }
  return initialEntries ? createMemoryRouter(routes, { initialEntries, future }) : createBrowserRouter(routes, { future })
}
