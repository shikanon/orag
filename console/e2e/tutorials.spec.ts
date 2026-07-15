import { expect, test } from '@playwright/test'
import { authenticateConsole } from './session'

const makeTutorial = (id: string, title: string, modality: 'text' | 'visual_document' | 'video', source: string) => ({
  id, slug: id, title, summary: `${title} 端到端实验。`, version: '1.0.0', status: 'published', modality,
  difficulty: modality === 'text' ? 'intermediate' : 'advanced', estimated_duration_minutes: 60,
  source_benchmark: source, source_url: 'https://example.test/source',
  scenario_dimensions: modality === 'video' ? ['短视频', '时间否定', '信息不足'] : ['事实查询', '显式否定', '信息不足'],
  pipeline_stages: ['P0 基线', 'P1 文档解析', 'P2 Chunking', 'P3 Contextual Retrieval', 'P4 稀疏召回', 'P5 多路召回', 'P6 Rewrite', 'P7 Rerank', 'P8 组合策略'],
  required_capabilities: ['embedding', 'rerank'], replay_available: true,
  packs: ['quick', 'benchmark'].map((tier) => ({ tier, manifest_url: `https://orag.oss-cn-guangzhou.aliyuncs.com/tutorial-packs/${id}/1.0.0/${tier}/manifest.json`, estimated_bytes: tier === 'quick' ? 536870912 : 4294967296, estimated_minutes: tier === 'quick' ? 20 : 90, requires_license_check: true })),
})

const tutorials = [
  makeTutorial('text-rag', '中文文本 RAG', 'text', 'CRUD-RAG'),
  makeTutorial('visual-document-rag', '视觉文档 RAG', 'visual_document', 'ViDoSeek'),
  makeTutorial('video-rag', '视频 RAG', 'video', 'Video-MME'),
]

test.beforeEach(async ({ page }) => {
  await authenticateConsole(page)
  await page.route('**/v1/projects', async (route) => route.fulfill({ json: { projects: [] } }))
  await page.route('**/v1/tutorials', async (route) => route.fulfill({ json: { tutorials } }))
  await page.route('**/v1/tutorials/*', async (route) => route.fulfill({ json: tutorials.find((item) => route.request().url().endsWith(item.id)) }))
})

test('opens the video tutorial from the global catalog', async ({ page }) => {
  await page.goto('/projects')
  await page.getByRole('link', { name: '教程实验室' }).click()
  await expect(page).toHaveURL(/\/tutorials$/)
  await page.getByRole('link', { name: /视频 RAG/ }).click()
  await expect(page).toHaveURL(/\/tutorials\/video-rag$/)
  await expect(page.getByText('Video-MME').first()).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Quick Pack' })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Benchmark Pack' })).toBeVisible()
  const clone = page.getByRole('button', { name: '克隆教程' })
  await expect(clone).toBeEnabled()
  const box = await clone.boundingBox()
  expect(box?.width).toBeGreaterThan(120)
  expect(box?.height).toBeLessThan(60)
})
