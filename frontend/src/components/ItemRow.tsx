import { useEffect, useRef, useState, type MouseEvent } from 'react'
import { useSortable } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import type { Item, Subtask } from '../types'
import { formatDue, formatFull, highlight } from '../format'
import { Icon } from './Icon'
import { t } from '../i18n/zh-CN'

/** 子任务（FR-108）相关的回调与展开状态，由便签窗口统一管理。 */
export interface SubApi {
  expanded: boolean
  focusAdd: boolean
  onToggleExpand: () => void
  onAdd: (title: string) => Promise<void>
  onToggle: (sub: Subtask, done: boolean) => void
  onRename: (sub: Subtask, title: string) => void
  onDelete: (sub: Subtask) => void
}

interface Props {
  item: Item
  sub?: SubApi
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

function SubRow({ sub, api }: { sub: Subtask; api: SubApi }) {
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState(sub.title)
  const ref = useRef<HTMLInputElement>(null)
  useEffect(() => { if (editing) { setDraft(sub.title); ref.current?.focus(); ref.current?.select() } }, [editing, sub.title])
  return (
    <li className={`subrow ${sub.done ? 'done' : ''}`} data-testid="subtask-row">
      <button className="check sm" role="checkbox" aria-checked={sub.done} aria-label={sub.done ? '取消完成子任务' : '完成子任务'}
        onClick={() => api.onToggle(sub, !sub.done)}>
        <Icon name="check" />
      </button>
      {editing ? (
        <input ref={ref} className="edit" value={draft} maxLength={200} aria-label="子任务标题"
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter') { setEditing(false); if (draft.trim() && draft.trim() !== sub.title) api.onRename(sub, draft.trim()) }
            else if (e.key === 'Escape') { e.stopPropagation(); setEditing(false) }
          }}
          onBlur={() => setEditing(false)} />
      ) : (
        <span className="subtitle" onDoubleClick={() => !sub.done && setEditing(true)}>{sub.title}</span>
      )}
      <button className="iconbtn subdel" aria-label={t('deleteSubtask')} title={t('deleteSubtask')} onClick={() => api.onDelete(sub)}><Icon name="x" /></button>
    </li>
  )
}

function SubPanel({ item, api }: { item: Item; api: SubApi }) {
  const [draft, setDraft] = useState('')
  const [err, setErr] = useState('')
  const ref = useRef<HTMLInputElement>(null)
  useEffect(() => { if (api.focusAdd) ref.current?.focus() }, [api.focusAdd])
  const add = async () => {
    const v = draft.trim()
    if (!v) return
    try { setErr(''); await api.onAdd(v); setDraft('') } catch (e) { setErr((e as Error).message) }
  }
  return (
    <div className="subs" data-testid="subtasks" onPointerDown={(e) => e.stopPropagation()} onClick={(e) => e.stopPropagation()}>
      {item.subtasks.length > 0 && (
        <ul className="sublist">{item.subtasks.map((s) => <SubRow key={s.id} sub={s} api={api} />)}</ul>
      )}
      <input ref={ref} className="subadd" value={draft} maxLength={200} placeholder={t('subtaskPlaceholder')} aria-label={t('addSubtask')}
        data-testid="subtask-add"
        onChange={(e) => setDraft(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === 'Enter' && !e.nativeEvent.isComposing) void add()
          else if (e.key === 'Escape' && item.subtasks.length === 0) { e.stopPropagation(); api.onToggleExpand() } // 空面板没有进度标签可点，用 Esc 收起
        }} />
      {err && <div className="err" role="alert">{err}</div>}
    </div>
  )
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
        {(item.dueAt != null || item.tags.length > 0 || props.showGroup || item.repeatText || item.subTotal > 0) && !editing && (
          <div className="meta">
            {item.subTotal > 0 && (
              <button className={`subchip ${item.subDone === item.subTotal ? 'all' : ''}`} data-testid="subchip"
                aria-expanded={!!props.sub?.expanded} aria-label={t('subtaskProgress', { done: item.subDone, total: item.subTotal })}
                title={t('subtasks')}
                onPointerDown={(e) => e.stopPropagation()}
                onClick={(e) => { e.stopPropagation(); props.sub?.onToggleExpand() }}>
                ☑ {item.subDone}/{item.subTotal}
              </button>
            )}
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
        {props.sub?.expanded && !editing && <SubPanel item={item} api={props.sub} />}
      </div>
      {item.subTotal === 0 && !editing && props.sub && !props.sub.expanded && (
        <button className="iconbtn rowadd" aria-label={t('addSubtask')} title={t('addSubtask')}
          onPointerDown={(e) => e.stopPropagation()}
          onClick={(e) => { e.stopPropagation(); props.sub?.onToggleExpand() }}><Icon name="plus" /></button>
      )}
      {item.hasAlarm && !editing && (
        <span className="side" title="已设置提醒">
          <Icon name={item.repeatText ? 'repeat' : 'bell'} />
        </span>
      )}
    </li>
  )
}
