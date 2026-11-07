import { act, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { App as AntApp } from 'antd'
import StorageBrowserPage from './StorageBrowserPage'

afterEach(() => { vi.unstubAllGlobals(); vi.restoreAllMocks() })

type Result = { status?: number; body: unknown }

function setup(
  second: Promise<Result>,
  firstMatches: Promise<Result> = Promise.resolve({ body: [] }),
) {
  const mutations: string[] = []
  vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input)
    let result: Result = { body: [] }
    if (init?.method && init.method !== 'GET') mutations.push(url)
    if (url === '/api/storages') result = { body: ['first', 'second', 'third'].map(id => ({ id, type: 'local', root: `/${id}` })) }
    else if (url === '/api/bootstrap') result = { body: { tmdb_configured: false } }
    else if (url.startsWith('/api/storages/first/entries')) result = { body: [{ path: '/same.mkv', name: 'same.mkv', size: 1, is_dir: false, modified: '' }] }
    else if (url.startsWith('/api/storages/first/matching-tasks')) result = await firstMatches
    else if (url.startsWith('/api/storages/second/entries')) result = await second
    else if (url.startsWith('/api/storages/third/entries')) result = { body: [{ path: '/third.mkv', name: 'third.mkv', size: 1, is_dir: false, modified: '' }] }
    const status = result.status ?? 200
    return { ok: status >= 200 && status < 300, status, text: async () => JSON.stringify(result.body) } as Response
  }))
  render(<MemoryRouter><AntApp><StorageBrowserPage /></AntApp></MemoryRouter>)
  return mutations
}

async function switchTo(user: ReturnType<typeof userEvent.setup>, id: string) {
  await user.click(screen.getByRole('combobox', { name: '存储' }))
  await user.click(await screen.findByText(`${id} (local)`))
}

test('failed storage switch clears old rows and disables mutations', async () => {
  const mutations = setup(Promise.resolve({ status: 502, body: { error: 'second storage unavailable' } }))
  const user = userEvent.setup()
  await screen.findByText('same.mkv')
  await switchTo(user, 'second')
  await screen.findByText('second storage unavailable')
  expect(screen.queryByText('same.mkv')).not.toBeInTheDocument()
  expect(screen.queryByRole('button', { name: '删除' })).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: '新建目录' })).toBeDisabled()
  expect(screen.getByRole('button', { name: '批量命名' })).toBeDisabled()
  expect(mutations).toEqual([])
})

test('slow response cannot restore rows after a newer storage switch', async () => {
  let resolve!: (value: Result) => void
  const pending = new Promise<Result>(done => { resolve = done })
  const mutations = setup(pending)
  const user = userEvent.setup()
  const row = (await screen.findByText('same.mkv')).closest('tr')!
  await user.click(within(row).getByRole('button', { name: '重命名' }))
  expect(await screen.findByRole('dialog')).toBeInTheDocument()
  await switchTo(user, 'second')
  expect(screen.queryByText('same.mkv')).not.toBeInTheDocument()
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: '新建目录' })).toBeDisabled()
  await switchTo(user, 'third')
  await screen.findByText('third.mkv')
  await act(async () => { resolve({ body: [{ path: '/stale.mkv', name: 'stale.mkv', size: 1, is_dir: false, modified: '' }] }) })
  expect(screen.queryByText('stale.mkv')).not.toBeInTheDocument()
  const currentRow = screen.getByText('third.mkv').closest('tr')!
  await user.click(within(currentRow).getByRole('button', { name: '删除' }))
  await user.click(await screen.findByRole('button', { name: /确\s*定/ }))
  expect(mutations).toEqual(['/api/storages/third/entry?path=%2Fthird.mkv'])
})

test('slow task lookup does not block actions or restore a stale task', async () => {
  let resolveMatches!: (value: Result) => void
  const pendingMatches = new Promise<Result>(done => { resolveMatches = done })
  setup(Promise.resolve({ body: [] }), pendingMatches)
  const user = userEvent.setup()

  await screen.findByText('same.mkv')
  expect(screen.getByRole('button', { name: '新建目录' })).toBeEnabled()

  await switchTo(user, 'third')
  await screen.findByText('third.mkv')
  await act(async () => {
    resolveMatches({ body: [{ id: 'stale-task', name: 'Stale task' }] })
  })

  expect(screen.queryByRole('button', { name: '运行当前' })).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: '新建目录' })).toBeEnabled()
})
