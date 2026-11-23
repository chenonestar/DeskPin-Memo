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
  /** 刚被叫醒：暂时不睡 / 不打盹 */
  awake?: boolean
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
  if (!i.awake && i.dnd.enabled && inDailyRange(i.now, i.dnd.start, i.dnd.end)) return 'sleeping'
  const endMin = minutesOf(i.workEnd)
  const endPlus = `${String(Math.floor(((endMin + STRETCH_MINUTES) % 1440) / 60)).padStart(2, '0')}:${String((endMin + STRETCH_MINUTES) % 60).padStart(2, '0')}`
  if (isWeekday(i.now) && inDailyRange(i.now, i.workEnd, endPlus)) return 'offwork'
  if (!i.awake && i.idleSeconds >= DOZE_AFTER) return 'dozing'
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

/** 毫秒 → H:MM（紧凑显示用，向下取整到分钟） */
export function formatHM(ms: number): string {
  const m = Math.max(0, Math.floor(ms / 60_000))
  return `${Math.floor(m / 60)}:${String(m % 60).padStart(2, '0')}`
}

export type MoneyMode = 'hover' | 'always' | 'never'

/**
 * 倒计时文字。紧凑：「下班 2:13 · ¥ ••」；展开（鼠标悬停）：「距下班 2:13:05 · 今天已赚 ¥386.20」。
 * 金额：没填月薪或周末不显示；never 不显示；hover 只在展开时显示数字；always 一直显示数字。
 */
export function countdownText(c: Countdown, expanded: boolean, salary: number, money: MoneyMode): string {
  let t: string
  switch (c.state) {
    case 'weekend': t = expanded ? '周末愉快' : '周末'; break
    case 'before': t = expanded ? `距上班 ${formatDuration(c.remainingMs)}` : `上班 ${formatHM(c.remainingMs)}`; break
    case 'after': t = expanded ? '已下班 🎉' : '已下班'; break
    default: t = expanded ? `距下班 ${formatDuration(c.remainingMs)}` : `下班 ${formatHM(c.remainingMs)}`
  }
  if (salary > 0 && c.state !== 'weekend' && money !== 'never') {
    const v = earnedToday(salary, c.fraction).toFixed(2)
    t += expanded ? ` · 今天已赚 ¥${v}` : money === 'always' ? ` · ¥${v}` : ' · ¥ ••'
  }
  return t
}

/** 倒计时和新闻都开启、且宽度小于这个值时分成两行。 */
export const TWO_ROW_WIDTH = 380

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

// ---------------------------------------------------------------- 节日装扮（二期）

/** 节日临时配饰。 */
export type HolidayAccessory = 'partyhat' | 'redscarf' | 'mugwort' | 'bunny' | 'santa'

/**
 * 农历节日的公历日期（2026–2035）。不在运行时用 Intl 的农历计算：ICU 在朔日接近北京时间午夜的年份会差一天
 * （例如 2027 春节实为 2 月 6 日、2030 春节实为 2 月 3 日，ICU 给出 2 月 7 日、2 月 2 日），这里以官方公布的日期为准。
 */
export const LUNAR_FESTIVALS: Record<number, { spring: string; dragonBoat: string; midAutumn: string }> = {
  2026: { spring: '2026-02-17', dragonBoat: '2026-06-19', midAutumn: '2026-09-25' },
  2027: { spring: '2027-02-06', dragonBoat: '2027-06-09', midAutumn: '2027-09-15' },
  2028: { spring: '2028-01-26', dragonBoat: '2028-05-28', midAutumn: '2028-10-03' },
  2029: { spring: '2029-02-13', dragonBoat: '2029-06-16', midAutumn: '2029-09-22' },
  2030: { spring: '2030-02-03', dragonBoat: '2030-06-05', midAutumn: '2030-09-12' },
  2031: { spring: '2031-01-23', dragonBoat: '2031-06-24', midAutumn: '2031-10-01' },
  2032: { spring: '2032-02-11', dragonBoat: '2032-06-12', midAutumn: '2032-09-19' },
  2033: { spring: '2033-01-31', dragonBoat: '2033-06-01', midAutumn: '2033-09-08' },
  2034: { spring: '2034-02-19', dragonBoat: '2034-06-20', midAutumn: '2034-09-27' },
  2035: { spring: '2035-02-08', dragonBoat: '2035-06-10', midAutumn: '2035-09-16' },
}

const dayNum = (d: Date) => Math.floor(Date.UTC(d.getFullYear(), d.getMonth(), d.getDate()) / 86_400_000)
const isoDayNum = (iso: string) => { const [y, m, d] = iso.split('-').map(Number); return Math.floor(Date.UTC(y, m - 1, d) / 86_400_000) }

/** 今天该戴的节日配饰（节日当天前后共 3 天；春节为除夕至初七）。中秋与国庆重叠时戴兔耳。 */
export function holidayOutfit(d: Date): { id: HolidayAccessory; name: string } | null {
  const n = dayNum(d)
  const m = d.getMonth() + 1
  const day = d.getDate()
  const near = (iso: string, before: number, after: number) => { const c = isoDayNum(iso); return n >= c - before && n <= c + after }
  for (const y of [d.getFullYear(), d.getFullYear() + 1]) { // 除夕可能在上一个公历年的年末（不会发生，但保持对称）
    const f = LUNAR_FESTIVALS[y]
    if (!f) continue
    if (near(f.spring, 1, 6)) return { id: 'redscarf', name: '春节' }
    if (near(f.dragonBoat, 1, 1)) return { id: 'mugwort', name: '端午' }
    if (near(f.midAutumn, 1, 1)) return { id: 'bunny', name: '中秋' }
  }
  if ((m === 12 && day === 31) || (m === 1 && day <= 2)) return { id: 'partyhat', name: '元旦' }
  if (m === 10 && day <= 3) return { id: 'redscarf', name: '国庆' }
  if (m === 12 && day >= 24 && day <= 26) return { id: 'santa', name: '圣诞' }
  return null
}

// ---------------------------------------------------------------- 纪念日 / 问候 / 喝水提醒（二期）

/** 陪伴纪念日：第 7、30、100 天，以及每满一年（同月同日）。返回要说的话，不是纪念日返回 null。 */
export function anniversary(created: Date, now: Date, days: number): string | null {
  if (days === 7 || days === 30 || days === 100) return `我们认识 ${days} 天啦！`
  const years = now.getFullYear() - created.getFullYear()
  if (years > 0 && now.getMonth() === created.getMonth() && now.getDate() === created.getDate()) return `我们认识 ${years} 年啦！`
  return null
}

export type Greeting = 'morning' | 'lunch'

/** 工作日的时段问候：上班后 3 小时内第一次出现说「早」，12 点到 13 点说「饭点到啦」。said 为今天已经说过的。 */
export function dueGreeting(now: Date, workStart: string, said: Set<Greeting>): Greeting | null {
  if (!isWeekday(now)) return null
  const t = now.getHours() * 60 + now.getMinutes()
  const s = minutesOf(workStart)
  if (!said.has('morning') && t >= s && t < s + 180 && t < 12 * 60) return 'morning'
  if (!said.has('lunch') && t >= 12 * 60 && t < 13 * 60) return 'lunch'
  return null
}

export const GREETING_LINE: Record<Greeting, string> = { morning: '早！今天也一起加油', lunch: '饭点到啦，去吃饭吧～' }

export interface WaterInput {
  now: Date
  enabled: boolean
  intervalMin: number
  lastAt: number | null // 上次提醒（或「10 分钟后」推算出的）时间
  workStart: string
  workEnd: string
  dnd: DND
  idleSeconds: number
  boss: boolean
}

/** 用户离开电脑超过这么久就不提醒（秒）。 */
export const WATER_AWAY = 5 * 60

/** 是否该提醒喝水 / 起身：只在工作日上班时间；免打扰、正经模式、离开电脑时不提醒。 */
export function waterDue(i: WaterInput): boolean {
  if (!i.enabled || i.boss || !isWeekday(i.now) || i.idleSeconds >= WATER_AWAY) return false
  if (i.dnd.enabled && inDailyRange(i.now, i.dnd.start, i.dnd.end)) return false
  if (!inDailyRange(i.now, i.workStart, i.workEnd)) return false
  const start = new Date(i.now.getFullYear(), i.now.getMonth(), i.now.getDate()).getTime() + minutesOf(i.workStart) * 60_000
  const from = i.lastAt != null && i.lastAt >= start ? i.lastAt : start // 每天从上班时间重新计时
  return i.now.getTime() - from >= i.intervalMin * 60_000
}

export const WATER_LINE = { water: '喝口水吧～', stand: '起来走走、伸个懒腰吧' } as const

/** 连点彩蛋：2 秒内点 5 次。 */
export function isSneezeCombo(clicks: number[], now: number): boolean {
  return clicks.filter((t) => now - t <= 2000).length >= 5
}

// ---------------------------------------------------------------- 自主动作（三期）

/** 十个随机小动作。 */
export type IdleAct = 'yawn' | 'scratch' | 'shakeoff' | 'lookaround' | 'chase' | 'clean' | 'read' | 'patrol' | 'butterfly' | 'liedown'
export type Activity = 'quiet' | 'normal' | 'lively'

/** 每个动作的时长（毫秒）与出现权重：常见的小动作多些，大动作少些。 */
export const IDLE_ACTS: Record<IdleAct, { ms: number; weight: number; label: string }> = {
  lookaround: { ms: 3000, weight: 5, label: '东张西望' },
  yawn: { ms: 1500, weight: 4, label: '打哈欠' },
  scratch: { ms: 2000, weight: 4, label: '挠痒痒' },
  shakeoff: { ms: 1000, weight: 3, label: '抖抖身子' },
  liedown: { ms: 8000, weight: 3, label: '趴下休息' },
  clean: { ms: 3000, weight: 2, label: '擦眼镜' },
  chase: { ms: 2500, weight: 2, label: '追尾巴' },
  read: { ms: 6000, weight: 2, label: '看书' },
  butterfly: { ms: 3000, weight: 1, label: '捉蝴蝶' },
  patrol: { ms: 6000, weight: 1, label: '巡逻' },
}

/** 巡逻需要的最小宽度（像素）。 */
export const PATROL_MIN_WIDTH = 240

/** 下一次自主动作的间隔（毫秒）；「安静」返回 null（不做随机动作）。rand ∈ [0,1)。 */
export function idleInterval(activity: Activity, rand: number): number | null {
  if (activity === 'quiet') return null
  const [lo, hi] = activity === 'lively' ? [30_000, 90_000] : [60_000, 180_000]
  return Math.round(lo + (hi - lo) * Math.min(Math.max(rand, 0), 0.999999))
}

/** 按权重挑一个动作：不连续重复上一个；便签太窄时不巡逻。rand ∈ [0,1)。 */
export function pickIdleAct(prev: IdleAct | null, rand: number, canPatrol: boolean): IdleAct {
  const pool = (Object.keys(IDLE_ACTS) as IdleAct[]).filter((a) => a !== prev && (canPatrol || a !== 'patrol'))
  const total = pool.reduce((n, a) => n + IDLE_ACTS[a].weight, 0)
  let x = Math.min(Math.max(rand, 0), 0.999999) * total
  for (const a of pool) {
    x -= IDLE_ACTS[a].weight
    if (x < 0) return a
  }
  return pool[pool.length - 1]
}

export interface IdleGate {
  mood: PetMood
  busy: boolean // 正在提醒 / 说话 / 拖动 / 玩球 / 互动 / 菜单或对话框开着
  boss: boolean
  hidden: boolean // 便签被隐藏 / 最小化
}

/** 现在能不能做随机动作：醒着、心情平常、没在忙。 */
export const canIdle = (g: IdleGate) => g.mood === 'normal' && !g.busy && !g.boss && !g.hidden

/** 鼠标在狗狗身上停留多久翻肚皮（毫秒）；翻肚皮最长持续多久。 */
export const BELLY_HOVER_MS = 3000
export const BELLY_MAX_MS = 6000
/** 停止打字多久后不再盯着输入框（毫秒）。 */
export const TYPING_LINGER_MS = 2000

// ---------------------------------------------------------------- 卷起便签（喝水 / 起身提醒）

/** 卷帘的几个阶段：跑到中间 → 卷起 → 卷着等你点 →（喝水 / 伸懒腰）→ 展开。 */
export type RollPhase = 'call' | 'rolling' | 'rolled' | 'finish' | 'unrolling'
/** 各阶段时长（毫秒）；rolled 一直等你点按钮。 */
export const ROLL_MS: Record<Exclude<RollPhase, 'rolled'>, number> = { call: 600, rolling: 1400, finish: 1000, unrolling: 1200 }
/** 最近这么久内打过字就先不卷（毫秒）。 */
export const ROLL_WAIT_TYPING_MS = 10_000

/** 下一个阶段；rolled 要等你回答（done = 喝好了 / 活动好了，snooze = 10 分钟后）；null 表示结束。 */
export function nextRollPhase(p: RollPhase, answer?: 'done' | 'snooze'): RollPhase | null {
  switch (p) {
    case 'call': return 'rolling'
    case 'rolling': return 'rolled'
    case 'rolled': return answer === 'done' ? 'finish' : answer === 'snooze' ? 'unrolling' : 'rolled'
    case 'finish': return 'unrolling'
    case 'unrolling': return null
  }
}

export interface ReminderStartInput {
  style: string // roll | bubble
  collapsed: boolean // 便签折叠成一行
  typedAgoMs: number | null // 距上次在便签里打字（null = 没打过）
  dialogOpen: boolean // 对话框开着
  editingRow: boolean // 正在改某条事项 / 子任务
  waitedMs: number // 已经因为你在忙等了多久
}

/** 对话框 / 编辑一直开着时最多等这么久，之后改为只冒气泡（不挡住你正在做的事）。 */
export const ROLL_WAIT_MAX_MS = 2 * 60_000

/**
 * 提醒到点时怎么开始：notify = 便签折叠着，只发系统通知；bubble = 只冒气泡；
 * wait = 你正忙（打字 / 编辑 / 对话框），等会儿再卷；roll = 卷起便签。
 */
export function reminderStart(i: ReminderStartInput): 'notify' | 'bubble' | 'wait' | 'roll' {
  if (i.collapsed) return 'notify'
  if (i.style === 'bubble') return 'bubble'
  if (i.dialogOpen || i.editingRow || (i.typedAgoMs != null && i.typedAgoMs < ROLL_WAIT_TYPING_MS)) return i.waitedMs >= ROLL_WAIT_MAX_MS ? 'bubble' : 'wait'
  return 'roll'
}

/** 「10 分钟后」：把上次提醒时间往前推，使 10 分钟后正好到期。 */
export const snoozeLast = (now: number, intervalMin: number) => now - intervalMin * 60_000 + 10 * 60_000
