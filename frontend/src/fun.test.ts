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
