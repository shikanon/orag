import { expect, test } from '@playwright/test'
import { authenticateConsole } from './session'

const projects = [
  { id: 'prj_a', tenant_id: 'tenant_a', name: 'Support', description: 'Customer answers', created_at: '2026-07-11T00:00:00Z', updated_at: '2026-07-11T00:00:00Z' },
  { id: 'prj_b', tenant_id: 'tenant_a', name: 'Search', description: 'Product discovery', created_at: '2026-07-11T00:00:00Z', updated_at: '2026-07-11T00:00:00Z' },
]

let projectDetailRequests = 0

test.beforeEach(async ({ page }) => {
  projectDetailRequests = 0
  await authenticateConsole(page)
  await page.route('**/v1/projects', async (route) => {
    if (route.request().method() === 'POST') {
      const input = route.request().postDataJSON()
      return route.fulfill({ json: { ...projects[0], id: 'prj_new', ...input } })
    }
    return route.fulfill({ json: { projects } })
  })
  await page.route('**/v1/projects/*', async (route) => {
    projectDetailRequests += 1
    return route.fulfill({ json: projects.find((item) => route.request().url().endsWith(item.id)) ?? projects[0] })
  })
})

test('switches projects and creates a project', async ({ page }) => {
  await page.goto('/projects/prj_a/overview')
  await expect(page.getByRole('button', { name: /Support/ })).toBeVisible()
  expect(projectDetailRequests).toBe(0)
  await page.getByRole('button', { name: /Support/ }).click()
  await page.getByRole('option', { name: /Search/ }).click()
  await expect(page).toHaveURL(/projects\/prj_b\/overview/)

  await page.getByRole('button', { name: /Search/ }).click()
  await page.getByRole('button', { name: /新建项目/ }).click()
  await page.getByLabel('项目名称').fill('Knowledge Ops')
  await page.getByLabel('项目说明').fill('Internal knowledge workflows')
  await page.getByRole('button', { name: '创建项目' }).click()
  await expect(page).toHaveURL(/projects\/prj_new\/overview/)
})

test('prefetches a feature route on intent and reports network state', async ({ page }) => {
  const requested: string[] = []
  page.on('request', (request) => requested.push(request.url()))

  await page.goto('/projects')
  await expect(page.getByText('网络在线')).toBeVisible()
  await page.getByRole('link', { name: 'API Keys' }).hover()
  await expect.poll(() => requested.some((url) => url.includes('/features/api-keys/api-key-list.tsx'))).toBe(true)

  await page.context().setOffline(true)
  await expect(page.getByText('网络离线')).toBeVisible()
  await page.context().setOffline(false)
  await expect(page.getByText('网络在线')).toBeVisible()
})
