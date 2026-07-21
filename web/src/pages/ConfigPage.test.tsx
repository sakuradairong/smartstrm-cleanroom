import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { App as AntApp } from 'antd'
import ConfigPage from './ConfigPage'
import type { ManagedConfig } from '../api/types'

const managed: ManagedConfig = {
  storages: [{ id: 'local', type: 'local', root: '/media', endpoint: '', username: '', password: '', token: '' }],
  tasks: [{
    id: 'movies', name: '电影', storage_id: 'local', source: '/movies', destination: '/strm', schedule: '',
    file_id_mode: false, dir_time_check: false, watch: false, incremental: true, sync_delete: false,
    media_ext: ['.mkv'], copy_ext: [], min_size: 0, max_size: 0, plugins: [], keep_local: [],
  }],
  plugins: [],
  integrations: { clouddrive2: {}, moviepilot: {} },
  tmdb: { api_key: '********', language: 'zh-CN' },
  media_tools: { ffmpeg: 'ffmpeg', timeout_seconds: 60 },
  history: { path: '/data/history.json', max_entries: 200 },
  media_proxy: { enabled: false, listen: '', upstream: '', server_type: '', rewrite_playback_info: false },
}

function stubFetch(config: ManagedConfig = managed) {
  const mock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = typeof input === 'string' ? input : input.toString()
    let body: unknown
    if (url === '/api/config/managed' && !init?.method) {
      body = {
		managed: config,
        bootstrap: { listen: ':8024', public_url: 'https://strm.example', admin_username: 'admin' },
        backup_available: true,
        restart_required: false,
      }
    } else if (url === '/api/config/webhook-token' && init?.method === 'POST') {
      body = { webhook_token: 'new-token', restart_required: true }
    } else {
      throw new Error(`unexpected fetch: ${url} ${init?.method ?? 'GET'}`)
    }
    return { ok: true, status: 200, text: async () => JSON.stringify(body) } as Response
  })
  vi.stubGlobal('fetch', mock)
  return mock
}

function renderPage() {
  return render(<AntApp><ConfigPage /></AntApp>)
}

afterEach(() => vi.unstubAllGlobals())

test('加载并渲染四个配置卡片', async () => {
  stubFetch()
  const { container } = renderPage()
  expect(await screen.findByText('基础启动配置')).toBeInTheDocument()
  expect(screen.getAllByText('存储').length).toBeGreaterThan(0)
  expect(screen.getAllByText('生成任务')).toHaveLength(2)
  expect(screen.getAllByText('高级能力')).toHaveLength(2)
  expect(screen.getByText(':8024')).toBeInTheDocument()
  expect(screen.getByText('https://strm.example')).toBeInTheDocument()
  const navigation = screen.getByRole('navigation', { name: '配置区块导航' })
  for (const section of [/基础配置/, /存\s*储/, /生成任务/, /高级能力/]) {
    expect(within(navigation).getByRole('button', { name: section })).toBeInTheDocument()
  }
  expect(container.querySelector('.config-save-bar')).toBeInTheDocument()
  expect(screen.getAllByRole('columnheader', { name: '标识' })).toHaveLength(2)
  expect(screen.getByRole('columnheader', { name: '根路径 / 服务地址' })).toBeInTheDocument()
})

test('存储表格渲染已有存储', async () => {
  stubFetch()
  renderPage()
  expect((await screen.findAllByText('local')).length).toBeGreaterThan(0)
  expect(screen.getByText('/media')).toBeInTheDocument()
})

test('123 官方存储显示挂载 File ID 且不显示密钥', async () => {
	const panConfig: ManagedConfig = {
		...managed,
		storages: [{
			id: 'pan123', type: '123_open', root: '', endpoint: '', username: '', password: '', token: '',
			client_id: 'client-id', client_secret: '********', root_file_id: '777',
		}],
		tasks: [],
	}
	stubFetch(panConfig)
	renderPage()
	expect(await screen.findByText('挂载 File ID：777')).toBeInTheDocument()
	expect(screen.queryByText('********')).not.toBeInTheDocument()
})

test('删除被任务引用的存储被阻止并提示', async () => {
  const fetchMock = stubFetch()
  renderPage()
  const row = (await screen.findByText('/media')).closest('tr')!
  fireEvent.click(within(row).getByRole('button', { name: /删\s*除/ }))
  fireEvent.click(await screen.findByRole('button', { name: /确\s*定/ }))
  expect(await screen.findByText(/请先删除或调整引用该存储的任务：movies/)).toBeInTheDocument()
  expect(fetchMock.mock.calls.some(([, init]) => (init as RequestInit | undefined)?.method === 'PUT')).toBe(false)
})

test('重置 Token 需要确认且结果显示一次', async () => {
  const fetchMock = stubFetch()
  renderPage()
  fireEvent.click(await screen.findByRole('button', { name: /重置 Webhook Token/ }))
  expect(fetchMock).not.toHaveBeenCalledWith('/api/config/webhook-token', expect.anything())
  fireEvent.click(await screen.findByRole('button', { name: /确\s*定/ }))
  await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/config/webhook-token', expect.objectContaining({ method: 'POST' })))
  const dialog = await screen.findByRole('dialog')
  expect(within(dialog).getByText(/仅显示一次/)).toBeInTheDocument()
  expect(within(dialog).getByText('new-token')).toBeInTheDocument()
})
