import { describe, expect, it } from 'vitest'
import { dayDiff, formatDue, fromLocalInput, highlight, hotkeyFromEvent, parseTags, toLocalInput } from './format'

const now = new Date(2026, 8, 29, 10, 30) // 2026-09-29 10:30 本地

describe('formatDue', () => {
  it('今天/明天/后天/昨天', () => {
    expect(formatDue(new Date(2026, 8, 29, 15, 0).getTime(), now)).toBe('今天 15:00')
    expect(formatDue(new Date(2026, 8, 30, 9, 5).getTime(), now)).toBe('明天 09:05')
    expect(formatDue(new Date(2026, 9, 1, 9, 0).getTime(), now)).toBe('后天 09:00')
    expect(formatDue(new Date(2026, 8, 28, 9, 0).getTime(), now)).toBe('昨天 09:00')
  })
  it('N 天后 / N 天前 / 跨年', () => {
    expect(formatDue(new Date(2026, 9, 2, 9, 0).getTime(), now)).toBe('3 天后')
    expect(formatDue(new Date(2026, 8, 25, 9, 0).getTime(), now)).toBe('4 天前')
    expect(formatDue(new Date(2026, 10, 20, 9, 0).getTime(), now)).toBe('11月20日 09:00')
    expect(formatDue(new Date(2027, 0, 2, 9, 0).getTime(), now)).toBe('2027年1月2日')
  })
  it('跨午夜按日历日计算而不是 24 小时', () => {
    expect(dayDiff(new Date(2026, 8, 30, 0, 5), new Date(2026, 8, 29, 23, 55))).toBe(1)
  })
})

describe('datetime-local 往返', () => {
  it('toLocalInput / fromLocalInput', () => {
    const ms = new Date(2026, 9, 5, 9, 0).getTime()
    expect(toLocalInput(ms)).toBe('2026-10-05T09:00')
    expect(fromLocalInput('2026-10-05T09:00')).toBe(ms)
    expect(fromLocalInput('')).toBeNull()
    expect(toLocalInput(null)).toBe('')
  })
})

describe('highlight', () => {
  it('切分命中片段（不区分大小写）', () => {
    expect(highlight('回复 HR 邮件', 'hr')).toEqual([{ s: '回复 ', hit: false }, { s: 'HR', hit: true }, { s: ' 邮件', hit: false }])
    expect(highlight('abc', '')).toEqual([{ s: 'abc', hit: false }])
    expect(highlight('aaa', 'a').filter((x) => x.hit)).toHaveLength(3)
  })
})

describe('parseTags / hotkey', () => {
  it('parseTags', () => expect(parseTags('#工作 财务，家')).toEqual(['工作', '财务', '家']))
  it('hotkeyFromEvent', () => {
    const base = { ctrlKey: false, altKey: false, shiftKey: false, metaKey: false }
    expect(hotkeyFromEvent({ ...base, key: 'n', ctrlKey: true, altKey: true })).toBe('Ctrl+Alt+N')
    expect(hotkeyFromEvent({ ...base, key: 'F5', ctrlKey: true })).toBe('Ctrl+F5')
    expect(hotkeyFromEvent({ ...base, key: 'n' })).toBeNull() // 没有修饰键
    expect(hotkeyFromEvent({ ...base, key: 'Control', ctrlKey: true })).toBeNull()
  })
})
