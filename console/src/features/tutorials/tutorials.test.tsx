import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { HttpResponse, http } from 'msw'
import { describe, expect, it } from 'vitest'
import { server, tutorials, useTutorialLiveRunHandlers } from '../../test/handlers'
import { renderApp } from '../../test/render-app'

describe('Tutorial catalog', () => {
  it('renders three end-to-end tutorials', async () => {
    renderApp('/tutorials')
    expect(await screen.findByRole('heading', { name: '教程与实验室' })).toBeVisible()
    expect(await screen.findByText('中文文本 RAG')).toBeVisible()
    expect(screen.getByText('视觉文档 RAG')).toBeVisible()
    expect(screen.getByText('视频 RAG')).toBeVisible()
  })

  it('shows scenario dimensions and pack requirements', async () => {
    renderApp('/tutorials/video-rag')
    expect(await screen.findByRole('heading', { name: '视频 RAG' })).toBeVisible()
    expect(screen.getByText('Replay 结果即将开放')).toBeVisible()
    expect(screen.getByRole('heading', { name: 'Quick Pack' })).toBeVisible()
    expect(screen.getByRole('heading', { name: 'Benchmark Pack' })).toBeVisible()
    expect(screen.getByText('时间否定')).toBeVisible()
    expect(screen.getByRole('button', { name: '克隆教程' })).toBeEnabled()
  })

  it('starts the chosen Quick Pack clone and shows server progress', async () => {
    useTutorialLiveRunHandlers()
    const user = userEvent.setup()
    renderApp('/tutorials/text-rag')
    await user.click(await screen.findByRole('button', { name: '克隆教程' }))
    await user.click(screen.getByRole('radio', { name: /Quick Pack/ }))
    await user.click(screen.getByRole('checkbox', { name: '我已确认数据许可' }))
    await user.click(screen.getByRole('button', { name: '创建实验项目' }))
    expect(await screen.findByText('Pack 已安装。支持运行声明的文本 Quick Pack 可进入基线 Live Run。')).toBeVisible()
    expect(await screen.findByRole('link', { name: '打开基线 Live Run' })).toBeVisible()
    expect(screen.queryByText(/manifest_url|access key/i)).not.toBeInTheDocument()
  })

  it('retries after the catalog API fails', async () => {
    let attempts = 0
    server.use(http.get('/v1/tutorials', () => {
      attempts += 1
      return attempts === 1 ? new HttpResponse(null, { status: 503 }) : HttpResponse.json({ tutorials })
    }))
    renderApp('/tutorials')
    const retry = await screen.findByRole('button', { name: '重新加载' })
    await userEvent.click(retry)
    expect(await screen.findByText('中文文本 RAG')).toBeVisible()
    expect(attempts).toBe(2)
  })

  it('links back to the library when a tutorial is unknown', async () => {
    server.use(http.get('/v1/tutorials/missing', () => new HttpResponse(null, { status: 404 })))
    renderApp('/tutorials/missing')
    expect(await screen.findByRole('alert')).toHaveTextContent('教程不存在')
    expect(screen.getByRole('link', { name: '返回教程库' })).toHaveAttribute('href', '/tutorials')
  })

  it('runs the server-derived P0 then renders auditable P1 and P2 comparisons without Pack locations', async () => {
    useTutorialLiveRunHandlers()
    const user = userEvent.setup()
    renderApp('/projects/prj_clone/tutorial/experiments/texp_clone')
    await user.click(await screen.findByRole('button', { name: '运行 P0 基线' }))
    expect(await screen.findByText('eval_tutorial_clone')).toBeVisible()
    await user.click(screen.getByRole('button', { name: '运行 P1 解析候选' }))
    expect(await screen.findByText('eval_tutorial_clone_p1')).toBeVisible()
    expect(await screen.findByRole('heading', { name: 'P0 与候选使用相同的对比输入' })).toBeVisible()
    expect(screen.getByText('accuracy')).toBeVisible()
    expect(screen.getByText('+50.0%')).toBeVisible()
    await user.click(screen.getByRole('button', { name: '运行 P2 分块候选' }))
    expect(await screen.findByText('eval_tutorial_clone_p2')).toBeVisible()
    expect(await screen.findByText('chunk_count')).toBeVisible()
    expect(screen.getAllByText('400/80').length).toBeGreaterThanOrEqual(2)
    expect(screen.queryByText(/oss|access key|bucket/i)).not.toBeInTheDocument()
  })
})
