import { api, ApiError, getJSON, postJSON, putJSON, deleteJSON } from './client'

function mockFetch(status: number, body: unknown) {
  return vi.fn(async (_path: string, _init?: RequestInit) => new Response(body === null ? null : JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  }))
}

beforeEach(() => { vi.stubGlobal('fetch', vi.fn()) })
afterEach(() => { vi.unstubAllGlobals() })

test('2xx 解析 JSON 响应', async () => {
  vi.stubGlobal('fetch', mockFetch(200, { ok: 1 }))
  await expect(getJSON<{ ok: number }>('/api/x')).resolves.toEqual({ ok: 1 })
})

test('204 返回 undefined', async () => {
  vi.stubGlobal('fetch', mockFetch(204, null))
  await expect(api('/api/x')).resolves.toBeUndefined()
})

test('非 2xx 提取后端 error 字段与状态码', async () => {
  vi.stubGlobal('fetch', mockFetch(400, { error: 'storage id is required' }))
  const err = (await api('/api/x').catch(e => e)) as ApiError
  expect(err).toBeInstanceOf(ApiError)
  expect(err.message).toBe('storage id is required')
  expect(err.status).toBe(400)
})

test('非 2xx 无 error 字段时回退状态码文案', async () => {
  vi.stubGlobal('fetch', mockFetch(500, null))
  const err = (await api('/api/x').catch(e => e)) as ApiError
  expect(err.message).toBe('请求失败（HTTP 500）')
})

test('网络异常抛中文网络错误', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => { throw new TypeError('failed to fetch') }))
  const err = (await api('/api/x').catch(e => e)) as ApiError
  expect(err.message).toBe('网络错误，请检查连接后重试')
  expect(err.status).toBe(0)
})

test('postJSON 发送 JSON 方法与请求体', async () => {
  const spy = mockFetch(200, {})
  vi.stubGlobal('fetch', spy)
  await postJSON('/api/x', { a: 1 })
  const [path, init] = spy.mock.calls[0]
  expect(path).toBe('/api/x')
  expect(init?.method).toBe('POST')
  expect(init?.body).toBe('{"a":1}')
})

test('putJSON/deleteJSON 使用对应方法', async () => {
  const spy = mockFetch(200, {})
  vi.stubGlobal('fetch', spy)
  await putJSON('/api/x', {})
  await deleteJSON('/api/y')
  expect(spy.mock.calls[0][1]?.method).toBe('PUT')
  expect(spy.mock.calls[1][1]?.method).toBe('DELETE')
})
