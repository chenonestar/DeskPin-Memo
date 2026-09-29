import { useLayoutEffect, useRef, useState, type ReactNode } from 'react'

export type MenuEntry =
  | { kind: 'item'; label: string; onClick?: () => void; danger?: boolean; checked?: boolean; keepOpen?: boolean; sub?: string }
  | { kind: 'sep' }
  | { kind: 'header'; label: string }
  | { kind: 'group'; label: string; children: MenuEntry[] } // 手风琴式子菜单（便签窗口很窄，不做飞出菜单）
  | { kind: 'custom'; node: ReactNode }

interface Props {
  x: number
  y: number
  entries: MenuEntry[]
  onClose: () => void
}

/** 在指定坐标弹出的菜单；自动避让窗口边缘。 */
export function Menu({ x, y, entries, onClose }: Props) {
  const ref = useRef<HTMLDivElement>(null)
  const [pos, setPos] = useState({ left: x, top: y })
  const [open, setOpen] = useState<string | null>(null)

  useLayoutEffect(() => {
    const el = ref.current
    if (!el) return
    const r = el.getBoundingClientRect()
    setPos({
      left: Math.max(4, Math.min(x, window.innerWidth - r.width - 4)),
      top: Math.max(4, Math.min(y, window.innerHeight - r.height - 4)),
    })
  }, [x, y, open, entries.length])

  const render = (e: MenuEntry, i: number, depth = 0): ReactNode => {
    switch (e.kind) {
      case 'sep':
        return <div key={i} className="sep" />
      case 'header':
        return <div key={i} className="mh">{e.label}</div>
      case 'custom':
        return <div key={i}>{e.node}</div>
      case 'group':
        return (
          <div key={i}>
            <button className="mi" onClick={() => setOpen(open === e.label ? null : e.label)}>
              <span className="ck" />
              {e.label}
              <span className="sub">{open === e.label ? '▾' : '▸'}</span>
            </button>
            {open === e.label && <div className="indent">{e.children.map((c, j) => render(c, j, depth + 1))}</div>}
          </div>
        )
      default:
        return (
          <button
            key={i}
            className={`mi ${e.danger ? 'danger' : ''}`}
            onClick={() => {
              e.onClick?.()
              if (!e.keepOpen) onClose()
            }}
          >
            <span className="ck">{e.checked ? '✓' : ''}</span>
            {e.label}
            {e.sub && <span className="sub">{e.sub}</span>}
          </button>
        )
    }
  }

  return (
    <>
      <div className="menu-backdrop" onMouseDown={onClose} onContextMenu={(e) => { e.preventDefault(); onClose() }} />
      <div ref={ref} className="menu" style={pos} role="menu">
        {entries.map((e, i) => render(e, i))}
      </div>
    </>
  )
}
