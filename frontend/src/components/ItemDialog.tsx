import { useMemo, useState } from 'react'
import { api } from '../api'
import type { Group, Item, ReminderInput } from '../types'
import { fromLocalInput, parseTags, toLocalInput } from '../format'
import { t } from '../i18n/zh-CN'
import { Modal } from './Dialog'

interface Props {
  item: Item
  groups: Group[]
  onClose: () => void
  onSaved: () => void
}

type RemindMode = 'none' | 'atDue' | 'b5' | 'b15' | 'b30' | 'b60' | 'b1440' | 'custom'
type RepeatKind = 'none' | 'daily' | 'workday' | 'weekly' | 'monthly' | 'yearly' | 'everyn' | 'raw'

const OFFSETS: Record<string, number> = { b5: 5, b15: 15, b30: 30, b60: 60, b1440: 1440 }
const WEEKDAYS = ['周日', '周一', '周二', '周三', '周四', '周五', '周六']
const DAYCODE = ['SU', 'MO', 'TU', 'WE', 'TH', 'FR', 'SA']

/** 从 RRULE 反推界面选项（与后端 recur.Build 生成的规则一一对应）。 */
export function parseRule(rule: string): { kind: RepeatKind; arg: number } {
  if (!rule) return { kind: 'none', arg: 0 }
  const m: Record<string, string> = {}
  rule.replace(/^RRULE:/, '').split(';').forEach((p) => { const [k, v] = p.split('='); if (k) m[k] = v })
  switch (m.FREQ) {
    case 'DAILY': return m.INTERVAL && m.INTERVAL !== '1' ? { kind: 'everyn', arg: Number(m.INTERVAL) } : { kind: 'daily', arg: 0 }
    case 'WEEKLY':
      if (m.BYDAY === 'MO,TU,WE,TH,FR') return { kind: 'workday', arg: 0 }
      if (m.BYDAY && DAYCODE.includes(m.BYDAY)) return { kind: 'weekly', arg: DAYCODE.indexOf(m.BYDAY) }
      break
    case 'MONTHLY': if (m.BYMONTHDAY) return { kind: 'monthly', arg: Number(m.BYMONTHDAY) }; break
    case 'YEARLY': return { kind: 'yearly', arg: 0 }
  }
  return { kind: 'raw', arg: 0 }
}

function initialRemind(item: Item): { mode: RemindMode; custom: number | null } {
  const r = item.reminders[0]
  if (!r) return { mode: 'none', custom: null }
  if (r.offsetMinutes > 0) {
    const k = Object.keys(OFFSETS).find((k) => OFFSETS[k] === r.offsetMinutes)
    if (k) return { mode: k as RemindMode, custom: null }
  }
  if (r.remindAt != null && r.remindAt === item.dueAt) return { mode: 'atDue', custom: null }
  if (r.remindAt != null) return { mode: 'custom', custom: r.remindAt }
  return { mode: 'atDue', custom: null }
}

export function ItemDialog({ item, groups, onClose, onSaved }: Props) {
  const init = useMemo(() => {
    const rm = initialRemind(item)
    const rep = parseRule(item.reminders.find((r) => r.repeatRule)?.repeatRule ?? '')
    return { ...rm, rep }
  }, [item])
  const [title, setTitle] = useState(item.title)
  const [note, setNote] = useState(item.note)
  const [due, setDue] = useState(toLocalInput(item.dueAt))
  const [priority, setPriority] = useState(item.priority)
  const [groupId, setGroupId] = useState(item.groupId)
  const [tags, setTags] = useState(item.tags.map((x) => '#' + x).join(' '))
  const [mode, setMode] = useState<RemindMode>(init.mode)
  const [custom, setCustom] = useState(toLocalInput(init.custom))
  const [kind, setKind] = useState<RepeatKind>(init.rep.kind)
  const [arg, setArg] = useState(init.rep.arg || (init.rep.kind === 'monthly' ? 1 : init.rep.kind === 'everyn' ? 2 : 0))
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  const rawRule = item.reminders.find((r) => r.repeatRule)?.repeatRule ?? ''

  const save = async () => {
    setErr('')
    const dueMs = fromLocalInput(due)
    const customMs = fromLocalInput(custom)
    if (!title.trim()) return setErr('标题不能为空')
    if (mode.startsWith('b') && mode !== 'none' && dueMs == null) return setErr('「截止前提醒」需要先设置截止时间')
    if (mode === 'atDue' && dueMs == null) return setErr('「到期时提醒」需要先设置截止时间')
    if (mode === 'custom' && customMs == null) return setErr('请选择提醒时间')
    if (kind !== 'none' && mode === 'none') return setErr('设置重复前请先选择提醒方式')
    setBusy(true)
    try {
      await api.updateItem(item.id, {
        title: title.trim(),
        note,
        groupId,
        priority,
        dueAt: dueMs,
        clearDue: dueMs == null && item.dueAt != null,
        tags: parseTags(tags),
      })
      // 提醒：只有配置变化才重写，避免无谓地推进版本号
      const rule = kind === 'none' ? '' : kind === 'raw' ? rawRule : await api.buildRepeat(kind, arg)
      const next: ReminderInput[] = mode === 'none' ? [] : [{
        offsetMinutes: OFFSETS[mode] ?? 0,
        remindAt: mode === 'custom' ? customMs : mode === 'atDue' ? dueMs : null,
        repeatRule: rule,
      }]
      const before = init.mode === 'none' ? [] : [{ mode: init.mode, custom: init.custom, rule: rawRule }]
      const after = next.length ? [{ mode, custom: customMs, rule }] : []
      const dueChanged = dueMs !== item.dueAt
      if (dueChanged || JSON.stringify(before) !== JSON.stringify(after)) await api.setReminders(item.id, next)
      onSaved()
    } catch (e) {
      setErr((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <Modal
      title={t('edit')}
      onClose={onClose}
      footer={<>
        <button className="btn" onClick={onClose}>{t('cancel')}</button>
        <button className="btn primary" disabled={busy} onClick={save}>{t('save')}</button>
      </>}
    >
      <div className="field">
        <label>{t('title')}</label>
        <input value={title} maxLength={200} autoFocus onChange={(e) => setTitle(e.target.value)} aria-label={t('title')} />
      </div>
      <div className="field">
        <label>{t('note')}</label>
        <textarea rows={2} value={note} onChange={(e) => setNote(e.target.value)} aria-label={t('note')} />
      </div>
      <div className="field">
        <label>{t('due')}</label>
        <div className="inline">
          <input type="datetime-local" value={due} onChange={(e) => setDue(e.target.value)} aria-label={t('due')} />
          {due && <button className="btn" style={{ flex: 'none' }} onClick={() => setDue('')}>{t('dueNone')}</button>}
        </div>
      </div>
      <div className="field">
        <div className="inline">
          <div>
            <label className="lbl">{t('priority')}</label>
            <select value={priority} onChange={(e) => setPriority(Number(e.target.value))} aria-label={t('priority')} style={{ width: '100%' }}>
              <option value={2}>{t('priorityHigh')}</option>
              <option value={1}>{t('priorityMid')}</option>
              <option value={0}>{t('priorityLow')}</option>
            </select>
          </div>
          <div>
            <label className="lbl">{t('group')}</label>
            <select value={groupId} onChange={(e) => setGroupId(e.target.value)} aria-label={t('group')} style={{ width: '100%' }}>
              {groups.map((g) => <option key={g.id} value={g.id}>{g.name}</option>)}
            </select>
          </div>
        </div>
      </div>
      <div className="field">
        <label>{t('tagsHint')}</label>
        <input value={tags} onChange={(e) => setTags(e.target.value)} aria-label={t('tags')} />
      </div>
      <div className="field">
        <label>{t('reminder')}</label>
        <select value={mode} onChange={(e) => setMode(e.target.value as RemindMode)} aria-label={t('reminder')}>
          <option value="none">{t('remindNone')}</option>
          <option value="atDue">{t('remindAtDue')}</option>
          <option value="b5">{t('remindBefore', { n: '5 分钟' })}</option>
          <option value="b15">{t('remindBefore', { n: '15 分钟' })}</option>
          <option value="b30">{t('remindBefore', { n: '30 分钟' })}</option>
          <option value="b60">{t('remindBefore', { n: '1 小时' })}</option>
          <option value="b1440">{t('remindBefore', { n: '1 天' })}</option>
          <option value="custom">指定时间…</option>
        </select>
        {mode === 'custom' && <input type="datetime-local" value={custom} onChange={(e) => setCustom(e.target.value)} aria-label="提醒时间" />}
      </div>
      {mode !== 'none' && (
        <div className="field">
          <label>{t('repeat')}</label>
          <div className="inline">
            <select value={kind} onChange={(e) => setKind(e.target.value as RepeatKind)} aria-label={t('repeat')}>
              <option value="none">{t('repeatNone')}</option>
              <option value="daily">{t('repeatDaily')}</option>
              <option value="workday">{t('repeatWorkday')}</option>
              <option value="weekly">{t('repeatWeekly')}</option>
              <option value="monthly">{t('repeatMonthly')}</option>
              <option value="yearly">{t('repeatYearly')}</option>
              <option value="everyn">{t('repeatEveryN')}</option>
              {rawRule && parseRule(rawRule).kind === 'raw' && <option value="raw">自定义规则</option>}
            </select>
            {kind === 'weekly' && (
              <select value={arg} onChange={(e) => setArg(Number(e.target.value))} aria-label="星期">
                {WEEKDAYS.map((d, i) => <option key={i} value={i}>{d}</option>)}
              </select>
            )}
            {kind === 'monthly' && (
              <select value={arg} onChange={(e) => setArg(Number(e.target.value))} aria-label="日期">
                {Array.from({ length: 31 }, (_, i) => <option key={i + 1} value={i + 1}>{i + 1} 日</option>)}
              </select>
            )}
            {kind === 'everyn' && (
              <input type="number" min={1} max={365} value={arg} onChange={(e) => setArg(Math.max(1, Number(e.target.value) || 1))} aria-label="间隔天数" />
            )}
          </div>
        </div>
      )}
      {err && <div className="err" role="alert">{err}</div>}
    </Modal>
  )
}
