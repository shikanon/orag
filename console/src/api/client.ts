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
export type Dataset = components['schemas']['Dataset']
export type CreateDatasetInput = components['schemas']['CreateDatasetRequest']
export type CreateDatasetItemInput = components['schemas']['CreateDatasetItemRequest']
export type RunEvaluationInput = components['schemas']['RunEvaluationRequest']
export type EvaluationResult = components['schemas']['RunEvaluationResponse']
export type Environment = components['schemas']['Environment']
export type EnvironmentKind = Environment['kind']
export type Release = components['schemas']['Release']
export type PromoteReleaseInput = components['schemas']['PromoteReleaseRequest']
export type RollbackReleaseInput = components['schemas']['RollbackReleaseRequest']
export type PipelineVersion = components['schemas']['PipelineVersion']
export type CreatePipelineVersionInput = components['schemas']['CreatePipelineVersionRequest']
export type ValidatePipelineVersionInput = components['schemas']['ValidatePipelineVersionRequest']
export type PipelineNodeDefinition = components['schemas']['PipelineNodeDefinition']
export type Pipeline = components['schemas']['Pipeline']
export type PipelineDraft = components['schemas']['PipelineDraft']
export type PipelineDebugRequest = components['schemas']['PipelineDebugRequest']
export type PipelineDebugResponse = components['schemas']['PipelineDebugResponse']
export type CreatePipelineInput = components['schemas']['CreatePipelineRequest']
export type SavePipelineDraftInput = components['schemas']['SavePipelineDraftRequest']

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

export const evaluationApi = {
  createDataset: (input: CreateDatasetInput) => request<Dataset>('/v1/datasets', { method: 'POST', body: JSON.stringify(input) }),
  addDatasetItem: (datasetId: string, input: CreateDatasetItemInput) => request<components['schemas']['DatasetItem']>(`/v1/datasets/${encodeURIComponent(datasetId)}/items`, { method: 'POST', body: JSON.stringify(input) }),
  run: (input: RunEvaluationInput) => request<EvaluationResult>('/v1/evaluations', { method: 'POST', body: JSON.stringify(input) }),
  get: (evaluationId: string) => request<EvaluationResult>(`/v1/evaluations/${encodeURIComponent(evaluationId)}`),
}

export const releaseApi = {
  versions: (projectId: string) => request<{ items: PipelineVersion[] }>(`/v1/projects/${encodeURIComponent(projectId)}/versions`),
  createVersion: (projectId: string, input: CreatePipelineVersionInput) => request<PipelineVersion>(`/v1/projects/${encodeURIComponent(projectId)}/versions`, { method: 'POST', body: JSON.stringify(input) }),
  validateVersion: (projectId: string, versionId: string, input: ValidatePipelineVersionInput) => request<{ version_id: string; environment: string; passed: boolean; content_hash: string }>(`/v1/projects/${encodeURIComponent(projectId)}/versions/${encodeURIComponent(versionId)}/validations`, { method: 'POST', body: JSON.stringify(input) }),
  environments: (projectId: string) => request<{ items: Environment[] }>(`/v1/projects/${encodeURIComponent(projectId)}/environments`),
  list: (projectId: string) => request<{ items: Release[] }>(`/v1/projects/${encodeURIComponent(projectId)}/releases`),
  promote: (projectId: string, input: PromoteReleaseInput) => request<Release>(`/v1/projects/${encodeURIComponent(projectId)}/releases:promote`, { method: 'POST', body: JSON.stringify(input) }),
  rollback: (projectId: string, environment: string, input: RollbackReleaseInput) => request<Release>(`/v1/projects/${encodeURIComponent(projectId)}/environments/${encodeURIComponent(environment)}/rollback`, { method: 'POST', body: JSON.stringify(input) }),
}

export const pipelineApi = {
  nodeDefinitions: () => request<{ items: PipelineNodeDefinition[] }>('/v1/pipeline-node-definitions'),
  list: (projectId: string) => request<{ items: Pipeline[] }>(`/v1/projects/${encodeURIComponent(projectId)}/pipelines`),
  create: (projectId: string, input: CreatePipelineInput) => request<Pipeline>(`/v1/projects/${encodeURIComponent(projectId)}/pipelines`, { method: 'POST', body: JSON.stringify(input) }),
  draft: (projectId: string, pipelineId: string) => request<PipelineDraft>(`/v1/projects/${encodeURIComponent(projectId)}/pipelines/${encodeURIComponent(pipelineId)}/draft`),
  saveDraft: (projectId: string, pipelineId: string, input: SavePipelineDraftInput) => request<PipelineDraft>(`/v1/projects/${encodeURIComponent(projectId)}/pipelines/${encodeURIComponent(pipelineId)}/draft`, { method: 'PUT', body: JSON.stringify(input) }),
  debug: (projectId: string, input: PipelineDebugRequest) => request<PipelineDebugResponse>(`/v1/projects/${encodeURIComponent(projectId)}/query:debug`, { method: 'POST', body: JSON.stringify(input) }),
  saveCase: (projectId: string, runId: string, input: { dataset_id: string; query: string; ground_truth: string; expected_evidence?: string[] }) => request<{ run_id: string; item: components['schemas']['DatasetItem'] }>(`/v1/projects/${encodeURIComponent(projectId)}/debug-runs/${encodeURIComponent(runId)}/save-case`, { method: 'POST', body: JSON.stringify(input) }),
}
