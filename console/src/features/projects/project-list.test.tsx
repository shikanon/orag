import { screen } from '@testing-library/react'
import { HttpResponse, http } from 'msw'
import { describe, expect, it } from 'vitest'
import { server } from '../../test/handlers'
import { renderApp } from '../../test/render-app'

describe('ProjectList states', () => {
  it('shows an instructional empty state', async () => {
    server.use(http.get('/v1/projects', () => HttpResponse.json({ projects: [] })))
    renderApp('/projects')
    expect(await screen.findByRole('heading', { name: '创建第一个 RAG 项目' })).toBeVisible()
    expect(screen.getByText(/隔离编排、测试集、环境配置和发布历史/)).toBeVisible()
  })

  it('distinguishes an API error from an empty project list', async () => {
    server.use(http.get('/v1/projects', () => new HttpResponse(null, { status: 503 })))
    renderApp('/projects')
    expect(await screen.findByRole('alert')).toHaveTextContent('项目加载失败')
    expect(screen.getByRole('button', { name: '重新加载' })).toBeEnabled()
  })
})
