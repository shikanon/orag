import { defineConfig } from 'vitest/config'

export default defineConfig({
  test: {
    pool: 'threads',
    environment: 'happy-dom',
    setupFiles: ['./src/test/setup.ts'],
    css: true,
    exclude: ['e2e/**', 'node_modules/**'],
  },
})

