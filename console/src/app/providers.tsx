import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'

export function createQueryClient() {
  return new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: 30_000 } } })
}

export function AppProviders({ children, queryClient }: { children: ReactNode; queryClient: QueryClient }) {
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
}
