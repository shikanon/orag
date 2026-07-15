import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useParams } from 'react-router-dom'
import { pipelineApi, type PipelineNodeDefinition, type SavePipelineDraftInput } from '../../api/client'

type EditorNode = { id: string; type: string; schema_version: number; config: Record<string, never> }

export function RAGStudio() {
  const { projectId = '' } = useParams()
  const client = useQueryClient()
  const nodes = useQuery({ queryKey: ['pipeline-node-definitions'], queryFn: pipelineApi.nodeDefinitions })
  const pipelines = useQuery({ queryKey: ['pipelines', projectId], queryFn: () => pipelineApi.list(projectId), enabled: Boolean(projectId) })
  const [pipelineId, setPipelineId] = useState('')
  const selectedID = pipelineId || pipelines.data?.items[0]?.id || ''
  const draft = useQuery({ queryKey: ['pipeline-draft', projectId, selectedID], queryFn: () => pipelineApi.draft(projectId, selectedID), enabled: Boolean(projectId && selectedID) })
  const [name, setName] = useState('Support RAG')
  const [editorNodes, setEditorNodes] = useState<EditorNode[]>([])
  const [revision, setRevision] = useState(0)
  const [notice, setNotice] = useState('')
  const definitions = nodes.data?.items ?? []
  const definitionByType = useMemo(() => new Map(definitions.map((item) => [item.type, item])), [definitions])
  const create = useMutation({ mutationFn: () => pipelineApi.create(projectId, { name: name.trim() }), onSuccess: (item) => { setPipelineId(item.id); setNotice(`已创建 ${item.id}`); client.invalidateQueries({ queryKey: ['pipelines', projectId] }) } })
  const save = useMutation({ mutationFn: () => pipelineApi.saveDraft(projectId, selectedID, { expected_revision: revision, definition: ({ nodes: editorNodes, edges: editorNodes.slice(1).map((item, index) => ({ id: `edge_${index + 1}`, source_node_id: editorNodes[index].id, source_port: 'out', target_node_id: item.id, target_port: 'in' })) } as unknown as SavePipelineDraftInput['definition']) }), onSuccess: (item) => { setRevision(item.revision); setNotice(`已保存 revision ${item.revision}`); client.invalidateQueries({ queryKey: ['pipeline-draft', projectId, selectedID] }) } })

  function loadDraft() {
    if (!draft.data) return
    setRevision(draft.data.revision)
    setEditorNodes((draft.data.definition.nodes ?? []) as EditorNode[])
  }
  function addNode(type: string) {
    const metadata = definitionByType.get(type)
    if (!metadata || editorNodes.some((item) => item.type === type && metadata.singleton)) return
    setEditorNodes((items) => [...items, { id: `${type}_${items.length + 1}`, type, schema_version: metadata.schema_version, config: {} }])
  }
  function fillStandardChain() {
    const ordered = ['init', 'query_route', 'semantic_cache_lookup', 'query_rewrite', 'multi_query', 'hybrid_retrieve', 'ark_rerank', 'context_pack', 'prompt_prefix_cache', 'ark_generate', 'semantic_cache_write']
    setEditorNodes(ordered.map((type, index) => ({ id: type, type, schema_version: definitionByType.get(type)?.schema_version ?? 1, config: {} })))
  }

  return <main className="content studio-page"><Link className="back-link" to={`/projects/${projectId}/overview`}>← 返回项目概览</Link><header className="page-header"><div><span className="eyebrow">Beta · Server-owned registry</span><h1>RAG Studio</h1><p>服务端 registry 约束节点类型，draft 以 revision 乐观并发保存。</p></div><code className="project-pill">{projectId}</code></header>
    <section className="studio-panel studio-workbench"><div className="section-heading"><p className="eyebrow">Pipeline draft</p><h2>选择或创建 Pipeline</h2></div><div className="studio-toolbar"><select aria-label="Pipeline" value={selectedID} onChange={(event) => setPipelineId(event.target.value)}><option value="">选择 Pipeline</option>{pipelines.data?.items.map((item) => <option value={item.id} key={item.id}>{item.name} · {item.id}</option>)}</select><input aria-label="Pipeline name" value={name} onChange={(event) => setName(event.target.value)} placeholder="Pipeline 名称" /><button className="secondary-button" type="button" disabled={create.isPending || !name.trim()} onClick={() => create.mutate()}>创建 Pipeline</button>{selectedID ? <><button className="secondary-button" type="button" disabled={!draft.data} onClick={loadDraft}>加载 Draft</button><button className="primary-button" type="button" disabled={save.isPending || !editorNodes.length} onClick={() => save.mutate()}>保存 Draft · rev {revision}</button></> : null}</div>{notice ? <p className="success-note">{notice}</p> : null}{draft.isError || save.isError ? <p className="debugger-error" role="alert">Draft 加载或保存失败，可能是 revision 已过期。</p> : null}<div className="studio-canvas"><div><p className="eyebrow">Canvas · {editorNodes.length} nodes</p>{editorNodes.length ? <ol className="pipeline-sequence">{editorNodes.map((item) => <li key={item.id}><strong>{item.id}</strong><code>{item.type}</code></li>)}</ol> : <p className="muted">创建 Pipeline 后，从右侧 palette 添加节点，或使用标准链路。</p>}</div><div><button className="secondary-button" type="button" disabled={!selectedID} onClick={fillStandardChain}>填充标准 RAG 链路</button><div className="node-palette">{definitions.map((node) => <NodeCard key={node.type} node={node} onAdd={addNode} />)}</div></div></div></section></main>
}

function NodeCard({ node, onAdd }: { node: PipelineNodeDefinition; onAdd: (type: string) => void }) {
  return <article className="node-card"><div><span className="eyebrow">{node.category}</span><h3>{node.display_name || node.type}</h3></div><code>{node.type}</code><p>{node.entry ? '查询入口 · ' : ''}{node.produces_answer ? '答案节点 · ' : ''}schema v{node.schema_version}</p>{node.allowed_targets?.length ? <small>允许连接：{node.allowed_targets.join(' → ')}</small> : null}<button className="secondary-button" type="button" onClick={() => onAdd(node.type)}>加入画布</button></article>
}
