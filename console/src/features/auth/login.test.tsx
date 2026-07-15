import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { HttpResponse, http } from 'msw'
import { describe, expect, it, vi } from 'vitest'
import { server } from '../../test/handlers'
import { renderApp } from '../../test/render-app'

describe('Console authentication', () => {
  it('returns to the requested page and sends the Bearer token after login', async () => {
    const authorization = vi.fn()
    server.use(http.get('/v1/api-keys', ({ request }) => {
      authorization(request.headers.get('Authorization'))
      return HttpResponse.json({ api_keys: [] })
    }))
    const user = userEvent.setup()
    const { router } = renderApp('/api-keys', { authenticated: false })

    expect(await screen.findByRole('heading', { name: '登录 ORAG Console' })).toBeVisible()
    await user.type(screen.getByLabelText('用户名'), 'admin')
    await user.type(screen.getByLabelText('密码'), 'admin')
    await user.click(screen.getByRole('button', { name: '登录' }))

    expect(await screen.findByRole('heading', { name: 'API Keys' })).toBeVisible()
    expect(router.state.location.pathname).toBe('/api-keys')
    await waitFor(() => expect(authorization).toHaveBeenCalledWith('Bearer signed-admin-token'))
  })

  it('keeps invalid credentials on the login page', async () => {
    const user = userEvent.setup()
    renderApp('/login', { authenticated: false })
    await user.type(screen.getByLabelText('用户名'), 'admin')
    await user.type(screen.getByLabelText('密码'), 'wrong')
    await user.click(screen.getByRole('button', { name: '登录' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('用户名或密码错误')
  })

  it('invalidates the session after an authenticated 401 response', async () => {
    server.use(http.get('/v1/projects', () => HttpResponse.json({ code: 'invalid_bearer_token' }, { status: 401 })))
    const { router } = renderApp('/projects')
    expect(await screen.findByRole('heading', { name: '登录 ORAG Console' })).toBeVisible()
    expect(router.state.location.pathname).toBe('/login')
  })
})
