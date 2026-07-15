import { expect, test } from '@playwright/test'
import { authenticateConsole } from './session'

test('creates a candidate, records evidence, and promotes it', async ({ page }) => {
  await authenticateConsole(page)
  await page.route('**/v1/projects/prj_a/environments', (route) => route.fulfill({ json: { items: [
    { id: 'dev', project_id: 'prj_a', kind: 'development', active_version_id: 'pv_1', revision: 0, bound: true },
    { id: 'stg', project_id: 'prj_a', kind: 'staging', revision: 0, bound: true },
    { id: 'prd', project_id: 'prj_a', kind: 'production', revision: 0, bound: true },
  ] } }))
  await page.route('**/v1/projects/prj_a/releases', (route) => route.fulfill({ json: { items: [] } }))
  await page.route('**/v1/projects/prj_a/versions', async (route) => {
    if (route.request().method() === 'POST') return route.fulfill({ json: { id: 'pv_new', project_id: 'prj_a', content_hash: 'sha256:abc', created_at: '' }, status: 201 })
    return route.fulfill({ json: { items: [{ id: 'pv_new', project_id: 'prj_a', content_hash: 'sha256:abc', created_at: '' }] } })
  })
  await page.route('**/v1/projects/prj_a/versions/pv_new/validations', (route) => route.fulfill({ json: { version_id: 'pv_new', environment: 'staging', passed: true, content_hash: 'sha256:abc' }, status: 201 }))
  await page.route('**/v1/projects/prj_a/releases:promote', (route) => route.fulfill({ json: { id: 'rel_new', project_id: 'prj_a', source_version_id: 'pv_1', target_version_id: 'pv_new', source_environment: 'development', target_environment: 'staging', action: 'promote', actor: 'user:e2e', created_at: '' }, status: 201 }))

  await page.goto('/projects/prj_a/releases')
  await page.getByLabel('内容 hash').first().fill('sha256:abc')
  await page.getByRole('button', { name: '创建版本' }).click()
  await expect(page.getByRole('listitem').getByText('pv_new')).toBeVisible()
  await page.locator('select').first().selectOption('pv_new')
  await page.getByRole('button', { name: '记录通过证据' }).click()
  await page.getByPlaceholder('pv_...').first().fill('pv_new')
  await page.getByRole('button', { name: '执行晋级' }).click()
  await expect(page.getByRole('listitem').getByText('pv_new')).toBeVisible()
})
