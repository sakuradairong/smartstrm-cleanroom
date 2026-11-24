import { renderHook, act } from '@testing-library/react'
import { vi } from 'vitest'
import { useTheme } from './theme'

function systemTheme(dark: boolean) {
  const media = new EventTarget()
  const query = Object.assign(media, {
    matches: dark,
    media: '(prefers-color-scheme: dark)',
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
  }) satisfies MediaQueryList
  vi.stubGlobal('matchMedia', vi.fn(() => query))
  return (matches: boolean) => {
    query.matches = matches
    const event = Object.assign(new Event('change'), { matches })
    act(() => { query.dispatchEvent(event) })
  }
}

beforeEach(() => {
  localStorage.clear()
})

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

test.each([null, 'rainbow', 'system'])('缺失或非法偏好 %s 跟随系统深色', stored => {
  systemTheme(true)
  if (stored !== null) localStorage.setItem('smartstrm.theme', stored)
  const { result } = renderHook(() => useTheme())
  expect(result.current.preference).toBe('system')
  expect(result.current.mode).toBe('dark')
})

test.each(['light', 'dark'] as const)('读取显式 %s 偏好，不受系统变化影响', preference => {
  const changeSystem = systemTheme(preference === 'light')
  localStorage.setItem('smartstrm.theme', preference)
  const { result } = renderHook(() => useTheme())
  changeSystem(preference === 'dark')
  expect(result.current.preference).toBe(preference)
  expect(result.current.mode).toBe(preference)
})

test('系统偏好跟随变化，离开后停止监听，重新选择时读取当前系统', () => {
  const changeSystem = systemTheme(false)
  const { result, unmount } = renderHook(() => useTheme())
  expect(result.current.mode).toBe('light')
  changeSystem(true)
  expect(result.current.mode).toBe('dark')
  act(() => result.current.setPreference('light'))
  changeSystem(false)
  changeSystem(true)
  expect(result.current.mode).toBe('light')
  act(() => result.current.setPreference('system'))
  expect(result.current.mode).toBe('dark')
  changeSystem(false)
  expect(result.current.mode).toBe('light')
  unmount()
  changeSystem(true)
})

test.each(['light', 'dark', 'system'] as const)('持久化选择的 %s 偏好', preference => {
  systemTheme(false)
  const { result } = renderHook(() => useTheme())
  act(() => result.current.setPreference(preference))
  expect(result.current.preference).toBe(preference)
  expect(localStorage.getItem('smartstrm.theme')).toBe(preference)
})

test('matchMedia 不可用时系统偏好使用浅色，仍可选择深色', () => {
  vi.stubGlobal('matchMedia', undefined)
  const { result } = renderHook(() => useTheme())
  expect(result.current.mode).toBe('light')
  act(() => result.current.setPreference('dark'))
  expect(result.current.mode).toBe('dark')
})

test('localStorage 不可用时仍可选择主题', () => {
  vi.stubGlobal('localStorage', undefined)
  const { result } = renderHook(() => useTheme())
  expect(result.current.preference).toBe('system')
  act(() => result.current.setPreference('dark'))
  expect(result.current.mode).toBe('dark')
})

test('存储访问被拒绝时仍可初始化和切换主题', () => {
  systemTheme(true)
  vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
    throw new DOMException('Storage blocked', 'SecurityError')
  })
  vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
    throw new DOMException('Storage blocked', 'SecurityError')
  })
  const { result } = renderHook(() => useTheme())
  expect(result.current.preference).toBe('system')
  expect(result.current.mode).toBe('dark')
  act(() => result.current.setPreference('light'))
  expect(result.current.mode).toBe('light')
})
