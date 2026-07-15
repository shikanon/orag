import { useState, type FormEvent } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { Link, useParams } from 'react-router-dom'
import { knowledgeBaseApi, queryApi, type QueryResponse, type TraceRecord } from '../../api/client'

export function APIDebugger() {
  const { projectId = '' } = useParams()
  const [knowledgeBaseId, setKnowledgeBaseId] = useState('')
  const [question, setQuestion] = useState('')
  const [profile, setProfile] = useState<'realtime' | 'high_precision'>('realtime')
  const [result, setResult] = useState<QueryResponse | null>(null)
  const [trace, setTrace] = useState<TraceRecord | null>(null)
  const bases = useQuery({ queryKey: ['knowledge-bases'], queryFn: knowledgeBaseApi.list })
  const query = useMutation({ mutationFn: queryApi.run, onSuccess: (response) => { setResult(response); setTrace(null) } })
  const traceQuery = useMutation({ mutationFn: queryApi.getTrace, onSuccess: setTrace })
  const projectBases = (bases.data?.items ?? []).filter((item) => !item.project_id || item.project_id === projectId)

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (knowledgeBaseId.trim() && question.trim()) query.mutate({ knowledge_base_id: knowledgeBaseId.trim(), query: question.trim(), profile })
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
        {query.isError ? <p className="debugger-error" role="alert">查询失败，请确认 API Key、知识库权限和服务状态。</p> : null}
        <button className="primary-button" disabled={query.isPending || !knowledgeBaseId.trim() || !question.trim()}>{query.isPending ? '运行中…' : '运行 RAG 查询'}</button>
      </form>
      <ResultPanel result={result} trace={trace} traceQuery={traceQuery} />
    </section>
  </main>
}

function ResultPanel({ result, trace, traceQuery }: { result: QueryResponse | null; trace: TraceRecord | null; traceQuery: ReturnType<typeof useMutation<TraceRecord, Error, string>> }) {
  if (!result) return <section className="debugger-result"><h2>结果</h2><div className="debugger-placeholder"><span>⌁</span><p>查询结果、引用和 trace 会显示在这里。</p></div></section>
  return <section className="debugger-result" aria-live="polite"><h2>结果</h2><div className="answer-card"><span className="eyebrow">Answer · {result.latency_ms} ms · cache {result.cache_status}</span><p>{result.answer || '服务没有返回答案。'}</p></div><h3>引用（{result.citations.length}）</h3>{result.citations.length === 0 ? <p className="muted">没有返回引用。</p> : <ul className="citation-list">{result.citations.map((citation) => <li key={`${citation.document_id}-${citation.chunk_id}`}><strong>{citation.section || citation.document_id}</strong><span>{citation.quote || citation.source_uri}</span></li>)}</ul>}<div className="trace-card"><div><span className="eyebrow">Trace ID</span><code>{result.trace_id}</code></div><button className="secondary-button" type="button" disabled={traceQuery.isPending} onClick={() => traceQuery.mutate(result.trace_id)}>{traceQuery.isPending ? '读取中…' : trace ? '刷新 Trace' : '查看 Trace'}</button></div>{trace ? <div className="trace-detail"><p><strong>{trace.node_spans.length}</strong> 个节点 · 总时延 <strong>{trace.latency_ms} ms</strong>{trace.has_error ? ' · 有错误' : ''}</p><ol>{trace.node_spans.map((span) => <li key={span.id}><span>{span.sequence}. {span.node_name}</span><code>{span.latency_ms} ms</code>{span.error ? <small>{span.error}</small> : null}</li>)}</ol></div> : null}</section>
}
