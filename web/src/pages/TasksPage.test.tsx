import { act, render, screen, waitFor, within } from '@testing-library/react'
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
  expect(screen.getAllByText('空闲')).toHaveLength(2)
  expect(screen.getAllByText('生成 1')).toHaveLength(2)
  expect(screen.getAllByText('复制 2')).toHaveLength(2)
  expect(screen.getAllByText('跳过 0').length).toBeGreaterThan(0)
  expect(screen.getAllByText('删除 3')).toHaveLength(2)
  expect(screen.getAllByText('失败 0').length).toBeGreaterThan(0)
  expect(screen.getByText('剧集')).toBeInTheDocument()
  expect(screen.getAllByText('运行中')).toHaveLength(2)
  expect(screen.getAllByText('队列 2')).toHaveLength(2)
  expect(screen.getByText(/添加或调整任务后会立即动态生效/)).toBeInTheDocument()
  expect(screen.getByRole('button', { name: /添加任务/ })).toBeInTheDocument()
  expect(screen.getAllByText('0 */6 * * *')).toHaveLength(2)
  expect(screen.getAllByRole('button', { name: /^运\s*行$/ })[0]).toHaveClass('ant-btn-default')
  expect(screen.getAllByRole('button', { name: /工\s*具/ })[0]).toHaveClass('ant-btn-text')
  expect(screen.getByText('显示 2 / 2 个任务')).toBeInTheDocument()
})

test('没有任务时禁用运行全部并突出添加任务入口', async () => {
  const fetchMock = stubFetch((url) => {
    if (url === '/api/tasks') return { body: { tasks: [], statuses: [] } }
    throw new Error('unexpected fetch: ' + url)
  })
  const user = userEvent.setup()
  renderPage()
  expect(await screen.findByText('还没有任务。任务用于扫描媒体并生成 STRM 文件。')).toBeInTheDocument()
  const runAll = screen.getByRole('button', { name: /运行全部/ })
  expect(runAll).toBeDisabled()
  expect(runAll).toHaveClass('ant-btn-default')
  const addTask = screen.getByRole('button', { name: /添加任务/ })
  expect(addTask).toHaveClass('ant-btn-primary')
  expect(document.querySelectorAll('.tasks-page .ant-btn-primary')).toHaveLength(1)
  expect(screen.queryByRole('button', { name: '创建第一个任务' })).not.toBeInTheDocument()
  await user.click(runAll)
  expect(fetchMock.mock.calls.filter(([, init]) => init?.method === 'POST')).toHaveLength(0)
  await user.click(addTask)
  expect(screen.getByTestId('location')).toHaveTextContent('/config?section=tasks&action=add-task')
})

test('运行全部原子提交任务并在请求期间防重，成功后刷新状态', async () => {
  const tasks = [makeTask(), makeTask({ id: 't2', name: '剧集' })]
  let queued = false
  let releaseRun: () => void = () => {}
  const fetchMock = stubFetch((url, init) => {
    if (url === '/api/tasks') {
      return { body: { tasks, statuses: [makeStatus({ queued: queued ? 1 : 0 })] } }
    }
    if (url === '/api/tasks/run-all' && init?.method === 'POST') {
      return new Promise<FetchResult>((resolve) => {
        releaseRun = () => {
          queued = true
          resolve({ body: { status: 'queued', tasks: ['t1', 't2'] } })
        }
      })
    }
    throw new Error('unexpected fetch: ' + url)
  })
  const user = userEvent.setup()
  renderPage()
  expect(await screen.findByText('电影')).toBeInTheDocument()
  const runAll = screen.getByRole('button', { name: /运行全部/ })
  expect(runAll).toHaveClass('ant-btn-primary')
  expect(screen.getByRole('button', { name: /添加任务/ })).toHaveClass('ant-btn-default')
  await user.click(runAll)
  await waitFor(() => expect(runAll).toHaveClass('ant-btn-loading'))
  expect(runAll).toHaveTextContent('正在加入队列')
  await user.click(runAll)
  const posts = fetchMock.mock.calls.filter(([, init]) => init?.method === 'POST')
  expect(posts).toHaveLength(1)
  expect(posts[0]).toEqual([
    '/api/tasks/run-all',
    expect.objectContaining({
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: '{}',
    }),
  ])
  releaseRun()
  expect(await screen.findByText('已将 2 个任务加入队列')).toBeInTheDocument()
  expect(await screen.findAllByText('队列 1')).toHaveLength(2)
  await waitFor(() => expect(runAll).not.toHaveClass('ant-btn-loading'))
})

test('运行全部失败在操作旁保留错误，成功重试后清除', async () => {
  let fail = true
  stubFetch((url) => {
    if (url === '/api/tasks') return { body: { tasks: [makeTask()], statuses: [] } }
    if (url === '/api/tasks/run-all') return fail
      ? { status: 409, body: { error: '任务队列不可用' } }
      : { body: { status: 'queued', tasks: ['t1'] } }
    throw new Error('unexpected fetch: ' + url)
  })
  const user = userEvent.setup()
  renderPage()
  expect(await screen.findByText('电影')).toBeInTheDocument()
  const runAll = screen.getByRole('button', { name: /运行全部/ })
  await user.click(runAll)
  const error = await screen.findByRole('alert')
  expect(error).toHaveTextContent('任务队列不可用')
  expect(error.closest('.task-page-actions')).toContainElement(runAll)
  expect(screen.getAllByText('任务队列不可用')).toHaveLength(1)
  expect(screen.queryByText(/已将 .* 个任务加入队列/)).not.toBeInTheDocument()
  await waitFor(() => expect(runAll).not.toHaveClass('ant-btn-loading'))
  expect(runAll).toBeEnabled()
  fail = false
  await user.click(runAll)
  expect(await screen.findByText('已将 1 个任务加入队列')).toBeInTheDocument()
  expect(screen.queryByText('任务队列不可用')).not.toBeInTheDocument()
})

test('初次加载时显示加载状态而不是空配置', async () => {
  let resolveLoad: (value: FetchResult) => void = () => {}
  stubFetch(() => new Promise<FetchResult>((resolve) => { resolveLoad = resolve }))
  renderPage()
  expect(within(document.querySelector('.tasks-page') ?? document.body).getByRole('status')).toHaveTextContent('正在加载任务')
  expect(screen.queryByText(/还没有任务/)).not.toBeInTheDocument()
  expect(screen.queryByText('显示 0 / 0 个任务')).not.toBeInTheDocument()
  expect(document.querySelectorAll('.tasks-page .ant-btn-primary')).toHaveLength(0)
  await act(async () => { resolveLoad({ body: { tasks: [], statuses: [] } }) })
  expect(await screen.findByText(/还没有任务/)).toBeInTheDocument()
  expect(screen.queryByText('正在加载任务')).not.toBeInTheDocument()
})

test('初次加载失败保留错误和重试入口，重试成功后显示任务', async () => {
  let fail = true
  stubFetch(() => fail
    ? { status: 500, body: { error: '读取任务失败' } }
    : { body: { tasks: [makeTask()], statuses: [] } })
  const user = userEvent.setup()
  renderPage()
  expect(await screen.findByRole('alert')).toHaveTextContent('读取任务失败')
  expect(screen.queryByText(/还没有任务/)).not.toBeInTheDocument()
  expect(screen.queryByText('显示 0 / 0 个任务')).not.toBeInTheDocument()
  expect(document.querySelectorAll('.tasks-page .ant-btn-primary')).toHaveLength(0)
  fail = false
  await user.click(screen.getByRole('button', { name: /重\s*试/ }))
  expect(await screen.findByText('电影')).toBeInTheDocument()
  expect(screen.queryByRole('alert')).not.toBeInTheDocument()
})

test('移动任务摘要包含状态、全部结果、错误及日志入口', async () => {
  stubFetch(() => ({ body: {
    tasks: [makeTask()],
    statuses: [makeStatus({ error: 'list /movies: open /missing: no such file or directory', queued: 2,
      result: { created: 1, copied: 2, skipped: 3, removed: 4, failed: 5 } })],
  } }))
  const user = userEvent.setup()
  renderPage()
  const summary = await screen.findByRole('group', { name: '电影运行摘要' })
  expect(summary).toHaveClass('task-mobile-summary')
  for (const text of ['失败', '队列 2', '生成 1', '复制 2', '跳过 3', '删除 4', '失败 5']) {
    expect(within(summary).getByText(text)).toBeInTheDocument()
  }
  expect(summary.querySelector('[data-error-kind="source-unavailable"]')).toBeInTheDocument()
  expect(within(summary).queryByText(/no such file or directory/i)).not.toBeInTheDocument()
  await user.click(within(summary).getByRole('button', { name: '查看日志' }))
  expect(screen.getByTestId('location')).toHaveTextContent('/history?task_id=t1')
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
  expect(screen.getByLabelText('筛选任务')).toBe(filter)
  expect(filter).toHaveAttribute('placeholder', '按名称、ID、存储或源路径筛选')
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
  await user.click(await screen.findByRole('button', { name: /^运\s*行$/ }))
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
  expect(screen.queryByRole('button', { name: /^运\s*行$/ })).not.toBeInTheDocument()
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
