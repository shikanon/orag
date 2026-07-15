import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

const apiTarget = (globalThis as { process?: { env?: Record<string, string | undefined> } }).process?.env?.ORAG_CONSOLE_API_TARGET ?? 'http://localhost:8080'

export default defineConfig({
  plugins: [react()],
  server: { proxy: { '/v1': apiTarget } },
})
