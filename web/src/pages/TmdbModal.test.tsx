import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { App as AntApp } from 'antd'
import TmdbModal from './TmdbModal'
import StorageBrowserPage from './StorageBrowserPage'

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

// 与旧 UI 拼接方式一致：query/type/year 全部携带
const SEARCH_URL = (query: string, type = 'tv', year = '') =>
  `/api/tmdb/search?query=${encodeURIComponent(query)}&type=${type}&year=${encodeURIComponent(year)}`

const RESULT = {
  id: 42,
  media_type: 'tv',
  title: '示例剧集',
  original_title: 'Demo Series',
  release_date: '2024-05-06',
  poster_path: '/poster.jpg',
  overview: '',
  vote_average: 8.5,
}

const TV_TEMPLATE = '{title} - S{season:2}E{episode:2}{.,episode_name}{ext}'

function renderModal() {
  const onDone = vi.fn()
  const onClose = vi.fn()
  render(
    <AntApp>
      <TmdbModal storageId="local" path="/shows" open onClose={onClose} onDone={onDone} />
    </AntApp>,
  )
  return { onDone, onClose }
}

function bodyOf(mock: ReturnType<typeof stubFetch>, call: number) {
  return JSON.parse(mock.mock.calls[call][1]?.body as string)
}

afterEach(() => {
  vi.unstubAllGlobals()
})

test('搜索表单提交并渲染结果卡片（≤12）', async () => {
  const many = Array.from({ length: 15 }, (_, i) => ({ ...RESULT, id: i + 1, title: `剧集${i + 1}` }))
  const fetchMock = stubFetch((url) => {
    if (url === SEARCH_URL('妖精的尾巴')) return { body: many }
    throw new Error('unexpected fetch: ' + url)
  })
  const user = userEvent.setup()
  renderModal()
  const dialog = await screen.findByRole('dialog')
  await user.click(within(dialog).getByPlaceholderText('影视名称'))
  await user.paste('妖精的尾巴')
  await user.click(within(dialog).getByRole('button', { name: /搜\s*索/ }))
  const cards = await within(dialog).findAllByTestId('tmdb-card')
  // 旧 UI results.slice(0,12)：最多渲染 12 张卡片
  expect(cards).toHaveLength(12)
  expect(within(dialog).queryByText('剧集13')).not.toBeInTheDocument()
  expect(within(cards[0]).getByText('剧集1')).toBeInTheDocument()
  expect(within(cards[0]).getByText(/2024-05-06 · 8\.5/)).toBeInTheDocument()
  // 海报地址按旧 UI 拼接 /api/tmdb/poster?path=<encodeURIComponent(poster_path)>
  const img = cards[0].querySelector('img')
  expect(img).not.toBeNull()
  expect(img?.getAttribute('src')).toBe('/api/tmdb/poster?path=' + encodeURIComponent('/poster.jpg'))
  expect(fetchMock).toHaveBeenCalledWith(SEARCH_URL('妖精的尾巴'), undefined)
}, 15000)

test('选择结果后进入模板步骤', async () => {
  stubFetch((url) => {
    if (url === SEARCH_URL('示例')) return { body: [RESULT] }
    throw new Error('unexpected fetch: ' + url)
  })
  const user = userEvent.setup()
  renderModal()
  const dialog = await screen.findByRole('dialog')
  await user.click(within(dialog).getByPlaceholderText('影视名称'))
  await user.paste('示例')
  await user.click(within(dialog).getByRole('button', { name: /搜\s*索/ }))
  const card = await within(dialog).findByTestId('tmdb-card')
  await user.click(card)
  // tv 默认模板与旧 selectTMDB 一致
  expect(await within(dialog).findByDisplayValue(TV_TEMPLATE)).toBeInTheDocument()
  // 变量说明逐字移植旧 UI
  expect(
    within(dialog).getByText(
      '命名模板变量：{title} {title_original} {year} {tmdbid} {season} {episode} {episode_name} {quality} {ext}',
    ),
  ).toBeInTheDocument()
}, 15000)

test('应用前显示预览确认', async () => {
  const fetchMock = stubFetch((url, init) => {
    if (url === SEARCH_URL('示例')) return { body: [RESULT] }
    if (url === '/api/storages/local/tmdb-rename' && init?.method === 'POST') {
      const body = JSON.parse(init.body as string)
      return {
        body: {
          preview: body.preview,
          details: RESULT,
          changes: [{ from: 'demo.s01e01.mkv', to: '示例剧集 - S01E01.第一集.mkv' }],
        },
      }
    }
    throw new Error('unexpected fetch: ' + url)
  })
  const user = userEvent.setup()
  const { onDone } = renderModal()
  const dialog = await screen.findByRole('dialog')
  await user.click(within(dialog).getByPlaceholderText('影视名称'))
  await user.paste('示例')
  await user.click(within(dialog).getByRole('button', { name: /搜\s*索/ }))
  await user.click(await within(dialog).findByTestId('tmdb-card'))
  await within(dialog).findByDisplayValue(TV_TEMPLATE)
  await user.click(within(dialog).getByRole('button', { name: /预\s*览/ }))
  expect(await within(dialog).findByText('demo.s01e01.mkv')).toBeInTheDocument()
  expect(within(dialog).getByText('示例剧集 - S01E01.第一集.mkv')).toBeInTheDocument()
  expect(fetchMock).toHaveBeenCalledTimes(2)
  // 请求体字段与 app.go tmdbRename 处理器一致
  expect(bodyOf(fetchMock, 1)).toEqual({
    path: '/shows',
    media_type: 'tv',
    tmdb_id: 42,
    template: TV_TEMPLATE,
    preview: true,
  })
  await user.click(within(dialog).getByRole('button', { name: '确认执行' }))
  expect(await screen.findByText('影视识别重命名完成')).toBeInTheDocument()
  await waitFor(() => expect(onDone).toHaveBeenCalled())
  expect(fetchMock).toHaveBeenCalledTimes(3)
  expect(bodyOf(fetchMock, 2)).toEqual({
    path: '/shows',
    media_type: 'tv',
    tmdb_id: 42,
    template: TV_TEMPLATE,
    preview: false,
  })
}, 15000)

test('搜索错误显示后端 error', async () => {
  stubFetch((url) => {
    if (url === SEARCH_URL('示例')) {
      return { status: 502, body: { error: 'TMDB 请求失败: connection refused' } }
    }
    throw new Error('unexpected fetch: ' + url)
  })
  const user = userEvent.setup()
  renderModal()
  const dialog = await screen.findByRole('dialog')
  await user.click(within(dialog).getByPlaceholderText('影视名称'))
  await user.paste('示例')
  await user.click(within(dialog).getByRole('button', { name: /搜\s*索/ }))
  expect(await within(dialog).findByText('TMDB 请求失败: connection refused')).toBeInTheDocument()
  // 停留在搜索步骤
  expect(within(dialog).getByPlaceholderText('影视名称')).toBeInTheDocument()
  expect(within(dialog).queryByTestId('tmdb-card')).not.toBeInTheDocument()
})

function stubPageFetch(tmdbConfigured: boolean) {
  return stubFetch((url) => {
    if (url === '/api/bootstrap') return { body: { tmdb_configured: tmdbConfigured } }
    if (url === '/api/storages') return { body: [{ id: 'local', type: 'local', root: '/' }] }
    if (url.startsWith('/api/storages/local/entries')) return { body: [] }
    if (url.startsWith('/api/storages/local/matching-tasks')) return { body: [] }
    throw new Error('unexpected fetch: ' + url)
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

test('tmdb_configured=false 时影视识别按钮禁用且提示', async () => {
  stubPageFetch(false)
  const user = userEvent.setup()
  renderPage()
  const button = await screen.findByRole('button', { name: '影视识别' })
  await waitFor(() => expect(button).toBeDisabled())
  await user.hover(button)
  // 提示文案与旧 UI 注入的警告一致（app.go renderIndexHTML）
  expect(await screen.findByText('TMDB API Key 未配置，请在配置页设置后重启服务')).toBeInTheDocument()
})

test('tmdb_configured=true 时按钮可用并打开识别弹窗', async () => {
  stubPageFetch(true)
  const user = userEvent.setup()
  renderPage()
  const button = await screen.findByRole('button', { name: '影视识别' })
  await waitFor(() => expect(button).toBeEnabled())
  await user.click(button)
  const dialog = await screen.findByRole('dialog')
  expect(within(dialog).getByPlaceholderText('影视名称')).toBeInTheDocument()
})
