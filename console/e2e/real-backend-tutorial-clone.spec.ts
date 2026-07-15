import { expect, test } from '@playwright/test'

const realBackendEnabled = (globalThis as { process?: { env?: Record<string, string | undefined> } }).process?.env?.ORAG_REAL_TUTORIAL_CLONE_E2E === '1'

test.describe('real PostgreSQL + Qdrant tutorial Pack installation', () => {
  test.skip(!realBackendEnabled, 'requires scripts/console-real-backend-tutorial-clone-e2e.sh')
  test.setTimeout(120_000)

  test('creates a project and installs the server-verified Quick Pack', async ({ page }) => {
    await page.goto('/login')
    await page.getByLabel('用户名').fill('e2e-admin')
    await page.getByLabel('密码').fill('e2e-password')
    await page.getByRole('button', { name: '登录' }).click()
    await expect(page).toHaveURL(/\/projects$/)

    await page.goto('/tutorials/text-rag')
    await page.getByRole('button', { name: '克隆教程' }).click()
    await page.getByRole('radio', { name: /Quick Pack/ }).check()
    await page.getByRole('checkbox', { name: '我已确认数据许可' }).check()
    await page.getByRole('button', { name: '创建实验项目' }).click()
    await expect(page).toHaveURL(/\/projects\/prj_[^/]+\/tutorial\/setup\?job=tclj_[^/]+$/)
    await expect(page.getByText('Pack 已安装，Live Run 即将开放。')).toBeVisible({ timeout: 60_000 })
    await expect(page.getByText(/access key|manifest_url|object_key/i)).toHaveCount(0)
  })
})
