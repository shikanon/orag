import { useState, type FormEvent } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { Link, useParams } from 'react-router-dom'
import { knowledgeBaseApi, pipelineApi, queryApi, type PipelineDebugResponse, type QueryResponse, type TraceRecord } from '../../api/client'

export function APIDebugger() {
  const { projectId = '' } = useParams()
  const [knowledgeBaseId, setKnowledgeBaseId] = useState('')
  const [question, setQuestion] = useState('')
  const [profile, setProfile] = useState<'realtime' | 'high_precision'>('realtime')
  const [pipelineId, setPipelineId] = useState('')
  const [revision, setRevision] = useState('0')
  const [result, setResult] = useState<QueryResponse | null>(null)
  const [debugResult, setDebugResult] = useState<PipelineDebugResponse | null>(null)
  const [trace, setTrace] = useState<TraceRecord | null>(null)
  const bases = useQuery({ queryKey: ['knowledge-bases'], queryFn: knowledgeBaseApi.list })
  const query = useMutation({ mutationFn: queryApi.run, onSuccess: (response) => { setResult(response); setTrace(null) } })
  const debug = useMutation({ mutationFn: (input: Parameters<typeof pipelineApi.debug>[1]) => pipelineApi.debug(projectId, input), onSuccess: (response) => { setDebugResult(response); setResult(response.response); setTrace(null) } })
  const traceQuery = useMutation({ mutationFn: queryApi.getTrace, onSuccess: setTrace })
  const projectBases = (bases.data?.items ?? []).filter((item) => !item.project_id || item.project_id === projectId)

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!knowledgeBaseId.trim() || !question.trim()) return
    if (pipelineId.trim()) {
      const expectedRevision = Number(revision)
      if (!Number.isInteger(expectedRevision) || expectedRevision < 0) return
      debug.mutate({ pipeline_id: pipelineId.trim(), expected_revision: expectedRevision, environment: 'development', query: { knowledge_base_id: knowledgeBaseId.trim(), query: question.trim(), profile } })
      return
    }
    query.mutate({ knowledge_base_id: knowledgeBaseId.trim(), query: question.trim(), profile })
  }

  return <main className="content debugger-page">
    <Link className="back-link" to={`/projects/${projectId}/overview`}>← 返回项目概览</Link>
    <header className="page-header"><div><span className="eyebrow">实验性 · 只读调试</span><h1>API Debugger</h1><p>运行一次真实 RAG 查询，并沿 trace 检查答案、引用和节点时延。</p></div><code className="project-pill">{projectId}</code></header>
    <section className="debugger-grid">
      <form className="debugger-form" onSubmit={submit}><h2>运行查询</h2>
        <label>Knowledge Base<select value={knowledgeBaseId} onChange={(event) => setKnowledgeBaseId(event.target.value)}><option value="">选择知识库</option>{projectBases.map((item) => <option value={item.id} key={item.id}>{item.name} · {item.id}</option>)}</select></label>
        <label>或手动输入 KB ID<input value={knowledgeBaseId} onChange={(event) => setKnowledgeBaseId(event.target.value)} placeholder="kb_..." /></label>
        <label>查询问题<textarea required rows={5} value={question} onChange={(event) => setQuestion(event.target.value)} placeholder="例如：这个项目如何处理 trace？" /></label>
        <label>Profile<select value={profile} onChange={(event) => setProfile(event.target.value as typeof profile)}><option value="realtime">realtime · 低延迟</option><option value="high_precision">high_precision · 高精度</option></select></label>
        <label>Pipeline ID（可选，启用 draft debug）<input value={pipelineId} onChange={(event) => setPipelineId(event.target.value)} placeholder="pipe_..." /></label>
        {pipelineId.trim() ? <label>Draft revision<input type="number" min="0" value={revision} onChange={(event) => setRevision(event.target.value)} /></label> : null}
        {query.isError || debug.isError ? <p className="debugger-error" role="alert">查询失败，请确认 API Key、知识库权限、Pipeline draft revision 和服务状态。</p> : null}
        <button className="primary-button" disabled={query.isPending || debug.isPending || !knowledgeBaseId.trim() || !question.trim()}>{query.isPending || debug.isPending ? '运行中…' : pipelineId.trim() ? '运行 Draft Debug' : '运行 RAG 查询'}</button>
      </form>
      <ResultPanel result={result} debugResult={debugResult} trace={trace} traceQuery={traceQuery} />
    </section>
  </main>
}

function ResultPanel({ result, debugResult, trace, traceQuery }: { result: QueryResponse | null; debugResult: PipelineDebugResponse | null; trace: TraceRecord | null; traceQuery: ReturnType<typeof useMutation<TraceRecord, Error, string>> }) {
  if (!result) return <section className="debugger-result"><h2>结果</h2><div className="debugger-placeholder"><span>⌁</span><p>查询结果、引用和 trace 会显示在这里。</p></div></section>
  return <section className="debugger-result" aria-live="polite"><h2>结果</h2>{debugResult ? <div className="trace-card"><div><span className="eyebrow">Draft Debug · revision {debugResult.revision}</span><code>{debugResult.trace_id}</code></div><span className="muted">{debugResult.events.length} 个节点事件</span></div> : null}<div className="answer-card"><span className="eyebrow">Answer · {result.latency_ms} ms · cache {result.cache_status}</span><p>{result.answer || '服务没有返回答案。'}</p></div>{debugResult?.events.length ? <div className="trace-detail"><p><strong>节点诊断</strong> · draft revision {debugResult.revision}</p><ol>{debugResult.events.map((event) => <li key={`${event.sequence}-${event.node_id}`}><span>{event.sequence}. {event.node_id}</span><code>{event.latency_ms} ms</code>{event.error ? <small>{event.error}</small> : null}</li>)}</ol></div> : null}<h3>引用（{result.citations.length}）</h3>{result.citations.length === 0 ? <p className="muted">没有返回引用。</p> : <ul className="citation-list">{result.citations.map((citation) => <li key={`${citation.document_id}-${citation.chunk_id}`}><strong>{citation.section || citation.document_id}</strong><span>{citation.quote || citation.source_uri}</span></li>)}</ul>}<div className="trace-card"><div><span className="eyebrow">Trace ID</span><code>{result.trace_id}</code></div><button className="secondary-button" type="button" disabled={traceQuery.isPending} onClick={() => traceQuery.mutate(result.trace_id)}>{traceQuery.isPending ? '读取中…' : trace ? '刷新 Trace' : '查看 Trace'}</button></div>{trace ? <div className="trace-detail"><p><strong>{trace.node_spans.length}</strong> 个节点 · 总时延 <strong>{trace.latency_ms} ms</strong>{trace.has_error ? ' · 有错误' : ''}</p>{trace.pipeline_version_id || trace.release_id || trace.environment ? <dl className="trace-lineage"><dt>运行版本</dt><dd>{trace.pipeline_version_id ?? '未绑定'}</dd><dt>发布记录</dt><dd>{trace.release_id ?? '未绑定'}</dd><dt>环境</dt><dd>{trace.environment ?? '未绑定'}</dd></dl> : null}<ol>{trace.node_spans.map((span) => <li key={span.id}><span>{span.sequence}. {span.node_name}</span><code>{span.latency_ms} ms</code>{span.error ? <small>{span.error}</small> : null}</li>)}</ol></div> : null}</section>
}
