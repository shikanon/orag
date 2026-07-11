import type { components } from './schema'

export type Project = components['schemas']['Project']
export type CreateProjectInput = components['schemas']['CreateProjectRequest']

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, { ...init, headers: { 'Content-Type': 'application/json', ...init?.headers } })
  if (!response.ok) throw new Error(`ORAG API request failed (${response.status})`)
  return response.json() as Promise<T>
}

export const projectApi = {
  list: () => request<{ projects: Project[] }>('/v1/projects'),
  get: (projectId: string) => request<Project>(`/v1/projects/${projectId}`),
  create: (input: CreateProjectInput) => request<Project>('/v1/projects', { method: 'POST', body: JSON.stringify(input) }),
}
