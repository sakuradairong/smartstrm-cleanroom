import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, useLocation } from 'react-router'
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
    <MemoryRouter initialEntries={['/tasks']}>
      <AntApp>
        <TasksPage />
        <LocationProbe />
      </AntApp>
    </MemoryRouter>,
  )
}

function LocationProbe() {
  const location = useLocation()
  return <output data-testid="location">{location.pathname}{location.search}</output>
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
  expect(screen.getByText('生成 1 / 复制 2 / 跳过 0 / 删除 3 / 失败 0')).toBeInTheDocument()
  expect(screen.getByText('剧集')).toBeInTheDocument()
  expect(screen.getByText('运行中')).toBeInTheDocument()
  expect(screen.getByText('队列 2')).toBeInTheDocument()
  expect(screen.getByText(/添加或调整任务后会立即动态生效/)).toBeInTheDocument()
  expect(screen.getByRole('button', { name: /添加任务/ })).toBeInTheDocument()
  expect(screen.getAllByText('0 */6 * * *')).toHaveLength(2)
  expect(screen.getAllByRole('button', { name: /运\s*行/ })[0]).toHaveClass('ant-btn-primary')
  expect(screen.getAllByRole('button', { name: /工\s*具/ })[0]).toHaveClass('ant-btn-text')
  expect(screen.getByText('显示 2 / 2 个任务')).toBeInTheDocument()
})

test('添加任务入口跳转到配置页并请求打开新增表单', async () => {
  stubFetch((url) => {
    if (url === '/api/tasks') return { body: { tasks: [], statuses: [] } }
    throw new Error('unexpected fetch: ' + url)
  })
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('button', { name: /添加任务/ }))
  expect(screen.getByTestId('location')).toHaveTextContent('/config?section=tasks&action=add-task')
})

test('可按名称、ID、存储或源路径筛选任务并清空筛选', async () => {
  const data: TasksResponse = {
    tasks: [
      makeTask(),
      makeTask({ id: 'anime', name: '动画', storage_id: 'webdav', source: '/shows/anime' }),
    ],
    statuses: [makeStatus(), makeStatus({ task_id: 'anime' })],
  }
  stubFetch((url) => {
    if (url === '/api/tasks') return { body: data }
    throw new Error('unexpected fetch: ' + url)
  })
  const user = userEvent.setup()
  renderPage()
  const filter = await screen.findByRole('searchbox', { name: '筛选任务' })
  await user.type(filter, 'WEBDAV')
  expect(screen.getByText('动画')).toBeInTheDocument()
  expect(screen.queryByText('电影')).not.toBeInTheDocument()
  expect(screen.getByText('显示 1 / 2 个任务')).toBeInTheDocument()
  await user.clear(filter)
  expect(screen.getByText('电影')).toBeInTheDocument()
  expect(screen.getByText('显示 2 / 2 个任务')).toBeInTheDocument()
})

test('手动刷新任务列表并更新计数', async () => {
  let loads = 0
  stubFetch((url) => {
    if (url !== '/api/tasks') throw new Error('unexpected fetch: ' + url)
    loads += 1
    return {
      body: loads === 1
        ? { tasks: [makeTask()], statuses: [makeStatus()] }
        : { tasks: [makeTask(), makeTask({ id: 't2', name: '剧集' })], statuses: [makeStatus()] },
    }
  })
  const user = userEvent.setup()
  renderPage()
  expect(await screen.findByText('显示 1 / 1 个任务')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: /刷\s*新/ }))
  expect(await screen.findByText('显示 2 / 2 个任务')).toBeInTheDocument()
  expect(screen.getByText('剧集')).toBeInTheDocument()
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
    if (url === '/api/tasks/t1/preview?path=%2F%E7%94%B5%E5%BD%B1%2F%E7%83%AD%E9%97%A8.mkv') {
      return { body: preview }
    }
    throw new Error('unexpected fetch: ' + url)
  })
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('button', { name: /预\s*览/ }))
  const dialog = await screen.findByRole('dialog')
  await user.type(within(dialog).getByLabelText('源文件路径'), '/电影/热门.mkv')
  await user.click(within(dialog).getByRole('button', { name: '生成预览' }))
  await waitFor(() => {
    const pre = document.querySelector('.ant-modal pre')
    expect(pre?.textContent).toBe(JSON.stringify(preview, null, 2))
  })
})

test('预览失败显示错误并解除按钮 loading', async () => {
  const data: TasksResponse = { tasks: [makeTask()], statuses: [makeStatus()] }
  stubFetch((url) => {
    if (url === '/api/tasks') return { body: data }
    if (url === '/api/tasks/t1/preview?path=%2Fmovies%2Fa.mkv') {
      return { status: 400, body: { error: 'preview failed' } }
    }
    throw new Error('unexpected fetch: ' + url)
  })
  const user = userEvent.setup()
  renderPage()
  const previewButton = await screen.findByRole('button', { name: /预\s*览/ })
  await user.click(previewButton)
  const dialog = await screen.findByRole('dialog')
  const pathInput = within(dialog).getByLabelText('源文件路径')
  await user.type(pathInput, '/movies/a.mkv')
  const submit = within(dialog).getByRole('button', { name: '生成预览' })
  await user.click(submit)
  expect(await screen.findByText('preview failed')).toBeInTheDocument()
  await waitFor(() => expect(submit).not.toHaveClass('ant-btn-loading'))
  expect(pathInput).toHaveValue('/movies/a.mkv')
})

test('接口错误显示后端 error 文案', async () => {
  stubFetch((url) => {
    if (url === '/api/tasks') return { status: 500, body: { error: 'boom' } }
    throw new Error('unexpected fetch: ' + url)
  })
  renderPage()
  expect(await screen.findByText('boom')).toBeInTheDocument()
})
