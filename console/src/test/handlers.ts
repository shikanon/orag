import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'

export const projects = [
  { id: 'prj_a', tenant_id: 'tenant_a', name: 'Support', description: 'Customer answers', created_at: '2026-07-11T00:00:00Z', updated_at: '2026-07-11T00:00:00Z' },
  { id: 'prj_b', tenant_id: 'tenant_a', name: 'Search', description: 'Product discovery', created_at: '2026-07-11T00:00:00Z', updated_at: '2026-07-11T00:00:00Z' },
]

const apiKeys = [
  { id: 'key_active', tenant_id: 'tenant_a', project_id: 'prj_a', name: 'Evaluation runner', prefix: 'orag_sk_key_active', role: 'project_editor' as const, created_by: 'user:admin', created_at: '2026-07-11T00:00:00Z' },
]

const packs = (id: string) => [
  { tier: 'quick', manifest_url: `https://orag.oss-cn-guangzhou.aliyuncs.com/tutorial-packs/${id}/1.0.0/quick/manifest.json`, estimated_bytes: 536870912, estimated_minutes: 20, requires_license_check: true },
  { tier: 'benchmark', manifest_url: `https://orag.oss-cn-guangzhou.aliyuncs.com/tutorial-packs/${id}/1.0.0/benchmark/manifest.json`, estimated_bytes: 4294967296, estimated_minutes: 90, requires_license_check: true },
]

export const tutorials = [
  { id: 'text-rag', slug: 'text-rag', title: '中文文本 RAG', summary: '逐步启用解析、分块、上下文检索、多路召回、Rewrite 与 Rerank。', version: '1.0.0', status: 'published', modality: 'text', difficulty: 'intermediate', estimated_duration_minutes: 45, source_benchmark: 'CRUD-RAG', source_url: 'https://github.com/IAAR-Shanghai/CRUD_RAG', scenario_dimensions: ['事实查询', '多跳问题', '显式否定', '错误前提', '信息不足'], pipeline_stages: ['P0 基线', 'P1 文档解析', 'P2 Chunking', 'P3 Contextual Retrieval', 'P4 稀疏召回', 'P5 多路召回', 'P6 Rewrite', 'P7 Rerank', 'P8 组合策略'], required_capabilities: ['embedding', 'rewrite', 'rerank'], packs: packs('text-rag'), replay_available: true },
  { id: 'visual-document-rag', slug: 'visual-document-rag', title: '视觉文档 RAG', summary: '对比文本抽取、视觉解析、分块和多模态检索策略。', version: '1.0.0', status: 'published', modality: 'visual_document', difficulty: 'advanced', estimated_duration_minutes: 60, source_benchmark: 'ViDoSeek', source_url: 'https://modelscope.cn/datasets/iic/ViDoSeek', scenario_dimensions: ['文本', '表格', '图表', '二维版面', '视觉否定', '信息不足'], pipeline_stages: ['P0 基线', 'P1 文档解析', 'P2 Chunking', 'P3 Contextual Retrieval', 'P4 稀疏召回', 'P5 多路召回', 'P6 Rewrite', 'P7 Rerank', 'P8 组合策略'], required_capabilities: ['visual_parser', 'multimodal_retrieval', 'rerank'], packs: packs('visual-document-rag'), replay_available: true },
  { id: 'video-rag', slug: 'video-rag', title: '视频 RAG', summary: '将视频理解、时间片段证据、字幕和音频信息纳入同一检索链路。', version: '1.0.0', status: 'published', modality: 'video', difficulty: 'advanced', estimated_duration_minutes: 75, source_benchmark: 'Video-MME', source_url: 'https://video-mme.github.io/home_page.html', scenario_dimensions: ['短视频', '中视频', '长视频', '视觉信息', '字幕', '音频', '时间否定', '错误前提', '信息不足'], pipeline_stages: ['P0 基线', 'P1 视频理解与打标', 'P2 时间分段', 'P3 Contextual Retrieval', 'P4 字幕召回', 'P5 多路召回', 'P6 Rewrite', 'P7 Rerank', 'P8 组合策略'], required_capabilities: ['doubao_seed_video_understanding', 'temporal_index', 'embedding', 'rerank'], packs: packs('video-rag'), replay_available: true },
]

export const server = setupServer(
  http.post('/v1/auth/login', async ({ request }) => {
    const input = await request.json() as { username: string; password: string }
    return input.username === 'admin' && input.password === 'admin'
      ? HttpResponse.json({ access_token: 'signed-admin-token', token_type: 'Bearer', expires_in: 3600 })
      : HttpResponse.json({ code: 'invalid_credentials', message: 'invalid username or password' }, { status: 401 })
  }),
  http.get('/v1/projects', () => HttpResponse.json({ projects })),
  http.get('/v1/projects/:projectId', ({ params }) => HttpResponse.json(projects.find((project) => project.id === params.projectId))),
  http.get('/v1/knowledge-bases', () => HttpResponse.json({ items: [] })),
  http.post('/v1/query', () => HttpResponse.json({ answer: 'Mock answer', citations: [], retrieved_chunks: [], trace_id: 'trace_mock', cache_status: 'bypass', profile: 'realtime', latency_ms: 1, created_at: '2026-07-11T00:00:00Z' })),
  http.get('/v1/traces/:traceId', ({ params }) => HttpResponse.json({ trace_id: params.traceId, tenant_id: 'tenant_a', profile: 'realtime', latency_ms: 1, created_at: '2026-07-11T00:00:00Z', has_error: false, error_count: 0, node_spans: [] })),
  http.get('/v1/api-keys', () => HttpResponse.json({ api_keys: apiKeys })),
  http.post('/v1/api-keys', async ({ request }) => {
    const input = await request.json() as { name: string; role: 'tenant_admin' | 'project_editor' | 'project_viewer'; project_id?: string }
    const apiKey = { id: 'key_new', tenant_id: 'tenant_a', name: input.name, prefix: 'orag_sk_key_new', role: input.role, project_id: input.project_id, created_by: 'user:admin', created_at: '2026-07-11T00:00:00Z' }
    return HttpResponse.json({ api_key: apiKey, secret: 'orag_sk_key_new_secret' }, { status: 201 })
  }),
  http.delete('/v1/api-keys/:apiKeyId', () => new HttpResponse(null, { status: 204 })),
  http.get('/v1/tutorials', () => HttpResponse.json({ tutorials })),
  http.get('/v1/tutorials/:templateId', ({ params }) => {
    const tutorial = tutorials.find((item) => item.id === params.templateId)
    return tutorial ? HttpResponse.json(tutorial) : new HttpResponse(null, { status: 404 })
  }),
)
