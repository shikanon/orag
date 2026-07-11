import { screen, waitFor } from '@testing-library/react'
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

  it('selects projects from the keyboard and closes with Escape', async () => {
    const user = userEvent.setup()
    const { router } = renderApp('/projects/prj_a/overview')
    const trigger = await screen.findByRole('button', { name: /Support/ })

    await user.click(trigger)
    await user.keyboard('{End}{Escape}')
    expect(screen.queryByRole('listbox')).not.toBeInTheDocument()
    expect(trigger).toHaveFocus()

    await user.keyboard('{Enter}{End}')
    expect(screen.getByRole('option', { name: /Search/ })).toHaveFocus()
    await user.keyboard('{Home}')
    expect(screen.getByRole('option', { name: /Support/ })).toHaveFocus()
    await user.keyboard('{ArrowUp}[Space]')
    expect(router.state.location.pathname).toBe('/projects/prj_b/overview')
    await waitFor(() => expect(screen.getByRole('button', { name: /Search/ })).toHaveFocus())
  })

  it('tabs to the new project action and restores trigger focus on Escape', async () => {
    const user = userEvent.setup()
    renderApp('/projects/prj_a/overview')
    const trigger = await screen.findByRole('button', { name: /Support/ })

    await user.click(trigger)
    await user.keyboard('{End}{Tab}')
    expect(screen.getByRole('button', { name: /新建项目/ })).toHaveFocus()
    await user.keyboard('{Escape}')

    expect(screen.queryByRole('listbox')).not.toBeInTheDocument()
    expect(trigger).toHaveFocus()
  })
})
