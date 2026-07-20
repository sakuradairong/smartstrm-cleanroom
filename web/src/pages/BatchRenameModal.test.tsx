import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { App as AntApp } from 'antd'
import BatchRenameModal from './BatchRenameModal'

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
  const onDone = vi.fn()
  const onClose = vi.fn()
  render(
    <AntApp>
      <BatchRenameModal storageId="local" path="/movies" open onClose={onClose} onDone={onDone} />
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

test('正则模式预览后确认执行', async () => {
  const fetchMock = stubFetch((url, init) => {
    if (url === '/api/storages/local/batch-rename' && init?.method === 'POST') {
      const body = JSON.parse(init.body as string)
      return {
        body: {
          preview: body.preview,
          changes: [
            { from: 'EP01.mkv', to: 'E01.mkv' },
            { from: 'EP02.mkv', to: 'E02.mkv' },
          ],
        },
      }
    }
    throw new Error('unexpected fetch: ' + url)
  })
  const user = userEvent.setup()
  const { onDone } = renderModal()
  const dialog = await screen.findByRole('dialog')
  // 默认即 regex 模式，使用旧 UI 的默认正则与替换内容
  await user.click(within(dialog).getByRole('button', { name: /预\s*览/ }))
  expect(await within(dialog).findByText('EP01.mkv')).toBeInTheDocument()
  expect(within(dialog).getByText('E01.mkv')).toBeInTheDocument()
  expect(within(dialog).getByText('EP02.mkv')).toBeInTheDocument()
  expect(within(dialog).getByText('E02.mkv')).toBeInTheDocument()
  expect(fetchMock).toHaveBeenCalledTimes(1)
  // 请求体字段与 app.go batchRename 处理器一致
  expect(bodyOf(fetchMock, 0)).toEqual({
    path: '/movies',
    mode: 'regex',
    pattern: 'EP(\\d+)',
    replacement: 'E$1',
    preview: true,
  })
  await user.click(within(dialog).getByRole('button', { name: '确认执行' }))
  expect(await screen.findByText('批量命名完成')).toBeInTheDocument()
  await waitFor(() => expect(onDone).toHaveBeenCalled())
  expect(fetchMock).toHaveBeenCalledTimes(2)
  expect(bodyOf(fetchMock, 1)).toEqual({
    path: '/movies',
    mode: 'regex',
    pattern: 'EP(\\d+)',
    replacement: 'E$1',
    preview: false,
  })
}, 15000)

test('三种模式切换渲染对应字段', async () => {
  stubFetch(() => {
    throw new Error('unexpected fetch')
  })
  const user = userEvent.setup()
  renderModal()
  const dialog = await screen.findByRole('dialog')
  // 默认 regex（正则）
  expect(within(dialog).getByPlaceholderText('匹配正则')).toBeInTheDocument()
  expect(within(dialog).getByPlaceholderText('替换内容')).toBeInTheDocument()
  // sequence（顺序）
  await user.click(within(dialog).getByText('sequence（顺序）'))
  expect(within(dialog).getByPlaceholderText('前缀')).toBeInTheDocument()
  expect(within(dialog).getByPlaceholderText('起始编号')).toBeInTheDocument()
  expect(within(dialog).getByPlaceholderText('补位宽度')).toBeInTheDocument()
  expect(within(dialog).getByPlaceholderText('后缀')).toBeInTheDocument()
  expect(within(dialog).queryByPlaceholderText('匹配正则')).not.toBeInTheDocument()
  // magic（魔法变量）
  await user.click(within(dialog).getByText('magic（魔法变量）'))
  expect(
    within(dialog).getByPlaceholderText('模板（支持 {title} {season} {episode} {ext}）'),
  ).toBeInTheDocument()
  expect(within(dialog).queryByPlaceholderText('前缀')).not.toBeInTheDocument()
  expect(within(dialog).queryByPlaceholderText('匹配正则')).not.toBeInTheDocument()
})

test('预览结果冲突时展示且仍需用户确认', async () => {
  const fetchMock = stubFetch((url, init) => {
    if (url === '/api/storages/local/batch-rename' && init?.method === 'POST') {
      // 后端 renamer.validate 冲突 → 400 + error 文案
      return { status: 400, body: { error: 'validate rename plan: duplicate target name "E01.mkv"' } }
    }
    throw new Error('unexpected fetch: ' + url)
  })
  const user = userEvent.setup()
  const { onDone } = renderModal()
  const dialog = await screen.findByRole('dialog')
  await user.click(within(dialog).getByRole('button', { name: /预\s*览/ }))
  expect(await within(dialog).findByText(/duplicate target name "E01\.mkv"/)).toBeInTheDocument()
  // 冲突未解决前不得出现确认执行，也不得发出第二个请求
  expect(within(dialog).queryByRole('button', { name: '确认执行' })).not.toBeInTheDocument()
  expect(fetchMock).toHaveBeenCalledTimes(1)
  expect(onDone).not.toHaveBeenCalled()
})
