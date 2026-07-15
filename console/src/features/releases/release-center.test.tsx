import { screen, waitFor } from '@testing-library/react'
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

  it('activates an evaluated frozen version in development with the observed revision', async () => {
    let activationRequested = false
    server.use(
      http.get('/v1/projects/prj_a/environments', () => HttpResponse.json({ items: [{ id: 'dev', project_id: 'prj_a', kind: 'development', revision: 0, bound: true }, { id: 'stg', project_id: 'prj_a', kind: 'staging', revision: 0, bound: true }, { id: 'prd', project_id: 'prj_a', kind: 'production', revision: 0, bound: true }] })),
      http.get('/v1/projects/prj_a/releases', () => HttpResponse.json({ items: [] })),
      http.get('/v1/projects/prj_a/versions', () => HttpResponse.json({ items: [{ id: 'pv_2', project_id: 'prj_a', pipeline_id: 'pipe_1', content_hash: 'sha256:2', created_at: '2026-07-16T00:00:00Z' }] })),
      http.post('/v1/projects/prj_a/environments/development/activate', async ({ request }) => {
        const body = await request.json() as { target_version_id: string; expected_active_version_id: string }
        expect(body).toEqual({ target_version_id: 'pv_2', expected_active_version_id: '' })
        activationRequested = true
        return HttpResponse.json({ id: 'rel_2', project_id: 'prj_a', target_version_id: 'pv_2', source_environment: 'development', target_environment: 'development', action: 'activate', actor: 'user:admin', created_at: '2026-07-16T00:00:00Z' }, { status: 201 })
      }),
    )
    renderApp('/projects/prj_a/releases')
    const developmentVersion = await screen.findByLabelText('选择要激活到 development 的版本')
    await waitFor(() => expect(developmentVersion).toHaveTextContent('pv_2'))
    await userEvent.selectOptions(developmentVersion, 'pv_2')
    const activate = screen.getByRole('button', { name: '激活 development' })
    await waitFor(() => expect(activate).toBeEnabled())
    await userEvent.click(activate)
    await waitFor(() => expect(activationRequested).toBe(true))
  })
})
