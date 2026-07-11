import { render } from '@testing-library/react'
import { RouterProvider } from 'react-router-dom'
import { createAppRouter } from '../app/router'
import { AppProviders, createQueryClient } from '../app/providers'

export function renderApp(path: string) {
  const queryClient = createQueryClient()
  const router = createAppRouter([path])
  render(<AppProviders queryClient={queryClient}><RouterProvider router={router} future={{ v7_startTransition: true }} /></AppProviders>)
  return { queryClient, router }
}
