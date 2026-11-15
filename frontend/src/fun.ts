// 摸鱼功能的纯逻辑（无副作用，便于单元测试）：狗狗的表情、下班倒计时、狗狗说的话。
import type { DND } from './types'

/** 狗狗的状态，按优先级从高到低。 */
export type PetMood = 'celebrate' | 'happy' | 'worried' | 'sleeping' | 'offwork' | 'dozing' | 'normal'

export interface MoodInput {
  now: Date
  /** 刚发生的事件带来的短暂状态（完成事项 / 摸头 → happy；升级 / 今天全部完成 → celebrate） */
  transient: 'happy' | 'celebrate' | null
  overdue: number
  worryEnabled: boolean
  dnd: DND
  workEnd: string
  idleSeconds: number
}

/** 打盹阈值：30 分钟没动电脑。 */
export const DOZE_AFTER = 30 * 60
/** 下班后伸懒腰持续 15 分钟。 */
export const STRETCH_MINUTES = 15

function minutesOf(hhmm: string): number {
  const [h, m] = hhmm.split(':').map(Number)
  return (h || 0) * 60 + (m || 0)
}

/** now 是否落在 [start, end)，支持跨午夜（如 22:00–08:00）。start == end 视为不在范围内。 */
export function inDailyRange(now: Date, start: string, end: string): boolean {
  const t = now.getHours() * 60 + now.getMinutes()
  const s = minutesOf(start)
  const e = minutesOf(end)
  if (s === e) return false
  return s < e ? t >= s && t < e : t >= s || t < e
}

export const isWeekday = (d: Date) => d.getDay() >= 1 && d.getDay() <= 5

export function petMood(i: MoodInput): PetMood {
  if (i.transient) return i.transient
  if (i.worryEnabled && i.overdue > 0) return 'worried'
  if (i.dnd.enabled && inDailyRange(i.now, i.dnd.start, i.dnd.end)) return 'sleeping'
  const endMin = minutesOf(i.workEnd)
  const endPlus = `${String(Math.floor(((endMin + STRETCH_MINUTES) % 1440) / 60)).padStart(2, '0')}:${String((endMin + STRETCH_MINUTES) % 60).padStart(2, '0')}`
  if (isWeekday(i.now) && inDailyRange(i.now, i.workEnd, endPlus)) return 'offwork'
  if (i.idleSeconds >= DOZE_AFTER) return 'dozing'
  return 'normal'
}

// ---------------------------------------------------------------- 下班倒计时 / 今天已赚

/** 每月计薪天数（国家规定的月计薪天数 21.75 天）。 */
export const PAY_DAYS_PER_MONTH = 21.75

export interface Countdown {
  state: 'weekend' | 'before' | 'working' | 'after'
  /** working：距下班的毫秒数；before：距上班的毫秒数 */
  remainingMs: number
  /** 今天工作时段已经过去的比例 0–1（用来算已赚） */
  fraction: number
}

export function countdown(now: Date, workStart: string, workEnd: string): Countdown {
  if (!isWeekday(now)) return { state: 'weekend', remainingMs: 0, fraction: 0 }
  const day = new Date(now.getFullYear(), now.getMonth(), now.getDate())
  const start = day.getTime() + minutesOf(workStart) * 60_000
  let end = day.getTime() + minutesOf(workEnd) * 60_000
  if (end <= start) end += 86_400_000 // 下班时间早于上班时间：按跨午夜的夜班理解
  const t = now.getTime()
  if (t < start) return { state: 'before', remainingMs: start - t, fraction: 0 }
  if (t >= end) return { state: 'after', remainingMs: 0, fraction: 1 }
  return { state: 'working', remainingMs: end - t, fraction: (t - start) / (end - start) }
}

/** 今天已赚多少（月薪 / 21.75 × 今天已过去的工作时段比例）。 */
export const earnedToday = (monthlySalary: number, fraction: number) =>
  monthlySalary > 0 ? (monthlySalary / PAY_DAYS_PER_MONTH) * Math.min(1, Math.max(0, fraction)) : 0

/** 毫秒 → H:MM:SS */
export function formatDuration(ms: number): string {
  const s = Math.max(0, Math.floor(ms / 1000))
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  return `${h}:${String(m).padStart(2, '0')}:${String(s % 60).padStart(2, '0')}`
}

// ---------------------------------------------------------------- 狗狗说的话

export const IDLE_LINES = [
  '再摸 5 分钟就干活',
  '喝口水吧～',
  '要不要起来伸个懒腰？',
  '今天也辛苦啦',
  '眼镜有点歪，帮我扶一下？',
  '工作是做不完的，摸鱼是摸得完的',
  '盯着屏幕好久了，看看远处吧',
  '汪！',
]

export interface TalkContext {
  mood: PetMood
  overdue: number
  levelUp: boolean
  newNews: boolean
  todayDone: boolean
  name: string
}

/** 选一句话：情境优先，否则从闲聊里随机挑（rand ∈ [0,1)）。 */
export function petLine(c: TalkContext, rand: number): string {
  if (c.levelUp) return '升级啦！谢谢你喂我～'
  if (c.todayDone) return '今天的事都做完啦，太棒了！'
  if (c.mood === 'offwork') return '下班啦！'
  if (c.mood === 'worried' && c.overdue > 0) return `有 ${c.overdue} 件事逾期了，我陪你一起搞定`
  if (c.mood === 'sleeping' || c.mood === 'dozing') return 'Zzz…'
  if (c.newNews) return '新闻条有新鲜事，推一下眼镜看看'
  return IDLE_LINES[Math.floor(rand * IDLE_LINES.length) % IDLE_LINES.length]
}

/** 闲聊最短间隔：每小时最多 1 次。 */
export const TALK_INTERVAL_MS = 60 * 60 * 1000

/** 相对时间：刚刚 / 12 分钟前 / 3 小时前 / 2 天前 */
export function ago(ms: number, now = Date.now()): string {
  const d = Math.max(0, now - ms)
  if (d < 60_000) return '刚刚'
  if (d < 3_600_000) return `${Math.floor(d / 60_000)} 分钟前`
  if (d < 86_400_000) return `${Math.floor(d / 3_600_000)} 小时前`
  return `${Math.floor(d / 86_400_000)} 天前`
}
