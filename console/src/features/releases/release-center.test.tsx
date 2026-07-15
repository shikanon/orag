import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { server } from '../../test/handlers'
import { renderApp } from '../../test/render-app'

describe('ReleaseCenter', () => {
  it('shows environment state and sends an optimistic promotion request', async () => {
    server.use(
      http.get('/v1/projects/prj_a/environments', () => HttpResponse.json({ items: [{ id: 'dev', project_id: 'prj_a', kind: 'development', active_version_id: 'pv_1', revision: 0, bound: true }, { id: 'stg', project_id: 'prj_a', kind: 'staging', revision: 0, bound: true }, { id: 'prd', project_id: 'prj_a', kind: 'production', revision: 0, bound: true }] })),
      http.get('/v1/projects/prj_a/releases', () => HttpResponse.json({ items: [] })),
      http.post('/v1/projects/prj_a/releases:promote', async ({ request }) => { const body = await request.json() as { target_version_id: string; expected_active_version_id: string }; expect(body.target_version_id).toBe('pv_1'); expect(body.expected_active_version_id).toBe(''); return HttpResponse.json({ id: 'rel_1', project_id: 'prj_a', source_version_id: 'pv_1', target_version_id: 'pv_1', source_environment: 'development', target_environment: 'staging', action: 'promote', actor: 'user:admin', created_at: '' }, { status: 201 }) }),
    )
    renderApp('/projects/prj_a/releases')
    expect(await screen.findByText('pv_1')).toBeVisible()
    await userEvent.type(screen.getByLabelText('不可变版本 ID'), 'pv_1')
    await userEvent.click(screen.getByRole('button', { name: '执行晋级' }))
    expect(await screen.findByText('pv_1')).toBeVisible()
  })
})
