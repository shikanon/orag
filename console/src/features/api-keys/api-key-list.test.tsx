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
})
