import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { App as AntApp } from 'antd'
import TasksPage from './TasksPage'
import type { TaskConfig, TaskStatus, TasksResponse } from '../api/types'

function makeTask(overrides: Partial<TaskConfig> = {}): TaskConfig {
  return {
    id: 't1',
    name: '电影',
    storage_id: 'local',
    source: '/movies',
    destination: '/strm/movies',
    schedule: '0 */6 * * *',
    file_id_mode: false,
    dir_time_check: false,
    watch: false,
    incremental: true,
    sync_delete: false,
    media_ext: ['.mkv'],
    copy_ext: ['.nfo'],
    min_size: 0,
    max_size: 0,
    plugins: [],
    keep_local: [],
    ...overrides,
  }
}

function makeStatus(overrides: Partial<TaskStatus> = {}): TaskStatus {
  return { task_id: 't1', running: false, queued: 0, result: {}, ...overrides }
}

type FetchResult = { status?: number; body?: unknown }
type FetchHandler = (url: string, init?: RequestInit) => FetchResult | Promise<FetchResult>

function stubFetch(handler: FetchHandler) {
  const mock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = typeof input === 'string' ? input : input.toString()
    const result = await handler(url, init)
    const status = result.status ?? 200
    return {
      ok: status >= 200 && status < 300,
      status,
      text: async () => (result.body === undefined ? '' : JSON.stringify(result.body)),
    } as Response
  })
  vi.stubGlobal('fetch', mock)
  return mock
}

function renderPage() {
  return render(
    <MemoryRouter>
      <AntApp>
        <TasksPage />
      </AntApp>
    </MemoryRouter>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

test('渲染任务行与状态徽标', async () => {
  const data: TasksResponse = {
    tasks: [
      makeTask(),
      makeTask({ id: 't2', name: '剧集', storage_id: 'webdav', source: '/tv' }),
    ],
    statuses: [
      makeStatus({ result: { created: 1, copied: 2, removed: 3, failed: 0 } }),
      makeStatus({ task_id: 't2', running: true, queued: 2 }),
    ],
  }
  stubFetch((url) => {
    if (url === '/api/tasks') return { body: data }
    throw new Error('unexpected fetch: ' + url)
  })
  renderPage()
  expect(await screen.findByText('电影')).toBeInTheDocument()
  expect(screen.getByText('t1')).toBeInTheDocument()
  expect(screen.getByText('local')).toBeInTheDocument()
  expect(screen.getByText('/movies')).toBeInTheDocument()
  expect(screen.getByText('空闲')).toBeInTheDocument()
  expect(screen.getByText('生成 1 / 复制 2 / 删除 3 / 失败 0')).toBeInTheDocument()
  expect(screen.getByText('剧集')).toBeInTheDocument()
  expect(screen.getByText('运行中')).toBeInTheDocument()
  expect(screen.getByText('队列 2')).toBeInTheDocument()
  expect(screen.getByText(/任务可在“配置”页添加或调整/)).toBeInTheDocument()
})

test('点击运行调用 run 接口并显示成功消息', async () => {
  const data: TasksResponse = { tasks: [makeTask()], statuses: [makeStatus()] }
  let releaseRun: () => void = () => {}
  const fetchMock = stubFetch((url, init) => {
    if (url === '/api/tasks') return { body: data }
    if (url === '/api/tasks/t1/run' && init?.method === 'POST') {
      return new Promise<FetchResult>((resolve) => { releaseRun = () => resolve({ body: {} }) })
    }
    throw new Error('unexpected fetch: ' + url)
  })
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('button', { name: /运\s*行/ }))
  const dialog = await screen.findByRole('dialog')
  const ok = within(dialog).getByRole('button', { name: /确\s*定/ })
  await user.click(ok)
  // POST 未完成期间按钮处于 loading 状态（点击被忽略，防重）
  await waitFor(() => expect(ok).toHaveClass('ant-btn-loading'))
  expect(fetchMock).toHaveBeenCalledWith(
    '/api/tasks/t1/run',
    expect.objectContaining({ method: 'POST', body: '{}' }),
  )
  releaseRun()
  expect(await screen.findByText('已加入队列')).toBeInTheDocument()
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
})

test('运行中任务显示停止按钮', async () => {
  const data: TasksResponse = {
    tasks: [makeTask()],
    statuses: [makeStatus({ running: true })],
  }
  const fetchMock = stubFetch((url, init) => {
    if (url === '/api/tasks') return { body: data }
    if (url === '/api/tasks/t1/stop' && init?.method === 'POST') return { body: {} }
    throw new Error('unexpected fetch: ' + url)
  })
  const user = userEvent.setup()
  renderPage()
  const stop = await screen.findByRole('button', { name: /停\s*止/ })
  expect(screen.queryByRole('button', { name: /运\s*行/ })).not.toBeInTheDocument()
  await user.click(stop)
  await waitFor(() => {
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/tasks/t1/stop',
      expect.objectContaining({ method: 'POST' }),
    )
  })
})

test('预览打开对话框显示返回内容', async () => {
  const data: TasksResponse = { tasks: [makeTask()], statuses: [makeStatus()] }
  const preview = { target: '/strm/movies', total: 1, files: [{ path: '/movies/a.mkv', strm: '/strm/movies/a.strm' }] }
  stubFetch((url) => {
    if (url === '/api/tasks') return { body: data }
    if (url === '/api/tasks/t1/preview') return { body: preview }
    throw new Error('unexpected fetch: ' + url)
  })
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('button', { name: /预\s*览/ }))
  await screen.findByRole('dialog')
  await waitFor(() => {
    const pre = document.querySelector('.ant-modal pre')
    expect(pre?.textContent).toBe(JSON.stringify(preview, null, 2))
  })
})

test('接口错误显示后端 error 文案', async () => {
  stubFetch((url) => {
    if (url === '/api/tasks') return { status: 500, body: { error: 'boom' } }
    throw new Error('unexpected fetch: ' + url)
  })
  renderPage()
  expect(await screen.findByText('boom')).toBeInTheDocument()
})
