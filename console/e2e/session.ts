import type { Page } from '@playwright/test'

export async function authenticateConsole(page: Page) {
  await page.addInitScript(() => {
    sessionStorage.setItem('orag.console.session.v1', JSON.stringify({ version: 1, accessToken: 'e2e-access-token', expiresAt: Date.now() + 3_600_000 }))
  })
}
