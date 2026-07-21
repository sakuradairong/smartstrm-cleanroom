import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { App as AntApp } from 'antd'
import ConfigPage from './ConfigPage'
import type { ManagedConfig } from '../api/types'

const managed: ManagedConfig = {
  storages: [], tasks: [], plugins: [], integrations: { clouddrive2: { '/cloud': '/media' }, moviepilot: {} },
  tmdb: { api_key: '********', language: 'zh-CN', cache_minutes: 60, base_url: '', image_base_url: '', proxy_url: '' },
  history: { path: '/data/history.jsonl', max_entries: 1000 }, media_tools: { ffmpeg: 'ffmpeg', timeout_seconds: 30 },
  media_proxy: { enabled: false, listen: ':8097', upstream: 'http://emby:8096', server_type: 'emby', rewrite_playback_info: false },
}

function setupFetch(putStatus = 200) {
  let putBody: ManagedConfig | undefined
  vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input)
    if (url === '/api/config/managed' && !init?.method) return response({ managed, bootstrap: { listen: ':8024', public_url: '', admin_username: '' }, backup_available: false })
    if (url === '/api/config/managed' && init?.method === 'PUT') {
      putBody = JSON.parse(String(init.body))
      return putStatus === 200 ? response({ managed: putBody, restart_required: true }) : response({ error: 'invalid cron' }, 400)
    }
    throw new Error(`unexpected ${url}`)
  }))
  return () => putBody
}

function response(body: unknown, status = 200): Response {
  return { ok: status < 400, status, text: async () => JSON.stringify(body) } as Response
}

afterEach(() => vi.unstubAllGlobals())

test('渲染全部高级能力字段', async () => {
  setupFetch()
  render(<AntApp><ConfigPage /></AntApp>)
  for (const label of ['TMDB API Key', 'TMDB 语言', '缓存分钟数', 'TMDB API 地址', '图片地址', '代理 URL', '历史文件路径', '历史最大条数', 'FFmpeg 路径', '媒体工具超时（秒）', '启用媒体代理', 'CloudDrive2 路径映射', 'MoviePilot 路径映射', '全局插件']) {
    expect(await screen.findByLabelText(label)).toBeInTheDocument()
  }
})

test('媒体代理启用后显示代理字段', async () => {
  setupFetch()
  const user = userEvent.setup()
  render(<AntApp><ConfigPage /></AntApp>)
  await user.click(await screen.findByLabelText('启用媒体代理'))
  for (const label of ['媒体代理监听', '媒体代理上游', '媒体服务器类型', '改写 PlaybackInfo']) expect(screen.getByLabelText(label)).toBeInTheDocument()
})

test('TMDB Key 保持脱敏值提交并显示重启提示', async () => {
  const getPutBody = setupFetch()
  const user = userEvent.setup()
  render(<AntApp><ConfigPage /></AntApp>)
  await user.click(await screen.findByRole('button', { name: /验证并保存全部设置/ }))
  await waitFor(() => expect(getPutBody()?.tmdb.api_key).toBe('********'))
  expect(await screen.findByRole('alert')).toHaveTextContent('配置已写入，请重启服务后生效')
})

test('JSON 非法时阻止保存', async () => {
  const getPutBody = setupFetch()
  const user = userEvent.setup()
  render(<AntApp><ConfigPage /></AntApp>)
  const field = await screen.findByLabelText('CloudDrive2 路径映射')
  fireEvent.change(field, { target: { value: '{bad' } })
  await user.click(screen.getByRole('button', { name: /验证并保存全部设置/ }))
  expect(getPutBody()).toBeUndefined()
  expect(screen.getByText(/请先修正高级能力中的 JSON 格式错误/)).toBeInTheDocument()
})

test('保存失败显示后端错误且草稿保留', async () => {
  setupFetch(400)
  render(<AntApp><ConfigPage /></AntApp>)
  const language = await screen.findByLabelText('TMDB 语言')
  fireEvent.change(language, { target: { value: 'en-US' } })
  fireEvent.click(screen.getByRole('button', { name: /验证并保存全部设置/ }))
  expect((await screen.findAllByText('invalid cron')).length).toBeGreaterThan(0)
  expect(language).toHaveValue('en-US')
})
