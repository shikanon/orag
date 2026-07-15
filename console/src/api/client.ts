import type { components } from './schema'
import { clearSession, getAccessToken } from '../features/auth/session'

export type Project = components['schemas']['Project']
export type CreateProjectInput = components['schemas']['CreateProjectRequest']
export type APIKey = components['schemas']['APIKey']
export type CreateAPIKeyInput = components['schemas']['CreateAPIKeyRequest']
export type CreateAPIKeyResponse = components['schemas']['CreateAPIKeyResponse']
export type TutorialTemplate = components['schemas']['TutorialTemplate']
export type LoginInput = components['schemas']['LoginRequest']
export type LoginResponse = components['schemas']['LoginResponse']
export type KnowledgeBase = components['schemas']['KnowledgeBase']
export type QueryRequest = components['schemas']['QueryRequest']
export type QueryResponse = components['schemas']['QueryResponse']
export type TraceRecord = components['schemas']['TraceRecord']

export class ApiError extends Error {
  constructor(public readonly status: number) {
    super(`ORAG API request failed (${status})`)
  }
}

async function request<T>(path: string, init?: RequestInit, authenticated = true): Promise<T> {
  const token = authenticated ? getAccessToken() : null
  const response = await fetch(path, { ...init, headers: { 'Content-Type': 'application/json', ...(token ? { Authorization: `Bearer ${token}` } : {}), ...init?.headers } })
  if (!response.ok) {
    if (authenticated && response.status === 401) clearSession()
    throw new ApiError(response.status)
  }
  return response.json() as Promise<T>
}

async function requestVoid(path: string, init?: RequestInit): Promise<void> {
  const token = getAccessToken()
  const response = await fetch(path, { ...init, headers: { 'Content-Type': 'application/json', ...(token ? { Authorization: `Bearer ${token}` } : {}), ...init?.headers } })
  if (!response.ok) {
    if (response.status === 401) clearSession()
    throw new ApiError(response.status)
  }
}

export const authApi = {
  login: (input: LoginInput) => request<LoginResponse>('/v1/auth/login', { method: 'POST', body: JSON.stringify(input) }, false),
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

export const knowledgeBaseApi = {
  list: () => request<{ items: KnowledgeBase[] }>('/v1/knowledge-bases'),
}

export const queryApi = {
  run: (input: QueryRequest) => request<QueryResponse>('/v1/query', { method: 'POST', body: JSON.stringify(input) }),
  getTrace: (traceId: string) => request<TraceRecord>(`/v1/traces/${encodeURIComponent(traceId)}`),
}
