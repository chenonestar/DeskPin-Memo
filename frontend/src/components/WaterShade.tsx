import { useCallback, useEffect, useRef, useState } from 'react'
import { api, on } from '../api'
import type { PetStatus, Settings } from '../types'
import { holidayOutfit, nextRollPhase, reminderStart, ROLL_MS, ROLL_WAIT_TYPING_MS, snoozeLast, WATER_LINE, waterCheck, type RollPhase } from '../fun'
import { petStore } from '../petStore'
import { Dog } from './Dog'

export type WaterKind = 'water' | 'stand'
/** 正在进行的喝水 / 起身提醒：roll = 狗狗卷起便签，bubble = 只冒气泡。 */
export type WaterReminder =
  | { kind: WaterKind; mode: 'roll'; phase: RollPhase; cups?: number }
  | { kind: WaterKind; mode: 'bubble' }
/** 提醒结束后狗狗说的话（每次 n 递增，便于狗狗那边识别「新的一句」）。 */
export interface WaterCheer { text: string; happy: boolean; n: number }
export interface WaterCtl {
  r: WaterReminder | null
  /** 到点了但你正忙，等你停手 */
  waiting: boolean
  cheer: WaterCheer | null
  answer: (snooze: boolean) => void
}

const BUBBLE_TIMEOUT = 120_000 // 气泡方式：两分钟没理会就收起

/** 正在改某条事项 / 子任务（焦点在事项行里的输入框）。 */
const editingRow = () => {
  const el = document.activeElement as HTMLElement | null
  return !!el && (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA') && !!el.closest('[data-testid="item-row"], .subs') && (el as HTMLInputElement).value !== ''
}

/** 写一行日志（只能是后端白名单里的固定事件，见 internal/app/fun.go petLogText）。 */
const plog = (code: string, kind = '') => { void api.petLog(code, kind).catch(() => undefined) }
type Cut = 'strong' | 'boss' | 'off' | 'away'

/**
 * 喝水 / 起身提醒的调度（FR 二期 #10 + 卷帘）：放在便签这一层，便签折叠时也能只发系统通知。
 * hidden = 狗狗被「暂时收起」。每一步都记一行日志，到点却没提醒时记下原因（每次到点只记一次）。
 */
export function useWaterReminder({ settings, boss, collapsed, hidden }: { settings: Settings; boss: boolean; collapsed: boolean; hidden: boolean }): WaterCtl {
  const active = settings.petEnabled && !boss && !hidden
  const [r, setR] = useState<WaterReminder | null>(null)
  const [waiting, setWaiting] = useState(false)
  const [cheer, setCheer] = useState<WaterCheer | null>(null)
  const [now, setNow] = useState(() => new Date())
  const [idle, setIdle] = useState(0)
  const [kick, setKick] = useState(0) // 等你停手后重新判断一次
  const typedAt = useRef<number | null>(null)
  const waitFrom = useRef<number | null>(null) // 开始因为你在忙而等待的时间
  const skipLogged = useRef('') // 已记过的「到点了但暂不提醒」（同一次到点、同一原因只记一次）
  const pending = useRef<{ text: string; happy: boolean } | null>(null)
  const cheerN = useRef(0)
  const rRef = useRef(r)
  rRef.current = r

  useEffect(() => {
    const tick = () => { setNow(new Date()); void api.idleSeconds().then(setIdle).catch(() => undefined) }
    tick()
    const id = window.setInterval(tick, 30_000)
    return () => clearInterval(id)
  }, [])

  // 记下最近一次在便签里打字的时间（keydown 与 input 都听：输入法 / 粘贴不一定有 keydown）
  useEffect(() => {
    const h = (e: Event) => {
      const el = e.target as HTMLElement | null
      if (el && (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA')) typedAt.current = Date.now()
    }
    window.addEventListener('keydown', h)
    window.addEventListener('input', h)
    return () => { window.removeEventListener('keydown', h); window.removeEventListener('input', h) }
  }, [])

  const say = (text: string, happy: boolean) => { cheerN.current += 1; setCheer({ text, happy, n: cheerN.current }) }

  /** 被打断（强提醒、正经模式、狗狗被关掉、便签切到别的界面）：立刻收起，不放动画，按「10 分钟后」算。 */
  const interrupt = useCallback((why: Cut) => {
    const cur = rRef.current
    if (!cur) return
    plog(`water.cut.${why}`, cur.kind)
    petStore.set('water.last', String(snoozeLast(Date.now(), settings.petWaterInterval)))
    pending.current = null
    setR(null)
  }, [settings.petWaterInterval])

  useEffect(() => on('reminder:strong', () => interrupt('strong')), [interrupt])
  useEffect(() => { if (boss || !active) interrupt(boss ? 'boss' : 'off') }, [boss, active, interrupt])
  const interruptRef = useRef(interrupt)
  interruptRef.current = interrupt
  useEffect(() => () => interruptRef.current('away'), []) // 便签被设置 / 快速输入替换掉

  // 卷起期间暂停鼠标穿透、恢复不透明（和强提醒一样），否则按钮点不到、看不清
  const rolling = r?.mode === 'roll'
  useEffect(() => { void api.setPetAlert(rolling).catch(() => undefined) }, [rolling])
  useEffect(() => () => { void api.setPetAlert(false).catch(() => undefined) }, [])

  // 到点了吗？
  useEffect(() => {
    if (r) return
    const lastRaw = petStore.get('water.last')
    const state = waterCheck({ now: new Date(), enabled: settings.petEnabled && settings.petWater, intervalMin: settings.petWaterInterval, lastAt: lastRaw ? Number(lastRaw) : null,
      workStart: settings.workStart, workEnd: settings.workEnd, dnd: settings.dnd, idleSeconds: idle, boss })
    const nextKind: WaterKind = petStore.get('water.kind') === 'water' ? 'stand' : 'water' // 喝水和起身交替
    const skip = state === 'boss' || state === 'dnd' || state === 'away' ? state : state === 'due' && hidden ? 'hidden' : null
    if (skip && skipLogged.current !== `${lastRaw}|${skip}`) { skipLogged.current = `${lastRaw}|${skip}`; plog(`water.skip.${skip}`, nextKind) }
    if (state !== 'due' || !active) { setWaiting(false); waitFrom.current = null; return }
    const typedAgo = typedAt.current == null ? null : Date.now() - typedAt.current
    const start = reminderStart({ style: settings.petWaterStyle, collapsed, typedAgoMs: typedAgo, dialogOpen: !!document.querySelector('.modal'), editingRow: editingRow(),
      waitedMs: waitFrom.current == null ? 0 : Date.now() - waitFrom.current })
    if (start === 'wait') {
      setWaiting(true)
      if (waitFrom.current == null) { waitFrom.current = Date.now(); plog('water.wait', nextKind) }
      const ms = typedAgo != null && typedAgo < ROLL_WAIT_TYPING_MS ? ROLL_WAIT_TYPING_MS - typedAgo + 50 : 5000
      const t = window.setTimeout(() => setKick((k) => k + 1), ms)
      return () => clearTimeout(t)
    }
    setWaiting(false)
    waitFrom.current = null
    const kind = nextKind
    petStore.set('water.kind', kind)
    petStore.set('water.last', String(Date.now()))
    void api.petNotify(WATER_LINE[kind]).catch(() => undefined)
    plog(`water.${start}`, kind)
    if (start === 'notify') return
    if (start === 'bubble') { setR({ kind, mode: 'bubble' }); return }
    ;(document.activeElement as HTMLElement | null)?.blur?.() // 卷起后输入框看不见了，别再往里打字
    setR({ kind, mode: 'roll', phase: 'call' })
  }, [now, idle, kick, active, hidden, r, boss, collapsed, settings.petEnabled, settings.petWater, settings.petWaterInterval, settings.petWaterStyle, settings.workStart, settings.workEnd, settings.dnd])

  // 卷帘按时间往下走；气泡两分钟没理会就收起
  useEffect(() => {
    if (!r) return
    if (r.mode === 'bubble') {
      const t = window.setTimeout(() => { if (rRef.current === r) plog('water.bubble.timeout', r.kind); setR((x) => (x === r ? null : x)) }, BUBBLE_TIMEOUT)
      return () => clearTimeout(t)
    }
    if (r.phase === 'rolled') return
    const t = window.setTimeout(() => {
      const next = nextRollPhase(r.phase)
      if (next) { setR({ ...r, phase: next }); return }
      setR(null)
      const p = pending.current
      pending.current = null
      if (p) say(p.text, p.happy)
    }, ROLL_MS[r.phase])
    return () => clearTimeout(t)
  }, [r])

  const answer = useCallback((snooze: boolean) => {
    const cur = rRef.current
    if (!cur || (cur.mode === 'roll' && cur.phase !== 'rolled')) return
    plog(snooze ? 'water.snooze' : 'water.done', cur.kind)
    const text = snooze ? '好，那我 10 分钟后再来叫你' : cur.kind === 'water' ? '咕嘟咕嘟，舒服～' : '伸个懒腰，舒服多了'
    if (snooze) petStore.set('water.last', String(snoozeLast(Date.now(), settings.petWaterInterval)))
    const drank = !snooze && cur.kind === 'water' ? api.drinkWater().catch(() => null) : Promise.resolve(null)
    if (cur.mode === 'bubble') {
      setR(null)
      void drank.then(() => say(text, !snooze))
      return
    }
    pending.current = { text, happy: !snooze }
    setR({ ...cur, phase: nextRollPhase('rolled', snooze ? 'snooze' : 'done')! })
    void drank.then((p) => { if (p) setR((x) => (x && x.mode === 'roll' && x.phase === 'finish' ? { ...x, cups: p.waterToday } : x)) })
  }, [settings.petWaterInterval])

  return { r, waiting, cheer, answer }
}

/** 卷帘：便签纸从下往上卷成纸筒停在标题栏下方，露出底下的墙面，狗狗坐在中间提醒你。 */
export function WaterShade({ r, settings, onAnswer }: { r: Extract<WaterReminder, { mode: 'roll' }>; settings: Settings; onAnswer: (snooze: boolean) => void }) {
  const [status, setStatus] = useState<PetStatus | null>(null)
  const [phase, setPhase] = useState<RollPhase>('call') // 先以「平铺」状态渲染一帧，切换后才有过渡动画
  useEffect(() => { void api.petStatus().then(setStatus).catch(() => undefined) }, [])
  useEffect(() => {
    if (r.phase === phase) return
    const id = requestAnimationFrame(() => setPhase(r.phase))
    return () => cancelAnimationFrame(id)
  }, [r.phase, phase])

  const pet = status?.pet
  const name = pet?.name ?? '博士'
  const holiday = settings.petHoliday ? holidayOutfit(new Date()) : null
  const finish = phase === 'finish'
  const cups = r.cups ?? pet?.waterToday ?? 0
  const water = r.kind === 'water'
  return (
    <div className={`rollshade ${phase} ${r.kind}`} data-testid="roll-shade" data-phase={phase} data-kind={r.kind}>
      <div className="rollwall">
        <div className="rollcard" role="alertdialog" aria-label={`${name}提醒你`}>
          <div className="rolldog">
            <Dog mood={finish ? 'happy' : 'normal'} accessory={holiday ? holiday.id : pet?.accessory ?? ''} glasses={pet?.glasses ?? 'round'}
              action={water ? 'water' : finish ? 'shakeoff' : null} effect={finish ? 'hearts' : null} size={64} />
          </div>
          <div className="rolltext">
            {finish ? (
              <div className="rollline" data-testid="roll-finish">{water ? `咕嘟咕嘟…今天第 ${cups} 杯` : '伸——懒——腰～'}</div>
            ) : (
              <>
                <div className="rollline">{name}提醒你：{WATER_LINE[r.kind]}</div>
                {water && <div className="rollsub">今天已经喝了 {cups} 杯</div>}
                <div className="rollbtns">
                  <button className="btn primary" onClick={() => onAnswer(false)} data-testid="roll-done" disabled={phase !== 'rolled'}>{water ? '喝好了' : '活动好了'}</button>
                  <button className="btn" onClick={() => onAnswer(true)} data-testid="roll-snooze" disabled={phase !== 'rolled'}>10 分钟后</button>
                </div>
              </>
            )}
          </div>
        </div>
      </div>
      <div className="rolltube" aria-hidden="true" />
    </div>
  )
}
