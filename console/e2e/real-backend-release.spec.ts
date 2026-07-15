import { expect, test, type Page } from '@playwright/test'

const realBackendEnabled = (globalThis as { process?: { env?: Record<string, string | undefined> } }).process?.env?.ORAG_REAL_BACKEND_E2E === '1'

test.describe('real PostgreSQL + Qdrant release lifecycle', () => {
  test.skip(!realBackendEnabled, 'requires scripts/console-real-backend-e2e.sh')
  test.setTimeout(120_000)

  test('runs immutable versions through evidence, production, trace lineage, and rollback', async ({ page }) => {
    await page.goto('/login')
    await page.getByLabel('用户名').fill('e2e-admin')
    await page.getByLabel('密码').fill('e2e-password')
    await page.getByRole('button', { name: '登录' }).click()
    await expect(page).toHaveURL(/\/projects$/)

    await page.getByRole('link', { name: '新建项目' }).click()
    await page.getByLabel('项目名称').fill('Console real backend E2E')
    await page.getByLabel('项目说明').fill('PostgreSQL and Qdrant browser release lifecycle')
    await page.getByRole('button', { name: '创建项目' }).click()
    await expect(page).toHaveURL(/\/projects\/prj_[^/]+\/overview$/)
    const projectID = page.url().match(/\/projects\/([^/]+)\/overview$/)?.[1]
    expect(projectID).toBeTruthy()

    const knowledgeBaseID = await createProjectKnowledgeBase(page, projectID!)
    await page.goto(`/projects/${projectID}/releases`)
    await bindAllEnvironments(page)

    await page.goto(`/projects/${projectID}/studio`)
    const versionOne = await createFrozenVersion(page, 'E2E primary pipeline')
    const versionTwo = await createFrozenVersion(page, 'E2E replacement pipeline')
    expect(versionTwo).not.toBe(versionOne)

    await page.goto(`/projects/${projectID}/evaluations`)
    const policyID = await createEvaluationPolicy(page, knowledgeBaseID)
    for (const versionID of [versionOne, versionTwo]) {
      for (const environment of ['development', 'staging', 'production'] as const) {
        await deriveEvidence(page, policyID, versionID, environment)
      }
    }

    await page.goto(`/projects/${projectID}/releases`)
    await activateDevelopment(page, versionOne)
    await promote(page, 'development', 'staging', versionOne)
    await promote(page, 'staging', 'production', versionOne)
    await activateDevelopment(page, versionTwo)
    await promote(page, 'development', 'staging', versionTwo)
    await promote(page, 'staging', 'production', versionTwo)

    await page.goto(`/projects/${projectID}/debug`)
    await page.getByLabel('Knowledge Base').selectOption(knowledgeBaseID)
    await page.getByLabel('查询问题').fill('Which immutable version serves production?')
    await page.getByRole('button', { name: '运行 RAG 查询' }).click()
    await expect(page.getByRole('button', { name: '查看 Trace' })).toBeVisible()
    await page.getByRole('button', { name: '查看 Trace' }).click()
    const lineage = page.locator('.trace-lineage')
    await expect(lineage).toContainText(versionTwo)
    await expect(lineage).toContainText('production')
    await expect(lineage.locator('dd').nth(1)).not.toHaveText('未绑定')

    await page.goto(`/projects/${projectID}/releases`)
    const rollback = page.getByRole('heading', { name: '原子回滚' }).locator('..')
    await rollback.getByLabel('环境').selectOption('production')
    await rollback.getByLabel('目标版本 ID').fill(versionOne)
    await rollback.getByLabel('原因').fill('Verify an evaluated production rollback')
    const rollbackResponse = page.waitForResponse((response) => response.request().method() === 'POST' && response.url().endsWith('/environments/production/rollback'))
    await rollback.getByRole('button', { name: '执行回滚' }).click()
    expect((await rollbackResponse).status()).toBe(201)
    await expect(environmentCard(page, '生产')).toContainText(versionOne)
    await expect(page.getByRole('heading', { name: '追加式发布历史' }).locator('..')).toContainText('回滚')
  })
})

async function createProjectKnowledgeBase(page: Page, projectID: string): Promise<string> {
  return page.evaluate(async (project) => {
    const session = JSON.parse(sessionStorage.getItem('orag.console.session.v1') ?? '{}') as { accessToken?: string }
    const response = await fetch('/v1/knowledge-bases', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${session.accessToken ?? ''}` },
      body: JSON.stringify({ name: 'E2E knowledge base', description: 'Real backend fixture', project_id: project }),
    })
    if (!response.ok) throw new Error(`knowledge base setup failed: ${response.status}`)
    return (await response.json() as { id: string }).id
  }, projectID)
}

async function bindAllEnvironments(page: Page) {
  const binding = page.getByRole('heading', { name: '环境资源绑定' }).locator('..')
  for (const environment of ['development', 'staging', 'production'] as const) {
    await binding.getByLabel('环境').selectOption(environment)
    await binding.getByLabel('绑定引用').fill(`e2e://${environment}`)
    const response = page.waitForResponse((item) => item.request().method() === 'PUT' && item.url().endsWith(`/environments/${environment}/binding`))
    await binding.getByRole('button', { name: '保存环境绑定' }).click()
    expect((await response).status()).toBe(200)
    await expect(binding.getByLabel('绑定引用')).toHaveValue('')
  }
  await expect(page.getByText('e2e://development')).toHaveCount(0)
  for (const label of ['开发', '预发', '生产'] as const) await expect(environmentCard(page, label)).toContainText('已绑定')
}

async function createFrozenVersion(page: Page, name: string): Promise<string> {
  const toolbar = page.locator('.studio-toolbar')
  await toolbar.getByLabel('Pipeline name').fill(name)
  await toolbar.getByRole('button', { name: '创建 Pipeline' }).click()
  await expect(page.locator('.success-note')).toContainText('已创建 pipe_')
  await page.getByRole('button', { name: '填充标准 RAG 链路' }).click()
  await page.getByRole('button', { name: /保存 Draft/ }).click()
  await expect(page.locator('.success-note')).toContainText('已保存 revision 1')
  const versionResponse = page.waitForResponse((response) => response.request().method() === 'POST' && /\/pipelines\/[^/]+\/versions$/.test(new URL(response.url()).pathname))
  await page.getByRole('button', { name: '创建不可变版本' }).click()
  const body = await (await versionResponse).json() as { version: { id: string } }
  await expect(page.locator('.success-note')).toContainText(`已创建不可变版本 ${body.version.id}`)
  return body.version.id
}

async function createEvaluationPolicy(page: Page, knowledgeBaseID: string): Promise<string> {
  const dataset = page.locator('section.evaluation-card').filter({ hasText: '1. 创建数据集' })
  await dataset.getByLabel('名称').fill('E2E release dataset')
  await dataset.getByRole('button', { name: '创建数据集' }).click()
  await expect(dataset.locator('.success-note code')).toBeVisible()

  const sample = page.locator('section.evaluation-card').filter({ hasText: '2. 添加样本' })
  await sample.getByLabel('问题').fill('What is the expected release behavior?')
  await sample.getByLabel('期望答案').fill('A production release is evaluated and traceable.')
  await sample.getByRole('button', { name: '添加样本' }).click()

  const evaluation = page.locator('section.evaluation-card').filter({ hasText: '3. 运行评测' })
  await evaluation.getByLabel('Knowledge Base').selectOption(knowledgeBaseID)
  const evaluationResponse = page.waitForResponse((response) => response.request().method() === 'POST' && response.url().endsWith('/evaluations'))
  await evaluation.getByRole('button', { name: '运行评测' }).click()
  expect((await evaluationResponse).status()).toBe(202)
  await expect(page.getByText('Run ID')).toBeVisible()

  const policy = page.locator('section.evaluation-card').filter({ hasText: '4. 创建不可变策略' })
  await policy.getByLabel('质量阈值').fill('0')
  const policyResponse = page.waitForResponse((response) => response.request().method() === 'POST' && /\/evaluation-policies$/.test(new URL(response.url()).pathname))
  await policy.getByRole('button', { name: '创建策略' }).click()
  const body = await (await policyResponse).json() as { id: string }
  await expect(policy).toContainText('已加载 1 个不可变策略')
  return body.id
}

async function deriveEvidence(page: Page, policyID: string, versionID: string, environment: 'development' | 'staging' | 'production') {
  const evidence = page.locator('section.evaluation-card').filter({ hasText: '5. 派生环境证据' })
  await evidence.getByLabel('策略').selectOption(policyID)
  await evidence.getByLabel('冻结版本').selectOption(versionID)
  await evidence.getByLabel('目标环境').selectOption(environment)
  const response = page.waitForResponse((item) => item.request().method() === 'POST' && item.url().endsWith(`/versions/${versionID}/evaluation-evidence`))
  await evidence.getByRole('button', { name: '派生环境证据' }).click()
  expect((await response).status()).toBe(201)
  await expect(evidence).toContainText(`Evidence passed · ${environment}`)
}

async function activateDevelopment(page: Page, versionID: string) {
  const activation = page.getByRole('heading', { name: '激活 development' }).locator('..')
  await activation.getByLabel('选择要激活到 development 的版本').selectOption(versionID)
  const response = page.waitForResponse((item) => item.request().method() === 'POST' && item.url().endsWith('/environments/development/activate'))
  await activation.getByRole('button', { name: '激活 development' }).click()
  expect((await response).status()).toBe(201)
  await expect(environmentCard(page, '开发')).toContainText(versionID)
}

async function promote(page: Page, source: 'development' | 'staging', target: 'staging' | 'production', versionID: string) {
  const form = page.getByRole('heading', { name: '晋级版本' }).locator('..')
  await form.getByLabel('来源').selectOption(source)
  await form.getByLabel('目标').selectOption(target)
  await form.getByLabel('不可变版本 ID').fill(versionID)
  const response = page.waitForResponse((item) => item.request().method() === 'POST' && item.url().endsWith('/releases:promote'))
  await form.getByRole('button', { name: '执行晋级' }).click()
  expect((await response).status()).toBe(201)
  await expect(environmentCard(page, target === 'staging' ? '预发' : '生产')).toContainText(versionID)
}

function environmentCard(page: Page, label: '开发' | '预发' | '生产') {
  return page.locator('.environment-card').filter({ hasText: label })
}
