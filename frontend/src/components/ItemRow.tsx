import { useEffect, useRef, useState, type MouseEvent } from 'react'
import { useSortable } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import type { Item } from '../types'
import { formatDue, formatFull, highlight } from '../format'
import { Icon } from './Icon'

interface Props {
  item: Item
  query?: string
  showGroup?: boolean
  editing: boolean
  sortable?: boolean
  /** 本地「刚勾选、3 秒后才移入已完成区」的过渡状态（FR-104） */
  lingering?: boolean
  selected?: boolean
  onSelect?: (item: Item) => void
  onToggle: (item: Item) => void
  onStartEdit: (item: Item) => void
  onCommitEdit: (item: Item, title: string) => void
  onCancelEdit: () => void
  onContext: (item: Item, e: MouseEvent) => void
}

function Title({ text, query }: { text: string; query?: string }) {
  if (!query) return <>{text}</>
  return <>{highlight(text, query).map((p, i) => (p.hit ? <mark key={i}>{p.s}</mark> : <span key={i}>{p.s}</span>))}</>
}

export function ItemRow(props: Props) {
  const { item, query, editing, lingering } = props
  const sortable = useSortable({ id: item.id, disabled: !props.sortable || editing })
  const style = props.sortable
    ? { transform: CSS.Transform.toString(sortable.transform), transition: sortable.transition, opacity: sortable.isDragging ? 0.6 : 1 }
    : undefined
  const done = item.status === 'done' || !!lingering
  const [draft, setDraft] = useState(item.title)
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (editing) {
      setDraft(item.title)
      inputRef.current?.focus()
      inputRef.current?.select()
    }
  }, [editing, item.title])

  const now = new Date()
  const cls = ['row', props.selected ? 'selected' : '', done ? 'done' : '', item.overdue && !done ? 'overdue' : '', `prio-${item.priority}`].join(' ')

  return (
    <li
      ref={props.sortable ? sortable.setNodeRef : undefined}
      style={style}
      className={cls}
      data-testid="item-row"
      data-id={item.id}
      onClick={() => props.onSelect?.(item)}
      onContextMenu={(e) => { e.preventDefault(); props.onContext(item, e) }}
      {...(props.sortable && !editing ? { ...sortable.attributes, ...sortable.listeners } : {})}
    >
      <span className="bar" />
      <button
        className="check"
        role="checkbox"
        aria-checked={done}
        aria-label={done ? '取消完成' : '标记完成'}
        onPointerDown={(e) => e.stopPropagation()}
        onClick={() => props.onToggle(item)}
      >
        <Icon name="check" />
      </button>
      <div className="main">
        {editing ? (
          <input
            ref={inputRef}
            className="edit"
            value={draft}
            maxLength={200}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') props.onCommitEdit(item, draft)
              else if (e.key === 'Escape') { e.stopPropagation(); props.onCancelEdit() }
            }}
            onBlur={() => props.onCancelEdit()}
            onPointerDown={(e) => e.stopPropagation()}
          />
        ) : (
          <div className="title" onDoubleClick={() => !done && props.onStartEdit(item)}>
            <Title text={item.title} query={query} />
          </div>
        )}
        {(item.dueAt != null || item.tags.length > 0 || props.showGroup || item.repeatText) && !editing && (
          <div className="meta">
            {item.dueAt != null && (
              <span className={`due ${item.overdue && !done ? 'overdue' : ''}`} title={formatFull(item.dueAt)}>
                {formatDue(item.dueAt, now)}
              </span>
            )}
            {item.repeatText && <span>{item.repeatText}</span>}
            {item.tags.map((t) => <span key={t} className="tag">#{t}</span>)}
            {props.showGroup && item.groupName && <span className="grpname">{item.groupName}</span>}
          </div>
        )}
        {query && item.note && !editing && (
          <div className="note"><Title text={item.note} query={query} /></div>
        )}
      </div>
      {item.hasAlarm && !editing && (
        <span className="side" title="已设置提醒">
          <Icon name={item.repeatText ? 'repeat' : 'bell'} />
        </span>
      )}
    </li>
  )
}
