import { screen } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { server } from '../../test/handlers'
import { renderApp } from '../../test/render-app'

describe('RAGStudio', () => {
  it('renders the server-owned node registry', async () => {
    server.use(http.get('/v1/pipeline-node-definitions', () => HttpResponse.json({ items: [{ type: 'init', display_name: 'Query Input', category: 'input', schema_version: 1, config_schema: {}, entry: true, allowed_targets: ['ark_generate'] }, { type: 'ark_generate', display_name: 'Generate', category: 'generation', schema_version: 1, config_schema: {}, produces_answer: true }] })))
    renderApp('/projects/prj_a/studio')
    expect(await screen.findByText('Query Input')).toBeVisible()
    expect(screen.getByText('Generate')).toBeVisible()
    expect(screen.getByText(/允许连接/)).toBeVisible()
  })
})
