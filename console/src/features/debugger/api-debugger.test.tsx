import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { server } from '../../test/handlers'
import { renderApp } from '../../test/render-app'

describe('API Debugger', () => {
  it('runs a query and reveals its trace', async () => {
    server.use(
      http.get('/v1/knowledge-bases', () => HttpResponse.json({ items: [{ id: 'kb_a', project_id: 'prj_a', name: 'Support', description: '', tenant_id: 'tenant_a', created_at: '', updated_at: '' }] })),
      http.post('/v1/query', () => HttpResponse.json({ answer: 'Use the trace endpoint.', citations: [{ chunk_id: 'c1', document_id: 'd1', source_uri: 'guide.md', quote: 'trace endpoint' }], retrieved_chunks: [], trace_id: 'trace_debug', cache_status: 'miss', profile: 'realtime', latency_ms: 42, created_at: '2026-07-11T00:00:00Z' })),
      http.get('/v1/traces/trace_debug', () => HttpResponse.json({ trace_id: 'trace_debug', tenant_id: 'tenant_a', profile: 'realtime', latency_ms: 42, created_at: '2026-07-11T00:00:00Z', has_error: false, error_count: 0, node_spans: [{ id: 'span1', node_name: 'retrieve', sequence: 1, latency_ms: 12, started_at: '', ended_at: '', created_at: '' }] })),
    )
    renderApp('/projects/prj_a/debug')
    await userEvent.selectOptions(await screen.findByLabelText('Knowledge Base'), 'kb_a')
    await userEvent.type(screen.getByLabelText('查询问题'), '如何查看 trace？')
    await userEvent.click(screen.getByRole('button', { name: '运行 RAG 查询' }))
    expect(await screen.findByText('Use the trace endpoint.')).toBeVisible()
    await userEvent.click(screen.getByRole('button', { name: '查看 Trace' }))
    expect(await screen.findByText(/1 个节点/)).toBeVisible()
    expect(screen.getByText('retrieve')).toBeVisible()
  })
})
