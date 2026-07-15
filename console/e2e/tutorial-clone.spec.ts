import { expect, test } from '@playwright/test'
import { authenticateConsole } from './session'

const tutorial = {
  id: 'text-rag', slug: 'text-rag', title: '中文文本 RAG', summary: '逐步启用解析、分块和检索策略。', version: '1.0.0', status: 'published', modality: 'text', difficulty: 'intermediate', estimated_duration_minutes: 45, source_benchmark: 'CRUD-RAG', source_url: 'https://github.com/IAAR-Shanghai/CRUD_RAG', scenario_dimensions: ['事实查询'], pipeline_stages: ['P0 基线'], required_capabilities: ['embedding'], packs: [{ tier: 'quick', manifest_url: 'https://public.example.invalid/manifest.json', estimated_bytes: 1024, estimated_minutes: 1, requires_license_check: true }, { tier: 'benchmark', manifest_url: 'https://public.example.invalid/benchmark-manifest.json', estimated_bytes: 2048, estimated_minutes: 2, requires_license_check: true }], replay_available: true,
}

const job = {
  id: 'tclj_clone', tenant_id: 'tenant_a', project_id: 'prj_clone', project_name: '中文文本 RAG 实验', project_description: '', template_id: 'text-rag', template_version: '1.0.0', pack_tier: 'quick', stage: 'pack_installed', status: 'completed', attempt: 1, events: [{ stage: 'pack_installed', outcome: 'completed', occurred_at: '2026-07-16T00:00:00Z' }], created_at: '2026-07-16T00:00:00Z', updated_at: '2026-07-16T00:00:00Z',
}

test.beforeEach(async ({ page }) => {
  await authenticateConsole(page)
  await page.route('**/v1/projects', (route) => route.fulfill({ json: { projects: [] } }))

  await page.route('**/v1/projects/prj_clone', (route) => route.fulfill({ json: { id: 'prj_clone', tenant_id: 'tenant_a', name: job.project_name, description: '', created_at: job.created_at, updated_at: job.updated_at } }))
  await page.route('**/v1/tutorials/text-rag', (route) => route.fulfill({ json: tutorial }))
  await page.route('**/v1/tutorials/text-rag/clones', (route) => route.fulfill({ status: 202, json: { job_id: job.id, project_id: job.project_id, poll_url: `/v1/tutorial-clone-jobs/${job.id}`, job } }))
  await page.route('**/v1/tutorial-clone-jobs/tclj_clone', (route) => route.fulfill({ json: job }))
  await page.route('**/v1/projects/prj_clone/tutorial-experiment', (route) => route.fulfill({ json: { id: 'texp_clone', tenant_id: 'tenant_a', project_id: job.project_id, template_id: job.template_id, template_version: job.template_version, pack_tier: job.pack_tier, pack_status: 'pack_installed', created_at: job.created_at, updated_at: job.updated_at } }))
})

test('clones a Quick Pack into a project and renders setup completion', async ({ page }) => {
  await page.goto('/tutorials/text-rag')
  await page.getByRole('button', { name: '克隆教程' }).click()
  await page.getByRole('radio', { name: /Quick Pack/ }).check()
  await page.getByRole('checkbox', { name: '我已确认数据许可' }).check()
  await page.getByRole('button', { name: '创建实验项目' }).click()
  await expect(page).toHaveURL(/\/projects\/prj_clone\/tutorial\/setup\?job=tclj_clone$/)
  await expect(page.getByText('Pack 已安装，Live Run 即将开放。')).toBeVisible()
  await expect(page.getByText(/access key|manifest_url|object_key/i)).toHaveCount(0)
})
