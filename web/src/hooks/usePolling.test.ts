import { renderHook, act } from '@testing-library/react'
import { usePolling } from './usePolling'

function setHidden(value: boolean) {
  Object.defineProperty(document, 'hidden', { configurable: true, get: () => value })
}

beforeEach(() => { vi.useFakeTimers(); setHidden(false) })
afterEach(() => { vi.useRealTimers() })

test('立即执行一次并按间隔轮询', async () => {
  const fn = vi.fn(async () => {})
  renderHook(() => usePolling(fn, 3000))
  await act(async () => { await vi.advanceTimersByTimeAsync(0) })
  expect(fn).toHaveBeenCalledTimes(1)
  await act(async () => { await vi.advanceTimersByTimeAsync(3000) })
  expect(fn).toHaveBeenCalledTimes(2)
})

test('上一次未完成时不重入', async () => {
  let resolve: () => void = () => {}
  const fn = vi.fn(() => new Promise<void>(r => { resolve = r }))
  renderHook(() => usePolling(fn, 3000))
  await act(async () => { await vi.advanceTimersByTimeAsync(0) })
  await act(async () => { await vi.advanceTimersByTimeAsync(9000) })
  expect(fn).toHaveBeenCalledTimes(1)
  await act(async () => { resolve(); await vi.advanceTimersByTimeAsync(0) })
  await act(async () => { await vi.advanceTimersByTimeAsync(3000) })
  expect(fn).toHaveBeenCalledTimes(2)
})

test('页面隐藏时暂停，恢复可见立即刷新', async () => {
  const fn = vi.fn(async () => {})
  renderHook(() => usePolling(fn, 3000))
  await act(async () => { await vi.advanceTimersByTimeAsync(0) })
  setHidden(true)
  act(() => { document.dispatchEvent(new Event('visibilitychange')) })
  await act(async () => { await vi.advanceTimersByTimeAsync(9000) })
  expect(fn).toHaveBeenCalledTimes(1)
  setHidden(false)
  await act(async () => { document.dispatchEvent(new Event('visibilitychange')); await vi.advanceTimersByTimeAsync(0) })
  expect(fn).toHaveBeenCalledTimes(2)
})

test('卸载后不再触发', async () => {
  const fn = vi.fn(async () => {})
  const { unmount } = renderHook(() => usePolling(fn, 3000))
  await act(async () => { await vi.advanceTimersByTimeAsync(0) })
  unmount()
  await act(async () => { await vi.advanceTimersByTimeAsync(9000) })
  expect(fn).toHaveBeenCalledTimes(1)
})
