import { useCallback, useState } from 'react'

export type ThemeMode = 'dark' | 'light'

const STORAGE_KEY = 'smartstrm.theme'

function readInitial(): ThemeMode {
  return localStorage.getItem(STORAGE_KEY) === 'light' ? 'light' : 'dark'
}

export function useTheme(): { mode: ThemeMode; toggle: () => void } {
  const [mode, setMode] = useState<ThemeMode>(readInitial)
  const toggle = useCallback(() => {
    setMode(current => {
      const next = current === 'dark' ? 'light' : 'dark'
      localStorage.setItem(STORAGE_KEY, next)
      return next
    })
  }, [])
  return { mode, toggle }
}
