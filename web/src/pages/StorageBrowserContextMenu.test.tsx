import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { App as AntApp } from 'antd'
import StorageBrowserPage from './StorageBrowserPage'
import type { MatchingTask, StorageEntry, StorageSummary } from '../api/types'

const STORAGES: StorageSummary[] = [{ id: 'local', type: 'local', root: '/' }]

const ROOT_ENTRIES: StorageEntry[] = [
  { path: '/movies', name: 'movies', size: 0, is_dir: true, modified: '2025-01-02 03:04:05' },
  { path: '/b.mkv', name: 'b.mkv', size: 2048, is_dir: false, modified: '2025-01-03 00:00:00' },
  { path: '/note.txt', name: 'note.txt', size: 10, is_dir: false, modified: '2025-01-01 00:00:00' },
]

const ENTRIES_URL = '/api/storages/local/entries?path=' + encodeURIComponent('/')
const MATCHING_URL = '/api/storages/local/matching-tasks?path=' + encodeURIComponent('/')
const STREAM_URL = '/api/storages/local/stream-url?path=' + encodeURIComponent('/b.mkv')

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

function stubBase(handler: FetchHandler) {
  return stubFetch((url, init) => {
    if (url === '/api/storages') return { body: STORAGES }
    if (url === ENTRIES_URL) return { body: ROOT_ENTRIES }
    return handler(url, init)
  })
}

// matching-tasks 固定返回 matches，其余请求交给 extra（缺省抛错）。
function handlerWith(matches: MatchingTask[], extra?: FetchHandler): FetchHandler {
  return (url, init) => {
    if (url === MATCHING_URL) return { body: matches }
    if (extra) return extra(url, init)
    throw new Error('unexpected fetch: ' + url)
  }
}

function renderPage() {
  return render(
    <MemoryRouter>
      <AntApp>
        <StorageBrowserPage />
      </AntApp>
    </MemoryRouter>,
  )
}

async function rowOf(name: string): Promise<HTMLElement> {
  const cell = await screen.findByText(name)
  const row = cell.closest('tr')
  if (!row) throw new Error('row not found for ' + name)
  return row
}

afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

test('媒体文件行右键打开菜单，选择复制调用 stream-url 流程', async () => {
  const streamURL = 'http://example.com/stream/local?path=%2Fb.mkv&sig=abc'
  stubBase(
    handlerWith([], (url) => {
      if (url === STREAM_URL) return { body: { url: streamURL } }
      throw new Error('unexpected fetch: ' + url)
    }),
  )
  const user = userEvent.setup()
  // userEvent.setup() 会替换 navigator.clipboard，须在其后覆盖为我们的 spy。
  const writeText = vi.fn<(text: string) => Promise<void>>().mockResolvedValue(undefined)
  Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
  renderPage()
  const row = await rowOf('b.mkv')
  expect(row).toHaveAttribute('aria-haspopup', 'menu')
  expect(row).toHaveAttribute('tabindex', '0')
  fireEvent.contextMenu(row, { clientX: 50, clientY: 60 })
  const menu = await screen.findByRole('menu')
  await user.click(within(menu).getByRole('menuitem', { name: /复制\s*STRM\s*地址/ }))
  await waitFor(() => expect(writeText).toHaveBeenCalledWith(streamURL))
  expect(screen.queryByRole('menu')).not.toBeInTheDocument()
  expect(await screen.findByText('STRM 地址已复制')).toBeInTheDocument()
})

test('非媒体文件行右键不出现自定义菜单', async () => {
  stubBase(handlerWith([]))
  renderPage()
  const dirRow = await rowOf('movies')
  const fileRow = await rowOf('note.txt')
  expect(dirRow).not.toHaveAttribute('aria-haspopup')
  expect(fileRow).not.toHaveAttribute('aria-haspopup')
  fireEvent.contextMenu(dirRow, { clientX: 10, clientY: 10 })
  fireEvent.contextMenu(fileRow, { clientX: 10, clientY: 10 })
  expect(screen.queryByRole('menu')).not.toBeInTheDocument()
})

test('键盘 Shift+F10 在聚焦媒体行时打开菜单', async () => {
  stubBase(handlerWith([]))
  renderPage()
  const row = await rowOf('b.mkv')
  row.focus()
  fireEvent.keyDown(row, { key: 'F10', shiftKey: true })
  expect(await screen.findByRole('menu')).toBeInTheDocument()
})

test('Esc 关闭菜单', async () => {
  stubBase(handlerWith([]))
  renderPage()
  const row = await rowOf('b.mkv')
  fireEvent.contextMenu(row, { clientX: 50, clientY: 60 })
  await screen.findByRole('menu')
  fireEvent.keyDown(document, { key: 'Escape' })
  await waitFor(() => expect(screen.queryByRole('menu')).not.toBeInTheDocument())
})

test('菜单外部点击关闭菜单', async () => {
  stubBase(handlerWith([]))
  renderPage()
  const row = await rowOf('b.mkv')
  fireEvent.contextMenu(row, { clientX: 50, clientY: 60 })
  await screen.findByRole('menu')
  fireEvent.mouseDown(document.body)
  await waitFor(() => expect(screen.queryByRole('menu')).not.toBeInTheDocument())
})

test('浏览后匹配到任务时显示“运行当前”，点击调用 run', async () => {
  const fetchMock = stubBase(
    handlerWith([{ id: 't1', name: '电影' }], (url, init) => {
      if (url === '/api/tasks/t1/run' && init?.method === 'POST') {
        return { status: 202, body: { status: 'queued' } }
      }
      throw new Error('unexpected fetch: ' + url)
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await screen.findByText('b.mkv')
  await user.click(await screen.findByRole('button', { name: '运行当前' }))
  await waitFor(() => {
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/tasks/t1/run',
      expect.objectContaining({ method: 'POST', body: JSON.stringify({ path: '/' }) }),
    )
  })
  expect(await screen.findByText('已将 电影 的当前目录加入队列')).toBeInTheDocument()
})

test('无匹配任务时不显示“运行当前”', async () => {
  const fetchMock = stubBase(handlerWith([]))
  renderPage()
  await screen.findByText('b.mkv')
  await waitFor(() => {
    expect(fetchMock.mock.calls.map((call) => String(call[0]))).toContain(MATCHING_URL)
  })
  expect(screen.queryByRole('button', { name: '运行当前' })).not.toBeInTheDocument()
})

// 旧 menuJS 的 runCurrentDirectory 始终取 currentMatchingTasks[0]，无多选 UI，按旧语义移植。
test('多个匹配任务时点击运行第一个匹配任务（旧 runCurrentDirectory 语义）', async () => {
  const fetchMock = stubBase(
    handlerWith(
      [
        { id: 't1', name: '电影' },
        { id: 't2', name: '剧集' },
      ],
      (url, init) => {
        if (url === '/api/tasks/t1/run' && init?.method === 'POST') {
          return { status: 202, body: { status: 'queued' } }
        }
        if (url === '/api/tasks/t2/run' && init?.method === 'POST') {
          return { status: 202, body: { status: 'queued' } }
        }
        throw new Error('unexpected fetch: ' + url)
      },
    ),
  )
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('button', { name: '运行当前' }))
  await waitFor(() => {
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/tasks/t1/run',
      expect.objectContaining({ method: 'POST', body: JSON.stringify({ path: '/' }) }),
    )
  })
  const runCalls = fetchMock.mock.calls.filter((call) => String(call[0]).includes('/run'))
  expect(runCalls).toHaveLength(1)
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
})
