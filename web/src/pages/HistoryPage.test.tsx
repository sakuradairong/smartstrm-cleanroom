import { render, screen, waitFor, act, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import HistoryPage from './HistoryPage'
import type { HistoryEvent } from '../api/types'
import { MemoryRouter } from 'react-router'

// jsdom 无 EventSource——注入可控 fake
class FakeEventSource {
  static instances: FakeEventSource[] = []
  listeners = new Map<string, (e: MessageEvent) => void>()
  onopen: (() => void) | null = null
  onerror: (() => void) | null = null
  closed = false
  constructor(public url: string) { FakeEventSource.instances.push(this) }
  addEventListener(type: string, fn: (e: MessageEvent) => void) { this.listeners.set(type, fn) }
  close() { this.closed = true }
  emit(item: unknown) { this.listeners.get('history')?.(new MessageEvent('history', { data: JSON.stringify(item) })) }
}

function makeEvent(id: number, overrides: Partial<HistoryEvent> = {}): HistoryEvent {
  return { id, timestamp: '2026-07-20T10:00:00Z', task_id: 't1', type: 'completed', ...overrides }
}

type FetchResult = { status?: number; body?: unknown }

function stubFetch(handler: (url: string) => FetchResult) {
  const mock = vi.fn(async (input: RequestInfo | URL) => {
    const url = typeof input === 'string' ? input : input.toString()
    const result = await handler(url)
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

// rAF stub 为 setTimeout(0)，配合真实定时器手动 flush，保证确定性
const flushFrame = () => act(async () => { await new Promise(r => setTimeout(r, 0)) })

function renderPage(initialEntry = '/history') {
  return render(<MemoryRouter initialEntries={[initialEntry]}><HistoryPage /></MemoryRouter>)
}

beforeEach(() => {
  FakeEventSource.instances = []
  vi.stubGlobal('EventSource', FakeEventSource)
  vi.stubGlobal('requestAnimationFrame', (cb: FrameRequestCallback) => setTimeout(() => cb(0), 0))
  vi.stubGlobal('cancelAnimationFrame', (id: number) => clearTimeout(id))
})

afterEach(() => {
  vi.unstubAllGlobals()
})

test('渲染初始历史行，时间与事件使用中文界面格式', async () => {
  stubFetch((url) => {
    if (url === '/api/history?limit=200') {
      return {
        body: [
          makeEvent(2, { task_id: 't2', type: 'failed', error: '磁盘已满', path: '/movies/b.mkv' }),
          makeEvent(1, { path: '/movies/a.mkv', created: 1, copied: 2, removed: 3, skipped: 4 }),
        ],
      }
    }
    throw new Error('unexpected fetch: ' + url)
  })
  renderPage()
  expect(await screen.findByText('/movies/a.mkv')).toBeInTheDocument()
  expect(screen.getByText('/movies/b.mkv')).toBeInTheDocument()
  expect(screen.getByText('t2')).toBeInTheDocument()
  expect(screen.getByText('生成 1 / 复制 2 / 删除 3 / 跳过 4 / 失败 0')).toBeInTheDocument()
  // 有 error 时显示 error + 失败计数
  expect(screen.getByText('磁盘已满 / 失败 0')).toBeInTheDocument()
  expect(screen.getAllByText('2026-07-20 10:00:00')).toHaveLength(2)
  expect(screen.getByText('已完成')).toBeInTheDocument()
  expect(screen.getByText('失败')).toBeInTheDocument()
  expect(screen.queryByText('completed')).not.toBeInTheDocument()
  expect(screen.queryByText('failed')).not.toBeInTheDocument()
})

test('SSE 注入新事件后新行出现在最前', async () => {
  stubFetch((url) => {
    if (url === '/api/history?limit=200') return { body: [makeEvent(1, { path: '/movies/a.mkv' })] }
    throw new Error('unexpected fetch: ' + url)
  })
  renderPage()
  expect(await screen.findByText('/movies/a.mkv')).toBeInTheDocument()
  const es = FakeEventSource.instances[0]
  act(() => { es.onopen?.() })
  expect(screen.getByText('实时连接已建立')).toBeInTheDocument()
  act(() => { es.emit(makeEvent(2, { path: '/movies/new.mkv', task_id: 't3' })) })
  await flushFrame()
  expect(await screen.findByText('/movies/new.mkv')).toBeInTheDocument()
  const firstRow = document.querySelector('.ant-table-tbody tr.ant-table-row')
  expect(firstRow).not.toBeNull()
  expect(within(firstRow as HTMLElement).getByText('/movies/new.mkv')).toBeInTheDocument()
})

test('断线显示重试中文案', async () => {
  stubFetch((url) => {
    if (url === '/api/history?limit=200') return { body: [] }
    throw new Error('unexpected fetch: ' + url)
  })
  renderPage()
  // 连接建立前即显示重试中
  expect(await screen.findByText('实时连接重试中…')).toBeInTheDocument()
  const es = FakeEventSource.instances[0]
  act(() => { es.onopen?.() })
  expect(screen.getByText('实时连接已建立')).toBeInTheDocument()
  act(() => { es.onerror?.() })
  expect(screen.getByText('实时连接重试中…')).toBeInTheDocument()
})

test('筛选按任务 ID 重新加载并重订阅', async () => {
  const fetchMock = stubFetch((url) => {
    if (url === '/api/history?limit=200') return { body: [makeEvent(1, { path: '/movies/a.mkv' })] }
    if (url === '/api/history?limit=200&task_id=t9') return { body: [makeEvent(9, { task_id: 't9', path: '/tv/b.mkv' })] }
    throw new Error('unexpected fetch: ' + url)
  })
  const user = userEvent.setup()
  renderPage()
  expect(await screen.findByText('/movies/a.mkv')).toBeInTheDocument()
  await user.type(screen.getByPlaceholderText('任务 ID（留空显示全部）'), 't9')
  await user.click(screen.getByRole('button', { name: /筛\s*选/ }))
  await waitFor(() => {
    expect(fetchMock.mock.calls.some(c => c[0] === '/api/history?limit=200&task_id=t9')).toBe(true)
  })
  expect(await screen.findByText('/tv/b.mkv')).toBeInTheDocument()
  // 旧连接关闭，新连接携带 task_id
  expect(FakeEventSource.instances).toHaveLength(2)
  expect(FakeEventSource.instances[0].closed).toBe(true)
  expect(FakeEventSource.instances[1].url).toBe('/api/history/stream?task_id=t9')
})

test('初始加载失败时显示错误并可重新加载', async () => {
  let attempts = 0
  stubFetch((url) => {
    if (url !== '/api/history?limit=200') throw new Error('unexpected fetch: ' + url)
    attempts += 1
    return attempts === 1 ? { status: 500, body: { error: '日志服务暂时不可用' } } : { body: [] }
  })
  const user = userEvent.setup()
  renderPage()
  expect(await screen.findByText('运行日志加载失败')).toBeInTheDocument()
  expect(screen.getByText('日志服务暂时不可用')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: '重新加载' }))
  await waitFor(() => expect(FakeEventSource.instances).toHaveLength(1))
  expect(screen.queryByText('运行日志加载失败')).not.toBeInTheDocument()
})
