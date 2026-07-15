import { expect, test } from '@playwright/test'

const projects = [
  { id: 'prj_a', tenant_id: 'tenant_a', name: 'Support', description: 'Customer answers', created_at: '2026-07-11T00:00:00Z', updated_at: '2026-07-11T00:00:00Z' },
]

const activeKey = { id: 'key_active', tenant_id: 'tenant_a', project_id: 'prj_a', name: 'Evaluation runner', prefix: 'orag_sk_key_active', role: 'project_editor', created_by: 'user:admin', created_at: '2026-07-11T00:00:00Z' }

test.beforeEach(async ({ page }) => {
  await page.route('**/v1/projects', async (route) => route.fulfill({ json: { projects } }))
  await page.route('**/v1/api-keys', async (route) => {
    if (route.request().method() === 'POST') {
      const input = route.request().postDataJSON()
      return route.fulfill({ status: 201, json: { api_key: { ...activeKey, id: 'key_new', prefix: 'orag_sk_key_new', ...input }, secret: 'orag_sk_key_new_one_time_secret' } })
    }
    return route.fulfill({ json: { api_keys: [activeKey] } })
  })
  await page.route('**/v1/api-keys/*', async (route) => route.fulfill({ status: 204, body: '' }))
})

test('creates, clears, and revokes API keys', async ({ page }) => {
  await page.goto('/api-keys')
  await expect(page.getByRole('heading', { name: 'API Keys' })).toBeVisible()

  await page.getByRole('button', { name: '创建 API Key' }).click()
  await page.getByLabel('名称').fill('CI runner')
  await page.getByRole('dialog').locator('label').filter({ hasText: /^项目/ }).locator('select').selectOption('prj_a')
  await page.getByRole('button', { name: '创建', exact: true }).click()
  await expect(page.getByTestId('created-api-key-secret')).toHaveText('orag_sk_key_new_one_time_secret')
  await page.getByRole('button', { name: '我已安全保存' }).click()
  await expect(page.getByText('orag_sk_key_new_one_time_secret')).toHaveCount(0)
  await expect(page.getByText('CI runner')).toBeVisible()

  await page.getByRole('button', { name: '撤销' }).first().click()
  await page.getByRole('button', { name: '确认撤销' }).click()
  await expect(page.getByText('已撤销')).toBeVisible()
})
