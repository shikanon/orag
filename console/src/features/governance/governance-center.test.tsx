import { screen } from '@testing-library/dom'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { server } from '../../test/handlers'
import { renderApp } from '../../test/render-app'

describe('GovernanceCenter', () => {
  it('shows project tasks, audit activity, and a retryable task timeline', async () => {
    server.use(
      http.get('/v1/tasks', () => HttpResponse.json({ data: [{ id: 'task_dead', tenant_id: 'tenant_a', project_id: 'prj_a', type: 'evaluation.run', pool: 'evaluation', status: 'dead_letter', attempt: 3, max_attempts: 3, error_code: 'provider_timeout', created_at: '2026-07-26T09:00:00Z', updated_at: '2026-07-26T09:10:00Z' }] })),
      http.get('/v1/projects/:projectId/audit-events', () => HttpResponse.json({ data: [{ id: 'audit_1', project_id: 'prj_a', actor_type: 'user', actor_id: 'admin', action: 'evaluation.requested', resource_type: 'task', resource_id: 'task_dead', outcome: 'success', created_at: '2026-07-26T09:00:00Z' }] })),
      http.get('/v1/tasks/:taskId/events', () => HttpResponse.json({ data: [{ id: 'event_1', task_id: 'task_dead', type: 'dead_lettered', created_at: '2026-07-26T09:10:00Z' }] })),
      http.post('/v1/tasks/:taskId:retry', () => HttpResponse.json({ task_id: 'task_dead', status: 'queued' })),
    )
    renderApp('/projects/prj_a/governance')
    expect(await screen.findByText('任务治理')).toBeVisible()
    expect(await screen.findByText('evaluation.run')).toBeVisible()
    expect(await screen.findByText('evaluation.requested')).toBeVisible()
    await userEvent.click(screen.getByRole('button', { name: /evaluation\.run/ }))
    expect(await screen.findByText('dead_lettered')).toBeVisible()
    await userEvent.click(screen.getByRole('button', { name: '重试任务' }))
  })
})
