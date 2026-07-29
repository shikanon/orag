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
	  http.get('/v1/evaluation-metrics', () => HttpResponse.json({ items: [{ name: 'answer_accuracy', display_name: 'Answer accuracy', description: '回答命中参考答案的关键项。', category: 'answer_quality', direction: 'higher_is_better', formula: '关键项命中', requires: ['ground_truth'], caveats: ['不识别同义改写。'], related_metrics: [] }] })),
      http.post('/v1/datasets', () => HttpResponse.json({ id: 'ds_a', project_id: 'prj_a', tenant_id: 'tenant_a', name: 'Golden', kind: 'golden', version: '1', created_at: '' }, { status: 201 })),
      http.post('/v1/datasets/ds_a/items', () => HttpResponse.json({ id: 'item_a', dataset_id: 'ds_a', query: 'Q', ground_truth: 'A', relevant_doc_ids: [] }, { status: 201 })),
	  http.post('/v1/evaluations', () => HttpResponse.json({ id: 'eval_a', dataset_id: 'ds_a', profile: 'realtime', total: 1, accuracy: 1, hit_rate: 1, metrics: { answer_accuracy: 1 }, metric_summaries: { answer_accuracy: { value: 1, eligible_sample_count: 1, total_sample_count: 1, annotation_coverage: 1, weighted_sample_count: 1, effective_sample_count: 1 } }, created_at: '', holdout_gate: { enabled: true, passed: true, quality: 1 } }, { status: 202 })),
    )
    renderApp('/projects/prj_a/evaluations')
    await userEvent.type(await screen.findByLabelText('名称'), 'Golden')
    await userEvent.click(screen.getByRole('button', { name: '创建数据集' }))
    await screen.findByText('Dataset ID:', { exact: false })
    await userEvent.type(screen.getByLabelText('问题'), 'Q')
    await userEvent.type(screen.getByLabelText('期望答案'), 'A')
    await userEvent.click(screen.getByRole('button', { name: '添加样本' }))
    await userEvent.selectOptions(screen.getByLabelText('Knowledge Base'), 'kb_a')
    await userEvent.click(screen.getByRole('button', { name: '运行评测' }))
    expect(await screen.findByText('GATE PASSED')).toBeVisible()
    expect(screen.getAllByText('1.000')).not.toHaveLength(0)
	await userEvent.click(await screen.findByRole('button', { name: '了解指标' }))
	expect(await screen.findByText('计算方式：')).toBeVisible()
  })

  it('derives immutable environment evidence from selected server resources', async () => {
    const policies: Array<{ id: string; tenant_id: string; project_id: string; dataset_id: string; name: string; version: number; gates: Array<{ metric: string; comparator: 'gte'; threshold: number }>; created_at: string }> = []
    let evidenceBody: unknown
    server.use(
      http.get('/v1/knowledge-bases', () => HttpResponse.json({ items: [{ id: 'kb_a', project_id: 'prj_a', name: 'Support', description: '', tenant_id: 'tenant_a', created_at: '', updated_at: '' }] })),
      http.get('/v1/projects/prj_a/versions', () => HttpResponse.json({ items: [{ id: 'pv_frozen', project_id: 'prj_a', pipeline_id: 'pipe_a', content_hash: 'sha256:abc', created_at: '' }] })),
      http.get('/v1/projects/prj_a/evaluation-policies', () => HttpResponse.json({ items: policies })),
      http.post('/v1/datasets', () => HttpResponse.json({ id: 'ds_a', project_id: 'prj_a', tenant_id: 'tenant_a', name: 'Golden', kind: 'golden', version: '1', created_at: '' }, { status: 201 })),
      http.post('/v1/datasets/ds_a/items', () => HttpResponse.json({ id: 'item_a', dataset_id: 'ds_a', query: 'Q', ground_truth: 'A', relevant_doc_ids: [] }, { status: 201 })),
      http.post('/v1/evaluations', () => HttpResponse.json({ id: 'eval_a', dataset_id: 'ds_a', profile: 'realtime', total: 1, accuracy: 1, hit_rate: 1, created_at: '', holdout_gate: { enabled: true, passed: true, quality: 1 } }, { status: 202 })),
      http.post('/v1/projects/prj_a/evaluation-policies', async ({ request }) => {
        const body = await request.json() as { dataset_id: string; name: string; gates: Array<{ metric: string; comparator: 'gte'; threshold: number }> }
        expect(body).toEqual({ dataset_id: 'ds_a', name: 'Release quality', gates: [{ metric: 'answer_accuracy', comparator: 'gte', threshold: 0.8 }] })
        const policy = { id: 'epol_a', tenant_id: 'tenant_a', project_id: 'prj_a', dataset_id: 'ds_a', name: body.name, version: 1, gates: body.gates, created_at: '' }
        policies.push(policy)
        return HttpResponse.json(policy, { status: 201 })
      }),
      http.post('/v1/projects/prj_a/versions/pv_frozen/evaluation-evidence', async ({ request }) => {
        evidenceBody = await request.json()
        return HttpResponse.json({ id: 'epev_a', tenant_id: 'tenant_a', project_id: 'prj_a', policy_id: 'epol_a', policy_version: 1, evaluation_run_id: 'eval_a', pipeline_version_id: 'pv_frozen', content_hash: 'sha256:abc', environment: 'development', frozen_input: { policy_id: 'epol_a', policy_version: 1, project_id: 'prj_a', dataset_id: 'ds_a', evaluation_run_id: 'eval_a', pipeline_version: 'pv_frozen', content_hash: 'sha256:abc', environment: 'development', gates: [], metrics: {} }, gate_results: [], passed: true, created_at: '' }, { status: 201 })
      }),
    )
    renderApp('/projects/prj_a/evaluations')
    await userEvent.type(await screen.findByLabelText('名称'), 'Golden')
    await userEvent.click(screen.getByRole('button', { name: '创建数据集' }))
    await userEvent.type(screen.getByLabelText('问题'), 'Q')
    await userEvent.type(screen.getByLabelText('期望答案'), 'A')
    await userEvent.click(screen.getByRole('button', { name: '添加样本' }))
    await userEvent.selectOptions(screen.getByLabelText('Knowledge Base'), 'kb_a')
    await userEvent.click(screen.getByRole('button', { name: '运行评测' }))
    await screen.findByText('GATE PASSED')
    await userEvent.click(screen.getByRole('button', { name: '创建策略' }))
    const policySelect = await screen.findByLabelText('策略')
    await userEvent.selectOptions(policySelect, 'epol_a')
    await userEvent.selectOptions(screen.getByLabelText('冻结版本'), 'pv_frozen')
    await userEvent.click(screen.getByRole('button', { name: '派生环境证据' }))
    expect(evidenceBody).toEqual({ policy_id: 'epol_a', evaluation_run_id: 'eval_a', environment: 'development' })
    expect(await screen.findByText('Evidence passed · development')).toBeVisible()
  })
})
