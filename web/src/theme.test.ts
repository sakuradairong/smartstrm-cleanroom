import { renderHook, act } from '@testing-library/react'
import { useTheme } from './theme'

beforeEach(() => localStorage.clear())

test('默认暗色主题', () => {
  const { result } = renderHook(() => useTheme())
  expect(result.current.mode).toBe('dark')
})

test('toggle 切换并持久化', () => {
  const { result } = renderHook(() => useTheme())
  act(() => result.current.toggle())
  expect(result.current.mode).toBe('light')
  expect(localStorage.getItem('smartstrm.theme')).toBe('light')
})

test('读取已存偏好，非法值回退 dark', () => {
  localStorage.setItem('smartstrm.theme', 'light')
  expect(renderHook(() => useTheme()).result.current.mode).toBe('light')
  localStorage.setItem('smartstrm.theme', 'rainbow')
  expect(renderHook(() => useTheme()).result.current.mode).toBe('dark')
})
