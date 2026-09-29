// 时间与文本的展示辅助函数（纯函数，便于测试）。
const pad = (n: number) => String(n).padStart(2, '0')

export const hhmm = (d: Date) => `${pad(d.getHours())}:${pad(d.getMinutes())}`

const startOfDay = (d: Date) => new Date(d.getFullYear(), d.getMonth(), d.getDate())

/** 相对日期差（按本地日历日）：今天 0，明天 1，昨天 -1。 */
export function dayDiff(due: Date, now: Date): number {
  return Math.round((startOfDay(due).getTime() - startOfDay(now).getTime()) / 86400000)
}

const WEEK = ['日', '一', '二', '三', '四', '五', '六']

/** 截止时间的相对显示：「今天 15:00」「明天 09:00」「2 天后」「昨天」「3 天前」。 */
export function formatDue(ms: number, now: Date = new Date(), hasTime = true): string {
  const d = new Date(ms)
  const diff = dayDiff(d, now)
  const time = hasTime ? ` ${hhmm(d)}` : ''
  if (diff === 0) return `今天${time}`
  if (diff === 1) return `明天${time}`
  if (diff === 2) return `后天${time}`
  if (diff === -1) return `昨天${time}`
  if (diff > 2 && diff <= 7) return `${diff} 天后`
  if (diff < -1 && diff >= -30) return `${-diff} 天前`
  if (d.getFullYear() === now.getFullYear()) return `${d.getMonth() + 1}月${d.getDate()}日${time}`
  return `${d.getFullYear()}年${d.getMonth() + 1}月${d.getDate()}日`
}

/** 完整日期时间（提示文字用）：2026-10-05 周一 09:00。 */
export function formatFull(ms: number): string {
  const d = new Date(ms)
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} 周${WEEK[d.getDay()]} ${hhmm(d)}`
}

/** 转为 <input type="datetime-local"> 的值（本地时间）。 */
export function toLocalInput(ms: number | null | undefined): string {
  if (ms == null) return ''
  const d = new Date(ms)
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${hhmm(d)}`
}

/** 解析 datetime-local 的值为 UTC 毫秒；空串返回 null。 */
export function fromLocalInput(v: string): number | null {
  if (!v) return null
  const t = new Date(v).getTime()
  return Number.isNaN(t) ? null : t
}

/** 把文本中的关键字切成 [文本, 是否命中] 片段，用于搜索高亮。 */
export function highlight(text: string, q: string): { s: string; hit: boolean }[] {
  const needle = q.trim().toLowerCase()
  if (!needle) return [{ s: text, hit: false }]
  const out: { s: string; hit: boolean }[] = []
  const lower = text.toLowerCase()
  let i = 0
  for (;;) {
    const j = lower.indexOf(needle, i)
    if (j < 0) break
    if (j > i) out.push({ s: text.slice(i, j), hit: false })
    out.push({ s: text.slice(j, j + needle.length), hit: true })
    i = j + needle.length
  }
  if (i < text.length) out.push({ s: text.slice(i), hit: false })
  return out.length ? out : [{ s: text, hit: false }]
}

/** 解析输入框里的标签：「#工作 财务」→ ['工作','财务']。 */
export function parseTags(s: string): string[] {
  return s.split(/[\s,，]+/).map((x) => x.replace(/^#/, '').trim()).filter(Boolean)
}

/** 由键盘事件生成快捷键字符串，如 Ctrl+Alt+N；只有修饰键时返回 null。 */
export function hotkeyFromEvent(e: { key: string; ctrlKey: boolean; altKey: boolean; shiftKey: boolean; metaKey: boolean }): string | null {
  const k = e.key
  if (['Control', 'Alt', 'Shift', 'Meta'].includes(k)) return null
  const parts: string[] = []
  if (e.ctrlKey) parts.push('Ctrl')
  if (e.altKey) parts.push('Alt')
  if (e.shiftKey) parts.push('Shift')
  if (e.metaKey) parts.push('Win')
  if (parts.length === 0) return null
  let key = k.length === 1 ? k.toUpperCase() : k
  if (/^F\d{1,2}$/.test(key)) key = key.toUpperCase()
  else if (key.length > 1) return null
  parts.push(key)
  return parts.join('+')
}
