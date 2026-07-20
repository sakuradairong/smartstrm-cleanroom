import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { App as AntApp } from 'antd'
import TaskToolsModal from './TaskToolsModal'

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

function renderModal() {
  const onChanged = vi.fn()
  render(
    <AntApp>
      <TaskToolsModal taskId="t1" open onClose={() => {}} onChanged={onChanged} />
    </AntApp>,
  )
  return { onChanged }
}

function bodyOf(mock: ReturnType<typeof stubFetch>, call: number) {
  return JSON.parse(mock.mock.calls[call][1]?.body as string)
}

afterEach(() => {
  vi.unstubAllGlobals()
})

test('内容替换两段式：先预览后执行', async () => {
  const fetchMock = stubFetch((url, init) => {
    if (url === '/api/tasks/t1/replace-content' && init?.method === 'POST') {
      return { body: { scanned: 5, changed: 2 } }
    }
    throw new Error('unexpected fetch: ' + url)
  })
  const user = userEvent.setup()
  const { onChanged } = renderModal()
  const dialog = await screen.findByRole('dialog')
  // 默认工具即“内容替换”
  await user.click(within(dialog).getByPlaceholderText('查找内容'))
  await user.paste('http://old')
  await user.click(within(dialog).getByPlaceholderText('替换为'))
  await user.paste('http://new')
  await user.click(within(dialog).getByRole('button', { name: /预\s*览/ }))
  expect(await within(dialog).findByText(/扫描 5 个 STRM，将修改 2 个/)).toBeInTheDocument()
  expect(fetchMock).toHaveBeenCalledTimes(1)
  expect(bodyOf(fetchMock, 0)).toEqual({ from: 'http://old', to: 'http://new', preview: true })
  await user.click(within(dialog).getByRole('button', { name: '确认执行' }))
  expect(await screen.findByText('已修改 2 个 STRM')).toBeInTheDocument()
  expect(fetchMock).toHaveBeenCalledTimes(2)
  expect(bodyOf(fetchMock, 1)).toEqual({ from: 'http://old', to: 'http://new', preview: false })
  expect(onChanged).toHaveBeenCalled()
}, 15000)

test('全量覆写需要危险确认后才发出 POST', async () => {
  const fetchMock = stubFetch((url, init) => {
    if (url === '/api/tasks/t1/full-overwrite' && init?.method === 'POST') {
      return { status: 202, body: { status: 'queued' } }
    }
    throw new Error('unexpected fetch: ' + url)
  })
  const user = userEvent.setup()
  renderModal()
  const dialog = await screen.findByRole('dialog')
  await user.click(within(dialog).getByText('全量覆写'))
  await user.click(within(dialog).getByRole('button', { name: '执行覆写' }))
  // 出现确认框，确认前不得发出请求
  await screen.findByText(/将重新生成该任务的全部生成文件/)
  expect(fetchMock).not.toHaveBeenCalled()
  await user.click(screen.getByRole('button', { name: /确\s*定/ }))
  await waitFor(() => {
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/tasks/t1/full-overwrite',
      expect.objectContaining({ method: 'POST', body: JSON.stringify({ path: '' }) }),
    )
  })
  expect(await screen.findByText('全量覆写已加入队列')).toBeInTheDocument()
})

test('清除生成文件需要危险确认并调用 DELETE', async () => {
  const fetchMock = stubFetch((url, init) => {
    if (url === '/api/tasks/t1/generated' && init?.method === 'DELETE') {
      return { body: { removed: 7 } }
    }
    throw new Error('unexpected fetch: ' + url)
  })
  const user = userEvent.setup()
  const { onChanged } = renderModal()
  const dialog = await screen.findByRole('dialog')
  await user.click(within(dialog).getByText('清除生成文件'))
  await user.click(within(dialog).getByRole('button', { name: '清除全部生成文件' }))
  await screen.findByText(/永久删除该任务目录中的全部生成文件/)
  expect(fetchMock).not.toHaveBeenCalled()
  await user.click(screen.getByRole('button', { name: /确\s*定/ }))
  await waitFor(() => {
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/tasks/t1/generated',
      expect.objectContaining({ method: 'DELETE' }),
    )
  })
  expect(await screen.findByText('已清除 7 个顶层项目')).toBeInTheDocument()
  expect(onChanged).toHaveBeenCalled()
})

test('封面提取按后端 CoverOptions 字段两段式提交', async () => {
  const fetchMock = stubFetch((url, init) => {
    if (url === '/api/tasks/t1/covers' && init?.method === 'POST') {
      const body = JSON.parse(init?.body as string)
      if (body.preview === true) {
        return { body: { scanned: 5, planned: 3, created: 0, skipped: 1, changes: [] } }
      }
      return { body: { scanned: 5, planned: 3, created: 3, skipped: 1, changes: [] } }
    }
    throw new Error('unexpected fetch: ' + url)
  })
  const user = userEvent.setup()
  const { onChanged } = renderModal()
  const dialog = await screen.findByRole('dialog')
  await user.click(within(dialog).getByText('提取封面'))
  const pathInput = within(dialog).getByPlaceholderText('任务内子目录（/ 表示全部）')
  await user.clear(pathInput)
  await user.click(pathInput)
  await user.paste('/sub')
  await user.click(within(dialog).getByRole('switch'))
  await user.click(within(dialog).getByRole('button', { name: /预\s*览/ }))
  expect(
    await within(dialog).findByText(/扫描 5 个 STRM，计划生成 3 张封面，跳过 1 张/),
  ).toBeInTheDocument()
  expect(fetchMock).toHaveBeenCalledTimes(1)
  expect(bodyOf(fetchMock, 0)).toEqual({ path: '/sub', position: '10s', overwrite: true, preview: true })
  await user.click(within(dialog).getByRole('button', { name: '确认执行' }))
  expect(await screen.findByText('已生成 3 张封面')).toBeInTheDocument()
  expect(fetchMock).toHaveBeenCalledTimes(2)
  expect(bodyOf(fetchMock, 1)).toEqual({ path: '/sub', position: '10s', overwrite: true, preview: false })
  expect(onChanged).toHaveBeenCalled()
})
