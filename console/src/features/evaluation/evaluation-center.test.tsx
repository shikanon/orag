import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { server } from '../../test/handlers'
import { renderApp } from '../../test/render-app'

describe('EvaluationCenter', () => {
  it('creates a dataset, adds a sample, and runs a gate', async () => {
    server.use(
      http.get('/v1/knowledge-bases', () => HttpResponse.json({ items: [{ id: 'kb_a', project_id: 'prj_a', name: 'Support', description: '', tenant_id: 'tenant_a', created_at: '', updated_at: '' }] })),
      http.post('/v1/datasets', () => HttpResponse.json({ id: 'ds_a', project_id: 'prj_a', tenant_id: 'tenant_a', name: 'Golden', kind: 'golden', version: '1', created_at: '' }, { status: 201 })),
      http.post('/v1/datasets/ds_a/items', () => HttpResponse.json({ id: 'item_a', dataset_id: 'ds_a', query: 'Q', ground_truth: 'A', relevant_doc_ids: [] }, { status: 201 })),
      http.post('/v1/evaluations', () => HttpResponse.json({ id: 'eval_a', dataset_id: 'ds_a', profile: 'realtime', total: 1, accuracy: 1, hit_rate: 1, created_at: '', holdout_gate: { enabled: true, passed: true, quality: 1 } }, { status: 202 })),
    )
    renderApp('/projects/prj_a/evaluations')
    await userEvent.type(await screen.findByLabelText('名称'), 'Golden')
    await userEvent.click(screen.getByRole('button', { name: '创建数据集' }))
    await screen.findByText('Dataset ID: ds_a')
    await userEvent.type(screen.getByLabelText('问题'), 'Q')
    await userEvent.type(screen.getByLabelText('期望答案'), 'A')
    await userEvent.click(screen.getByRole('button', { name: '添加样本' }))
    await userEvent.selectOptions(screen.getByLabelText('Knowledge Base'), 'kb_a')
    await userEvent.click(screen.getByRole('button', { name: '运行评测' }))
    expect(await screen.findByText('GATE PASSED')).toBeVisible()
    expect(screen.getByText('1.000')).toBeVisible()
  })
})
