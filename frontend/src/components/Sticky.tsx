import { useCallback, useEffect, useMemo, useRef, useState, type MouseEvent } from 'react'
import { DndContext, PointerSensor, closestCenter, useSensor, useSensors, type DragEndEvent } from '@dnd-kit/core'
import { SortableContext, arrayMove, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { api, on } from '../api'
import type { Group, Item, Settings, WindowState, WinMode } from '../types'
import { t } from '../i18n/zh-CN'
import { useDataVersion, useDebounced, useSnack } from '../hooks'
import { Icon } from './Icon'
import { ItemRow, type SubApi } from './ItemRow'
import { Menu, type MenuEntry } from './Menu'
import { ItemDialog } from './ItemDialog'
import { ConfirmDialog, PromptDialog } from './Dialog'
import type { QuickPreview } from '../types'
import { formatDue } from '../format'

type View =
  | { kind: 'group'; id: string }
  | { kind: 'smart'; name: 'today' | 'overdue' | 'next7' | 'nodate' | 'done' }
  | { kind: 'tag'; tag: string }
  | { kind: 'search' }
  | { kind: 'trash' }

const SMART_LABEL: Record<string, string> = {
  today: t('viewToday'), overdue: t('viewOverdue'), next7: t('viewNext7'), nodate: t('viewNoDate'), done: t('viewDone'),
}
const COLORS = ['#FFF3B0', '#D9F2C9', '#CFE8FF', '#FFD9E4', '#E4D9FF', '#E8E8E8']
const LINGER_MS = 3000

interface Props {
  groups: Group[]
  settings: Settings
  win: WindowState | null
  setWin: (w: WindowState) => void
  reloadGroups: () => void
  onOpenSettings: () => void
}

export function Sticky({ groups, settings, win, setWin, reloadGroups, onOpenSettings }: Props) {
  const [view, setView] = useState<View>({ kind: 'group', id: '' })
  const [todo, setTodo] = useState<Item[]>([])
  const [done, setDone] = useState<Item[]>([])
  const [flat, setFlat] = useState<Item[]>([]) // 智能视图 / 搜索 / 回收站 / 标签 的结果
  const [overdueCount, setOverdueCount] = useState(0)
  const [showDone, setShowDone] = useState(false)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [expanded, setExpanded] = useState<Set<string>>(new Set()) // 展开了子任务面板的事项
  const [focusSubId, setFocusSubId] = useState<string | null>(null)
  const [dialogItem, setDialogItem] = useState<Item | null>(null)
  const [menu, setMenu] = useState<{ x: number; y: number; entries: MenuEntry[] } | null>(null)
  const [prompt, setPrompt] = useState<{ title: string; label?: string; initial?: string; onOk: (v: string) => void } | null>(null)
  const [confirm, setConfirm] = useState<{ title: string; message: string; onOk: () => void } | null>(null)
  const [query, setQuery] = useState('')
  const [draft, setDraft] = useState('')
  const [preview, setPreview] = useState<QuickPreview | null>(null)
  const [hovered, setHovered] = useState(false)
  const [ctrlLive, setCtrlLive] = useState(false) // 鼠标穿透开启期间按住 Ctrl：临时可操作
  const [lingering, setLingering] = useState<Set<string>>(new Set())
  const [error, setError] = useState('')
  const { snack, show, hide } = useSnack()
  const version = useDataVersion()
  const lingerRef = useRef(new Map<string, number>())
  const pendingRefresh = useRef(false)
  const opacityQueue = useRef<Promise<unknown>>(Promise.resolve()) // 透明度滑块连续触发：请求必须按顺序发出，避免旧值后到覆盖新值
  const addRef = useRef<HTMLInputElement>(null)
  const searchRef = useRef<HTMLInputElement>(null)
  const dq = useDebounced(query, 120)
  const dDraft = useDebounced(draft, 100)

  // 当前分组：首次从后端读取（便签当前显示的分组）
  useEffect(() => {
    api.currentGroup().then((id) => setView((v) => (v.kind === 'group' && !v.id ? { kind: 'group', id } : v)))
  }, [])

  const groupId = view.kind === 'group' ? view.id : win?.groupId ?? ''
  const group = groups.find((g) => g.id === groupId)
  const collapsed = !!win?.collapsed

  // ---- 数据加载 ----
  const load = useCallback(async () => {
    if (lingerRef.current.size > 0) { pendingRefresh.current = true; return } // 勾选过渡期间先不刷新
    try {
      setError('')
      switch (view.kind) {
        case 'group': {
          if (!view.id) return
          const gv = await api.groupContent(view.id)
          setTodo(gv.todo); setDone(gv.done); setOverdueCount(gv.overdue)
          break
        }
        case 'smart': setFlat(await api.smartView(view.name)); break
        case 'tag': setFlat(await api.byTag(view.tag)); break
        case 'trash': setFlat(await api.trash()); break
        case 'search': setFlat(dq.trim() ? await api.search(dq) : []); break
      }
    } catch (e) {
      setError((e as Error).message)
    }
  }, [view, dq])

  useEffect(() => { void load() }, [load, version])
  useEffect(() => on('window:interactive', (v) => setCtrlLive(!!v)), [])
  useEffect(() => { if (!win?.clickThrough) setCtrlLive(false) }, [win?.clickThrough])

  // ---- 操作 ----
  const toggle = useCallback(async (item: Item) => {
    const willDone = item.status !== 'done' && !lingerRef.current.has(item.id)
    try {
      if (willDone) {
        // 显示删除线，3 秒后移入「已完成」区
        const timer = window.setTimeout(() => {
          lingerRef.current.delete(item.id)
          setLingering(new Set(lingerRef.current.keys()))
          if (pendingRefresh.current && lingerRef.current.size === 0) { pendingRefresh.current = false; void load() }
        }, LINGER_MS)
        lingerRef.current.set(item.id, timer)
        setLingering(new Set(lingerRef.current.keys()))
        await api.toggle(item.id, true)
        show(t('doneToast') + '：' + item.title, () => void undo())
      } else {
        const tm = lingerRef.current.get(item.id)
        if (tm) { window.clearTimeout(tm); lingerRef.current.delete(item.id); setLingering(new Set(lingerRef.current.keys())) }
        await api.toggle(item.id, false)
        pendingRefresh.current = false
        void load()
      }
    } catch (e) { setError((e as Error).message) }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [load, show])

  const undo = useCallback(async () => {
    try {
      // 撤销时清掉过渡状态，让界面立刻回到真实数据
      lingerRef.current.forEach((tm) => window.clearTimeout(tm))
      lingerRef.current.clear(); setLingering(new Set()); pendingRefresh.current = false
      await api.undo()
      hide()
      void load()
    } catch (e) { setError((e as Error).message) }
  }, [hide, load])

  const remove = useCallback(async (item: Item) => {
    await api.remove(item.id)
    show(t('deletedToast'), () => void undo())
  }, [show, undo])

  const commitEdit = async (item: Item, title: string) => {
    setEditingId(null)
    if (title.trim() && title.trim() !== item.title) {
      try { await api.updateItem(item.id, { title: title.trim() }) } catch (e) { setError((e as Error).message) }
    }
  }

  const addItem = async () => {
    const text = draft.trim()
    if (!text) return
    setDraft(''); setPreview(null)
    try {
      await api.quickCreate(text, groupId)
      if (view.kind !== 'group') setView({ kind: 'group', id: groupId })
    } catch (e) { setError((e as Error).message); setDraft(text) }
  }

  useEffect(() => {
    let alive = true
    if (!dDraft.trim()) { setPreview(null); return }
    api.quickParse(dDraft, groupId).then((p) => alive && setPreview(p)).catch(() => alive && setPreview(null))
    return () => { alive = false }
  }, [dDraft, groupId])

  // ---- 窗口级操作 ----
  const setMode = async (m: WinMode) => setWin(await api.setWindowMode(m))
  const changeGroup = async (id: string) => {
    setView({ kind: 'group', id })
    setWin(await api.showGroup(id))
  }

  // ---- 快捷键（FR-502）----
  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      const inField = (e.target as HTMLElement)?.tagName === 'INPUT' || (e.target as HTMLElement)?.tagName === 'TEXTAREA'
      if (e.ctrlKey && e.key.toLowerCase() === 'n') { e.preventDefault(); addRef.current?.focus() }
      else if (e.ctrlKey && e.key.toLowerCase() === 'f') { e.preventDefault(); setView({ kind: 'search' }); setTimeout(() => searchRef.current?.focus(), 0) }
      else if (e.ctrlKey && e.key.toLowerCase() === 'z' && !inField) { e.preventDefault(); void undo() }
      else if (e.key === 'Escape' && view.kind !== 'group') setView({ kind: 'group', id: groupId })
      else if (e.key === 'Delete' && !inField && selectedId) { const it = [...todo, ...done, ...flat].find((i) => i.id === selectedId); if (it && view.kind !== 'trash') { setSelectedId(null); void remove(it) } }
    }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }, [undo, view.kind, groupId, selectedId, todo, done, flat, remove])

  // ---- 菜单 ----
  const groupMenu = (e: MouseEvent) => {
    const r = (e.currentTarget as HTMLElement).getBoundingClientRect()
    const entries: MenuEntry[] = [{ kind: 'header', label: t('groups') }]
    groups.forEach((g) => entries.push({ kind: 'item', label: g.name, checked: view.kind === 'group' && view.id === g.id, onClick: () => void changeGroup(g.id) }))
    entries.push({ kind: 'item', label: t('newGroup'), onClick: () => setPrompt({ title: t('newGroup'), label: t('groupNamePrompt'), onOk: async (name) => { setPrompt(null); const g = await api.createGroup(name, ''); reloadGroups(); await changeGroup(g.id) } }) })
    entries.push({ kind: 'sep' }, { kind: 'header', label: t('views') })
    ;(['today', 'overdue', 'next7', 'nodate', 'done'] as const).forEach((n) =>
      entries.push({ kind: 'item', label: SMART_LABEL[n], checked: view.kind === 'smart' && view.name === n, onClick: () => setView({ kind: 'smart', name: n }) }))
    entries.push({ kind: 'sep' })
    entries.push({ kind: 'item', label: t('trash'), onClick: () => setView({ kind: 'trash' }) })
    setMenu({ x: r.left, y: r.bottom + 2, entries })
    void api.bootstrap().then((b) => {
      if (b.tags.length === 0) return
      setMenu((m) => m && { ...m, entries: [...m.entries, { kind: 'sep' }, { kind: 'header', label: t('tags') }, ...b.tags.map((tag): MenuEntry => ({ kind: 'item', label: '#' + tag, onClick: () => setView({ kind: 'tag', tag }) }))] })
    })
  }

  const modeMenu = (e: MouseEvent) => {
    const r = (e.currentTarget as HTMLElement).getBoundingClientRect()
    const cur = win?.mode ?? 'desktop'
    setMenu({ x: r.left - 60, y: r.bottom + 2, entries: [
      { kind: 'item', label: t('modeDesktop'), checked: cur === 'desktop', onClick: () => void setMode('desktop') },
      { kind: 'item', label: t('modeTop'), checked: cur === 'top', onClick: () => void setMode('top') },
      { kind: 'item', label: t('modeNormal'), checked: cur === 'normal', onClick: () => void setMode('normal') },
    ] })
  }

  const mainMenu = (e: MouseEvent) => {
    const r = (e.currentTarget as HTMLElement).getBoundingClientRect()
    const opacity = win?.opacity || settings.opacity // 0 / 空 = 跟随设置里的默认值
    const curColor = win?.color || settings.stickyColor
    const entries: MenuEntry[] = [
      { kind: 'item', label: t('search') + '  Ctrl+F', onClick: () => { setView({ kind: 'search' }); setTimeout(() => searchRef.current?.focus(), 0) } },
      { kind: 'item', label: t('trash'), onClick: () => setView({ kind: 'trash' }) },
      { kind: 'sep' },
      { kind: 'item', label: win?.locked ? t('unlockPosition') : t('lockPosition'), onClick: async () => setWin(await api.setWindowLocked(!win?.locked)) },
      { kind: 'item', label: t('collapse'), onClick: async () => setWin(await api.toggleCollapse()) },
      { kind: 'item', label: t('clickThrough'), checked: !!win?.clickThrough, onClick: async () => {
        const w = await api.setClickThrough(!win?.clickThrough)
        setWin(w)
        if (w.clickThrough) show(t('clickThroughOn'), undefined, 9000) // 开启后窗口不再响应鼠标：告知如何恢复
      } },
      { kind: 'custom', node: (
        <div style={{ padding: '4px 10px' }}>
          <div className="lbl">{t('opacity')} {Math.round(opacity * 100)}%</div>
          <input type="range" min={30} max={100} defaultValue={Math.round(opacity * 100)} aria-label={t('opacity')}
            onChange={(ev) => {
              const v = Number(ev.target.value) / 100
              opacityQueue.current = opacityQueue.current.then(() => api.setWindowOpacity(v)).then((w) => setWin(w)).catch(() => undefined)
            }} />
        </div>) },
      { kind: 'custom', node: (
        <div className="swatches" role="group" aria-label={t('color')}>
          {COLORS.map((c) => <button key={c} className={`swatch ${curColor.toLowerCase() === c.toLowerCase() ? 'on' : ''}`} style={{ background: c }} aria-label={c}
            onClick={async () => setWin(await api.setWindowColor(c))} />)}
        </div>) },
      { kind: 'item', label: t('resetAppearance'), onClick: async () => setWin(await api.resetWindowAppearance()) },
      { kind: 'sep' },
    ]
    if (group && !group.isDefault) {
      entries.push(
        { kind: 'item', label: '重命名分组…', onClick: () => setPrompt({ title: '重命名分组', initial: group.name, onOk: async (n) => { setPrompt(null); await api.updateGroup(group.id, n, null); reloadGroups() } }) },
        { kind: 'item', label: '删除分组', danger: true, onClick: async () => { const inbox = groups.find((g) => g.isDefault)!; await api.deleteGroup(group.id); reloadGroups(); await changeGroup(inbox.id) } },
        { kind: 'sep' },
      )
    }
    entries.push(
      { kind: 'item', label: t('settings'), onClick: onOpenSettings },
      { kind: 'item', label: t('hide'), onClick: () => void api.toggleVisible() },
      { kind: 'item', label: t('quit'), onClick: () => void api.quit() },
    )
    setMenu({ x: r.right - 200, y: r.bottom + 2, entries })
  }

  const itemMenu = (item: Item, e: MouseEvent) => {
    const others = groups.filter((g) => g.id !== item.groupId)
    const prio = (p: number, label: string): MenuEntry => ({ kind: 'item', label, checked: item.priority === p, onClick: () => void api.updateItem(item.id, { priority: p }) })
    setMenu({ x: e.clientX, y: e.clientY, entries: view.kind === 'trash' ? [
      { kind: 'item', label: t('restore'), onClick: () => void api.restore(item.id) },
    ] : [
      { kind: 'item', label: t('edit'), onClick: () => setEditingId(item.id) },
      { kind: 'item', label: t('setReminder'), onClick: () => setDialogItem(item) },
      { kind: 'item', label: t('addSubtask'), onClick: () => { setExpanded((c) => new Set(c).add(item.id)); setFocusSubId(item.id) } },
      { kind: 'group', label: t('moveToGroup'), children: others.map((g): MenuEntry => ({ kind: 'item', label: g.name, onClick: () => void api.updateItem(item.id, { groupId: g.id }) })) },
      { kind: 'group', label: t('setPriority'), children: [prio(2, t('priorityHigh')), prio(1, t('priorityMid')), prio(0, t('priorityLow'))] },
      { kind: 'sep' },
      { kind: 'item', label: t('delete'), danger: true, onClick: () => void remove(item) },
    ] })
  }

  // ---- 拖拽排序 ----
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 6 } }))
  const onDragEnd = async (e: DragEndEvent) => {
    const { active, over } = e
    if (!over || active.id === over.id) return
    const from = todo.findIndex((i) => i.id === active.id)
    const to = todo.findIndex((i) => i.id === over.id)
    const next = arrayMove(todo, from, to)
    setTodo(next)
    const after = next[to + 1]
    try { await api.reorder(String(active.id), after ? after.id : '') } catch (er) { setError((er as Error).message) }
  }

  // ---- 渲染 ----
  const shown = useMemo(() => {
    const seen = new Set(todo.map((i) => i.id))
    // 刚勾选、尚在过渡的事项仍显示在未完成区
    const extra = done.filter((i) => lingering.has(i.id) && !seen.has(i.id))
    return [...todo, ...extra]
  }, [todo, done, lingering])
  const doneShown = done.filter((i) => !lingering.has(i.id))
  const subApi = (item: Item): SubApi => ({
    expanded: expanded.has(item.id),
    focusAdd: focusSubId === item.id,
    onToggleExpand: () => {
      setExpanded((cur) => { const n = new Set(cur); n.has(item.id) ? n.delete(item.id) : n.add(item.id); return n })
      setFocusSubId(expanded.has(item.id) ? null : item.id) // 展开时聚焦「添加子任务」输入框
    },
    onAdd: async (title) => { await api.addSubtask(item.id, title) },
    onToggle: (sub, done) => void api.toggleSubtask(sub.id, done),
    onRename: (sub, title) => void api.renameSubtask(sub.id, title).catch((e: Error) => setError(e.message)),
    onDelete: (sub) => { void api.deleteSubtask(sub.id); show(t('deleteSubtask'), () => void undo()) },
  })
  const rowProps = (item: Item) => ({
    sub: subApi(item),
    item, editing: editingId === item.id, lingering: lingering.has(item.id), selected: selectedId === item.id, onSelect: (i: Item) => setSelectedId(i.id),
    onToggle: toggle, onStartEdit: (i: Item) => setEditingId(i.id), onCommitEdit: commitEdit,
    onCancelEdit: () => setEditingId(null), onContext: itemMenu,
  })

  const title =
    view.kind === 'group' ? group?.name ?? '' :
    view.kind === 'smart' ? SMART_LABEL[view.name] :
    view.kind === 'tag' ? '#' + view.tag :
    view.kind === 'search' ? t('search') : t('trash')

  const style = {
    // 窗口自己没单独设置颜色 / 透明度（空 / 0）时跟随设置里的默认值
    '--sticky-bg': win?.color || settings.stickyColor,
    // 原生模式下透明度由外壳用窗口级 alpha 处理，CSS 必须保持不透明
    '--win-opacity': win?.nativeOpacity ? '1' : String(win?.opacity || settings.opacity),
  } as React.CSSProperties
  const modeIcon = win?.mode === 'top' ? 'top' : win?.mode === 'normal' ? 'window' : 'pin'
  const native = !!win?.nativeOpacity
  const faded = !native && settings.fadeOnLeave && !hovered // 原生模式下的变淡由外壳处理

  return (
    <div className={`sticky ${faded ? 'faded' : ''}`} style={style} data-testid="sticky"
      onMouseEnter={() => { setHovered(true); if (native && settings.fadeOnLeave) void api.setFaded(false) }}
      onMouseLeave={() => { setHovered(false); if (native && settings.fadeOnLeave) void api.setFaded(true) }}>
      <div className="titlebar" data-testid="titlebar"
        onMouseDown={(e) => { if (e.button === 0 && !(e.target as HTMLElement).closest('button')) void api.beginDrag() }}
        onDoubleClick={async (e) => { if (!(e.target as HTMLElement).closest('button')) setWin(await api.toggleCollapse()) }}>
        <button className="grp" onClick={groupMenu} aria-label="切换分组或视图" data-testid="group-name">
          {title}<Icon name="chevron" />
        </button>
        {view.kind === 'group' && (
          <span className="count" data-testid="count">
            {t('unfinished', { n: shown.length })}{overdueCount > 0 && <> · <b>{overdueCount} {t('overdue')}</b></>}
          </span>
        )}
        <span className="spacer" />
        {win?.clickThrough && (
          <span className={`ctbadge ${ctrlLive ? 'live' : ''}`} data-testid="ct-badge" data-live={ctrlLive}
            title={ctrlLive ? t('clickThroughLiveHint') : t('clickThroughBadgeHint')}>
            {ctrlLive ? t('clickThroughLive') : t('clickThroughBadge')}
          </span>
        )}
        {win?.locked && <span className="iconbtn" title={t('lockPosition')}><Icon name="lock" /></span>}
        <button className="iconbtn on" title={t(win?.mode === 'top' ? 'modeTop' : win?.mode === 'normal' ? 'modeNormal' : 'modeDesktop')} onClick={modeMenu} aria-label="窗口模式" data-testid="mode-btn"><Icon name={modeIcon} /></button>
        <button className="iconbtn" title={t('search')} onClick={() => { setView({ kind: 'search' }); setTimeout(() => searchRef.current?.focus(), 0) }} aria-label={t('search')}><Icon name="search" /></button>
        <button className="iconbtn" title={t('menu')} onClick={mainMenu} aria-label={t('menu')} data-testid="menu-btn"><Icon name="menu" /></button>
      </div>

      {!collapsed && (
        <>
          {error && <div className="banner" role="alert" onClick={() => setError('')}>{error}</div>}
          {view.kind === 'search' && (
            <div style={{ padding: '6px 8px 0' }}>
              <input ref={searchRef} style={{ width: '100%' }} value={query} placeholder={t('searchPlaceholder')} onChange={(e) => setQuery(e.target.value)} aria-label={t('search')} data-testid="search-input" />
            </div>
          )}
          {view.kind === 'trash' && (
            <div className="section" style={{ cursor: 'default' }}>
              <span>{t('trashHint')}</span><span style={{ flex: 1 }} />
              {flat.length > 0 && <button className="btn" onClick={() => setConfirm({ title: t('emptyTrash'), message: t('emptyTrashConfirm'), onOk: async () => { setConfirm(null); await api.emptyTrash() } })}>{t('emptyTrash')}</button>}
            </div>
          )}
          <div className="body">
            {view.kind === 'group' ? (
              <>
                {shown.length === 0 && doneShown.length === 0 && <div className="empty">{t('emptyList')}</div>}
                {shown.length > 0 && (
                  <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={onDragEnd}>
                    <SortableContext items={shown.map((i) => i.id)} strategy={verticalListSortingStrategy}>
                      <ul className="list" data-testid="todo-list">
                        {shown.map((i) => <ItemRow key={i.id} {...rowProps(i)} sortable />)}
                      </ul>
                    </SortableContext>
                  </DndContext>
                )}
                {doneShown.length > 0 && (
                  <>
                    <div className="section" onClick={() => setShowDone(!showDone)} data-testid="done-toggle">
                      <Icon name={showDone ? 'chevron' : 'chevronRight'} /> {t('completed')} ({doneShown.length})
                    </div>
                    {showDone && <ul className="list" data-testid="done-list">{doneShown.map((i) => <ItemRow key={i.id} {...rowProps(i)} />)}</ul>}
                  </>
                )}
              </>
            ) : flat.length === 0 ? <div className="empty">{view.kind === 'search' && !query ? '' : t('emptyView')}</div> : (
              <ul className="list" data-testid="flat-list">
                {flat.map((i) => (
                  view.kind === 'trash' ? (
                    <li key={i.id} className="row" data-testid="item-row" onContextMenu={(e) => { e.preventDefault(); itemMenu(i, e) }}>
                      <span className="bar" />
                      <div className="main"><div className="title">{i.title}</div>
                        <div className="meta"><span>{i.groupName}</span>{i.dueAt != null && <span>{formatDue(i.dueAt)}</span>}</div></div>
                      <button className="btn" onClick={() => void api.restore(i.id)}>{t('restore')}</button>
                    </li>
                  ) : <ItemRow key={i.id} {...rowProps(i)} query={view.kind === 'search' ? dq : undefined} showGroup />
                ))}
              </ul>
            )}
          </div>

          {snack && (
            <div className="snack" role="status" data-testid="snack">
              <span>{snack.text}</span>
              {snack.undo && <button onClick={() => { snack.undo?.() }}>{t('undo')}</button>}
            </div>
          )}

          {view.kind !== 'trash' && view.kind !== 'search' && (
            <div className="addbar">
              {preview && draft.trim() && (preview.dueAt != null || preview.tags.length > 0 || preview.repeatDesc || preview.groupId !== groupId) && (
                <div className="preview" data-testid="preview">
                  {preview.dueAt != null && <span className="chip">{formatDue(preview.dueAt, new Date(), true)}{preview.hasTime ? '' : '（默认时刻）'}</span>}
                  {preview.repeatDesc && <span className="chip">⟳ {preview.repeatDesc}</span>}
                  {preview.groupId !== groupId && <span className="chip">→ {preview.groupName}</span>}
                  {preview.tags.map((tg) => <span key={tg} className="chip">#{tg}</span>)}
                </div>
              )}
              <input ref={addRef} value={draft} placeholder={t('addPlaceholder')} maxLength={260}
                onChange={(e) => setDraft(e.target.value)}
                onKeyDown={(e) => { if (e.key === 'Enter' && !e.nativeEvent.isComposing) void addItem() }}
                aria-label="添加事项" data-testid="add-input" />
            </div>
          )}
          {!win?.locked && <div className="grip" onMouseDown={(e) => { e.preventDefault(); void api.beginResize() }} aria-hidden="true" />}
        </>
      )}

      {menu && <Menu x={menu.x} y={menu.y} entries={menu.entries} onClose={() => setMenu(null)} />}
      {dialogItem && <ItemDialog item={dialogItem} groups={groups} onClose={() => setDialogItem(null)} onSaved={() => { setDialogItem(null); void load() }} />}
      {prompt && <PromptDialog title={prompt.title} label={prompt.label} initial={prompt.initial} onOk={prompt.onOk} onCancel={() => setPrompt(null)} />}
      {confirm && <ConfirmDialog title={confirm.title} message={confirm.message} onOk={confirm.onOk} onCancel={() => setConfirm(null)} />}
    </div>
  )
}
