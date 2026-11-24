import { describe, expect, it } from 'vitest'
import { ago, countdown, countdownText, earnedToday, formatDuration, formatHM, inDailyRange, petLine, petMood, type MoodInput } from './fun'

// 2026-11-16 是星期一
const at = (d: number, h: number, m = 0) => new Date(2026, 10, d, h, m)
const base: MoodInput = {
  now: at(16, 10), transient: null, overdue: 0, worryEnabled: true,
  dnd: { enabled: false, start: '22:00', end: '08:00' }, workEnd: '18:00', idleSeconds: 0,
}

describe('狗狗表情', () => {
  it('优先级：事件 > 担心 > 睡觉 > 下班 > 打盹 > 平常', () => {
    expect(petMood(base)).toBe('normal')
    expect(petMood({ ...base, idleSeconds: 1800 })).toBe('dozing')
    expect(petMood({ ...base, idleSeconds: 1799 })).toBe('normal')
    expect(petMood({ ...base, now: at(16, 18, 5), idleSeconds: 4000 })).toBe('offwork')
    expect(petMood({ ...base, now: at(16, 18, 15) })).toBe('normal') // 伸懒腰只持续 15 分钟
    expect(petMood({ ...base, now: at(21, 18, 5) })).toBe('normal') // 周六不「下班」
    expect(petMood({ ...base, now: at(16, 23), dnd: { enabled: true, start: '22:00', end: '08:00' } })).toBe('sleeping')
    expect(petMood({ ...base, now: at(16, 7), dnd: { enabled: true, start: '22:00', end: '08:00' }, overdue: 2 })).toBe('worried')
    expect(petMood({ ...base, overdue: 2, worryEnabled: false })).toBe('normal') // 可以关掉「担心」
    expect(petMood({ ...base, overdue: 2, transient: 'happy' })).toBe('happy')
    expect(petMood({ ...base, transient: 'celebrate' })).toBe('celebrate')
  })
  it('跨午夜的时间段', () => {
    expect(inDailyRange(at(16, 23), '22:00', '08:00')).toBe(true)
    expect(inDailyRange(at(16, 7, 59), '22:00', '08:00')).toBe(true)
    expect(inDailyRange(at(16, 8), '22:00', '08:00')).toBe(false)
    expect(inDailyRange(at(16, 12), '09:00', '18:00')).toBe(true)
    expect(inDailyRange(at(16, 12), '09:00', '09:00')).toBe(false)
  })
  it('下班时间接近午夜时，伸懒腰的时段跨过午夜', () => {
    expect(petMood({ ...base, now: at(16, 23, 55), workEnd: '23:50' })).toBe('offwork')
    expect(petMood({ ...base, now: at(17, 0, 3), workEnd: '23:50' })).toBe('offwork')
  })
})

describe('下班倒计时', () => {
  it('上班前 / 工作中 / 下班后 / 周末', () => {
    expect(countdown(at(16, 8), '09:00', '18:00')).toMatchObject({ state: 'before', remainingMs: 3_600_000 })
    const w = countdown(at(16, 13, 30), '09:00', '18:00')
    expect(w.state).toBe('working')
    expect(w.remainingMs).toBe(4.5 * 3_600_000)
    expect(w.fraction).toBeCloseTo(0.5)
    expect(countdown(at(16, 18), '09:00', '18:00').state).toBe('after')
    expect(countdown(at(22, 12), '09:00', '18:00').state).toBe('weekend')
  })
  it('今天已赚：月薪 / 21.75 × 已过比例', () => {
    expect(earnedToday(21750, 0.5)).toBeCloseTo(500)
    expect(earnedToday(0, 0.5)).toBe(0)
    expect(earnedToday(21750, 2)).toBeCloseTo(1000)
  })
  it('格式化', () => {
    expect(formatDuration(2 * 3_600_000 + 13 * 60_000 + 5_000)).toBe('2:13:05')
    expect(formatDuration(-5)).toBe('0:00:00')
    const now = 10_000_000_000
    expect(ago(now - 30_000, now)).toBe('刚刚')
    expect(ago(now - 12 * 60_000, now)).toBe('12 分钟前')
    expect(ago(now - 3 * 3_600_000, now)).toBe('3 小时前')
    expect(ago(now - 50 * 3_600_000, now)).toBe('2 天前')
  })
})

describe('紧凑倒计时与金额显示', () => {
  const working = countdown(at(16, 15, 46, ), '09:00', '18:00') // 距下班 2:14:00
  it('紧凑只显示时分，悬停展开到秒', () => {
    expect(formatHM(2 * 3_600_000 + 13 * 60_000 + 59_000)).toBe('2:13')
    expect(countdownText(working, false, 0, 'hover')).toBe('下班 2:14')
    expect(countdownText(working, true, 0, 'hover')).toBe('距下班 2:14:00')
    expect(countdownText(countdown(at(16, 8, 15), '09:00', '18:00'), false, 0, 'hover')).toBe('上班 0:45')
    expect(countdownText(countdown(at(16, 19), '09:00', '18:00'), false, 0, 'hover')).toBe('已下班')
    expect(countdownText(countdown(at(22, 12), '09:00', '18:00'), false, 21750, 'always')).toBe('周末')
  })
  it('金额：悬停才显示 / 一直显示 / 不显示；没填月薪不显示', () => {
    const c = countdown(at(16, 13, 30), '09:00', '18:00') // 已过一半
    expect(countdownText(c, false, 21750, 'hover')).toBe('下班 4:30 · ¥ ••')
    expect(countdownText(c, true, 21750, 'hover')).toBe('距下班 4:30:00 · 今天已赚 ¥500.00')
    expect(countdownText(c, false, 21750, 'always')).toBe('下班 4:30 · ¥500.00')
    expect(countdownText(c, true, 21750, 'never')).toBe('距下班 4:30:00')
    expect(countdownText(c, false, 0, 'always')).toBe('下班 4:30')
  })
})

describe('狗狗说的话', () => {
  const c = { mood: 'normal' as const, overdue: 0, levelUp: false, newNews: false, todayDone: false, name: '眼镜' }
  it('情境优先', () => {
    expect(petLine({ ...c, levelUp: true }, 0)).toContain('升级')
    expect(petLine({ ...c, todayDone: true }, 0)).toContain('做完')
    expect(petLine({ ...c, mood: 'offwork' }, 0)).toBe('下班啦！')
    expect(petLine({ ...c, mood: 'worried', overdue: 3 }, 0)).toContain('3 件事逾期')
    expect(petLine({ ...c, newNews: true }, 0)).toContain('新闻')
  })
  it('否则随机闲聊', () => {
    expect(petLine(c, 0)).toBe('再摸 5 分钟就干活')
    expect(petLine(c, 0.999)).toBe('汪！')
  })
})

import { anniversary, dueGreeting, holidayOutfit, isSneezeCombo, LUNAR_FESTIVALS, waterDue, type WaterInput } from './fun'

describe('节日装扮', () => {
  const d = (y: number, m: number, day: number) => new Date(y, m - 1, day, 10)
  it('农历节日：除夕到初七戴红围巾，端午 / 中秋前后各一天', () => {
    expect(holidayOutfit(d(2027, 2, 5))?.name).toBe('春节') // 除夕
    expect(holidayOutfit(d(2027, 2, 12))?.name).toBe('春节') // 初七
    expect(holidayOutfit(d(2027, 2, 13))).toBeNull()
    expect(holidayOutfit(d(2027, 2, 4))).toBeNull()
    expect(holidayOutfit(d(2026, 6, 19))).toMatchObject({ id: 'mugwort', name: '端午' })
    expect(holidayOutfit(d(2026, 9, 26))).toMatchObject({ id: 'bunny', name: '中秋' })
    expect(holidayOutfit(d(2026, 9, 27))).toBeNull()
  })
  it('公历节日；中秋与国庆重叠时戴兔耳', () => {
    expect(holidayOutfit(d(2026, 12, 31))?.id).toBe('partyhat')
    expect(holidayOutfit(d(2027, 1, 2))?.id).toBe('partyhat')
    expect(holidayOutfit(d(2026, 10, 2))).toMatchObject({ id: 'redscarf', name: '国庆' })
    expect(holidayOutfit(d(2026, 12, 25))?.id).toBe('santa')
    expect(holidayOutfit(d(2031, 10, 1))?.name).toBe('中秋')
    expect(holidayOutfit(d(2026, 11, 17))).toBeNull()
  })
  it('日期表与 ICU 农历一致，只有已知的两处 ICU 偏差（朔日接近午夜）', () => {
    const f = new Intl.DateTimeFormat('zh-CN-u-ca-chinese', { month: 'numeric', day: 'numeric' })
    const lunar = (iso: string) => { const [y, m, dd] = iso.split('-').map(Number); const p = Object.fromEntries(f.formatToParts(new Date(y, m - 1, dd, 12)).map((x) => [x.type, x.value])); return `${p.month}/${p.day}` }
    const icuOff = new Set(['2027-02-06', '2030-02-03'])
    for (const v of Object.values(LUNAR_FESTIVALS)) {
      if (!icuOff.has(v.spring)) expect(lunar(v.spring)).toBe('1/1')
      expect(lunar(v.dragonBoat)).toBe('5/5')
      expect(lunar(v.midAutumn)).toBe('8/15')
    }
  })
})

describe('纪念日 / 问候 / 喝水提醒 / 连点', () => {
  it('纪念日', () => {
    const c = new Date(2026, 10, 15)
    expect(anniversary(c, new Date(2026, 10, 21), 7)).toBe('我们认识 7 天啦！')
    expect(anniversary(c, new Date(2027, 1, 22), 100)).toBe('我们认识 100 天啦！')
    expect(anniversary(c, new Date(2027, 10, 15), 366)).toBe('我们认识 1 年啦！')
    expect(anniversary(c, new Date(2026, 10, 20), 6)).toBeNull()
  })
  it('时段问候：工作日、每天一次', () => {
    expect(dueGreeting(at(16, 9, 5), '09:00', new Set())).toBe('morning')
    expect(dueGreeting(at(16, 8, 50), '09:00', new Set())).toBeNull()
    expect(dueGreeting(at(16, 9, 5), '09:00', new Set(['morning']))).toBeNull()
    expect(dueGreeting(at(16, 12, 10), '09:00', new Set(['morning']))).toBe('lunch')
    expect(dueGreeting(at(21, 12, 10), '09:00', new Set())).toBeNull() // 周六
  })
  it('喝水提醒：上班时间按间隔；离开电脑 / 免打扰 / 正经模式 / 周末不提醒', () => {
    const base: WaterInput = { now: at(16, 9, 44), enabled: true, intervalMin: 45, lastAt: null, workStart: '09:00', workEnd: '18:00',
      dnd: { enabled: false, start: '22:00', end: '08:00' }, idleSeconds: 0, boss: false }
    expect(waterDue(base)).toBe(false)
    expect(waterDue({ ...base, now: at(16, 9, 45) })).toBe(true)
    expect(waterDue({ ...base, now: at(16, 10, 20), lastAt: at(16, 9, 45).getTime() })).toBe(false)
    expect(waterDue({ ...base, now: at(16, 10, 30), lastAt: at(16, 9, 45).getTime() })).toBe(true)
    expect(waterDue({ ...base, now: at(16, 9, 30), lastAt: at(13, 17, 0).getTime() })).toBe(false) // 前一天的记录不算：从今天上班开始计
    expect(waterDue({ ...base, now: at(16, 10, 0), idleSeconds: 400 })).toBe(false)
    expect(waterDue({ ...base, now: at(16, 10, 0), boss: true })).toBe(false)
    expect(waterDue({ ...base, now: at(16, 19, 0) })).toBe(false)
    expect(waterDue({ ...base, now: at(21, 10, 0) })).toBe(false)
    expect(waterDue({ ...base, now: at(16, 10, 0), enabled: false })).toBe(false)
    expect(waterDue({ ...base, now: at(16, 10, 0), dnd: { enabled: true, start: '09:30', end: '11:00' } })).toBe(false)
  })
  it('叫醒后暂时不睡', () => {
    expect(petMood({ ...base, idleSeconds: 4000, awake: true })).toBe('normal')
    expect(petMood({ ...base, now: at(16, 23), dnd: { enabled: true, start: '22:00', end: '08:00' }, awake: true })).toBe('normal')
  })
  it('连点彩蛋', () => {
    expect(isSneezeCombo([0, 300, 600, 900, 1200], 1300)).toBe(true)
    expect(isSneezeCombo([0, 300, 600, 900], 1000)).toBe(false)
    expect(isSneezeCombo([0, 300, 600, 900, 2600], 2700)).toBe(false)
  })
})

import { canIdle, IDLE_ACTS, idleInterval, pickIdleAct, type IdleAct } from './fun'

describe('自主动作调度', () => {
  it('间隔：安静不做；正常 1–3 分钟；活泼 30–90 秒', () => {
    expect(idleInterval('quiet', 0.5)).toBeNull()
    expect(idleInterval('normal', 0)).toBe(60_000)
    expect(idleInterval('normal', 0.999999)).toBeLessThanOrEqual(180_000)
    expect(idleInterval('lively', 0)).toBe(30_000)
    expect(idleInterval('lively', 0.999999)).toBeLessThanOrEqual(90_000)
  })
  it('不连续重复；太窄不巡逻；每个动作都可能被选到', () => {
    const seen = new Set<IdleAct>()
    for (let i = 0; i < 1000; i++) {
      const r = (i + 0.5) / 1000
      const a = pickIdleAct('lookaround', r, true)
      expect(a).not.toBe('lookaround')
      seen.add(a)
      expect(pickIdleAct(null, r, false)).not.toBe('patrol')
    }
    expect(seen.size).toBe(Object.keys(IDLE_ACTS).length - 1)
    expect(pickIdleAct(null, 0, true)).toBe('lookaround') // 权重最高的排在最前
  })
  it('什么时候能做：醒着、平常、不忙、非正经模式、便签可见', () => {
    const g = { mood: 'normal' as const, busy: false, boss: false, hidden: false }
    expect(canIdle(g)).toBe(true)
    for (const m of ['happy', 'celebrate', 'worried', 'sleeping', 'dozing', 'offwork'] as const) expect(canIdle({ ...g, mood: m })).toBe(false)
    expect(canIdle({ ...g, busy: true })).toBe(false)
    expect(canIdle({ ...g, boss: true })).toBe(false)
    expect(canIdle({ ...g, hidden: true })).toBe(false)
  })
})

import { nextRollPhase, reminderStart, ROLL_WAIT_MAX_MS, ROLL_WAIT_TYPING_MS, snoozeLast } from './fun'

describe('卷起便签', () => {
  it('阶段按顺序走，卷着时等你回答', () => {
    expect(nextRollPhase('call')).toBe('rolling')
    expect(nextRollPhase('rolling')).toBe('rolled')
    expect(nextRollPhase('rolled')).toBe('rolled')
    expect(nextRollPhase('rolled', 'done')).toBe('finish')
    expect(nextRollPhase('rolled', 'snooze')).toBe('unrolling')
    expect(nextRollPhase('finish')).toBe('unrolling')
    expect(nextRollPhase('unrolling')).toBeNull()
  })
  it('到点时：折叠只发通知、气泡方式、正忙就等、否则卷起', () => {
    const base = { style: 'roll', collapsed: false, typedAgoMs: null, dialogOpen: false, editingRow: false, waitedMs: 0 }
    expect(reminderStart(base)).toBe('roll')
    expect(reminderStart({ ...base, collapsed: true })).toBe('notify')
    expect(reminderStart({ ...base, collapsed: true, style: 'bubble' })).toBe('notify')
    expect(reminderStart({ ...base, style: 'bubble', typedAgoMs: 0 })).toBe('bubble') // 气泡不挡东西，不用等
    expect(reminderStart({ ...base, typedAgoMs: 3000 })).toBe('wait')
    expect(reminderStart({ ...base, typedAgoMs: ROLL_WAIT_TYPING_MS })).toBe('roll')
    expect(reminderStart({ ...base, dialogOpen: true })).toBe('wait')
    expect(reminderStart({ ...base, editingRow: true })).toBe('wait')
    expect(reminderStart({ ...base, dialogOpen: true, waitedMs: ROLL_WAIT_MAX_MS })).toBe('bubble') // 一直忙：别再等了，冒个气泡
  })
  it('10 分钟后：推算出的上次提醒时间正好在 10 分钟后到期', () => {
    const now = new Date(2026, 10, 16, 10, 0) // 周一
    const last = snoozeLast(now.getTime(), 45)
    const inp = { enabled: true, intervalMin: 45, lastAt: last, workStart: '09:00', workEnd: '18:00', dnd: { enabled: false, start: '22:00', end: '08:00' }, idleSeconds: 0, boss: false }
    expect(waterDue({ ...inp, now: new Date(now.getTime() + 9 * 60_000) })).toBe(false)
    expect(waterDue({ ...inp, now: new Date(now.getTime() + 10 * 60_000) })).toBe(true)
  })
})

import { tossEvent, tossLine, waterCheck } from './fun'

describe('喝水提醒的状态（用于日志）', () => {
  const base = { now: new Date(2026, 10, 16, 10, 0), enabled: true, intervalMin: 45, lastAt: null, workStart: '09:00', workEnd: '18:00',
    dnd: { enabled: false, start: '12:00', end: '13:00' }, idleSeconds: 0, boss: false }
  it('先看是否开着 / 上班时间，再看间隔，最后看为什么不能提醒', () => {
    expect(waterCheck(base)).toBe('due')
    expect(waterCheck({ ...base, enabled: false })).toBe('off')
    expect(waterCheck({ ...base, now: new Date(2026, 10, 15, 10, 0) })).toBe('off') // 星期日
    expect(waterCheck({ ...base, now: new Date(2026, 10, 16, 19, 0) })).toBe('off')
    expect(waterCheck({ ...base, now: new Date(2026, 10, 16, 9, 30) })).toBe('notyet')
    expect(waterCheck({ ...base, now: new Date(2026, 10, 16, 9, 30), boss: true, idleSeconds: 9999 })).toBe('notyet') // 没到点不记原因
    expect(waterCheck({ ...base, boss: true })).toBe('boss')
    expect(waterCheck({ ...base, dnd: { enabled: true, start: '09:30', end: '11:00' } })).toBe('dnd')
    expect(waterCheck({ ...base, idleSeconds: 301 })).toBe('away')
  })
})

describe('狗狗把过去的时间扔掉', () => {
  const at = (h: number, m: number, s = 0) => new Date(2026, 10, 16, h, m, s) // 星期一
  const toss = (a: Date, b: Date, mode: 'hour' | 'half' | 'off' = 'hour') => tossEvent(a, b, '09:00', '18:00', mode)
  it('每小时：剩余从 3:00 变成 2:59 时扔掉「3」', () => {
    expect(toss(at(14, 59, 0), at(14, 59, 15))).toBeNull() // 3:01 → 3:00
    expect(toss(at(14, 59, 50), at(15, 0, 5))).toEqual({ kind: 'hour', text: '3', last: false }) // 3:00 → 2:59
    expect(toss(at(15, 0, 50), at(15, 1, 5))).toBeNull() // 2:59 → 2:58
    expect(toss(at(15, 59, 55), at(16, 0, 10))).toEqual({ kind: 'hour', text: '2', last: false })
    expect(toss(at(16, 59, 55), at(17, 0, 10))).toEqual({ kind: 'hour', text: '1', last: true }) // 进入最后一小时
    expect(toss(at(9, 59, 55), at(10, 0, 10))).toEqual({ kind: 'hour', text: '8', last: false })
  })
  it('每 30 分钟：跨过半点扔一个「30 分」', () => {
    expect(toss(at(15, 29, 55), at(15, 30, 10), 'half')).toEqual({ kind: 'half', text: '30 分', last: false }) // 2:30 → 2:29
    expect(toss(at(14, 59, 50), at(15, 0, 5), 'half')).toEqual({ kind: 'half', text: '30 分', last: false })
    expect(toss(at(15, 10, 0), at(15, 10, 15), 'half')).toBeNull()
    expect(toss(at(16, 59, 55), at(17, 0, 10), 'half')?.last).toBe(true)
  })
  it('关闭、间隔太久、不在上班时间都不扔', () => {
    expect(toss(at(14, 59, 50), at(15, 0, 5), 'off')).toBeNull()
    expect(toss(at(14, 50), at(15, 10))).toBeNull() // 电脑睡了 20 分钟
    expect(toss(at(17, 59, 50), at(18, 0, 5))).toBeNull() // 下班了：已有伸懒腰
    expect(toss(at(8, 59, 50), at(9, 0, 5))).toBeNull()
    expect(tossEvent(new Date(2026, 10, 15, 14, 59, 50), new Date(2026, 10, 15, 15, 0, 5), '09:00', '18:00', 'hour')).toBeNull() // 周日
  })
  it('说的话：最后一小时一定说，其余约三分之一', () => {
    expect(tossLine({ kind: 'hour', text: '1', last: true }, 0.99)).toBe('最后一小时，坚持住！')
    expect(tossLine({ kind: 'hour', text: '3', last: false }, 0.1)).toBe('又熬过一小时！')
    expect(tossLine({ kind: 'hour', text: '3', last: false }, 0.5)).toBeNull()
    expect(tossLine({ kind: 'half', text: '30 分', last: false }, 0.2)).toBe('又过了半小时～')
  })
})
