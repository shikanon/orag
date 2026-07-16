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

const completedCloneJob = {
  id: 'tclj_clone', tenant_id: 'tenant_a', project_id: 'prj_clone', project_name: '中文文本 RAG 实验', project_description: 'Mock clone', template_id: 'text-rag', template_version: '1.0.0', pack_tier: 'quick', stage: 'pack_installed', status: 'completed', attempt: 1, events: [{ stage: 'pack_installed', outcome: 'completed', occurred_at: '2026-07-16T00:00:00Z' }], created_at: '2026-07-16T00:00:00Z', updated_at: '2026-07-16T00:00:00Z',
} as const

export const tutorialExperiment = {
  id: 'texp_clone', tenant_id: 'tenant_a', project_id: completedCloneJob.project_id,
  template_id: 'text-rag', template_version: '1.0.0', pack_tier: 'quick', pack_status: 'pack_installed',
  runtime_status: 'ready', knowledge_base_id: 'tkb_clone', dataset_id: 'tds_clone', baseline_profile: 'realtime', baseline_top_k: 5,
  variants: [
    { id: 'baseline', chapter: 'p0_basic_baseline', parser_method: 'basic', chunk_size_tokens: 800, chunk_overlap_tokens: 120, contextual_retrieval: false, available: true },
    { id: 'p1_structured_json', chapter: 'p1_document_parser', parser_method: 'structured_json', chunk_size_tokens: 800, chunk_overlap_tokens: 120, contextual_retrieval: false, available: true },
    { id: 'p2_recursive_400_80', chapter: 'p2_chunking', parser_method: 'basic', chunk_size_tokens: 400, chunk_overlap_tokens: 80, contextual_retrieval: false, available: true },
    { id: 'p3_contextual_retrieval', chapter: 'p3_contextual_retrieval', parser_method: 'basic', chunk_size_tokens: 800, chunk_overlap_tokens: 120, contextual_retrieval: true, available: true },
  ],
  created_at: '2026-07-16T00:00:00Z', updated_at: '2026-07-16T00:00:00Z',
} as const

export const completedTutorialRun = {
  id: 'terun_clone', tenant_id: 'tenant_a', project_id: completedCloneJob.project_id, experiment_id: tutorialExperiment.id,
  variant: 'baseline', parser_method: 'basic', chunk_size_tokens: 800, chunk_overlap_tokens: 120, contextual_retrieval_enabled: false, indexed_chunk_count: 2, average_chunk_tokens: 600, contextualized_chunk_count: 0, average_context_tokens: 0, comparison_fingerprint: 'comparison_fingerprint', knowledge_base_id: 'tkb_clone', dataset_id: 'tds_clone', profile: 'realtime', top_k: 5, stage: 'completed', status: 'completed', evaluation_run_id: 'eval_tutorial_clone',
  events: [{ stage: 'index_private_pack', outcome: 'completed', occurred_at: '2026-07-16T00:00:00Z' }, { stage: 'run_evaluation', outcome: 'completed', occurred_at: '2026-07-16T00:00:01Z' }, { stage: 'completed', outcome: 'completed', occurred_at: '2026-07-16T00:00:02Z' }],
  created_at: '2026-07-16T00:00:00Z', updated_at: '2026-07-16T00:00:02Z',
} as const

const completedTutorialP1Run = {
  id: 'terun_clone_p1', tenant_id: 'tenant_a', project_id: completedCloneJob.project_id, experiment_id: tutorialExperiment.id,
  variant: 'p1_structured_json', baseline_run_id: completedTutorialRun.id, parser_method: 'structured_json', chunk_size_tokens: 800, chunk_overlap_tokens: 120, indexed_chunk_count: 2, average_chunk_tokens: 600, comparison_fingerprint: 'comparison_fingerprint', definition_fingerprint: 'p1_definition_fingerprint', knowledge_base_id: 'tkb_clone_p1', dataset_id: 'tds_clone', profile: 'realtime', top_k: 5, stage: 'completed', status: 'completed', evaluation_run_id: 'eval_tutorial_clone_p1',
  events: [{ stage: 'index_private_pack', outcome: 'completed', occurred_at: '2026-07-16T00:01:00Z' }, { stage: 'run_evaluation', outcome: 'completed', occurred_at: '2026-07-16T00:01:01Z' }, { stage: 'completed', outcome: 'completed', occurred_at: '2026-07-16T00:01:02Z' }],
  created_at: '2026-07-16T00:01:00Z', updated_at: '2026-07-16T00:01:02Z',
} as const

const tutorialComparison = {
  baseline: completedTutorialRun,
  candidate: completedTutorialP1Run,
  comparable: true,
  metrics: [{ name: 'accuracy', baseline: 0.5, candidate: 0.75, absolute_delta: 0.25, relative_delta: 0.5 }],
  index_metrics: [{ name: 'average_chunk_tokens', baseline: 600, candidate: 600, absolute_delta: 0, relative_delta: 0 }, { name: 'chunk_count', baseline: 2, candidate: 2, absolute_delta: 0, relative_delta: 0 }],
} as const

const completedTutorialP2Run = {
  id: 'terun_clone_p2', tenant_id: 'tenant_a', project_id: completedCloneJob.project_id, experiment_id: tutorialExperiment.id,
  variant: 'p2_recursive_400_80', baseline_run_id: completedTutorialRun.id, parser_method: 'basic', chunk_size_tokens: 400, chunk_overlap_tokens: 80, indexed_chunk_count: 4, average_chunk_tokens: 320, comparison_fingerprint: 'comparison_fingerprint', definition_fingerprint: 'p2_definition_fingerprint', knowledge_base_id: 'tkb_clone_p2', dataset_id: 'tds_clone', profile: 'realtime', top_k: 5, stage: 'completed', status: 'completed', evaluation_run_id: 'eval_tutorial_clone_p2',
  events: [{ stage: 'index_private_pack', outcome: 'completed', occurred_at: '2026-07-16T00:02:00Z' }, { stage: 'run_evaluation', outcome: 'completed', occurred_at: '2026-07-16T00:02:01Z' }, { stage: 'completed', outcome: 'completed', occurred_at: '2026-07-16T00:02:02Z' }],
  created_at: '2026-07-16T00:02:00Z', updated_at: '2026-07-16T00:02:02Z',
} as const

const completedTutorialP3Run = {
  id: 'terun_clone_p3', tenant_id: 'tenant_a', project_id: completedCloneJob.project_id, experiment_id: tutorialExperiment.id,
  variant: 'p3_contextual_retrieval', baseline_run_id: completedTutorialRun.id, parser_method: 'basic', chunk_size_tokens: 800, chunk_overlap_tokens: 120, contextual_retrieval_enabled: true, indexed_chunk_count: 2, average_chunk_tokens: 600, contextualized_chunk_count: 2, average_context_tokens: 18, comparison_fingerprint: 'comparison_fingerprint', definition_fingerprint: 'p3_definition_fingerprint', knowledge_base_id: 'tkb_clone_p3', dataset_id: 'tds_clone', profile: 'realtime', top_k: 5, stage: 'completed', status: 'completed', evaluation_run_id: 'eval_tutorial_clone_p3',
  events: [{ stage: 'index_private_pack', outcome: 'completed', occurred_at: '2026-07-16T00:03:00Z' }, { stage: 'run_evaluation', outcome: 'completed', occurred_at: '2026-07-16T00:03:01Z' }, { stage: 'completed', outcome: 'completed', occurred_at: '2026-07-16T00:03:02Z' }],
  created_at: '2026-07-16T00:03:00Z', updated_at: '2026-07-16T00:03:02Z',
} as const

const tutorialP3Comparison = {
  baseline: completedTutorialRun, candidate: completedTutorialP3Run, comparable: true,
  metrics: [{ name: 'accuracy', baseline: 0.5, candidate: 0.75, absolute_delta: 0.25, relative_delta: 0.5 }],
  index_metrics: [{ name: 'average_chunk_tokens', baseline: 600, candidate: 600, absolute_delta: 0, relative_delta: 0 }, { name: 'chunk_count', baseline: 2, candidate: 2, absolute_delta: 0, relative_delta: 0 }, { name: 'contextualized_chunk_count', baseline: 0, candidate: 2, absolute_delta: 2 }, { name: 'average_context_tokens', baseline: 0, candidate: 18, absolute_delta: 18 }],
} as const

const tutorialP2Comparison = {
  baseline: completedTutorialRun,
  candidate: completedTutorialP2Run,
  comparable: true,
  metrics: [{ name: 'accuracy', baseline: 0.5, candidate: 0.75, absolute_delta: 0.25, relative_delta: 0.5 }],
  index_metrics: [{ name: 'average_chunk_tokens', baseline: 600, candidate: 320, absolute_delta: -280, relative_delta: -0.4667 }, { name: 'chunk_count', baseline: 2, candidate: 4, absolute_delta: 2, relative_delta: 1 }],
} as const

export const server = setupServer(
  http.post('/v1/auth/login', async ({ request }) => {
    const input = await request.json() as { username: string; password: string }
    return input.username === 'admin' && input.password === 'admin'
      ? HttpResponse.json({ access_token: 'signed-admin-token', token_type: 'Bearer', expires_in: 3600 })
      : HttpResponse.json({ code: 'invalid_credentials', message: 'invalid username or password' }, { status: 401 })
  }),
  http.get('/v1/projects', () => HttpResponse.json({ projects })),
  http.get('/v1/projects/:projectId', ({ params }) => HttpResponse.json(projects.find((project) => project.id === params.projectId))),
  http.get('/v1/projects/:projectId/pipelines', () => HttpResponse.json({ items: [] })),
  http.get('/v1/projects/:projectId/versions', () => HttpResponse.json({ items: [] })),
  http.get('/v1/projects/:projectId/evaluation-policies', () => HttpResponse.json({ items: [] })),
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
  http.post('/v1/tutorials/:templateId/clones', async ({ params, request }) => {
    const input = await request.json() as { project?: { name?: string } }
    const template = tutorials.find((item) => item.id === params.templateId)
    if (!template || !input.project?.name) return new HttpResponse(null, { status: 400 })
    const job = { ...completedCloneJob, project_name: input.project.name, template_id: template.id, template_version: template.version }
    return HttpResponse.json({ job_id: job.id, project_id: job.project_id, poll_url: `/v1/tutorial-clone-jobs/${job.id}`, job }, { status: 202 })
  }),
  http.get('/v1/tutorial-clone-jobs/:jobId', ({ params }) => params.jobId === completedCloneJob.id ? HttpResponse.json(completedCloneJob) : new HttpResponse(null, { status: 404 })),
  http.post('/v1/tutorial-clone-jobs/:jobId:retry', ({ params }) => params.jobId === completedCloneJob.id ? HttpResponse.json(completedCloneJob, { status: 202 }) : new HttpResponse(null, { status: 404 })),
)

export function useTutorialLiveRunHandlers() {
  server.use(
    http.get('/v1/projects/prj_clone/tutorial-experiment', () => HttpResponse.json(tutorialExperiment)),
    http.post('/v1/projects/prj_clone/tutorial-experiments/texp_clone/runs', async ({ request }) => {
      const input = await request.json() as { variant: string }
      const run = input.variant === 'p1_structured_json' ? completedTutorialP1Run : input.variant === 'p2_recursive_400_80' ? completedTutorialP2Run : input.variant === 'p3_contextual_retrieval' ? completedTutorialP3Run : completedTutorialRun
      return HttpResponse.json({ run_id: run.id, poll_url: `/v1/projects/${completedCloneJob.project_id}/tutorial-experiments/${tutorialExperiment.id}/runs/${run.id}`, run }, { status: 202 })
    }),
    http.get('/v1/projects/prj_clone/tutorial-experiments/texp_clone/runs/terun_clone', () => HttpResponse.json(completedTutorialRun)),
    http.get('/v1/projects/prj_clone/tutorial-experiments/texp_clone/runs/terun_clone_p1', () => HttpResponse.json(completedTutorialP1Run)),
    http.get('/v1/projects/prj_clone/tutorial-experiments/texp_clone/runs/terun_clone_p1/comparison', () => HttpResponse.json(tutorialComparison)),
    http.get('/v1/projects/prj_clone/tutorial-experiments/texp_clone/runs/terun_clone_p2', () => HttpResponse.json(completedTutorialP2Run)),
    http.get('/v1/projects/prj_clone/tutorial-experiments/texp_clone/runs/terun_clone_p2/comparison', () => HttpResponse.json(tutorialP2Comparison)),
    http.get('/v1/projects/prj_clone/tutorial-experiments/texp_clone/runs/terun_clone_p3', () => HttpResponse.json(completedTutorialP3Run)),
    http.get('/v1/projects/prj_clone/tutorial-experiments/texp_clone/runs/terun_clone_p3/comparison', () => HttpResponse.json(tutorialP3Comparison)),
    http.post('/v1/projects/prj_clone/tutorial-experiments/texp_clone/runs/terun_clone:cancel', () => HttpResponse.json(completedTutorialRun, { status: 202 })),
  )
}
