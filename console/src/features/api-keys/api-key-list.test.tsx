import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { HttpResponse, http } from 'msw'
import { describe, expect, it, vi } from 'vitest'
import { server } from '../../test/handlers'
import { renderApp } from '../../test/render-app'

describe('APIKeyList', () => {
  it('shows a newly created secret once and clears it when the dialog closes', async () => {
    const user = userEvent.setup()
    renderApp('/api-keys')

    await user.click(await screen.findByRole('button', { name: '创建 API Key' }))
    const dialog = screen.getByRole('dialog')
    await user.type(within(dialog).getByLabelText('名称'), 'CI runner')
    await user.selectOptions(within(dialog).getByLabelText('项目'), 'prj_a')
    await user.click(within(dialog).getByRole('button', { name: /^创建$/ }))

    expect(await screen.findByRole('heading', { name: '立即保存 API Key' })).toBeVisible()
    expect(screen.getByTestId('created-api-key-secret')).toHaveTextContent('orag_sk_key_new_secret')
    await user.click(screen.getByRole('button', { name: '我已安全保存' }))
    expect(screen.queryByText('orag_sk_key_new_secret')).not.toBeInTheDocument()
    expect(await screen.findByText('CI runner')).toBeVisible()
  })

  it('requires confirmation before revoking an active key', async () => {
    const user = userEvent.setup()
    const revoked = vi.fn()
    server.use(http.delete('/v1/api-keys/:apiKeyId', ({ params }) => { revoked(params.apiKeyId); return new HttpResponse(null, { status: 204 }) }))
    renderApp('/api-keys')

    await user.click(await screen.findByRole('button', { name: '撤销' }))
    const dialog = screen.getByRole('alertdialog')
    expect(within(dialog).getByText(/所有自动化请求会立即失效/)).toBeVisible()
    await user.click(within(dialog).getByRole('button', { name: '确认撤销' }))
    await waitFor(() => expect(revoked).toHaveBeenCalledWith('key_active'))
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
    expect(screen.getByText('已撤销')).toBeVisible()
  })

  it('rotates an active key and displays the replacement secret once', async () => {
    const user = userEvent.setup()
    const rotated = vi.fn()
    server.use(http.post('/v1/api-keys/:apiKeyId/rotate', ({ params }) => { rotated(params.apiKeyId); return HttpResponse.json({ api_key: { id: 'key_rotated', tenant_id: 'tenant_a', name: 'Active runner', prefix: 'orag_sk_key_rotated', role: 'project_editor', project_id: 'prj_a', created_by: 'user:admin', created_at: '2026-07-11T00:00:00Z', rotated_from_key_id: params.apiKeyId }, secret: 'orag_sk_key_rotated_secret' }, { status: 201 }) }))
    renderApp('/api-keys')

    await user.click(await screen.findByRole('button', { name: '轮换' }))
    const dialog = screen.getByRole('alertdialog')
    expect(within(dialog).getByText(/立即撤销当前密钥/)).toBeVisible()
    await user.click(within(dialog).getByRole('button', { name: '确认轮换' }))
    await waitFor(() => expect(rotated).toHaveBeenCalledWith('key_active'))
    expect(await screen.findByTestId('rotated-api-key-secret')).toHaveTextContent('orag_sk_key_rotated_secret')
    await user.click(screen.getByRole('button', { name: '我已安全保存' }))
    expect(screen.queryByText('orag_sk_key_rotated_secret')).not.toBeInTheDocument()
  })
})
