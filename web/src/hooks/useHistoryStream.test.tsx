import { renderHook, act } from '@testing-library/react'
import { useHistoryStream } from './useHistoryStream'
import { getJSON } from '../api/client'
import type { HistoryEvent } from '../api/types'

vi.mock('../api/client', () => ({ getJSON: vi.fn() }))

const getJSONMock = vi.mocked(getJSON)

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

// rAF stub 为 setTimeout(0)，配合真实定时器手动 flush，保证确定性
const flushFrame = () => act(async () => { await new Promise(r => setTimeout(r, 0)) })

beforeEach(() => {
  FakeEventSource.instances = []
  getJSONMock.mockReset()
  vi.stubGlobal('EventSource', FakeEventSource)
  vi.stubGlobal('requestAnimationFrame', (cb: FrameRequestCallback) => setTimeout(() => cb(0), 0))
  vi.stubGlobal('cancelAnimationFrame', (id: number) => clearTimeout(id))
})

afterEach(() => {
  vi.unstubAllGlobals()
})

test('初始加载历史并建立 SSE', async () => {
  const initial = [makeEvent(2), makeEvent(1)]
  getJSONMock.mockResolvedValue(initial)
  const { result } = renderHook(() => useHistoryStream(''))
  await flushFrame()
  expect(getJSONMock).toHaveBeenCalledWith('/api/history?limit=200')
  expect(result.current.events).toEqual(initial)
  expect(FakeEventSource.instances).toHaveLength(1)
  expect(FakeEventSource.instances[0].url).toBe('/api/history/stream')
  expect(result.current.connected).toBe(false)
  act(() => { FakeEventSource.instances[0].onopen?.() })
  expect(result.current.connected).toBe(true)
  act(() => { FakeEventSource.instances[0].onerror?.() })
  expect(result.current.connected).toBe(false)
})

test('带任务过滤时初始请求与 SSE 均携带 task_id', async () => {
  getJSONMock.mockResolvedValue([])
  renderHook(() => useHistoryStream('t9'))
  await flushFrame()
  expect(getJSONMock).toHaveBeenCalledWith('/api/history?limit=200&task_id=t9')
  expect(FakeEventSource.instances[0].url).toBe('/api/history/stream?task_id=t9')
})

test('SSE 事件按 rAF 批量合并，重复 id 去重', async () => {
  getJSONMock.mockResolvedValue([makeEvent(1)])
  let renders = 0
  const { result } = renderHook(() => { renders++; return useHistoryStream('') })
  await flushFrame()
  const es = FakeEventSource.instances[0]
  act(() => { es.onopen?.() })
  renders = 0
  act(() => {
    es.emit(makeEvent(2))
    es.emit(makeEvent(2))
    es.emit(makeEvent(3))
  })
  // rAF 未触发前不更新
  expect(renders).toBe(0)
  expect(result.current.events.map(e => e.id)).toEqual([1])
  await flushFrame()
  // 同一帧内多条事件只触发一次渲染，且新事件在前
  expect(renders).toBe(1)
  expect(result.current.events.map(e => e.id)).toEqual([3, 2, 1])
  // 已展示的 id 再次推送被去重
  act(() => { es.emit(makeEvent(2)) })
  await flushFrame()
  expect(result.current.events.map(e => e.id)).toEqual([3, 2, 1])
})

test('事件总数超过 200 截断保留最新', async () => {
  getJSONMock.mockResolvedValue([])
  const { result } = renderHook(() => useHistoryStream(''))
  await flushFrame()
  const es = FakeEventSource.instances[0]
  act(() => { for (let i = 1; i <= 150; i++) es.emit(makeEvent(i)) })
  await flushFrame()
  expect(result.current.events).toHaveLength(150)
  act(() => { for (let i = 151; i <= 250; i++) es.emit(makeEvent(i)) })
  await flushFrame()
  const ids = result.current.events.map(e => e.id)
  expect(ids).toHaveLength(200)
  expect(ids[0]).toBe(250)
  expect(ids[199]).toBe(51)
})

test('pending 超过 200 丢弃最旧待渲染', async () => {
  getJSONMock.mockResolvedValue([])
  const { result } = renderHook(() => useHistoryStream(''))
  await flushFrame()
  const es = FakeEventSource.instances[0]
  act(() => { for (let i = 1; i <= 205; i++) es.emit(makeEvent(i)) })
  await flushFrame()
  const ids = result.current.events.map(e => e.id)
  expect(ids).toHaveLength(200)
  expect(ids[0]).toBe(205)
  expect(ids[199]).toBe(6)
  expect(ids).not.toContain(5)
})

test('卸载时 close 且不再更新', async () => {
  getJSONMock.mockResolvedValue([makeEvent(1)])
  let renders = 0
  const { result, unmount } = renderHook(() => { renders++; return useHistoryStream('') })
  await flushFrame()
  const es = FakeEventSource.instances[0]
  expect(result.current.events.map(e => e.id)).toEqual([1])
  unmount()
  expect(es.closed).toBe(true)
  const afterUnmount = renders
  act(() => { es.emit(makeEvent(2)) })
  await flushFrame()
  expect(renders).toBe(afterUnmount)
})
