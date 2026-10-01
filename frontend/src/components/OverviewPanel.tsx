import { useCallback, useEffect, useState } from 'react'
import { api } from '../api'
import type { Item, Overview } from '../types'
import { formatDue } from '../format'
import { t } from '../i18n/zh-CN'

/** 每日概览（FR-308）：每天首次启动时展示逾期与今天到期的事项，可直接勾选完成。 */
export function OverviewPanel() {
  const [ov, setOv] = useState<Overview | null>(null)
  const [done, setDone] = useState<Set<string>>(new Set())
  const load = useCallback(() => { void api.overview().then(setOv) }, [])
  useEffect(load, [load])
  useEffect(() => {
    const h = (e: KeyboardEvent) => e.key === 'Escape' && void api.closeOverview()
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }, [])

  const toggle = async (it: Item) => {
    const next = new Set(done)
    const willDone = !next.has(it.id)
    willDone ? next.add(it.id) : next.delete(it.id)
    setDone(next)
    await api.toggle(it.id, willDone)
  }
  const turnOff = async () => {
    const s = await api.getSettings()
    await api.saveSettings({ ...s, dailyOverview: false })
    await api.closeOverview()
  }

  if (!ov) return null
  const pending = [...ov.overdue, ...ov.today].filter((i) => !done.has(i.id)).length
  const Section = ({ title, items, danger }: { title: string; items: Item[]; danger?: boolean }) =>
    items.length === 0 ? null : (
      <>
        <div className="section" style={{ cursor: 'default', color: danger ? 'var(--danger)' : undefined, fontWeight: 600 }}>
          {title}（{items.length}）
        </div>
        <ul className="list" data-testid={danger ? 'overview-overdue' : 'overview-today'}>
          {items.map((it) => (
            <li key={it.id} className={`row ${done.has(it.id) ? 'done' : ''} ${danger ? 'overdue' : ''} prio-${it.priority}`} data-testid="overview-row">
              <span className="bar" />
              <button className="check" role="checkbox" aria-checked={done.has(it.id)} aria-label={done.has(it.id) ? '取消完成' : '标记完成'} onClick={() => void toggle(it)}>
                <svg className="icon" viewBox="0 0 24 24"><path d="M5 12.5l4.5 4.5L19 7.5" /></svg>
              </button>
              <div className="main">
                <div className="title">{it.title}</div>
                <div className="meta">
                  {it.dueAt != null && <span className={`due ${danger ? 'overdue' : ''}`}>{formatDue(it.dueAt)}</span>}
                  {it.groupName && <span className="grpname">{it.groupName}</span>}
                  {it.tags.map((tg) => <span key={tg} className="tag">#{tg}</span>)}
                </div>
              </div>
            </li>
          ))}
        </ul>
      </>
    )

  return (
    <div className="settings" data-testid="overview" role="dialog" aria-label="今日概览">
      <div className="head"
        onMouseDown={(e) => { if (e.button === 0 && !(e.target as HTMLElement).closest('button')) void api.beginDrag() }}>
        {t('overviewTitle')} · {ov.date}
        <span className="spacer" />
      </div>
      <div className="body" style={{ flex: 1 }}>
        <div className="hint" style={{ padding: '8px 12px 0' }} data-testid="overview-summary">
          {ov.overdue.length > 0 && <>已逾期 <b style={{ color: 'var(--danger)' }}>{ov.overdue.length}</b> 项，</>}今天还有 <b>{ov.today.length}</b> 项到期
        </div>
        <Section title={t('overdue')} items={ov.overdue} danger />
        <Section title={t('overviewToday')} items={ov.today} />
      </div>
      <div className="btns" style={{ padding: '8px 12px', margin: 0, justifyContent: 'space-between', borderTop: '1px solid var(--line)' }}>
        <button className="btn" onClick={() => void turnOff()} style={{ fontSize: 12 }} data-testid="overview-off">{t('overviewOff')}</button>
        <button className="btn primary" onClick={() => void api.closeOverview()} data-testid="overview-close">
          {pending === 0 ? t('overviewAllDone') : t('overviewClose')}
        </button>
      </div>
    </div>
  )
}
