import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { App as AntApp } from 'antd'
import StorageBrowserPage from './StorageBrowserPage'
import type { StorageEntry, StorageSummary } from '../api/types'

const STORAGES: StorageSummary[] = [{ id: 'local', type: 'local', root: '/' }]

const ROOT_ENTRIES: StorageEntry[] = [
  { path: '/movies', name: 'movies', size: 0, is_dir: true, modified: '2025-01-02 03:04:05' },
  { path: '/b.mkv', name: 'b.mkv', size: 2048, is_dir: false, modified: '2025-01-03 00:00:00' },
  { path: '/a.mkv', name: 'a.mkv', size: 1024, is_dir: false, modified: '2025-01-01 00:00:00' },
]

const MOVIES_ENTRIES: StorageEntry[] = [
  { path: '/movies/sub.mkv', name: 'sub.mkv', size: 1, is_dir: false, modified: '' },
]

const ENTRIES_URL = '/api/storages/local/entries?path=' + encodeURIComponent('/')
const MOVIES_ENTRIES_URL = '/api/storages/local/entries?path=' + encodeURIComponent('/movies')

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

function stubBase(handler: FetchHandler = () => { throw new Error('unexpected fetch') }) {
  return stubFetch((url, init) => {
    if (url === '/api/storages') return { body: STORAGES }
    if (url === ENTRIES_URL) return { body: ROOT_ENTRIES }
    if (url === MOVIES_ENTRIES_URL) return { body: MOVIES_ENTRIES }
    return handler(url, init)
  })
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

function bodyRows(): HTMLElement[] {
  return screen.getAllByRole('row').slice(1) as HTMLElement[]
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

test('加载存储并浏览根目录', async () => {
  stubBase()
  const { container } = renderPage()
  expect(await screen.findByText('a.mkv')).toBeInTheDocument()
  expect(screen.getByText('b.mkv')).toBeInTheDocument()
  expect(screen.getByText('movies')).toBeInTheDocument()
  expect(screen.getByText('目录')).toBeInTheDocument()
  expect(screen.getAllByText('文件')).toHaveLength(2)
  expect(screen.getByRole('status')).toHaveTextContent('显示 3 / 共 3 项')
  expect(screen.getByRole('group', { name: '位置导航' })).toBeInTheDocument()
  expect(screen.getByRole('group', { name: '筛选和显示' })).toBeInTheDocument()
  expect(screen.getByRole('group', { name: '目录操作' })).toBeInTheDocument()
  expect(container.querySelector('.storage-table .ant-table-content')).toHaveStyle({ overflowX: 'auto' })
})

test('筛选输入本地过滤并显示计数', async () => {
  stubBase()
  const user = userEvent.setup()
  renderPage()
  await screen.findByText('a.mkv')
  await user.type(screen.getByPlaceholderText('筛选'), 'mkv')
  expect(screen.getByText('a.mkv')).toBeInTheDocument()
  expect(screen.getByText('b.mkv')).toBeInTheDocument()
  expect(screen.queryByText('movies')).not.toBeInTheDocument()
  expect(screen.getByRole('status')).toHaveTextContent('显示 2 / 共 3 项')
})

test('点击名称表头切换排序方向', async () => {
  stubBase()
  const user = userEvent.setup()
  renderPage()
  await screen.findByText('a.mkv')
  // 默认名称升序
  let rows = bodyRows()
  expect(within(rows[0]).getByText('a.mkv')).toBeInTheDocument()
  expect(within(rows[1]).getByText('b.mkv')).toBeInTheDocument()
  expect(within(rows[2]).getByText('movies')).toBeInTheDocument()
  await user.click(screen.getAllByRole('columnheader', { name: /名称/ })[0])
  await waitFor(() => {
    rows = bodyRows()
    expect(within(rows[0]).getByText('movies')).toBeInTheDocument()
  })
  expect(within(rows[1]).getByText('b.mkv')).toBeInTheDocument()
  expect(within(rows[2]).getByText('a.mkv')).toBeInTheDocument()
})

test('筛选词大小写不敏感', async () => {
  stubBase()
  const user = userEvent.setup()
  renderPage()
  await screen.findByText('a.mkv')
  await user.type(screen.getByPlaceholderText('筛选'), 'MKV')
  expect(screen.getByText('a.mkv')).toBeInTheDocument()
  expect(screen.getByText('b.mkv')).toBeInTheDocument()
  expect(screen.queryByText('movies')).not.toBeInTheDocument()
})

test('空目录显示中文空态', async () => {
  stubFetch((url) => {
    if (url === '/api/storages') return { body: STORAGES }
    if (url === ENTRIES_URL) return { body: [] }
    throw new Error('unexpected fetch: ' + url)
  })
  renderPage()
  expect(await screen.findByText('当前目录为空')).toBeInTheDocument()
  expect(screen.queryByText('No data')).not.toBeInTheDocument()
})

test('没有存储时引导用户前往添加存储', async () => {
  stubFetch((url) => {
    if (url === '/api/storages') return { body: [] }
    throw new Error('unexpected fetch: ' + url)
  })
  renderPage()
  expect(await screen.findByText(/尚未添加存储/)).toBeInTheDocument()
  expect(screen.getByRole('button', { name: '前往添加存储' })).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: '新建目录' })).not.toBeInTheDocument()
})

test('TMDB 未配置时直接显示禁用原因', async () => {
  stubBase((url) => {
    if (url === '/api/bootstrap') return { body: { tmdb_configured: false } }
    throw new Error('unexpected fetch: ' + url)
  })
  renderPage()
  const button = await screen.findByRole('button', { name: '影视识别' })
  await waitFor(() => expect(button).toBeDisabled())
  expect(screen.getByText('未配置 TMDB API Key')).toBeInTheDocument()
})

test('重命名流程：modal 输入新名后调用 rename 接口', async () => {
  const fetchMock = stubBase((url, init) => {
    if (url === '/api/storages/local/rename' && init?.method === 'POST') return { body: { status: 'renamed' } }
    throw new Error('unexpected fetch: ' + url)
  })
  const user = userEvent.setup()
  renderPage()
  const row = await rowOf('a.mkv')
  await user.click(within(row).getByRole('button', { name: '重命名' }))
  const dialog = await screen.findByRole('dialog')
  const input = within(dialog).getByDisplayValue('a.mkv')
  await user.clear(input)
  await user.type(input, 'c.mkv')
  await user.click(within(dialog).getByRole('button', { name: /确\s*定/ }))
  await waitFor(() => {
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/storages/local/rename',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ path: '/a.mkv', new_name: 'c.mkv' }),
      }),
    )
  })
})

test('删除需要确认后调用 DELETE entry', async () => {
  const fetchMock = stubBase((url, init) => {
    if (url === '/api/storages/local/entry?path=' + encodeURIComponent('/a.mkv') && init?.method === 'DELETE') {
      return { status: 204 }
    }
    throw new Error('unexpected fetch: ' + url)
  })
  const user = userEvent.setup()
  renderPage()
  const row = await rowOf('a.mkv')
  await user.click(within(row).getByRole('button', { name: /删\s*除/ }))
  await user.click(await screen.findByRole('button', { name: /确\s*定/ }))
  await waitFor(() => {
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/storages/local/entry?path=' + encodeURIComponent('/a.mkv'),
      expect.objectContaining({ method: 'DELETE' }),
    )
  })
})

test('复制STRM地址写入剪贴板，仅文件行有该按钮', async () => {
  const streamURL = 'http://example.com/stream/local?path=%2Fb.mkv&sig=abc'
  stubBase((url) => {
    if (url === '/api/storages/local/stream-url?path=' + encodeURIComponent('/b.mkv')) {
      return { body: { url: streamURL } }
    }
    throw new Error('unexpected fetch: ' + url)
  })
  const user = userEvent.setup()
  // userEvent.setup() 会替换 navigator.clipboard，须在其后覆盖为我们的 spy。
  const writeText = vi.fn<(text: string) => Promise<void>>().mockResolvedValue(undefined)
  Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
  renderPage()
  const dirRow = await rowOf('movies')
  expect(within(dirRow).queryByRole('button', { name: /复制\s*STRM\s*地址/ })).not.toBeInTheDocument()
  const fileRow = await rowOf('b.mkv')
  await user.click(within(fileRow).getByRole('button', { name: /复制\s*STRM\s*地址/ }))
  await waitFor(() => expect(writeText).toHaveBeenCalledWith(streamURL))
  expect(await screen.findByText('STRM 地址已复制')).toBeInTheDocument()
})

test('上一级按钮拼接父路径', async () => {
  const fetchMock = stubBase()
  const user = userEvent.setup()
  renderPage()
  // 目录名渲染为 Typography.Link（无 href 的 <a>，无 link role），按文本点击
  await user.click(await screen.findByText('movies'))
  await screen.findByText('sub.mkv')
  await user.click(screen.getByRole('button', { name: '上一级' }))
  await screen.findByText('a.mkv')
  const urls = fetchMock.mock.calls.map((call) => String(call[0]))
  expect(urls).toContain(MOVIES_ENTRIES_URL)
  expect(urls.filter((u) => u === ENTRIES_URL).length).toBeGreaterThanOrEqual(2)
})
