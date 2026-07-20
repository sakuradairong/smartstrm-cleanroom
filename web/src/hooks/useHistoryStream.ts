import { useEffect, useReducer, useRef } from 'react'
import { getJSON } from '../api/client'
import type { HistoryEvent } from '../api/types'

const MAX_EVENTS = 200

export function useHistoryStream(taskID: string): { events: HistoryEvent[]; connected: boolean } {
  const eventsRef = useRef<HistoryEvent[]>([])
  const connectedRef = useRef(false)
  const [, bump] = useReducer((x: number) => x + 1, 0)

  useEffect(() => {
    let cancelled = false
    let source: EventSource | undefined
    let frame = 0
    const pending: HistoryEvent[] = []
    const pendingIDs = new Set<number>()

    const flush = () => {
      frame = 0
      if (!pending.length || cancelled) return
      const known = new Set(eventsRef.current.map(e => e.id))
      const fresh: HistoryEvent[] = []
      for (const item of pending) {
        if (!known.has(item.id)) {
          fresh.push(item)
          known.add(item.id)
        }
      }
      pending.length = 0
      pendingIDs.clear()
      eventsRef.current = [...fresh.reverse(), ...eventsRef.current].slice(0, MAX_EVENTS)
      bump()
    }
    const queue = (item: HistoryEvent) => {
      if (pendingIDs.has(item.id) || eventsRef.current.some(e => e.id === item.id)) return
      if (pending.length === MAX_EVENTS) {
        const dropped = pending.shift()
        if (dropped) pendingIDs.delete(dropped.id)
      }
      pending.push(item)
      pendingIDs.add(item.id)
      if (!frame) frame = requestAnimationFrame(flush)
    }

    const load = async () => {
      const query = new URLSearchParams({ limit: '200' })
      if (taskID) query.set('task_id', taskID)
      try {
        const initial = await getJSON<HistoryEvent[]>(`/api/history?${query}`)
        if (cancelled) return
        eventsRef.current = (initial ?? []).slice(0, MAX_EVENTS)
        bump()
        const streamQuery = taskID ? `?task_id=${encodeURIComponent(taskID)}` : ''
        source = new EventSource(`/api/history/stream${streamQuery}`)
        source.addEventListener('history', e => queue(JSON.parse((e as MessageEvent).data)))
        source.onopen = () => { connectedRef.current = true; bump() }
        source.onerror = () => { connectedRef.current = false; bump() }
      } catch {
        if (!cancelled) { connectedRef.current = false; bump() }
      }
    }
    void load()

    return () => {
      cancelled = true
      if (frame) cancelAnimationFrame(frame)
      source?.close()
      eventsRef.current = []
      connectedRef.current = false
    }
  }, [taskID])

  return { events: eventsRef.current, connected: connectedRef.current }
}
