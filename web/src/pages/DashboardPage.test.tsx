import { render, screen, within } from '@testing-library/react'
import { App as AntApp } from 'antd'
import { MemoryRouter } from 'react-router'
import DashboardPage from './DashboardPage'

function response(body: unknown, status = 200): Response {
  return { ok: status < 400, status, text: async () => JSON.stringify(body) } as Response
}

function renderPage(body: unknown, status = 200) {
  vi.stubGlobal('fetch', vi.fn(async () => response(body, status)))
  return render(<MemoryRouter><AntApp><DashboardPage /></AntApp></MemoryRouter>)
}

afterEach(() => vi.unstubAllGlobals())

test('渲染状态卡片与最近运行记录', async () => {
  renderPage({
    tasks: { total: 2, running: 1, failed: 0, queued: 1 }, storages: 2,
    history: [{ id: 7, timestamp: '2026-07-20T10:00:00Z', task_id: 'movies', type: 'completed', created: 3, copied: 1, removed: 0, skipped: 2, failed: 0 }],
  })
  expect(await screen.findByText('任务总数')).toBeInTheDocument()
  for (const title of ['运行中', '失败', '队列', '存储总数', '最近运行']) expect(screen.getByText(title)).toBeInTheDocument()
  const row = screen.getByText('movies').closest('tr')!
  expect(within(row).getByText('completed')).toBeInTheDocument()
  expect(within(row).getByText(/新增 3.*复制 1.*删除 0.*跳过 2.*失败 0/)).toBeInTheDocument()
})

test('无历史时显示空态', async () => {
  renderPage({ tasks: { total: 0, running: 0, failed: 0, queued: 0 }, storages: 0, history: [] })
  expect(await screen.findByText('暂无运行记录')).toBeInTheDocument()
})

test('加载失败显示错误', async () => {
  renderPage({ error: 'overview unavailable' }, 503)
  expect((await screen.findAllByText('overview unavailable')).length).toBeGreaterThan(0)
})

test('快捷入口指向任务、存储和配置页', async () => {
  renderPage({ tasks: { total: 0, running: 0, failed: 0, queued: 0 }, storages: 0, history: [] })
  expect(await screen.findByRole('link', { name: '运行任务' })).toHaveAttribute('href', '/tasks')
  expect(screen.getByRole('link', { name: '浏览存储' })).toHaveAttribute('href', '/storage')
  expect(screen.getByRole('link', { name: '修改配置' })).toHaveAttribute('href', '/config')
})
