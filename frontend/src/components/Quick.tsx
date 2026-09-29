import { useEffect, useRef, useState } from 'react'
import { api, on } from '../api'
import type { Group, QuickPreview } from '../types'
import { t } from '../i18n/zh-CN'
import { useDebounced } from '../hooks'
import { formatDue, formatFull } from '../format'

/** 快速输入框（4.2）：Spotlight 风格，实时预览解析结果，Enter 保存、Tab 详细字段、Esc 关闭。 */
export function Quick({ groups, groupId }: { groups: Group[]; groupId: string }) {
  const [text, setText] = useState('')
  const [p, setP] = useState<QuickPreview | null>(null)
  const [detail, setDetail] = useState(false)
  const [note, setNote] = useState('')
  const [gid, setGid] = useState('')
  const [prio, setPrio] = useState<number | ''>('')
  const [err, setErr] = useState('')
  const inputRef = useRef<HTMLInputElement>(null)
  const d = useDebounced(text, 80)

  useEffect(() => { inputRef.current?.focus() }, [])
  useEffect(() => on('overlay:open', () => { setText(''); setDetail(false); setNote(''); setErr(''); inputRef.current?.focus() }), [])
  useEffect(() => {
    let alive = true
    if (!d.trim()) { setP(null); return }
    api.quickParse(d, gid || groupId).then((r) => alive && setP(r)).catch(() => alive && setP(null))
    return () => { alive = false }
  }, [d, gid, groupId])

  const close = () => void api.closeQuick()
  const commit = async () => {
    if (!text.trim()) return
    try {
      const pv = await api.quickParse(text, gid || groupId)
      await api.createItem({
        title: pv.title,
        note,
        groupId: gid || pv.groupId,
        priority: prio === '' ? pv.priority : prio,
        dueAt: pv.dueAt,
        tags: pv.tags,
        reminders: pv.dueAt != null ? [{ remindAt: pv.dueAt, offsetMinutes: 0, repeatRule: pv.repeatRule }] : [],
      })
      setText(''); setNote('')
      close()
    } catch (e) { setErr((e as Error).message) }
  }

  return (
    <div className="quick-wrap" data-testid="quick">
      <div className="quick"
        onKeyDown={(e) => {
          if (e.key === 'Escape') { e.preventDefault(); close() }
          else if (e.key === 'Enter' && !e.nativeEvent.isComposing) { e.preventDefault(); void commit() }
          else if (e.key === 'Tab' && !detail) { e.preventDefault(); setDetail(true); setTimeout(() => (document.querySelector('[data-testid="quick-note"]') as HTMLElement)?.focus(), 0) }
        }}>
        <input ref={inputRef} className="big" value={text} placeholder={t('quickPlaceholder')} onChange={(e) => setText(e.target.value)} aria-label="快速新建" data-testid="quick-input" />
        <div className="preview" data-testid="quick-preview">
          {p && text.trim() ? (
            <>
              <span>{t('parsed')}：</span>
              <span className="chip">{p.title}</span>
              {p.dueAt != null && <span className="chip" title={formatFull(p.dueAt)}>📅 {formatDue(p.dueAt)}{p.hasTime ? '' : '（默认时刻）'}</span>}
              {p.repeatDesc && <span className="chip">⟳ {p.repeatDesc}</span>}
              <span className="chip">📁 {p.groupName}</span>
              {p.tags.map((tg) => <span key={tg} className="chip">#{tg}</span>)}
            </>
          ) : <span>&nbsp;</span>}
        </div>
        {detail && (
          <div className="extra">
            <input placeholder={t('note')} value={note} onChange={(e) => setNote(e.target.value)} data-testid="quick-note" aria-label={t('note')} />
            <select value={gid || groupId} onChange={(e) => setGid(e.target.value)} aria-label={t('quickGroup')}>
              {groups.map((g) => <option key={g.id} value={g.id}>{g.name}</option>)}
            </select>
            <select value={prio} onChange={(e) => setPrio(e.target.value === '' ? '' : Number(e.target.value))} aria-label={t('priority')}>
              <option value="">{t('priority')}</option>
              <option value={2}>{t('priorityHigh')}</option>
              <option value={1}>{t('priorityMid')}</option>
              <option value={0}>{t('priorityLow')}</option>
            </select>
          </div>
        )}
        {err && <div className="err" role="alert">{err}</div>}
        <div className="foot">{t('quickHint')}</div>
      </div>
    </div>
  )
}
