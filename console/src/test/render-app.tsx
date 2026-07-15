import { render } from '@testing-library/react'
import { RouterProvider } from 'react-router-dom'
import { createAppRouter } from '../app/router'
import { AppProviders, createQueryClient } from '../app/providers'
import { clearSession, storeSession } from '../features/auth/session'

export function renderApp(path: string, options: { authenticated?: boolean } = {}) {
  if (options.authenticated === false) clearSession()
  else storeSession('test-access-token', 3600)
  const queryClient = createQueryClient()
  const router = createAppRouter([path])
  render(<AppProviders queryClient={queryClient}><RouterProvider router={router} future={{ v7_startTransition: true }} /></AppProviders>)
  return { queryClient, router }
}
