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
    await expect(page.getByText('Pack 已安装。支持运行声明的文本 Quick Pack 可进入基线 Live Run。')).toBeVisible({ timeout: 60_000 })
    await page.getByRole('link', { name: '打开基线 Live Run' }).click()
    await expect(page.getByRole('heading', { name: '文本 Quick 单变量实验' })).toBeVisible()
    await page.getByRole('button', { name: '运行 P0 基线' }).click()
    await expect(page.getByText('标准评测 Run')).toBeVisible({ timeout: 60_000 })
    await page.getByRole('button', { name: '运行 P5 多查询候选' }).click()
    await expect(page.getByRole('heading', { name: 'P0 与候选使用相同的对比输入' })).toBeVisible({ timeout: 60_000 })
    await expect(page.getByText('查询扩展', { exact: true }).locator('..').getByText('multi_query/3', { exact: true })).toBeVisible()
    await expect(page.getByText('兼容 P0 父运行')).toBeVisible()
    await expect(page.getByText(/access key|manifest_url|object_key/i)).toHaveCount(0)
  })
})
