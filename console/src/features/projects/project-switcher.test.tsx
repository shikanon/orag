import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import { renderApp } from '../../test/render-app'

describe('ProjectSwitcher', () => {
  it('switches project route without leaking the previous project cache', async () => {
    const { queryClient, router } = renderApp('/projects/prj_a/overview')

    await userEvent.click(await screen.findByRole('button', { name: /Support/ }))
    await userEvent.click(screen.getByRole('option', { name: /Search/ }))

    expect(router.state.location.pathname).toBe('/projects/prj_b/overview')
    expect(queryClient.getQueryData(['projects', 'prj_a'])).toBeDefined()
    expect(queryClient.getQueryData(['projects', 'prj_b'])).toBeDefined()
  })
})
