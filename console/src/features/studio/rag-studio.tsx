import { useQuery } from '@tanstack/react-query'
import { Link, useParams } from 'react-router-dom'
import { pipelineApi } from '../../api/client'

export function RAGStudio() {
  const { projectId = '' } = useParams()
  const nodes = useQuery({ queryKey: ['pipeline-node-definitions'], queryFn: pipelineApi.nodeDefinitions })
  return <main className="content studio-page"><Link className="back-link" to={`/projects/${projectId}/overview`}>← 返回项目概览</Link><header className="page-header"><div><span className="eyebrow">Beta · Server-owned registry</span><h1>RAG Studio</h1><p>只能使用服务端注册且通过类型约束的节点构建查询 DAG。编辑器与 draft 持久化将在此 registry 之上展开。</p></div><code className="project-pill">{projectId}</code></header><section className="studio-panel"><div className="section-heading"><p className="eyebrow">Node palette</p><h2>可用节点</h2><p>节点定义来自 API，不由浏览器硬编码；entry、answer producer 和允许的连接目标都会显示。</p></div>{nodes.isLoading ? <p className="muted">加载节点定义…</p> : nodes.isError ? <p className="debugger-error" role="alert">无法加载节点 registry。</p> : <div className="node-palette">{nodes.data?.items.map((node) => <article className="node-card" key={node.type}><div><span className="eyebrow">{node.category}</span><h3>{node.display_name || node.type}</h3></div><code>{node.type}</code><p>{node.entry ? '查询入口 · ' : ''}{node.produces_answer ? '答案节点 · ' : ''}schema v{node.schema_version}</p>{node.allowed_targets?.length ? <small>允许连接：{node.allowed_targets.join(' → ')}</small> : null}</article>)}</div>}</section></main>
}
