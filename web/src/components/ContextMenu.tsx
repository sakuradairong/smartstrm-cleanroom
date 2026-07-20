import { useEffect, useRef, useState } from 'react'
import type { CSSProperties } from 'react'
import { createPortal } from 'react-dom'

export interface ContextMenuItem {
  key: string
  label: string
}

// 视觉样式移植自旧 internal/app/app.go 注入的 .storage-menu CSS。
const MENU_STYLE: CSSProperties = {
  position: 'fixed',
  zIndex: 1000,
  minWidth: 170,
  padding: '0.35rem',
  background: '#202832',
  border: '1px solid #596675',
  borderRadius: 8,
  boxShadow: '0 8px 24px #0008',
}

const ITEM_STYLE: CSSProperties = {
  display: 'block',
  width: '100%',
  textAlign: 'left',
  background: 'transparent',
  border: 'none',
  color: '#f0f0f0',
  padding: '4px 8px',
  cursor: 'pointer',
}

export default function ContextMenu({
  x,
  y,
  items,
  onSelect,
  onClose,
}: {
  x: number
  y: number
  items: ContextMenuItem[]
  onSelect: (key: string) => void
  onClose: () => void
}) {
  const ref = useRef<HTMLDivElement>(null)
  const [pos, setPos] = useState({ left: x, top: y })

  // 挂载后按实际尺寸做视口夹取，与旧 openStorageContextMenu 一致：
  // Math.max(8, Math.min(x, innerWidth - width - 8))
  useEffect(() => {
    const el = ref.current
    if (!el) return
    const rect = el.getBoundingClientRect()
    setPos({
      left: Math.max(8, Math.min(x, window.innerWidth - rect.width - 8)),
      top: Math.max(8, Math.min(y, window.innerHeight - rect.height - 8)),
    })
  }, [x, y])

  // Esc 与外部 mousedown 关闭（旧 UI 用 pointerdown，这里按组件规范用 mousedown）。
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose()
    }
    const onMouseDown = (event: MouseEvent) => {
      if (ref.current && !ref.current.contains(event.target as Node)) onClose()
    }
    document.addEventListener('keydown', onKeyDown)
    document.addEventListener('mousedown', onMouseDown)
    return () => {
      document.removeEventListener('keydown', onKeyDown)
      document.removeEventListener('mousedown', onMouseDown)
    }
  }, [onClose])

  return createPortal(
    <div ref={ref} role="menu" aria-label="文件操作" style={{ ...MENU_STYLE, left: pos.left, top: pos.top }}>
      {items.map((item, index) => (
        <button
          key={item.key}
          type="button"
          role="menuitem"
          autoFocus={index === 0}
          style={ITEM_STYLE}
          onClick={() => onSelect(item.key)}
        >
          {item.label}
        </button>
      ))}
    </div>,
    document.body,
  )
}
