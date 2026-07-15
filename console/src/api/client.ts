import type { components } from './schema'

export type Project = components['schemas']['Project']
export type CreateProjectInput = components['schemas']['CreateProjectRequest']
export type APIKey = components['schemas']['APIKey']
export type CreateAPIKeyInput = components['schemas']['CreateAPIKeyRequest']
export type CreateAPIKeyResponse = components['schemas']['CreateAPIKeyResponse']
export type TutorialTemplate = components['schemas']['TutorialTemplate']

export class ApiError extends Error {
  constructor(public readonly status: number) {
    super(`ORAG API request failed (${status})`)
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, { ...init, headers: { 'Content-Type': 'application/json', ...init?.headers } })
  if (!response.ok) throw new ApiError(response.status)
  return response.json() as Promise<T>
}

async function requestVoid(path: string, init?: RequestInit): Promise<void> {
  const response = await fetch(path, { ...init, headers: { 'Content-Type': 'application/json', ...init?.headers } })
  if (!response.ok) throw new ApiError(response.status)
}

export const projectApi = {
  list: () => request<{ projects: Project[] }>('/v1/projects'),
  get: (projectId: string) => request<Project>(`/v1/projects/${projectId}`),
  create: (input: CreateProjectInput) => request<Project>('/v1/projects', { method: 'POST', body: JSON.stringify(input) }),
}

export const apiKeyApi = {
  list: () => request<{ api_keys: APIKey[] }>('/v1/api-keys'),
  create: (input: CreateAPIKeyInput) => request<CreateAPIKeyResponse>('/v1/api-keys', { method: 'POST', body: JSON.stringify(input) }),
  revoke: (apiKeyId: string) => requestVoid(`/v1/api-keys/${encodeURIComponent(apiKeyId)}`, { method: 'DELETE' }),
}

export const tutorialApi = {
  list: () => request<{ tutorials: TutorialTemplate[] }>('/v1/tutorials'),
  get: (templateId: string) => request<TutorialTemplate>(`/v1/tutorials/${encodeURIComponent(templateId)}`),
  getVersion: (templateId: string, version: string) => request<TutorialTemplate>(
    `/v1/tutorials/${encodeURIComponent(templateId)}/versions/${encodeURIComponent(version)}`,
  ),
}
