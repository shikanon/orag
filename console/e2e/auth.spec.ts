import { expect, test } from '@playwright/test'

test('logs in, preserves the requested destination, and logs out', async ({ page }) => {
  let authorization = ''
  await page.route('**/v1/auth/login', async (route) => route.fulfill({ json: { access_token: 'browser-admin-token', token_type: 'Bearer', expires_in: 3600 } }))
  await page.route('**/v1/projects', async (route) => {
    authorization = route.request().headers().authorization ?? ''
    return route.fulfill({ json: { projects: [] } })
  })
  await page.route('**/v1/api-keys', async (route) => route.fulfill({ json: { api_keys: [] } }))

  await page.goto('/api-keys')
  await expect(page.getByRole('heading', { name: '登录 ORAG Console' })).toBeVisible()
  await page.getByLabel('用户名').fill('admin')
  await page.getByLabel('密码').fill('admin')
  await page.getByRole('button', { name: '登录' }).click()
  await expect(page).toHaveURL(/\/api-keys$/)
  await expect(page.getByRole('heading', { name: 'API Keys' })).toBeVisible()
  expect(authorization).toBe('Bearer browser-admin-token')

  await page.getByRole('button', { name: '退出' }).click()
  await expect(page.getByRole('heading', { name: '登录 ORAG Console' })).toBeVisible()
})
