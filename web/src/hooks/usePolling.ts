import { useEffect, useRef } from 'react'

// usePolling runs fn immediately and then on a non-reentrant setTimeout
// chain. Polling pauses while the document is hidden and refreshes
// immediately when the page becomes visible again.
export function usePolling(fn: () => Promise<void> | void, intervalMs: number, active = true): void {
  const saved = useRef(fn)
  saved.current = fn

  useEffect(() => {
    if (!active) return
    let timer = 0
    let running = false
    let stopped = false

    const schedule = () => {
      if (!stopped && !document.hidden) timer = window.setTimeout(cycle, intervalMs)
    }
    const cycle = async () => {
      if (running || stopped || document.hidden) return
      running = true
      try {
        await saved.current()
      } finally {
        running = false
      }
      schedule()
    }
    const onVisibility = () => {
      if (document.hidden) window.clearTimeout(timer)
      else void cycle()
    }

    document.addEventListener('visibilitychange', onVisibility)
    void cycle()
    return () => {
      stopped = true
      window.clearTimeout(timer)
      document.removeEventListener('visibilitychange', onVisibility)
    }
  }, [intervalMs, active])
}
